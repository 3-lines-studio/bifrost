package bifrost

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/dochtml"
	"github.com/3-lines-studio/bifrost/internal/protocol"
)

type compiledManifest struct {
	manifest    protocol.Manifest
	views       map[string]protocol.BuiltView
	routes      map[string]protocol.BuiltRoute
	files       map[string]protocol.FileRef
	clientFiles map[string]protocol.FileRef
	public      map[string]protocol.FileRef
}

func parseManifest(data []byte) (protocol.Manifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest protocol.Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return protocol.Manifest{}, fmt.Errorf("bifrost: decode manifest: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return protocol.Manifest{}, err
	}
	return manifest, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("bifrost: manifest contains more than one JSON value")
		}
		return fmt.Errorf("bifrost: decode manifest suffix: %w", err)
	}
	return nil
}

func validateManifest(assets fs.FS, spec protocol.Spec, specHash string, manifest protocol.Manifest) (*compiledManifest, error) {
	if assets == nil {
		return nil, errors.New("bifrost: nil asset filesystem")
	}
	if manifest.Schema != protocol.Schema {
		return nil, fmt.Errorf("bifrost: manifest schema %d does not match supported schema %d", manifest.Schema, protocol.Schema)
	}
	if !validDigest(manifest.BuildID) {
		return nil, fmt.Errorf("bifrost: invalid build ID %q", manifest.BuildID)
	}
	if manifest.SpecHash != specHash {
		return nil, fmt.Errorf("bifrost: stale manifest: spec hash %q does not match app %q", manifest.SpecHash, specHash)
	}
	if manifest.Toolchain.Bifrost == "" || manifest.Toolchain.Bun == "" || manifest.Toolchain.Vite == "" || manifest.Toolchain.React == "" {
		return nil, errors.New("bifrost: manifest toolchain is incomplete")
	}
	if !supportedMajorVersion(manifest.Toolchain.Vite, 8) {
		return nil, fmt.Errorf("bifrost: unsupported Vite version %q; Bifrost requires Vite 8", manifest.Toolchain.Vite)
	}
	if !supportedReactVersion(manifest.Toolchain.React) {
		return nil, fmt.Errorf("bifrost: unsupported React version %q; V1 requires React 19", manifest.Toolchain.React)
	}

	views := make(map[string]protocol.BuiltView, len(manifest.Views))
	files := make(map[string]protocol.FileRef)
	clientFiles := make(map[string]protocol.FileRef, len(manifest.ClientFiles))
	for _, file := range manifest.ClientFiles {
		if err := validateFileRef(file); err != nil {
			return nil, fmt.Errorf("bifrost: invalid client file: %w", err)
		}
		if err := collectFile(clientFiles, file); err != nil {
			return nil, err
		}
		if err := collectFile(files, file); err != nil {
			return nil, err
		}
	}
	for i, view := range manifest.Views {
		if err := validateBuiltView(view); err != nil {
			return nil, fmt.Errorf("bifrost: manifest view %d: %w", i, err)
		}
		if _, exists := views[view.ID]; exists {
			return nil, fmt.Errorf("bifrost: duplicate manifest view ID %q", view.ID)
		}
		views[view.ID] = view
		for _, file := range viewFiles(view) {
			if err := collectFile(files, file); err != nil {
				return nil, fmt.Errorf("bifrost: view %q: %w", view.ID, err)
			}
		}
		clientRefs := append([]protocol.FileRef{view.Client.Entry}, view.Client.Styles...)
		clientRefs = append(clientRefs, view.Client.Imports...)
		for _, file := range clientRefs {
			registered, exists := clientFiles[file.Path]
			if !exists || registered != file {
				return nil, fmt.Errorf("bifrost: view %q client file %q is not registered", view.ID, file.Path)
			}
		}
	}

	specRoutes := make(map[string]protocol.RouteSpec, len(spec.Routes))
	for _, route := range spec.Routes {
		specRoutes[route.Pattern] = route
	}
	routes := make(map[string]protocol.BuiltRoute, len(manifest.Routes))
	needsRuntime := false
	for i, route := range manifest.Routes {
		declared, exists := specRoutes[route.Pattern]
		if !exists {
			return nil, fmt.Errorf("bifrost: manifest route %q is not declared by the app", route.Pattern)
		}
		if _, exists := routes[route.Pattern]; exists {
			return nil, fmt.Errorf("bifrost: duplicate manifest route %q", route.Pattern)
		}
		if route.Kind == "server" {
			needsRuntime = true
		}
		if route.Kind != declared.Kind {
			return nil, fmt.Errorf("bifrost: manifest route %q kind %q does not match app kind %q", route.Pattern, route.Kind, declared.Kind)
		}
		view, exists := views[route.ViewID]
		if !exists {
			return nil, fmt.Errorf("bifrost: manifest route %q references unknown view %q", route.Pattern, route.ViewID)
		}
		if view.Source != declared.View {
			return nil, fmt.Errorf("bifrost: manifest route %q view source %q does not match app source %q", route.Pattern, view.Source, declared.View)
		}
		if err := validateViewForRoute(view, route.Kind); err != nil {
			return nil, fmt.Errorf("bifrost: manifest route %q: %w", route.Pattern, err)
		}
		if err := validateDocuments(route); err != nil {
			return nil, fmt.Errorf("bifrost: manifest route %d: %w", i, err)
		}
		for _, document := range route.Documents {
			if err := collectFile(files, document.File); err != nil {
				return nil, fmt.Errorf("bifrost: route %q document %q: %w", route.Pattern, document.Path, err)
			}
		}
		routes[route.Pattern] = route
	}
	if len(routes) != len(specRoutes) {
		missing := make([]string, 0, len(specRoutes)-len(routes))
		for pattern := range specRoutes {
			if _, exists := routes[pattern]; !exists {
				missing = append(missing, pattern)
			}
		}
		slices.Sort(missing)
		return nil, fmt.Errorf("bifrost: manifest is missing routes: %s", strings.Join(missing, ", "))
	}

	if needsRuntime && manifest.Runtime == nil && os.Getenv("BIFROST_DEV_DIR") == "" {
		return nil, errors.New("bifrost: server routes require an embedded renderer runtime")
	}
	if manifest.Runtime != nil {
		if manifest.RuntimeCompression != "" && manifest.RuntimeCompression != "gzip" {
			return nil, fmt.Errorf("bifrost: unsupported runtime compression %q", manifest.RuntimeCompression)
		}
		if err := validateFileRef(*manifest.Runtime); err != nil {
			return nil, fmt.Errorf("bifrost: invalid renderer runtime: %w", err)
		}
		if err := collectFile(files, *manifest.Runtime); err != nil {
			return nil, err
		}
	}

	public := make(map[string]protocol.FileRef, len(manifest.Public))
	for _, asset := range manifest.Public {
		if err := validatePublicURL(asset.URL); err != nil {
			return nil, err
		}
		if _, exists := public[asset.URL]; exists {
			return nil, fmt.Errorf("bifrost: duplicate public URL %q", asset.URL)
		}
		if err := validateFileRef(asset.File); err != nil {
			return nil, fmt.Errorf("bifrost: public URL %q: %w", asset.URL, err)
		}
		if err := collectFile(files, asset.File); err != nil {
			return nil, err
		}
		public[asset.URL] = asset.File
	}

	if err := verifyFiles(assets, files); err != nil {
		return nil, err
	}
	return &compiledManifest{manifest: manifest, views: views, routes: routes, files: files, clientFiles: clientFiles, public: public}, nil
}

func validatePublicURL(value string) error {
	if err := validateDocumentPath(value); err != nil {
		return fmt.Errorf("bifrost: invalid public URL %q: %w", value, err)
	}
	if strings.ContainsAny(value, "{}") {
		return fmt.Errorf("bifrost: public URL %q contains reserved characters", value)
	}
	if value == "/" || strings.HasPrefix(value, dochtml.AssetPrefix) {
		return fmt.Errorf("bifrost: reserved public URL %q", value)
	}
	return nil
}

func supportedMajorVersion(version string, major int) bool {
	version = strings.TrimSpace(version)
	prefix := fmt.Sprintf("%d", major)
	return version == prefix || strings.HasPrefix(version, prefix+".")
}

func supportedReactVersion(version string) bool {
	version = strings.TrimSpace(version)
	for len(version) > 0 && (version[0] < '0' || version[0] > '9') {
		version = version[1:]
	}
	return version == "19" || strings.HasPrefix(version, "19.")
}

func validateBuiltView(view protocol.BuiltView) error {
	if !validDigest(view.ID) {
		return fmt.Errorf("invalid view ID %q", view.ID)
	}
	if view.Source == "" || path.Clean(view.Source) != view.Source || !fs.ValidPath(view.Source) {
		return fmt.Errorf("invalid source path %q", view.Source)
	}
	if view.Mode != "hydrate" && view.Mode != "mount" {
		return fmt.Errorf("invalid mode %q", view.Mode)
	}
	if err := validateFileRef(view.Client.Entry); err != nil {
		return fmt.Errorf("invalid client entry: %w", err)
	}
	related := append(slices.Clone(view.Client.Styles), view.Client.Imports...)
	for _, file := range related {
		if err := validateFileRef(file); err != nil {
			return err
		}
	}
	if view.Server != nil {
		if err := validateFileRef(view.Server.Entry); err != nil {
			return fmt.Errorf("invalid server entry: %w", err)
		}
		for _, file := range view.Server.Imports {
			if err := validateFileRef(file); err != nil {
				return fmt.Errorf("invalid server import: %w", err)
			}
		}
	}
	return nil
}

func validateViewForRoute(view protocol.BuiltView, kind string) error {
	switch kind {
	case "server":
		if view.Mode != "hydrate" || (view.Server == nil && os.Getenv("BIFROST_DEV_DIR") == "") {
			return errors.New("server route requires a hydrate view with a server entry")
		}
	case "static":
		if view.Mode != "hydrate" {
			return errors.New("static route requires a hydrate view")
		}
	case "client":
		if view.Mode != "mount" || view.Server != nil {
			return errors.New("client route requires a mount view without a server entry")
		}
	default:
		return fmt.Errorf("unknown route kind %q", kind)
	}
	return nil
}

func validateDocuments(route protocol.BuiltRoute) error {
	if route.Kind != "static" && len(route.Documents) != 0 {
		return fmt.Errorf("non-static route %q contains static documents", route.Pattern)
	}
	seen := make(map[string]struct{}, len(route.Documents))
	for _, document := range route.Documents {
		if err := validateDocumentPath(document.Path); err != nil {
			return err
		}
		if _, exists := seen[document.Path]; exists {
			return fmt.Errorf("duplicate static document path %q", document.Path)
		}
		seen[document.Path] = struct{}{}
		if !pathMatchesPattern(route.Pattern, document.Path) {
			return fmt.Errorf("static document path %q does not match pattern %q", document.Path, route.Pattern)
		}
		if err := validateFileRef(document.File); err != nil {
			return err
		}
		if len(document.Props) > 0 && !json.Valid(document.Props) {
			return fmt.Errorf("static document %q has invalid props JSON", document.Path)
		}
		normalized, err := normalizeDocument(documentFromProtocol(document.Document))
		if err != nil || protocolDocument(normalized) != document.Document {
			return fmt.Errorf("static document %q has invalid document attributes", document.Path)
		}
	}
	if route.Kind == "static" && !patternIsDynamic(route.Pattern) && len(route.Documents) != 1 {
		return fmt.Errorf("exact static route %q must contain one document", route.Pattern)
	}
	return nil
}

func validateDocumentPath(value string) error {
	if value == "" || !strings.HasPrefix(value, "/") {
		return fmt.Errorf("static document path %q must be absolute", value)
	}
	if strings.ContainsAny(value, "?#") || strings.Contains(value, "//") {
		return fmt.Errorf("invalid static document path %q", value)
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Path != value {
		return fmt.Errorf("invalid static document path %q", value)
	}
	clean := path.Clean(value)
	if value != "/" && clean != value && clean+"/" != value {
		return fmt.Errorf("static document path %q is not clean", value)
	}
	return nil
}

func pathMatchesPattern(pattern, requestPath string) bool {
	mux := http.NewServeMux()
	mux.Handle("GET "+pattern, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	request := &http.Request{Method: http.MethodGet, URL: &url.URL{Path: requestPath}}
	_, matched := mux.Handler(request)
	return matched == "GET "+pattern
}

func validateFileRef(file protocol.FileRef) error {
	if file.Path == "" || file.Path == "." || !fs.ValidPath(file.Path) || path.Clean(file.Path) != file.Path {
		return fmt.Errorf("invalid artifact path %q", file.Path)
	}
	if file.Size < 0 {
		return fmt.Errorf("artifact %q has negative size", file.Path)
	}
	if !validDigest(file.Hash) {
		return fmt.Errorf("artifact %q has invalid SHA-256 hash %q", file.Path, file.Hash)
	}
	return nil
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func viewFiles(view protocol.BuiltView) []protocol.FileRef {
	serverFiles := 0
	if view.Server != nil {
		serverFiles = 1 + len(view.Server.Imports)
	}
	files := make([]protocol.FileRef, 0, 1+len(view.Client.Styles)+len(view.Client.Imports)+serverFiles)
	files = append(files, view.Client.Entry)
	files = append(files, view.Client.Styles...)
	files = append(files, view.Client.Imports...)
	if view.Server != nil {
		files = append(files, view.Server.Entry)
		files = append(files, view.Server.Imports...)
	}
	return files
}

func collectFile(files map[string]protocol.FileRef, file protocol.FileRef) error {
	if existing, exists := files[file.Path]; exists {
		if existing != file {
			return fmt.Errorf("artifact %q has conflicting metadata", file.Path)
		}
		return nil
	}
	files[file.Path] = file
	return nil
}

func verifyFiles(assets fs.FS, files map[string]protocol.FileRef) error {
	paths := make([]string, 0, len(files))
	for filePath := range files {
		paths = append(paths, filePath)
	}
	slices.Sort(paths)
	for _, filePath := range paths {
		ref := files[filePath]
		file, err := assets.Open(filePath)
		if err != nil {
			return fmt.Errorf("bifrost: open artifact %q: %w", filePath, err)
		}
		hash := sha256.New()
		size, copyErr := io.Copy(hash, bufio.NewReader(file))
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("bifrost: hash artifact %q: %w", filePath, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("bifrost: close artifact %q: %w", filePath, closeErr)
		}
		if size != ref.Size {
			return fmt.Errorf("bifrost: artifact %q size %d does not match manifest size %d", filePath, size, ref.Size)
		}
		actualHash := hex.EncodeToString(hash.Sum(nil))
		if actualHash != ref.Hash {
			return fmt.Errorf("bifrost: artifact %q hash %s does not match manifest hash %s", filePath, actualHash, ref.Hash)
		}
	}
	return nil
}

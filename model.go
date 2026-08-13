package bifrost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

// Config is the complete input to New. Its slices are copied during app
// construction.
type Config struct {
	// SourceRoot is the directory against which frontend view paths resolve. An
	// empty value means the current directory.
	SourceRoot string
	Routes     []Route
	AppPlugins []AppPlugin

	// Assets contains a production manifest and its referenced files. When it
	// is nil, New creates a declaration-only app for build and development
	// phases.
	Assets fs.FS

	// RenderConcurrency is the number of isolated production renderer processes.
	// Each process handles one render at a time. Zero uses one process. Development
	// always uses one process because it owns one Vite module graph.
	RenderConcurrency int
	// RenderQueue bounds requests waiting for renderer capacity. Zero uses 64.
	RenderQueue int

	Limits Limits
	Logger *slog.Logger
}

// App is an immutable, validated application model. Runtime services are added
// to this type after model compilation succeeds.
type App struct {
	sourceRoot string
	routes     []Route
	hooks      registeredHooks
	spec       protocol.Spec
	specHash   string
	runtime    *runtimeState
	appPlugins []string
	limits     Limits
	logger     *slog.Logger
}

// New validates declarations, runs plugin registration, and starts the
// production runtime. Build phases may omit Assets; normal applications may
// not.
func New(config Config) (*App, error) {
	app, err := newApp(config)
	if err != nil {
		return nil, err
	}
	if config.Assets == nil && !Building() {
		return nil, errors.New("bifrost: Config.Assets is required outside build phases")
	}
	return app, nil
}

func newApp(config Config) (*App, error) {
	state := &registryState{}
	seenPlugins := make(map[string]struct{}, len(config.AppPlugins))

	for i, plugin := range config.AppPlugins {
		if plugin == nil {
			return nil, fmt.Errorf("bifrost: plugin %d is nil", i)
		}
		name := strings.TrimSpace(plugin.Name())
		if name == "" {
			return nil, fmt.Errorf("bifrost: plugin %d has an empty name", i)
		}
		if _, exists := seenPlugins[name]; exists {
			return nil, fmt.Errorf("bifrost: duplicate plugin name %q", name)
		}
		seenPlugins[name] = struct{}{}

		registry := &AppRegistry{pluginName: name, state: state}
		if err := plugin.Register(registry); err != nil {
			registry.sealed = true
			return nil, fmt.Errorf("bifrost: register plugin %q: %w", name, err)
		}
		registry.sealed = true
	}

	routes := make([]Route, 0, len(config.Routes)+len(state.routes))
	routes = append(routes, config.Routes...)
	routes = append(routes, state.routes...)

	sourceRoot := config.SourceRoot
	if sourceRoot == "" {
		sourceRoot = "."
	}
	absRoot, err := filepath.Abs(sourceRoot)
	if err != nil {
		return nil, fmt.Errorf("bifrost: resolve source root: %w", err)
	}

	normalizedRoutes, err := validateRoutes(absRoot, routes)
	if err != nil {
		return nil, err
	}
	spec, specHash, err := makeBuildSpec(normalizedRoutes)
	if err != nil {
		return nil, err
	}

	limits, err := normalizeLimits(config.Limits)
	if err != nil {
		return nil, err
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	pluginNames := make([]string, 0, len(seenPlugins))
	for name := range seenPlugins {
		pluginNames = append(pluginNames, name)
	}
	slices.Sort(pluginNames)
	app := &App{
		sourceRoot: absRoot,
		routes:     slices.Clone(normalizedRoutes),
		hooks:      cloneHooks(state.hooks),
		spec:       spec,
		specHash:   specHash,
		appPlugins: pluginNames,
		limits:     limits,
		logger:     logger,
	}
	if err := app.handleBuildPhase(); err != nil {
		return nil, err
	}
	if os.Getenv(buildPhaseEnv) != "" {
		return app, nil
	}
	if config.Assets != nil {
		if err := app.initializeProduction(config); err != nil {
			return nil, err
		}
	}
	return app, nil
}

// MustNew is New with panic-on-error behavior for programs that treat invalid
// app declarations as programmer errors.
func MustNew(config Config) *App {
	app, err := New(config)
	if err != nil {
		panic(err)
	}
	return app
}

func cloneHooks(src registeredHooks) registeredHooks {
	return registeredHooks{
		middleware:    slices.Clone(src.middleware),
		errorHandler:  src.errorHandler,
		assetHeaders:  src.assetHeaders,
		loadHooks:     slices.Clone(src.loadHooks),
		queueHooks:    slices.Clone(src.queueHooks),
		renderHooks:   slices.Clone(src.renderHooks),
		responseHooks: slices.Clone(src.responseHooks),
	}
}

func validateRoutes(sourceRoot string, routes []Route) ([]Route, error) {
	normalized := make([]Route, len(routes))
	seen := make(map[string]struct{}, len(routes))

	for i, route := range routes {
		if route.kind == routeInvalid {
			return nil, fmt.Errorf("bifrost: route %d was not created by Server, Static, or Client", i)
		}
		if err := validatePattern(route.pattern); err != nil {
			return nil, fmt.Errorf("bifrost: route %d: %w", i, err)
		}
		if _, exists := seen[route.pattern]; exists {
			return nil, fmt.Errorf("bifrost: duplicate route pattern %q", route.pattern)
		}
		seen[route.pattern] = struct{}{}

		view, err := normalizeView(sourceRoot, route.view)
		if err != nil {
			return nil, fmt.Errorf("bifrost: route %q: %w", route.pattern, err)
		}
		route.view = view

		if route.kind == routeStatic && patternIsDynamic(route.pattern) && route.generate == nil {
			return nil, fmt.Errorf("bifrost: dynamic static route %q requires a generator", route.pattern)
		}
		normalized[i] = route
	}

	if err := validateMuxConflicts(normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func validatePattern(pattern string) error {
	if pattern == "" {
		return errors.New("route pattern is empty")
	}
	if strings.TrimSpace(pattern) != pattern {
		return fmt.Errorf("route pattern %q has leading or trailing whitespace", pattern)
	}
	if !strings.HasPrefix(pattern, "/") {
		return fmt.Errorf("route pattern %q must start with /", pattern)
	}
	if pattern == "/_bifrost" || strings.HasPrefix(pattern, "/_bifrost/") {
		return fmt.Errorf("route pattern %q uses the reserved Bifrost asset prefix", pattern)
	}
	if strings.ContainsAny(pattern, "?#") {
		return fmt.Errorf("route pattern %q must not contain a query or fragment", pattern)
	}
	return nil
}

func validateMuxConflicts(routes []Route) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("bifrost: invalid route patterns: %v", recovered)
		}
	}()

	mux := http.NewServeMux()
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	for _, route := range routes {
		mux.Handle("GET "+route.pattern, handler)
	}
	return nil
}

func normalizeView(sourceRoot, view string) (string, error) {
	if view == "" {
		return "", errors.New("view path is empty")
	}
	if strings.TrimSpace(view) != view {
		return "", fmt.Errorf("view path %q has leading or trailing whitespace", view)
	}
	if strings.ContainsRune(view, '\\') {
		return "", fmt.Errorf("view path %q must use forward slashes", view)
	}
	if path.IsAbs(view) {
		return "", fmt.Errorf("view path %q must be relative", view)
	}

	clean := path.Clean(view)
	switch strings.ToLower(path.Ext(clean)) {
	case ".tsx", ".jsx", ".ts", ".js":
	default:
		return "", fmt.Errorf("view path %q must use .tsx, .jsx, .ts, or .js", view)
	}
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("view path %q escapes the source root", view)
	}

	absView := filepath.Join(sourceRoot, filepath.FromSlash(clean))
	rel, err := filepath.Rel(sourceRoot, absView)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("view path %q escapes the source root", view)
	}
	return clean, nil
}

func patternIsDynamic(pattern string) bool {
	return strings.Contains(strings.ReplaceAll(pattern, "{$}", ""), "{")
}

func makeBuildSpec(routes []Route) (protocol.Spec, string, error) {
	spec := protocol.Spec{
		Schema: protocol.Schema,
		Routes: make([]protocol.RouteSpec, len(routes)),
	}
	for i, route := range routes {
		spec.Routes[i] = protocol.RouteSpec{
			Pattern: route.pattern,
			View:    route.view,
			Kind:    route.kind.String(),
		}
	}

	slices.SortFunc(spec.Routes, func(a, b protocol.RouteSpec) int {
		return strings.Compare(a.Pattern, b.Pattern)
	})
	data, err := json.Marshal(spec)
	if err != nil {
		return protocol.Spec{}, "", fmt.Errorf("bifrost: encode build spec: %w", err)
	}
	digest := sha256.Sum256(data)
	return spec, hex.EncodeToString(digest[:]), nil
}

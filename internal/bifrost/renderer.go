package bifrost

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type BifrostError struct {
	Message string
	Stack   string
	Errors  []BifrostErrorDetail
}

func (e *BifrostError) Error() string {
	return e.Message
}

type BifrostErrorDetail struct {
	Message   string
	File      string
	Line      int
	Column    int
	LineText  string
	Specifier string
	Referrer  string
}

type Renderer struct {
	cmd           *exec.Cmd
	socket        string
	client        *http.Client
	renderCache   *renderCache
	AssetsFS      embed.FS
	timingEnabled bool
	logger        *slog.Logger
	ssrTempDir    string
}

func NewRenderer() (*Renderer, error) {
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("bifrost-%d.sock", os.Getpid()))

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	rendererSource := BunRendererProdSource
	if IsDev() {
		rendererSource = BunRendererDevSource
	}

	cmd := exec.Command("bun", "run", "--smol", "-")
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "BIFROST_SOCKET="+socket)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = strings.NewReader(rendererSource)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start bun: %w", err)
	}

	if err := waitForSocket(socket, 5*time.Second); err != nil {
		cmd.Process.Kill()
		return nil, err
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("unix", socket)
		},
	}
	client := &http.Client{Transport: transport}

	return &Renderer{
		cmd:         cmd,
		socket:      socket,
		client:      client,
		renderCache: newRenderCache(5 * time.Minute),
	}, nil
}

func (r *Renderer) SetTimingEnabled(enabled bool) {
	r.timingEnabled = enabled
}

type renderResponse struct {
	HTML  string `json:"html"`
	Head  string `json:"head"`
	Error *struct {
		Message string `json:"message"`
		Stack   string `json:"stack"`
		Errors  []struct {
			Message string `json:"message"`
			Stack   string `json:"stack"`
		} `json:"errors"`
	} `json:"error"`
}

func (r *Renderer) Render(componentPath string, props map[string]interface{}) (renderedPage, error) {
	propsHash, err := hashProps(props)
	if err != nil {
		return renderedPage{}, err
	}
	cacheKey := componentPath + ":" + propsHash

	if cached, found := r.renderCache.get(cacheKey); found {
		return cached, nil
	}

	reqBody := map[string]interface{}{
		"path":  componentPath,
		"props": props,
	}

	var result renderResponse
	if err := r.postJSON("/render", reqBody, &result); err != nil {
		return renderedPage{}, err
	}

	if result.Error != nil {
		var sb strings.Builder
		sb.WriteString(result.Error.Message)

		if len(result.Error.Errors) > 0 {
			sb.WriteString("\n\nErrors:")
			for i, err := range result.Error.Errors {
				sb.WriteString(fmt.Sprintf("\n  %d. %s", i+1, err.Message))
				if err.Stack != "" {
					sb.WriteString(fmt.Sprintf("\n     Stack: %s", err.Stack))
				}
			}
		}

		if result.Error.Stack != "" {
			sb.WriteString(fmt.Sprintf("\n\nStack:\n%s", result.Error.Stack))
		}

		return renderedPage{}, fmt.Errorf("%s", sb.String())
	}

	page := renderedPage{
		Body: result.HTML,
		Head: result.Head,
	}

	r.renderCache.set(cacheKey, page)
	return page, nil
}

func (r *Renderer) postJSON(endpoint string, body interface{}, result interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", "http://localhost"+endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return json.NewDecoder(resp.Body).Decode(result)
}

type errorPosition struct {
	LineText string `json:"lineText"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
}

type buildResponse struct {
	OK    bool `json:"ok"`
	Error *struct {
		Message string `json:"message"`
		Stack   string `json:"stack"`
		Errors  []struct {
			Message   string        `json:"message"`
			Position  errorPosition `json:"position"`
			Specifier string        `json:"specifier"`
			Referrer  string        `json:"referrer"`
		} `json:"errors"`
	} `json:"error"`
}

func (r *Renderer) Build(entrypoints []string, outdir string) error {
	if len(entrypoints) == 0 {
		return fmt.Errorf("missing entrypoints")
	}

	if outdir == "" {
		return fmt.Errorf("missing outdir")
	}

	reqBody := map[string]interface{}{
		"entrypoints": entrypoints,
		"outdir":      outdir,
	}

	var result buildResponse
	if err := r.postJSON("/build", reqBody, &result); err != nil {
		return err
	}

	if result.Error != nil {
		errors := make([]BifrostErrorDetail, len(result.Error.Errors))
		for i, err := range result.Error.Errors {
			errors[i] = BifrostErrorDetail{
				Message:   err.Message,
				File:      err.Position.File,
				Line:      err.Position.Line,
				Column:    err.Position.Column,
				LineText:  err.Position.LineText,
				Specifier: err.Specifier,
				Referrer:  err.Referrer,
			}
		}
		return &BifrostError{
			Message: result.Error.Message,
			Stack:   result.Error.Stack,
			Errors:  errors,
		}
	}

	if !result.OK {
		return fmt.Errorf("build failed for entrypoints %v -> %s", entrypoints, outdir)
	}

	return nil
}

func (r *Renderer) Stop() error {
	if r.ssrTempDir != "" {
		os.RemoveAll(r.ssrTempDir)
	}
	return r.cmd.Process.Kill()
}

func (r *Renderer) ClearCache() {
	r.renderCache.clear()
}

type pagePaths struct {
	entryDir     string
	publicDir    string
	outdir       string
	entryName    string
	entryPath    string
	manifestPath string
}

func calculatePagePaths(componentPath string) pagePaths {
	entryDir := ".bifrost"
	publicDir := entryDir
	outdir := filepath.Join(publicDir, "dist")
	entryName := EntryNameForPath(componentPath)
	entryPath := filepath.Join(entryDir, entryName+".tsx")
	manifestPath := filepath.Join(entryDir, "manifest.json")

	return pagePaths{
		entryDir:     entryDir,
		publicDir:    publicDir,
		outdir:       outdir,
		entryName:    entryName,
		entryPath:    entryPath,
		manifestPath: manifestPath,
	}
}

func (r *Renderer) loadManifestForMode(paths pagePaths, isDev bool) *buildManifest {
	if isDev || r.AssetsFS == (embed.FS{}) {
		man, _ := loadManifest(paths.manifestPath)
		return man
	}
	man, _ := r.loadManifestFromEmbed(paths.manifestPath)
	return man
}

func buildOptions(componentPath string, opts ...interface{}) options {
	var loader propsLoader
	pageOpts := []PageOption{}

	for _, opt := range opts {
		switch o := opt.(type) {
		case propsLoader:
			loader = o
		case func(*http.Request) (map[string]interface{}, error):
			loader = o
		case PageOption:
			pageOpts = append(pageOpts, o)
		}
	}

	result := options{
		ComponentPath: componentPath,
		PropsLoader:   loader,
	}

	for _, opt := range pageOpts {
		opt(&result)
	}

	return result
}

func defaultPropsLoader(loader propsLoader) propsLoader {
	if loader != nil {
		return loader
	}
	return func(*http.Request) (map[string]interface{}, error) {
		return map[string]interface{}{}, nil
	}
}

func (r *Renderer) NewPage(componentPath string, opts ...interface{}) *Page {
	return createPage(r, componentPath, opts...)
}

func createPage(r *Renderer, componentPath string, opts ...interface{}) *Page {
	options := buildOptions(componentPath, opts...)
	paths := calculatePagePaths(componentPath)

	var man *buildManifest
	if r != nil {
		man = r.loadManifestForMode(paths, IsDev())
	}

	scriptSrc, cssHref, chunks, isStaticFromManifest, ssrPath := getAssetsFromManifest(man, paths.entryName)
	isDev := IsDev()

	isStatic := options.Static || isStaticFromManifest

	var staticPath string
	if isStatic && !isDev {
		staticPath = filepath.Join(BifrostDir, "pages", paths.entryName, "index.html")
	}

	needsSetup := (man == nil || isDev) && !isStatic
	if isStatic && isDev && r != nil {
		needsSetup = true
	}

	return &Page{
		renderer:    r,
		opts:        options,
		propsLoader: defaultPropsLoader(options.PropsLoader),
		entryDir:    paths.entryDir,
		outdir:      paths.outdir,
		entryPath:   paths.entryPath,
		entryName:   paths.entryName,
		scriptSrc:   scriptSrc,
		cssHref:     cssHref,
		chunks:      chunks,
		manifest:    man,
		isDev:       isDev,
		needsSetup:  needsSetup,
		staticPath:  staticPath,
		ssrPath:     ssrPath,
	}
}

func validateSetupInputs(r *Renderer, opts options) error {
	if r == nil {
		return fmt.Errorf("missing renderer")
	}

	if opts.ComponentPath == "" {
		return fmt.Errorf("missing component path")
	}

	return nil
}

func (r *Renderer) setupPage(opts options, entryDir string, outdir string, entryPath string) error {
	if err := validateSetupInputs(r, opts); err != nil {
		return err
	}

	componentImport, err := ComponentImportPath(entryPath, opts.ComponentPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		return err
	}

	if err := os.MkdirAll(outdir, 0o755); err != nil {
		return err
	}

	if err := writeClientEntry(entryPath, componentImport); err != nil {
		return err
	}

	if err := r.Build([]string{entryPath}, outdir); err != nil {
		return err
	}

	return nil
}

func (r *Renderer) loadManifestFromEmbed(path string) (*buildManifest, error) {
	embedPath := filepath.ToSlash(path)
	data, err := r.AssetsFS.ReadFile(embedPath)
	if err != nil {
		return nil, err
	}
	return parseManifest(data)
}

func (r *Renderer) SetAssetsFS(fs embed.FS) {
	r.AssetsFS = fs
}

func (r *Renderer) extractSSRBundles() (string, error) {
	if r.ssrTempDir != "" {
		return r.ssrTempDir, nil
	}

	tempDir, err := os.MkdirTemp("", "bifrost-ssr-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir for SSR bundles: %w", err)
	}

	ssrDir := filepath.Join(".bifrost", "ssr")
	entries, err := r.AssetsFS.ReadDir(ssrDir)
	if err != nil {
		return tempDir, nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		data, err := r.AssetsFS.ReadFile(filepath.Join(ssrDir, entry.Name()))
		if err != nil {
			continue
		}

		if err := os.WriteFile(filepath.Join(tempDir, entry.Name()), data, 0644); err != nil {
			os.RemoveAll(tempDir)
			return "", fmt.Errorf("failed to write SSR bundle %s: %w", entry.Name(), err)
		}
	}

	r.ssrTempDir = tempDir
	return tempDir, nil
}

func (r *Renderer) getSSRBundlePath(ssrManifestPath string) string {
	if ssrManifestPath == "" {
		return ""
	}

	if r.AssetsFS == (embed.FS{}) {
		return filepath.Join(".bifrost", ssrManifestPath)
	}

	tempDir, err := r.extractSSRBundles()
	if err != nil {
		return ""
	}

	bundleName := filepath.Base(ssrManifestPath)
	return filepath.Join(tempDir, bundleName)
}

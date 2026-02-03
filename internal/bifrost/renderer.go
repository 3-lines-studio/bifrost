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
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

var activeBuildWatchers sync.Map

type Renderer struct {
	cmd           *exec.Cmd
	socket        string
	client        *http.Client
	renderCache   *renderCache
	AssetsFS      embed.FS
	timingEnabled bool
	logger        *slog.Logger
}

func NewRenderer() (*Renderer, error) {
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("bifrost-%d.sock", os.Getpid()))

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	cmd := exec.Command("bun", "run", "--smol", "-")
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "BIFROST_SOCKET="+socket)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = strings.NewReader(BunRendererSource)

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
		fullError := result.Error.Message
		if result.Error.Stack != "" {
			fullError = result.Error.Stack
		}
		return renderedPage{}, fmt.Errorf("%s", fullError)
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

type buildResponse struct {
	OK    bool `json:"ok"`
	Error *struct {
		Message string `json:"message"`
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
		return fmt.Errorf("build error: %s", result.Error.Message)
	}

	if !result.OK {
		return fmt.Errorf("build failed")
	}

	return nil
}

func (r *Renderer) Stop() error {
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

func buildOptions(componentPath string, loaders ...propsLoader) options {
	var loader propsLoader
	if len(loaders) > 0 {
		loader = loaders[0]
	}

	isDev := IsDev()
	return options{
		ComponentPath: componentPath,
		PropsLoader:   loader,
		Watch:         isDev,
	}
}

func defaultPropsLoader(loader propsLoader) propsLoader {
	if loader != nil {
		return loader
	}
	return func(*http.Request) (map[string]interface{}, error) {
		return map[string]interface{}{}, nil
	}
}

func (r *Renderer) NewPage(componentPath string, loaders ...propsLoader) *Page {
	opts := buildOptions(componentPath, loaders...)
	paths := calculatePagePaths(componentPath)
	man := r.loadManifestForMode(paths, IsDev())
	scriptSrc, cssHref, chunks := getAssetsFromManifest(man, paths.entryName)
	isDev := IsDev()

	return &Page{
		renderer:    r,
		opts:        opts,
		propsLoader: defaultPropsLoader(opts.PropsLoader),
		entryDir:    paths.entryDir,
		outdir:      paths.outdir,
		entryPath:   paths.entryPath,
		entryName:   paths.entryName,
		scriptSrc:   scriptSrc,
		cssHref:     cssHref,
		chunks:      chunks,
		manifest:    man,
		isDev:       isDev,
		needsSetup:  man == nil || isDev,
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

	if opts.Watch {
		watchDir := opts.WatchDir
		if watchDir == "" {
			watchDir = "."
		}
		r.startBuildWatcher(entryPath, outdir, watchDir)
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

type debouncer struct {
	timer *time.Timer
	delay time.Duration
	fn    func()
}

func newDebouncer(delay time.Duration, fn func()) *debouncer {
	return &debouncer{delay: delay, fn: fn}
}

func (d *debouncer) trigger() {
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.delay, d.fn)
}

func (r *Renderer) startBuildWatcher(entryPath string, outdir string, watchDir string) {
	key := entryPath + "::" + outdir + "::" + watchDir
	if _, loaded := activeBuildWatchers.LoadOrStore(key, struct{}{}); loaded {
		return
	}

	go func() {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return
		}
		defer watcher.Close()

		if err := watchDirs(watcher, watchDir); err != nil {
			return
		}

		debounce := newDebouncer(200*time.Millisecond, func() {
			if err := r.Build([]string{entryPath}, outdir); err == nil {
				r.ClearCache()
				if events := GetReloadEvents(); events != nil {
					events.notify()
				}
			}
		})

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if !isWatchEvent(event.Op) {
					continue
				}

				if shouldAddWatchDir(event) {
					_ = watchDirs(watcher, event.Name)
					continue
				}

				if !ShouldRebuildForPath(event.Name) {
					continue
				}

				debounce.trigger()
			case <-watcher.Errors:
			}
		}
	}()
}

func (r *Renderer) SetAssetsFS(fs embed.FS) {
	r.AssetsFS = fs
}

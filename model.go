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

type Config struct {
	SourceRoot string
	Routes     []Route
	AppPlugins []AppPlugin

	Assets fs.FS

	RenderConcurrency int
	RenderQueue       int

	Limits Limits
	Logger *slog.Logger
}

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

		for layoutIndex, layout := range route.layouts {
			normalized, err := normalizeView(sourceRoot, layout)
			if err != nil {
				return nil, fmt.Errorf("bifrost: route %q layout: %w", route.pattern, err)
			}
			if !htmlView(normalized) {
				return nil, fmt.Errorf("bifrost: route %q layout %q must use .html", route.pattern, layout)
			}
			route.layouts[layoutIndex] = normalized
		}

		template := htmlView(route.view)
		if template {
			if route.kind != routeServer {
				return nil, fmt.Errorf("bifrost: route %q must be a server route to use an .html view", route.pattern)
			}
			if route.navigation {
				return nil, fmt.Errorf("bifrost: route %q cannot enable navigation with an .html view", route.pattern)
			}
		} else if len(route.layouts) > 0 {
			return nil, fmt.Errorf("bifrost: route %q uses layouts with a non-html view", route.pattern)
		}

		if route.script != "" {
			if !template {
				return nil, fmt.Errorf("bifrost: route %q uses a script with a non-html view", route.pattern)
			}
			switch strings.ToLower(path.Ext(route.script)) {
			case ".js", ".ts":
			default:
				return nil, fmt.Errorf("bifrost: route %q script %q must use .js or .ts", route.pattern, route.script)
			}
			normalized, err := normalizeView(sourceRoot, route.script)
			if err != nil {
				return nil, fmt.Errorf("bifrost: route %q script: %w", route.pattern, err)
			}
			route.script = normalized
		}

		if route.partials != "" {
			if !template {
				return nil, fmt.Errorf("bifrost: route %q uses partials with a non-html view", route.pattern)
			}
			normalized, err := normalizeView(sourceRoot, route.partials)
			if err != nil {
				return nil, fmt.Errorf("bifrost: route %q partials: %w", route.pattern, err)
			}
			if !htmlView(normalized) {
				return nil, fmt.Errorf("bifrost: route %q partials %q must use .html", route.pattern, route.partials)
			}
			route.partials = normalized
		}

		if len(route.errorViews) > 0 || route.notFoundView != "" {
			if !template {
				return nil, fmt.Errorf("bifrost: route %q uses error views with a non-html view", route.pattern)
			}
			for errorIndex, errorView := range route.errorViews {
				normalized, err := normalizeView(sourceRoot, errorView)
				if err != nil {
					return nil, fmt.Errorf("bifrost: route %q error view: %w", route.pattern, err)
				}
				if !htmlView(normalized) {
					return nil, fmt.Errorf("bifrost: route %q error view %q must use .html", route.pattern, errorView)
				}
				route.errorViews[errorIndex] = normalized
			}
			if route.notFoundView != "" {
				normalized, err := normalizeView(sourceRoot, route.notFoundView)
				if err != nil {
					return nil, fmt.Errorf("bifrost: route %q not found view: %w", route.pattern, err)
				}
				if !htmlView(normalized) {
					return nil, fmt.Errorf("bifrost: route %q not found view %q must use .html", route.pattern, route.notFoundView)
				}
				route.notFoundView = normalized
			}
		}

		if route.navigation && route.kind != routeServer {
			return nil, fmt.Errorf("bifrost: navigation requires a server route: %q", route.pattern)
		}

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
	case ".tsx", ".jsx", ".ts", ".js", ".html":
	default:
		return "", fmt.Errorf("view path %q must use .tsx, .jsx, .ts, .js, or .html", view)
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

func htmlView(view string) bool {
	return strings.HasSuffix(strings.ToLower(view), ".html")
}

func makeBuildSpec(routes []Route) (protocol.Spec, string, error) {
	spec := protocol.Spec{
		Schema: protocol.Schema,
		Routes: make([]protocol.RouteSpec, len(routes)),
	}
	for i, route := range routes {
		spec.Routes[i] = protocol.RouteSpec{
			Pattern:    route.pattern,
			View:       route.view,
			Kind:       route.kind.String(),
			Navigation: route.navigation,
			Layouts:    slices.Clone(route.layouts),
			Script:     route.script,
			Partials:   route.partials,
			Errors:     slices.Clone(route.errorViews),
			NotFound:   route.notFoundView,
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

package bifrost

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// AppPlugin adds server routes, HTTP middleware, and runtime hooks during app
// construction. Frontend plugins belong in vite.config.ts.
type AppPlugin interface {
	Name() string
	Register(*AppRegistry) error
}

// Middleware is standard Go HTTP middleware.
type Middleware func(http.Handler) http.Handler

// ErrorHandler handles an error before Bifrost writes a response.
type ErrorHandler func(http.ResponseWriter, *http.Request, error)

// AssetHeaderHook may add or replace headers for built and public assets.
type AssetHeaderHook func(http.Header, bool)

// LoadEvent reports one completed loader call.
type LoadEvent struct {
	Pattern  string
	Duration time.Duration
	Err      error
}

// QueueEvent reports one renderer admission and queue wait.
type QueueEvent struct {
	Pattern string
	Wait    time.Duration
	Err     error
}

// RenderEvent reports one completed renderer call.
type RenderEvent struct {
	Pattern  string
	Duration time.Duration
	Err      error
}

// ResponseEvent reports one completed page response.
type ResponseEvent struct {
	Pattern  string
	Status   int
	Bytes    int64
	Duration time.Duration
	Err      error
}

// LoadHook observes completed loader calls. It must not mutate request state.
type LoadHook func(context.Context, LoadEvent)

// QueueHook observes renderer admission and queue outcomes.
type QueueHook func(context.Context, QueueEvent)

// RenderHook observes completed renderer calls. It must not mutate render
// state.
type RenderHook func(context.Context, RenderEvent)

// ResponseHook observes completed page responses.
type ResponseHook func(context.Context, ResponseEvent)

type registeredHooks struct {
	middleware    []Middleware
	errorHandler  ErrorHandler
	assetHeaders  AssetHeaderHook
	loadHooks     []LoadHook
	queueHooks    []QueueHook
	renderHooks   []RenderHook
	responseHooks []ResponseHook
}

type registryState struct {
	routes []Route
	hooks  registeredHooks
}

// AppRegistry contains typed server extension points. An AppRegistry is valid
// only during AppPlugin.Register.
type AppRegistry struct {
	pluginName string
	sealed     bool
	state      *registryState
}

var errRegistrySealed = errors.New("bifrost: plugin registry is sealed")

func (r *AppRegistry) ready() error {
	if r == nil || r.sealed || r.state == nil {
		return errRegistrySealed
	}
	return nil
}

// AddRoutes adds normal Bifrost routes. They receive the same validation as
// routes declared in Config.
func (r *AppRegistry) AddRoutes(routes ...Route) error {
	if err := r.ready(); err != nil {
		return err
	}
	r.state.routes = append(r.state.routes, routes...)
	return nil
}

// Use appends standard HTTP middleware.
func (r *AppRegistry) Use(middleware Middleware) error {
	if err := r.ready(); err != nil {
		return err
	}
	if middleware == nil {
		return errors.New("bifrost: nil middleware")
	}
	r.state.hooks.middleware = append(r.state.hooks.middleware, middleware)
	return nil
}

// HandleErrors installs the app error handler. Only one plugin may install it.
func (r *AppRegistry) HandleErrors(handler ErrorHandler) error {
	if err := r.ready(); err != nil {
		return err
	}
	if handler == nil {
		return errors.New("bifrost: nil error handler")
	}
	if r.state.hooks.errorHandler != nil {
		return errors.New("bifrost: error handler already registered")
	}
	r.state.hooks.errorHandler = handler
	return nil
}

// AssetHeaders installs an asset response header hook. The bool argument is
// true for a public/ file and false for a hashed build artifact.
func (r *AppRegistry) AssetHeaders(hook AssetHeaderHook) error {
	if err := r.ready(); err != nil {
		return err
	}
	if hook == nil {
		return errors.New("bifrost: nil asset header hook")
	}
	if r.state.hooks.assetHeaders != nil {
		return errors.New("bifrost: asset header hook already registered")
	}
	r.state.hooks.assetHeaders = hook
	return nil
}

// OnLoad registers a typed loader observation hook.
func (r *AppRegistry) OnLoad(hook LoadHook) error {
	if err := r.ready(); err != nil {
		return err
	}
	if hook == nil {
		return errors.New("bifrost: nil load hook")
	}
	r.state.hooks.loadHooks = append(r.state.hooks.loadHooks, hook)
	return nil
}

// OnQueue registers a typed renderer queue observation hook.
func (r *AppRegistry) OnQueue(hook QueueHook) error {
	if err := r.ready(); err != nil {
		return err
	}
	if hook == nil {
		return errors.New("bifrost: nil queue hook")
	}
	r.state.hooks.queueHooks = append(r.state.hooks.queueHooks, hook)
	return nil
}

// OnRender registers a typed renderer observation hook.
func (r *AppRegistry) OnRender(hook RenderHook) error {
	if err := r.ready(); err != nil {
		return err
	}
	if hook == nil {
		return errors.New("bifrost: nil render hook")
	}
	r.state.hooks.renderHooks = append(r.state.hooks.renderHooks, hook)
	return nil
}

// OnResponse registers a typed response observation hook.
func (r *AppRegistry) OnResponse(hook ResponseHook) error {
	if err := r.ready(); err != nil {
		return err
	}
	if hook == nil {
		return errors.New("bifrost: nil response hook")
	}
	r.state.hooks.responseHooks = append(r.state.hooks.responseHooks, hook)
	return nil
}

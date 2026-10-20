package bifrost

import (
	"context"
	"errors"
	"net/http"
	"time"
)

type AppPlugin interface {
	Name() string
	Register(*AppRegistry) error
}

type Middleware func(http.Handler) http.Handler

type ErrorHandler func(http.ResponseWriter, *http.Request, error)

type AssetHeaderHook func(http.Header, bool)

type LoadEvent struct {
	Pattern  string
	Duration time.Duration
	Err      error
}

type QueueEvent struct {
	Pattern string
	Wait    time.Duration
	Err     error
}

type RenderEvent struct {
	Pattern  string
	Duration time.Duration
	Err      error
}

type ResponseEvent struct {
	Pattern  string
	Status   int
	Bytes    int64
	Duration time.Duration
	Err      error
}

type LoadHook func(context.Context, LoadEvent)

type QueueHook func(context.Context, QueueEvent)

type RenderHook func(context.Context, RenderEvent)

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

func (r *AppRegistry) AddRoutes(routes ...Route) error {
	if err := r.ready(); err != nil {
		return err
	}
	r.state.routes = append(r.state.routes, routes...)
	return nil
}

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

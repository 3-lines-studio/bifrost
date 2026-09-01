package app

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/3-lines-studio/bifrost"
	"github.com/hibiken/asynq"

	"github.com/3-lines-studio/bifrost/example/modular/internal/config"
)

// Module is the uniform app-surface every domain module implements. Capabilities
// a module lacks are one-line no-ops, so the shape never changes.
type Module interface {
	RegisterHTTP(mux *http.ServeMux)
	Pages() []bifrost.Route
	RegisterTasks(mux *asynq.ServeMux)
	Run(context.Context) error
}

type App struct {
	handler http.Handler
	tasks   *asynq.ServeMux
	server  *asynq.Server
	runners []func(context.Context) error
	logger  *slog.Logger
	addr    string
}

// New composes the bifrost app, HTTP mux, and asynq mux from a set of modules.
// It wires only; it does not start any server. It is safe during the Bifrost
// describe/generate phases because nothing connects a listener here.
func New(cfg *config.Module, fsys fs.FS, logger *slog.Logger, modules ...Module) (*App, error) {
	var pages []bifrost.Route
	for _, module := range modules {
		pages = append(pages, module.Pages()...)
	}

	value := cfg.Value()
	bifrostApp, err := bifrost.New(bifrost.Config{
		SourceRoot: value.SourceRoot,
		Assets:     fsys,
		Routes:     pages,
	})
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	// Register only mounts bifrost page routes and asset handlers after the
	// runtime is compiled. During the Bifrost describe/generate phase there is
	// no runtime, so registration is skipped; main returns immediately after
	// wiring in that mode.
	if !bifrost.Building() {
		if err := bifrostApp.Register(mux); err != nil {
			return nil, err
		}
	}
	for _, module := range modules {
		module.RegisterHTTP(mux)
	}
	mux.Handle("/", http.HandlerFunc(http.NotFound))

	taskMux := asynq.NewServeMux()
	runners := make([]func(context.Context) error, 0, len(modules))
	for _, module := range modules {
		module.RegisterTasks(taskMux)
		runners = append(runners, module.Run)
	}

	redis := value.Redis
	server := asynq.NewServer(asynq.RedisClientOpt{
		Addr:     redis.Addr,
		Password: redis.Password,
		DB:       redis.DB,
	}, asynq.Config{Concurrency: value.Concurrency})

	return &App{
		handler: mux,
		tasks:   taskMux,
		server:  server,
		runners: runners,
		logger:  logger,
		addr:    value.HTTPAddr,
	}, nil
}

func (a *App) Handler() http.Handler { return a.handler }

// Run starts the asynq server and each module's background loop, then serves
// HTTP until ctx is done, at which point it drains both.
func (a *App) Run(ctx context.Context) error {
	if err := a.server.Start(a.tasks); err != nil {
		return err
	}
	for _, run := range a.runners {
		go func(fn func(context.Context) error) { _ = fn(ctx) }(run)
	}

	httpServer := &http.Server{Addr: a.addr, Handler: a.handler}
	go func() {
		<-ctx.Done()
		a.server.Shutdown()
		shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shut)
	}()

	err := httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	a.server.Shutdown()
	return err
}

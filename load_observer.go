package bifrost

import (
	"context"
	"net/http"
	"time"
)

type loadObserverKey struct{}

func ObserveLoad(ctx context.Context, started time.Time, err error) {
	observe, _ := ctx.Value(loadObserverKey{}).(func(time.Time, error))
	if observe != nil {
		observe(started, err)
	}
}

type loadObserverHandler struct {
	pattern string
	next    http.Handler
	hooks   []LoadHook
}

func (h *loadObserverHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	observed := context.WithValue(ctx, loadObserverKey{}, func(started time.Time, err error) {
		event := LoadEvent{Pattern: h.pattern, Duration: time.Since(started), Err: err}
		for _, hook := range h.hooks {
			hook(ctx, event)
		}
	})
	h.next.ServeHTTP(w, request.WithContext(observed))
}

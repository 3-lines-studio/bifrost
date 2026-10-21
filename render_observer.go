package bifrost

import (
	"context"
	"net/http"
	"time"
)

type renderObserverKey struct{}

func ObserveRender(ctx context.Context, started time.Time, err error) {
	observe, _ := ctx.Value(renderObserverKey{}).(func(time.Time, error))
	if observe != nil {
		observe(started, err)
	}
}

type renderObserverHandler struct {
	pattern string
	next    http.Handler
	hooks   []RenderHook
}

func (h *renderObserverHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	observed := context.WithValue(ctx, renderObserverKey{}, func(started time.Time, err error) {
		event := RenderEvent{Pattern: h.pattern, Duration: time.Since(started), Err: err}
		for _, hook := range h.hooks {
			hook(ctx, event)
		}
	})
	h.next.ServeHTTP(w, request.WithContext(observed))
}

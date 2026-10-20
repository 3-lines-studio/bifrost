package bifrost

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRenderObserverHandlerReportsRenderEvent(t *testing.T) {
	want := errors.New("render failed")
	var received RenderEvent
	handler := &renderObserverHandler{
		pattern: "/page",
		hooks: []RenderHook{func(_ context.Context, event RenderEvent) {
			received = event
		}},
		next: http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			ObserveRender(request.Context(), time.Now().Add(-time.Millisecond), want)
		}),
	}
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/page", nil))
	if received.Pattern != "/page" || !errors.Is(received.Err, want) || received.Duration <= 0 {
		t.Fatalf("event = %+v", received)
	}
}

func TestObserveRenderWithoutObserverIsNoop(t *testing.T) {
	ObserveRender(context.Background(), time.Now(), errors.New("ignored"))
}

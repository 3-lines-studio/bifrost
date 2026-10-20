package bifrost

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoadObserverHandlerReportsLoadEvent(t *testing.T) {
	want := errors.New("boom")
	var received LoadEvent
	handler := &loadObserverHandler{
		pattern: "/page",
		hooks: []LoadHook{func(_ context.Context, event LoadEvent) {
			received = event
		}},
		next: http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			ObserveLoad(request.Context(), time.Now().Add(-time.Millisecond), want)
		}),
	}
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/page", nil))
	if received.Pattern != "/page" || !errors.Is(received.Err, want) || received.Duration <= 0 {
		t.Fatalf("event = %+v", received)
	}
}

func TestObserveLoadWithoutObserverIsNoop(t *testing.T) {
	ObserveLoad(context.Background(), time.Now(), errors.New("ignored"))
}

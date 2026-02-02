package bifrost

import (
	"net/http"
	"strings"
	"sync"
)

var (
	reloadEvents     *reload
	reloadEventsOnce sync.Once
)

func GetReloadEvents() *reload {
	reloadEventsOnce.Do(func() {
		if IsDev() {
			reloadEvents = newReload()
		}
	})
	return reloadEvents
}

type reload struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

func newReload() *reload {
	return &reload{
		subs: map[chan struct{}]struct{}{},
	}
}

func (h *reload) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *reload) unsubscribe(ch chan struct{}) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
	close(ch)
}

func (h *reload) notify() {
	h.mu.Lock()
	for ch := range h.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	h.mu.Unlock()
}

func serveReloadSSE(w http.ResponseWriter, req *http.Request) {
	reloader := GetReloadEvents()
	if reloader == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	_, _ = w.Write([]byte("event: ready\ndata: 1\n\n"))
	flusher.Flush()

	ch := reloader.subscribe()
	defer reloader.unsubscribe(ch)

	for {
		select {
		case <-req.Context().Done():
			return
		case <-ch:
			_, _ = w.Write([]byte("event: reload\ndata: 1\n\n"))
			flusher.Flush()
		}
	}
}

func appendReloadScript(html string) string {
	if !IsDev() {
		return html
	}

	if strings.Contains(html, "__bifrost_reload") {
		return html
	}

	script := "<script>" + reloadScriptSource + "</script>"

	if strings.Contains(html, "</body>") {
		return strings.Replace(html, "</body>", script+"</body>", 1)
	}

	return html + script
}

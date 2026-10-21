package bifrost

import "net/http"

type cacheControlHandler struct {
	value string
	next  http.Handler
}

func (h *cacheControlHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if h.value != "" && w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", h.value)
	}
	h.next.ServeHTTP(w, request)
}

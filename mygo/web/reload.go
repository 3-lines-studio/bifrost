package web

import (
	"net/http"
	"strconv"
	"time"
)

// ReloadHandler holds the stream the Reload script of a page follows: it sends
// the id of the process that answers, and keeps the connection open while the
// process lives. A restarted server answers with another id, so the page
// reloads; a page served by one that keeps running reloads never.
func ReloadHandler() http.Handler {
	id := strconv.FormatInt(time.Now().UnixNano(), 36)
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		writer, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming is not available", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		_, _ = w.Write([]byte("data: " + id + "\n\n"))
		writer.Flush()
		ping := time.NewTicker(30 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-request.Context().Done():
				return
			case <-ping.C:
				if _, err := w.Write([]byte(": ping\n\n")); err != nil {
					return
				}
				writer.Flush()
			}
		}
	})
}

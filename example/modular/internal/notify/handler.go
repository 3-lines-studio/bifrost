package notify

import (
	"net/http"

	"github.com/3-lines-studio/bifrost/example/modular/internal/web"
)

// RegisterHTTP mounts the module's REST routes.
func (m *Module) RegisterHTTP(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/notify", m.handleSend)
}

func (m *Module) handleSend(w http.ResponseWriter, r *http.Request) {
	var body sendRequest
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, web.ErrBadRequest)
		return
	}
	if err := m.Send(r.Context(), body.To, body.Subject); err != nil {
		web.WriteError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusAccepted, map[string]bool{"queued": true})
}

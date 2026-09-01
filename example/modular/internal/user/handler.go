package user

import (
	"net/http"

	"github.com/3-lines-studio/bifrost/example/modular/internal/web"
)

// RegisterHTTP mounts the module's REST routes. Handlers contain no business
// logic; they decode, call the service, and delegate responses to web.
func (m *Module) RegisterHTTP(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/users/{id}", m.handleGet)
	mux.HandleFunc("GET /api/users", m.handleList)
}

func (m *Module) handleGet(w http.ResponseWriter, r *http.Request) {
	user, err := m.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		web.WriteError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusOK, user)
}

func (m *Module) handleList(w http.ResponseWriter, r *http.Request) {
	users, err := m.List(r.Context())
	if err != nil {
		web.WriteError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusOK, users)
}

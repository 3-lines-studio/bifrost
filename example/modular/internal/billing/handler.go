package billing

import (
	"net/http"

	"github.com/3-lines-studio/bifrost/example/modular/internal/web"
)

// RegisterHTTP mounts the module's REST routes. Handlers contain no business
// logic; they decode, call the service, and delegate responses to web.
func (m *Module) RegisterHTTP(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/invoices/{id}", m.handleGet)
	mux.HandleFunc("POST /api/invoices", m.handleCreate)
}

func (m *Module) handleGet(w http.ResponseWriter, r *http.Request) {
	invoice, err := m.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		web.WriteError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusOK, invoice)
}

func (m *Module) handleCreate(w http.ResponseWriter, r *http.Request) {
	var body createRequest
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, web.ErrBadRequest)
		return
	}
	invoice, err := m.Issue(r.Context(), body.UserID, body.Amount)
	if err != nil {
		web.WriteError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusCreated, invoice)
}

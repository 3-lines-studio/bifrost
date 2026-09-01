package user

import (
	"net/http"

	"github.com/3-lines-studio/bifrost"
)

// Pages returns the module's server-rendered routes. Loaders contain no
// business logic; they call the service and return PageData.
func (m *Module) Pages() []bifrost.Route {
	return []bifrost.Route{
		bifrost.Server("/users/{id}", "pages/user.tsx", m.pageUser),
	}
}

func (m *Module) pageUser(r *http.Request) (any, error) {
	user, err := m.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	return bifrost.PageData{
		Props:    user,
		Document: bifrost.Document{Lang: m.i18n.Default()},
	}, nil
}

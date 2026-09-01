package user

import (
	"context"
	"errors"

	"github.com/3-lines-studio/bifrost/example/modular/internal/db"
	"github.com/3-lines-studio/bifrost/example/modular/internal/i18n"
	"github.com/3-lines-studio/bifrost/example/modular/internal/web"
)

// Module owns the user domain. It exposes the app-surface methods so the
// composition root can mount its HTTP routes, pages, tasks, and background work.
type Module struct {
	repo repo
	i18n *i18n.Module
}

func New() *Module { return &Module{} }

// Wire injects the module's dependencies. It constructs the private store
// adapter from the db module.
func (m *Module) Wire(database *db.Module, catalog *i18n.Module) {
	m.repo = newPostgresRepo(database.Pool())
	m.i18n = catalog
}

func (m *Module) Get(ctx context.Context, id string) (*User, error) {
	user, err := m.repo.Get(ctx, id)
	if errors.Is(err, web.ErrNotFound) {
		return nil, web.ErrNotFound
	}
	return user, err
}

func (m *Module) List(ctx context.Context) ([]User, error) {
	return m.repo.List(ctx)
}

// Activate marks a user active. It is invoked by the async task handler.
func (m *Module) Activate(ctx context.Context, id string) error {
	_, err := m.repo.Get(ctx, id)
	return err
}

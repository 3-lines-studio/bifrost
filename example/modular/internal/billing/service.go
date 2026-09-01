package billing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/3-lines-studio/bifrost/example/modular/internal/db"
	"github.com/3-lines-studio/bifrost/example/modular/internal/i18n"
	"github.com/3-lines-studio/bifrost/example/modular/internal/queue"
	"github.com/3-lines-studio/bifrost/example/modular/internal/storage"
	"github.com/3-lines-studio/bifrost/example/modular/internal/user"
	"github.com/3-lines-studio/bifrost/example/modular/internal/web"
)

// Module owns the billing domain. It depends on the user module.
type Module struct {
	repo    repo
	queue   *queue.Module
	storage *storage.Module
	i18n    *i18n.Module
	user    *user.Module
}

func New() *Module { return &Module{} }

func (m *Module) Wire(database *db.Module, q *queue.Module, s *storage.Module, catalog *i18n.Module, usr *user.Module) {
	m.repo = newPostgresRepo(database.Pool())
	m.queue = q
	m.storage = s
	m.i18n = catalog
	m.user = usr
}

func (m *Module) Get(ctx context.Context, id string) (*Invoice, error) {
	invoice, err := m.repo.Get(ctx, id)
	if errors.Is(err, web.ErrNotFound) {
		return nil, web.ErrNotFound
	}
	return invoice, err
}

// Issue creates an invoice for a user, then enqueues a task to collect payment.
func (m *Module) Issue(ctx context.Context, userID string, amount int64) (*Invoice, error) {
	if _, err := m.user.Get(ctx, userID); err != nil {
		return nil, err
	}
	invoice := &Invoice{ID: newID(), UserID: userID, Amount: amount, Status: "open", CreatedAt: time.Now()}
	if err := m.repo.Create(ctx, invoice); err != nil {
		return nil, err
	}
	if err := m.queue.Enqueue(ctx, "billing:charge", []byte(`{"invoiceId":"`+invoice.ID+`"}`)); err != nil {
		return nil, err
	}
	return invoice, nil
}

// Charge marks an invoice paid. It is invoked by the async task handler.
func (m *Module) Charge(ctx context.Context, id string) error {
	invoice, err := m.Get(ctx, id)
	if err != nil {
		return err
	}
	return m.repo.UpdateStatus(ctx, invoice.ID, "paid")
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

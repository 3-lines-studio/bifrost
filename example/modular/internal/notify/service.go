package notify

import (
	"context"

	"github.com/3-lines-studio/bifrost/example/modular/internal/i18n"
	"github.com/3-lines-studio/bifrost/example/modular/internal/mailer"
	"github.com/3-lines-studio/bifrost/example/modular/internal/queue"
)

// Module owns email notification. It has no persistence, so it has no repo or
// store; it depends on the queue, mailer, and i18n modules.
type Module struct {
	queue  *queue.Module
	mailer *mailer.Module
	i18n   *i18n.Module
}

func New() *Module { return &Module{} }

func (m *Module) Wire(q *queue.Module, sender *mailer.Module, catalog *i18n.Module) {
	m.queue = q
	m.mailer = sender
	m.i18n = catalog
}

// Send enqueues an email task.
func (m *Module) Send(ctx context.Context, to, subject string) error {
	return m.queue.Enqueue(ctx, "notify:email", []byte(`{"to":"`+to+`","subject":"`+subject+`"}`))
}

// Deliver sends one email. It is invoked by the async task handler.
func (m *Module) Deliver(ctx context.Context, to, subject string) error {
	body := m.i18n.Value(m.i18n.Default(), subject)
	return m.mailer.Send(ctx, to, subject, body)
}

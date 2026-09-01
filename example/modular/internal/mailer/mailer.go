package mailer

import (
	"context"

	"github.com/3-lines-studio/bifrost/example/modular/internal/config"
)

// Module is a leaf module. It owns the mail transport. The scaffold keeps a
// minimal interface so it compiles without an SMTP client; swap Module for a
// real adapter in production.
type Module struct {
	host string
	port int
}

func New() *Module { return &Module{} }

func (m *Module) Wire(cfg *config.Module) {
	smtp := cfg.Value().SMTP
	m.host = smtp.Host
	m.port = smtp.Port
}

func (m *Module) Send(ctx context.Context, to, subject, body string) error {
	// Stub. Replace with the SMTP or email-service Send call in production.
	return nil
}

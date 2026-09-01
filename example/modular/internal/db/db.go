package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/3-lines-studio/bifrost/example/modular/internal/config"
)

// Module is a leaf module. It owns the connection pool. Other modules depend
// on it and read Pool or run a transaction with Tx.
type Module struct {
	pool *pgxpool.Pool
}

func New() *Module { return &Module{} }

// Wire builds the pool from config. It parses the URL but never dials, so it is
// safe during the Bifrost describe/generate phases.
func (m *Module) Wire(ctx context.Context, cfg *config.Module) {
	pool, err := pgxpool.New(ctx, cfg.Value().Postgres.URL)
	if err != nil {
		panic(err)
	}
	m.pool = pool
}

func (m *Module) Pool() *pgxpool.Pool { return m.pool }

// Tx runs fn inside a transaction, committing on success and rolling back on
// error.
func (m *Module) Tx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

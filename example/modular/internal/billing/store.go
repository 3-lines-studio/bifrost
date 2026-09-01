package billing

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/3-lines-studio/bifrost/example/modular/internal/web"
)

type postgresRepo struct {
	pool *pgxpool.Pool
}

func newPostgresRepo(pool *pgxpool.Pool) *postgresRepo {
	return &postgresRepo{pool: pool}
}

func (r *postgresRepo) Create(ctx context.Context, invoice *Invoice) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO invoices (id, user_id, amount, status, created_at) VALUES ($1, $2, $3, $4, $5)`,
		invoice.ID, invoice.UserID, invoice.Amount, invoice.Status, invoice.CreatedAt)
	return err
}

func (r *postgresRepo) Get(ctx context.Context, id string) (*Invoice, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, user_id, amount, status, created_at FROM invoices WHERE id = $1`, id)
	var invoice Invoice
	if err := row.Scan(&invoice.ID, &invoice.UserID, &invoice.Amount, &invoice.Status, &invoice.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.ErrNotFound
		}
		return nil, err
	}
	return &invoice, nil
}

func (r *postgresRepo) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := r.pool.Exec(ctx, `UPDATE invoices SET status = $2 WHERE id = $1`, id, status)
	return err
}

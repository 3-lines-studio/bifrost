package user

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

func (r *postgresRepo) Get(ctx context.Context, id string) (*User, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, email, plan, created_at FROM users WHERE id = $1`, id)
	var user User
	if err := row.Scan(&user.ID, &user.Email, &user.Plan, &user.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *postgresRepo) List(ctx context.Context) ([]User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, email, plan, created_at FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.Email, &user.Plan, &user.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

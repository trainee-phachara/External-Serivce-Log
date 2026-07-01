package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a connection pool for connString.
func NewPool(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	return pgxpool.New(ctx, connString)
}

// EnsurePaymentsTable creates the payments table if it does not already exist.
func EnsurePaymentsTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS payments (
			id         SERIAL PRIMARY KEY,
			order_id   INTEGER NOT NULL,
			user_id    INTEGER NOT NULL,
			amount     NUMERIC(10,2) NOT NULL,
			status     VARCHAR(20) NOT NULL DEFAULT 'pending',
			method     VARCHAR(20) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	return err
}

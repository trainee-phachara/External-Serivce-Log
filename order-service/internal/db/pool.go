package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a connection pool for connString.
func NewPool(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	return pgxpool.New(ctx, connString)
}

// EnsureOrdersTable creates the orders table if it does not already exist.
// Items are stored as JSONB to avoid a separate order_items table.
func EnsureOrdersTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS orders (
			id           SERIAL PRIMARY KEY,
			user_id      INTEGER NOT NULL,
			items        JSONB NOT NULL DEFAULT '[]',
			status       VARCHAR(20) NOT NULL DEFAULT 'pending',
			total_amount NUMERIC(10,2) NOT NULL DEFAULT 0,
			created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	return err
}

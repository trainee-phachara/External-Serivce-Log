package payments

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"payment-service/internal/types"
)

const paymentColumns = "id, order_id, user_id, amount, status, method, created_at, updated_at"

// Repository persists payments in PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository returns a Repository backed by pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func scanPayment(row pgx.Row, p *types.Payment) error {
	return row.Scan(&p.ID, &p.OrderID, &p.UserID, &p.Amount, &p.Status, &p.Method, &p.CreatedAt, &p.UpdatedAt)
}

// Create inserts a new payment and returns the inserted row.
func (r *Repository) Create(ctx context.Context, input types.CreatePaymentInput) (types.Payment, error) {
	var p types.Payment
	row := r.pool.QueryRow(ctx,
		"INSERT INTO payments (order_id, user_id, amount, method) VALUES ($1, $2, $3, $4) RETURNING "+paymentColumns,
		input.OrderID, input.UserID, input.Amount, input.Method,
	)
	err := scanPayment(row, &p)
	return p, err
}

// FindAll returns all payments ordered by id.
func (r *Repository) FindAll(ctx context.Context) ([]types.Payment, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+paymentColumns+" FROM payments ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]types.Payment, 0)
	for rows.Next() {
		var p types.Payment
		if err := scanPayment(rows, &p); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// FindByID returns the payment with the given id, or nil if it does not exist.
func (r *Repository) FindByID(ctx context.Context, id int) (*types.Payment, error) {
	var p types.Payment
	row := r.pool.QueryRow(ctx, "SELECT "+paymentColumns+" FROM payments WHERE id = $1", id)
	if err := scanPayment(row, &p); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// FindByOrderID returns all payments for the given order_id, ordered by id.
func (r *Repository) FindByOrderID(ctx context.Context, orderID int) ([]types.Payment, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT "+paymentColumns+" FROM payments WHERE order_id = $1 ORDER BY id ASC",
		orderID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]types.Payment, 0)
	for rows.Next() {
		var p types.Payment
		if err := scanPayment(rows, &p); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// Update changes the status of the payment with the given id.
// Returns nil if the payment does not exist.
func (r *Repository) Update(ctx context.Context, id int, input types.UpdatePaymentInput) (*types.Payment, error) {
	existing, err := r.FindByID(ctx, id)
	if err != nil || existing == nil {
		return nil, err
	}

	status := existing.Status
	if input.Status != nil {
		status = *input.Status
	}

	var p types.Payment
	row := r.pool.QueryRow(ctx,
		"UPDATE payments SET status = $1, updated_at = now() WHERE id = $2 RETURNING "+paymentColumns,
		status, id,
	)
	if err := scanPayment(row, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Remove deletes the payment with the given id, reporting whether a row was removed.
func (r *Repository) Remove(ctx context.Context, id int) (bool, error) {
	tag, err := r.pool.Exec(ctx, "DELETE FROM payments WHERE id = $1", id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

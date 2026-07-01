package orders

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"order-service/internal/types"
)

const orderColumns = "id, user_id, items, status, total_amount, created_at, updated_at"

// Repository persists orders in PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository returns a Repository backed by pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func scanOrder(row pgx.Row, o *types.Order) error {
	var itemsJSON []byte
	if err := row.Scan(&o.ID, &o.UserID, &itemsJSON, &o.Status, &o.TotalAmount, &o.CreatedAt, &o.UpdatedAt); err != nil {
		return err
	}
	return json.Unmarshal(itemsJSON, &o.Items)
}

func computeTotal(items []types.OrderItem) float64 {
	total := 0.0
	for _, item := range items {
		total += float64(item.Quantity) * item.UnitPrice
	}
	return total
}

// Create inserts a new order and returns the inserted row.
func (r *Repository) Create(ctx context.Context, input types.CreateOrderInput) (types.Order, error) {
	itemsJSON, err := json.Marshal(input.Items)
	if err != nil {
		return types.Order{}, err
	}

	total := computeTotal(input.Items)

	var o types.Order
	row := r.pool.QueryRow(ctx,
		"INSERT INTO orders (user_id, items, total_amount) VALUES ($1, $2, $3) RETURNING "+orderColumns,
		input.UserID, itemsJSON, total,
	)
	err = scanOrder(row, &o)
	return o, err
}

// FindAll returns all orders ordered by id.
func (r *Repository) FindAll(ctx context.Context) ([]types.Order, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+orderColumns+" FROM orders ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]types.Order, 0)
	for rows.Next() {
		var o types.Order
		if err := scanOrder(rows, &o); err != nil {
			return nil, err
		}
		result = append(result, o)
	}
	return result, rows.Err()
}

// FindByID returns the order with the given id, or nil if it does not exist.
func (r *Repository) FindByID(ctx context.Context, id int) (*types.Order, error) {
	var o types.Order
	row := r.pool.QueryRow(ctx, "SELECT "+orderColumns+" FROM orders WHERE id = $1", id)
	if err := scanOrder(row, &o); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &o, nil
}

// Update changes the status of the order with the given id.
// Returns nil if the order does not exist.
func (r *Repository) Update(ctx context.Context, id int, input types.UpdateOrderInput) (*types.Order, error) {
	existing, err := r.FindByID(ctx, id)
	if err != nil || existing == nil {
		return nil, err
	}

	status := existing.Status
	if input.Status != nil {
		status = *input.Status
	}

	var o types.Order
	row := r.pool.QueryRow(ctx,
		"UPDATE orders SET status = $1, updated_at = now() WHERE id = $2 RETURNING "+orderColumns,
		status, id,
	)
	if err := scanOrder(row, &o); err != nil {
		return nil, err
	}
	return &o, nil
}

// Remove deletes the order with the given id, reporting whether a row was removed.
func (r *Repository) Remove(ctx context.Context, id int) (bool, error) {
	tag, err := r.pool.Exec(ctx, "DELETE FROM orders WHERE id = $1", id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

package users

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"user-service/internal/types"
)

const userColumns = "id, name, email, created_at, updated_at"

// Repository persists users in PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository returns a Repository backed by pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func scanUser(row pgx.Row, u *types.User) error {
	return row.Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt, &u.UpdatedAt)
}

// Create inserts a new user and returns the inserted row.
func (r *Repository) Create(ctx context.Context, input types.CreateUserInput) (types.User, error) {
	var u types.User
	row := r.pool.QueryRow(ctx,
		"INSERT INTO users (name, email) VALUES ($1, $2) RETURNING "+userColumns,
		input.Name, input.Email,
	)
	err := scanUser(row, &u)
	return u, err
}

// FindAll returns all users ordered by id.
func (r *Repository) FindAll(ctx context.Context) ([]types.User, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+userColumns+" FROM users ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]types.User, 0)
	for rows.Next() {
		var u types.User
		if err := scanUser(rows, &u); err != nil {
			return nil, err
		}
		result = append(result, u)
	}
	return result, rows.Err()
}

// FindByID returns the user with the given id, or nil if it does not exist.
func (r *Repository) FindByID(ctx context.Context, id int) (*types.User, error) {
	var u types.User
	row := r.pool.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE id = $1", id)
	if err := scanUser(row, &u); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// Update applies input to the user with the given id, leaving any unset
// fields unchanged. It returns nil if the user does not exist.
func (r *Repository) Update(ctx context.Context, id int, input types.UpdateUserInput) (*types.User, error) {
	existing, err := r.FindByID(ctx, id)
	if err != nil || existing == nil {
		return nil, err
	}

	name := existing.Name
	if input.Name != nil {
		name = *input.Name
	}
	email := existing.Email
	if input.Email != nil {
		email = *input.Email
	}

	var u types.User
	row := r.pool.QueryRow(ctx,
		"UPDATE users SET name = $1, email = $2, updated_at = now() WHERE id = $3 RETURNING "+userColumns,
		name, email, id,
	)
	if err := scanUser(row, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// Remove deletes the user with the given id, reporting whether a row was removed.
func (r *Repository) Remove(ctx context.Context, id int) (bool, error) {
	tag, err := r.pool.Exec(ctx, "DELETE FROM users WHERE id = $1", id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

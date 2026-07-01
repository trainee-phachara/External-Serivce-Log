package users

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"user-service/internal/db"
	"user-service/internal/types"
)

func newTestRepository(t *testing.T) *Repository {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("user_service_test"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Errorf("terminate postgres container: %v", err)
		}
	})

	connString, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := db.EnsureUsersTable(ctx, pool); err != nil {
		t.Fatalf("ensure users table: %v", err)
	}

	return NewRepository(pool)
}

func TestRepository_CreateReturnsInsertedRow(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	user, err := repo.Create(ctx, types.CreateUserInput{Name: "Alice", Email: "alice@example.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if user.ID == 0 {
		t.Error("ID = 0, want non-zero")
	}
	if user.Name != "Alice" {
		t.Errorf("Name = %q, want %q", user.Name, "Alice")
	}
	if user.Email != "alice@example.com" {
		t.Errorf("Email = %q, want %q", user.Email, "alice@example.com")
	}
	if user.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if user.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero")
	}
}

func TestRepository_FindAllOrdersByID(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	if _, err := repo.Create(ctx, types.CreateUserInput{Name: "Alice", Email: "alice@example.com"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := repo.Create(ctx, types.CreateUserInput{Name: "Bob", Email: "bob@example.com"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	all, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}

	if len(all) != 2 {
		t.Fatalf("len(all) = %d, want 2", len(all))
	}
	if all[0].Name != "Alice" {
		t.Errorf("all[0].Name = %q, want %q", all[0].Name, "Alice")
	}
	if all[1].Name != "Bob" {
		t.Errorf("all[1].Name = %q, want %q", all[1].Name, "Bob")
	}
}

func TestRepository_FindByID(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, types.CreateUserInput{Name: "Alice", Email: "alice@example.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	found, err := repo.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found == nil {
		t.Fatal("found = nil, want a user")
	}
	if found.ID != created.ID || found.Name != created.Name || found.Email != created.Email {
		t.Errorf("found = %+v, want %+v", *found, created)
	}
	if !found.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", found.CreatedAt, created.CreatedAt)
	}
	if !found.UpdatedAt.Equal(created.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want %v", found.UpdatedAt, created.UpdatedAt)
	}
}

func TestRepository_FindByIDReturnsNilForMissingUser(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	found, err := repo.FindByID(ctx, 999)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found != nil {
		t.Errorf("found = %+v, want nil", found)
	}
}

func strPtr(s string) *string { return &s }

func TestRepository_UpdateNameOnly(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, types.CreateUserInput{Name: "Alice", Email: "alice@example.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := repo.Update(ctx, created.ID, types.UpdateUserInput{Name: strPtr("Alice Updated")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil {
		t.Fatal("updated = nil, want a user")
	}
	if updated.Name != "Alice Updated" {
		t.Errorf("Name = %q, want %q", updated.Name, "Alice Updated")
	}
	if updated.Email != "alice@example.com" {
		t.Errorf("Email = %q, want %q", updated.Email, "alice@example.com")
	}
}

func TestRepository_UpdateEmailOnly(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, types.CreateUserInput{Name: "Alice", Email: "alice@example.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := repo.Update(ctx, created.ID, types.UpdateUserInput{Email: strPtr("alice2@example.com")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil {
		t.Fatal("updated = nil, want a user")
	}
	if updated.Name != "Alice" {
		t.Errorf("Name = %q, want %q", updated.Name, "Alice")
	}
	if updated.Email != "alice2@example.com" {
		t.Errorf("Email = %q, want %q", updated.Email, "alice2@example.com")
	}
}

func TestRepository_UpdateBothFields(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, types.CreateUserInput{Name: "Alice", Email: "alice@example.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := repo.Update(ctx, created.ID, types.UpdateUserInput{
		Name:  strPtr("Alice Updated"),
		Email: strPtr("alice2@example.com"),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil {
		t.Fatal("updated = nil, want a user")
	}
	if updated.Name != "Alice Updated" {
		t.Errorf("Name = %q, want %q", updated.Name, "Alice Updated")
	}
	if updated.Email != "alice2@example.com" {
		t.Errorf("Email = %q, want %q", updated.Email, "alice2@example.com")
	}
}

func TestRepository_UpdateReturnsNilForMissingUser(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	updated, err := repo.Update(ctx, 999, types.UpdateUserInput{Name: strPtr("Nope")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated != nil {
		t.Errorf("updated = %+v, want nil", updated)
	}
}

func TestRepository_RemoveDeletesUser(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, types.CreateUserInput{Name: "Alice", Email: "alice@example.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	removed, err := repo.Remove(ctx, created.ID)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !removed {
		t.Error("removed = false, want true")
	}

	found, err := repo.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found != nil {
		t.Errorf("found = %+v, want nil", found)
	}
}

func TestRepository_RemoveReturnsFalseForMissingUser(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	removed, err := repo.Remove(ctx, 999)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if removed {
		t.Error("removed = true, want false")
	}
}

package orders

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"order-service/internal/db"
	"order-service/internal/types"
)

func newTestRepository(t *testing.T) *Repository {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("order_service_test"),
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

	if err := db.EnsureOrdersTable(ctx, pool); err != nil {
		t.Fatalf("ensure orders table: %v", err)
	}

	return NewRepository(pool)
}

func sampleInput() types.CreateOrderInput {
	return types.CreateOrderInput{
		UserID: 1,
		Items: []types.OrderItem{
			{ProductID: 10, ProductName: "Widget", Quantity: 2, UnitPrice: 5.00},
		},
	}
}

func TestRepository_CreateReturnsInsertedRow(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	order, err := repo.Create(ctx, sampleInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if order.ID == 0 {
		t.Error("ID = 0, want non-zero")
	}
	if order.UserID != 1 {
		t.Errorf("UserID = %d, want 1", order.UserID)
	}
	if order.Status != types.OrderStatusPending {
		t.Errorf("Status = %q, want pending", order.Status)
	}
	if order.TotalAmount != 10.00 {
		t.Errorf("TotalAmount = %f, want 10.00", order.TotalAmount)
	}
	if len(order.Items) != 1 || order.Items[0].ProductName != "Widget" {
		t.Errorf("Items = %+v, want [{Widget ...}]", order.Items)
	}
	if order.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
}

func TestRepository_FindAllOrdersByID(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	input1 := types.CreateOrderInput{UserID: 1, Items: []types.OrderItem{{ProductID: 1, ProductName: "A", Quantity: 1, UnitPrice: 1}}}
	input2 := types.CreateOrderInput{UserID: 2, Items: []types.OrderItem{{ProductID: 2, ProductName: "B", Quantity: 1, UnitPrice: 2}}}

	if _, err := repo.Create(ctx, input1); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := repo.Create(ctx, input2); err != nil {
		t.Fatalf("Create: %v", err)
	}

	all, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("len(all) = %d, want 2", len(all))
	}
	if all[0].UserID != 1 {
		t.Errorf("all[0].UserID = %d, want 1", all[0].UserID)
	}
	if all[1].UserID != 2 {
		t.Errorf("all[1].UserID = %d, want 2", all[1].UserID)
	}
}

func TestRepository_FindByID(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, sampleInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	found, err := repo.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found == nil {
		t.Fatal("found = nil, want an order")
	}
	if found.ID != created.ID || found.UserID != created.UserID {
		t.Errorf("found = %+v, want %+v", *found, created)
	}
}

func TestRepository_FindByIDReturnsNilForMissingOrder(t *testing.T) {
	repo := newTestRepository(t)

	found, err := repo.FindByID(context.Background(), 999)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found != nil {
		t.Errorf("found = %+v, want nil", found)
	}
}

func statusPtr(s types.OrderStatus) *types.OrderStatus { return &s }

func TestRepository_UpdateStatus(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, sampleInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := repo.Update(ctx, created.ID, types.UpdateOrderInput{Status: statusPtr(types.OrderStatusShipped)})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil {
		t.Fatal("updated = nil, want an order")
	}
	if updated.Status != types.OrderStatusShipped {
		t.Errorf("Status = %q, want shipped", updated.Status)
	}
	// items and total unchanged
	if updated.TotalAmount != created.TotalAmount {
		t.Errorf("TotalAmount changed: got %f, want %f", updated.TotalAmount, created.TotalAmount)
	}
}

func TestRepository_UpdateReturnsNilForMissingOrder(t *testing.T) {
	repo := newTestRepository(t)

	updated, err := repo.Update(context.Background(), 999, types.UpdateOrderInput{Status: statusPtr(types.OrderStatusCancelled)})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated != nil {
		t.Errorf("updated = %+v, want nil", updated)
	}
}

func TestRepository_RemoveDeletesOrder(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, sampleInput())
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

func TestRepository_RemoveReturnsFalseForMissingOrder(t *testing.T) {
	repo := newTestRepository(t)

	removed, err := repo.Remove(context.Background(), 999)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if removed {
		t.Error("removed = true, want false")
	}
}

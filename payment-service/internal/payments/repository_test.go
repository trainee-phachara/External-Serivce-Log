package payments

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"payment-service/internal/db"
	"payment-service/internal/types"
)

func newTestRepository(t *testing.T) *Repository {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("payment_service_test"),
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

	if err := db.EnsurePaymentsTable(ctx, pool); err != nil {
		t.Fatalf("ensure payments table: %v", err)
	}

	return NewRepository(pool)
}

func sampleInput() types.CreatePaymentInput {
	return types.CreatePaymentInput{
		OrderID: 1,
		UserID:  2,
		Amount:  250.00,
		Method:  types.PaymentMethodPromptPay,
	}
}

func statusPtr(s types.PaymentStatus) *types.PaymentStatus { return &s }

func TestRepository_CreateReturnsInsertedRow(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	payment, err := repo.Create(ctx, sampleInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if payment.ID == 0 {
		t.Error("ID = 0, want non-zero")
	}
	if payment.OrderID != 1 {
		t.Errorf("OrderID = %d, want 1", payment.OrderID)
	}
	if payment.UserID != 2 {
		t.Errorf("UserID = %d, want 2", payment.UserID)
	}
	if payment.Amount != 250.00 {
		t.Errorf("Amount = %f, want 250.00", payment.Amount)
	}
	if payment.Status != types.PaymentStatusPending {
		t.Errorf("Status = %q, want pending", payment.Status)
	}
	if payment.Method != types.PaymentMethodPromptPay {
		t.Errorf("Method = %q, want promptpay", payment.Method)
	}
	if payment.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
}

func TestRepository_FindAllOrdersByID(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	input1 := types.CreatePaymentInput{OrderID: 1, UserID: 1, Amount: 100, Method: types.PaymentMethodCreditCard}
	input2 := types.CreatePaymentInput{OrderID: 2, UserID: 1, Amount: 200, Method: types.PaymentMethodBankTransfer}

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
	if all[0].OrderID != 1 {
		t.Errorf("all[0].OrderID = %d, want 1", all[0].OrderID)
	}
	if all[1].OrderID != 2 {
		t.Errorf("all[1].OrderID = %d, want 2", all[1].OrderID)
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
		t.Fatal("found = nil, want a payment")
	}
	if found.ID != created.ID || found.Amount != created.Amount {
		t.Errorf("found = %+v, want %+v", *found, created)
	}
}

func TestRepository_FindByIDReturnsNilForMissingPayment(t *testing.T) {
	repo := newTestRepository(t)

	found, err := repo.FindByID(context.Background(), 999)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found != nil {
		t.Errorf("found = %+v, want nil", found)
	}
}

func TestRepository_FindByOrderID(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	// สร้าง 2 payments สำหรับ order 1 และ 1 payment สำหรับ order 2
	input := types.CreatePaymentInput{OrderID: 1, UserID: 1, Amount: 100, Method: types.PaymentMethodPromptPay}
	if _, err := repo.Create(ctx, input); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := repo.Create(ctx, input); err != nil {
		t.Fatalf("Create: %v", err)
	}
	input2 := types.CreatePaymentInput{OrderID: 2, UserID: 1, Amount: 200, Method: types.PaymentMethodPromptPay}
	if _, err := repo.Create(ctx, input2); err != nil {
		t.Fatalf("Create: %v", err)
	}

	payments, err := repo.FindByOrderID(ctx, 1)
	if err != nil {
		t.Fatalf("FindByOrderID: %v", err)
	}
	if len(payments) != 2 {
		t.Errorf("len(payments) = %d, want 2", len(payments))
	}
	for _, p := range payments {
		if p.OrderID != 1 {
			t.Errorf("OrderID = %d, want 1", p.OrderID)
		}
	}
}

func TestRepository_FindByOrderIDReturnsEmptyForUnknownOrder(t *testing.T) {
	repo := newTestRepository(t)

	payments, err := repo.FindByOrderID(context.Background(), 999)
	if err != nil {
		t.Fatalf("FindByOrderID: %v", err)
	}
	if len(payments) != 0 {
		t.Errorf("len(payments) = %d, want 0", len(payments))
	}
}

func TestRepository_UpdateStatus(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, sampleInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := repo.Update(ctx, created.ID, types.UpdatePaymentInput{Status: statusPtr(types.PaymentStatusCompleted)})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil {
		t.Fatal("updated = nil, want a payment")
	}
	if updated.Status != types.PaymentStatusCompleted {
		t.Errorf("Status = %q, want completed", updated.Status)
	}
	// amount and method unchanged
	if updated.Amount != created.Amount {
		t.Errorf("Amount changed: got %f, want %f", updated.Amount, created.Amount)
	}
	if updated.Method != created.Method {
		t.Errorf("Method changed: got %q, want %q", updated.Method, created.Method)
	}
}

func TestRepository_UpdateReturnsNilForMissingPayment(t *testing.T) {
	repo := newTestRepository(t)

	updated, err := repo.Update(context.Background(), 999, types.UpdatePaymentInput{Status: statusPtr(types.PaymentStatusFailed)})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated != nil {
		t.Errorf("updated = %+v, want nil", updated)
	}
}

func TestRepository_RemoveDeletesPayment(t *testing.T) {
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

func TestRepository_RemoveReturnsFalseForMissingPayment(t *testing.T) {
	repo := newTestRepository(t)

	removed, err := repo.Remove(context.Background(), 999)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if removed {
		t.Error("removed = true, want false")
	}
}

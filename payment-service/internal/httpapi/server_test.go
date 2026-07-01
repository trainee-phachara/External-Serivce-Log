package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"payment-service/internal/db"
	"payment-service/internal/payments"
	"payment-service/internal/types"
)

func newTestHandler(t *testing.T) (http.Handler, *fakeLogClient) {
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

	repo := payments.NewRepository(pool)
	fake := &fakeLogClient{}
	return NewHandler(repo, fake), fake
}

func doRequest(t *testing.T, handler http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("decode body: %v, body=%s", err, rec.Body.String())
	}
}

func validPaymentBody() map[string]interface{} {
	return map[string]interface{}{
		"order_id": 1,
		"user_id":  2,
		"amount":   250.00,
		"method":   "promptpay",
	}
}

func createTestPayment(t *testing.T, handler http.Handler) types.Payment {
	t.Helper()
	rec := doRequest(t, handler, http.MethodPost, "/payments", validPaymentBody())
	if rec.Code != http.StatusCreated {
		t.Fatalf("create payment status = %d, want %d", rec.Code, http.StatusCreated)
	}
	var p types.Payment
	decodeBody(t, rec, &p)
	return p
}

func TestCreatePayment_ReturnsCreatedPayment(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodPost, "/payments", validPaymentBody())

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var p types.Payment
	decodeBody(t, rec, &p)
	if p.ID == 0 {
		t.Error("ID = 0, want non-zero")
	}
	if p.OrderID != 1 {
		t.Errorf("OrderID = %d, want 1", p.OrderID)
	}
	if p.Status != types.PaymentStatusPending {
		t.Errorf("Status = %q, want pending", p.Status)
	}
	if p.Amount != 250.00 {
		t.Errorf("Amount = %f, want 250.00", p.Amount)
	}
	if p.Method != types.PaymentMethodPromptPay {
		t.Errorf("Method = %q, want promptpay", p.Method)
	}
}

func TestCreatePayment_RejectsInvalidBody(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodPost, "/payments", map[string]interface{}{})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var body struct{ Errors []string `json:"errors"` }
	decodeBody(t, rec, &body)
	if len(body.Errors) == 0 {
		t.Error("Errors is empty, want at least one validation error")
	}
}

func TestCreatePayment_FiresLogEntryOnFinish(t *testing.T) {
	handler, fake := newTestHandler(t)

	doRequest(t, handler, http.MethodPost, "/payments", validPaymentBody())

	entries := fake.Entries()
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].Endpoint != "/payments" {
		t.Errorf("Endpoint = %q, want /payments", entries[0].Endpoint)
	}
	if entries[0].HTTPStatus != "201" {
		t.Errorf("HTTPStatus = %q, want 201", entries[0].HTTPStatus)
	}
}

func TestListPayments_ReturnsEmptyArrayWhenNone(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, "/payments", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []types.Payment
	decodeBody(t, rec, &got)
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

func TestListPayments_ReturnsAllPayments(t *testing.T) {
	handler, _ := newTestHandler(t)

	createTestPayment(t, handler)
	createTestPayment(t, handler)

	rec := doRequest(t, handler, http.MethodGet, "/payments", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []types.Payment
	decodeBody(t, rec, &got)
	if len(got) != 2 {
		t.Errorf("len(got) = %d, want 2", len(got))
	}
}

func TestListPaymentsByOrder_ReturnsOnlyMatchingPayments(t *testing.T) {
	handler, _ := newTestHandler(t)

	// สร้าง payment สำหรับ order 1 สองใบ
	doRequest(t, handler, http.MethodPost, "/payments", validPaymentBody())
	doRequest(t, handler, http.MethodPost, "/payments", validPaymentBody())
	// order 2 หนึ่งใบ
	body2 := validPaymentBody()
	body2["order_id"] = 2
	doRequest(t, handler, http.MethodPost, "/payments", body2)

	rec := doRequest(t, handler, http.MethodGet, "/payments/order/1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []types.Payment
	decodeBody(t, rec, &got)
	if len(got) != 2 {
		t.Errorf("len(got) = %d, want 2", len(got))
	}
}

func TestListPaymentsByOrder_RejectsNonIntegerOrderID(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, "/payments/order/abc", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetPayment_RejectsNonIntegerID(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, "/payments/abc", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetPayment_ReturnsNotFoundForMissingPayment(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, "/payments/999", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetPayment_ReturnsPaymentWhenFound(t *testing.T) {
	handler, _ := newTestHandler(t)

	created := createTestPayment(t, handler)

	rec := doRequest(t, handler, http.MethodGet, fmt.Sprintf("/payments/%d", created.ID), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got types.Payment
	decodeBody(t, rec, &got)
	if got.ID != created.ID || got.Amount != created.Amount {
		t.Errorf("got = %+v, want %+v", got, created)
	}
}

func TestUpdatePayment_RejectsNonIntegerID(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodPut, "/payments/abc", map[string]interface{}{"status": "completed"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdatePayment_RejectsInvalidStatus(t *testing.T) {
	handler, _ := newTestHandler(t)

	created := createTestPayment(t, handler)
	rec := doRequest(t, handler, http.MethodPut, fmt.Sprintf("/payments/%d", created.ID), map[string]interface{}{"status": "processing"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdatePayment_ReturnsNotFoundForMissingPayment(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodPut, "/payments/999", map[string]interface{}{"status": "completed"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdatePayment_UpdatesStatusAndReturnsPayment(t *testing.T) {
	handler, _ := newTestHandler(t)

	created := createTestPayment(t, handler)
	rec := doRequest(t, handler, http.MethodPut, fmt.Sprintf("/payments/%d", created.ID), map[string]interface{}{"status": "completed"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got types.Payment
	decodeBody(t, rec, &got)
	if got.Status != types.PaymentStatusCompleted {
		t.Errorf("Status = %q, want completed", got.Status)
	}
}

func TestDeletePayment_RejectsNonIntegerID(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodDelete, "/payments/abc", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeletePayment_ReturnsNotFoundForMissingPayment(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodDelete, "/payments/999", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeletePayment_DeletesPaymentAndReturnsNoContent(t *testing.T) {
	handler, _ := newTestHandler(t)

	created := createTestPayment(t, handler)

	rec := doRequest(t, handler, http.MethodDelete, fmt.Sprintf("/payments/%d", created.ID), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	getRec := doRequest(t, handler, http.MethodGet, fmt.Sprintf("/payments/%d", created.ID), nil)
	if getRec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

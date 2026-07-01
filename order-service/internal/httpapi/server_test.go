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

	"order-service/internal/db"
	"order-service/internal/orders"
	"order-service/internal/types"
)

func newTestHandler(t *testing.T) (http.Handler, *fakeLogClient) {
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

	repo := orders.NewRepository(pool)
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

func validOrderBody() map[string]interface{} {
	return map[string]interface{}{
		"user_id": 1,
		"items": []interface{}{
			map[string]interface{}{
				"product_id":   10,
				"product_name": "Widget",
				"quantity":     2,
				"unit_price":   5.00,
			},
		},
	}
}

func createTestOrder(t *testing.T, handler http.Handler) types.Order {
	t.Helper()
	rec := doRequest(t, handler, http.MethodPost, "/orders", validOrderBody())
	if rec.Code != http.StatusCreated {
		t.Fatalf("create order status = %d, want %d", rec.Code, http.StatusCreated)
	}
	var order types.Order
	decodeBody(t, rec, &order)
	return order
}

func TestCreateOrder_ReturnsCreatedOrder(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodPost, "/orders", validOrderBody())

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var order types.Order
	decodeBody(t, rec, &order)
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
}

func TestCreateOrder_RejectsInvalidBody(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodPost, "/orders", map[string]interface{}{})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var body struct {
		Errors []string `json:"errors"`
	}
	decodeBody(t, rec, &body)
	if len(body.Errors) == 0 {
		t.Error("Errors is empty, want at least one validation error")
	}
}

func TestCreateOrder_FiresLogEntryOnFinish(t *testing.T) {
	handler, fake := newTestHandler(t)

	doRequest(t, handler, http.MethodPost, "/orders", validOrderBody())

	entries := fake.Entries()
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].Endpoint != "/orders" {
		t.Errorf("Endpoint = %q, want /orders", entries[0].Endpoint)
	}
	if entries[0].HTTPStatus != "201" {
		t.Errorf("HTTPStatus = %q, want 201", entries[0].HTTPStatus)
	}
}

func TestListOrders_ReturnsEmptyArrayWhenNoOrders(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, "/orders", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []types.Order
	decodeBody(t, rec, &got)
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

func TestListOrders_ReturnsAllCreatedOrders(t *testing.T) {
	handler, _ := newTestHandler(t)

	createTestOrder(t, handler)
	createTestOrder(t, handler)

	rec := doRequest(t, handler, http.MethodGet, "/orders", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []types.Order
	decodeBody(t, rec, &got)
	if len(got) != 2 {
		t.Errorf("len(got) = %d, want 2", len(got))
	}
}

func TestGetOrder_RejectsNonIntegerID(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, "/orders/abc", nil)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetOrder_ReturnsNotFoundForMissingOrder(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, "/orders/999", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetOrder_ReturnsOrderWhenFound(t *testing.T) {
	handler, _ := newTestHandler(t)

	created := createTestOrder(t, handler)

	rec := doRequest(t, handler, http.MethodGet, fmt.Sprintf("/orders/%d", created.ID), nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got types.Order
	decodeBody(t, rec, &got)
	if got.ID != created.ID || got.UserID != created.UserID {
		t.Errorf("got = %+v, want %+v", got, created)
	}
}

func TestUpdateOrder_RejectsNonIntegerID(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodPut, "/orders/abc", map[string]interface{}{"status": "shipped"})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateOrder_RejectsInvalidStatus(t *testing.T) {
	handler, _ := newTestHandler(t)

	created := createTestOrder(t, handler)

	rec := doRequest(t, handler, http.MethodPut, fmt.Sprintf("/orders/%d", created.ID), map[string]interface{}{"status": "unknown"})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateOrder_ReturnsNotFoundForMissingOrder(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodPut, "/orders/999", map[string]interface{}{"status": "shipped"})

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdateOrder_UpdatesStatusAndReturnsOrder(t *testing.T) {
	handler, _ := newTestHandler(t)

	created := createTestOrder(t, handler)

	rec := doRequest(t, handler, http.MethodPut, fmt.Sprintf("/orders/%d", created.ID), map[string]interface{}{"status": "shipped"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got types.Order
	decodeBody(t, rec, &got)
	if got.Status != types.OrderStatusShipped {
		t.Errorf("Status = %q, want shipped", got.Status)
	}
}

func TestDeleteOrder_RejectsNonIntegerID(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodDelete, "/orders/abc", nil)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteOrder_ReturnsNotFoundForMissingOrder(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodDelete, "/orders/999", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteOrder_DeletesOrderAndReturnsNoContent(t *testing.T) {
	handler, _ := newTestHandler(t)

	created := createTestOrder(t, handler)

	rec := doRequest(t, handler, http.MethodDelete, fmt.Sprintf("/orders/%d", created.ID), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	getRec := doRequest(t, handler, http.MethodGet, fmt.Sprintf("/orders/%d", created.ID), nil)
	if getRec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

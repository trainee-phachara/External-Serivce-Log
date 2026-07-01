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

	"user-service/internal/db"
	"user-service/internal/types"
	"user-service/internal/users"
)

func newTestHandler(t *testing.T) (http.Handler, *fakeLogClient) {
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

	repo := users.NewRepository(pool)
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

func createTestUser(t *testing.T, handler http.Handler, name, email string) types.User {
	t.Helper()
	rec := doRequest(t, handler, http.MethodPost, "/users", map[string]interface{}{"name": name, "email": email})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user status = %d, want %d", rec.Code, http.StatusCreated)
	}
	var user types.User
	decodeBody(t, rec, &user)
	return user
}

func TestCreateUser_ReturnsCreatedUser(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodPost, "/users", map[string]interface{}{"name": "Alice", "email": "alice@example.com"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var user types.User
	decodeBody(t, rec, &user)
	if user.Name != "Alice" || user.Email != "alice@example.com" {
		t.Errorf("user = %+v, want name=Alice email=alice@example.com", user)
	}
	if user.ID == 0 {
		t.Error("ID = 0, want non-zero")
	}
}

func TestCreateUser_RejectsInvalidBody(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodPost, "/users", map[string]interface{}{"name": ""})

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

func TestCreateUser_FiresLogEntryOnFinish(t *testing.T) {
	handler, fake := newTestHandler(t)

	doRequest(t, handler, http.MethodPost, "/users", map[string]interface{}{"name": "Alice", "email": "alice@example.com"})

	entries := fake.Entries()
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].Endpoint != "/users" {
		t.Errorf("Endpoint = %q, want %q", entries[0].Endpoint, "/users")
	}
	if entries[0].HTTPStatus != "201" {
		t.Errorf("HTTPStatus = %q, want %q", entries[0].HTTPStatus, "201")
	}
}

func TestListUsers_ReturnsEmptyArrayWhenNoUsers(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, "/users", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []types.User
	decodeBody(t, rec, &got)
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

func TestListUsers_ReturnsAllCreatedUsers(t *testing.T) {
	handler, _ := newTestHandler(t)

	createTestUser(t, handler, "Alice", "alice@example.com")
	createTestUser(t, handler, "Bob", "bob@example.com")

	rec := doRequest(t, handler, http.MethodGet, "/users", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []types.User
	decodeBody(t, rec, &got)
	if len(got) != 2 {
		t.Errorf("len(got) = %d, want 2", len(got))
	}
}

func TestGetUser_RejectsNonIntegerID(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, "/users/abc", nil)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetUser_ReturnsNotFoundForMissingUser(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodGet, "/users/999", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetUser_ReturnsUserWhenFound(t *testing.T) {
	handler, _ := newTestHandler(t)

	created := createTestUser(t, handler, "Alice", "alice@example.com")

	rec := doRequest(t, handler, http.MethodGet, fmt.Sprintf("/users/%d", created.ID), nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got types.User
	decodeBody(t, rec, &got)
	if got.ID != created.ID || got.Name != created.Name || got.Email != created.Email {
		t.Errorf("got = %+v, want %+v", got, created)
	}
	if !got.CreatedAt.Equal(created.CreatedAt) || !got.UpdatedAt.Equal(created.UpdatedAt) {
		t.Errorf("timestamps differ: got = %+v, want %+v", got, created)
	}
}

func TestUpdateUser_RejectsNonIntegerID(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodPut, "/users/abc", map[string]interface{}{"name": "New"})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateUser_RejectsInvalidBody(t *testing.T) {
	handler, _ := newTestHandler(t)

	created := createTestUser(t, handler, "Alice", "alice@example.com")

	rec := doRequest(t, handler, http.MethodPut, fmt.Sprintf("/users/%d", created.ID), map[string]interface{}{})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateUser_ReturnsNotFoundForMissingUser(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodPut, "/users/999", map[string]interface{}{"name": "New"})

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdateUser_UpdatesAndReturnsUser(t *testing.T) {
	handler, _ := newTestHandler(t)

	created := createTestUser(t, handler, "Alice", "alice@example.com")

	rec := doRequest(t, handler, http.MethodPut, fmt.Sprintf("/users/%d", created.ID), map[string]interface{}{"name": "Alice Updated"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got types.User
	decodeBody(t, rec, &got)
	if got.Name != "Alice Updated" {
		t.Errorf("Name = %q, want %q", got.Name, "Alice Updated")
	}
	if got.Email != "alice@example.com" {
		t.Errorf("Email = %q, want %q", got.Email, "alice@example.com")
	}
}

func TestDeleteUser_RejectsNonIntegerID(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodDelete, "/users/abc", nil)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteUser_ReturnsNotFoundForMissingUser(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := doRequest(t, handler, http.MethodDelete, "/users/999", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteUser_DeletesUserAndReturnsNoContent(t *testing.T) {
	handler, _ := newTestHandler(t)

	created := createTestUser(t, handler, "Alice", "alice@example.com")

	rec := doRequest(t, handler, http.MethodDelete, fmt.Sprintf("/users/%d", created.ID), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	getRec := doRequest(t, handler, http.MethodGet, fmt.Sprintf("/users/%d", created.ID), nil)
	if getRec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

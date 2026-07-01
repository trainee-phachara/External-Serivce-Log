package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	logclient "github.com/trainee-phachara/External-Serivce-Log/client"
)

// fakeLogClient records every entry passed to SendLog for inspection in tests.
type fakeLogClient struct {
	mu      sync.Mutex
	entries []logclient.LogEntryInput
}

func (f *fakeLogClient) SendLog(entry logclient.LogEntryInput) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, entry)
}

func (f *fakeLogClient) Close() error { return nil }

func (f *fakeLogClient) Entries() []logclient.LogEntryInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]logclient.LogEntryInput(nil), f.entries...)
}

func TestRequestLogger_SendsLogEntryOnFinish(t *testing.T) {
	fake := &fakeLogClient{}
	handler := withRequestLogger(fake, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusCreated, map[string]interface{}{"id": 1})
	}))

	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(`{"user_id":1,"items":[]}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	entries := fake.Entries()
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	entry := entries[0]

	if entry.Source != (logclient.LogSource{AppName: "order-service", ServiceName: "order"}) {
		t.Errorf("Source = %+v, want {order-service order}", entry.Source)
	}
	if entry.TraceID == "" {
		t.Error("TraceID is empty, want a generated id")
	}
	if entry.Endpoint != "/orders" {
		t.Errorf("Endpoint = %q, want %q", entry.Endpoint, "/orders")
	}
	if entry.HTTPStatus != "201" {
		t.Errorf("HTTPStatus = %q, want %q", entry.HTTPStatus, "201")
	}
	if entry.Type != "response" {
		t.Errorf("Type = %q, want response", entry.Type)
	}
	if entry.Direction != "inbound" {
		t.Errorf("Direction = %q, want inbound", entry.Direction)
	}

	var metadata map[string]string
	if err := json.Unmarshal([]byte(entry.MetadataJSON), &metadata); err != nil {
		t.Fatalf("unmarshal metadata_json: %v", err)
	}
	if metadata["method"] != "POST" {
		t.Errorf("metadata method = %q, want POST", metadata["method"])
	}
}

func TestRequestLogger_DefaultsPayloadsToEmptyObject(t *testing.T) {
	fake := &fakeLogClient{}
	handler := withRequestLogger(fake, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodDelete, "/orders/1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	entries := fake.Entries()
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	entry := entries[0]

	if entry.HTTPStatus != "204" {
		t.Errorf("HTTPStatus = %q, want 204", entry.HTTPStatus)
	}
	if entry.RawPayloadJSON != "{}" {
		t.Errorf("RawPayloadJSON = %q, want {}", entry.RawPayloadJSON)
	}
	if entry.PayloadJSON != "{}" {
		t.Errorf("PayloadJSON = %q, want {}", entry.PayloadJSON)
	}
}

func TestRequestLogger_GeneratesDifferentTraceIDsPerRequest(t *testing.T) {
	fake := &fakeLogClient{}
	handler := withRequestLogger(fake, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/orders", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	entries := fake.Entries()
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].TraceID == entries[1].TraceID {
		t.Errorf("TraceID1 = TraceID2 = %q, want different ids", entries[0].TraceID)
	}
}

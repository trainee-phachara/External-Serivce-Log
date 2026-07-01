package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"external-service-log/internal/buffer"
	"external-service-log/internal/flusher"
	"external-service-log/internal/logstore"
	"external-service-log/internal/types"
)

type fakeStore struct {
	entries []types.LogEntry
	err     error
	filter  logstore.FindLogsFilter
}

func (f *fakeStore) InsertLogs(_ context.Context, _ []types.BufferedLog) error { return nil }
func (f *fakeStore) FindLogs(_ context.Context, filter logstore.FindLogsFilter) ([]types.LogEntry, error) {
	f.filter = filter
	return f.entries, f.err
}

func validPayload(overrides map[string]interface{}) map[string]interface{} {
	body := map[string]interface{}{
		"source": map[string]interface{}{
			"app_name":     "order-service",
			"service_name": "order",
		},
		"trace_id":    "trace-123",
		"endpoint":    "/api/v1/orders",
		"http_status": "200",
		"type":        "request",
		"direction":   "inbound",
		"metadata":    map[string]interface{}{"user_id": "u-1"},
		"raw_payload": map[string]interface{}{"raw": true},
		"payload":     map[string]interface{}{"id": float64(1)},
	}
	for k, v := range overrides {
		body[k] = v
	}
	return body
}

func newTestServer(maxSize int) (http.Handler, *buffer.LogBuffer, *int32, *int32, *flusher.BatchFlusher) {
	buf := buffer.New()
	var calls int32
	var total int32
	insert := func(_ context.Context, logs []types.BufferedLog) error {
		atomic.AddInt32(&calls, 1)
		atomic.AddInt32(&total, int32(len(logs)))
		return nil
	}
	fl := flusher.New(buf, insert, flusher.Options{MaxSize: maxSize, Interval: 5 * time.Second})
	return NewHandler(buf, fl, &fakeStore{}), buf, &calls, &total, fl
}

func postIngest(handler http.Handler, body interface{}) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return body
}

func TestIngest_AcceptsValidLogAndPushesToBuffer(t *testing.T) {
	handler, buf, _, _, _ := newTestServer(3)

	rec := postIngest(handler, validPayload(nil))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if body := decodeBody(t, rec); body["status"] != "accepted" {
		t.Errorf("body = %v, want status=accepted", body)
	}
	if got := buf.Size(); got != 1 {
		t.Errorf("buffer size = %d, want 1", got)
	}
}

func TestIngest_StoresEntryShape(t *testing.T) {
	handler, buf, _, _, _ := newTestServer(3)

	postIngest(handler, validPayload(map[string]interface{}{"trace_id": "trace-abc"}))

	entry := buf.Drain()[0]
	if entry.Entry.TraceID != "trace-abc" {
		t.Errorf("trace_id = %q, want %q", entry.Entry.TraceID, "trace-abc")
	}
	wantSource := types.LogSource{AppName: "order-service", ServiceName: "order"}
	if entry.Entry.Source != wantSource {
		t.Errorf("source = %+v, want %+v", entry.Entry.Source, wantSource)
	}
	if entry.Entry.Timestamp.IsZero() {
		t.Error("timestamp is zero, want a recent time")
	}
}


func TestIngest_RejectsMissingRequiredFields(t *testing.T) {
	handler, buf, _, _, _ := newTestServer(3)

	rec := postIngest(handler, map[string]interface{}{"trace_id": "trace-1"})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	body := decodeBody(t, rec)
	errs, ok := body["errors"].([]interface{})
	if !ok || len(errs) == 0 {
		t.Errorf("errors = %v, want non-empty array", body["errors"])
	}
	if got := buf.Size(); got != 0 {
		t.Errorf("buffer size = %d, want 0", got)
	}
}

func TestIngest_RejectsInvalidTypeAndDirection(t *testing.T) {
	handler, buf, _, _, _ := newTestServer(3)

	rec := postIngest(handler, validPayload(map[string]interface{}{
		"type":      "invalid-type",
		"direction": "invalid-direction",
	}))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	body := decodeBody(t, rec)
	errs, _ := body["errors"].([]interface{})
	var sawType, sawDirection bool
	for _, e := range errs {
		s, _ := e.(string)
		if strings.Contains(s, "type is required") {
			sawType = true
		}
		if strings.Contains(s, "direction is required") {
			sawDirection = true
		}
	}
	if !sawType || !sawDirection {
		t.Errorf("errors = %v, want entries mentioning type and direction", errs)
	}
	if got := buf.Size(); got != 0 {
		t.Errorf("buffer size = %d, want 0", got)
	}
}

func TestIngest_TriggersFlushAtThreshold(t *testing.T) {
	handler, buf, calls, total, fl := newTestServer(3)

	postIngest(handler, validPayload(map[string]interface{}{"trace_id": "trace-1"}))
	postIngest(handler, validPayload(map[string]interface{}{"trace_id": "trace-2"}))

	if got := atomic.LoadInt32(calls); got != 0 {
		t.Fatalf("insert called %d times before threshold, want 0", got)
	}

	postIngest(handler, validPayload(map[string]interface{}{"trace_id": "trace-3"}))

	if err := fl.Flush(context.Background()); err != nil { // sync point
		t.Fatalf("Flush returned error: %v", err)
	}

	if got := atomic.LoadInt32(calls); got != 1 {
		t.Errorf("insert called %d times, want 1", got)
	}
	if got := atomic.LoadInt32(total); got != 3 {
		t.Errorf("insert received %d logs total, want 3", got)
	}
	if !buf.IsEmpty() {
		t.Error("buffer not empty after threshold flush")
	}
}

func TestGetLogs_ReturnsEntriesFromStore(t *testing.T) {
	store := &fakeStore{
		entries: []types.LogEntry{
			{TraceID: "t-1", Type: types.LogTypeRequest},
			{TraceID: "t-2", Type: types.LogTypeResponse},
		},
	}
	buf := buffer.New()
	fl := flusher.New(buf, store.InsertLogs, flusher.Options{MaxSize: 100, Interval: 5 * time.Second})
	handler := NewHandler(buf, fl, store)

	req := httptest.NewRequest(http.MethodGet, "/logs?type=request&app=order-service&limit=10", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if store.filter.Type != types.LogTypeRequest {
		t.Errorf("filter.Type = %q, want %q", store.filter.Type, types.LogTypeRequest)
	}
	if store.filter.AppName != "order-service" {
		t.Errorf("filter.AppName = %q, want %q", store.filter.AppName, "order-service")
	}
	if store.filter.Limit != 10 {
		t.Errorf("filter.Limit = %d, want 10", store.filter.Limit)
	}

	var entries []types.LogEntry
	if err := json.NewDecoder(rec.Body).Decode(&entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("len(entries) = %d, want 2", len(entries))
	}
}

func TestGetLogs_ReturnsErrorFromStore(t *testing.T) {
	store := &fakeStore{err: fmt.Errorf("db down")}
	buf := buffer.New()
	fl := flusher.New(buf, store.InsertLogs, flusher.Options{MaxSize: 100, Interval: 5 * time.Second})
	handler := NewHandler(buf, fl, store)

	req := httptest.NewRequest(http.MethodGet, "/logs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestCORS_OptionsReturnsNoContent(t *testing.T) {
	handler, _, _, _, _ := newTestServer(3)

	req := httptest.NewRequest(http.MethodOptions, "/logs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("CORS origin = %q, want %q", got, "*")
	}
}

func TestIngest_DefaultsOptionalFieldsToEmptyObjects(t *testing.T) {
	handler, buf, _, _, _ := newTestServer(3)

	postIngest(handler, map[string]interface{}{
		"source":      map[string]interface{}{"app_name": "user-service", "service_name": "user"},
		"trace_id":    "trace-minimal",
		"endpoint":    "/api/v1/users",
		"http_status": "201",
		"type":        "response",
		"direction":   "outbound",
	})

	entry := buf.Drain()[0].Entry
	if len(entry.Metadata) != 0 {
		t.Errorf("metadata = %v, want empty", entry.Metadata)
	}
	if len(entry.RawPayload) != 0 {
		t.Errorf("raw_payload = %v, want empty", entry.RawPayload)
	}
	if len(entry.Payload) != 0 {
		t.Errorf("payload = %v, want empty", entry.Payload)
	}
}

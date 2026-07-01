package ingest

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"external-service-log/internal/buffer"
	"external-service-log/internal/flusher"
	"external-service-log/internal/types"
)

func validIngestBody(overrides map[string]interface{}) map[string]interface{} {
	body := map[string]interface{}{
		"source": map[string]interface{}{
			"app_name":     "order-service",
			"service_name": "order",
		},
		"trace_id":    "trace-1",
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

func newTestFlusher(buf *buffer.LogBuffer) (*flusher.BatchFlusher, *int32, *int32) {
	var calls int32
	var total int32
	insert := func(_ context.Context, logs []types.BufferedLog) error {
		atomic.AddInt32(&calls, 1)
		atomic.AddInt32(&total, int32(len(logs)))
		return nil
	}
	return flusher.New(buf, insert, flusher.Options{MaxSize: 3, Interval: 5 * time.Second}), &calls, &total
}

func TestProcessIngest_AcceptsValidBody(t *testing.T) {
	buf := buffer.New()
	fl, _, _ := newTestFlusher(buf)

	result := ProcessIngest(validIngestBody(map[string]interface{}{"trace_id": "trace-abc"}), buf, fl)

	if !result.Accepted || len(result.Errors) != 0 {
		t.Fatalf("result = %+v, want accepted with no errors", result)
	}
	if got := buf.Size(); got != 1 {
		t.Fatalf("buffer size = %d, want 1", got)
	}

	drained := buf.Drain()
	entry := drained[0]
	if entry.Entry.TraceID != "trace-abc" {
		t.Errorf("trace_id = %q, want %q", entry.Entry.TraceID, "trace-abc")
	}
	if entry.Entry.Timestamp.IsZero() {
		t.Error("timestamp is zero, want a recent time")
	}
}


func TestProcessIngest_DefaultsOptionalFieldsToEmptyObjects(t *testing.T) {
	buf := buffer.New()
	fl, _, _ := newTestFlusher(buf)

	body := validIngestBody(nil)
	delete(body, "metadata")
	delete(body, "raw_payload")
	delete(body, "payload")

	ProcessIngest(body, buf, fl)

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

func TestProcessIngest_RejectsInvalidBodyWithoutTouchingBuffer(t *testing.T) {
	buf := buffer.New()
	fl, _, _ := newTestFlusher(buf)

	result := ProcessIngest(map[string]interface{}{"trace_id": "trace-1"}, buf, fl)

	if result.Accepted {
		t.Error("Accepted = true, want false")
	}
	if len(result.Errors) == 0 {
		t.Error("Errors is empty, want at least one error")
	}
	if got := buf.Size(); got != 0 {
		t.Errorf("buffer size = %d, want 0", got)
	}
}

func TestProcessIngest_TriggersFlushAtThreshold(t *testing.T) {
	buf := buffer.New()
	fl, calls, total := newTestFlusher(buf)

	ProcessIngest(validIngestBody(map[string]interface{}{"trace_id": "trace-1"}), buf, fl)
	ProcessIngest(validIngestBody(map[string]interface{}{"trace_id": "trace-2"}), buf, fl)

	if got := atomic.LoadInt32(calls); got != 0 {
		t.Errorf("insert called %d times before threshold, want 0", got)
	}

	ProcessIngest(validIngestBody(map[string]interface{}{"trace_id": "trace-3"}), buf, fl)

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

package buffer

import (
	"testing"
	"time"

	"external-service-log/internal/types"
)

func makeLog(traceID string) types.BufferedLog {
	return types.BufferedLog{
		Entry: types.LogEntry{
			Timestamp:  time.Now(),
			Source:     types.LogSource{AppName: "order-service", ServiceName: "order"},
			TraceID:    traceID,
			Metadata:   map[string]interface{}{},
			Endpoint:   "/api/v1/orders",
			HTTPStatus: "200",
			RawPayload: map[string]interface{}{},
			Payload:    map[string]interface{}{},
			Type:       types.LogTypeRequest,
			Direction:  types.LogDirectionInbound,
		},
	}
}

func TestLogBuffer_StartsEmpty(t *testing.T) {
	b := New()
	if got := b.Size(); got != 0 {
		t.Errorf("Size() = %d, want 0", got)
	}
	if !b.IsEmpty() {
		t.Error("IsEmpty() = false, want true")
	}
}

func TestLogBuffer_PushIncreasesSize(t *testing.T) {
	b := New()
	b.Push(makeLog("trace-1"))
	b.Push(makeLog("trace-2"))

	if got := b.Size(); got != 2 {
		t.Errorf("Size() = %d, want 2", got)
	}
	if b.IsEmpty() {
		t.Error("IsEmpty() = true, want false")
	}
}

func TestLogBuffer_DrainEmptiesBuffer(t *testing.T) {
	b := New()
	b.Push(makeLog("trace-1"))
	b.Push(makeLog("trace-2"))

	drained := b.Drain()

	if len(drained) != 2 {
		t.Fatalf("len(drained) = %d, want 2", len(drained))
	}
	if drained[0].Entry.TraceID != "trace-1" {
		t.Errorf("drained[0].Entry.TraceID = %q, want %q", drained[0].Entry.TraceID, "trace-1")
	}
	if drained[1].Entry.TraceID != "trace-2" {
		t.Errorf("drained[1].Entry.TraceID = %q, want %q", drained[1].Entry.TraceID, "trace-2")
	}
	if got := b.Size(); got != 0 {
		t.Errorf("Size() after drain = %d, want 0", got)
	}
	if !b.IsEmpty() {
		t.Error("IsEmpty() after drain = false, want true")
	}
}

func TestLogBuffer_DrainEmptyBuffer(t *testing.T) {
	b := New()
	drained := b.Drain()
	if len(drained) != 0 {
		t.Errorf("len(drained) = %d, want 0", len(drained))
	}
}

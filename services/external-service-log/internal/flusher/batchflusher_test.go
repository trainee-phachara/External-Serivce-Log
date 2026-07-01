package flusher

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"external-service-log/internal/buffer"
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

// countingInsert returns an InsertFunc plus pointers to the number of times
// it was called and the total number of logs it received.
func countingInsert() (InsertFunc, *int32, *int32) {
	var calls int32
	var total int32
	fn := func(_ context.Context, logs []types.BufferedLog) error {
		atomic.AddInt32(&calls, 1)
		atomic.AddInt32(&total, int32(len(logs)))
		return nil
	}
	return fn, &calls, &total
}

func TestBatchFlusher_DoesNotInsertWhenBufferEmpty(t *testing.T) {
	buf := buffer.New()
	insert, calls, _ := countingInsert()
	f := New(buf, insert, Options{MaxSize: 100, Interval: 5 * time.Second})

	if err := f.Flush(context.Background()); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}

	if got := atomic.LoadInt32(calls); got != 0 {
		t.Errorf("insert called %d times, want 0", got)
	}
}

func TestBatchFlusher_FlushDrainsBuffer(t *testing.T) {
	buf := buffer.New()
	insert, calls, total := countingInsert()
	f := New(buf, insert, Options{MaxSize: 100, Interval: 5 * time.Second})

	buf.Push(makeLog("trace-1"))
	buf.Push(makeLog("trace-2"))

	if err := f.Flush(context.Background()); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}

	if got := atomic.LoadInt32(calls); got != 1 {
		t.Errorf("insert called %d times, want 1", got)
	}
	if got := atomic.LoadInt32(total); got != 2 {
		t.Errorf("insert received %d logs total, want 2", got)
	}
	if !buf.IsEmpty() {
		t.Error("buffer not empty after flush")
	}
}

func TestBatchFlusher_OnLogPushedTriggersFlushAtThreshold(t *testing.T) {
	buf := buffer.New()
	insert, calls, total := countingInsert()
	f := New(buf, insert, Options{MaxSize: 3, Interval: 5 * time.Second})

	buf.Push(makeLog("trace-1"))
	f.OnLogPushed()
	buf.Push(makeLog("trace-2"))
	f.OnLogPushed()

	if got := atomic.LoadInt32(calls); got != 0 {
		t.Errorf("insert called %d times before threshold, want 0", got)
	}

	buf.Push(makeLog("trace-3"))
	f.OnLogPushed()

	// OnLogPushed's flush may run in a separate goroutine; calling Flush
	// here blocks until any in-flight flush completes (then no-ops if the
	// buffer is already drained), giving us a deterministic sync point.
	if err := f.Flush(context.Background()); err != nil {
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

func TestBatchFlusher_OnLogPushedDoesNotFlushBelowThreshold(t *testing.T) {
	buf := buffer.New()
	insert, calls, _ := countingInsert()
	f := New(buf, insert, Options{MaxSize: 100, Interval: 5 * time.Second})

	buf.Push(makeLog("trace-1"))
	f.OnLogPushed()

	if got := atomic.LoadInt32(calls); got != 0 {
		t.Errorf("insert called %d times, want 0", got)
	}
	if got := buf.Size(); got != 1 {
		t.Errorf("buffer size = %d, want 1", got)
	}
}

func TestBatchFlusher_FlushesOnTick(t *testing.T) {
	buf := buffer.New()
	insert, calls, _ := countingInsert()
	f := New(buf, insert, Options{MaxSize: 100, Interval: 5 * time.Second})

	tick := make(chan time.Time, 1)
	stop := make(chan struct{})
	done := make(chan struct{})
	f.runLoop(tick, func() {}, stop, done)

	buf.Push(makeLog("trace-1"))
	tick <- time.Now()
	if err := f.Flush(context.Background()); err != nil { // sync point
		t.Fatalf("Flush returned error: %v", err)
	}

	if got := atomic.LoadInt32(calls); got != 1 {
		t.Errorf("insert called %d times, want 1", got)
	}
	if !buf.IsEmpty() {
		t.Error("buffer not empty after tick flush")
	}

	buf.Push(makeLog("trace-2"))
	tick <- time.Now()
	if err := f.Flush(context.Background()); err != nil { // sync point
		t.Fatalf("Flush returned error: %v", err)
	}

	if got := atomic.LoadInt32(calls); got != 2 {
		t.Errorf("insert called %d times, want 2", got)
	}

	close(stop)
	<-done
}

func TestBatchFlusher_StartIsNoOpWhenAlreadyStarted(t *testing.T) {
	buf := buffer.New()
	insert, _, _ := countingInsert()
	f := New(buf, insert, Options{MaxSize: 100, Interval: 5 * time.Second})

	f.Start()
	firstStopCh := f.stopCh

	f.Start()
	secondStopCh := f.stopCh

	if firstStopCh != secondStopCh {
		t.Error("Start() created a new ticker loop when already started")
	}

	f.Stop()
}

func TestBatchFlusher_StopPreventsFurtherFlushes(t *testing.T) {
	buf := buffer.New()
	insert, calls, _ := countingInsert()
	f := New(buf, insert, Options{MaxSize: 100, Interval: time.Millisecond})

	f.Start()
	f.Stop()

	buf.Push(makeLog("trace-1"))

	// Stop() joins the ticker goroutine before returning, so no further
	// ticks can be processed regardless of how long we wait.
	time.Sleep(5 * time.Millisecond)

	if got := atomic.LoadInt32(calls); got != 0 {
		t.Errorf("insert called %d times after Stop, want 0", got)
	}
	if got := buf.Size(); got != 1 {
		t.Errorf("buffer size = %d, want 1 (untouched)", got)
	}
}

func TestBatchFlusher_SerializesConcurrentFlushes(t *testing.T) {
	buf := buffer.New()

	var activeInserts int32
	var overlapped int32
	var totalLogs int32
	insert := func(_ context.Context, logs []types.BufferedLog) error {
		if atomic.AddInt32(&activeInserts, 1) > 1 {
			atomic.StoreInt32(&overlapped, 1)
		}
		time.Sleep(time.Millisecond)
		atomic.AddInt32(&totalLogs, int32(len(logs)))
		atomic.AddInt32(&activeInserts, -1)
		return nil
	}

	f := New(buf, insert, Options{MaxSize: 100, Interval: 5 * time.Second})

	var wg sync.WaitGroup
	for i, traceID := range []string{"trace-1", "trace-2", "trace-3"} {
		wg.Add(1)
		go func(i int, traceID string) {
			defer wg.Done()
			buf.Push(makeLog(traceID))
			_ = f.Flush(context.Background())
		}(i, traceID)
	}
	wg.Wait()

	if atomic.LoadInt32(&overlapped) != 0 {
		t.Error("inserts overlapped, want serialized")
	}
	if got := atomic.LoadInt32(&totalLogs); got != 3 {
		t.Errorf("total logs inserted = %d, want 3", got)
	}
	if !buf.IsEmpty() {
		t.Error("buffer should be empty after all flushes complete")
	}
}

package flusher

import (
	"context"
	"sync"
	"time"

	"external-service-log/internal/buffer"
	"external-service-log/internal/types"
)

// InsertFunc persists a batch of buffered logs (e.g. to MongoDB).
type InsertFunc func(ctx context.Context, logs []types.BufferedLog) error

// Options configures a BatchFlusher.
type Options struct {
	// MaxSize is the buffer size at which OnLogPushed triggers an immediate flush.
	MaxSize int
	// Interval is how often the ticker triggers a flush while started.
	Interval time.Duration
}

// BatchFlusher periodically (and on-demand) drains a LogBuffer and persists
// the drained logs via Insert. Flushes are serialized via a mutex so that
// concurrent triggers (ticker + size threshold) never run doFlush in
// parallel, mirroring the promise-chaining guarantee of the original
// TypeScript implementation.
type BatchFlusher struct {
	buffer *buffer.LogBuffer
	insert InsertFunc
	opts   Options

	flushMu sync.Mutex

	startMu sync.Mutex
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// New creates a BatchFlusher for buffer, using insert to persist drained logs.
func New(buf *buffer.LogBuffer, insert InsertFunc, opts Options) *BatchFlusher {
	return &BatchFlusher{
		buffer: buf,
		insert: insert,
		opts:   opts,
	}
}

// Start begins the periodic ticker that triggers a flush every
// Options.Interval. Calling Start when already started is a no-op.
func (f *BatchFlusher) Start() {
	f.startMu.Lock()
	defer f.startMu.Unlock()
	if f.stopCh != nil {
		return
	}

	ticker := time.NewTicker(f.opts.Interval)
	f.stopCh = make(chan struct{})
	f.doneCh = make(chan struct{})
	f.runLoop(ticker.C, ticker.Stop, f.stopCh, f.doneCh)
}

// runLoop drives the flush-on-tick goroutine; split out so tests can supply
// a manually-controlled tick channel for deterministic timing.
func (f *BatchFlusher) runLoop(tick <-chan time.Time, stopTicker func(), stop <-chan struct{}, done chan<- struct{}) {
	go func() {
		defer close(done)
		defer stopTicker()
		for {
			select {
			case <-tick:
				_ = f.Flush(context.Background())
			case <-stop:
				return
			}
		}
	}()
}

// Stop halts the ticker started by Start and waits for its goroutine to
// exit. Calling Stop when not started is a no-op.
func (f *BatchFlusher) Stop() {
	f.startMu.Lock()
	defer f.startMu.Unlock()
	if f.stopCh == nil {
		return
	}
	close(f.stopCh)
	<-f.doneCh
	f.stopCh = nil
	f.doneCh = nil
}

// OnLogPushed should be called after pushing a log onto the buffer; it
// triggers an immediate flush once the buffer reaches Options.MaxSize.
func (f *BatchFlusher) OnLogPushed() {
	if f.buffer.Size() >= f.opts.MaxSize {
		go func() {
			_ = f.Flush(context.Background())
		}()
	}
}

// Flush drains the buffer and inserts the drained logs. Concurrent calls to
// Flush are serialized: a call that arrives while another is in progress
// waits its turn, then (typically) finds the buffer already drained and
// becomes a no-op.
func (f *BatchFlusher) Flush(ctx context.Context) error {
	f.flushMu.Lock()
	defer f.flushMu.Unlock()
	return f.doFlush(ctx)
}

func (f *BatchFlusher) doFlush(ctx context.Context) error {
	if f.buffer.IsEmpty() {
		return nil
	}
	logs := f.buffer.Drain()
	return f.insert(ctx, logs)
}

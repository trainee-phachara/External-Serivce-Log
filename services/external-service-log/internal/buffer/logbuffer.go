package buffer

import (
	"sync"

	"external-service-log/internal/types"
)

// LogBuffer is an in-memory, concurrency-safe queue of buffered logs awaiting
// a batch flush to MongoDB.
type LogBuffer struct {
	mu    sync.Mutex
	items []types.BufferedLog
}

// New creates an empty LogBuffer.
func New() *LogBuffer {
	return &LogBuffer{}
}

// Push appends a log to the buffer.
func (b *LogBuffer) Push(log types.BufferedLog) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items = append(b.items, log)
}

// Size returns the number of buffered logs.
func (b *LogBuffer) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.items)
}

// IsEmpty reports whether the buffer has no items.
func (b *LogBuffer) IsEmpty() bool {
	return b.Size() == 0
}

// Drain atomically removes and returns all buffered logs.
func (b *LogBuffer) Drain() []types.BufferedLog {
	b.mu.Lock()
	defer b.mu.Unlock()
	drained := b.items
	b.items = nil
	return drained
}

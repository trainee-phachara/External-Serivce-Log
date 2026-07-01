package logstore

import (
	"context"

	"external-service-log/internal/types"
)

// FindLogsFilter controls which logs a LogStore returns.
type FindLogsFilter struct {
	AppName string         // empty = all apps
	Type    types.LogType  // empty = all types
	Limit   int64          // 0 = default 50
}

// LogStore is the interface any log storage backend must implement.
// Swapping MongoDB for another database only requires implementing this interface.
type LogStore interface {
	InsertLogs(ctx context.Context, logs []types.BufferedLog) error
	FindLogs(ctx context.Context, filter FindLogsFilter) ([]types.LogEntry, error)
}

package mongostore

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"

	"external-service-log/internal/types"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()

	container, err := mongodb.Run(ctx, "mongo:7")
	if err != nil {
		t.Fatalf("start mongodb container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Errorf("terminate mongodb container: %v", err)
		}
	})

	uri, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	store, err := Connect(ctx, uri, "service_logs_test")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(context.Background()); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	return store
}

func TestConnect_CreatesTimeSeriesCollections(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	specs, err := store.db.ListCollectionSpecifications(ctx, bson.D{})
	if err != nil {
		t.Fatalf("list collection specifications: %v", err)
	}

	byName := make(map[string]bson.Raw, len(specs))
	for _, spec := range specs {
		byName[spec.Name] = spec.Options
	}

	for _, name := range collectionNames {
		opts, ok := byName[string(name)]
		if !ok {
			t.Errorf("collection %s was not created", name)
			continue
		}

		timeField := opts.Lookup("timeseries", "timeField").StringValue()
		if timeField != "timestamp" {
			t.Errorf("collection %s timeField = %q, want %q", name, timeField, "timestamp")
		}

		metaField := opts.Lookup("timeseries", "metaField").StringValue()
		if metaField != "source" {
			t.Errorf("collection %s metaField = %q, want %q", name, metaField, "source")
		}
	}
}

func TestConnect_IsIdempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := ensureTimeSeriesCollections(ctx, store.db); err != nil {
		t.Fatalf("second ensureTimeSeriesCollections call returned error: %v", err)
	}
}

func TestInsertLogs_GroupsByCollectionAndInserts(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	source := types.LogSource{AppName: "order-service", ServiceName: "order"}
	makeEntry := func(traceID string, logType types.LogType) types.LogEntry {
		return types.LogEntry{
			Timestamp:  time.Now(),
			Source:     source,
			TraceID:    traceID,
			Metadata:   map[string]interface{}{},
			Endpoint:   "/api/v1/orders",
			HTTPStatus: "200",
			RawPayload: map[string]interface{}{},
			Payload:    map[string]interface{}{},
			Type:       logType,
			Direction:  types.LogDirectionInbound,
		}
	}

	logs := []types.BufferedLog{
		{Collection: types.CollectionAPILogs, Entry: makeEntry("trace-1", types.LogTypeRequest)},
		{Collection: types.CollectionAPILogs, Entry: makeEntry("trace-2", types.LogTypeResponse)},
		{Collection: types.CollectionErrorLogs, Entry: makeEntry("trace-3", types.LogTypeResponse)},
	}

	if err := store.InsertLogs(ctx, logs); err != nil {
		t.Fatalf("InsertLogs: %v", err)
	}

	var apiLogs []types.LogEntry
	cursor, err := store.db.Collection(string(types.CollectionAPILogs)).Find(ctx, bson.D{})
	if err != nil {
		t.Fatalf("find api_logs: %v", err)
	}
	if err := cursor.All(ctx, &apiLogs); err != nil {
		t.Fatalf("decode api_logs: %v", err)
	}
	if len(apiLogs) != 2 {
		t.Errorf("api_logs count = %d, want 2", len(apiLogs))
	}

	var errorLogs []types.LogEntry
	cursor, err = store.db.Collection(string(types.CollectionErrorLogs)).Find(ctx, bson.D{})
	if err != nil {
		t.Fatalf("find error_logs: %v", err)
	}
	if err := cursor.All(ctx, &errorLogs); err != nil {
		t.Fatalf("decode error_logs: %v", err)
	}
	if len(errorLogs) != 1 {
		t.Errorf("error_logs count = %d, want 1", len(errorLogs))
	}
	if len(errorLogs) == 1 && errorLogs[0].TraceID != "trace-3" {
		t.Errorf("error_logs[0].TraceID = %q, want %q", errorLogs[0].TraceID, "trace-3")
	}

	count, err := store.db.Collection(string(types.CollectionEventLogs)).CountDocuments(ctx, bson.D{})
	if err != nil {
		t.Fatalf("count event_logs: %v", err)
	}
	if count != 0 {
		t.Errorf("event_logs count = %d, want 0", count)
	}
}

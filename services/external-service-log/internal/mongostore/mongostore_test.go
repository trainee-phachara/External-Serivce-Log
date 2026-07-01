package mongostore

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"

	"external-service-log/internal/logstore"
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

func TestConnect_CreatesTimeSeriesCollection(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	specs, err := store.db.ListCollectionSpecifications(ctx, bson.D{})
	if err != nil {
		t.Fatalf("list collection specifications: %v", err)
	}

	var found bool
	for _, spec := range specs {
		if spec.Name != types.CollectionName {
			continue
		}
		found = true

		timeField := spec.Options.Lookup("timeseries", "timeField").StringValue()
		if timeField != "timestamp" {
			t.Errorf("timeField = %q, want %q", timeField, "timestamp")
		}

		metaField := spec.Options.Lookup("timeseries", "metaField").StringValue()
		if metaField != "source" {
			t.Errorf("metaField = %q, want %q", metaField, "source")
		}
	}

	if !found {
		t.Errorf("collection %s was not created", types.CollectionName)
	}
}

func TestConnect_IsIdempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := ensureTimeSeriesCollections(ctx, store.db); err != nil {
		t.Fatalf("second ensureTimeSeriesCollections call returned error: %v", err)
	}
}

func TestInsertLogs_InsertsAllToSingleCollection(t *testing.T) {
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
		{Entry: makeEntry("trace-1", types.LogTypeRequest)},
		{Entry: makeEntry("trace-2", types.LogTypeResponse)},
		{Entry: makeEntry("trace-3", types.LogTypeEvent)},
	}

	if err := store.InsertLogs(ctx, logs); err != nil {
		t.Fatalf("InsertLogs: %v", err)
	}

	count, err := store.db.Collection(types.CollectionName).CountDocuments(ctx, bson.D{})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

func TestFindLogs_FiltersByType(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	source := types.LogSource{AppName: "order-service", ServiceName: "order"}
	logs := []types.BufferedLog{
		{Entry: types.LogEntry{Timestamp: time.Now(), Source: source, TraceID: "t-1", Metadata: map[string]interface{}{}, Endpoint: "/orders", HTTPStatus: "200", RawPayload: map[string]interface{}{}, Payload: map[string]interface{}{}, Type: types.LogTypeRequest, Direction: types.LogDirectionInbound}},
		{Entry: types.LogEntry{Timestamp: time.Now(), Source: source, TraceID: "t-2", Metadata: map[string]interface{}{}, Endpoint: "/orders", HTTPStatus: "200", RawPayload: map[string]interface{}{}, Payload: map[string]interface{}{}, Type: types.LogTypeResponse, Direction: types.LogDirectionInbound}},
		{Entry: types.LogEntry{Timestamp: time.Now(), Source: source, TraceID: "t-3", Metadata: map[string]interface{}{}, Endpoint: "order.placed", HTTPStatus: "200", RawPayload: map[string]interface{}{}, Payload: map[string]interface{}{}, Type: types.LogTypeEvent, Direction: types.LogDirectionInbound}},
	}

	if err := store.InsertLogs(ctx, logs); err != nil {
		t.Fatalf("InsertLogs: %v", err)
	}

	entries, err := store.FindLogs(ctx, logstore.FindLogsFilter{Type: types.LogTypeEvent})
	if err != nil {
		t.Fatalf("FindLogs: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1", len(entries))
	}
	if entries[0].TraceID != "t-3" {
		t.Errorf("trace_id = %q, want %q", entries[0].TraceID, "t-3")
	}
}

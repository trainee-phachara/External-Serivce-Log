package mongostore

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"external-service-log/internal/types"
)

// ttlSeconds is how long documents remain in each time-series collection
// before MongoDB automatically expires them (30 days).
const ttlSeconds = 60 * 60 * 24 * 30

var collectionNames = []types.CollectionName{
	types.CollectionAPILogs,
	types.CollectionEventLogs,
	types.CollectionErrorLogs,
}

// Store persists buffered logs into MongoDB time-series collections.
type Store struct {
	client *mongo.Client
	db     *mongo.Database
}

// Connect opens a MongoDB connection, selects dbName, and ensures the
// api_logs, event_logs, and error_logs time-series collections exist.
func Connect(ctx context.Context, uri, dbName string) (*Store, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("connect to mongo: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("ping mongo: %w", err)
	}

	db := client.Database(dbName)
	if err := ensureTimeSeriesCollections(ctx, db); err != nil {
		return nil, err
	}

	return &Store{client: client, db: db}, nil
}

func ensureTimeSeriesCollections(ctx context.Context, db *mongo.Database) error {
	existing, err := db.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}

	existingNames := make(map[string]bool, len(existing))
	for _, name := range existing {
		existingNames[name] = true
	}

	for _, name := range collectionNames {
		if existingNames[string(name)] {
			continue
		}

		timeSeriesOpts := options.TimeSeries().
			SetTimeField("timestamp").
			SetMetaField("source").
			SetGranularity("seconds")
		opts := options.CreateCollection().
			SetTimeSeriesOptions(timeSeriesOpts).
			SetExpireAfterSeconds(ttlSeconds)

		if err := db.CreateCollection(ctx, string(name), opts); err != nil {
			return fmt.Errorf("create collection %s: %w", name, err)
		}
	}

	return nil
}

// InsertLogs groups logs by their destination collection and inserts each
// group with a single InsertMany call.
func (s *Store) InsertLogs(ctx context.Context, logs []types.BufferedLog) error {
	grouped := make(map[types.CollectionName][]types.LogEntry)
	for _, log := range logs {
		grouped[log.Collection] = append(grouped[log.Collection], log.Entry)
	}

	for collectionName, entries := range grouped {
		if _, err := s.db.Collection(string(collectionName)).InsertMany(ctx, entries); err != nil {
			return fmt.Errorf("insert into %s: %w", collectionName, err)
		}
	}

	return nil
}

// Close disconnects the underlying MongoDB client.
func (s *Store) Close(ctx context.Context) error {
	return s.client.Disconnect(ctx)
}

package logclient

import (
	"context"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "order-service/internal/grpc/pb"
)

// LogSource identifies the application/service emitting a log entry.
type LogSource struct {
	AppName     string
	ServiceName string
}

// LogEntryInput is the data sent to external-service-log's IngestService.
type LogEntryInput struct {
	Source         LogSource
	TraceID        string
	Endpoint       string
	HTTPStatus     string
	Type           string
	Direction      string
	MetadataJSON   string
	RawPayloadJSON string
	PayloadJSON    string
}

// LogClient sends log entries to external-service-log.
type LogClient interface {
	// SendLog fires the log entry without waiting for the response.
	SendLog(entry LogEntryInput)
	Close() error
}

type grpcLogClient struct {
	conn   *grpc.ClientConn
	client pb.IngestServiceClient
}

// New connects to the IngestService at address.
func New(address string) (LogClient, error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &grpcLogClient{conn: conn, client: pb.NewIngestServiceClient(conn)}, nil
}

func (c *grpcLogClient) SendLog(entry LogEntryInput) {
	go func() {
		_, err := c.client.Ingest(context.Background(), &pb.IngestRequest{
			Source: &pb.LogSource{
				AppName:     entry.Source.AppName,
				ServiceName: entry.Source.ServiceName,
			},
			TraceId:        entry.TraceID,
			Endpoint:       entry.Endpoint,
			HttpStatus:     entry.HTTPStatus,
			Type:           entry.Type,
			Direction:      entry.Direction,
			MetadataJson:   entry.MetadataJSON,
			RawPayloadJson: entry.RawPayloadJSON,
			PayloadJson:    entry.PayloadJSON,
		})
		if err != nil {
			log.Printf("Failed to send log to external-service-log: %v", err)
		}
	}()
}

func (c *grpcLogClient) Close() error {
	return c.conn.Close()
}

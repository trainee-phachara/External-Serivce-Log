package logclient

import (
	"context"
	"log"
	"net"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "user-service/internal/grpc/pb"
)

type fakeIngestServer struct {
	pb.UnimplementedIngestServiceServer
	handle func(context.Context, *pb.IngestRequest) (*pb.IngestResponse, error)
}

func (s *fakeIngestServer) Ingest(ctx context.Context, req *pb.IngestRequest) (*pb.IngestResponse, error) {
	return s.handle(ctx, req)
}

func startTestServer(t *testing.T, fake *fakeIngestServer) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	server := grpc.NewServer()
	pb.RegisterIngestServiceServer(server, fake)

	go func() {
		_ = server.Serve(lis)
	}()
	t.Cleanup(server.Stop)

	return lis.Addr().String()
}

func validEntry(modify func(*LogEntryInput)) LogEntryInput {
	entry := LogEntryInput{
		Source:         LogSource{AppName: "user-service", ServiceName: "user"},
		TraceID:        "trace-1",
		Endpoint:       "/users",
		HTTPStatus:     "200",
		Type:           "response",
		Direction:      "inbound",
		MetadataJSON:   `{"method":"GET"}`,
		RawPayloadJSON: "{}",
		PayloadJSON:    `{"id":1}`,
	}
	if modify != nil {
		modify(&entry)
	}
	return entry
}

func TestSendLog_SendsEntryFieldsToIngestService(t *testing.T) {
	received := make(chan *pb.IngestRequest, 1)
	fake := &fakeIngestServer{
		handle: func(_ context.Context, req *pb.IngestRequest) (*pb.IngestResponse, error) {
			received <- req
			return &pb.IngestResponse{Accepted: true, Errors: []string{}}, nil
		},
	}
	address := startTestServer(t, fake)

	client, err := New(address)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	client.SendLog(validEntry(func(e *LogEntryInput) { e.TraceID = "trace-abc" }))

	req := <-received
	if req.TraceId != "trace-abc" {
		t.Errorf("TraceId = %q, want %q", req.TraceId, "trace-abc")
	}
	if req.Endpoint != "/users" {
		t.Errorf("Endpoint = %q, want %q", req.Endpoint, "/users")
	}
	if req.HttpStatus != "200" {
		t.Errorf("HttpStatus = %q, want %q", req.HttpStatus, "200")
	}
	if req.Source.GetAppName() != "user-service" || req.Source.GetServiceName() != "order" {
		t.Errorf("Source = %+v, want {order-service order}", req.Source)
	}
}

func TestSendLog_LogsErrorWhenServerCallFails(t *testing.T) {
	fake := &fakeIngestServer{
		handle: func(_ context.Context, _ *pb.IngestRequest) (*pb.IngestResponse, error) {
			return nil, status.Error(codes.Internal, "boom")
		},
	}
	address := startTestServer(t, fake)

	client, err := New(address)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	var buf strings.Builder
	var mu sync.Mutex
	done := make(chan struct{})

	originalOutput := log.Writer()
	log.SetOutput(writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		n, err := buf.Write(p)
		select {
		case <-done:
		default:
			close(done)
		}
		return n, err
	}))
	t.Cleanup(func() { log.SetOutput(originalOutput) })

	client.SendLog(validEntry(nil))

	<-done

	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(buf.String(), "Failed to send log to external-service-log") {
		t.Errorf("log output = %q, want to contain failure message", buf.String())
	}
	if !strings.Contains(buf.String(), "boom") {
		t.Errorf("log output = %q, want to contain %q", buf.String(), "boom")
	}
}

func TestClose_DoesNotError(t *testing.T) {
	fake := &fakeIngestServer{
		handle: func(_ context.Context, _ *pb.IngestRequest) (*pb.IngestResponse, error) {
			return &pb.IngestResponse{Accepted: true, Errors: []string{}}, nil
		},
	}
	address := startTestServer(t, fake)

	client, err := New(address)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := client.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

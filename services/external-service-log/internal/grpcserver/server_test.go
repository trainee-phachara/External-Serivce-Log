package grpcserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"external-service-log/internal/buffer"
	"external-service-log/internal/flusher"
	pb "external-service-log/internal/grpc/pb"
	"external-service-log/internal/types"
)

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	return string(b)
}

func validRequest(t *testing.T, modify func(*pb.IngestRequest)) *pb.IngestRequest {
	t.Helper()
	req := &pb.IngestRequest{
		Source:         &pb.LogSource{AppName: "order-service", ServiceName: "order"},
		TraceId:        "trace-123",
		Endpoint:       "/api/v1/orders",
		HttpStatus:     "200",
		Type:           "request",
		Direction:      "inbound",
		MetadataJson:   mustJSON(t, map[string]interface{}{"user_id": "u-1"}),
		RawPayloadJson: mustJSON(t, map[string]interface{}{"raw": true}),
		PayloadJson:    mustJSON(t, map[string]interface{}{"id": 1}),
	}
	if modify != nil {
		modify(req)
	}
	return req
}

func newTestServer() (*Server, *buffer.LogBuffer) {
	buf := buffer.New()
	insert := func(_ context.Context, _ []types.BufferedLog) error { return nil }
	fl := flusher.New(buf, insert, flusher.Options{MaxSize: 100, Interval: 5 * time.Second})
	return New(buf, fl), buf
}

func TestIngest_AcceptsValidRequestAndPushesToBuffer(t *testing.T) {
	s, buf := newTestServer()

	resp, err := s.Ingest(context.Background(), validRequest(t, func(r *pb.IngestRequest) {
		r.TraceId = "trace-abc"
	}))

	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	if !resp.Accepted {
		t.Fatalf("Accepted = false, errors = %v", resp.Errors)
	}
	if len(resp.Errors) != 0 {
		t.Errorf("Errors = %v, want empty", resp.Errors)
	}

	if got := buf.Size(); got != 1 {
		t.Fatalf("buffer size = %d, want 1", got)
	}
	entry := buf.Drain()[0]
	if entry.Collection != types.CollectionAPILogs {
		t.Errorf("collection = %q, want %q", entry.Collection, types.CollectionAPILogs)
	}
	if entry.Entry.TraceID != "trace-abc" {
		t.Errorf("trace_id = %q, want %q", entry.Entry.TraceID, "trace-abc")
	}
	if got, want := entry.Entry.Metadata["user_id"], "u-1"; got != want {
		t.Errorf("metadata.user_id = %v, want %v", got, want)
	}
}

func TestIngest_RoutesErrorStatusToErrorLogs(t *testing.T) {
	s, buf := newTestServer()

	resp, err := s.Ingest(context.Background(), validRequest(t, func(r *pb.IngestRequest) {
		r.HttpStatus = "500"
		r.Type = "response"
	}))

	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	if !resp.Accepted {
		t.Fatalf("Accepted = false, errors = %v", resp.Errors)
	}

	entry := buf.Drain()[0]
	if entry.Collection != types.CollectionErrorLogs {
		t.Errorf("collection = %q, want %q", entry.Collection, types.CollectionErrorLogs)
	}
}

func TestIngest_RejectsRequestMissingRequiredFields(t *testing.T) {
	s, buf := newTestServer()

	resp, err := s.Ingest(context.Background(), &pb.IngestRequest{TraceId: "trace-1"})

	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	if resp.Accepted {
		t.Error("Accepted = true, want false")
	}
	if len(resp.Errors) == 0 {
		t.Error("Errors is empty, want at least one error")
	}
	if got := buf.Size(); got != 0 {
		t.Errorf("buffer size = %d, want 0", got)
	}
}

func TestIngest_RejectsMalformedJSONInOptionalField(t *testing.T) {
	s, buf := newTestServer()

	resp, err := s.Ingest(context.Background(), validRequest(t, func(r *pb.IngestRequest) {
		r.MetadataJson = "{not-json"
	}))

	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	if resp.Accepted {
		t.Error("Accepted = true, want false")
	}
	if !containsErrorSubstring(resp.Errors, "metadata_json must be valid JSON") {
		t.Errorf("Errors = %v, want one containing %q", resp.Errors, "metadata_json must be valid JSON")
	}
	if got := buf.Size(); got != 0 {
		t.Errorf("buffer size = %d, want 0", got)
	}
}

func TestIngest_RejectsNonObjectJSONField(t *testing.T) {
	s, _ := newTestServer()

	resp, err := s.Ingest(context.Background(), validRequest(t, func(r *pb.IngestRequest) {
		r.PayloadJson = "[1, 2, 3]"
	}))

	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	if resp.Accepted {
		t.Error("Accepted = true, want false")
	}
	if !containsErrorSubstring(resp.Errors, "payload_json must be a JSON object") {
		t.Errorf("Errors = %v, want one containing %q", resp.Errors, "payload_json must be a JSON object")
	}
}

func containsErrorSubstring(errs []string, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

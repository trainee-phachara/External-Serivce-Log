package grpcserver

import (
	"context"
	"encoding/json"

	"external-service-log/internal/buffer"
	"external-service-log/internal/flusher"
	pb "external-service-log/internal/grpc/pb"
	"external-service-log/internal/ingest"
)

// Server implements pb.IngestServiceServer, delegating validation and
// buffering to the shared ingest.ProcessIngest used by the HTTP API.
type Server struct {
	pb.UnimplementedIngestServiceServer

	buf *buffer.LogBuffer
	fl  *flusher.BatchFlusher
}

// New returns a Server backed by buf and fl.
func New(buf *buffer.LogBuffer, fl *flusher.BatchFlusher) *Server {
	return &Server{buf: buf, fl: fl}
}

// Ingest validates req, and if valid, classifies and buffers it. It always
// returns a normal response (never a gRPC error status), reporting any
// validation problems via IngestResponse.errors.
func (s *Server) Ingest(_ context.Context, req *pb.IngestRequest) (*pb.IngestResponse, error) {
	body, errs := toRequestMap(req)
	if body == nil {
		return &pb.IngestResponse{Accepted: false, Errors: errs}, nil
	}

	result := ingest.ProcessIngest(body, s.buf, s.fl)
	return &pb.IngestResponse{Accepted: result.Accepted, Errors: result.Errors}, nil
}

// toRequestMap converts a gRPC IngestRequest into the map[string]interface{}
// shape expected by ingest.ValidateIngestBody, decoding the JSON-encoded
// metadata/raw_payload/payload fields along the way.
func toRequestMap(req *pb.IngestRequest) (map[string]interface{}, []string) {
	errs := make([]string, 0)

	body := map[string]interface{}{
		"source": map[string]interface{}{
			"app_name":     req.GetSource().GetAppName(),
			"service_name": req.GetSource().GetServiceName(),
		},
		"trace_id":    req.GetTraceId(),
		"endpoint":    req.GetEndpoint(),
		"http_status": req.GetHttpStatus(),
		"type":        req.GetType(),
		"direction":   req.GetDirection(),
	}

	if metadata, ok := parseOptionalJSONObject(req.GetMetadataJson(), "metadata_json", &errs); ok && metadata != nil {
		body["metadata"] = metadata
	}
	if rawPayload, ok := parseOptionalJSONObject(req.GetRawPayloadJson(), "raw_payload_json", &errs); ok && rawPayload != nil {
		body["raw_payload"] = rawPayload
	}
	if payload, ok := parseOptionalJSONObject(req.GetPayloadJson(), "payload_json", &errs); ok && payload != nil {
		body["payload"] = payload
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return body, nil
}

// parseOptionalJSONObject decodes value as a JSON object. An empty value is
// treated as "not provided" (ok=true, object=nil). Malformed JSON or a JSON
// value that isn't an object appends a descriptive error to errs and returns
// ok=false.
func parseOptionalJSONObject(value, fieldName string, errs *[]string) (map[string]interface{}, bool) {
	if value == "" {
		return nil, true
	}

	var parsed interface{}
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		*errs = append(*errs, fieldName+" must be valid JSON")
		return nil, false
	}

	obj, ok := parsed.(map[string]interface{})
	if !ok {
		*errs = append(*errs, fieldName+" must be a JSON object")
		return nil, false
	}
	return obj, true
}

package ingest

import (
	"time"

	"external-service-log/internal/buffer"
	"external-service-log/internal/flusher"
	"external-service-log/internal/types"
)

// ProcessResult is the outcome of processing an ingest request, regardless
// of whether it arrived via HTTP or gRPC.
type ProcessResult struct {
	Accepted bool
	Errors   []string
}

// ProcessIngest validates rawBody, and if valid, builds a LogEntry, classifies
// its destination collection, pushes it onto buf, and notifies fl so it can
// trigger a size-based flush. This is the transport-agnostic core shared by
// the HTTP and gRPC ingest entry points.
func ProcessIngest(rawBody interface{}, buf *buffer.LogBuffer, fl *flusher.BatchFlusher) ProcessResult {
	validation := ValidateIngestBody(rawBody)
	if !validation.Valid {
		return ProcessResult{Accepted: false, Errors: validation.Errors}
	}

	body := toIngestRequestBody(rawBody.(map[string]interface{}))

	entry := types.LogEntry{
		Timestamp:  time.Now(),
		Source:     body.Source,
		TraceID:    body.TraceID,
		Metadata:   defaultObject(body.Metadata),
		Endpoint:   body.Endpoint,
		HTTPStatus: body.HTTPStatus,
		RawPayload: defaultObject(body.RawPayload),
		Payload:    defaultObject(body.Payload),
		Type:       body.Type,
		Direction:  body.Direction,
	}

	buf.Push(types.BufferedLog{Entry: entry})
	fl.OnLogPushed()

	return ProcessResult{Accepted: true, Errors: []string{}}
}

func defaultObject(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	return m
}

// toIngestRequestBody converts a validated raw body map into an
// IngestRequestBody. It assumes candidate has already passed
// ValidateIngestBody, so required fields are guaranteed present with the
// expected types.
func toIngestRequestBody(candidate map[string]interface{}) types.IngestRequestBody {
	source, _ := candidate["source"].(map[string]interface{})
	metadata, _ := candidate["metadata"].(map[string]interface{})
	rawPayload, _ := candidate["raw_payload"].(map[string]interface{})
	payload, _ := candidate["payload"].(map[string]interface{})

	return types.IngestRequestBody{
		Source: types.LogSource{
			AppName:     source["app_name"].(string),
			ServiceName: source["service_name"].(string),
		},
		TraceID:    candidate["trace_id"].(string),
		Metadata:   metadata,
		Endpoint:   candidate["endpoint"].(string),
		HTTPStatus: candidate["http_status"].(string),
		RawPayload: rawPayload,
		Payload:    payload,
		Type:       types.LogType(candidate["type"].(string)),
		Direction:  types.LogDirection(candidate["direction"].(string)),
	}
}

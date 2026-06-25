package types

import "time"

// LogType is the kind of HTTP exchange a log entry represents.
type LogType string

const (
	LogTypeRequest  LogType = "request"
	LogTypeResponse LogType = "response"
	LogTypeEvent    LogType = "event"
)

// LogDirection is the direction of a logged exchange relative to the
// reporting service.
type LogDirection string

const (
	LogDirectionInbound  LogDirection = "inbound"
	LogDirectionOutbound LogDirection = "outbound"
)

// CollectionName is the name of one of the MongoDB time-series collections
// logs are routed into.
type CollectionName string

const (
	CollectionAPILogs   CollectionName = "api_logs"
	CollectionEventLogs CollectionName = "event_logs"
	CollectionErrorLogs CollectionName = "error_logs"
)

// LogSource identifies the application and service that produced a log entry.
type LogSource struct {
	AppName     string `bson:"app_name" json:"app_name"`
	ServiceName string `bson:"service_name" json:"service_name"`
}

// LogEntry is the document shape stored in MongoDB.
type LogEntry struct {
	Timestamp  time.Time              `bson:"timestamp" json:"timestamp"`
	Source     LogSource              `bson:"source" json:"source"`
	TraceID    string                 `bson:"trace_id" json:"trace_id"`
	Metadata   map[string]interface{} `bson:"metadata" json:"metadata"`
	Endpoint   string                 `bson:"endpoint" json:"endpoint"`
	HTTPStatus string                 `bson:"http_status" json:"http_status"`
	RawPayload map[string]interface{} `bson:"raw_payload" json:"raw_payload"`
	Payload    map[string]interface{} `bson:"payload" json:"payload"`
	Type       LogType                `bson:"type" json:"type"`
	Direction  LogDirection           `bson:"direction" json:"direction"`
}

// IngestRequestBody is the shape of the body accepted by POST /ingest and the
// gRPC Ingest RPC, before defaults are applied.
type IngestRequestBody struct {
	Source     LogSource              `json:"source"`
	TraceID    string                 `json:"trace_id"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Endpoint   string                 `json:"endpoint"`
	HTTPStatus string                 `json:"http_status"`
	RawPayload map[string]interface{} `json:"raw_payload,omitempty"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
	Type       LogType                `json:"type"`
	Direction  LogDirection           `json:"direction"`
}

// BufferedLog pairs a LogEntry with the collection it should be inserted into.
type BufferedLog struct {
	Collection CollectionName
	Entry      LogEntry
}

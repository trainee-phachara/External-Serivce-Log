package ingest

import (
	"strconv"

	"external-service-log/internal/types"
)

// ClassifyCollection decides which MongoDB collection a log entry should be
// routed to: any response with an HTTP status >= 400 is an error log,
// request/response exchanges below that go to api_logs, and everything else
// is an event log.
func ClassifyCollection(body types.IngestRequestBody) types.CollectionName {
	if statusCode, err := strconv.Atoi(body.HTTPStatus); err == nil && statusCode >= 400 {
		return types.CollectionErrorLogs
	}
	if body.Type == types.LogTypeRequest || body.Type == types.LogTypeResponse {
		return types.CollectionAPILogs
	}
	return types.CollectionEventLogs
}

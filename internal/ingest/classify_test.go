package ingest

import (
	"testing"

	"external-service-log/internal/types"
)

func makeBody(modify func(*types.IngestRequestBody)) types.IngestRequestBody {
	body := types.IngestRequestBody{
		Source:     types.LogSource{AppName: "order-service", ServiceName: "order"},
		TraceID:    "trace-1",
		Endpoint:   "/api/v1/orders",
		HTTPStatus: "200",
		Type:       types.LogTypeRequest,
		Direction:  types.LogDirectionInbound,
	}
	if modify != nil {
		modify(&body)
	}
	return body
}

func TestClassifyCollection_ErrorStatusOverridesType(t *testing.T) {
	got := ClassifyCollection(makeBody(func(b *types.IngestRequestBody) {
		b.HTTPStatus = "404"
		b.Type = types.LogTypeResponse
	}))
	if got != types.CollectionErrorLogs {
		t.Errorf("got %q, want %q", got, types.CollectionErrorLogs)
	}

	got = ClassifyCollection(makeBody(func(b *types.IngestRequestBody) {
		b.HTTPStatus = "500"
		b.Type = types.LogTypeRequest
	}))
	if got != types.CollectionErrorLogs {
		t.Errorf("got %q, want %q", got, types.CollectionErrorLogs)
	}
}

func TestClassifyCollection_RequestResponseBelow400(t *testing.T) {
	got := ClassifyCollection(makeBody(func(b *types.IngestRequestBody) {
		b.HTTPStatus = "200"
		b.Type = types.LogTypeRequest
	}))
	if got != types.CollectionAPILogs {
		t.Errorf("got %q, want %q", got, types.CollectionAPILogs)
	}

	got = ClassifyCollection(makeBody(func(b *types.IngestRequestBody) {
		b.HTTPStatus = "201"
		b.Type = types.LogTypeResponse
	}))
	if got != types.CollectionAPILogs {
		t.Errorf("got %q, want %q", got, types.CollectionAPILogs)
	}
}

func TestClassifyCollection_EverythingElseIsEventLog(t *testing.T) {
	got := ClassifyCollection(makeBody(func(b *types.IngestRequestBody) {
		b.HTTPStatus = "200"
		b.Type = types.LogType("event")
	}))
	if got != types.CollectionEventLogs {
		t.Errorf("got %q, want %q", got, types.CollectionEventLogs)
	}
}

func TestClassifyCollection_NonNumericStatusBelowThreshold(t *testing.T) {
	got := ClassifyCollection(makeBody(func(b *types.IngestRequestBody) {
		b.HTTPStatus = "n/a"
		b.Type = types.LogTypeRequest
	}))
	if got != types.CollectionAPILogs {
		t.Errorf("got %q, want %q", got, types.CollectionAPILogs)
	}
}

package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	logclient "github.com/trainee-phachara/External-Serivce-Log/client"
)

const (
	sourceAppName     = "user-service"
	sourceServiceName = "user"
)

// responseRecorder captures the status code and body written by the wrapped handler.
type responseRecorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func sendEventLog(logClient logclient.LogClient, eventName string, payloadJSON string) {
	logClient.SendLog(logclient.LogEntryInput{
		Source:      logclient.LogSource{AppName: sourceAppName, ServiceName: sourceServiceName},
		TraceID:     uuid.NewString(),
		Endpoint:    eventName,
		HTTPStatus:  "200",
		Type:        "event",
		Direction:   "inbound",
		PayloadJSON: payloadJSON,
	})
}

// withRequestLogger wraps next with middleware that reports every request to
// logClient once the response has finished.
func withRequestLogger(logClient logclient.LogClient, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawBody, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(rawBody))

		rec := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}

		rawPayloadJSON := "{}"
		if len(rawBody) > 0 {
			rawPayloadJSON = string(rawBody)
		}

		payloadJSON := rec.body.String()
		if payloadJSON == "" {
			payloadJSON = "{}"
		}

		metadataJSON, _ := json.Marshal(map[string]string{"method": r.Method})

		logClient.SendLog(logclient.LogEntryInput{
			Source:         logclient.LogSource{AppName: sourceAppName, ServiceName: sourceServiceName},
			TraceID:        uuid.NewString(),
			Endpoint:       r.URL.Path,
			HTTPStatus:     strconv.Itoa(status),
			Type:           "response",
			Direction:      "inbound",
			MetadataJSON:   string(metadataJSON),
			RawPayloadJSON: rawPayloadJSON,
			PayloadJSON:    payloadJSON,
		})
	})
}

package httpapi

import (
	"encoding/json"
	"net/http"

	"external-service-log/internal/buffer"
	"external-service-log/internal/flusher"
	"external-service-log/internal/ingest"
)

// NewHandler returns the HTTP handler exposing POST /ingest.
func NewHandler(buf *buffer.LogBuffer, fl *flusher.BatchFlusher) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /ingest", func(w http.ResponseWriter, r *http.Request) {
		var body interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"errors": []string{"Request body must be valid JSON"},
			})
			return
		}

		result := ingest.ProcessIngest(body, buf, fl)
		if !result.Accepted {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": result.Errors})
			return
		}

		writeJSON(w, http.StatusAccepted, map[string]interface{}{"status": "accepted"})
	})

	return mux
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"external-service-log/internal/buffer"
	"external-service-log/internal/flusher"
	"external-service-log/internal/ingest"
	"external-service-log/internal/logstore"
	"external-service-log/internal/types"
)

// NewHandler returns the HTTP handler exposing POST /ingest and GET /logs.
func NewHandler(buf *buffer.LogBuffer, fl *flusher.BatchFlusher, store logstore.LogStore) http.Handler {
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

	mux.HandleFunc("GET /logs", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		logType := types.LogType(q.Get("type"))
		appName := q.Get("app")
		limit, _ := strconv.ParseInt(q.Get("limit"), 10, 64)

		entries, err := store.FindLogs(r.Context(), logstore.FindLogsFilter{
			Type:    logType,
			AppName: appName,
			Limit:   limit,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, entries)
	})

	return withCORS(mux)
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

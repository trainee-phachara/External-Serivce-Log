package httpapi

import (
	"encoding/json"
	"net/http"

	"order-service/internal/logclient"
	"order-service/internal/orders"
)

// NewHandler returns the HTTP handler exposing the order CRUD endpoints,
// wrapped with request logging to logClient.
func NewHandler(repo *orders.Repository, logClient logclient.LogClient) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /orders", createOrder(repo, logClient))
	mux.HandleFunc("GET /orders", listOrders(repo))
	mux.HandleFunc("GET /orders/{id}", getOrder(repo))
	mux.HandleFunc("PUT /orders/{id}", updateOrder(repo, logClient))
	mux.HandleFunc("DELETE /orders/{id}", deleteOrder(repo, logClient))

	return withCORS(withRequestLogger(logClient, mux))
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

// decodeJSONBody decodes the request body as JSON, defaulting to an empty
// object when the body is missing or malformed.
func decodeJSONBody(r *http.Request) interface{} {
	var body interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return map[string]interface{}{}
	}
	return body
}

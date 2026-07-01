package httpapi

import (
	"encoding/json"
	"net/http"

	logclient "github.com/trainee-phachara/External-Serivce-Log/client"
	"payment-service/internal/payments"
)

// NewHandler returns the HTTP handler exposing the payment CRUD endpoints
// plus the GET /payments/order/{order_id} query endpoint,
// wrapped with request logging to logClient.
func NewHandler(repo *payments.Repository, logClient logclient.LogClient) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /payments", createPayment(repo, logClient))
	mux.HandleFunc("GET /payments", listPayments(repo))
	mux.HandleFunc("GET /payments/order/{order_id}", listPaymentsByOrder(repo))
	mux.HandleFunc("GET /payments/{id}", getPayment(repo))
	mux.HandleFunc("PUT /payments/{id}", updatePayment(repo, logClient))
	mux.HandleFunc("DELETE /payments/{id}", deletePayment(repo, logClient))

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

func decodeJSONBody(r *http.Request) interface{} {
	var body interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return map[string]interface{}{}
	}
	return body
}

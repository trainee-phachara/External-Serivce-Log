package httpapi

import (
	"encoding/json"
	"net/http"

	logclient "github.com/trainee-phachara/External-Serivce-Log/client"
	"user-service/internal/users"
)

// NewHandler returns the HTTP handler exposing the user CRUD endpoints,
// wrapped with request logging to logClient.
func NewHandler(repo *users.Repository, logClient logclient.LogClient) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /users", createUser(repo, logClient))
	mux.HandleFunc("GET /users", listUsers(repo))
	mux.HandleFunc("GET /users/{id}", getUser(repo))
	mux.HandleFunc("PUT /users/{id}", updateUser(repo, logClient))
	mux.HandleFunc("DELETE /users/{id}", deleteUser(repo, logClient))

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

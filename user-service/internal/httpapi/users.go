package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	logclient "github.com/trainee-phachara/External-Serivce-Log/client"
	"user-service/internal/users"
)

func createUser(repo *users.Repository, logClient logclient.LogClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := decodeJSONBody(r)

		result := users.ValidateCreateUser(body)
		if !result.Valid {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": result.Errors})
			return
		}

		candidate, _ := body.(map[string]interface{})
		user, err := repo.Create(r.Context(), users.ToCreateUserInput(candidate))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"errors": []string{"internal server error"}})
			return
		}

		if p, err := json.Marshal(user); err == nil {
			sendEventLog(logClient, "user.created", string(p))
		}
		writeJSON(w, http.StatusCreated, user)
	}
}

func listUsers(repo *users.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all, err := repo.FindAll(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"errors": []string{"internal server error"}})
			return
		}

		writeJSON(w, http.StatusOK, all)
	}
}

func getUser(repo *users.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": []string{"id must be an integer"}})
			return
		}

		user, err := repo.FindByID(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"errors": []string{"internal server error"}})
			return
		}
		if user == nil {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"errors": []string{"user not found"}})
			return
		}

		writeJSON(w, http.StatusOK, user)
	}
}

func updateUser(repo *users.Repository, logClient logclient.LogClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": []string{"id must be an integer"}})
			return
		}

		body := decodeJSONBody(r)
		result := users.ValidateUpdateUser(body)
		if !result.Valid {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": result.Errors})
			return
		}

		candidate, _ := body.(map[string]interface{})
		user, err := repo.Update(r.Context(), id, users.ToUpdateUserInput(candidate))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"errors": []string{"internal server error"}})
			return
		}
		if user == nil {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"errors": []string{"user not found"}})
			return
		}

		if p, err := json.Marshal(user); err == nil {
			sendEventLog(logClient, "user.updated", string(p))
		}
		writeJSON(w, http.StatusOK, user)
	}
}

func deleteUser(repo *users.Repository, logClient logclient.LogClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": []string{"id must be an integer"}})
			return
		}

		removed, err := repo.Remove(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"errors": []string{"internal server error"}})
			return
		}
		if !removed {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"errors": []string{"user not found"}})
			return
		}

		sendEventLog(logClient, "user.deleted", `{"id":`+strconv.Itoa(id)+`}`)
		w.WriteHeader(http.StatusNoContent)
	}
}

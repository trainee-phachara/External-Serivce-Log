package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	logclient "github.com/trainee-phachara/External-Serivce-Log/client"
	"order-service/internal/orders"
)

func createOrder(repo *orders.Repository, logClient logclient.LogClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := decodeJSONBody(r)

		result := orders.ValidateCreateOrder(body)
		if !result.Valid {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": result.Errors})
			return
		}

		candidate, _ := body.(map[string]interface{})
		order, err := repo.Create(r.Context(), orders.ToCreateOrderInput(candidate))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"errors": []string{"internal server error"}})
			return
		}

		if p, err := json.Marshal(order); err == nil {
			sendEventLog(logClient, "order.created", string(p))
		}
		writeJSON(w, http.StatusCreated, order)
	}
}

func listOrders(repo *orders.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all, err := repo.FindAll(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"errors": []string{"internal server error"}})
			return
		}

		writeJSON(w, http.StatusOK, all)
	}
}

func getOrder(repo *orders.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": []string{"id must be an integer"}})
			return
		}

		order, err := repo.FindByID(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"errors": []string{"internal server error"}})
			return
		}
		if order == nil {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"errors": []string{"order not found"}})
			return
		}

		writeJSON(w, http.StatusOK, order)
	}
}

func updateOrder(repo *orders.Repository, logClient logclient.LogClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": []string{"id must be an integer"}})
			return
		}

		body := decodeJSONBody(r)
		result := orders.ValidateUpdateOrder(body)
		if !result.Valid {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": result.Errors})
			return
		}

		candidate, _ := body.(map[string]interface{})
		order, err := repo.Update(r.Context(), id, orders.ToUpdateOrderInput(candidate))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"errors": []string{"internal server error"}})
			return
		}
		if order == nil {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"errors": []string{"order not found"}})
			return
		}

		if p, err := json.Marshal(order); err == nil {
			sendEventLog(logClient, "order.updated", string(p))
		}
		writeJSON(w, http.StatusOK, order)
	}
}

func deleteOrder(repo *orders.Repository, logClient logclient.LogClient) http.HandlerFunc {
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
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"errors": []string{"order not found"}})
			return
		}

		sendEventLog(logClient, "order.deleted", `{"id":`+strconv.Itoa(id)+`}`)
		w.WriteHeader(http.StatusNoContent)
	}
}

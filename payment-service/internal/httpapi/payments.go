package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	logclient "github.com/trainee-phachara/External-Serivce-Log/client"
	"payment-service/internal/payments"
)

func createPayment(repo *payments.Repository, logClient logclient.LogClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := decodeJSONBody(r)

		result := payments.ValidateCreatePayment(body)
		if !result.Valid {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": result.Errors})
			return
		}

		candidate, _ := body.(map[string]interface{})
		payment, err := repo.Create(r.Context(), payments.ToCreatePaymentInput(candidate))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"errors": []string{"internal server error"}})
			return
		}

		if p, err := json.Marshal(payment); err == nil {
			sendEventLog(logClient, "payment.created", string(p))
		}
		writeJSON(w, http.StatusCreated, payment)
	}
}

func listPayments(repo *payments.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all, err := repo.FindAll(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"errors": []string{"internal server error"}})
			return
		}
		writeJSON(w, http.StatusOK, all)
	}
}

func listPaymentsByOrder(repo *payments.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orderID, err := strconv.Atoi(r.PathValue("order_id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": []string{"order_id must be an integer"}})
			return
		}

		result, err := repo.FindByOrderID(r.Context(), orderID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"errors": []string{"internal server error"}})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func getPayment(repo *payments.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": []string{"id must be an integer"}})
			return
		}

		payment, err := repo.FindByID(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"errors": []string{"internal server error"}})
			return
		}
		if payment == nil {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"errors": []string{"payment not found"}})
			return
		}

		writeJSON(w, http.StatusOK, payment)
	}
}

func updatePayment(repo *payments.Repository, logClient logclient.LogClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": []string{"id must be an integer"}})
			return
		}

		body := decodeJSONBody(r)
		result := payments.ValidateUpdatePayment(body)
		if !result.Valid {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": result.Errors})
			return
		}

		candidate, _ := body.(map[string]interface{})
		payment, err := repo.Update(r.Context(), id, payments.ToUpdatePaymentInput(candidate))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"errors": []string{"internal server error"}})
			return
		}
		if payment == nil {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"errors": []string{"payment not found"}})
			return
		}

		if p, err := json.Marshal(payment); err == nil {
			sendEventLog(logClient, "payment.updated", string(p))
		}
		writeJSON(w, http.StatusOK, payment)
	}
}

func deletePayment(repo *payments.Repository, logClient logclient.LogClient) http.HandlerFunc {
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
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"errors": []string{"payment not found"}})
			return
		}

		sendEventLog(logClient, "payment.deleted", `{"id":`+strconv.Itoa(id)+`}`)
		w.WriteHeader(http.StatusNoContent)
	}
}

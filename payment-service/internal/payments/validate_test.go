package payments

import (
	"testing"
)

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func validCreateBody() map[string]interface{} {
	return map[string]interface{}{
		"order_id": float64(1),
		"user_id":  float64(2),
		"amount":   float64(250.00),
		"method":   "promptpay",
	}
}

// --- ValidateCreatePayment ---

func TestValidateCreatePayment_AcceptsValidBody(t *testing.T) {
	result := ValidateCreatePayment(validCreateBody())
	if !result.Valid || len(result.Errors) != 0 {
		t.Errorf("result = %+v, want valid with no errors", result)
	}
}

func TestValidateCreatePayment_AcceptsAllMethods(t *testing.T) {
	for _, m := range []string{"credit_card", "bank_transfer", "promptpay"} {
		body := validCreateBody()
		body["method"] = m
		result := ValidateCreatePayment(body)
		if !result.Valid {
			t.Errorf("method=%q: Valid = false, want true", m)
		}
	}
}

func TestValidateCreatePayment_RejectsNonObjectBody(t *testing.T) {
	for _, body := range []interface{}{nil, "nope", 42} {
		result := ValidateCreatePayment(body)
		if result.Valid {
			t.Errorf("ValidateCreatePayment(%v).Valid = true, want false", body)
		}
		if !containsString(result.Errors, "body must be a JSON object") {
			t.Errorf("Errors = %v, want body-must-be-object error", result.Errors)
		}
	}
}

func TestValidateCreatePayment_RejectsMissingOrderID(t *testing.T) {
	body := validCreateBody()
	delete(body, "order_id")
	result := ValidateCreatePayment(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "order_id is required") {
		t.Errorf("Errors = %v, want order_id required", result.Errors)
	}
}

func TestValidateCreatePayment_RejectsNonPositiveOrderID(t *testing.T) {
	for _, id := range []float64{0, -1, 1.5} {
		body := validCreateBody()
		body["order_id"] = id
		result := ValidateCreatePayment(body)
		if result.Valid {
			t.Errorf("order_id=%v: Valid = true, want false", id)
		}
		if !containsString(result.Errors, "order_id must be a positive integer") {
			t.Errorf("order_id=%v: Errors = %v, want positive-integer error", id, result.Errors)
		}
	}
}

func TestValidateCreatePayment_RejectsMissingUserID(t *testing.T) {
	body := validCreateBody()
	delete(body, "user_id")
	result := ValidateCreatePayment(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "user_id is required") {
		t.Errorf("Errors = %v, want user_id required", result.Errors)
	}
}

func TestValidateCreatePayment_RejectsMissingAmount(t *testing.T) {
	body := validCreateBody()
	delete(body, "amount")
	result := ValidateCreatePayment(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "amount is required") {
		t.Errorf("Errors = %v, want amount required", result.Errors)
	}
}

func TestValidateCreatePayment_RejectsNonPositiveAmount(t *testing.T) {
	for _, a := range []float64{0, -1} {
		body := validCreateBody()
		body["amount"] = a
		result := ValidateCreatePayment(body)
		if result.Valid {
			t.Errorf("amount=%v: Valid = true, want false", a)
		}
		if !containsString(result.Errors, "amount must be a positive number") {
			t.Errorf("amount=%v: Errors = %v, want positive-number error", a, result.Errors)
		}
	}
}

func TestValidateCreatePayment_RejectsMissingMethod(t *testing.T) {
	body := validCreateBody()
	delete(body, "method")
	result := ValidateCreatePayment(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "method is required") {
		t.Errorf("Errors = %v, want method required", result.Errors)
	}
}

func TestValidateCreatePayment_RejectsUnknownMethod(t *testing.T) {
	body := validCreateBody()
	body["method"] = "bitcoin"
	result := ValidateCreatePayment(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "method must be one of: credit_card, bank_transfer, promptpay") {
		t.Errorf("Errors = %v, want invalid-method error", result.Errors)
	}
}

func TestValidateCreatePayment_CollectsAllErrors(t *testing.T) {
	result := ValidateCreatePayment(map[string]interface{}{})
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if len(result.Errors) != 4 {
		t.Errorf("len(Errors) = %d, want 4 (one per missing field)", len(result.Errors))
	}
}

// --- ValidateUpdatePayment ---

func TestValidateUpdatePayment_AcceptsAllStatuses(t *testing.T) {
	for _, s := range []string{"pending", "completed", "failed", "refunded"} {
		result := ValidateUpdatePayment(map[string]interface{}{"status": s})
		if !result.Valid {
			t.Errorf("status=%q: Valid = false, want true", s)
		}
	}
}

func TestValidateUpdatePayment_RejectsNonObjectBody(t *testing.T) {
	result := ValidateUpdatePayment(nil)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "body must be a JSON object") {
		t.Errorf("Errors = %v, want body-must-be-object error", result.Errors)
	}
}

func TestValidateUpdatePayment_RejectsMissingStatus(t *testing.T) {
	result := ValidateUpdatePayment(map[string]interface{}{})
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "status is required") {
		t.Errorf("Errors = %v, want status required", result.Errors)
	}
}

func TestValidateUpdatePayment_RejectsUnknownStatus(t *testing.T) {
	result := ValidateUpdatePayment(map[string]interface{}{"status": "processing"})
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "status must be one of: pending, completed, failed, refunded") {
		t.Errorf("Errors = %v, want invalid-status error", result.Errors)
	}
}

// --- ToCreatePaymentInput / ToUpdatePaymentInput ---

func TestToCreatePaymentInput(t *testing.T) {
	body := map[string]interface{}{
		"order_id": float64(10),
		"user_id":  float64(5),
		"amount":   float64(99.50),
		"method":   "credit_card",
	}
	input := ToCreatePaymentInput(body)
	if input.OrderID != 10 {
		t.Errorf("OrderID = %d, want 10", input.OrderID)
	}
	if input.UserID != 5 {
		t.Errorf("UserID = %d, want 5", input.UserID)
	}
	if input.Amount != 99.50 {
		t.Errorf("Amount = %f, want 99.50", input.Amount)
	}
	if input.Method != "credit_card" {
		t.Errorf("Method = %q, want credit_card", input.Method)
	}
}

func TestToUpdatePaymentInput(t *testing.T) {
	input := ToUpdatePaymentInput(map[string]interface{}{"status": "completed"})
	if input.Status == nil {
		t.Fatal("Status = nil, want non-nil")
	}
	if *input.Status != "completed" {
		t.Errorf("Status = %q, want completed", *input.Status)
	}
}

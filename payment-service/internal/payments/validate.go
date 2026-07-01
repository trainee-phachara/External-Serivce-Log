package payments

import (
	"payment-service/internal/types"
)

// ValidationResult is the outcome of validating a create/update payment request body.
type ValidationResult struct {
	Valid  bool
	Errors []string
}

var validStatuses = map[string]bool{
	"pending":   true,
	"completed": true,
	"failed":    true,
	"refunded":  true,
}

var validMethods = map[string]bool{
	"credit_card":   true,
	"bank_transfer": true,
	"promptpay":     true,
}

// ValidateCreatePayment validates a raw, untyped request body for POST /payments.
func ValidateCreatePayment(body interface{}) ValidationResult {
	candidate, ok := body.(map[string]interface{})
	if !ok {
		return ValidationResult{Valid: false, Errors: []string{"body must be a JSON object"}}
	}

	errs := make([]string, 0)

	orderIDRaw, hasOrderID := candidate["order_id"]
	if !hasOrderID {
		errs = append(errs, "order_id is required")
	} else if id, ok := orderIDRaw.(float64); !ok || id <= 0 || id != float64(int(id)) {
		errs = append(errs, "order_id must be a positive integer")
	}

	userIDRaw, hasUserID := candidate["user_id"]
	if !hasUserID {
		errs = append(errs, "user_id is required")
	} else if id, ok := userIDRaw.(float64); !ok || id <= 0 || id != float64(int(id)) {
		errs = append(errs, "user_id must be a positive integer")
	}

	amountRaw, hasAmount := candidate["amount"]
	if !hasAmount {
		errs = append(errs, "amount is required")
	} else if a, ok := amountRaw.(float64); !ok || a <= 0 {
		errs = append(errs, "amount must be a positive number")
	}

	methodRaw, hasMethod := candidate["method"]
	if !hasMethod {
		errs = append(errs, "method is required")
	} else if m, ok := methodRaw.(string); !ok || !validMethods[m] {
		errs = append(errs, "method must be one of: credit_card, bank_transfer, promptpay")
	}

	return ValidationResult{Valid: len(errs) == 0, Errors: errs}
}

// ValidateUpdatePayment validates a raw, untyped request body for PUT /payments/{id}.
func ValidateUpdatePayment(body interface{}) ValidationResult {
	candidate, ok := body.(map[string]interface{})
	if !ok {
		return ValidationResult{Valid: false, Errors: []string{"body must be a JSON object"}}
	}

	errs := make([]string, 0)

	statusRaw, hasStatus := candidate["status"]
	if !hasStatus {
		errs = append(errs, "status is required")
	} else if s, ok := statusRaw.(string); !ok || !validStatuses[s] {
		errs = append(errs, "status must be one of: pending, completed, failed, refunded")
	}

	return ValidationResult{Valid: len(errs) == 0, Errors: errs}
}

// ToCreatePaymentInput converts a validated create-payment body into a CreatePaymentInput.
// Safe to call without re-checking types because ValidateCreatePayment already passed.
func ToCreatePaymentInput(candidate map[string]interface{}) types.CreatePaymentInput {
	return types.CreatePaymentInput{
		OrderID: int(candidate["order_id"].(float64)),
		UserID:  int(candidate["user_id"].(float64)),
		Amount:  candidate["amount"].(float64),
		Method:  types.PaymentMethod(candidate["method"].(string)),
	}
}

// ToUpdatePaymentInput converts a validated update-payment body into an UpdatePaymentInput.
func ToUpdatePaymentInput(candidate map[string]interface{}) types.UpdatePaymentInput {
	s := types.PaymentStatus(candidate["status"].(string))
	return types.UpdatePaymentInput{Status: &s}
}

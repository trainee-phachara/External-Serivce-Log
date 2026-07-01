package orders

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

func validItemMap() map[string]interface{} {
	return map[string]interface{}{
		"product_id":   float64(1),
		"product_name": "Widget",
		"quantity":     float64(2),
		"unit_price":   float64(9.99),
	}
}

func validCreateBody() map[string]interface{} {
	return map[string]interface{}{
		"user_id": float64(1),
		"items":   []interface{}{validItemMap()},
	}
}

// --- ValidateCreateOrder ---

func TestValidateCreateOrder_AcceptsValidBody(t *testing.T) {
	result := ValidateCreateOrder(validCreateBody())
	if !result.Valid || len(result.Errors) != 0 {
		t.Errorf("result = %+v, want valid with no errors", result)
	}
}

func TestValidateCreateOrder_RejectsNonObjectBody(t *testing.T) {
	for _, body := range []interface{}{nil, "nope", 42} {
		result := ValidateCreateOrder(body)
		if result.Valid {
			t.Errorf("ValidateCreateOrder(%v).Valid = true, want false", body)
		}
		if !containsString(result.Errors, "body must be a JSON object") {
			t.Errorf("Errors = %v, want body-must-be-object error", result.Errors)
		}
	}
}

func TestValidateCreateOrder_RejectsMissingUserID(t *testing.T) {
	body := map[string]interface{}{
		"items": []interface{}{validItemMap()},
	}
	result := ValidateCreateOrder(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "user_id is required") {
		t.Errorf("Errors = %v, want user_id required error", result.Errors)
	}
}

func TestValidateCreateOrder_RejectsNonPositiveUserID(t *testing.T) {
	for _, uid := range []float64{0, -1, 1.5} {
		body := map[string]interface{}{
			"user_id": uid,
			"items":   []interface{}{validItemMap()},
		}
		result := ValidateCreateOrder(body)
		if result.Valid {
			t.Errorf("user_id=%v: Valid = true, want false", uid)
		}
		if !containsString(result.Errors, "user_id must be a positive integer") {
			t.Errorf("user_id=%v: Errors = %v, want positive-integer error", uid, result.Errors)
		}
	}
}

func TestValidateCreateOrder_RejectsMissingItems(t *testing.T) {
	body := map[string]interface{}{"user_id": float64(1)}
	result := ValidateCreateOrder(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "items is required") {
		t.Errorf("Errors = %v, want items required error", result.Errors)
	}
}

func TestValidateCreateOrder_RejectsEmptyItems(t *testing.T) {
	body := map[string]interface{}{
		"user_id": float64(1),
		"items":   []interface{}{},
	}
	result := ValidateCreateOrder(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "items must not be empty") {
		t.Errorf("Errors = %v, want items-not-empty error", result.Errors)
	}
}

func TestValidateCreateOrder_RejectsNonArrayItems(t *testing.T) {
	body := map[string]interface{}{
		"user_id": float64(1),
		"items":   "not an array",
	}
	result := ValidateCreateOrder(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "items must be an array") {
		t.Errorf("Errors = %v, want items-must-be-array error", result.Errors)
	}
}

func TestValidateCreateOrder_RejectsItemMissingFields(t *testing.T) {
	body := map[string]interface{}{
		"user_id": float64(1),
		"items":   []interface{}{map[string]interface{}{}},
	}
	result := ValidateCreateOrder(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "items[0].product_id is required") {
		t.Errorf("Errors = %v, want product_id required", result.Errors)
	}
	if !containsString(result.Errors, "items[0].product_name is required") {
		t.Errorf("Errors = %v, want product_name required", result.Errors)
	}
	if !containsString(result.Errors, "items[0].quantity is required") {
		t.Errorf("Errors = %v, want quantity required", result.Errors)
	}
	if !containsString(result.Errors, "items[0].unit_price is required") {
		t.Errorf("Errors = %v, want unit_price required", result.Errors)
	}
}

func TestValidateCreateOrder_RejectsItemNonObjectElement(t *testing.T) {
	body := map[string]interface{}{
		"user_id": float64(1),
		"items":   []interface{}{"not an object"},
	}
	result := ValidateCreateOrder(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "items[0] must be an object") {
		t.Errorf("Errors = %v, want items[0]-must-be-object error", result.Errors)
	}
}

func TestValidateCreateOrder_RejectsNegativeUnitPrice(t *testing.T) {
	item := validItemMap()
	item["unit_price"] = float64(-1)
	body := map[string]interface{}{
		"user_id": float64(1),
		"items":   []interface{}{item},
	}
	result := ValidateCreateOrder(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "items[0].unit_price must be a non-negative number") {
		t.Errorf("Errors = %v, want unit_price error", result.Errors)
	}
}

// --- ValidateUpdateOrder ---

func TestValidateUpdateOrder_AcceptsValidStatuses(t *testing.T) {
	for _, s := range []string{"pending", "processing", "shipped", "delivered", "cancelled"} {
		result := ValidateUpdateOrder(map[string]interface{}{"status": s})
		if !result.Valid {
			t.Errorf("status=%q: Valid = false, want true", s)
		}
	}
}

func TestValidateUpdateOrder_RejectsNonObjectBody(t *testing.T) {
	result := ValidateUpdateOrder(nil)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "body must be a JSON object") {
		t.Errorf("Errors = %v, want body-must-be-object error", result.Errors)
	}
}

func TestValidateUpdateOrder_RejectsMissingStatus(t *testing.T) {
	result := ValidateUpdateOrder(map[string]interface{}{})
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "status is required") {
		t.Errorf("Errors = %v, want status required error", result.Errors)
	}
}

func TestValidateUpdateOrder_RejectsUnknownStatus(t *testing.T) {
	result := ValidateUpdateOrder(map[string]interface{}{"status": "unknown"})
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "status must be one of: pending, processing, shipped, delivered, cancelled") {
		t.Errorf("Errors = %v, want invalid-status error", result.Errors)
	}
}

// --- ToCreateOrderInput / ToUpdateOrderInput ---

func TestToCreateOrderInput(t *testing.T) {
	body := map[string]interface{}{
		"user_id": float64(5),
		"items": []interface{}{
			map[string]interface{}{
				"product_id":   float64(10),
				"product_name": "Widget",
				"quantity":     float64(3),
				"unit_price":   float64(1.5),
			},
		},
	}
	input := ToCreateOrderInput(body)
	if input.UserID != 5 {
		t.Errorf("UserID = %d, want 5", input.UserID)
	}
	if len(input.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(input.Items))
	}
	item := input.Items[0]
	if item.ProductID != 10 || item.ProductName != "Widget" || item.Quantity != 3 || item.UnitPrice != 1.5 {
		t.Errorf("item = %+v, want {10 Widget 3 1.5}", item)
	}
}

func TestToUpdateOrderInput(t *testing.T) {
	input := ToUpdateOrderInput(map[string]interface{}{"status": "shipped"})
	if input.Status == nil {
		t.Fatal("Status = nil, want non-nil")
	}
	if *input.Status != "shipped" {
		t.Errorf("Status = %q, want shipped", *input.Status)
	}
}

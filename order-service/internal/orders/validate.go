package orders

import (
	"fmt"

	"order-service/internal/types"
)

// ValidationResult is the outcome of validating a create/update order request body.
type ValidationResult struct {
	Valid  bool
	Errors []string
}

var validStatuses = map[string]bool{
	"pending":    true,
	"processing": true,
	"shipped":    true,
	"delivered":  true,
	"cancelled":  true,
}

// ValidateCreateOrder validates a raw, untyped request body for POST /orders.
func ValidateCreateOrder(body interface{}) ValidationResult {
	candidate, ok := body.(map[string]interface{})
	if !ok {
		return ValidationResult{Valid: false, Errors: []string{"body must be a JSON object"}}
	}

	errs := make([]string, 0)

	userIDRaw, hasUserID := candidate["user_id"]
	if !hasUserID {
		errs = append(errs, "user_id is required")
	} else {
		// JSON numbers always decode as float64
		userIDFloat, ok := userIDRaw.(float64)
		if !ok || userIDFloat <= 0 || userIDFloat != float64(int(userIDFloat)) {
			errs = append(errs, "user_id must be a positive integer")
		}
	}

	itemsRaw, hasItems := candidate["items"]
	if !hasItems {
		errs = append(errs, "items is required")
	} else {
		itemSlice, ok := itemsRaw.([]interface{})
		if !ok {
			errs = append(errs, "items must be an array")
		} else if len(itemSlice) == 0 {
			errs = append(errs, "items must not be empty")
		} else {
			for i, raw := range itemSlice {
				item, ok := raw.(map[string]interface{})
				if !ok {
					errs = append(errs, fmt.Sprintf("items[%d] must be an object", i))
					continue
				}
				errs = append(errs, validateItem(item, i)...)
			}
		}
	}

	return ValidationResult{Valid: len(errs) == 0, Errors: errs}
}

func validateItem(item map[string]interface{}, i int) []string {
	errs := make([]string, 0)

	pidRaw, hasPID := item["product_id"]
	if !hasPID {
		errs = append(errs, fmt.Sprintf("items[%d].product_id is required", i))
	} else if pid, ok := pidRaw.(float64); !ok || pid <= 0 || pid != float64(int(pid)) {
		errs = append(errs, fmt.Sprintf("items[%d].product_id must be a positive integer", i))
	}

	nameRaw, hasName := item["product_name"]
	if !hasName {
		errs = append(errs, fmt.Sprintf("items[%d].product_name is required", i))
	} else if name, ok := nameRaw.(string); !ok || name == "" {
		errs = append(errs, fmt.Sprintf("items[%d].product_name must be a non-empty string", i))
	}

	qtyRaw, hasQty := item["quantity"]
	if !hasQty {
		errs = append(errs, fmt.Sprintf("items[%d].quantity is required", i))
	} else if qty, ok := qtyRaw.(float64); !ok || qty <= 0 || qty != float64(int(qty)) {
		errs = append(errs, fmt.Sprintf("items[%d].quantity must be a positive integer", i))
	}

	priceRaw, hasPrice := item["unit_price"]
	if !hasPrice {
		errs = append(errs, fmt.Sprintf("items[%d].unit_price is required", i))
	} else if price, ok := priceRaw.(float64); !ok || price < 0 {
		errs = append(errs, fmt.Sprintf("items[%d].unit_price must be a non-negative number", i))
	}

	return errs
}

// ValidateUpdateOrder validates a raw, untyped request body for PUT /orders/{id}.
func ValidateUpdateOrder(body interface{}) ValidationResult {
	candidate, ok := body.(map[string]interface{})
	if !ok {
		return ValidationResult{Valid: false, Errors: []string{"body must be a JSON object"}}
	}

	errs := make([]string, 0)

	statusRaw, hasStatus := candidate["status"]
	if !hasStatus {
		errs = append(errs, "status is required")
	} else if s, ok := statusRaw.(string); !ok || !validStatuses[s] {
		errs = append(errs, "status must be one of: pending, processing, shipped, delivered, cancelled")
	}

	return ValidationResult{Valid: len(errs) == 0, Errors: errs}
}

// ToCreateOrderInput converts a validated create-order body into a CreateOrderInput.
// Safe to call without re-checking types because ValidateCreateOrder already passed.
func ToCreateOrderInput(candidate map[string]interface{}) types.CreateOrderInput {
	userID := int(candidate["user_id"].(float64))
	rawItems := candidate["items"].([]interface{})

	items := make([]types.OrderItem, len(rawItems))
	for i, raw := range rawItems {
		m := raw.(map[string]interface{})
		items[i] = types.OrderItem{
			ProductID:   int(m["product_id"].(float64)),
			ProductName: m["product_name"].(string),
			Quantity:    int(m["quantity"].(float64)),
			UnitPrice:   m["unit_price"].(float64),
		}
	}

	return types.CreateOrderInput{UserID: userID, Items: items}
}

// ToUpdateOrderInput converts a validated update-order body into an UpdateOrderInput.
func ToUpdateOrderInput(candidate map[string]interface{}) types.UpdateOrderInput {
	s := types.OrderStatus(candidate["status"].(string))
	return types.UpdateOrderInput{Status: &s}
}

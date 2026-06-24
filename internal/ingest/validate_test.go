package ingest

import "testing"

func validBody() map[string]interface{} {
	return map[string]interface{}{
		"source": map[string]interface{}{
			"app_name":     "order-service",
			"service_name": "order",
		},
		"trace_id":    "trace-1",
		"endpoint":    "/api/v1/orders",
		"http_status": "200",
		"type":        "request",
		"direction":   "inbound",
		"metadata":    map[string]interface{}{"foo": "bar"},
		"raw_payload": map[string]interface{}{"raw": true},
		"payload":     map[string]interface{}{"id": float64(1)},
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func TestValidateIngestBody_AcceptsFullyValidBody(t *testing.T) {
	result := ValidateIngestBody(validBody())
	if !result.Valid {
		t.Errorf("Valid = false, errors = %v", result.Errors)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %v, want empty", result.Errors)
	}
}

func TestValidateIngestBody_AcceptsBodyWithoutOptionalFields(t *testing.T) {
	body := validBody()
	delete(body, "metadata")
	delete(body, "raw_payload")
	delete(body, "payload")

	result := ValidateIngestBody(body)
	if !result.Valid {
		t.Errorf("Valid = false, errors = %v", result.Errors)
	}
}

func TestValidateIngestBody_RejectsNonObjectBody(t *testing.T) {
	result := ValidateIngestBody("not-an-object")
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "Request body must be a JSON object") {
		t.Errorf("Errors = %v, want to contain %q", result.Errors, "Request body must be a JSON object")
	}
}

func TestValidateIngestBody_RejectsMissingSource(t *testing.T) {
	body := validBody()
	delete(body, "source")

	result := ValidateIngestBody(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	want := "source must be an object with app_name and service_name"
	if !containsString(result.Errors, want) {
		t.Errorf("Errors = %v, want to contain %q", result.Errors, want)
	}
}

func TestValidateIngestBody_RejectsSourceMissingFields(t *testing.T) {
	body := validBody()
	body["source"] = map[string]interface{}{"app_name": "order-service"}

	result := ValidateIngestBody(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	want := "source.service_name is required and must be a non-empty string"
	if !containsString(result.Errors, want) {
		t.Errorf("Errors = %v, want to contain %q", result.Errors, want)
	}
}

func TestValidateIngestBody_RejectsMissingRequiredStringFields(t *testing.T) {
	body := validBody()
	delete(body, "trace_id")
	delete(body, "endpoint")
	delete(body, "http_status")

	result := ValidateIngestBody(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	for _, want := range []string{
		"trace_id is required and must be a non-empty string",
		"endpoint is required and must be a non-empty string",
		"http_status is required and must be a non-empty string",
	} {
		if !containsString(result.Errors, want) {
			t.Errorf("Errors = %v, want to contain %q", result.Errors, want)
		}
	}
}

func TestValidateIngestBody_RejectsInvalidType(t *testing.T) {
	body := validBody()
	body["type"] = "banana"

	result := ValidateIngestBody(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	want := "type is required and must be one of: request, response"
	if !containsString(result.Errors, want) {
		t.Errorf("Errors = %v, want to contain %q", result.Errors, want)
	}
}

func TestValidateIngestBody_RejectsInvalidDirection(t *testing.T) {
	body := validBody()
	body["direction"] = "sideways"

	result := ValidateIngestBody(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	want := "direction is required and must be one of: inbound, outbound"
	if !containsString(result.Errors, want) {
		t.Errorf("Errors = %v, want to contain %q", result.Errors, want)
	}
}

func TestValidateIngestBody_RejectsNonObjectOptionalFields(t *testing.T) {
	body := validBody()
	body["metadata"] = "oops"
	body["raw_payload"] = []interface{}{"oops"}
	body["payload"] = float64(42)

	result := ValidateIngestBody(body)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	for _, want := range []string{
		"metadata must be an object when provided",
		"raw_payload must be an object when provided",
		"payload must be an object when provided",
	} {
		if !containsString(result.Errors, want) {
			t.Errorf("Errors = %v, want to contain %q", result.Errors, want)
		}
	}
}

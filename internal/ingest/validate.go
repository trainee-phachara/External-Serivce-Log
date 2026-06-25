package ingest

var validLogTypes = map[string]bool{"request": true, "response": true, "event": true}
var validLogDirections = map[string]bool{"inbound": true, "outbound": true}

// ValidationResult is the outcome of validating an ingest request body.
type ValidationResult struct {
	Valid  bool
	Errors []string
}

func isNonEmptyString(value interface{}) bool {
	s, ok := value.(string)
	return ok && len(s) > 0
}

func isPlainObject(value interface{}) (map[string]interface{}, bool) {
	m, ok := value.(map[string]interface{})
	return m, ok
}

// ValidateIngestBody validates a raw, untyped ingest request body (decoded
// JSON from HTTP, or an equivalent map built from a gRPC request) and
// collects every validation error found rather than stopping at the first.
func ValidateIngestBody(body interface{}) ValidationResult {
	candidate, ok := isPlainObject(body)
	if !ok {
		return ValidationResult{Valid: false, Errors: []string{"Request body must be a JSON object"}}
	}

	errors := make([]string, 0)

	if source, isObject := isPlainObject(candidate["source"]); !isObject {
		errors = append(errors, "source must be an object with app_name and service_name")
	} else {
		if !isNonEmptyString(source["app_name"]) {
			errors = append(errors, "source.app_name is required and must be a non-empty string")
		}
		if !isNonEmptyString(source["service_name"]) {
			errors = append(errors, "source.service_name is required and must be a non-empty string")
		}
	}

	if !isNonEmptyString(candidate["trace_id"]) {
		errors = append(errors, "trace_id is required and must be a non-empty string")
	}

	if !isNonEmptyString(candidate["endpoint"]) {
		errors = append(errors, "endpoint is required and must be a non-empty string")
	}

	if !isNonEmptyString(candidate["http_status"]) {
		errors = append(errors, "http_status is required and must be a non-empty string")
	}

	if logType, ok := candidate["type"].(string); !ok || !validLogTypes[logType] {
		errors = append(errors, "type is required and must be one of: request, response")
	}

	if direction, ok := candidate["direction"].(string); !ok || !validLogDirections[direction] {
		errors = append(errors, "direction is required and must be one of: inbound, outbound")
	}

	if metadata, present := candidate["metadata"]; present {
		if _, isObject := isPlainObject(metadata); !isObject {
			errors = append(errors, "metadata must be an object when provided")
		}
	}

	if rawPayload, present := candidate["raw_payload"]; present {
		if _, isObject := isPlainObject(rawPayload); !isObject {
			errors = append(errors, "raw_payload must be an object when provided")
		}
	}

	if payload, present := candidate["payload"]; present {
		if _, isObject := isPlainObject(payload); !isObject {
			errors = append(errors, "payload must be an object when provided")
		}
	}

	return ValidationResult{Valid: len(errors) == 0, Errors: errors}
}

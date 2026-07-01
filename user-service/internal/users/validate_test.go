package users

import (
	"reflect"
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

func TestValidateCreateUser_AcceptsValidBody(t *testing.T) {
	result := ValidateCreateUser(map[string]interface{}{"name": "Alice", "email": "alice@example.com"})
	if !result.Valid || len(result.Errors) != 0 {
		t.Errorf("result = %+v, want valid with no errors", result)
	}
}

func TestValidateCreateUser_RejectsNonObjectBody(t *testing.T) {
	for _, body := range []interface{}{nil, "nope"} {
		result := ValidateCreateUser(body)
		if result.Valid {
			t.Errorf("ValidateCreateUser(%v).Valid = true, want false", body)
		}
		want := []string{"body must be a JSON object"}
		if !reflect.DeepEqual(result.Errors, want) {
			t.Errorf("ValidateCreateUser(%v).Errors = %v, want %v", body, result.Errors, want)
		}
	}
}

func TestValidateCreateUser_RejectsMissingOrEmptyName(t *testing.T) {
	missing := ValidateCreateUser(map[string]interface{}{"email": "alice@example.com"})
	if missing.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(missing.Errors, "name is required and must be a non-empty string") {
		t.Errorf("Errors = %v, want to contain the name error", missing.Errors)
	}

	blank := ValidateCreateUser(map[string]interface{}{"name": "   ", "email": "alice@example.com"})
	if !containsString(blank.Errors, "name is required and must be a non-empty string") {
		t.Errorf("Errors = %v, want to contain the name error", blank.Errors)
	}
}

func TestValidateCreateUser_RejectsMissingOrEmptyEmail(t *testing.T) {
	missing := ValidateCreateUser(map[string]interface{}{"name": "Alice"})
	if missing.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(missing.Errors, "email is required and must be a non-empty string") {
		t.Errorf("Errors = %v, want to contain the email error", missing.Errors)
	}

	blank := ValidateCreateUser(map[string]interface{}{"name": "Alice", "email": "   "})
	if !containsString(blank.Errors, "email is required and must be a non-empty string") {
		t.Errorf("Errors = %v, want to contain the email error", blank.Errors)
	}
}

func TestValidateCreateUser_RejectsMalformedEmail(t *testing.T) {
	result := ValidateCreateUser(map[string]interface{}{"name": "Alice", "email": "not-an-email"})
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "email must be a valid email address") {
		t.Errorf("Errors = %v, want to contain the malformed email error", result.Errors)
	}
}

func TestValidateUpdateUser_AcceptsPartialAndFullBodies(t *testing.T) {
	cases := []map[string]interface{}{
		{"name": "Alice"},
		{"email": "alice@example.com"},
		{"name": "Alice", "email": "alice@example.com"},
	}
	for _, body := range cases {
		result := ValidateUpdateUser(body)
		if !result.Valid || len(result.Errors) != 0 {
			t.Errorf("ValidateUpdateUser(%v) = %+v, want valid with no errors", body, result)
		}
	}
}

func TestValidateUpdateUser_RejectsNonObjectBody(t *testing.T) {
	result := ValidateUpdateUser(nil)
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	want := []string{"body must be a JSON object"}
	if !reflect.DeepEqual(result.Errors, want) {
		t.Errorf("Errors = %v, want %v", result.Errors, want)
	}
}

func TestValidateUpdateUser_RejectsEmptyBody(t *testing.T) {
	result := ValidateUpdateUser(map[string]interface{}{})
	if result.Valid {
		t.Error("Valid = true, want false")
	}
	if !containsString(result.Errors, "at least one of name or email must be provided") {
		t.Errorf("Errors = %v, want to contain the empty-body error", result.Errors)
	}
}

func TestValidateUpdateUser_RejectsEmptyStringName(t *testing.T) {
	result := ValidateUpdateUser(map[string]interface{}{"name": "   "})
	if !containsString(result.Errors, "name must be a non-empty string when provided") {
		t.Errorf("Errors = %v, want to contain the empty-name error", result.Errors)
	}
}

func TestValidateUpdateUser_RejectsEmptyStringEmail(t *testing.T) {
	result := ValidateUpdateUser(map[string]interface{}{"email": "   "})
	if !containsString(result.Errors, "email must be a non-empty string when provided") {
		t.Errorf("Errors = %v, want to contain the empty-email error", result.Errors)
	}
}

func TestValidateUpdateUser_RejectsMalformedEmail(t *testing.T) {
	result := ValidateUpdateUser(map[string]interface{}{"email": "not-an-email"})
	if !containsString(result.Errors, "email must be a valid email address") {
		t.Errorf("Errors = %v, want to contain the malformed email error", result.Errors)
	}
}

func TestToCreateUserInput(t *testing.T) {
	input := ToCreateUserInput(map[string]interface{}{"name": "Alice", "email": "alice@example.com"})
	if input.Name != "Alice" || input.Email != "alice@example.com" {
		t.Errorf("input = %+v, want {Alice alice@example.com}", input)
	}
}

func TestToUpdateUserInput(t *testing.T) {
	t.Run("only name", func(t *testing.T) {
		input := ToUpdateUserInput(map[string]interface{}{"name": "Alice"})
		if input.Name == nil || *input.Name != "Alice" {
			t.Errorf("Name = %v, want Alice", input.Name)
		}
		if input.Email != nil {
			t.Errorf("Email = %v, want nil", input.Email)
		}
	})

	t.Run("only email", func(t *testing.T) {
		input := ToUpdateUserInput(map[string]interface{}{"email": "alice@example.com"})
		if input.Email == nil || *input.Email != "alice@example.com" {
			t.Errorf("Email = %v, want alice@example.com", input.Email)
		}
		if input.Name != nil {
			t.Errorf("Name = %v, want nil", input.Name)
		}
	})

	t.Run("empty body", func(t *testing.T) {
		input := ToUpdateUserInput(map[string]interface{}{})
		if input.Name != nil || input.Email != nil {
			t.Errorf("input = %+v, want both nil", input)
		}
	})
}

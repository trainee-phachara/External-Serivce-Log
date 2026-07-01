package users

import (
	"regexp"
	"strings"

	"user-service/internal/types"
)

// ValidationResult is the outcome of validating a create/update user request body.
type ValidationResult struct {
	Valid  bool
	Errors []string
}

var emailRegex = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func isNonEmptyString(value interface{}) bool {
	s, ok := value.(string)
	return ok && strings.TrimSpace(s) != ""
}

// ValidateCreateUser validates a raw, untyped request body for POST /users.
func ValidateCreateUser(body interface{}) ValidationResult {
	candidate, ok := body.(map[string]interface{})
	if !ok {
		return ValidationResult{Valid: false, Errors: []string{"body must be a JSON object"}}
	}

	errors := make([]string, 0)

	name := candidate["name"]
	if !isNonEmptyString(name) {
		errors = append(errors, "name is required and must be a non-empty string")
	}

	email := candidate["email"]
	if !isNonEmptyString(email) {
		errors = append(errors, "email is required and must be a non-empty string")
	} else if !emailRegex.MatchString(email.(string)) {
		errors = append(errors, "email must be a valid email address")
	}

	return ValidationResult{Valid: len(errors) == 0, Errors: errors}
}

// ValidateUpdateUser validates a raw, untyped request body for PUT /users/{id}.
func ValidateUpdateUser(body interface{}) ValidationResult {
	candidate, ok := body.(map[string]interface{})
	if !ok {
		return ValidationResult{Valid: false, Errors: []string{"body must be a JSON object"}}
	}

	errors := make([]string, 0)

	name, hasName := candidate["name"]
	if hasName && !isNonEmptyString(name) {
		errors = append(errors, "name must be a non-empty string when provided")
	}

	email, hasEmail := candidate["email"]
	if hasEmail {
		if !isNonEmptyString(email) {
			errors = append(errors, "email must be a non-empty string when provided")
		} else if !emailRegex.MatchString(email.(string)) {
			errors = append(errors, "email must be a valid email address")
		}
	}

	if !hasName && !hasEmail {
		errors = append(errors, "at least one of name or email must be provided")
	}

	return ValidationResult{Valid: len(errors) == 0, Errors: errors}
}

// ToCreateUserInput converts a validated create-user body into a CreateUserInput.
func ToCreateUserInput(candidate map[string]interface{}) types.CreateUserInput {
	return types.CreateUserInput{
		Name:  candidate["name"].(string),
		Email: candidate["email"].(string),
	}
}

// ToUpdateUserInput converts a validated update-user body into an UpdateUserInput.
func ToUpdateUserInput(candidate map[string]interface{}) types.UpdateUserInput {
	input := types.UpdateUserInput{}
	if name, ok := candidate["name"].(string); ok {
		input.Name = &name
	}
	if email, ok := candidate["email"].(string); ok {
		input.Email = &email
	}
	return input
}

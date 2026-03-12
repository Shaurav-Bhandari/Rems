package middleware

import (
	"backend/utils"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
)

// ============================================================================
// REQUEST BODY VALIDATION MIDDLEWARE
// Integrates go-playground/validator/v10 to validate request bodies parsed
// into DTO structs. Returns structured 422 Unprocessable Entity responses
// with per-field error details. Provides both middleware and helper functions
// for use in handlers.
// ============================================================================

// Validate is the singleton validator instance used across the application.
// It is initialized once and reused (validator is goroutine-safe).
var Validate *validator.Validate

func init() {
	Validate = validator.New()

	// Register a custom tag name function so that JSON field names are
	// reported in error messages instead of Go struct field names.
	Validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "" || name == "-" {
			return fld.Name
		}
		return name
	})
}

// ValidationError represents a single field validation error.
type ValidationError struct {
	Field   string `json:"field"`
	Tag     string `json:"tag"`
	Value   string `json:"value,omitempty"`
	Message string `json:"message"`
}

// ValidateBody is a generic helper function that parses and validates a
// request body into the given struct type T. Returns the parsed struct
// and nil on success, or nil and an error response on failure.
//
// Usage in handlers:
//
//	func CreateOrder(c fiber.Ctx) error {
//	    req, err := middleware.ValidateBody[DTO.CreateOrderRequest](c)
//	    if err != nil {
//	        return err // Already sent the 422 response
//	    }
//	    // Use req...
//	}
func ValidateBody[T any](c fiber.Ctx) (*T, error) {
	var body T

	// Parse body
	if err := c.Bind().JSON(&body); err != nil {
		return nil, utils.SendResponse(c, fiber.StatusBadRequest,
			"Invalid request body: malformed JSON", map[string]interface{}{
				"parse_error": err.Error(),
			})
	}

	// Validate
	if err := Validate.Struct(body); err != nil {
		validationErrors := formatValidationErrors(err)
		return nil, utils.SendResponse(c, fiber.StatusUnprocessableEntity,
			"Validation failed", map[string]interface{}{
				"errors": validationErrors,
				"count":  len(validationErrors),
			})
	}

	return &body, nil
}

// ValidateStruct validates any struct and returns formatted errors.
// Useful when you need to validate a struct that wasn't parsed from a request body.
func ValidateStruct(s interface{}) []ValidationError {
	if err := Validate.Struct(s); err != nil {
		return formatValidationErrors(err)
	}
	return nil
}

// formatValidationErrors converts validator.ValidationErrors into
// a slice of human-readable ValidationError structs.
func formatValidationErrors(err error) []ValidationError {
	var errors []ValidationError

	if validationErrs, ok := err.(validator.ValidationErrors); ok {
		for _, e := range validationErrs {
			ve := ValidationError{
				Field:   e.Field(),
				Tag:     e.Tag(),
				Value:   fmt.Sprintf("%v", e.Value()),
				Message: buildValidationMessage(e),
			}
			errors = append(errors, ve)
		}
	} else {
		// Non-validation error (shouldn't happen, but be defensive)
		errors = append(errors, ValidationError{
			Field:   "unknown",
			Tag:     "unknown",
			Message: err.Error(),
		})
	}

	return errors
}

// buildValidationMessage creates a human-readable error message from a
// validator.FieldError.
func buildValidationMessage(fe validator.FieldError) string {
	field := fe.Field()

	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", field)
	case "email":
		return fmt.Sprintf("%s must be a valid email address", field)
	case "min":
		if fe.Type().Kind() == reflect.String {
			return fmt.Sprintf("%s must be at least %s characters long", field, fe.Param())
		}
		return fmt.Sprintf("%s must be at least %s", field, fe.Param())
	case "max":
		if fe.Type().Kind() == reflect.String {
			return fmt.Sprintf("%s must be at most %s characters long", field, fe.Param())
		}
		return fmt.Sprintf("%s must be at most %s", field, fe.Param())
	case "oneof":
		return fmt.Sprintf("%s must be one of: %s", field, fe.Param())
	case "eqfield":
		return fmt.Sprintf("%s must match %s", field, fe.Param())
	case "uuid4", "uuid":
		return fmt.Sprintf("%s must be a valid UUID", field)
	case "e164":
		return fmt.Sprintf("%s must be a valid phone number in E.164 format", field)
	case "url":
		return fmt.Sprintf("%s must be a valid URL", field)
	case "gte":
		return fmt.Sprintf("%s must be greater than or equal to %s", field, fe.Param())
	case "lte":
		return fmt.Sprintf("%s must be less than or equal to %s", field, fe.Param())
	case "gt":
		return fmt.Sprintf("%s must be greater than %s", field, fe.Param())
	case "lt":
		return fmt.Sprintf("%s must be less than %s", field, fe.Param())
	case "len":
		return fmt.Sprintf("%s must be exactly %s characters long", field, fe.Param())
	case "alphanum":
		return fmt.Sprintf("%s must contain only alphanumeric characters", field)
	default:
		return fmt.Sprintf("%s failed validation: %s", field, fe.Tag())
	}
}

package errors

import (
	"fmt"
	"net/http"
	"testing"
)

func TestBadRequest(t *testing.T) {
	err := BadRequest("invalid input")
	if err.Code != CodeBadRequest {
		t.Errorf("expected code %s, got %s", CodeBadRequest, err.Code)
	}
	if err.Message != "invalid input" {
		t.Errorf("expected message 'invalid input', got '%s'", err.Message)
	}
	if err.HTTPStatus != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, err.HTTPStatus)
	}
}

func TestUnauthorized(t *testing.T) {
	err := Unauthorized("no access")
	if err.Code != CodeUnauthorized {
		t.Errorf("expected code %s, got %s", CodeUnauthorized, err.Code)
	}
	if err.HTTPStatus != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, err.HTTPStatus)
	}
}

func TestNotFound(t *testing.T) {
	err := NotFound("user")
	if err.Message != "user not found" {
		t.Errorf("expected message 'user not found', got '%s'", err.Message)
	}
	if err.HTTPStatus != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, err.HTTPStatus)
	}
}

func TestConflict(t *testing.T) {
	err := Conflict("already exists")
	if err.Code != CodeConflict {
		t.Errorf("expected code %s, got %s", CodeConflict, err.Code)
	}
	if err.HTTPStatus != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, err.HTTPStatus)
	}
}

func TestInternal(t *testing.T) {
	err := Internal("something broke")
	if err.Code != CodeInternal {
		t.Errorf("expected code %s, got %s", CodeInternal, err.Code)
	}
	if err.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, err.HTTPStatus)
	}
}

func TestAs(t *testing.T) {
	original := BadRequest("test error")
	wrapped := fmt.Errorf("wrapping: %w", original)

	extracted, ok := As(wrapped)
	if !ok {
		t.Fatal("expected As() to find APIError in wrapped error")
	}
	if extracted.Code != CodeBadRequest {
		t.Errorf("expected code %s, got %s", CodeBadRequest, extracted.Code)
	}
	if extracted.Message != "test error" {
		t.Errorf("expected message 'test error', got '%s'", extracted.Message)
	}
}

func TestAs_NotAPIError(t *testing.T) {
	plainErr := fmt.Errorf("just a plain error")
	_, ok := As(plainErr)
	if ok {
		t.Fatal("expected As() to return false for non-APIError")
	}
}

func TestWithCause(t *testing.T) {
	cause := fmt.Errorf("database connection failed")
	err := Internal("operation failed").WithCause(cause)

	// Check Error() includes cause
	errStr := err.Error()
	if errStr == "" {
		t.Fatal("Error() returned empty string")
	}

	// Check Unwrap returns the cause
	unwrapped := err.Unwrap()
	if unwrapped == nil {
		t.Fatal("expected Unwrap() to return cause, got nil")
	}
	if unwrapped.Error() != "database connection failed" {
		t.Errorf("expected cause message 'database connection failed', got '%s'", unwrapped.Error())
	}
}

func TestWithDetails(t *testing.T) {
	details := map[string]string{"field": "email", "reason": "invalid format"}
	err := BadRequest("validation failed").WithDetails(details)
	if err.Details == nil {
		t.Fatal("expected Details to be set")
	}
}

func TestWithRequestID(t *testing.T) {
	err := BadRequest("test").WithRequestID("req-123")
	if err.RequestID != "req-123" {
		t.Errorf("expected request_id 'req-123', got '%s'", err.RequestID)
	}
}

func TestRateLimitExceeded(t *testing.T) {
	err := RateLimitExceeded()
	if err.Code != CodeTooManyRequests {
		t.Errorf("expected code %s, got %s", CodeTooManyRequests, err.Code)
	}
	if err.HTTPStatus != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, err.HTTPStatus)
	}
}

func TestValidation(t *testing.T) {
	fields := []ValidationError{
		{Field: "email", Message: "required"},
		{Field: "password", Message: "too short"},
	}
	err := Validation(fields)
	if err.Code != CodeValidation {
		t.Errorf("expected code %s, got %s", CodeValidation, err.Code)
	}
	if err.Details == nil {
		t.Fatal("expected Details to contain validation errors")
	}
}

func TestError_String_WithCause(t *testing.T) {
	cause := fmt.Errorf("timeout")
	err := Internal("failed").WithCause(cause)
	s := err.Error()
	if s == "" {
		t.Fatal("Error() should not be empty")
	}
	// Should contain the code, message, and cause
	expected := "INTERNAL_ERROR: failed (caused by: timeout)"
	if s != expected {
		t.Errorf("expected '%s', got '%s'", expected, s)
	}
}

func TestError_String_WithoutCause(t *testing.T) {
	err := BadRequest("bad input")
	s := err.Error()
	expected := "BAD_REQUEST: bad input"
	if s != expected {
		t.Errorf("expected '%s', got '%s'", expected, s)
	}
}

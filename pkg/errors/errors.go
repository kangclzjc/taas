package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// Code is a machine-readable error code.
type Code string

const (
	// 4xx client errors
	CodeBadRequest       Code = "BAD_REQUEST"
	CodeUnauthorized     Code = "UNAUTHORIZED"
	CodeForbidden        Code = "FORBIDDEN"
	CodeNotFound         Code = "NOT_FOUND"
	CodeConflict         Code = "CONFLICT"
	CodeTooManyRequests  Code = "RATE_LIMIT_EXCEEDED"
	CodeQuotaExceeded    Code = "QUOTA_EXCEEDED"
	CodeTokenExpired     Code = "TOKEN_EXPIRED"
	CodeTokenRevoked     Code = "TOKEN_REVOKED"
	CodeInvalidToken     Code = "INVALID_TOKEN"
	CodeModelNotReady    Code = "MODEL_NOT_READY"
	CodeBudgetExceeded   Code = "BUDGET_EXCEEDED"
	CodeValidation       Code = "VALIDATION_ERROR"

	// 5xx server errors
	CodeInternal         Code = "INTERNAL_ERROR"
	CodeServiceUnavail   Code = "SERVICE_UNAVAILABLE"
	CodeDynamoError      Code = "DYNAMO_ERROR"
	CodeUpstream         Code = "UPSTREAM_ERROR"
)

// APIError is a structured error returned in API responses.
type APIError struct {
	Code       Code   `json:"code"`
	Message    string `json:"message"`
	Details    any    `json:"details,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	HTTPStatus int    `json:"-"`
	cause      error
}

func (e *APIError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *APIError) Unwrap() error { return e.cause }

func (e *APIError) WithCause(err error) *APIError {
	e.cause = err
	return e
}

func (e *APIError) WithDetails(d any) *APIError {
	e.Details = d
	return e
}

func (e *APIError) WithRequestID(id string) *APIError {
	e.RequestID = id
	return e
}

// Constructors

func New(code Code, message string, status int) *APIError {
	return &APIError{Code: code, Message: message, HTTPStatus: status}
}

func BadRequest(message string) *APIError {
	return New(CodeBadRequest, message, http.StatusBadRequest)
}

func Unauthorized(message string) *APIError {
	return New(CodeUnauthorized, message, http.StatusUnauthorized)
}

func Forbidden(message string) *APIError {
	return New(CodeForbidden, message, http.StatusForbidden)
}

func NotFound(resource string) *APIError {
	return New(CodeNotFound, fmt.Sprintf("%s not found", resource), http.StatusNotFound)
}

func Conflict(message string) *APIError {
	return New(CodeConflict, message, http.StatusConflict)
}

func RateLimitExceeded() *APIError {
	return New(CodeTooManyRequests, "rate limit exceeded", http.StatusTooManyRequests)
}

func QuotaExceeded(resource string) *APIError {
	return New(CodeQuotaExceeded, fmt.Sprintf("quota exceeded for %s", resource), http.StatusTooManyRequests)
}

func Internal(message string) *APIError {
	return New(CodeInternal, message, http.StatusInternalServerError)
}

func ServiceUnavailable(service string) *APIError {
	return New(CodeServiceUnavail, fmt.Sprintf("%s is unavailable", service), http.StatusServiceUnavailable)
}

// As checks if an error is an *APIError.
func As(err error) (*APIError, bool) {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

// ValidationError carries field-level validation failures.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func Validation(fields []ValidationError) *APIError {
	return New(CodeValidation, "validation failed", http.StatusBadRequest).WithDetails(fields)
}

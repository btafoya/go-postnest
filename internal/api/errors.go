package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// AppError is the unified application error type.
type AppError struct {
	Code       string      `json:"code"`
	Message    string      `json:"message"`
	Details    []FieldError `json:"details,omitempty"`
	StatusCode int         `json:"-"`
	Err        error       `json:"-"`
}

// FieldError describes a validation issue.
type FieldError struct {
	Field  string `json:"field"`
	Issue  string `json:"issue"`
	Param  string `json:"param,omitempty"`
}

func (e *AppError) Error() string { return e.Message }

// Common errors.
var (
	ErrNotFound     = &AppError{Code: "not_found", Message: "Resource not found", StatusCode: http.StatusNotFound}
	ErrUnauthorized = &AppError{Code: "unauthorized", Message: "Authentication required", StatusCode: http.StatusUnauthorized}
	ErrForbidden    = &AppError{Code: "forbidden", Message: "Access denied", StatusCode: http.StatusForbidden}
	ErrValidation   = &AppError{Code: "validation_failed", Message: "Request validation failed", StatusCode: http.StatusBadRequest}
	ErrConflict     = &AppError{Code: "conflict", Message: "Resource conflict", StatusCode: http.StatusConflict}
	ErrRateLimited  = &AppError{Code: "rate_limited", Message: "Too many requests", StatusCode: http.StatusTooManyRequests}
	ErrInternal     = &AppError{Code: "internal_error", Message: "Internal server error", StatusCode: http.StatusInternalServerError}
)

// NewValidationError creates a validation error with field details.
func NewValidationError(details []FieldError) *AppError {
	return &AppError{Code: "validation_failed", Message: "Request validation failed", Details: details, StatusCode: http.StatusBadRequest}
}

// WriteError writes a JSON error response.
func WriteError(w http.ResponseWriter, err error) {
	var appErr *AppError
	if !As(err, &appErr) {
		slog.Error("unhandled error mapped to 500", "err", err)
		appErr = ErrInternal
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appErr.StatusCode)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": appErr})
}

// As is a wrapper around errors.As.
func As(err error, target any) bool {
	return errors.As(err, target)
}

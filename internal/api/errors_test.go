package api

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestAs_Unwrapped(t *testing.T) {
	inner := ErrNotFound
	wrapped := fmt.Errorf("wrapped: %w", inner)

	var target *AppError
	if !As(wrapped, &target) {
		t.Fatal("expected As to match wrapped AppError")
	}
	if target.Code != "not_found" {
		t.Errorf("code = %q, want not_found", target.Code)
	}
}

func TestAs_Direct(t *testing.T) {
	var target *AppError
	if !As(ErrValidation, &target) {
		t.Fatal("expected As to match direct AppError")
	}
	if target.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", target.StatusCode, http.StatusBadRequest)
	}
}

func TestAs_NonAppError(t *testing.T) {
	err := errors.New("plain error")
	var target *AppError
	if As(err, &target) {
		t.Fatal("expected As to NOT match plain error")
	}
}

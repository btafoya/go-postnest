package admin

import (
	"errors"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/go-postnest/postnest/internal/api"
)

func TestMapValidationErrors_Nil(t *testing.T) {
	var got []api.FieldError = mapValidationErrors(nil)
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestMapValidationErrors_Single(t *testing.T) {
	type testReq struct {
		Name string `validate:"required"`
	}
	var req testReq
	err := validate.Struct(req)
	if err == nil {
		t.Fatal("expected validation error")
	}
	got := mapValidationErrors(err)
	if len(got) != 1 {
		t.Fatalf("expected 1 field error, got %d", len(got))
	}
	if got[0].Field != "Name" {
		t.Errorf("field = %q, want Name", got[0].Field)
	}
	if got[0].Issue != "required" {
		t.Errorf("issue = %q, want required", got[0].Issue)
	}
}

func TestMapValidationErrors_NonValidationError(t *testing.T) {
	got := mapValidationErrors(errors.New("some error"))
	if got != nil {
		t.Fatalf("expected nil for non-validation error, got %v", got)
	}
}

func TestValidate_InstanceNotNil(t *testing.T) {
	if validate == nil {
		t.Fatal("expected validate to be initialized")
	}
}

func TestDomainNameValidator(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", false},
		{"bad double dot", "bad..domain", false},
		{"valid domain", "example.com", true},
		{"valid subdomain", "sub.example.com", true},
		{"invalid chars", "exam*ple.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type testReq struct {
				Name string `validate:"domainname"`
			}
			err := validate.Struct(testReq{Name: tt.input})
			var ve validator.ValidationErrors
			if errors.As(err, &ve) {
				if tt.want {
					t.Fatalf("expected pass for %q, got validation error", tt.input)
				}
				return
			}
			if err != nil && !errors.As(err, &ve) {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.want && err == nil {
				t.Fatalf("expected fail for %q, got pass", tt.input)
			}
		})
	}
}

func TestMapValidationErrors_Multiple(t *testing.T) {
	type multiReq struct {
		Name  string `validate:"required"`
		Email string `validate:"required,email"`
	}
	var req multiReq
	err := validate.Struct(req)
	if err == nil {
		t.Fatal("expected validation error")
	}
	got := mapValidationErrors(err)
	if len(got) != 2 {
		t.Fatalf("expected 2 field errors, got %d", len(got))
	}
	for _, fe := range got {
		if fe.Field != "Name" && fe.Field != "Email" {
			t.Errorf("unexpected field %q", fe.Field)
		}
		if fe.Issue == "" {
			t.Error("expected non-empty issue")
		}
	}
}

func TestMapValidationErrors_Wrapped(t *testing.T) {
	type testReq struct {
		Name string `validate:"required"`
	}
	var req testReq
	inner := validate.Struct(req)
	if inner == nil {
		t.Fatal("expected validation error")
	}
	wrapped := errors.New("wrapper: " + inner.Error()) // not a standard wrap, let's wrap properly
	// Use fmt.Errorf or errors.Join
	// Actually let's use a custom wrapper
	wrapped = &wrapErr{err: inner}
	got := mapValidationErrors(wrapped)
	if len(got) != 1 {
		t.Fatalf("expected 1 field error from wrapped, got %d", len(got))
	}
}

type wrapErr struct {
	err error
}

func (w *wrapErr) Error() string { return w.err.Error() }
func (w *wrapErr) Unwrap() error { return w.err }

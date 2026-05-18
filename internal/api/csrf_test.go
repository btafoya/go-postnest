package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCSRF_SafeMethodsBypass(t *testing.T) {
	called := false
	h := CSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/messages", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if !called {
		t.Fatal("GET should bypass CSRF")
	}
}

func TestCSRF_LoginBypass(t *testing.T) {
	called := false
	h := CSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if !called {
		t.Fatal("login should bypass CSRF")
	}
}

func TestCSRF_MissingCookieRejected(t *testing.T) {
	h := CSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/drafts", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestCSRF_MismatchRejected(t *testing.T) {
	h := CSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/drafts", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "secret"})
	req.Header.Set("X-CSRF-Token", "wrong")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestCSRF_MatchAccepted(t *testing.T) {
	called := false
	h := CSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/drafts", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "matchtoken"})
	req.Header.Set("X-CSRF-Token", "matchtoken")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if !called || rr.Code != http.StatusOK {
		t.Fatalf("matching token should pass; called=%v code=%d", called, rr.Code)
	}
}

func TestNewCSRFToken_Unique(t *testing.T) {
	a := NewCSRFToken()
	b := NewCSRFToken()
	if a == b {
		t.Fatal("tokens must be unique")
	}
}

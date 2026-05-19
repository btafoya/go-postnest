package admin

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/go-postnest/postnest/internal/certmanager"
	"github.com/go-postnest/postnest/internal/crypto"
)

type mockTLSMgr struct {
	reloadCalls int
	renewCalls  int
	reloadErr   error
}

func (m *mockTLSMgr) Status() certmanager.Status {
	return certmanager.Status{Domains: []string{"mail.example.com"}, DNSProvider: "cloudflare"}
}
func (m *mockTLSMgr) Reload(cfg certmanager.Config) error { m.reloadCalls++; return m.reloadErr }
func (m *mockTLSMgr) ForceRenew() error                   { m.renewCalls++; return nil }

func testCipher(t *testing.T) *crypto.Cipher {
	t.Helper()
	k := make([]byte, 32)
	rand.Read(k)
	c, err := crypto.NewCipher(base64.StdEncoding.EncodeToString(k))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func tlsHandler(t *testing.T, store Store) (*Handler, *mockTLSMgr) {
	mgr := &mockTLSMgr{}
	h := newTestHandler(store)
	h.WithTLS(mgr, testCipher(t), func() error { mgr.reloadCalls++; return nil })
	return h, mgr
}

func doReq(t *testing.T, h *Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestTLSUnavailableWithoutManager(t *testing.T) {
	h := newTestHandler(&mockStore{})
	w := doReq(t, h, http.MethodGet, "/admin/api/v1/tls/status", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d want 503", w.Code)
	}
}

func TestTLSStatus(t *testing.T) {
	h, _ := tlsHandler(t, &mockStore{})
	w := doReq(t, h, http.MethodGet, "/admin/api/v1/tls/status", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d want 200: %s", w.Code, w.Body.String())
	}
}

func TestTLSConfigRoundTripMasksSecrets(t *testing.T) {
	store := &mockStore{}
	h, _ := tlsHandler(t, store)

	w := doReq(t, h, http.MethodPut, "/admin/api/v1/tls/config", putTLSConfigReq{
		Email:       "admin@x.com",
		Directory:   "staging",
		DNSProvider: "cloudflare",
		Credentials: map[string]string{"CLOUDFLARE_DNS_API_TOKEN": "supersecret"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("put got %d: %s", w.Code, w.Body.String())
	}
	if store.lastSetACMECreds == "" || bytes.Contains([]byte(store.lastSetACMECreds), []byte("supersecret")) {
		t.Fatalf("credentials not encrypted at rest: %q", store.lastSetACMECreds)
	}

	w = doReq(t, h, http.MethodGet, "/admin/api/v1/tls/config", nil)
	if bytes.Contains(w.Body.Bytes(), []byte("supersecret")) {
		t.Fatal("GET leaked plaintext secret")
	}
	var resp struct {
		Config tlsConfigDTO `json:"config"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Config.CredsSet["CLOUDFLARE_DNS_API_TOKEN"] {
		t.Fatal("expected creds_set flag true for stored token")
	}
}

func TestTLSConfigRejectsBadProvider(t *testing.T) {
	h, _ := tlsHandler(t, &mockStore{})
	w := doReq(t, h, http.MethodPut, "/admin/api/v1/tls/config", putTLSConfigReq{
		Enabled: true, Email: "a@b.com", Directory: "staging", DNSProvider: "azure",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d want 400", w.Code)
	}
}

func TestTLSAddDomainTriggersReload(t *testing.T) {
	store := &mockStore{}
	h, _ := tlsHandler(t, store)
	w := doReq(t, h, http.MethodPost, "/admin/api/v1/tls/domains", map[string]string{"domain": "imap.example.com"})
	if w.Code != http.StatusCreated {
		t.Fatalf("got %d want 201: %s", w.Code, w.Body.String())
	}
	if len(store.acmeDomains) != 1 {
		t.Fatalf("expected 1 domain stored, got %d", len(store.acmeDomains))
	}
}

func TestTLSForceRenew(t *testing.T) {
	h, mgr := tlsHandler(t, &mockStore{})
	w := doReq(t, h, http.MethodPost, "/admin/api/v1/tls/renew", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d want 200", w.Code)
	}
	if mgr.renewCalls != 1 {
		t.Fatalf("expected ForceRenew called once, got %d", mgr.renewCalls)
	}
}

func TestTLSProvidersCurated(t *testing.T) {
	h, _ := tlsHandler(t, &mockStore{})
	w := doReq(t, h, http.MethodGet, "/admin/api/v1/tls/providers", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d want 200", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("namesilo")) {
		t.Fatal("expected namesilo in provider list")
	}
}

func TestTLSEnableToggle(t *testing.T) {
	store := &mockStore{}
	h, mgr := tlsHandler(t, store)

	// Enable ACME
	w := doReq(t, h, http.MethodPut, "/admin/api/v1/tls/config", putTLSConfigReq{
		Enabled: true, Email: "admin@x.com", Directory: "staging", DNSProvider: "cloudflare",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("enable got %d: %s", w.Code, w.Body.String())
	}
	if !store.acmeConfig.Enabled {
		t.Fatal("expected ACME enabled in store")
	}
	if mgr.reloadCalls != 1 {
		t.Fatalf("expected reload called once, got %d", mgr.reloadCalls)
	}

	// Disable ACME
	w = doReq(t, h, http.MethodPut, "/admin/api/v1/tls/config", putTLSConfigReq{
		Enabled: false, Email: "admin@x.com", Directory: "staging", DNSProvider: "cloudflare",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("disable got %d: %s", w.Code, w.Body.String())
	}
	if store.acmeConfig.Enabled {
		t.Fatal("expected ACME disabled in store")
	}
}

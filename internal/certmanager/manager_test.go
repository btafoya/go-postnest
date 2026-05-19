package certmanager

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-acme/lego/v4/lego"
)

// seedAccount writes a fake ACME registration so applyConfig does not hit
// the live ACME server during unit tests.
func seedAccount(t *testing.T, cfg Config) {
	t.Helper()
	caDirURL := lego.LEDirectoryStaging
	if cfg.Directory == "production" {
		caDirURL = lego.LEDirectoryProduction
	}
	dir := filepath.Join(cfg.CertDir, "accounts", hashString(caDirURL+"|"+cfg.Email))
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatal(err)
	}
	reg := `{"uri":"https://example.test/acme/acct/1","body":{"status":"valid"}}`
	if err := os.WriteFile(filepath.Join(dir, "account.json"), []byte(reg), 0600); err != nil {
		t.Fatal(err)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func baseCfg(t *testing.T) Config {
	cfg := Config{
		Email:          "admin@example.com",
		Domains:        []string{"mail.example.com", "imap.example.com"},
		Directory:      "staging",
		CertDir:        t.TempDir(),
		DNSProvider:    "cloudflare",
		DNSCredentials: map[string]string{"CLOUDFLARE_DNS_API_TOKEN": "fake"},
	}
	seedAccount(t, cfg)
	return cfg
}

func TestNewManagerValidation(t *testing.T) {
	// Empty email creates a disabled manager.
	cfg := baseCfg(t)
	cfg.Email = ""
	if _, err := NewManager(cfg, testLogger()); err != nil {
		t.Fatalf("expected disabled manager for missing email, got error: %v", err)
	}

	// Active manager with no domains is allowed (pending domain configuration).
	cfg = baseCfg(t)
	cfg.Domains = nil
	if _, err := NewManager(cfg, testLogger()); err != nil {
		t.Fatalf("expected manager with no domains, got error: %v", err)
	}

	// Unsupported provider is still rejected.
	cfg = baseCfg(t)
	cfg.DNSProvider = "azure"
	if _, err := NewManager(cfg, testLogger()); err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestNormalizedDomainsDedupSort(t *testing.T) {
	c := Config{Domains: []string{"B.example.com", "a.example.com", " a.example.com ", ""}}
	got := c.normalizedDomains()
	want := []string{"a.example.com", "b.example.com"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestCertPathTracksDomainSet(t *testing.T) {
	m, err := NewManager(baseCfg(t), testLogger())
	if err != nil {
		t.Fatal(err)
	}
	p1 := m.certPath

	cfg := m.cfg
	cfg.Domains = append(cfg.Domains, "smtp.example.com")
	if err := m.applyConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if m.certPath == p1 {
		t.Fatal("cert path must change when domain set changes")
	}
}

func TestStatusNoCert(t *testing.T) {
	m, err := NewManager(baseCfg(t), testLogger())
	if err != nil {
		t.Fatal(err)
	}
	st := m.Status()
	if st.HasCert {
		t.Fatal("expected HasCert=false with no certificate")
	}
	if len(st.Domains) != 2 || st.DNSProvider != "cloudflare" {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestReloadRejectsBadConfig(t *testing.T) {
	m, err := NewManager(baseCfg(t), testLogger())
	if err != nil {
		t.Fatal(err)
	}
	bad := baseCfg(t)
	bad.DNSProvider = "azure"
	if err := m.Reload(bad); err == nil {
		t.Fatal("expected reload to reject unsupported provider")
	}
	// Original config must be retained on failed reload.
	if m.cfg.DNSProvider != "cloudflare" {
		t.Fatalf("config mutated on failed reload: %s", m.cfg.DNSProvider)
	}
}

func TestNeedsRenewalNoCert(t *testing.T) {
	m, err := NewManager(baseCfg(t), testLogger())
	if err != nil {
		t.Fatal(err)
	}
	m.cfg.RenewBefore = 720 * time.Hour
	if !m.needsRenewal() {
		t.Fatal("expected needsRenewal=true when no cert loaded")
	}
}

package certmanager

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
)

// Config holds ACME configuration. Domains are issued as a single SAN
// certificate. DNSCredentials are already-decrypted provider credentials.
type Config struct {
	Email          string
	Domains        []string
	Directory      string // "staging" or "production"
	CertDir        string
	DNSProvider    string
	DNSCredentials map[string]string
	RenewInterval  time.Duration
	RenewBefore    time.Duration
}

func (c Config) normalizedDomains() []string {
	d := make([]string, 0, len(c.Domains))
	seen := make(map[string]struct{}, len(c.Domains))
	for _, x := range c.Domains {
		x = strings.ToLower(strings.TrimSpace(x))
		if x == "" {
			continue
		}
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		d = append(d, x)
	}
	sort.Strings(d)
	return d
}

// User implements registration.User interface.
type User struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *User) GetEmail() string                        { return u.Email }
func (u *User) GetRegistration() *registration.Resource { return u.Registration }
func (u *User) GetPrivateKey() crypto.PrivateKey        { return u.key }

// Status is an immutable snapshot of certificate state for the admin UI.
type Status struct {
	Domains       []string  `json:"domains"`
	Issuer        string    `json:"issuer"`
	NotBefore     time.Time `json:"not_before"`
	NotAfter      time.Time `json:"not_after"`
	DaysRemaining int       `json:"days_remaining"`
	Directory     string    `json:"directory"`
	DNSProvider   string    `json:"dns_provider"`
	HasCert       bool      `json:"has_cert"`
}

// Manager handles ACME certificate lifecycle for a SAN certificate.
type Manager struct {
	logger *slog.Logger

	mu         sync.RWMutex // guards cfg, client, user, paths, in-flight obtain
	cfg        Config
	domains    []string
	client     *lego.Client
	user       *User
	certPath   string
	keyPath    string
	accountDir string

	current atomic.Pointer[tls.Certificate]

	renewing    atomic.Bool
	lastRenewMu sync.Mutex
	lastRenewErr error

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewManager creates a certificate manager. It validates configuration,
// loads or generates the ACME account key, and prepares the lego client.
// No network I/O is performed. An empty Config (no email) creates a manager
// in disabled state that can be activated later via Reload.
func NewManager(cfg Config, logger *slog.Logger) (*Manager, error) {
	m := &Manager{
		logger: logger,
		stopCh: make(chan struct{}),
	}
	if cfg.Email != "" {
		if err := m.applyConfig(cfg); err != nil {
			return nil, err
		}
	} else {
		m.cfg = cfg
	}
	return m, nil
}

// applyConfig validates cfg, (re)builds the lego client, registration and
// DNS provider, and recomputes cert/key paths. Caller must hold m.mu, or
// call before the manager is shared (NewManager).
func (m *Manager) applyConfig(cfg Config) error {
	if cfg.Email == "" {
		return fmt.Errorf("ACME email is required")
	}
	domains := cfg.normalizedDomains()
	if cfg.CertDir == "" {
		cfg.CertDir = "/var/lib/postnest/certs"
	}
	if cfg.RenewInterval == 0 {
		cfg.RenewInterval = 24 * time.Hour
	}
	if cfg.RenewBefore == 0 {
		cfg.RenewBefore = 720 * time.Hour // 30 days
	}
	if cfg.DNSProvider == "" {
		cfg.DNSProvider = "cloudflare"
	}
	if !SupportedProvider(cfg.DNSProvider) {
		return fmt.Errorf("unsupported DNS provider: %s", cfg.DNSProvider)
	}

	if err := os.MkdirAll(cfg.CertDir, 0750); err != nil {
		return fmt.Errorf("create cert dir: %w", err)
	}

	caDirURL := lego.LEDirectoryStaging
	if cfg.Directory == "production" {
		caDirURL = lego.LEDirectoryProduction
	}

	accountDir := filepath.Join(cfg.CertDir, "accounts", hashString(caDirURL+"|"+cfg.Email))
	// Cert files are keyed by the (sorted) domain set so changing the set
	// does not silently reuse a stale certificate.
	setHash := hashString(strings.Join(domains, ","))

	m.cfg = cfg
	m.domains = domains
	m.certPath = filepath.Join(cfg.CertDir, setHash+".crt")
	m.keyPath = filepath.Join(cfg.CertDir, setHash+".key")
	m.accountDir = accountDir

	key, err := m.loadOrCreateAccountKey()
	if err != nil {
		return fmt.Errorf("load or create ACME account key: %w", err)
	}
	user := &User{Email: cfg.Email, key: key}

	newLegoClient := func(u *User) (*lego.Client, error) {
		lc := lego.NewConfig(u)
		lc.CADirURL = caDirURL
		lc.Certificate.KeyType = certcrypto.EC256
		return lego.NewClient(lc)
	}

	// Phase 1: a client with no registration. lego bakes the JWS Key ID
	// into the API core at construction time from User.GetRegistration(),
	// so a client built before the account is resolved signs every request
	// with the JWK instead of the kid. That is only valid for new-account;
	// new-order is then rejected with "No Key ID in JWS header". This
	// bootstrap client is used solely for account registration/resolution,
	// which authenticate via JWK.
	bootClient, err := newLegoClient(user)
	if err != nil {
		return fmt.Errorf("create lego client: %w", err)
	}

	reg, err := m.loadOrCreateRegistration(bootClient)
	if err != nil {
		return fmt.Errorf("load or create ACME registration: %w", err)
	}
	user.Registration = reg

	// Phase 2: rebuild the client now that the user carries a registration
	// so lego binds the account URL as the JWS Key ID for all subsequent
	// requests (new-order, finalize, etc.).
	client, err := newLegoClient(user)
	if err != nil {
		return fmt.Errorf("create lego client: %w", err)
	}
	m.client = client
	m.user = user

	provider, err := BuildProvider(cfg.DNSProvider, cfg.DNSCredentials)
	if err != nil {
		return err
	}
	if err := m.client.Challenge.SetDNS01Provider(provider,
		dns01.AddRecursiveNameservers([]string{"1.1.1.1:53", "8.8.8.8:53"}),
		dns01.PropagationWait(2*time.Minute, false),
	); err != nil {
		return fmt.Errorf("set DNS-01 provider: %w", err)
	}
	return nil
}

// Start loads an existing certificate or obtains a new one, then starts
// the background renewal worker. When the manager is disabled (no email
// configured) this is a no-op apart from starting the worker.
func (m *Manager) Start(ctx context.Context) error {
	if m.client != nil && len(m.domains) > 0 {
		if err := m.loadCertificate(); err == nil {
			m.logger.Info("loaded existing certificate",
				"domains", m.domains,
				"path", m.certPath,
			)
		} else {
			m.logger.Info("no valid existing certificate, obtaining new one",
				"domains", m.domains,
				"error", err,
			)
			if err := m.obtainCertificate(); err != nil {
				return fmt.Errorf("obtain certificate: %w", err)
			}
		}
	} else if m.client != nil {
		m.logger.Info("ACME configured but no domains yet, waiting for domains")
	} else {
		m.logger.Info("ACME manager started in disabled state")
	}

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.renewalLoop(ctx)
	}()

	return nil
}

// Stop signals the renewal worker to stop and waits for it to exit.
func (m *Manager) Stop() error {
	close(m.stopCh)
	m.wg.Wait()
	return nil
}

// Reload swaps in a new configuration at runtime. The lego client and DNS
// provider are rebuilt. If the domain set changed, a fresh certificate is
// obtained immediately; otherwise the existing certificate is retained.
// Transitioning to disabled (empty email) clears the active certificate.
func (m *Manager) Reload(cfg Config) error {
	// Transition to disabled.
	if cfg.Email == "" {
		m.mu.Lock()
		m.cfg = cfg
		m.domains = nil
		m.client = nil
		m.user = nil
		m.certPath = ""
		m.keyPath = ""
		m.accountDir = ""
		m.current.Store(nil)
		m.mu.Unlock()
		m.logger.Info("ACME disabled, certificate cleared")
		return nil
	}

	m.mu.Lock()
	oldDomains := strings.Join(m.domains, ",")
	if err := m.applyConfig(cfg); err != nil {
		m.mu.Unlock()
		return err
	}
	newDomains := strings.Join(m.domains, ",")
	domainsChanged := oldDomains != newDomains
	m.mu.Unlock()

	if domainsChanged {
		m.logger.Info("ACME domain set changed, obtaining new certificate",
			"old", oldDomains, "new", newDomains)
		if err := m.obtainCertificate(); err != nil {
			return fmt.Errorf("obtain after reload: %w", err)
		}
	} else if m.current.Load() == nil {
		if len(m.domains) > 0 {
			if err := m.loadCertificate(); err == nil {
				m.logger.Info("ACME enabled, loaded existing certificate")
				return nil
			}
			if err := m.obtainCertificate(); err != nil {
				return fmt.Errorf("obtain after reload: %w", err)
			}
		}
	}
	return nil
}

// GetCertificate returns the current tls.Certificate. It is safe for
// concurrent use and intended for tls.Config.GetCertificate.
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cert := m.current.Load()
	if cert == nil {
		return nil, fmt.Errorf("no certificate available")
	}
	return cert, nil
}

// CurrentCertificate returns the currently loaded certificate for inspection.
func (m *Manager) CurrentCertificate() *tls.Certificate {
	return m.current.Load()
}

// Status returns a snapshot of certificate state for the admin UI.
func (m *Manager) Status() Status {
	m.mu.RLock()
	st := Status{
		Domains:     append([]string(nil), m.domains...),
		Directory:   m.cfg.Directory,
		DNSProvider: m.cfg.DNSProvider,
	}
	m.mu.RUnlock()

	cert := m.current.Load()
	if cert == nil || len(cert.Certificate) == 0 {
		return st
	}
	x, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return st
	}
	st.HasCert = true
	st.Issuer = x.Issuer.CommonName
	st.NotBefore = x.NotBefore
	st.NotAfter = x.NotAfter
	st.DaysRemaining = int(time.Until(x.NotAfter).Hours() / 24)
	return st
}

// ForceRenew starts an asynchronous certificate renewal and returns immediately.
// Poll RenewStatus to observe progress and the final result.
func (m *Manager) ForceRenew() error {
	if !m.renewing.CompareAndSwap(false, true) {
		return fmt.Errorf("certificate renewal already in progress")
	}
	go func() {
		defer m.renewing.Store(false)
		err := m.obtainCertificate()
		m.lastRenewMu.Lock()
		m.lastRenewErr = err
		m.lastRenewMu.Unlock()
	}()
	return nil
}

// RenewStatus reports whether a renewal is currently in-flight and the error
// from the most recent attempt (nil if it succeeded or never ran).
func (m *Manager) RenewStatus() (inProgress bool, lastErr error) {
	inProgress = m.renewing.Load()
	m.lastRenewMu.Lock()
	lastErr = m.lastRenewErr
	m.lastRenewMu.Unlock()
	return inProgress, lastErr
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

func (m *Manager) loadOrCreateAccountKey() (crypto.PrivateKey, error) {
	if err := os.MkdirAll(m.accountDir, 0750); err != nil {
		return nil, err
	}

	keyPath := filepath.Join(m.accountDir, "account.key")
	data, err := os.ReadFile(keyPath)
	if err == nil {
		block, _ := pem.Decode(data)
		if block != nil {
			key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err == nil {
				return key, nil
			}
		}
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate account key: %w", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return nil, err
	}

	return privateKey, nil
}

func (m *Manager) loadOrCreateRegistration(client *lego.Client) (*registration.Resource, error) {
	regPath := filepath.Join(m.accountDir, "account.json")

	data, err := os.ReadFile(regPath)
	if err == nil {
		var reg registration.Resource
		if err := json.Unmarshal(data, &reg); err == nil && reg.URI != "" {
			return &reg, nil
		}
	}

	reg, err := client.Registration.Register(registration.RegisterOptions{
		TermsOfServiceAgreed: true,
	})
	if err != nil {
		return nil, fmt.Errorf("register ACME account: %w", err)
	}

	data, err = json.Marshal(reg)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(regPath, data, 0600); err != nil {
		return nil, err
	}

	return reg, nil
}

func (m *Manager) loadCertificate() error {
	m.mu.RLock()
	certPath, keyPath := m.certPath, m.keyPath
	want := append([]string(nil), m.domains...)
	m.mu.RUnlock()

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("parse certificate: %w", err)
	}
	if len(cert.Certificate) == 0 {
		return fmt.Errorf("certificate chain is empty")
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return fmt.Errorf("parse x509 certificate: %w", err)
	}
	if time.Now().After(x509Cert.NotAfter) {
		return fmt.Errorf("certificate expired on %s", x509Cert.NotAfter)
	}

	// Every configured domain must be covered by the SAN list.
	san := make(map[string]struct{}, len(x509Cert.DNSNames))
	for _, d := range x509Cert.DNSNames {
		san[strings.ToLower(d)] = struct{}{}
	}
	if x509Cert.Subject.CommonName != "" {
		san[strings.ToLower(x509Cert.Subject.CommonName)] = struct{}{}
	}
	for _, d := range want {
		if _, ok := san[d]; !ok {
			return fmt.Errorf("certificate missing domain %q in SAN", d)
		}
	}

	m.current.Store(&cert)
	return nil
}

func (m *Manager) obtainCertificate() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.domains) == 0 {
		return fmt.Errorf("no domains configured")
	}

	request := certificate.ObtainRequest{
		Domains: append([]string(nil), m.domains...),
		Bundle:  true,
	}

	certs, err := m.client.Certificate.Obtain(request)
	if err != nil {
		return fmt.Errorf("ACME obtain: %w", err)
	}

	if err := os.WriteFile(m.certPath, certs.Certificate, 0644); err != nil {
		return fmt.Errorf("write certificate: %w", err)
	}
	if err := os.WriteFile(m.keyPath, certs.PrivateKey, 0600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}

	cert, err := tls.X509KeyPair(certs.Certificate, certs.PrivateKey)
	if err != nil {
		return fmt.Errorf("load obtained certificate: %w", err)
	}

	m.current.Store(&cert)
	m.logger.Info("certificate obtained", "domains", m.domains)
	return nil
}

func (m *Manager) renewalLoop(ctx context.Context) {
	m.mu.RLock()
	hasConfig := m.client != nil && len(m.domains) > 0
	m.mu.RUnlock()

	if hasConfig && m.needsRenewal() {
		m.logger.Info("certificate needs renewal on startup", "domains", m.domains)
		if err := m.obtainCertificate(); err != nil {
			m.logger.Error("initial renewal attempt failed", "error", err)
		}
	}

	ticker := time.NewTicker(m.cfg.RenewInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.mu.RLock()
			hasConfig := m.client != nil && len(m.domains) > 0
			m.mu.RUnlock()
			if !hasConfig || !m.needsRenewal() {
				continue
			}
			m.logger.Info("attempting certificate renewal", "domains", m.domains)
			if err := m.obtainCertificate(); err != nil {
				m.logger.Error("certificate renewal failed", "error", err)
				continue
			}
			m.logger.Info("certificate renewed successfully", "domains", m.domains)
		}
	}
}

func (m *Manager) needsRenewal() bool {
	cert := m.current.Load()
	if cert == nil || len(cert.Certificate) == 0 {
		return true
	}
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return true
	}
	return time.Now().Add(m.cfg.RenewBefore).After(x509Cert.NotAfter)
}

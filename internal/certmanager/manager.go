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
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/registration"
)

// Config holds ACME configuration.
type Config struct {
	Email         string
	Domain        string
	Directory     string // "staging" or "production"
	CertDir       string
	DNSProvider   string
	RenewInterval time.Duration
	RenewBefore   time.Duration
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

// Manager handles ACME certificate lifecycle.
type Manager struct {
	cfg        Config
	logger     *slog.Logger
	client     *lego.Client
	user       *User
	certPath   string
	keyPath    string
	accountDir string

	current atomic.Pointer[tls.Certificate]
	mu      sync.RWMutex

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewManager creates a certificate manager. It validates configuration,
// loads or generates the ACME account key, and prepares the lego client.
// No network I/O is performed.
func NewManager(cfg Config, logger *slog.Logger) (*Manager, error) {
	if cfg.Email == "" {
		return nil, fmt.Errorf("ACME email is required")
	}
	if cfg.Domain == "" {
		return nil, fmt.Errorf("ACME domain is required")
	}
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

	// Ensure certificate directory exists.
	if err := os.MkdirAll(cfg.CertDir, 0750); err != nil {
		return nil, fmt.Errorf("create cert dir: %w", err)
	}

	caDirURL := lego.LEDirectoryStaging
	if cfg.Directory == "production" {
		caDirURL = lego.LEDirectoryProduction
	}

	accountDir := filepath.Join(cfg.CertDir, "accounts", hashString(caDirURL))

	m := &Manager{
		cfg:        cfg,
		logger:     logger,
		certPath:   filepath.Join(cfg.CertDir, cfg.Domain+".crt"),
		keyPath:    filepath.Join(cfg.CertDir, cfg.Domain+".key"),
		accountDir: accountDir,
		stopCh:     make(chan struct{}),
	}

	// Load or generate account key.
	key, err := m.loadOrCreateAccountKey()
	if err != nil {
		return nil, fmt.Errorf("load or create ACME account key: %w", err)
	}

	user := &User{Email: cfg.Email, key: key}

	// Create lego client.
	legoCfg := lego.NewConfig(user)
	legoCfg.CADirURL = caDirURL
	legoCfg.Certificate.KeyType = certcrypto.EC256

	client, err := lego.NewClient(legoCfg)
	if err != nil {
		return nil, fmt.Errorf("create lego client: %w", err)
	}
	m.client = client

	// Load or create registration.
	reg, err := m.loadOrCreateRegistration(user)
	if err != nil {
		return nil, fmt.Errorf("load or create ACME registration: %w", err)
	}
	user.Registration = reg
	m.user = user

	// Set up DNS provider.
	if err := m.setupDNSProvider(); err != nil {
		return nil, err
	}

	return m, nil
}

// Start loads an existing certificate or obtains a new one, then starts
// the background renewal worker. It blocks until the initial certificate
// is available.
func (m *Manager) Start(ctx context.Context) error {
	if err := m.loadCertificate(); err == nil {
		m.logger.Info("loaded existing certificate",
			"domain", m.cfg.Domain,
			"path", m.certPath,
		)
	} else {
		m.logger.Info("no valid existing certificate, obtaining new one",
			"domain", m.cfg.Domain,
			"error", err,
		)
		if err := m.obtainCertificate(ctx); err != nil {
			return fmt.Errorf("obtain certificate: %w", err)
		}
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

func (m *Manager) loadOrCreateRegistration(user *User) (*registration.Resource, error) {
	regPath := filepath.Join(m.accountDir, "account.json")

	data, err := os.ReadFile(regPath)
	if err == nil {
		var reg registration.Resource
		if err := json.Unmarshal(data, &reg); err == nil && reg.URI != "" {
			return &reg, nil
		}
	}

	reg, err := m.client.Registration.Register(registration.RegisterOptions{
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

func (m *Manager) setupDNSProvider() error {
	switch m.cfg.DNSProvider {
	case "cloudflare":
		provider, err := cloudflare.NewDNSProvider()
		if err != nil {
			return fmt.Errorf("create cloudflare DNS provider: %w", err)
		}
		return m.client.Challenge.SetDNS01Provider(provider,
			dns01.AddRecursiveNameservers([]string{"1.1.1.1:53", "8.8.8.8:53"}),
			dns01.PropagationWait(2*time.Minute, false),
		)
	default:
		return fmt.Errorf("unsupported DNS provider: %s", m.cfg.DNSProvider)
	}
}

func (m *Manager) loadCertificate() error {
	certPEM, err := os.ReadFile(m.certPath)
	if err != nil {
		return err
	}
	keyPEM, err := os.ReadFile(m.keyPath)
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

	// Verify domain matches.
	found := x509Cert.Subject.CommonName == m.cfg.Domain
	if !found {
		for _, san := range x509Cert.DNSNames {
			if san == m.cfg.Domain {
				found = true
				break
			}
		}
	}
	if !found {
		return fmt.Errorf("certificate domain mismatch")
	}

	m.current.Store(&cert)
	return nil
}

func (m *Manager) obtainCertificate(ctx context.Context) error {
	request := certificate.ObtainRequest{
		Domains: []string{m.cfg.Domain},
		Bundle:  true,
	}

	// lego's Obtain does not accept context directly; rely on internal
	// timeouts from the DNS provider.
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
	m.logger.Info("certificate obtained", "domain", m.cfg.Domain)
	return nil
}

func (m *Manager) renewalLoop(ctx context.Context) {
	// Immediate check on startup.
	if m.needsRenewal() {
		m.logger.Info("certificate needs renewal on startup", "domain", m.cfg.Domain)
		if err := m.renew(ctx); err != nil {
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
			if !m.needsRenewal() {
				continue
			}
			m.logger.Info("attempting certificate renewal", "domain", m.cfg.Domain)
			if err := m.renew(ctx); err != nil {
				m.logger.Error("certificate renewal failed", "error", err)
				continue
			}
			m.logger.Info("certificate renewed successfully", "domain", m.cfg.Domain)
		}
	}
}

func (m *Manager) needsRenewal() bool {
	cert := m.current.Load()
	if cert == nil {
		return true
	}
	if len(cert.Certificate) == 0 {
		return true
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return true
	}

	return time.Now().Add(m.cfg.RenewBefore).After(x509Cert.NotAfter)
}

func (m *Manager) renew(ctx context.Context) error {
	request := certificate.ObtainRequest{
		Domains: []string{m.cfg.Domain},
		Bundle:  true,
	}

	certs, err := m.client.Certificate.Obtain(request)
	if err != nil {
		return fmt.Errorf("ACME renew: %w", err)
	}

	if err := os.WriteFile(m.certPath, certs.Certificate, 0644); err != nil {
		return fmt.Errorf("write certificate: %w", err)
	}
	if err := os.WriteFile(m.keyPath, certs.PrivateKey, 0600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}

	cert, err := tls.X509KeyPair(certs.Certificate, certs.PrivateKey)
	if err != nil {
		return fmt.Errorf("load renewed certificate: %w", err)
	}

	m.current.Store(&cert)
	return nil
}

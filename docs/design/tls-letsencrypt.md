# Design: Automatic TLS for SMTP/IMAP with Let's Encrypt

## 1. Goals

- Eliminate manual certificate provisioning for embedded SMTP (465) and IMAP (993) services.
- Use `go-acme/lego` as an **in-process Go library** with **DNS-01** challenges via **Cloudflare**.
- Support **staging → production** ACME directory switching.
- **Auto-obtain** on first startup, **auto-renew** before expiry, **hot-reload** into active listeners.
- Run **implicit TLS only** (465/993) when lego-managed TLS is active; disable plaintext ports.
- **Require TLS for authentication** — `AllowInsecureAuth = false` when TLS is active.

## 2. Non-Goals

- CalDAV/CardDAV TLS (handled by external reverse proxy).
- HTTP-01 or TLS-ALPN-01 challenge support (DNS-01 only for this phase).
- Wildcard certificates (single domain only).
- Admin API endpoints for manual renewal.

## 3. Component Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     cmd/server/main.go                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐   │
│  │  HTTP Server │  │  IMAPServer  │  │   SMTPServer     │   │
│  │   (:8080)    │  │   (:993)     │  │    (:465)        │   │
│  └──────┬───────┘  └──────┬───────┘  └────────┬─────────┘   │
│         │                 │                     │             │
│         └─────────────────┴─────────────────────┘             │
│                           │                                   │
│              ┌────────────▼────────────┐                      │
│              │    tls.Config           │                      │
│              │  (GetCertificate hook)  │                      │
│              └────────────┬────────────┘                      │
│                           │                                   │
│              ┌────────────▼────────────┐                      │
│              │   CertManager             │                      │
│              │  (internal/certmanager)   │                      │
│              │                           │                      │
│              │  ┌─────────────────────┐  │                      │
│              │  │  Certificate Store    │  │  (atomic swap)       │
│              │  │  cert + key on disk   │  │                      │
│              │  └─────────────────────┘  │                      │
│              │                           │                      │
│              │  ┌─────────────────────┐  │                      │
│              │  │  Lego ACME Client   │  │                      │
│              │  │  (DNS-01/Cloudflare)│  │                      │
│              │  └─────────────────────┘  │                      │
│              │                           │                      │
│              │  ┌─────────────────────┐  │                      │
│              │  │  Renewal Worker     │  │  (background)        │
│              │  │  daily check        │  │                      │
│              │  └─────────────────────┘  │                      │
│              └───────────────────────────┘                      │
└─────────────────────────────────────────────────────────────┘
```

### New Components

| Package | Responsibility |
|---------|--------------|
| `internal/certmanager` | ACME account lifecycle, certificate obtain/renew, hot-reload, filesystem persistence. |

### Modified Components

| Package | Change |
|---------|--------|
| `internal/config` | Add `[acme]` TOML section and `ACME_*` env vars. |
| `cmd/server/main.go` | Wire `CertManager`, build dynamic `tls.Config`, start IMAPS/SMTPS only when ACME enabled. |
| `internal/imap/imap.go` | Accept `allowInsecureAuth bool`; set based on TLS presence. |
| `internal/smtp/smtp.go` | Accept `allowInsecureAuth bool`; set based on TLS presence. |
| `internal/config/template.go` | Add `[acme]` section to default template. |

## 4. Interfaces

### 4.1 CertManager

```go
package certmanager

import (
    "context"
    "crypto/tls"
)

// Manager handles ACME certificate lifecycle.
type Manager struct {
    // ... internal fields
}

// Config holds ACME configuration.
type Config struct {
    Email           string        // ACME account contact email
    Domain          string        // Domain to obtain certificate for
    Directory       string        // "staging" or "production"
    CertDir         string        // Directory to store certs + account key
    DNSProvider     string        // "cloudflare" (extensible)
    RenewInterval   time.Duration // How often to check expiry (default 24h)
    RenewBefore     time.Duration // Renew when expiry < this (default 720h = 30d)
}

// NewManager creates a certificate manager. Does not perform network I/O.
func NewManager(cfg Config, logger *slog.Logger) (*Manager, error)

// Start obtains (or loads) the certificate and starts the renewal worker.
// Blocks until the initial certificate is ready.
func (m *Manager) Start(ctx context.Context) error

// Stop signals the renewal worker to stop.
func (m *Manager) Stop() error

// GetCertificate returns the current tls.Certificate (safe for concurrent use).
// Intended for use as tls.Config.GetCertificate.
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error)

// CurrentCertificate returns the loaded certificate for inspection.
func (m *Manager) CurrentCertificate() *tls.Certificate
```

### 4.2 SMTP / IMAP Constructor Changes

```go
// internal/smtp/smtp.go
func NewServer(addr string, tlsCfg *tls.Config, allowInsecureAuth bool, ...)

// internal/imap/imap.go
func NewServer(addr string, tlsCfg *tls.Config, allowInsecureAuth bool, ...)
```

When `tlsCfg != nil` and `allowInsecureAuth == false`, authentication is only permitted over TLS.

## 5. Lego Integration Details

### 5.1 Account Persistence

To avoid re-creating ACME accounts on every restart (rate limits), the manager persists:

- `account.key` — ECDSA P-256 private key for ACME account.
- `account.json` — `registration.Resource` (URI, ToS agreement status).

These are stored in `CertDir/accounts/<ca_dir_hash>/` following lego's conventional layout.

### 5.2 Certificate Persistence

Lego's `certificate.Obtain` returns raw PEM bytes. The manager writes:

- `<domain>.crt` — certificate chain.
- `<domain>.key` — private key.

On startup, if these files exist and are valid (not expired, domain matches), the manager loads them instead of re-obtaining.

### 5.3 DNS-01 Provider Setup

```go
import (
    "github.com/go-acme/lego/v4/challenge/dns01"
    "github.com/go-acme/lego/v4/providers/dns/cloudflare"
)

provider, err := cloudflare.NewDNSProvider()
if err != nil { ... }

err = client.Challenge.SetDNS01Provider(provider,
    dns01.AddRecursiveNameservers([]string{"1.1.1.1:53"}),
    dns01.SetPropogationTimeout(2*time.Minute),
)
```

Cloudflare credentials are read from standard lego env vars (`CLOUDFLARE_DNS_API_TOKEN`), loaded by the application and passed through or left to lego's internal env lookup.

### 5.4 Renewal Logic

```go
func (m *Manager) renewalLoop(ctx context.Context) {
    ticker := time.NewTicker(m.cfg.RenewInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            if m.needsRenewal() {
                if err := m.renew(ctx); err != nil {
                    m.logger.Error("certificate renewal failed", "error", err)
                    continue
                }
                m.logger.Info("certificate renewed successfully")
            }
        }
    }
}
```

### 5.5 Hot Reload

The `tls.Config` used by SMTP and IMAP uses `GetCertificate` instead of a static `Certificates` slice:

```go
tlsConfig := &tls.Config{
    GetCertificate: certManager.GetCertificate,
}
```

When renewal completes, `GetCertificate` atomically swaps to the new `tls.Certificate` loaded from disk. Existing connections are unaffected; new connections receive the renewed certificate.

## 6. Configuration Design

### 6.1 TOML Section

```toml
[acme]
enabled       = false
email         = "admin@example.com"
domain        = "mail.example.com"
directory     = "staging"   # "staging" or "production"
cert_dir      = "./certs"
renew_interval = "24h"
renew_before   = "720h"

[acme.dns]
provider      = "cloudflare"  # extensible to other lego providers
```

### 6.2 Environment Variables

| Variable | Description |
|----------|-------------|
| `POSTNEST_ACME_ENABLED` | `true` to enable ACME-managed TLS |
| `POSTNEST_ACME_EMAIL` | ACME account email |
| `POSTNEST_ACME_DOMAIN` | Domain to obtain certificate for |
| `POSTNEST_ACME_DIRECTORY` | `staging` or `production` |
| `POSTNEST_ACME_CERT_DIR` | Storage directory |
| `POSTNEST_ACME_RENEW_INTERVAL` | Check interval |
| `POSTNEST_ACME_RENEW_BEFORE` | Renew threshold |
| `POSTNEST_ACME_DNS_PROVIDER` | DNS provider name (bootstrap only) |
| `POSTNEST_SECRET_KEY` | **Required when ACME enabled.** Base64-encoded 32-byte AES-256 key used to encrypt DNS provider credentials at rest. Generate with `openssl rand -base64 32`. |

### 6.3 Runtime Configuration (DB-backed)

ACME email, directory, DNS provider, DNS credentials, and the SAN domain
list are persisted in the database (`acme_config`, `acme_domains`, migration
`000010`) and managed at runtime from the admin UI (**Admin → TLS /
Certificates**, super-admin only). Changes hot-reload the certificate
manager; changing the domain set re-issues the SAN certificate.

The `POSTNEST_ACME_*` env/TOML values are a **one-time bootstrap**: on first
run with an empty `acme_config`, `POSTNEST_ACME_EMAIL` /
`POSTNEST_ACME_DOMAIN` (comma-separated allowed) / `POSTNEST_ACME_DNS_PROVIDER`
seed the DB. Thereafter the DB is authoritative. `POSTNEST_ACME_CERT_DIR`,
`POSTNEST_ACME_RENEW_INTERVAL`, and `POSTNEST_ACME_RENEW_BEFORE` remain
operational env settings (not exposed in the UI).

**Certificate model:** a single SAN certificate covers all configured
domains. **Supported DNS providers** (curated allowlist): `cloudflare`,
`route53`, `digitalocean`, `gcloud`, `namesilo`. Each provider's required
credential fields are described by the `/admin/api/v1/tls/providers`
endpoint and rendered dynamically in the UI; secret fields are write-only
(blank submission preserves the stored value, GET never returns plaintext).

> **Key rotation:** changing `POSTNEST_SECRET_KEY` orphans previously
> encrypted DNS credentials — they must be re-entered via the admin UI.

### 6.4 Backward Compatibility

- When `acme.enabled = false` (default), existing behavior is preserved:
  - If `tls.cert_path` / `tls.key_path` are provided, static TLS is used.
  - If no TLS config is provided, plaintext listeners run on `smtp_addr` / `imap_addr`.
- When `acme.enabled = true`, static TLS config is ignored and ACME takes over.

## 7. Application Lifecycle Integration

### 7.1 Startup Flow

```
main()
  ├─ Load Config
  ├─ If ACME enabled:
  │    ├─ Create CertManager
  │    ├─ certManager.Start(ctx)  // blocks until cert ready
  │    ├─ Build tls.Config with GetCertificate hook
  │    └─ allowInsecureAuth = false
  ├─ Else if static TLS:
  │    ├─ Load tls.Certificate from disk
  │    ├─ Build tls.Config with static cert
  │    └─ allowInsecureAuth = false
  ├─ Else:
  │    └─ allowInsecureAuth = true  // plaintext fallback
  ├─ Create IMAPS Server (:993)  if TLS available
  ├─ Create SMTPS Server (:465) if TLS available
  └─ If plaintext and no TLS:
      ├─ Create IMAP Server (:143)
      └─ Create SMTP Server (:587)
```

### 7.2 Shutdown Flow

```
Signal received
  ├─ HTTP server shutdown
  ├─ IMAP/SMTP server shutdown
  ├─ certManager.Stop()  // cancel renewal worker
  └─ DB/Redis cleanup
```

### 7.3 Decision Matrix: Which Ports to Bind

| ACME Enabled | Static TLS | Listeners Started |
|--------------|------------|-------------------|
| Yes          | —          | IMAPS (993), SMTPS (465) only |
| No           | Yes        | IMAPS (993), SMTPS (465) only |
| No           | No         | IMAP (143), SMTP (587) only |

## 8. Security Considerations

1. **ACME Account Key**: Stored with `0600` permissions. If compromised, attacker can revoke certificates but not impersonate other domains.
2. **Certificate Private Key**: Stored with `0600` permissions.
3. **Cloudflare Token**: Supplied via environment variable; never logged or persisted to config file.
4. **Auth Requirement**: `AllowInsecureAuth` is `false` whenever TLS is active. Plaintext ports are not started when TLS is configured.
5. **Staging → Production**: Explicit config switch prevents accidental staging use in production.

## 9. Error Handling & Observability

| Scenario | Behavior |
|----------|----------|
| Initial obtain fails | `Start()` returns error; server exits. |
| Renewal fails | Log error; continue serving existing cert. Retry next interval. |
| Certificate expires without renewal | Existing connections continue until process restart, but new connections will fail TLS handshake. Log critical error. |
| DNS provider API failure | Treated as obtain/renew failure; retry with backoff. |
| ACME rate limit | Log error; backoff and retry. |

### Logging Events

- `certificate_loaded` (info) — on startup when existing cert found.
- `certificate_obtained` (info) — on successful initial issuance.
- `certificate_renewed` (info) — on successful renewal.
- `renewal_failed` (error) — on renewal error.
- `certificate_expiring` (warn) — when cert is within `renew_before` and renewal not yet attempted.

## 10. Testing Strategy

1. **Unit**: Mock lego client to test `CertManager` state machine (load → obtain → renew → hot swap).
2. **Integration**: Use Let's Encrypt **Staging** against a real Cloudflare-controlled test domain.
3. **Regression**: Verify plaintext fallback still works when ACME is disabled.
4. **E2E**: Connect with `openssl s_client` to 465/993 and verify certificate chain.

## 11. Implementation Order

1. Add `internal/certmanager` package with `Manager`, `Config`, and lego integration.
2. Extend `internal/config` with `[acme]` section and env overrides.
3. Update `internal/imap/imap.go` and `internal/smtp/smtp.go` to accept `allowInsecureAuth`.
4. Refactor `cmd/server/main.go`:
   - Wire `CertManager` when ACME enabled.
   - Use `GetCertificate` TLS hook.
   - Start only TLS or only plaintext listeners based on config.
5. Update `internal/config/template.go` with new ACME section.
6. Add integration tests with staging CA.

---

## Open Questions for User Confirmation

1. **Cert directory default**: Is `./certs` in the working directory acceptable, or should it default to `/var/lib/postnest/certs`?
2. **HTTP server TLS**: Should the HTTP server (`:8080`) also use the ACME certificate, or remain HTTP-only (behind reverse proxy)?
3. **Log level for cert events**: Should certificate lifecycle events use a dedicated logger namespace (e.g., `logger.With("component", "certmanager")`)?

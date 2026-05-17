# Postnest Code Conventions

## Module & Language Versions

- **Module**: `github.com/go-postnest/postnest`
- **Go**: `1.25.0` (`go.mod`)
- **Frontend**: React `19.0.0`, Vite `6.0.0`, React Router DOM `7.0.0`, Tailwind CSS `3.4.15`

## Repository Layout

```
cmd/<binary>/main.go          # one main per binary: server, worker, migrate, admin, webui
internal/<package>/           # flat internal packages; no further sub-packages
  *.go                        # implementation files
  *_test.go                   # test files co-located with code
web/                          # Vite + React frontend
  src/
    api.js                    # single axios client exporting API functions
    components/*.jsx            # React functional components
    sse.js                    # ES6 SSE client class
    styles/index.css           # Tailwind directives + custom layers
```

## Go Code Style

### Imports
Standard library imports come first, followed by a blank line, then third-party imports. Group the project’s own imports last.

```go
import (
	"context"
	"fmt"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/go-postnest/postnest/internal/api"
	"github.com/go-postnest/postnest/internal/models"
)
```

### Naming

| Construct | Convention | Example |
|---|---|---|
| Exported types/functions | PascalCase | `type Service struct`, `NewServer(...)` |
| Unexported helpers | camelCase | `hashPassword`, `extractToken` |
| Constructor functions | `New<T>` | `NewService`, `NewPGStore`, `NewHandler` |
| Interface names | Noun ending in `-er` or role name | `Store`, `Processor`, `DomainLister` |
| Test files | `<file>_test.go` | `errors_test.go` |
| Context param | `ctx context.Context` | always first parameter |
| HTTP request/response | `r *http.Request`, `w http.ResponseWriter` | standard Go idiom |
| Error sentinel variables | `Err<Name>` | `ErrNotFound` |
| Acronyms | All-caps in names | `TLSConfig`, `SMTPAddr`, `IMAPAddr` |

### Struct Tags
Use struct tags for JSON serialization, TOML config mapping, and database scan mapping where applicable:

```go
type rawServer struct {
    HTTPAddr     string        `toml:"http_addr"`
    ReadTimeout  time.Duration `toml:"read_timeout"`
}
```

### Constants
Group related constants in `const` blocks. Prefer typed string constants for enum-like values.

```go
const (
    queueJobs    = "postnest:jobs"
    queueDelayed = "postnest:delayed"
    queueDead    = "postnest:dead"
)
```

### Context Keys
Define a custom string type for context keys to avoid collisions with other packages:

```go
// `internal/api/middleware.go`
type ctxKey string

const (
    ctxKeyUser      ctxKey = "user"
    ctxKeyDomainID  ctxKey = "domain_id"
    ctxKeyRequestID ctxKey = "request_id"
)
```

### Time Handling
Always store and compare times in UTC:

```go
user.CreatedAt = time.Now().UTC()
```

### ID Generation
Use UUID v7 via `github.com/google/uuid`:

```go
id := uuid.Must(uuid.NewV7())
```

## Error Handling

### Application Error Type
The project defines a unified error type in `internal/api/errors.go`:

```go
// AppError is the unified application error type.
type AppError struct {
    Code       string      `json:"code"`
    Message    string      `json:"message"`
    Details    []FieldError `json:"details,omitempty"`
    StatusCode int         `json:"-"`
    Err        error       `json:"-"`
}
```

Common sentinel errors are declared as package variables:

```go
var (
    ErrNotFound     = &AppError{Code: "not_found", ...}
    ErrUnauthorized = &AppError{Code: "unauthorized", ...}
    ErrForbidden    = &AppError{Code: "forbidden", ...}
    ErrValidation   = &AppError{Code: "validation_failed", ...}
)
```

### Writing JSON Errors
All HTTP error responses go through `api.WriteError`, which serializes the `AppError` to JSON and sets the correct status code:

```go
func WriteError(w http.ResponseWriter, err error) {
    var appErr *AppError
    if !As(err, &appErr) {
        appErr = ErrInternal
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(appErr.StatusCode)
    _ = json.NewEncoder(w).Encode(map[string]any{"error": appErr})
}
```

### Wrapping and Matching
The project wraps standard errors and matches with `errors.As` via a thin helper:

```go
func As(err error, target any) bool {
    return errors.As(err, target)
}
```

Example usage in tests and middleware:

```go
wrapped := fmt.Errorf("wrapped: %w", ErrNotFound)
var target *AppError
if As(wrapped, &target) { ... }
```

### Database Errors
Database queries return raw `pgx` errors. `pgx.ErrNoRows` is checked explicitly to translate into application-level errors rather than leaking driver details:

```go
if err := row.Scan(&u.ID, ...); err != nil {
    if err == pgx.ErrNoRows {
        return nil, fmt.Errorf("invalid credentials")
    }
    return nil, err
}
```

## Logging

### Logger Setup
The project uses `log/slog` with a structured JSON handler configured in `internal/logger/logger.go`:

```go
func New() *slog.Logger {
    return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))
}
```

### Logging Patterns
- Pass `*slog.Logger` as a constructor dependency rather than using the global default.
- Log with structured key-value pairs, not string interpolation.
- Log errors at `Error` level; startup events at `Info` level; TLS downgrade warnings at `Warn` level.

```go
logger.Info("http server starting", "addr", cfg.HTTPAddr)
logger.Error("failed to connect to postgres", "error", err)
logger.Warn("running without TLS with insecure auth allowed", "imap", imapAddr, "smtp", smtpAddr)
```

### Request Logging
`internal/api/middleware.go` provides a `StructuredLogger` middleware that logs method, path, duration, and request ID for every HTTP request.

## Configuration

### Dual-Source Config
Configuration is loaded from **TOML files** and overridden by **environment variables**.

- Default config path: `/etc/postnest/postnest.conf`
- Override via `POSTNEST_CONFIG_PATH`
- Environment variable format: `POSTNEST_<SECTION>_<KEY>` (e.g., `POSTNEST_DATABASE_DSN`)

### Legacy Compatibility
A legacy environment map (`internal/config/loader.go`) preserves backward compatibility for older variable names (e.g., `POSTGRES_DSN` maps to `POSTNEST_DATABASE_DSN`).

### Validation
The loader validates required fields after merging sources and returns an explicit error listing missing values:

```go
var missing []string
if cfg.PostgresDSN == "" {
    missing = append(missing, "database.dsn (...)")
}
return nil, fmt.Errorf("config validation failed:\n  - %s", strings.Join(missing, "\n  - "))
```

## Database & Transactions

### Raw SQL with pgx
No ORM is used. The project uses `pgx/v5` with raw SQL and explicit `Scan` calls:

```go
row := s.pool.QueryRow(ctx, `SELECT id, email FROM users WHERE email=$1`, email)
var u models.User
if err := row.Scan(&u.ID, &u.Email); err != nil {
    ...
}
```

### Transaction Pattern
Begin, defer rollback, commit on success:

```go
tx, err := s.pool.Begin(ctx)
if err != nil {
    return err
}
defer tx.Rollback(ctx)

// ... insert operations ...

return tx.Commit(ctx)
```

## Authentication & Security

### Password Hashing
Argon2id with a custom `$`-delimited base64 encoding:

```go
hash := argon2.IDKey([]byte(password), salt, s.argonTime, s.argonMemory, s.argonThreads, 32)
encoded := base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash)
```

### Session Tokens
Tokens are generated from 32 random bytes, base64-URL-encoded for the client, and SHA-256 hashed for storage:

```go
tokenBytes := make([]byte, 32)
_, _ = rand.Read(tokenBytes)
token := base64.RawURLEncoding.EncodeToString(tokenBytes)
hash := sha256.Sum256(tokenBytes)
hashStr := base64.RawStdEncoding.EncodeToString(hash[:])
```

### Secure Cookies
Session cookies are HttpOnly, Secure (when TLS is enabled), SameSite=Lax, and 7-day expiry:

```go
http.SetCookie(w, &http.Cookie{
    Name:     "session",
    Value:    token,
    Path:     "/",
    HttpOnly: true,
    Secure:   secure,
    SameSite: http.SameSiteLaxMode,
    MaxAge:   86400 * 7,
})
```

## HTTP API Patterns

### Router
Chi (`github.com/go-chi/chi/v5`) is used for HTTP routing.

### Middleware Stack (order matters)
1. `api.RequestID` — injects or propagates `X-Request-ID`
2. `api.StructuredLogger` — structured request logging
3. `api.Recovery` — recovers panics, returns 500, logs stack trace
4. `api.CORS` — origin-restricted CORS headers
5. `api.NewRateLimiter(...).Handler` — per-IP token-bucket rate limiting

### Handler Registration Pattern
Packages expose a `RegisterRoutes(r chi.Router)` method on their handler struct. The main server wires them together in `cmd/server/main.go`.

### Response Helpers
A local `writeJSON` helper in `cmd/server/main.go` sets `Content-Type` and encodes the body:

```go
func writeJSON(w http.ResponseWriter, status int, body any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(body)
}
```

`internal/webmail/webmail.go` defines its own identical local helper rather than importing a shared one.

### Domain Context
Multi-tenant domain IDs are extracted from query params (`domain_id`), headers (`X-Domain-ID`), or URL params, then validated via `RequireDomainAdmin` middleware and stored in context.

## Worker & Background Job Patterns

### Job Structure
Jobs are JSON-serialized structs enqueued in Redis lists:

```go
// `internal/workers/workers.go`
type Job struct {
    ID          string          `json:"id"`
    Type        string          `json:"type"`
    Payload     json.RawMessage `json:"payload"`
    MaxAttempts int             `json:"max_attempts"`
    CreatedAt   int64           `json:"created_at"`
    ScheduledAt int64           `json:"scheduled_at"`
}
```

### Processor Interface
Each background job type implements the `Processor` interface:

```go
type Processor interface {
    Process(ctx context.Context, job *Job) error
}
```

### Retry & Dead Letter
Failed jobs are retried with exponential backoff via a Redis sorted set (`postnest:delayed`). After `MaxAttempts`, they move to a dead-letter queue (`postnest:dead`).

## Frontend Conventions

### File Structure
- Components live in `web/src/components/*.jsx`
- API calls are centralized in `web/src/api.js`
- Styling uses Tailwind CSS; custom base styles in `web/src/styles/index.css`

### Component Style
Functional components with hooks, default exports, and destructured props:

```jsx
// `web/src/components/Login.jsx`
export default function Login({ onLogin }) {
  const [email, setEmail] = useState('')
  ...
}
```

### HTTP Client
Axios is configured with a base URL and a response interceptor for global error handling:

```js
// `web/src/api.js`
const api = axios.create({
  baseURL: '/api/v1',
  withCredentials: true,
})

api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      window.location.href = '/login'
    }
    return Promise.reject(error)
  }
)
```

### Icons
All icons come from `lucide-react`:

```jsx
import { Mail, Lock, AlertCircle } from 'lucide-react'
```

### Routing
React Router DOM v7 is used in `web/src/App.jsx` with route definitions and a layout wrapper.

### SSE
A custom `SSEClient` class wraps `EventSource` with reconnect logic and emits to registered listeners.

## Build & Deployment

### Makefile Targets
```
make build          # builds all binaries and the webui
make build-server   # go build ./cmd/server
make build-webui    # cd web && npm run build
make test           # go test ./...
make migrate        # golang-migrate up
make admin-setup    # creates initial admin user + domain
```

### Docker
- `Dockerfile.server` — Go binary build
- `Dockerfile.webui` — Vite build + static serve
- `Dockerfile.migrate` — migration runner
- `Dockerfile.worker` — worker binary
- `docker-compose.yml` brings up Postgres, Redis, server, worker, and webui.

### TLS Strategy
Three mutually exclusive TLS modes chosen via config:
1. **ACME/Let's Encrypt** — `certmanager` package handles DNS-01 issuance and renewal
2. **Static certificates** — `tls.LoadX509KeyPair`
3. **No TLS** — optionally allows insecure plaintext auth for local development

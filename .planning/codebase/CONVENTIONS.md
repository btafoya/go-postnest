# PostNest Coding Conventions

This document catalogs the coding conventions and design patterns used throughout the PostNest Go 1.25 codebase.

---

## Code Style

- **Formatting**: Standard `gofmt` / `goimports`. No custom formatters.
- **Line length**: No enforced limit; long SQL queries are written as raw multi-line strings.
- **Imports**: Grouped into three blocks: stdlib, third-party, then internal packages.

  ```go
  // internal/mailstore/pgstore.go
  import (
      "context"
      "fmt"
      "time"

      "github.com/google/uuid"
      "github.com/jackc/pgx/v5"
      "github.com/jackc/pgx/v5/pgxpool"

      "github.com/go-postnest/postnest/internal/models"
  )
  ```

- **Comments**: Every exported symbol has a full-sentence comment starting with the symbol name. Unexported helpers are documented when non-trivial.

---

## Naming Conventions

| Category | Pattern | Example |
|----------|---------|---------|
| **Packages** | Single word, lowercase, no underscores | `mailstore`, `certmanager`, `webhook` |
| **Types** | PascalCase, descriptive | `PGStore`, `AppError`, `ListOptions` |
| **Interfaces** | Noun describing capability | `Store`, `Processor` |
| **Constructors** | `New` or `New<T>` | `NewLoader`, `NewPGStore`, `NewManager` |
| **Methods** | Verb or verb-noun | `CreateMessage`, `CountUnreadByLabel` |
| **Variables** | camelCase, short when scoped | `ctx`, `cfg`, `tx`, `w`, `r` |
| **Constants** | camelCase or PascalCase if exported | `defaultConfigPath` |
| **Context keys** | Unexported typed string | `type ctxKey string` |
| **Error variables** | `Err<Name>` | `ErrNotFound`, `ErrUnauthorized` |
| **Test functions** | `Test<Struct>_<Method>_<Scenario>` | `TestLoader_Load_FromFile` |

---

## Error Handling Patterns

### Wrapping with Context
All errors are wrapped using `fmt.Errorf("...: %w", err)` to preserve the causal chain:

```go
// internal/db/db.go
func New(dsn string, maxConns int) (*Pool, error) {
    cfg, err := pgxpool.ParseConfig(dsn)
    if err != nil {
        return nil, fmt.Errorf("parse postgres dsn: %w", err)
    }
    // ...
}
```

### Sentinel Errors
Package-level sentinel errors are declared as `var` and checked with `==`:

```go
// internal/mailstore/pgstore.go
var ErrNotFound = fmt.Errorf("not found")

// Checked at call sites:
if err == pgx.ErrNoRows {
    return nil, ErrNotFound
}
```

### Custom HTTP Error Type
The `api` package defines a unified `AppError` type for all HTTP responses:

```go
// internal/api/errors.go
type AppError struct {
    Code       string      `json:"code"`
    Message    string      `json:"message"`
    Details    []FieldError `json:"details,omitempty"`
    StatusCode int         `json:"-"`
    Err        error       `json:"-"`
}

var (
    ErrNotFound     = &AppError{Code: "not_found",     StatusCode: http.StatusNotFound}
    ErrUnauthorized = &AppError{Code: "unauthorized",  StatusCode: http.StatusUnauthorized}
    ErrForbidden    = &AppError{Code: "forbidden",     StatusCode: http.StatusForbidden}
    ErrValidation   = &AppError{Code: "validation_failed", StatusCode: http.StatusBadRequest}
    ErrInternal     = &AppError{Code: "internal_error",    StatusCode: http.StatusInternalServerError}
)

// WriteError serializes the error as JSON and sets the correct HTTP status.
func WriteError(w http.ResponseWriter, err error) { ... }
```

---

## Logging Approach

- **Logger**: `log/slog` with JSON output to `os.Stdout`.
- **Levels**: Info for normal operations, Warn for non-fatal degradations, Error for failures.
- **Key-value pairs**: All logs are structured. Keys are lowercase snake_case.

```go
// internal/logger/logger.go
func New() *slog.Logger {
    return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))
}
```

```go
// Typical usage:
logger.Info("request",
    "method", r.Method,
    "path", r.URL.Path,
    "duration", time.Since(start).String(),
    "request_id", RequestIDFromContext(r.Context()),
)
```

---

## Context Usage

`context.Context` is propagated as the first argument in every function that performs I/O or calls into external systems:

```go
// internal/mailstore/mailstore.go
type Store interface {
    CreateMessage(ctx context.Context, msg *models.Message, labelIDs []uuid.UUID, attachments []*models.Attachment) error
    GetMessage(ctx context.Context, domainID, userID, messageID uuid.UUID) (*models.Message, error)
    // ...
}
```

Context values are used sparingly for request-scoped metadata (user, domain ID, request ID). Keys are unexported typed strings to avoid collisions:

```go
// internal/api/middleware.go
type ctxKey string
const (
    ctxKeyUser      ctxKey = "user"
    ctxKeyDomainID  ctxKey = "domain_id"
    ctxKeyRequestID ctxKey = "request_id"
)
```

---

## Configuration Patterns

### Dual-Source Config: TOML + Environment Variables

`internal/config/loader.go` implements a two-layer config system:

1. **TOML file** (`/etc/postnest/postnest.conf`) provides defaults and structured settings.
2. **Environment variables** (`POSTNEST_<SECTION>_<KEY>`) override TOML values reflectively.
3. **Legacy env vars** are supported for backward compatibility (e.g., `POSTGRES_DSN` maps to `POSTNEST_DATABASE_DSN`).

```go
// internal/config/loader.go
func (l *Loader) Load() (*Config, error) {
    raw := rawConfig{ /* hard-coded defaults */ }

    if _, err := os.Stat(l.Path); err == nil {
        if _, err := toml.DecodeFile(l.Path, &raw); err != nil {
            return nil, fmt.Errorf("failed to decode config file %s: %w", l.Path, err)
        }
    }

    applyEnvOverrides(&raw)
    // ... translate to Config struct and validate
}
```

The legacy `internal/config/config.go` also exposes a simpler env-only path with fallback helpers (`getEnv`, `parseInt`, `parseDuration`).

---

## Middleware Patterns

HTTP middleware lives in `internal/api/middleware.go` and follows the standard `func(http.Handler) http.Handler` signature, often returning closures when dependencies are needed.

### Available Middleware

| Middleware | Purpose |
|-----------|---------|
| `RequestID` | Injects or propagates `X-Request-ID` |
| `StructuredLogger` | Logs every request with duration, method, path |
| `Recovery` | Recovers panics and writes JSON 500 |
| `CORS` | Basic CORS headers; short-circuits `OPTIONS` |
| `RequireSession` | Validates Bearer token or session cookie via `auth.Service` |
| `RequireDomainAdmin` | Checks domain admin role; falls back to super-admin |

```go
// internal/api/middleware.go
func RequireSession(svc *auth.Service) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := extractToken(r)
            if token == "" {
                WriteError(w, ErrUnauthorized)
                return
            }
            // ...
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

---

## Database Access Patterns

### Connection Pooling
`internal/db/db.go` wraps `pgxpool.Pool` with a thin struct that validates connectivity on creation:

```go
// internal/db/db.go
type Pool struct {
    *pgxpool.Pool
}

func New(dsn string, maxConns int) (*Pool, error) {
    cfg, err := pgxpool.ParseConfig(dsn)
    if err != nil { return nil, fmt.Errorf("parse postgres dsn: %w", err) }
    if maxConns > 0 { cfg.MaxConns = int32(maxConns) }
    pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
    // ... ping with 5s timeout
}
```

### Repository Pattern
Each domain defines an interface in the root package file and a `PGStore` implementation:

```go
// internal/mailstore/mailstore.go
type Store interface { ... }

// internal/mailstore/pgstore.go
type PGStore struct {
    pool *pgxpool.Pool
}
```

### SQL Style
- Raw SQL with positional parameters (`$1`, `$2`).
- Multi-line strings for readability.
- `COALESCE` used for optional updates in `UPDATE` statements.
- `ON CONFLICT DO NOTHING` / `ON CONFLICT ... DO UPDATE` for upserts.

### Transactions
Explicit `Begin`/`Commit`/`Rollback` with `defer tx.Rollback(ctx)`:

```go
// internal/mailstore/pgstore.go
func (s *PGStore) CreateMessage(ctx context.Context, msg *models.Message, ...) error {
    tx, err := s.pool.Begin(ctx)
    if err != nil { return err }
    defer tx.Rollback(ctx)
    // ... inserts ...
    return tx.Commit(ctx)
}
```

---

## Consistent Design Patterns

### Constructor / Factory Pattern
Every major component exposes a constructor that accepts dependencies explicitly:

```go
// internal/auth/auth.go
func NewService(pool *pgxpool.Pool, argonTime, argonMemory uint32, argonThreads uint8, sessionKey string) *Service

// internal/redis/redis.go
func New(redisURL string) (*Client, error)

// internal/workers/workers.go
func NewPool(r *redis.Client, logger *slog.Logger, concurrency int, pollInterval time.Duration) *Pool
```

### Interface-Driven Design
High-level code depends on interfaces, not concrete types:

```go
// internal/mailstore/mailstore.go
type Store interface { ... }

// internal/smtp/smtp.go
type smtpBackend struct {
    store    mailstore.Store   // interface, not *PGStore
    auth     *auth.Service
    postmark *postmark.Client
}
```

### Worker Pool with Job Processor Interface
Background jobs are dispatched via Redis lists and processed by type-registered `Processor` implementations:

```go
// internal/workers/workers.go
type Processor interface {
    Process(ctx context.Context, job *Job) error
}

func (p *Pool) Register(jobType string, proc Processor) {
    p.processors[jobType] = proc
}
```

### UUID v7 for Primary Keys
All entities generate IDs with `uuid.Must(uuid.NewV7())` for time-sortable, collision-resistant identifiers.

```go
// internal/mailstore/pgstore.go
if msg.ID == uuid.Nil {
    msg.ID = uuid.Must(uuid.NewV7())
}
```

### Embedded Migrations
Database migrations are embedded using `//go:embed` and driven by `golang-migrate/migrate`:

```go
// internal/migrate/migrate.go
//go:embed migrations/*.sql
var migrationsFS embed.FS
```

# Code Conventions

## Language & Tooling

- **Go 1.25** (`go 1.25.0` in `go.mod`).
- Standard toolchain only; no `testify`, `gomock`, or code-generation frameworks in use.
- `go fmt` is the implied formatter (no custom `gofmt` rules).

## Package Structure

- Packages are grouped by **domain boundary**, not by layer:
  - `internal/api` — HTTP middleware and shared API primitives.
  - `internal/auth` — Authentication service.
  - `internal/redis` — Redis client wrapper.
  - `internal/workers` — Background job pool.
  - `internal/webmail` — REST API handlers for the webmail UI.
  - `internal/webhook` — Inbound webhook handlers.
  - `internal/smtp` — SMTP server.
  - `internal/mailstore` — Persistence interface.
  - `internal/models` — Canonical domain structs.
- `cmd/` contains runnable binaries (`server`, `worker`, `migrate`).
- `internal/config` holds the unified configuration loader.

## Naming

| Kind | Rule |
|------|------|
| Exported identifiers | PascalCase (`AppError`, `WriteError`) |
| Unexported identifiers | camelCase (`extractToken`, `ctxKeyUser`) |
| Constructor functions | `New{Type}` (`NewPool`, `NewHandler`) |
| Interface names | Noun describing capability (`Processor`, `DomainLister`, `Store`) |
| Implementation types | Descriptive noun (`smtpBackend`, `smtpSession`, `testProcessor`) |
| Error variables | `Err{Name}` (`ErrNotFound`, `ErrUnauthorized`) |
| Context keys | Custom string type `ctxKey` + `const` (`ctxKeyUser ctxKey = "user"`) |

## Imports

- Grouped by: stdlib → third-party → internal.
- Aliases are used only to avoid collisions or clarify origin:
  - `goredis "github.com/redis/go-redis/v9"`
  - `gomail "github.com/emersion/go-message/mail"`

## Error Handling

### Unified Application Error Type

All HTTP-facing errors flow through `internal/api.AppError`:

```go
type AppError struct {
    Code       string      `json:"code"`
    Message    string      `json:"message"`
    Details    []FieldError `json:"details,omitempty"`
    StatusCode int         `json:"-"`
    Err        error       `json:"-"`
}
```

- `Code` is a machine-readable slug (e.g., `"not_found"`, `"validation_failed"`).
- `StatusCode` maps to the HTTP status code returned by `WriteError`.
- Sentinel error values are declared as package-level variables (`var ErrNotFound = &AppError{...}`).

### Error Translation

- Handlers **do not** leak raw database or library errors to the client.
- `WriteError` unwraps wrapped errors with `errors.As`; if the error is not an `*AppError`, it falls back to `ErrInternal`.
- `api.As` is a thin wrapper around `errors.As` to keep the `api` package self-contained.

### Validation

- Validation errors are built with `NewValidationError([]FieldError{...})`.
- Each `FieldError` names the field, the issue type, and an optional param.

## Interface Design

- **Keep interfaces small** when they describe a capability (`DomainLister` has one method).
- **Keep interfaces large** when they describe a bounded context (`mailstore.Store` has ~25 methods). Both patterns coexist; the large interface lives in the consumer package (`mailstore`) and is implemented by the concrete persistence layer.
- Handlers depend on interfaces, not concrete stores (`mailstore.Store`, `DomainLister`).

## Struct Tags & Encoding

- JSON tags use `camelCase` with `omitempty` for optional slices.
- TOML tags use `snake_case` for config structs.
- `env` tags are used in the legacy `Config` struct (transitioning to TOML + env overrides).

## HTTP Middleware

- Middleware is written as **higher-order functions** that return `func(http.Handler) http.Handler`.
- Middleware that needs configuration (e.g., `CORS`, `StructuredLogger`, `RequireSession`) accepts dependencies in the outer closure.
- `chi/v5` is the router.

## Context Usage

- `context.Context` is passed explicitly through all service and store methods.
- Request-scoped values (user, domain ID, request ID) are stored via `context.WithValue` using typed string keys (not bare strings).
- `WithUser`, `UserFromContext`, `DomainIDFromContext`, `RequestIDFromContext` provide typed accessors.

## Logging

- Standard library `log/slog` only.
- Structured key-value logging: `logger.Info("request", "method", r.Method, "path", r.URL.Path)`.
- Errors are logged at `Error` level; panics are recovered by the `Recovery` middleware and logged with the stack trace.

## UUIDs

- `github.com/google/uuid` is the UUID library.
- V7 UUIDs are generated for new entities: `uuid.Must(uuid.NewV7())`.
- `uuid.Nil` is the zero value sentinel.

## Database Access

- `pgx/v5` pool for PostgreSQL.
- Raw SQL via `pool.QueryRow`, `pool.Query`, `pool.Exec`.
- Transactions are used explicitly (`tx, err := s.pool.Begin(ctx)`); `defer tx.Rollback(ctx)` is the standard pattern.
- `pgx.ErrNoRows` is checked to convert to domain errors.

## Security Patterns

- **Password hashing**: Argon2id with per-user salt; custom encoding (`base64.RawStdEncoding`) joined by `$`.
- **Token storage**: SHA-256 hash of the raw token is stored; raw token is returned once on creation.
- **Constant-time comparison**: Hand-rolled `constantTimeCompare` for hash verification.
- **Session cookies**: `HttpOnly`, `Secure` (configurable), `SameSite=Lax`, 7-day expiry.
- **Rate limiting**: In-memory token bucket per IP with periodic stale-entry cleanup.

## Configuration

- TOML file (`postnest.conf`) with environment variable overrides.
- Env var naming: `POSTNEST_<SECTION>_<KEY>` in `SCREAMING_SNAKE_CASE`.
- Legacy env vars are mapped for backward compatibility.
- `Loader` uses reflection to walk the raw config struct and apply overrides.
- Validation is explicit: collect missing fields into a slice and return a single formatted error.

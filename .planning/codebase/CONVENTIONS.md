# Code Conventions

This document captures the coding style, naming rules, architectural patterns, error handling, and interface design used across the Postnest Go codebase.

## Language & Formatting

- **Go 1.25** — the module pins `go 1.25.0`.
- Standard `gofmt` formatting is assumed; no custom style overrides are visible.
- Imports are grouped in three blocks: stdlib, third-party, internal packages.
- File names are lowercase snake_case (e.g. `errors.go`, `middleware_test.go`, `pgstore.go`).

## Naming

| Category | Rule | Example |
|----------|------|---------|
| Package | Matches directory name, short and singular | `package api`, `package mailstore` |
| Exported type | PascalCase | `AppError`, `RateLimiter`, `PGStore` |
| Interface | Noun / capability name | `Processor`, `DomainLister`, `Store` |
| Constructor | `New<Name>` | `NewPool`, `NewHandler`, `NewPGStore` |
| Exported func / method | PascalCase | `WriteError`, `RequireSession` |
| Unexported helper | camelCase, starts lower | `extractToken`, `isClosedErr`, `toScreamingSnakeCase` |
| Variable / const | camelCase / PascalCase depending on export | `ctxKeyUser`, `queueJobs`, `ErrNotFound` |
| Error sentinel | `Err<Name>` | `ErrUnauthorized`, `ErrRateLimited` |
| Test helper | `setup<TestSubject>` or `newTestHandler` | `setupTestRedis`, `newTestHandler` |

## Structs & Models

- Models live in `internal/models` and are plain structs with exported fields. No ORM tags are used; fields are scanned manually with `pgx`.
- ID fields are `github.com/google/uuid` (`uuid.UUID`).
- Timestamps are `time.Time` and are set to `time.Now().UTC()` before persistence.
- Nullable foreign keys use pointers (e.g. `ThreadID *uuid.UUID`).
- Other slices use value types (e.g. `ToAddresses []string`).
- Handler structs hold their dependencies explicitly:
  ```go
  type Handler struct {
      store mailstore.Store
      auth  DomainLister
      redis *redis.Client
  }
  ```
- Configuration is loaded via `internal/config.Loader` which reads a TOML file and applies `POSTNEST_<SECTION>_<KEY>` environment overrides, with legacy fallback names.

## Error Handling

- The unified application error type is `*AppError` (`internal/api/errors.go`):
  ```go
  type AppError struct {
      Code       string      `json:"code"`
      Message    string      `json:"message"`
      Details    []FieldError `json:"details,omitempty"`
      StatusCode int         `json:"-"`
      Err        error       `json:"-"`
  }
  ```
- Sentinel errors are pre-declared package variables:
  ```go
  var ErrNotFound = &AppError{Code: "not_found", ...}
  ```
- Wrapping is done with `fmt.Errorf("...: %w", err)`.
- `api.As` is a thin wrapper over `errors.As` for unwrapping `*AppError`.
- Validation failures are built with `NewValidationError([]FieldError{...})`.
- HTTP responses use `api.WriteError(w, err)`, which falls back to `ErrInternal` if the error is not an `*AppError`.

## Interfaces & Dependency Injection

- Large interfaces are avoided where possible. `mailstore.Store` is the canonical broad interface (≈25 methods), but tests show that callers often depend on narrower interfaces (e.g. `DomainLister` with a single method).
- Dependency injection is manual: constructors accept concrete or interface types, and wire-up happens in `cmd/server/main.go` and `cmd/worker/main.go`.
- There is no DI container or code generation for mocks.

## HTTP Layer

- Router: `github.com/go-chi/chi/v5`.
- Middleware returns `func(http.Handler) http.Handler` or is a method on a struct (`RateLimiter.Handler`).
- Context values are stored using typed string keys (`type ctxKey string`) to avoid collisions:
  ```go
  const ctxKeyUser ctxKey = "user"
  ```
- Helpers expose read/write access to context values:
  ```go
  func WithUser(ctx context.Context, user *models.User) context.Context
  func UserFromContext(ctx context.Context) *models.User
  ```
- JSON output uses a small helper `writeJSON` (defined in `cmd/server/main.go`) or `api.WriteError`.
- Status codes are the standard `http.Status*` constants.

## Database Access

- Driver: `github.com/jackc/pgx/v5` via `pgxpool.Pool`.
- Queries are hand-written SQL with positional parameters (`$1`, `$2`, …).
- Transactions follow the standard `Begin → defer Rollback → Commit` pattern:
  ```go
  tx, err := s.pool.Begin(ctx)
  if err != nil { return err }
  defer tx.Rollback(ctx)
  // ... work ...
  return tx.Commit(ctx)
  ```
- Row scanning is explicit; `pgx.ErrNoRows` is checked for “not found” cases.

## Logging

- `log/slog` is used everywhere.
- Key-value logging style: `logger.Info("request", "method", r.Method, "path", r.URL.Path, ...)`.
- Panic recovery logs the stack via `string(debug.Stack())`.

## Configuration

- `internal/config.Loader` reads a TOML file (`/etc/postnest/postnest.conf` by default) into a `rawConfig` struct, then translates it to the flat `Config` struct.
- Environment overrides are applied reflectively: `POSTNEST_<SECTION>_<KEY>`.
- Legacy environment variable names are mapped for backward compatibility (e.g. `POSTGRES_DSN` → `POSTNEST_DATABASE_DSN`).
- Validation is performed after merging; missing required fields produce an aggregated error string.

## Security & Cryptography

- Password hashing uses Argon2id with configurable time, memory, and thread parameters.
- Session and API tokens are random 32-byte values, base64-encoded for transport, SHA-256 hashed for storage.
- A constant-time byte comparison helper (`constantTimeCompare`) is used for password verification.
- Cookies are `HttpOnly`, `Secure` (when TLS is enabled), `SameSite=Lax`, 7-day MaxAge.
- Rate limiting is a simple in-memory per-IP token bucket protected by `sync.Mutex`.

## Concurrency

- Worker pool (`internal/workers.Pool`) spawns goroutines based on a configurable concurrency level.
- Graceful shutdown uses `sync.WaitGroup` and context cancellation.
- Redis-backed queues (`LPush`, `BRPop`, `ZAdd`) are used for job distribution.

## Context

- `context.Context` is always the first parameter in functions that need it.
- Timeouts are created inline when crossing process boundaries (e.g. `context.WithTimeout(r.Context(), 5*time.Second)`).

## Testing Helpers

- `api.WithUser` is explicitly documented as being used by tests and middleware to inject a user into a context.
- Handler tests manually construct `chi.RouteContext` and inject it into the request context when URL parameters are required (`webmail_test.go`).

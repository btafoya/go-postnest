# Postnest Testing Patterns

## Overview

The project uses Go’s standard `testing` package exclusively. There are **no external test frameworks** such as testify, ginkgo, or testify/assert. Tests are co-located with the code they exercise (`*_test.go` in the same package). There are currently **no frontend tests** in the React/Vite codebase.

Test file inventory:
- `internal/api/errors_test.go`
- `internal/api/middleware_test.go`
- `internal/auth/auth_test.go`
- `internal/config/loader_test.go`
- `internal/redis/redis_test.go`
- `internal/smtp/smtp_test.go`
- `internal/webhook/webhook_test.go`
- `internal/webmail/webmail_test.go`
- `internal/workers/workers_test.go`

## Test Framework

### Standard Library Only
Every test file imports only `testing` (plus production dependencies):

```go
import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
)
```

There is **no `testify`**, **no `assert` helper library**, and **no `require` package**. All assertions are written with raw `if` statements, `t.Fatalf`, and `t.Errorf`.

### Assertion Patterns

Prefer `t.Fatalf` for setup errors that prevent the test from continuing, and `t.Errorf` for assertion failures that allow later assertions to run:

```go
// `internal/api/errors_test.go`
func TestAs_Direct(t *testing.T) {
    var target *AppError
    if !As(ErrValidation, &target) {
        t.Fatal("expected As to match direct AppError")
    }
    if target.StatusCode != http.StatusBadRequest {
        t.Errorf("status = %d, want %d", target.StatusCode, http.StatusBadRequest)
    }
}
```

```go
// `internal/webmail/webmail_test.go`
func TestCreateDraft(t *testing.T) {
    h, store := newTestHandler()
    ...
    if rr.Code != http.StatusCreated {
        t.Errorf("status = %d, want %d", rr.Code, http.StatusCreated)
    }
    if len(store.messages) != 1 {
        t.Fatalf("expected 1 message, got %d", len(store.messages))
    }
    msg := store.messages[0]
    if msg.Subject != "Hello" {
        t.Errorf("subject = %q, want Hello", msg.Subject)
    }
}
```

### Naming Conventions

Test names follow `Test<Component>_<Scenario>`:

```go
func TestCORS_AllowedOrigin(t *testing.T)
func TestCORS_DisallowedOrigin(t *testing.T)
func TestCORS_Preflight(t *testing.T)
func TestRateLimiter_AllowsWithinLimit(t *testing.T)
func TestRateLimiter_BlocksOverLimit(t *testing.T)
func TestRecovery_RecoversPanic(t *testing.T)
func TestWorker_RetryAndDeadLetter(t *testing.T)
func TestWorker_PromotesDelayed(t *testing.T)
```

## Mocking Strategy

### Hand-Rolled In-Memory Mocks
The project does **not use a mocking framework**. Instead, it defines small in-memory structs that implement the interfaces under test.

Example from `internal/webmail/webmail_test.go`:

```go
type mockStore struct {
    messages []*models.Message
    labels   []*models.Label
}

func (m *mockStore) CreateMessage(ctx context.Context, msg *models.Message, labelIDs []uuid.UUID, attachments []*models.Attachment) error {
    m.messages = append(m.messages, msg)
    return nil
}

func (m *mockStore) GetMessage(ctx context.Context, domainID, userID, messageID uuid.UUID) (*models.Message, error) {
    for _, msg := range m.messages {
        if msg.ID == messageID {
            return msg, nil
        }
    }
    return nil, mailstore.ErrNotFound
}

func (m *mockStore) ListMessages(ctx context.Context, domainID, userID uuid.UUID, labelID *uuid.UUID, opts mailstore.ListOptions) ([]*models.Message, int64, error) {
    return m.messages, int64(len(m.messages)), nil
}
// ... remaining Store methods
```

### Partial Interface Implementations
Mocks only need to implement the methods actually called by the code under test. Unused methods may return `nil, nil` or empty values.

```go
// `internal/webmail/webmail_test.go`
type mockAuth struct{}

func (a *mockAuth) GetUserDomains(ctx context.Context, userID uuid.UUID) ([]*models.DomainMember, error) {
    return []*models.DomainMember{{DomainID: userID, UserID: userID, Role: "admin"}}, nil
}
```

### Factory Helpers
Tests use factory functions to create the handler + mock pair, keeping individual tests short:

```go
// `internal/webmail/webmail_test.go`
func newTestHandler() (*Handler, *mockStore) {
    store := &mockStore{}
    return NewHandler(store, &mockAuth{}, nil), store
}
```

## HTTP Handler Testing

HTTP handler tests use `net/http/httptest` to construct requests and record responses:

```go
// `internal/webmail/webmail_test.go`
reqBody, _ := json.Marshal(map[string]any{
    "subject":   "Hello",
    "to":        []map[string]string{{"address": "bob@example.com"}},
    "html_body": "<p>Hi</p>",
})
req := httptest.NewRequest(http.MethodPost, "/api/v1/drafts", bytes.NewReader(reqBody))
req = req.WithContext(api.WithUser(req.Context(), &models.User{...}))
rr := httptest.NewRecorder()

h.createDraft(rr, req)

if rr.Code != http.StatusCreated {
    t.Errorf("status = %d, want %d", rr.Code, http.StatusCreated)
}
```

### Chi Routing Context
When testing handlers that read Chi URL parameters, inject the route context manually:

```go
// `internal/webmail/webmail_test.go`
chiCtx := chi.NewRouteContext()
chiCtx.URLParams.Add("id", draftID.String())
req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
```

### Context Helpers for Tests
The `api` package exports `WithUser` specifically for use by tests and middleware:

```go
// `internal/api/middleware.go`
func WithUser(ctx context.Context, user *models.User) context.Context {
    return context.WithValue(ctx, ctxKeyUser, user)
}
```

This allows tests to bypass session middleware and inject a user directly into the request context.

## Database Testing

### In-Memory Redis (miniredis)
Redis-dependent tests use `github.com/alicebob/miniredis/v2` to spin up an ephemeral Redis server per test:

```go
// `internal/redis/redis_test.go` and `internal/workers/workers_test.go`
func setupTestPool(t *testing.T) (*Pool, *miniredis.Miniredis, *time.Time) {
    m := miniredis.RunT(t)
    c, err := redis.New("redis://" + m.Addr())
    if err != nil {
        t.Fatalf("new redis: %v", err)
    }
    logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
    p := NewPool(c, logger, 1, 100*time.Millisecond)
    now := time.Now()
    p.nowFunc = func() time.Time { return now }
    return p, m, &now
}
```

`miniredis.RunT(t)` automatically cleans up resources when the test finishes.

### Time Manipulation in Tests
For time-sensitive logic (delayed jobs, rate limiting), tests inject a custom time function:

```go
// `internal/workers/workers_test.go`
p.nowFunc = func() time.Time { return now }
```

`miniredis` provides `FastForward(duration)` to advance its internal clock without sleeping.

### Database Pool Tests
`internal/db/db.go` and the auth package depend on PostgreSQL. There are **no unit tests that spin up a real Postgres instance** or use an in-memory equivalent. Database access is tested indirectly via the mock store pattern in handler tests.

## Table-Driven Tests

The project uses table-driven tests where multiple similar cases exist:

```go
// `internal/api/middleware_test.go`
func TestCORS_AllowedOrigin(t *testing.T) {
    handler := CORS([]string{"https://example.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))

    req := httptest.NewRequest(http.MethodGet, "/", nil)
    req.Header.Set("Origin", "https://example.com")
    rr := httptest.NewRecorder()
    handler.ServeHTTP(rr, req)

    if rr.Code != http.StatusOK {
        t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
    }
    if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
        t.Errorf("CORS header = %q, want https://example.com", got)
    }
}
```

For repeated setup, extract a shared helper rather than embedding everything in a table literal.

## Coverage Patterns

### What Is Tested
- **Middleware**: CORS, rate limiting, panic recovery, session cookies, request ID injection
- **Error types**: Wrapping/unwrapping, `errors.As` compatibility
- **Auth**: Password hashing and verification round-trip, invalid hash handling
- **Config loader**: File loading, environment overrides, legacy env fallback, missing-required validation
- **SMTP backend**: LOGIN auth mechanism, invalid credential rejection
- **Webhooks**: Deduplication logic, TTL eviction, missing MessageID handling
- **Workers**: Job enqueue/dequeue, retry with dead-letter promotion, delayed job scheduling
- **Webmail handlers**: Draft creation, draft update, JSON request/response shapes

### What Is Not Tested (Gaps)
- **No frontend tests** (`web/` has zero test files)
- **No integration tests** against real Postgres
- **No benchmark tests** (`*_bench.go`)
- **No fuzz tests**
- **No e2e/API contract tests**
- **No coverage for `internal/certmanager/`**, `internal/dav/`, `internal/imap/` (IMAP backend has no `_test.go`)
- **No tests for `cmd/` binaries** (setup logic is exercised manually via `make admin-setup`)

## Test Isolation

Tests do not share mutable global state. Each test that needs Redis creates its own `miniredis` instance. The worker pool tests cancel their background context and call `Stop` before returning.

```go
func TestWorker_PromotesDelayed(t *testing.T) {
    p, m, _ := setupTestPool(t)
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    ...

    p.Stop(context.Background())
}
```

## Running Tests

### Go Tests
```bash
go test ./...
```

Or per-package:
```bash
go test ./internal/api/
go test ./internal/workers/
```

### Makefile
```bash
make test
```

### Verbose Output
```bash
go test -v ./internal/workers/
```

## Recommended Additions

Based on current patterns, the following would extend coverage while staying consistent with existing style:

1. **Table-driven config loader tests** for edge cases (invalid duration strings, empty TOML file)
2. **Middleware tests** for `RequireSession` and `RequireDomainAdmin` using the mock auth pattern
3. **Database integration tests** in a separate `*_integration_test.go` file using `testing.Short()` guard
4. **Frontend unit tests** with Vitest (consistent with the Vite toolchain) for `api.js` interceptors and component rendering
5. **Benchmark tests** for password hashing (`argon2id`) and rate limiter token bucket
6. **Contract tests** that validate JSON serialization of all `models` structs against the API spec

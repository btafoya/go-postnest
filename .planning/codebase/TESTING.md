# Testing Patterns

## Framework

- **Standard library only**: `testing` + `net/http/httptest`.
- No external assertion libraries (`testify`, `gotest.tools`, etc.).
- Tests are run with `go test ./...`.

## Test File Layout

- Tests live in the same package as the code under test (`package api`, `package redis`, etc.).
- File naming: `{module}_test.go` (`errors_test.go`, `middleware_test.go`, `redis_test.go`).

## Test Naming

Tests use descriptive, sentence-like names following the pattern:

```
Test<Function>_<Scenario>
```

Examples:
- `TestCORS_AllowedOrigin`
- `TestRateLimiter_BlocksOverLimit`
- `TestEnqueueDelayed_PromotesReadyDelayed`
- `TestDedup_DuplicateMessage`
- `TestWorker_RetryAndDeadLetter`

## Assertions

| Situation | Pattern |
|-----------|---------|
| Fatal setup error | `t.Fatalf("setup description: %v", err)` |
| Fatal logical failure | `t.Fatal("expected X")` |
| Non-fatal mismatch | `t.Errorf("field = %q, want %q", got, want)` |
| Boolean expectation | `if !ok { t.Error("expected X") }` |

No table-driven tests are used in the current suite; each scenario is a separate top-level function.

## Mocking

All mocks are **hand-rolled** in test files or in a `*_test.go` helper.

### In-Memory Mocks

- `mockStore` in `webmail_test.go` implements `mailstore.Store` with in-memory slices (`messages []*models.Message`, `labels []*models.Label`).
- `mockAuth` in `webmail_test.go` implements `DomainLister`.
- `testProcessor` in `workers_test.go` implements `workers.Processor` with `called int` and `fail bool`.

### Partial Implementations

Mocks implement the full interface but stub methods that are not relevant to the test with zero-value returns:

```go
func (m *mockStore) DeleteMessage(ctx context.Context, ...) error { return nil }
func (m *mockStore) GetAttachment(ctx context.Context, ...) (*models.Attachment, error) { return nil, nil }
```

## External Dependency Replacement

| Dependency | Replacement | Usage |
|------------|-------------|-------|
| Redis | `github.com/alicebob/miniredis/v2` | `miniredis.RunT(t)` creates an isolated in-memory Redis server per test. |
| PostgreSQL | Hand-rolled mocks (no `pgx` mock library) | `mailstore.Store` interface allows swapping the real DB for an in-memory mock. |
| Time | `nowFunc` injection | `Pool.nowFunc` is replaced in tests to control `time.Now()` behavior. |
| HTTP | `net/http/httptest` | `httptest.NewRecorder()` and `httptest.NewRequest(...)` for handler tests. |

### Redis Test Helpers

Each Redis-dependent package follows the same setup pattern:

```go
func setupTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
    m := miniredis.RunT(t)
    c, err := redis.New("redis://" + m.Addr())
    if err != nil {
        t.Fatalf("new redis client: %v", err)
    }
    return c, m
}
```

`miniredis.FastForward(duration)` is used to advance TTLs and timeouts without real sleeps.

### Worker Pool Test Helpers

```go
func setupTestPool(t *testing.T) (*Pool, *miniredis.Miniredis, *time.Time) {
    m := miniredis.RunT(t)
    c, _ := redis.New("redis://" + m.Addr())
    p := NewPool(c, logger, 1, 100*time.Millisecond)
    now := time.Now()
    p.nowFunc = func() time.Time { return now }
    return p, m, &now
}
```

## HTTP Handler Testing

- Requests are built with `httptest.NewRequest(method, path, body)`.
- `chi` route context values are injected manually when URL parameters are required:

```go
chiCtx := chi.NewRouteContext()
chiCtx.URLParams.Add("id", draftID.String())
req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
```

- Authentication context is injected with `api.WithUser(req.Context(), &models.User{...})`.
- Response assertions check `rr.Code`, `rr.Header()`, and `rr.Body.String()`.

## Configuration Testing

- `t.TempDir()` creates isolated directories for config file tests.
- `t.Setenv(key, value)` is used for env-override tests; Go automatically cleans up after the test.
- Legacy environment variable fallback is tested by writing to the real environment (`t.Setenv`).

## Time Manipulation

- **Injected clocks**: `Pool.nowFunc` is replaced with a closure that returns a controllable `time.Time`.
- **miniredis time travel**: `m.FastForward(10 * time.Second)` advances the Redis server's internal clock.
- Real sleeps are used sparingly for worker pool lifecycle tests (`time.Sleep(300 * time.Millisecond)`) where polling is unavoidable.

## Coverage Patterns

### Error Paths

- Tests verify that malformed input produces the expected error:
  - `TestVerifyPassword_InvalidHash` checks both `"nope"` and `""` hashes.
  - `TestAs_NonAppError` confirms a plain error does not match `*AppError`.

### Boundary Conditions

- Rate limiter: exactly at-limit requests pass, the next is blocked.
- Worker retries: `MaxAttempts = 2` results in exactly one dead-letter entry after two failures.
- Dedup TTL: a key evicted after 6 minutes is treated as a new message.

### Middleware

- Each middleware is tested in isolation:
  - `CORS` with allowed, disallowed, and preflight origins.
  - `Recovery` with an explicit `panic("boom")`.
  - `RateLimiter` with burst and over-limit sequences.
  - `SetSessionCookie` inspects the cookie attributes (`HttpOnly`, `Secure`, `SameSite`, `MaxAge`).

## What Is Not Tested

- There are **no integration tests** against a real PostgreSQL instance.
- There are **no end-to-end tests** or browser automation tests.
- There is **no benchmark suite** (`*_bench.go`).
- There is **no fuzz testing**.
- There is **no race detector annotation** (`-race` is not mentioned).

## Test Isolation

- Each test gets its own `miniredis` instance.
- File-system tests use `t.TempDir()`.
- Environment variable tests use `t.Setenv()`.
- HTTP tests share no mutable global state; all state is in the injected mocks.

# Testing Patterns

This document describes how tests are structured, what tools are used, how dependencies are mocked, and the observable coverage across the Postnest codebase.

## Framework & Tooling

- **Pure standard library**: only `testing` is used. No `testify`, `ginkgo`, `gomega`, or `mockgen` appear in `go.mod`.
- **Redis**: `github.com/alicebob/miniredis/v2` provides an in-memory Redis server for tests.
- **HTTP**: `net/http/httptest` is used for handler and middleware tests.
- **Files**: `os.WriteFile` and `t.TempDir()` are used for config-loader tests.

## File & Package Layout

- Test files are co-located with production code: `*_test.go` in the same package.
- Examples:
  - `internal/api/errors_test.go` tests `internal/api/errors.go`
  - `internal/webmail/webmail_test.go` tests `internal/webmail/webmail.go`
  - `internal/config/loader_test.go` tests `internal/config/loader.go`

## Test Naming

Tests follow the Go convention `Test<Name>` with PascalCase. Scenarios are separated by underscores:

```go
func TestLoader_Load_FromFile(t *testing.T)       // config/loader_test.go
func TestRateLimiter_BlocksOverLimit(t *testing.T) // api/middleware_test.go
func TestWorker_RetryAndDeadLetter(t *testing.T)   // workers/workers_test.go
```

No subtests (`t.Run`) are used in the existing suite, but the naming convention already encodes the scenario.

## Setup & Teardown Helpers

Each test package defines small setup helpers that create fresh resources:

```go
// internal/redis/redis_test.go
func setupTestRedis(t *testing.T) (*Client, *miniredis.Miniredis) {
    m := miniredis.RunT(t)
    c, err := New("redis://" + m.Addr())
    if err != nil { t.Fatalf(...) }
    return c, m
}

// internal/workers/workers_test.go
func setupTestPool(t *testing.T) (*Pool, *miniredis.Miniredis, *time.Time) { ... }

// internal/webmail/webmail_test.go
func newTestHandler() (*Handler, *mockStore) { ... }
```

These helpers guarantee isolation: every test gets a new Redis instance, new pool, or new in-memory store.

## Mocking

Mocks are hand-written structs that implement the interface(s) required by the unit under test. Only methods actually exercised by tests carry real logic; the rest return zero values:

```go
// internal/webmail/webmail_test.go
type mockStore struct { messages []*models.Message; labels []*models.Label }

func (m *mockStore) CreateMessage(...) error { m.messages = append(m.messages, msg); return nil }
func (m *mockStore) GetMessage(...) (*models.Message, error) { ... }
func (m *mockStore) DeleteMessage(...) error { return nil } // stub
```

```go
// internal/workers/workers_test.go
type testProcessor struct { called int; fail bool }

func (tp *testProcessor) Process(ctx context.Context, job *Job) error {
    tp.called++
    if tp.fail { return errors.New("process error") }
    return nil
}
```

No mocking framework is used; this keeps the test dependency tree minimal and avoids generated-code drift.

## HTTP Handler / Middleware Tests

HTTP tests construct requests with `httptest.NewRequest` and record responses with `httptest.NewRecorder`:

```go
req := httptest.NewRequest(http.MethodGet, "/", nil)
req.Header.Set("Origin", "https://example.com")
rr := httptest.NewRecorder()
h.ServeHTTP(rr, req)
```

Assertions check status code, headers, cookies, or body fragments:

```go
if rr.Code != http.StatusNoContent { t.Errorf("status = %d, want %d", ...) }
if got := rr.Header().Get("Access-Control-Allow-Origin"); got != want { ... }
```

When testing `chi` handlers that read URL parameters, the route context is injected manually:

```go
chiCtx := chi.NewRouteContext()
chiCtx.URLParams.Add("id", draftID.String())
req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
```

Authentication context is injected via `api.WithUser`:

```go
req = req.WithContext(api.WithUser(req.Context(), &models.User{...}))
```

## Redis Tests

Redis tests rely on `miniredis`:

```go
m := miniredis.RunT(t)
c, _ := redis.New("redis://" + m.Addr())
```

`miniredis` provides `FastForward(duration)` to simulate TTL expiry without real sleeps:

```go
_ = h.dedup(ctx, payload)
m.FastForward(6 * time.Minute) // advance past the 5-minute TTL
if !h.dedup(ctx, payload) { t.Error("expected true after TTL expiry") }
```

Tests also verify sorted-set scores, list lengths, and blocking pop behaviour:

```go
items, _ := c.UniversalClient.ZRangeByScore(ctx, "delayed", &goredis.ZRangeBy{Min:"-inf", Max:"+inf"}).Result()
```

## Async / Time-Based Tests

The worker tests deal with asynchronous job processing. Two techniques are used:

1. **Time injection**: `Pool` exposes a `nowFunc` field that tests replace to control time:
   ```go
   p.nowFunc = func() time.Time { return now }
   ```

2. **Polling loops**: tests sleep briefly and poll for side effects rather than relying on exact timing:
   ```go
   found := false
   for i := 0; i < 30; i++ {
       if tp.called > 0 { found = true; break }
       time.Sleep(50 * time.Millisecond)
   }
   ```

3. **Clock advancement**: `miniredis.FastForward` advances Redis-internal TTLs and BRPop timeouts.

## Configuration Tests

`config/loader_test.go` demonstrates file and environment testing:

```go
dir := t.TempDir()
path := filepath.Join(dir, "postnest.conf")
_ = os.WriteFile(path, []byte(content), 0640)
```

Environment variables are set with `t.Setenv` (automatically cleaned up):

```go
t.Setenv("POSTNEST_DATABASE_DSN", "postgres://env@localhost/env")
```

Legacy environment variables are tested by setting the old key and asserting the loader maps it correctly:

```go
t.Setenv("POSTGRES_DSN", "postgres://legacy@localhost/legacy")
```

Missing-required validation is tested by invoking `Load()` with no config file and no env vars, expecting a non-nil error.

## Error Tests

`internal/api/errors_test.go` exercises the custom error type:

```go
inner := ErrNotFound
wrapped := fmt.Errorf("wrapped: %w", inner)
var target *AppError
if !As(wrapped, &target) { t.Fatal("expected As to match") }
```

It covers three cases:
1. **Wrapped** — `errors.As` unwraps through `fmt.Errorf("%w")`.
2. **Direct** — sentinel error matches directly.
3. **Non-AppError** — plain `error` does not match.

## Assertion Style

- `t.Fatalf` is reserved for **setup failures** or conditions that make the rest of the test meaningless (e.g. can’t write temp file, can’t connect to miniredis).
- `t.Errorf` is used for **assertion mismatches** (wrong status code, wrong header, wrong count).
- No helper like `assertEq` is defined; comparisons are written inline. This is verbose but avoids an external dependency.

## Coverage Summary

| Package | Tests | Notes |
|---------|-------|-------|
| `internal/api` | `errors_test.go`, `middleware_test.go` | CORS, rate limiter, panic recovery, cookie writing, error wrapping |
| `internal/config` | `loader_test.go` | File load, env override, legacy env, validation failure |
| `internal/redis` | `redis_test.go` | Enqueue, dequeue, delayed promote, dead letter |
| `internal/smtp` | `smtp_test.go` | LOGIN SASL server, auth mechanism list |
| `internal/webhook` | `webhook_test.go` | Deduplication (new, duplicate, no MessageID, TTL eviction) |
| `internal/webmail` | `webmail_test.go` | Create draft, update draft (handler-level) |
| `internal/workers` | `workers_test.go` | Job UUID generation, retry/dead-letter, delayed promotion |

**Untested packages** (as of this analysis):
- `internal/auth` — no `auth_test.go`; session/crypto logic is exercised only via integration.
- `internal/mailstore` — `PGStore` has no dedicated tests; covered indirectly via `webmail_test.go` mocks.
- `internal/models` — no `models_test.go`.
- `cmd/server` and `cmd/worker` — no main-level tests.

## Patterns to Follow

1. **Keep tests in the same package** unless an external (black-box) test is specifically needed.
2. **Use `t.TempDir()` and `t.Setenv()`** for filesystem and environment tests.
3. **Write hand-rolled mocks** when the interface surface is small; keep them in the `_test.go` file.
4. **Use `httptest` for HTTP units** and inject `chi` route context manually when URL params are required.
5. **Use `miniredis` for Redis units** and leverage `FastForward` to avoid real-time sleeps.
6. **Control time via dependency injection** (e.g. `nowFunc`) rather than sleeping in tests when possible.
7. **Prefer `t.Errorf` for assertions** and `t.Fatalf` for setup errors.
8. **Name tests after the scenario** (`Test<Subject>_<State>_<Expected>`) so failures are self-describing.

## Anti-Patterns to Avoid

- Do not add `testify` or another assertion library without a team decision; the current suite is intentionally dependency-light.
- Do not write table-driven tests without a clear reason; the existing suite uses linear tests because most cases have distinct setup requirements.
- Avoid real `time.Sleep` in tests when `miniredis.FastForward` or injected clocks can achieve the same result.
- Do not share mutable mock state between tests; always return fresh instances from setup helpers.

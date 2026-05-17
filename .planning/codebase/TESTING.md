# Testing Patterns

**Analysis Date:** 2026-05-17

## Test Framework

**Runner:**
- Go's built-in `testing` package only (no testify, ginkgo, gomega, or other assertion libraries)
- `go test ./...` is the standard run command
- Go version 1.25.0

**Assertion Library:**
- Manual `if` checks with `t.Fatalf`, `t.Fatalf`, `t.Errorf`
- No external assertion library dependency

**Run Commands:**
```bash
go test ./...                      # Run all tests
go test -v ./...                  # Verbose output
go test ./internal/api/...        # Single package
go test -run TestCORS ./internal/api/  # Single test by name
go test -cover ./...              # Coverage summary
go test -coverprofile=c.out ./... && go tool cover -html=c.out  # HTML report
```

## Test File Organization

**Location:**
- `*_test.go` alongside source files in the same package
- No separate `tests/` directory tree

**Naming:**
- `package_test.go` for package-level tests (auth_test.go, middleware_test.go)
- No distinction between unit/integration in filename
- All test files use the same package as the code under test

**Structure:**
```
internal/
  api/
    errors.go
    errors_test.go
    middleware.go
    middleware_test.go
  auth/
    auth.go
    auth_test.go
  config/
    loader.go
    loader_test.go
  redis/
    redis.go
    redis_test.go
  smtp/
    smtp.go
    smtp_test.go
  webhook/
    webhook.go
    webhook_test.go
  webmail/
    webmail.go
    webmail_test.go
  workers/
    workers.go
    workers_test.go
```

## Test Structure

**Suite Organization:**
```go
func TestFeature_Scenario(t *testing.T) {
    // arrange
    // act
    // assert
}
```

**Patterns:**
- Test names follow `Test<Feature>_<Scenario>` (e.g., TestLoader_Load_FromFile, TestCORS_AllowedOrigin)
- No table-driven tests observed; each scenario is a separate top-level function
- `t.Fatalf` for setup/assertion failures that make remaining checks invalid
- `t.Errorf` for assertion failures where subsequent checks may still provide useful info
- No `beforeEach`/`afterEach` equivalent; setup inlined or extracted to helper functions

## Mocking

**Framework:**
- Manual mock structs implementing interfaces (no mock generation library)
- Define minimal structs in test files that satisfy required interfaces

**Patterns:**
```go
// In-memory store implementing the interface
type mockStore struct {
    messages []*models.Message
    labels   []*models.Label
}

func (m *mockStore) GetMessage(ctx context.Context, ...) (*models.Message, error) {
    for _, msg := range m.messages {
        if msg.ID == messageID {
            return msg, nil
        }
    }
    return nil, mailstore.ErrNotFound
}
```

**What to Mock:**
- Database stores (mockStore implements mailstore.Store)
- Authentication service (mockAuth implements minimal auth interface)
- Redis (use miniredis in-memory server for real Redis behavior)
- External HTTP/webhook endpoints (use httptest)
- Time (inject `nowFunc` into structs to control time in tests)

**What NOT to Mock:**
- Standard library utilities (uuid, json, strings)
- Pure functions without side effects
- Internal business logic when testing at the same layer

## Fixtures and Factories

**Test Data:**
- Inline creation in each test function
- No centralized fixture files or factory libraries
- Temp directories via `t.TempDir()` for file-based tests
- Environment variables via `t.Setenv()` for config loader tests

**Setup Helpers:**
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

**Location:**
- Setup helpers defined in the same `*_test.go` file that uses them
- `newTestHandler()` returns initialized handler + mock store for webmail tests

## Coverage

**Requirements:**
- No explicit coverage target configured
- No CI enforcement detected
- Coverage for awareness and development confidence

**Configuration:**
- Built-in Go coverage via `-cover` flag
- No exclusions configured

**View Coverage:**
```bash
go test -cover ./...
go test -coverprofile=c.out ./... && go tool cover -html=c.out
```

## Test Types

**Unit Tests:**
- Test single functions/methods in isolation
- Mock all external dependencies (stores, services)
- Fast execution; no network or real database required

**Integration Tests:**
- Redis integration via miniredis (real Redis protocol, in-memory)
- HTTP integration via `httptest.NewRecorder` and `httptest.NewRequest`
- No observed PostgreSQL integration tests (database layer mocked)

**E2E Tests:**
- No end-to-end or browser automation tests detected
- Docker Compose provides runtime environment for manual/integration testing

## Common Patterns

**Async Testing:**
- Redis dequeue uses `miniredis.FastForward(time.Second)` to avoid blocking
- Worker pool tests use `time.Sleep` with short durations to allow goroutine scheduling
- Context cancellation used to stop background workers cleanly

**Error Testing:**
```go
func TestVerifyPassword_InvalidHash(t *testing.T) {
    s := NewService(nil, 1, 64*1024, 4, "test-session-key")
    if s.verifyPassword("any", "nope") {
        t.Error("verifyPassword should fail for malformed hash")
    }
}
```

**HTTP Handler Testing:**
```go
req := httptest.NewRequest(http.MethodPost, "/api/v1/drafts", bytes.NewReader(reqBody))
rr := httptest.NewRecorder()
h.createDraft(rr, req)
if rr.Code != http.StatusCreated {
    t.Errorf("status = %d, want %d", rr.Code, http.StatusCreated)
}
```

**Context Injection for Tests:**
```go
req = req.WithContext(api.WithUser(req.Context(), &models.User{...}))
chiCtx := chi.NewRouteContext()
chiCtx.URLParams.Add("id", draftID.String())
req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
```

## External Test Dependencies

**In-Memory Infrastructure:**
- `github.com/alicebob/miniredis/v2` for Redis testing
- `net/http/httptest` for HTTP handler testing

**No Test Database:**
- PostgreSQL is mocked, not integration-tested
- docker-compose.yml provides Postgres/Redis for runtime/manual testing

---

*Testing analysis: 2026-05-17*
*Update when test patterns change*

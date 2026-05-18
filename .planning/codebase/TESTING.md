# Testing Patterns

**Analysis Date:** 2026-05-18

## Go Test Framework

**Runner:** Go standard `testing` package (Go 1.25)
- No external assertion library ( testify, ginkgo, etc. not used )
- Config: none needed; `go test ./...` from Makefile

**Run Commands:**
```bash
make test              # Run all Go tests
go test ./...          # Direct invocation
go test -v ./internal/auth   # Verbose single package
```

## Go Test File Organization

**Location:** Co-located in same package as source code
- `internal/auth/auth_test.go` tests `internal/auth/auth.go`
- `internal/calendar/ical_test.go` tests `internal/calendar/ical.go`

**Naming:** `Test[Function]_[Scenario]`
- `TestHashAndVerifyPassword`
- `TestLoginServer_InvalidCredentials`
- `TestDedup_TTLEviction`

## Go Test Structure

**Standard Pattern:**
```go
func TestCreateDraft(t *testing.T) {
    h, store := newTestHandler()

    reqBody, _ := json.Marshal(map[string]any{
        "subject": "Hello",
        "to": []map[string]string{{"address": "bob@example.com"}},
    })
    req := httptest.NewRequest(http.MethodPost, "/api/v1/drafts", bytes.NewReader(reqBody))
    req = req.WithContext(api.WithUser(req.Context(), &models.User{...}))
    rr := httptest.NewRecorder()

    h.createDraft(rr, req)

    if rr.Code != http.StatusCreated {
        t.Errorf("status = %d, want %d", rr.Code, http.StatusCreated)
    }
    if len(store.messages) != 1 {
        t.Fatalf("expected 1 message, got %d", len(store.messages))
    }
}
```

**Assertion Style:**
- Setup failures: `t.Fatalf("setup: %v", err)`
- Value mismatches: `t.Errorf("field = %q, want %q", got, want)`
- Boolean checks: `t.Error("expected condition")` / `t.Fatal("unexpected condition")`
- No table-driven tests observed; each scenario is a separate test function

## Go Mocking

**Framework:** Hand-rolled interface implementations

**Pattern:**
```go
type mockStore struct {
    messages []*models.Message
    labels   []*models.Label
}

func (m *mockStore) CreateMessage(ctx context.Context, msg *models.Message, labelIDs []uuid.UUID, attachments []*models.Attachment) error {
    m.messages = append(m.messages, msg)
    return nil
}
```

**Common Mock Types:**
- `mockStore` - implements `mailstore.Store` interface for handler tests (`internal/webmail/webmail_test.go`)
- `mockAuth` - implements `DomainLister` / auth interface (`internal/webmail/webmail_test.go`)
- `testProcessor` - implements `workers.Processor` for worker tests (`internal/workers/workers_test.go`)

**What to Mock:**
- Database stores (via interface implementations)
- Authentication services
- External API clients (postmark)
- Redis (via `miniredis` in-memory server)

**What NOT to Mock:**
- Standard library utilities (UUID generation, JSON encoding)
- HTTP request/response objects (use `httptest` directly)

## Go Test Fixtures and Setup

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

func newTestHandler() (*Handler, *mockStore) {
    store := &mockStore{}
    return NewHandler(store, &mockAuth{}, nil, 25<<20), store
}
```

**Time Control:**
- Injectable clock functions for deterministic timing
  ```go
  p.nowFunc = func() time.Time { return now }
  ```
- `miniredis.FastForward(duration)` for TTL testing

**Temp Files:**
```go
dir := t.TempDir()
path := filepath.Join(dir, "postnest.conf")
```

**Environment Variables:**
```go
t.Setenv("POSTNEST_DATABASE_DSN", "postgres://env@localhost/env")
```

## Go Coverage

**Requirements:** None enforced

**View Coverage:**
```bash
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Go Test Types

**Unit Tests:**
- Pure logic tests: `internal/calendar/ical_test.go` (round-trip ICS encoding)
- Password hashing: `internal/auth/auth_test.go`
- DTO conversion: `internal/webmail/dto_test.go`

**Integration Tests (with in-memory dependencies):**
- Redis-backed deduplication: `internal/webhook/webhook_test.go` (uses `miniredis`)
- Worker pool retry/dead-letter: `internal/workers/workers_test.go` (uses `miniredis`)
- Config loader: `internal/config/loader_test.go` (uses temp files + env vars)

**HTTP Handler Tests:**
- Middleware: `internal/api/middleware_test.go` (CORS, rate limiter, recovery, cookies)
- CSRF: `internal/api/csrf_test.go`
- Error handling: `internal/api/errors_test.go`
- Webmail handlers: `internal/webmail/webmail_test.go` (uses `httptest.NewRecorder`)
- SMTP session: `internal/smtp/smtp_test.go`

## Web Test Framework

**Runner:** Vitest 4.1.6
- Config: `web/vite.config.js`
- Environment: `jsdom`
- Globals enabled (`describe`, `it`, `expect` available without import in test files)

**Assertion Library:**
- Vitest built-in `expect`
- `@testing-library/jest-dom` for DOM matchers (`toBeInTheDocument`, `toBeTruthy`)

**Run Commands:**
```bash
cd web && npm test              # Run all tests
cd web && npm run test:cov     # Coverage with v8 provider
```

## Web Test File Organization

**Location:** `web/src/test/`
- `setup.js` - test bootstrap
- Component tests: `messageview.test.jsx`
- Utility tests: `api.test.js`, `richeditor.test.js`

**Naming:** `[subject].test.[ext]`

## Web Test Structure

**Pattern:**
```javascript
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'

describe('MessageView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders html_body in a sandboxed iframe', async () => {
    getMessage.mockResolvedValue({ id: '1', subject: 'Hi', html_body: '<p>Rich</p>' })
    const { container } = renderAt('1')
    await waitFor(() => {
      const iframe = container.querySelector('iframe[title="message-body"]')
      expect(iframe).toBeTruthy()
    })
  })
})
```

## Web Mocking

**Framework:** Vitest `vi` namespace

**Pattern:**
```javascript
vi.mock('../api', () => ({
  getMessage: vi.fn(),
  patchMessage: vi.fn().mockResolvedValue({}),
}))
```

**MSW:** Installed (`msw` in devDependencies) but not used in existing tests

**What to Mock:**
- API module imports (`../api`)
- React Router context via `MemoryRouter`

## Web Fixtures

**No dedicated fixture files.**
- Inline mock data in test files
- Component rendering helper functions:
  ```javascript
  function renderAt(id) {
    return render(
      <MemoryRouter initialEntries={[`/message/${id}`]}>
        <Routes>
          <Route path="/message/:id" element={<MessageView />} />
        </Routes>
      </MemoryRouter>
    )
  }
  ```

## Web Coverage

**Requirements:** None enforced

**Config:**
```javascript
// vite.config.js
coverage: {
  provider: 'v8',
  reporter: ['text', 'json-summary'],
}
```

## Common Async Patterns

**Go:**
- Context with timeout for DB/external calls: `ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second); defer cancel()`
- Background job polling with `select { case <-ctx.Done(): return; default: }`

**JavaScript:**
- `async/await` with `try/catch` in components
- `waitFor` from `@testing-library/react` for async DOM assertions

---

*Testing analysis: 2026-05-18*

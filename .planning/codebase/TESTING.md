# Testing Patterns

**Analysis Date:** 2026-05-18

## Test Framework

**Go:**
- Runner: Standard `testing` package (Go 1.25)
- No external assertion libraries (no testify, no ginkgo)
- Run command: `go test ./...` (via `Makefile`)

**Frontend:**
- Runner: Vitest 4.1.6 (configured inside `vite.config.js`)
- Assertion: Vitest `expect` + `@testing-library/jest-dom` matchers
- Config: `web/vite.config.js`
- Run commands:
```bash
cd web && npm test              # Run all tests (vitest run)
cd web && npm run test:cov      # Run with coverage (vitest run --coverage)
```

## Test File Organization

**Go:**
- Location: Co-located with source in same package
- Naming: `*_test.go` (e.g., `auth_test.go`, `webmail_test.go`)
- Package declaration: same package name as source (e.g., `package auth`)

**Frontend:**
- Location: `web/src/test/` (separate test directory)
- Naming: `[name].test.js` or `[name].test.jsx`
- Files:
  - `web/src/test/setup.js` — test setup
  - `web/src/test/api.test.js` — API utility tests
  - `web/src/test/messageview.test.jsx` — React component tests
  - `web/src/test/richeditor.test.js` — Editor utility tests

## Test Structure

**Go Suite Organization:**
```go
func TestHashAndVerifyPassword(t *testing.T) {
    s := NewService(nil, 1, 64*1024, 4, "test-session-key")

    hash, err := s.hashPassword("correct-horse-battery-staple")
    if err != nil {
        t.Fatalf("hashPassword failed: %v", err)
    }
    if hash == "" {
        t.Fatal("hashPassword returned empty string")
    }

    if !s.verifyPassword("correct-horse-battery-staple", hash) {
        t.Error("verifyPassword should succeed for correct password")
    }
}
```

**Patterns:**
- Use `t.Fatalf` for setup failures that prevent test continuation
- Use `t.Errorf` / `t.Error` for assertion failures that allow remaining checks
- No table-driven tests observed in current test suite

**Frontend Suite Organization:**
```javascript
import { describe, it, expect, vi, beforeEach } from 'vitest'

describe('MessageView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders html_body in a sandboxed iframe', async () => {
    getMessage.mockResolvedValue({ ... })
    const { container } = renderAt('1')
    await waitFor(() => {
      const iframe = container.querySelector('iframe[title="message-body"]')
      expect(iframe).toBeTruthy()
    })
  })
})
```

## Mocking

**Go:**
- Framework: None (hand-rolled structs implementing interfaces)
- Pattern: define a `mock{type}` struct that satisfies the interface under test

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
// ... remaining interface methods
```

- In-memory state tracking on the mock struct itself
- Mock functions embedded directly in test file

**Frontend:**
- Framework: Vitest `vi.mock()`
- Pattern: mock the API module at import time

Example from `web/src/test/messageview.test.jsx`:
```javascript
vi.mock('../api', () => ({
  getMessage: vi.fn(),
  patchMessage: vi.fn().mockResolvedValue({}),
  deleteMessage: vi.fn(),
}))
```

- Clear mocks in `beforeEach`
- `mockResolvedValue` / `mockRejectedValue` for async functions

**What to Mock:**
- External services (Redis via `miniredis`)
- Database layer (via `mockStore` implementing `mailstore.Store`)
- API calls in frontend (via `vi.mock`)

**What NOT to Mock:**
- Standard library functions (use real `httptest`, real `time.Now`)
- No evidence of mocking `pgx` directly (SQL tests appear absent)

## Fixtures and Factories

**Go:**
- No dedicated fixture files
- Setup helpers return configured objects:
  ```go
  func setupTestPool(t *testing.T) (*Pool, *miniredis.Miniredis, *time.Time) { ... }
  func newTestHandler() (*Handler, *mockStore) { ... }
  ```
- Inline test data creation (e.g., `t.TempDir()`, `os.WriteFile`)

**Frontend:**
- Inline mock data in test cases (e.g., `getMessage.mockResolvedValue({ id: '1', subject: 'Hi' })`)
- No factory functions observed

## Coverage

**Frontend:**
- Provider: `@vitest/coverage-v8`
- Reporters: `text`, `json-summary`
- View: `cd web && npm run test:cov`

**Go:**
- No enforced coverage target detected
- Standard: `go test -cover ./...`

## Test Types

**Unit Tests:**
- Go: handler logic, domain logic, utility functions (e.g., `TestToMessageDTO`, `TestSnippetTruncates`)
- Frontend: pure utility functions (`parseRecipients`, `htmlToText`), component rendering

**Integration Tests:**
- Limited scope; mostly unit tests with mocked dependencies
- Redis tests use `miniredis` (in-memory Redis)

**E2E Tests:**
- Not used

## Common Patterns

**Go Async/HTTP Testing:**
```go
req := httptest.NewRequest(http.MethodPost, "/api/v1/drafts", bytes.NewReader(reqBody))
req = req.WithContext(api.WithUser(req.Context(), &models.User{...}))
rr := httptest.NewRecorder()
h.createDraft(rr, req)

if rr.Code != http.StatusCreated {
    t.Errorf("status = %d, want %d", rr.Code, http.StatusCreated)
}
```

**Go Context Injection:**
- Tests inject user/domain into context via helper functions:
  ```go
  func WithUser(ctx context.Context, user *models.User) context.Context
  ```

**Go Redis Testing:**
```go
func setupTestRedis(t *testing.T) (*Client, *miniredis.Miniredis) {
    m := miniredis.RunT(t)
    c, err := New("redis://" + m.Addr())
    if err != nil {
        t.Fatalf("new redis client: %v", err)
    }
    return c, m
}
```

**Frontend Component Testing:**
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

**Frontend Async Testing:**
```javascript
await waitFor(() => {
  const iframe = container.querySelector('iframe[title="message-body"]')
  expect(iframe).toBeTruthy()
})
```

**Time Manipulation:**
- Go: inject `nowFunc` into structs to override time in tests (e.g., `Pool.nowFunc`)
- `miniredis.FastForward(duration)` for advancing Redis TTLs

---

*Testing analysis: 2026-05-18*

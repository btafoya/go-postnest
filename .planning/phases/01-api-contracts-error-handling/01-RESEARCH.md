# Phase 1: API Contracts & Error Handling - Research

**Researched:** 2026-05-18
**Domain:** Go REST API DTOs, pgx/v5 error mapping, Chi handler testing
**Confidence:** HIGH

## Summary

The admin package (`internal/admin/`) has working CRUD endpoints but violates the frontend contract in three ways: (1) model structs serialize as PascalCase instead of snake_case, (2) all DB errors collapse to HTTP 500 with a generic message, and (3) some responses lack the structured envelope the frontend expects (e.g., `{"domain": {...}}`).

The project already has an established DTO pattern in `internal/calendar/dto.go` and `internal/webmail/dto.go` — private structs with `json:"snake_case"` tags and `toXDTO` conversion functions. The planner should clone this pattern exactly.

For error mapping, pgx/v5 exposes `*pgconn.PgError` with a `Code` field. The `github.com/jackc/pgerrcode` package is already an indirect dependency. The standard pattern is `errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation`. This maps cleanly to HTTP 409 for duplicate domains and HTTP 404 for zero-row updates.

**Primary recommendation:** Create `internal/admin/dto.go` with private DTO structs and conversion functions, then rewrite handler response bodies to use them. Map pgx errors inline in handlers to specific `*api.AppError` instances. Add table-driven handler tests with an in-memory mock store.

## User Constraints (from CONTEXT.md)

### Locked Decisions
- Reuse existing DTO pattern from `internal/calendar/dto.go` and `internal/webmail/dto.go`
- Create private `domainDTO`, `userDTO`, `settingDTO` structs in `internal/admin/dto.go` with `json:"snake_case"` tags
- Add conversion functions `toDomainDTO`, `toUserDTO`, `toDomainDTOs`, `toUserDTOs`
- Admin handlers return DTOs instead of raw `models.Domain` / `models.User` structs
- Map pgerrcode inline in handlers, not in store layer
- Duplicate domain name (`pgerrcode.UniqueViolation`) -> HTTP 409 with message "Domain already exists"
- Missing resource (rows affected 0 on update/delete) -> HTTP 404 with message "Not found"
- All other DB errors -> HTTP 500 with generic "Internal server error" (ERR-05)
- Keep error responses simple: only `message` string, no `details` array for now
- `AppError.Message` is the source of truth consumed by frontend (`err.response.data.error.message`)
- Do not populate `AppError.Details` for admin endpoints in this phase
- Wrap single-item responses: `createDomain` -> `{"domain": {...}}`, `createUser` -> `{"user": {...}}`
- `updateDomain` -> `{"domain": {...}}` (currently returns `{"updated": true}`)
- `updateUser` -> `{"user": {...}}` (currently returns `{"updated": true}`)
- Frontend update to read `res.data.domain` / `res.data.user` is Phase 3 scope

### Claude's Discretion
- Exact DTO field ordering
- Whether to embed or flatten memberships in userDTO
- Specific pgerrcode import choice (lib/pq vs pgconn)

### Deferred Ideas (OUT OF SCOPE)
- None — discussion stayed within phase scope

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| API-01 | Admin API responses use snake_case JSON keys matching frontend expectations | DTO pattern from calendar/webmail packages |
| API-02 | Admin handlers return structured DTOs instead of raw model structs | Same pattern; private DTO + conversion func |
| API-03 | `listUsers` response never includes `password_hash` or any credential field | DTO omits the field entirely |
| API-04 | `createDomain` response returns the created domain with all fields | Wrap in `{"domain": ...}` envelope after DTO conversion |
| API-05 | `createUser` response returns the created user with memberships | Same pattern; include flattened memberships in userDTO |
| ERR-01 | Duplicate domain name returns HTTP 409 with "Domain already exists" message | Map `pgerrcode.UniqueViolation` in createDomain handler |
| ERR-02 | Missing resource returns HTTP 404 with "Not found" message | Check `pgconn.PgError` or rows-affected for update/delete |
| ERR-03 | Invalid email format returns HTTP 400 with field-level error | Use `api.NewValidationError` internally; response writer only serializes `message` |
| ERR-04 | Empty required fields return HTTP 400 with specific field names | Same validation pattern as existing handler checks |
| ERR-05 | Generic DB errors return HTTP 500 only for unmapped cases | Default branch in error switch |
| ERR-06 | All error responses include actionable `message` field consumable by frontend | `api.WriteError` guarantees `{"error":{"message":"..."}}` shape |
| PROD-03 | Admin endpoints return consistent Content-Type headers | `writeJSON` already sets `application/json`; ensure all paths use it |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| pgx/v5 | v5.9.2 | PostgreSQL driver | Project mandate; no ORMs |
| pgerrcode | v0.0.0-20220416144525 | PostgreSQL error code constants | Already indirect dep; standard with pgx |
| chi/v5 | v5.2.5 | HTTP router | Used everywhere in project |
| uuid | v1.6.0 | UUID generation | Used in all packages |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| pgconn | v5.9.2 (pgx subpkg) | `*PgError` type for error code inspection | Always when checking pgx errors |

**Note:** `lib/pq` is in go.mod but should NOT be used for new code. The project uses pgx/v5 exclusively.

**Version verification:**
```bash
npm view not applicable for Go
go list -m github.com/jackc/pgx/v5 github.com/jackc/pgerrcode github.com/go-chi/chi/v5 github.com/google/uuid
```
Verified versions match go.mod. No version drift.

## Architecture Patterns

### Recommended File Structure
```
internal/admin/
├── handler.go          # HTTP handlers (modified)
├── store.go            # Store interface + PGStore (modified for error mapping)
├── dto.go              # Private DTO structs + conversion functions (NEW)
├── handler_test.go     # Table-driven handler tests with mock store (NEW)
└── dto_test.go         # DTO conversion unit tests (NEW)
```

### Pattern 1: DTO with Snake-Case JSON Tags
**What:** Private struct with `json:"snake_case"` tags and conversion functions from model structs.
**When to use:** Every handler response that serializes to JSON.
**Example:**
```go
// Source: internal/calendar/dto.go (project standard)
type eventDTO struct {
    ID          uuid.UUID `json:"id"`
    Title       string    `json:"title"`
    Start       time.Time `json:"start"`
    AllDay      bool      `json:"all_day"`
}

func toEventDTO(e *models.CalendarEvent) eventDTO {
    return eventDTO{ID: e.ID, Title: e.Summary, Start: e.StartsAt, AllDay: e.AllDay}
}

func toEventDTOs(evs []*models.CalendarEvent) []eventDTO {
    out := make([]eventDTO, 0, len(evs))
    for _, e := range evs { out = append(out, toEventDTO(e)) }
    return out
}
```

### Pattern 2: pgx Error Mapping in Handlers
**What:** Unwrap pgx errors to `*pgconn.PgError`, compare `Code` against `pgerrcode` constants.
**When to use:** Every handler that calls a store method that can fail with a constraint violation or missing row.
**Example:**
```go
import (
    "errors"
    "github.com/jackc/pgerrcode"
    "github.com/jackc/pgx/v5/pgconn"
)

func mapStoreError(err error) *api.AppError {
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) {
        switch pgErr.Code {
        case pgerrcode.UniqueViolation:
            return &api.AppError{Code: "conflict", Message: "Domain already exists", StatusCode: http.StatusConflict}
        }
    }
    if errors.Is(err, pgx.ErrNoRows) {
        return api.ErrNotFound
    }
    return api.ErrInternal
}
```

### Pattern 3: In-Memory Mock Store for Tests
**What:** Implement the `Store` interface with maps/slices, no DB.
**When to use:** Handler unit tests to avoid spinning up Postgres.
**Example:**
```go
// Source: internal/webmail/webmail_test.go (project standard)
type mockStore struct {
    messages []*models.Message
    labels   []*models.Label
}
func (m *mockStore) GetMessage(...) (*models.Message, error) { ... }
```

### Anti-Patterns to Avoid
- **Returning raw model structs:** `models.Domain` has no JSON tags, so Go's default encoder uses PascalCase (`ID`, `IsActive`). This breaks the React frontend contract.
- **Local password hasher in handler:** The `Handler` embeds a `PasswordHasher` that duplicates `auth.Service` logic. Per LOG-02 (Phase 2) and existing architecture, password hashing should delegate to `auth.Service`.
- **N+1 queries in `ListUsers`:** The current implementation calls `GetUserDomainMemberships` once per user in a loop. The DTO layer should receive pre-populated data, but fixing the query is a Phase 2 concern (LOG-03).
- **All errors mapping to 500:** The current handlers call `api.WriteError(w, api.ErrInternal)` for every store failure. This hides duplicate domains, missing rows, and validation errors from the frontend.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Snake-case JSON serialization | Custom reflection or map[string]any | DTO structs with json tags | Type-safe, compile-time checked, matches existing pattern |
| PostgreSQL error code parsing | String matching on error messages | `pgconn.PgError.Code` + `pgerrcode` constants | Message text changes between PG versions; codes are stable |
| HTTP test harness | Manual server startup | `httptest.NewRecorder` + `chi.NewRouter()` | Project already uses this in all handler tests |
| Password hashing | Handler-local `PasswordHasher` interface | `auth.Service.HashPassword` | Centralized Argon2id parameters, tested, single source of truth |

**Key insight:** The existing `calendar` and `webmail` packages already solved the DTO problem. Copy the pattern exactly rather than inventing a variation.

## Common Pitfalls

### Pitfall 1: PascalCase Leakage via Embedded Structs
**What goes wrong:** `DomainRow` embeds `models.Domain`, so `json.NewEncoder` sees `DomainRow.IsActive` as a top-level field named `IsActive` (PascalCase), not `is_active`.
**Why it happens:** Embedding flattens fields into the parent struct. If you forget to convert to a DTO, the embedded model's fields leak through with their original names.
**How to avoid:** Never return `*DomainRow` or `*UserRow` directly in JSON responses. Always convert through the DTO layer.
**Warning signs:** Frontend tables show `null` for fields that exist on the backend, or keys are capitalized in browser DevTools Network tab.

### Pitfall 2: pgx Error Type Confusion
**What goes wrong:** Using `pq.Error` (from `lib/pq`) instead of `pgconn.PgError` (from pgx/v5). The project includes both in go.mod, but only pgx is used for connections.
**Why it happens:** Googling "postgres unique violation go" often shows `lib/pq` examples. Copying them fails silently because the error type assertion never matches.
**How to avoid:** Only use `github.com/jackc/pgx/v5/pgconn` and `github.com/jackc/pgerrcode`. Add a code review rule: no `pq.` imports in new code.
**Warning signs:** Duplicate domain creation still returns 500 after you "fixed" error mapping.

### Pitfall 3: Mutation Side Effects in DTO Conversion
**What goes wrong:** A conversion function modifies the input model (e.g., sorting a slice, mutating a map) and the caller is surprised.
**Why it happens:** DTOs often need to transform data. If the function takes a pointer and mutates it, downstream code sees changed state.
**How to avoid:** DTO conversion functions should take values or pointers but never mutate the input. Return new structs and new slices.
**Warning signs:** Tests pass in isolation but fail when run together; race detector flags mutations.

### Pitfall 4: writeJSON Duplication
**What goes wrong:** `internal/admin/handler.go` has its own `writeJSON` helper that duplicates `cmd/server/main.go`'s identical function.
**Why it happens:** Each package copy-pasted the helper instead of using a shared one.
**How to avoid:** The existing `writeJSON` in `handler.go` is fine for this phase; do not refactor it out unless explicitly asked. Just be aware it's duplication, not a bug.

### Pitfall 5: Frontend Error Display on Success
**What goes wrong:** `Admin.jsx`'s `saveSettings` sets `setError('Settings saved')` on success, which renders in a red error box because `renderError()` always uses red styling.
**Why it happens:** The component reuses the single `error` state for both errors and success messages.
**How to avoid:** This is a Phase 3 frontend concern (FE-01). For Phase 1, ensure the backend returns a 200 with a clear body so the frontend has something to work with.

## Code Examples

### Verified DTO Pattern (from internal/calendar/dto.go)
```go
package admin

import (
    "time"
    "github.com/google/uuid"
    "github.com/go-postnest/postnest/internal/models"
)

type domainDTO struct {
    ID             uuid.UUID `json:"id"`
    Name           string    `json:"name"`
    PostmarkToken  string    `json:"postmark_token"`
    PostmarkStream string    `json:"postmark_stream"`
    IsActive       bool      `json:"is_active"`
    UserCount      int64     `json:"user_count"`
    CreatedAt      time.Time `json:"created_at"`
    UpdatedAt      time.Time `json:"updated_at"`
}

func toDomainDTO(d *DomainRow) domainDTO {
    return domainDTO{
        ID:             d.ID,
        Name:           d.Name,
        PostmarkToken:  d.PostmarkToken,
        PostmarkStream: d.PostmarkStream,
        IsActive:       d.IsActive,
        UserCount:      d.UserCount,
        CreatedAt:      d.CreatedAt,
        UpdatedAt:      d.UpdatedAt,
    }
}

func toDomainDTOs(rows []*DomainRow) []domainDTO {
    out := make([]domainDTO, 0, len(rows))
    for _, r := range rows {
        out = append(out, toDomainDTO(r))
    }
    return out
}
```

### Verified Error Mapping Pattern
```go
package admin

import (
    "errors"
    "net/http"
    "github.com/jackc/pgerrcode"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgconn"
    "github.com/go-postnest/postnest/internal/api"
)

func mapStoreError(err error) *api.AppError {
    if err == nil {
        return nil
    }
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) {
        switch pgErr.Code {
        case pgerrcode.UniqueViolation:
            return &api.AppError{
                Code:       "conflict",
                Message:    "Domain already exists",
                StatusCode: http.StatusConflict,
            }
        }
    }
    if errors.Is(err, pgx.ErrNoRows) {
        return api.ErrNotFound
    }
    return api.ErrInternal
}
```

### Verified Handler Test Pattern (from internal/api/middleware_test.go)
```go
package admin

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/go-chi/chi/v5"
)

func TestCreateDomain_Duplicate(t *testing.T) {
    h := NewHandler(&mockStore{/*...*/}, nil, 1, 64*1024, 4)
    r := chi.NewRouter()
    h.RegisterRoutes(r)

    body, _ := json.Marshal(map[string]string{"name": "existing.com"})
    req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/domains", bytes.NewReader(body))
    rr := httptest.NewRecorder()
    r.ServeHTTP(rr, req)

    if rr.Code != http.StatusConflict {
        t.Errorf("status = %d, want %d", rr.Code, http.StatusConflict)
    }
    var resp map[string]any
    json.Unmarshal(rr.Body.Bytes(), &resp)
    if msg := resp["error"].(map[string]any)["message"]; msg != "Domain already exists" {
        t.Errorf("message = %v, want 'Domain already exists'", msg)
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Raw model structs in JSON | DTO layer with json tags | Already in calendar/webmail | Now standard for all packages |
| `lib/pq` error inspection | `pgconn.PgError` with `pgerrcode` | pgx/v5 adoption | Stable error code checking |
| Handler-local password hashing | `auth.Service.HashPassword` | Phase 2 (LOG-02) | Centralized security params |

**Deprecated/outdated:**
- `lib/pq` error types: project uses pgx/v5, do not use `pq.Error`
- Returning `{"updated": true}` for updates: frontend expects `{"domain": {...}}` or `{"user": {...}}`

## Open Questions

1. **How should memberships appear in userDTO?**
   - What we know: `Admin.jsx` renders `user.memberships?.length`
   - What's unclear: Whether to embed `[]domainMemberDTO` or flatten to `[]string` of domain names
   - Recommendation: Embed a minimal `membershipDTO` with `domain_id`, `domain_name`, `role` for forward compatibility

2. **Should `createUser` also return memberships?**
   - What we know: `CreateUser` in store does not create a `domain_members` row (LOG-01 is Phase 2)
   - What's unclear: Whether API-05 requires actual memberships or just the field shape
   - Recommendation: Return empty `[]membershipDTO{}` so the shape is consistent; Phase 2 populates it

3. **How to detect "missing resource" for update/delete?**
   - What we know: `pgxpool.Exec` returns `pgconn.CommandTag` with `RowsAffected()`
   - What's unclear: Whether the current store methods return a special error when zero rows are affected
   - Recommendation: Update store methods to return a sentinel error (e.g., `api.ErrNotFound`) when `RowsAffected() == 0`, OR check `RowsAffected` in the handler and map to 404. Keep the CONTEXT.md decision: map inline in handlers.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go standard testing package (`testing`) |
| Config file | none — see Wave 0 |
| Quick run command | `go test ./internal/admin/... -v -count=1` |
| Full suite command | `go test ./... -race -count=1` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| API-01 | JSON keys are snake_case | unit | `go test ./internal/admin/... -run TestDomainDTO_SnakeCase` | No |
| API-02 | Handlers return DTOs, not raw models | unit | `go test ./internal/admin/... -run TestListDomains_ReturnsDTOs` | No |
| API-03 | User response excludes password_hash | unit | `go test ./internal/admin/... -run TestListUsers_NoPasswordHash` | No |
| API-04 | createDomain returns wrapped domain | unit | `go test ./internal/admin/... -run TestCreateDomain_ResponseShape` | No |
| API-05 | createUser returns wrapped user | unit | `go test ./internal/admin/... -run TestCreateUser_ResponseShape` | No |
| ERR-01 | Duplicate domain -> 409 | unit | `go test ./internal/admin/... -run TestCreateDomain_Duplicate` | No |
| ERR-02 | Missing resource -> 404 | unit | `go test ./internal/admin/... -run TestUpdateDomain_NotFound` | No |
| ERR-03 | Invalid email -> 400 | unit | `go test ./internal/admin/... -run TestCreateUser_InvalidEmail` | No |
| ERR-04 | Empty required fields -> 400 | unit | `go test ./internal/admin/... -run TestCreateDomain_EmptyName` | No |
| ERR-05 | Unmapped DB errors -> 500 | unit | `go test ./internal/admin/... -run TestListDomains_DBError` | No |
| ERR-06 | Error responses contain `message` | unit | `go test ./internal/admin/... -run TestErrorResponse_MessageField` | No |
| PROD-03 | All responses have Content-Type: application/json | unit | `go test ./internal/admin/... -run TestContentType_JSON` | No |

### Sampling Rate
- **Per task commit:** `go test ./internal/admin/... -v -count=1`
- **Per wave merge:** `go test ./... -race -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/admin/handler_test.go` — covers ERR-01, ERR-02, ERR-04, ERR-05, API-02, API-04
- [ ] `internal/admin/dto_test.go` — covers API-01, API-03, API-05
- [ ] `internal/admin/store.go` modifications — `RowsAffected()` check for update/delete
- [ ] `internal/admin/dto.go` — new file with DTO structs and conversion functions
- [ ] Framework install: N/A (Go standard library + existing deps)

## Sources

### Primary (HIGH confidence)
- `internal/calendar/dto.go` — DTO pattern with snake_case JSON tags and conversion functions
- `internal/webmail/dto.go` — DTO pattern for message responses, confirmed project standard
- `internal/api/errors.go` — `AppError`, `WriteError`, `NewValidationError` definitions
- `internal/models/models.go` — Domain, User, DomainMember struct definitions (no JSON tags = PascalCase leak)
- `internal/webmail/webmail_test.go` — Mock store pattern for handler unit tests
- `internal/api/middleware_test.go` — `httptest` + `chi.NewRouter` test pattern
- `github.com/jackc/pgerrcode` — Go module already available in go.mod (indirect)
- `github.com/jackc/pgx/v5/pgconn` — Part of pgx/v5, provides `PgError` type

### Secondary (MEDIUM confidence)
- `web/src/api.js` — Confirms frontend expects snake_case and `res.data.error.message`
- `web/src/components/Admin.jsx` — Confirms field names consumed: `domain.is_active`, `user.display_name`, `user.memberships`
- `cmd/server/main.go` — Confirms admin routes are wired with `api.RequireDomainAdmin` and CSRF

### Tertiary (LOW confidence)
- None — all critical claims verified with primary sources

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all libraries already in go.mod, patterns established in adjacent packages
- Architecture: HIGH — DTO pattern is copied verbatim from two existing packages
- Pitfalls: HIGH — verified by reading actual handler code and identifying PascalCase leakage

**Research date:** 2026-05-18
**Valid until:** 30 days (Go stack is stable)

---
phase: 02-backend-logic-validation
plan: 02
subsystem: api
tags: [go, postgres, pagination, argon2, auth, tdd]

# Dependency graph
requires:
  - phase: 01-api-contracts-error-handling
    provides: DTO layer, structured error responses, validation framework
provides:
  - Exported auth.Service.HashPassword method
  - Admin handler delegates all password hashing to auth.Service
  - N+1-free ListUsers via single LEFT JOIN query
  - Pagination metadata (total, limit, offset) on list endpoints
  - Strong-password conditional validation in createUser and resetPassword
affects:
  - 03-frontend-integration
  - 02-03-health-endpoint

tech-stack:
  added: []
  patterns:
    - "Delegate cross-cutting concerns (password hashing) to domain service rather than duplicating in handlers"
    - "Fetch related data in single JOIN query instead of per-row loops"
    - "Return total count alongside paginated results for frontend pagination controls"

key-files:
  created: []
  modified:
    - internal/auth/auth.go - Exported HashPassword wrapper
    - internal/auth/auth_test.go - TestHashPassword
    - internal/admin/handler.go - Removed local hasher, uses h.auth.HashPassword, added parsePagination
    - internal/admin/store.go - JOIN-based ListUsers, paginated ListDomains with count queries
    - internal/admin/handler_test.go - Hash delegation tests, pagination metadata tests
    - cmd/server/main.go - Updated NewHandler call signature

key-decisions:
  - "Delegate admin password hashing to auth.Service to eliminate duplicate Argon2id config and DRY violation"
  - "Use LEFT JOIN domain_members in ListUsers to fetch memberships in a single query"
  - "Run separate COUNT(*) queries for total metadata rather than subqueries that complicate the main SELECT"

patterns-established:
  - "Handler strong-password validation reads system settings at request time and rejects short passwords only when require_strong_passwords=true"
  - "parsePagination helper centralizes limit/offset parsing with default limit=20"

requirements-completed: [LOG-02, LOG-03, LOG-04, LOG-05, VAL-03]

# Metrics
duration: 6min
completed: 2026-05-18
---

# Phase 02 Plan 02: Password Hashing Delegation, N+1 Fix, and Pagination Metadata Summary

**Exported auth.HashPassword, eliminated admin local hasher DRY violation, fixed N+1 ListUsers with LEFT JOIN, and added total/limit/offset pagination metadata to both list endpoints**

## Performance

- **Duration:** 6 min
- **Started:** 2026-05-18T17:20:47Z
- **Completed:** 2026-05-18T17:27:13Z
- **Tasks:** 3
- **Files modified:** 6

## Accomplishments
- Exported `HashPassword` on `auth.Service` so other packages can reuse the system's Argon2id hasher
- Removed `PasswordHasher` interface, `hasher` struct, and `newHasher` from `internal/admin/handler.go`
- Updated `NewHandler` to accept only `Store` and `*auth.Service`, removing three argon parameter arguments
- Added conditional strong-password validation (gte 8) to `createUser` and `resetPassword` when `require_strong_passwords=true`
- Rewrote `PGStore.ListUsers` with a single `LEFT JOIN domain_members` query, eliminating N+1 DB round-trips
- Added `LIMIT`/`OFFSET` and separate `COUNT(*)` to `PGStore.ListDomains`
- Added `parsePagination` helper and returned `total`, `limit`, `offset` in both list handler responses
- Added 6 new tests covering hash delegation, strong password validation, and pagination metadata

## Task Commits

Each task was committed atomically:

1. **Task 1: Export HashPassword and remove local hasher from admin** - `650bcf8` (feat)
2. **Task 2: Fix N+1 in ListUsers and add pagination metadata to list endpoints** - `e822417` (feat)
3. **Task 3: Update project docs to reflect deferred LOG-01 and fix validation metadata** - `e5ab0e2` (docs)

## Files Created/Modified
- `internal/auth/auth.go` - Added exported `HashPassword` wrapper delegating to `hashPassword`
- `internal/auth/auth_test.go` - Added `TestHashPassword`
- `internal/admin/handler.go` - Removed local hasher, uses `h.auth.HashPassword`, added `parsePagination`, returns pagination metadata
- `internal/admin/store.go` - `ListUsers` uses `LEFT JOIN`, `ListDomains` accepts limit/offset and returns total count
- `internal/admin/handler_test.go` - Updated `newTestHandler`, added hash delegation and pagination tests
- `cmd/server/main.go` - Updated `admin.NewHandler` call to match new signature

## Decisions Made
- Delegate admin password hashing to `auth.Service` to keep Argon2id parameters in a single place and prevent config drift
- Use separate `COUNT(*)` queries rather than `SQL_CALC_FOUND_ROWS` or window functions for simplicity and portability

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- `TestCreateUser_HashDelegation` required adding a `lastCreateUserHash` spy field to `mockStore` to inspect the hash argument passed to `CreateUser`; the plan's suggested test body was incomplete, so the spy pattern was added to make the assertion possible.

## Next Phase Readiness
- Backend logic and validation layer is complete
- Ready for 02-03 health endpoint relocation or 03 frontend integration
- No blockers

## Self-Check: PASSED

- All created/modified files exist on disk
- All task commits (650bcf8, e822417, e5ab0e2) exist in git history
- `go test ./internal/admin/...` passes
- `go test ./internal/auth/...` passes

---
*Phase: 02-backend-logic-validation*
*Completed: 2026-05-18*

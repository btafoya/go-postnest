---
phase: 01-api-contracts-error-handling
plan: 02
subsystem: api
tags: [postgres, pgerrcode, pgx, error-handling, http-status, table-driven-tests]

requires:
  - phase: 01-api-contracts-error-handling
    plan: 01
    provides: DTO layer and response envelopes

provides:
  - ErrNotFound sentinel in admin store with RowsAffected checks
  - mapStoreError helper mapping PostgreSQL errors to specific HTTP status codes
  - Validation errors with specific field-level messages
  - Table-driven handler tests covering all error and response scenarios

affects:
  - admin-ui
  - api-contracts

tech-stack:
  added: [github.com/jackc/pgerrcode]
  patterns:
    - "Store methods return ErrNotFound when RowsAffected == 0"
    - "mapStoreError helper centralizes error-to-HTTP mapping"
    - "Specific validation messages without Details array for frontend consumption"

key-files:
  created:
    - internal/admin/handler_test.go - Table-driven tests for all error and response scenarios
  modified:
    - internal/admin/store.go - Added ErrNotFound sentinel and RowsAffected checks to update/delete/toggle/reset methods
    - internal/admin/handler.go - Added mapStoreError helper, replaced all store-error paths, updated validation messages
    - go.mod - Added github.com/jackc/pgerrcode dependency
    - go.sum - Added github.com/jackc/pgerrcode checksums

key-decisions:
  - "Removed unused fmt import from handler.go to satisfy go vet"
  - "Deferred pre-existing race conditions in internal/workers to out-of-scope tracking"

patterns-established:
  - "RowsAffected check pattern: capture pgconn.CommandTag, check RowsAffected() == 0, return ErrNotFound"
  - "mapStoreError pattern: switch on pgerrcode for database errors, errors.Is for sentinels, fallback to 500"
  - "Validation error pattern: return &AppError with specific Message field instead of Details array"

requirements-completed: [ERR-01, ERR-02, ERR-03, ERR-04, ERR-05, ERR-06, PROD-03]

duration: 5min
completed: 2026-05-18
---

# Phase 01: Plan 02 — API Error Handling & Response Contracts Summary

**PostgreSQL error mapping to specific HTTP status codes (409/404/400/500) with actionable messages, plus table-driven handler tests covering all scenarios**

## Performance

- **Duration:** 5 min
- **Started:** 2026-05-18T13:20:19Z
- **Completed:** 2026-05-18T13:25:22Z
- **Tasks:** 3
- **Files modified:** 5

## Accomplishments

- Added `ErrNotFound` sentinel and `RowsAffected()` checks to all admin store update/delete/toggle/reset methods
- Implemented `mapStoreError` helper mapping `pgerrcode.UniqueViolation` to 409 Conflict, `ErrNotFound`/`pgx.ErrNoRows` to 404 Not Found, and unmapped errors to 500 Internal Server Error
- Replaced all generic `api.ErrInternal` store-error responses with resource-specific mapped errors
- Updated validation errors to use specific `&api.AppError{Message: "..."}` responses without Details array for direct frontend consumption
- Added email `@` validation in `createUser`
- Created 11 table-driven handler tests covering DTO shapes, password hash omission, duplicate detection, not-found responses, validation failures, database errors, message field presence, and Content-Type headers

## Task Commits

Each task was committed atomically:

1. **Task 1: Add ErrNotFound sentinel and RowsAffected checks to store.go** - `af08d23` (feat)
2. **Task 2: Add mapStoreError helper and update all handler error paths** - `daf91f5` (feat)
3. **Task 3: Create internal/admin/handler_test.go with table-driven error and response tests** - `84404ee` (test)

**Plan metadata:** `e0f8e9a` (docs: complete 01-02 plan)

## Files Created/Modified

- `internal/admin/store.go` - Added `ErrNotFound` sentinel; updated UpdateDomain, DeleteDomain, ToggleDomainActive, UpdateUser, DeleteUser, ResetPassword to check `ct.RowsAffected() == 0`
- `internal/admin/handler.go` - Added `mapStoreError` helper; replaced all store-error `api.ErrInternal` with mapped errors; updated validation to specific messages
- `internal/admin/handler_test.go` - Created with 11 tests: response shapes, error statuses (409, 404, 400, 500), messages, Content-Type
- `go.mod` - Added `github.com/jackc/pgerrcode`
- `go.sum` - Added `github.com/jackc/pgerrcode` checksums

## Decisions Made

- Removed unused `fmt` import from `handler.go` during Task 2 to satisfy `go vet` (deviation Rule 1)
- Deferred pre-existing race conditions in `internal/workers` package to out-of-scope tracking; not caused by current task changes

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Removed unused fmt import from handler.go**
- **Found during:** Task 2
- **Issue:** `go vet` failed with `"fmt" imported and not used` after adding imports per plan instructions
- **Fix:** Removed the unused `fmt` import from the import block
- **Files modified:** `internal/admin/handler.go`
- **Verification:** `go build ./internal/admin/...` and `go vet ./internal/admin/...` pass
- **Committed in:** `daf91f5` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Minor import cleanup. No scope creep.

## Issues Encountered

- `go test ./... -race -count=1` reveals pre-existing data races in `internal/workers` (`TestWorker_RetryAndDeadLetter`, `TestWorker_PromotesDelayed`). These are unrelated to admin package changes and were documented in `deferred-items.md` for future attention.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Admin API error handling is complete and tested
- Frontend can now consume specific, actionable error messages
- Ready for subsequent phases that build on admin API contracts

## Self-Check: PASSED

- All created files verified on disk
- All task commits verified in git history (`af08d23`, `daf91f5`, `84404ee`)

---
*Phase: 01-api-contracts-error-handling*
*Completed: 2026-05-18*

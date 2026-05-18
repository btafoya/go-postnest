---
phase: 01-api-contracts-error-handling
plan: "01"
subsystem: api
tags: [dto, json, snake_case, admin, go]

requires:
  - phase: none
    provides: []
provides:
  - Admin DTO layer with snake_case JSON keys
  - Handler responses wrapped in {domain:...} and {user:...} envelopes
  - Password hash elimination from all user responses
  - Unit tests covering DTO shape and conversion correctness
affects:
  - 01-api-contracts-error-handling-02
  - 02-backend-logic-validation
  - 03-frontend-integration-production-hardening

tech-stack:
  added: []
  patterns:
    - "Private DTO struct + conversion function pattern (internal/calendar/dto.go standard)"
    - "Envelope wrapping for single-item create/update responses"
    - "Zero-value time.Time for partial DTO construction in update handlers"

key-files:
  created:
    - internal/admin/dto.go
    - internal/admin/dto_test.go
  modified:
    - internal/admin/handler.go

key-decisions:
  - "Followed existing project DTO pattern from internal/calendar/dto.go: private structs, snake_case json tags, dedicated conversion functions"
  - "Left updateDomain and updateUser DTOs without timestamps because store does not return the updated row; zero-value times are acceptable per plan specification"
  - "Settings endpoints (getSettings/updateSettings) and toggleDomainActive left unchanged per plan; they already use appropriate map/struct shapes"

patterns-established:
  - "Admin package DTOs live in dto.go, private, with toXDTO and toXDTOs conversion functions"
  - "List endpoints return pluralized key ({domains: [...], users: [...]}); single-item endpoints return wrapped envelope ({domain: {...}, user: {...}})"
  - "User responses must never contain credential fields; DTO struct explicitly omits PasswordHash"

requirements-completed: [API-01, API-02, API-03, API-04, API-05, PROD-03]

duration: 3min
completed: 2026-05-18
---

# Phase 1 Plan 1: Admin DTO Layer and Response Envelopes Summary

**Admin API returns snake_case DTOs with wrapped envelopes, eliminating password_hash leakage and satisfying the React frontend contract.**

## Performance

- **Duration:** 3 min
- **Started:** 2026-05-18T13:14:57Z
- **Completed:** 2026-05-18T13:18:07Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments

- Created `internal/admin/dto.go` with `domainDTO`, `userDTO`, and `membershipDTO` structs using snake_case JSON tags
- Implemented 8 conversion functions (`toDomainDTO`, `toDomainDTOs`, `toDomainDTOFromModel`, `toMembershipDTO`, `toMembershipDTOs`, `toUserDTO`, `toUserDTOFromRow`, `toUserDTOs`)
- Updated `internal/admin/handler.go` to return DTOs in all domain/user endpoints
- Wrapped single-item create/update responses in `{"domain": {...}}` and `{"user": {...}}` envelopes
- Removed manual `PasswordHash = ""` clearing loop from `listUsers`; DTOs inherently omit the field
- Created `internal/admin/dto_test.go` with 6 table-driven and direct tests covering snake_case tags, password omission, membership arrays, and conversion correctness
- Verified `go build ./...`, `go vet ./internal/admin/...`, and `go test ./internal/admin/... -v -count=1` all pass

## Task Commits

Each task was committed atomically:

1. **Task 1: Create internal/admin/dto.go with DTO structs and conversion functions** - `280859e` (feat)
2. **Task 2: Update handler.go to return DTOs in all JSON responses** - `b9a3419` (feat)
3. **Task 3: Create internal/admin/dto_test.go with DTO shape tests** - `c8342e3` (test)

**Plan metadata:** TBD (final docs commit after this summary)

## Files Created/Modified

- `internal/admin/dto.go` - Private DTO structs and conversion functions for domain, user, and membership shapes
- `internal/admin/handler.go` - Updated listDomains, createDomain, updateDomain, listUsers, createUser, updateUser to return DTOs/envelopes
- `internal/admin/dto_test.go` - Unit tests for JSON shape, password omission, membership nesting, and batch conversion

## Decisions Made

- Followed existing project DTO pattern from `internal/calendar/dto.go`: private structs, snake_case json tags, dedicated conversion functions
- Left `updateDomain` and `updateUser` DTOs without timestamps because the store does not return the updated row; zero-value times are acceptable per plan specification
- `Settings` endpoints (`getSettings`/`updateSettings`) and `toggleDomainActive` left unchanged per plan; they already use appropriate map/struct shapes

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed TestToUserDTOs compile error due to non-existent PasswordHash field on userDTO**
- **Found during:** Task 3 (TestToUserDTOs test implementation)
- **Issue:** The plan acceptance criterion stated `Assert PasswordHash is empty string (DTO omits it)`, but `userDTO` struct intentionally omits the `PasswordHash` field entirely, making `out[0].PasswordHash` a compile error
- **Fix:** Changed the assertion to marshal the DTO to JSON, unmarshal into `map[string]any`, and verify that the key `password_hash` does not exist
- **Files modified:** `internal/admin/dto_test.go`
- **Verification:** `go test ./internal/admin/... -v -count=1` passes
- **Committed in:** `c8342e3` (Task 3 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Minor test implementation correction. No scope creep.

## Issues Encountered

- None beyond the test assertion adjustment documented above.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Admin DTO layer is complete and tested; handlers now produce correct JSON shapes
- Ready for Plan 01-02: Error mapping and handler tests (ERR-01..ERR-06, PROD-03)
- No blockers

---
*Phase: 01-api-contracts-error-handling*
*Completed: 2026-05-18*

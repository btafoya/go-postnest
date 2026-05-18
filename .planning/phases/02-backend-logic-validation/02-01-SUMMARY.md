---
phase: 02-backend-logic-validation
plan: 01
subsystem: api/validation
tags: [go-playground/validator, struct-tags, validation, admin, field-errors]

requires:
  - phase: 01-api-contracts-error-handling
    provides: api.FieldError, api.NewValidationError, mapStoreError helper
provides:
  - Package-level validator instance with RequiredStructEnabled
  - Custom domainname validator using RFC-compliant regex
  - mapValidationErrors helper converting validator errors to api.FieldError slice
  - JSON tag name function for lowercase field names in validation errors
  - Struct-tag validation on all admin request structs
  - Pagination param validation with default-preserving logic
  - Structured UUID parse error responses with field-level details
affects:
  - 02-02-password-hashing
  - 03-frontend-integration

tech-stack:
  added:
    - github.com/go-playground/validator/v10 (promoted from indirect to direct)
  patterns:
    - Struct-tag validation with validate tags and validate.Struct orchestration
    - Custom validator registration for domain names
    - JSON tag name mapping for frontend-friendly field errors

key-files:
  created:
    - internal/admin/validate.go - Package validator, custom domainname validator, mapValidationErrors
    - internal/admin/validate_test.go - Table-driven tests for error mapping and domainname validation
  modified:
    - internal/admin/handler.go - Added validate tags to request structs, replaced inline checks with validate.Struct, updated UUID errors
    - internal/admin/handler_test.go - Updated existing tests for details array, added validation scenario tests

key-decisions:
  - "Register json tag name func on validator so field errors use lowercase snake_case names matching the frontend"
  - "Validate pagination params only when explicitly provided, preserving default limit=20 behavior for absent params"
  - "Map UUID parse failures to api.NewValidationError with field=id and issue=uuid for consistency"

patterns-established:
  - "All new request structs get validate tags alongside json tags"
  - "Handler validation uses validate.Struct(req) followed by api.NewValidationError(mapValidationErrors(err))"
  - "Validation errors return details array with field/issue/param for frontend consumption"

requirements-completed: [VAL-01, VAL-02, VAL-04, VAL-05]

metrics:
  duration: 5min
  completed: "2026-05-18"
---

# Phase 2 Plan 1: Validation Layer Summary

**Structured go-playground/validator v10 integration with custom domainname validator, json tag field naming, and field-level error details for all admin request structs**

## Performance

- **Duration:** 5 min
- **Started:** 2026-05-18T17:12:54Z
- **Completed:** 2026-05-18T17:18:24Z
- **Tasks:** 2
- **Files modified:** 4 (2 created, 2 modified)

## Accomplishments
- Replaced all ad-hoc inline validation in admin handlers with structured validator struct tags
- Added custom `domainname` validator enforcing RFC-compliant domain name rules
- Registered JSON tag name function so validation errors expose lowercase field names
- Added `listParams` validation for pagination with default-preserving logic
- Updated UUID parse failures to return consistent `api.NewValidationError` with field-level details
- All existing and new tests pass (29 test cases)

## Task Commits

Each task was committed atomically:

1. **Task 1: Create validate.go with package-level validator and error mapper**
   - `72214a1` - test(02-01): add failing tests for validation layer and domainname validator
   - `18e8d2a` - feat(02-01): implement package-level validator and mapValidationErrors helper
2. **Task 2: Add validate tags to request structs and replace inline checks**
   - `58fa513` - feat(02-01): add struct tag validation to admin handlers and replace inline checks

## Files Created/Modified
- `internal/admin/validate.go` - Package-level validator with custom domainname rule and mapValidationErrors helper
- `internal/admin/validate_test.go` - Table-driven tests for error mapping, custom validator, and wrapped errors
- `internal/admin/handler.go` - Request structs with validate tags, validate.Struct calls, structured UUID errors
- `internal/admin/handler_test.go` - Updated legacy tests for details array format, added validation scenario tests

## Decisions Made
- Registered a JSON tag name function on the validator instance so that `e.Field()` returns the `json` tag value (e.g., `name` instead of `Name`). This ensures frontend-friendly field names in validation error details without manual mapping.
- For pagination validation, opted to validate only when query params are explicitly provided. Missing params default to `limit=20, offset=0` without triggering a validation error. Explicitly invalid params (e.g., `limit=0`, `limit=200`, `offset=-1`) return 400 with field-level details.
- Mapped UUID parse failures to `api.NewValidationError([]api.FieldError{{Field: "id", Issue: "uuid"}})` so all 400 responses from the admin package follow the same structured format.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed capitalized field names in validation errors**
- **Found during:** Task 2 (handler validation integration)
- **Issue:** `go-playground/validator` returns Go struct field names (e.g., `Name`, `Email`, `Password`) in `e.Field()`, but the frontend expects lowercase snake_case names (`name`, `email`, `password`). Tests asserting on lowercase field names failed.
- **Fix:** Added `validate.RegisterTagNameFunc` in `init()` to map struct fields to their `json` tag values, producing lowercase field names in all validation error details.
- **Files modified:** `internal/admin/validate.go`
- **Verification:** All table-driven validation tests pass with lowercase field assertions
- **Committed in:** `58fa513` (Task 2 commit)

**2. [Rule 3 - Blocking] Fixed import compilation failure in validate_test.go**
- **Found during:** Task 1 (test compilation)
- **Issue:** Added `api` import in `validate_test.go` for `api.FieldError` type, but the compiler marked it as unused because the variable type was inferred from `mapValidationErrors` return value rather than explicitly declared.
- **Fix:** Changed `got := mapValidationErrors(nil)` to `var got []api.FieldError = mapValidationErrors(nil)` to make the import explicit.
- **Files modified:** `internal/admin/validate_test.go`
- **Verification:** `go test` compiles and passes
- **Committed in:** `72214a1` (Task 1 commit)

---

**Total deviations:** 2 auto-fixed (1 bug, 1 blocking)
**Impact on plan:** Both fixes necessary for correctness and compilation. No scope creep.

## Issues Encountered
- The `go get` command for `go-playground/validator/v10` produced no output and did not modify `go.mod`/`go.sum` because the dependency was already present as an indirect dependency. This was expected and did not block execution.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Validation layer is complete and all admin handlers return structured field-level errors.
- Ready for 02-02: password hashing delegation, N+1 fix, and pagination metadata.
- No blockers.

## Self-Check: PASSED

- `internal/admin/validate.go` exists
- `internal/admin/validate_test.go` exists
- `.planning/phases/02-backend-logic-validation/02-01-SUMMARY.md` exists
- Commit `72214a1` found
- Commit `18e8d2a` found
- Commit `58fa513` found

---
*Phase: 02-backend-logic-validation*
*Completed: 2026-05-18*

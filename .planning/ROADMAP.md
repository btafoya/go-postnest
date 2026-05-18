# Roadmap: Go-PostNest — Admin Completion

## Overview

Complete the admin dashboard for production use by fixing API contracts, error handling, backend validation and logic, frontend integration, and production hardening. The journey moves from foundational contract fixes to backend logic repairs, then finishes with frontend polish and comprehensive tests.

## Phases

- [ ] **Phase 1: API Contracts & Error Handling** - Fix JSON shapes, DTOs, and structured errors
- [ ] **Phase 2: Backend Logic & Validation** - Fix input validation, memberships, hashing, pagination, and health endpoint
- [ ] **Phase 3: Frontend Integration & Production Hardening** - Polish UI feedback, enforce settings at runtime, and achieve full test coverage

## Phase Details

### Phase 1: API Contracts & Error Handling
**Goal**: Admin API returns correct data shapes and meaningful errors that the frontend can consume.
**Depends on**: Nothing (first phase)
**Requirements**: API-01, API-02, API-03, API-04, API-05, ERR-01, ERR-02, ERR-03, ERR-04, ERR-05, ERR-06, PROD-03
**Success Criteria** (what must be TRUE):
  1. Admin frontend receives correctly typed JSON responses with snake_case keys for all domain, user, and setting fields.
  2. User list response never contains password_hash or any credential field.
  3. Duplicate domain name returns HTTP 409 with "Domain already exists".
  4. Missing resource returns HTTP 404 with "Not found".
  5. Invalid inputs return HTTP 400 with specific, actionable field-level error messages.
**Plans:** 1/2 plans executed

Plans:
- [ ] 01-01: DTO layer and response envelopes (API-01..API-05, PROD-03)
- [ ] 01-02: Error mapping and handler tests (ERR-01..ERR-06, PROD-03)

### Phase 2: Backend Logic & Validation
**Goal**: Admin backend correctly validates inputs and fixes core logic gaps.
**Depends on**: Phase 1
**Requirements**: VAL-01, VAL-02, VAL-03, VAL-04, VAL-05, LOG-01, LOG-02, LOG-03, LOG-04, LOG-05, LOG-06
**Success Criteria** (what must be TRUE):
  1. Admin can create a user and the user is immediately assigned to at least one domain.
  2. Admin can create and update user passwords using the system's standard auth hashing service.
  3. Domain and user list views show correct total counts and load without N+1 query delays.
  4. Health endpoint returns DB, Redis, IMAP, SMTP, and worker queue metrics in a consistent JSON format.
  5. Admin receives clear validation errors for invalid domain names, emails, passwords, pagination params, or malformed UUIDs before any DB write occurs.
**Plans:** 2

Plans:
- [ ] 02-01: TBD

### Phase 3: Frontend Integration & Production Hardening
**Goal**: Admin frontend correctly displays data and handles all states, and the admin package is production-ready with tests.
**Depends on**: Phase 2
**Requirements**: FE-01, FE-02, FE-03, FE-04, FE-05, PROD-01, PROD-02, PROD-04, PROD-05
**Success Criteria** (what must be TRUE):
  1. Saving system settings shows a green success indicator; API errors render as readable red messages with backend details.
  2. Domain and user tables render all fields correctly without exposing credentials.
  3. Health dashboard displays DB, Redis, IMAP, SMTP, and worker queue metrics.
  4. User creation and password reset enforce the strong-password policy when enabled, and rate limiting respects runtime system settings.
  5. `go test ./internal/admin/...` passes with all handler methods covered.
**Plans:** 2

Plans:
- [ ] 03-01: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. API Contracts & Error Handling | 1/2 | In Progress|  |
| 2. Backend Logic & Validation | 0/TBD | Not started | - |
| 3. Frontend Integration & Production Hardening | 0/TBD | Not started | - |

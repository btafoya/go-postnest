# Requirements: Admin Dashboard Completion

**Defined:** 2026-05-18
**Core Value:** Admin can reliably manage domains, users, and security settings through the web UI without hitting missing or broken APIs

## v1 Requirements

### API Contracts

- [x] **API-01**: Admin API responses use snake_case JSON keys matching frontend expectations (`id`, `is_active`, `display_name`)
- [x] **API-02**: Admin handlers return structured DTOs instead of raw model structs to frontend
- [x] **API-03**: `listUsers` response never includes `password_hash` or any credential field
- [x] **API-04**: `createDomain` response returns the created domain with all fields
- [x] **API-05**: `createUser` response returns the created user with memberships

### Error Handling

- [x] **ERR-01**: Duplicate domain name returns HTTP 409 with "Domain already exists" message
- [x] **ERR-02**: Missing resource returns HTTP 404 with "Not found" message
- [x] **ERR-03**: Invalid email format returns HTTP 400 with field-level error
- [x] **ERR-04**: Empty required fields return HTTP 400 with specific field names
- [x] **ERR-05**: Generic DB errors return HTTP 500 only for unmapped cases
- [x] **ERR-06**: All error responses include actionable `message` field consumable by frontend

### Input Validation

- [x] **VAL-01**: Domain name validated as non-empty string with reasonable length limit
- [x] **VAL-02**: Email validated with RFC 5322-aware check before store call
- [x] **VAL-03**: Password minimum length enforced when `require_strong_passwords` is enabled
- [x] **VAL-04**: Pagination params validated (limit 1-100, offset >= 0)
- [x] **VAL-05**: UUID path params validated before store calls

### Backend Logic

- [ ] **LOG-01**: `createUser` creates corresponding `domain_members` row for at least one domain
- [x] **LOG-02**: Admin password hashing uses `auth.Service.HashPassword` instead of local hasher
- [x] **LOG-03**: `ListUsers` fetches all memberships in single query (fix N+1)
- [x] **LOG-04**: `ListDomains` returns pagination metadata (total count)
- [x] **LOG-05**: `ListUsers` returns pagination metadata (total count)
- [x] **LOG-06**: Health endpoint moved from `cmd/server/main.go` to `internal/admin/health.go`

### Frontend Integration

- [ ] **FE-01**: Admin `saveSettings` shows green success indicator instead of red error box
- [ ] **FE-02**: All admin API calls handle structured error responses and display messages
- [ ] **FE-03**: Domain table renders all fields from snake_case response
- [ ] **FE-04**: User table renders all fields without exposing credentials
- [ ] **FE-05**: Health dashboard displays all metrics from backend response

### Production Hardening

- [ ] **PROD-01**: `require_strong_passwords` setting enforced during user creation and password reset
- [ ] **PROD-02**: Rate limiting config read from `system_settings` at runtime
- [x] **PROD-03**: Admin endpoints return consistent Content-Type headers
- [ ] **PROD-04**: All admin handler methods covered by unit tests
- [ ] **PROD-05**: `go test ./internal/admin/...` passes

## v2 Requirements

### Audit & Analytics

- **AUDIT-01**: Admin actions logged to audit table (who, what, when)
- **AUDIT-02**: Domain membership management UI (add/remove users from domains)
- **AUDIT-03**: Delivery analytics dashboard (Postmark stats)
- **AUDIT-04**: Storage metrics (message counts, attachment sizes)

## Out of Scope

| Feature | Reason |
|---------|--------|
| DNS management | Postmark handles DNS; out of project scope |
| MTA management | We proxy SMTP to Postmark; no local MTA |
| Mailbox migration | Not needed for admin completion |
| Custom spam engine | Postmark provides spam headers |
| Mobile admin app | Web UI is the target platform |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| API-01 | Phase 1 | Complete |
| API-02 | Phase 1 | Complete |
| API-03 | Phase 1 | Complete |
| API-04 | Phase 1 | Complete |
| API-05 | Phase 1 | Complete |
| ERR-01 | Phase 1 | Complete |
| ERR-02 | Phase 1 | Complete |
| ERR-03 | Phase 1 | Complete |
| ERR-04 | Phase 1 | Complete |
| ERR-05 | Phase 1 | Complete |
| ERR-06 | Phase 1 | Complete |
| PROD-03 | Phase 1 | Complete |
| VAL-01 | Phase 2 | Complete |
| VAL-02 | Phase 2 | Complete |
| VAL-03 | Phase 2 | Complete |
| VAL-04 | Phase 2 | Complete |
| VAL-05 | Phase 2 | Complete |
| LOG-01 | Phase 2 | Pending |
| LOG-02 | Phase 2 | Complete |
| LOG-03 | Phase 2 | Complete |
| LOG-04 | Phase 2 | Complete |
| LOG-05 | Phase 2 | Complete |
| LOG-06 | Phase 2 | Complete |
| FE-01 | Phase 3 | Pending |
| FE-02 | Phase 3 | Pending |
| FE-03 | Phase 3 | Pending |
| FE-04 | Phase 3 | Pending |
| FE-05 | Phase 3 | Pending |
| PROD-01 | Phase 3 | Pending |
| PROD-02 | Phase 3 | Pending |
| PROD-04 | Phase 3 | Pending |
| PROD-05 | Phase 3 | Pending |

**Coverage:**
- v1 requirements: 32 total
- Mapped to phases: 32
- Unmapped: 0

---
*Requirements defined: 2026-05-18*
*Last updated: 2026-05-18 after roadmap creation*

---
phase: 2
slug: backend-logic-validation
status: draft
nyquist_compliant: true
wave_0_complete: true
created: 2026-05-18
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none |
| **Quick run command** | `go test ./internal/admin/...` |
| **Full suite command** | `go test ./...` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/admin/...`
- **After every plan wave:** Run `go test ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 2-01-01 | 01 | 1 | VAL-01, VAL-05 | unit | `go test ./internal/admin/... -run TestMapValidationErrors -v` | ✅ | ✅ green |
| 2-01-01 | 01 | 1 | VAL-01 | unit | `go test ./internal/admin/... -run TestDomainNameValidator -v` | ✅ | ✅ green |
| 2-01-02 | 01 | 1 | VAL-01 | unit | `go test ./internal/admin/... -run TestCreateDomain_Validation -v` | ✅ | ✅ green |
| 2-01-02 | 01 | 1 | VAL-02 | unit | `go test ./internal/admin/... -run TestCreateUser_Validation -v` | ✅ | ✅ green |
| 2-01-02 | 01 | 1 | VAL-04 | unit | `go test ./internal/admin/... -run TestListUsers_InvalidPagination -v` | ✅ | ✅ green |
| 2-01-02 | 01 | 1 | VAL-01 | unit | `go test ./internal/admin/... -run TestCreateDomain_EmptyName -v` | ✅ | ✅ green |
| 2-01-02 | 01 | 1 | VAL-02 | unit | `go test ./internal/admin/... -run TestCreateUser_InvalidEmail -v` | ✅ | ✅ green |
| 2-02-01 | 02 | 2 | LOG-02 | unit | `go test ./internal/auth/... -run TestHashPassword -v` | ✅ | ✅ green |
| 2-02-01 | 02 | 2 | LOG-02, VAL-03 | unit | `go test ./internal/admin/... -run TestCreateUser_HashDelegation -v` | ✅ | ✅ green |
| 2-02-02 | 02 | 2 | LOG-04 | unit | `go test ./internal/admin/... -run TestListDomains_PaginationMeta -v` | ✅ | ✅ green |
| 2-02-02 | 02 | 2 | LOG-05 | unit | `go test ./internal/admin/... -run TestListUsers_PaginationMeta -v` | ✅ | ✅ green |
| 2-02-02 | 02 | 2 | LOG-03 | unit | `go test ./internal/admin/... -run TestListUsers_Memberships -v` | ✅ | ✅ green |
| 2-03-01 | 03 | 3 | LOG-06 | unit | `go test ./internal/admin/... -run TestHealthHandler_RegisterRoutes -v` | ❌ W0 | ⬜ pending |
| 2-03-02 | 03 | 3 | LOG-06 | unit | `go test ./internal/admin/... -run TestHealthHandler -v` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements.

- [x] `internal/admin/handler_test.go` — existing handler tests
- [x] `internal/admin/dto_test.go` — existing DTO tests
- [x] `go test ./internal/admin/...` — passes

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Health endpoint IMAP/SMTP TCP dial | LOG-06 | Requires live IMAP/SMTP listeners | Start server, `curl /admin/api/v1/health`, verify `imap` and `smtp` fields are boolean |

*All other phase behaviors have automated verification.*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 10s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending

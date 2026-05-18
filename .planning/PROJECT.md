# Go-PostNest — Admin Completion

## What This Is

Complete the admin dashboard for production use. Fix broken or unwired backend APIs that the React admin frontend expects. Make domain management, user management, system settings, and health monitoring fully functional and production-ready.

## Core Value

Admin can reliably manage domains, users, and security settings through the web UI without hitting missing or broken APIs.

## Requirements

### Validated

- ✓ Admin dashboard React UI (4 tabs: overview, domains, users, security) — existing
- ✓ Domain CRUD APIs (list, create, update, delete, toggle active) — existing
- ✓ User CRUD APIs (list, create, update, delete, reset password) — existing
- ✓ System settings APIs (get, update) — existing
- ✓ Health check endpoint with DB/Redis/IMAP/SMTP/worker queue metrics — existing
- ✓ Admin database migration (`domains.is_active`, `system_settings` table) — existing
- ✓ Web UI route proxying for `/admin/api/*` — existing
- ✓ CSRF protection and domain-admin authorization on admin routes — existing
- ✓ Frontend API client functions for all admin endpoints — existing

### Active

- [ ] Identify and fix broken or unwired admin APIs
- [ ] Complete any missing APIs the React frontend calls but backend does not implement
- [ ] Production-harden admin endpoints (input validation, error messages, edge cases)
- [ ] Ensure admin frontend correctly handles all API responses and error states
- [ ] Verify admin migration runs cleanly on fresh and existing databases
- [ ] Confirm health endpoint returns all fields frontend expects

### Out of Scope

- New admin features beyond existing 4 tabs — defer to future milestone
- Redesign admin UI — work with existing React/Tailwind components
- Non-admin APIs (mail, contacts, calendar) — those are separate workstreams
- IMAP/SMTP protocol changes — admin is REST API only

## Context

This is a brownfield task on an existing Go-PostNest installation. The admin panel (`internal/admin/`, `web/src/components/Admin.jsx`) was built in prior phases but UAT revealed some APIs are broken or not fully wired. The backend uses Chi router with PostgreSQL via pgx. Frontend uses React 19 + Vite + Tailwind, axios with CSRF double-submit.

## Constraints

- **Tech stack**: Go + Chi + pgx/v5 + React 19 — no new dependencies
- **Architecture**: Follow existing `internal/admin/` package structure, don't redesign
- **Auth**: Must use existing `api.RequireDomainAdmin` middleware
- **Frontend**: Match existing Gmail-inspired styling, don't add new UI frameworks

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Fix existing admin package rather than rewrite | Code is mostly correct, just has gaps | — Pending |
| Keep frontend changes surgical | UI exists and is functional | — Pending |

---
*Last updated: 2026-05-18 after initialization*

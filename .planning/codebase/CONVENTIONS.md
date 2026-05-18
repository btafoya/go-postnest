# Coding Conventions

**Analysis Date:** 2026-05-18

## Languages

**Primary:**
- Go 1.25 - Backend services, API handlers, SMTP/IMAP workers, database layer
- JavaScript (JSX) - React frontend (Vite-based SPA)

## Naming Patterns

**Go Files:**
- Package matches directory name exactly: `package calendar`, `package auth`
- Core implementation: `handler.go`, `pgstore.go`, `models.go`, `dto.go`
- Entry points: `main.go` in `cmd/<binary>/`
- Tests: `<name>_test.go` co-located with source

**Go Types:**
- Exported types: PascalCase (`Service`, `Handler`, `Pool`, `Store`)
- Unexported types: camelCase (`smtpBackend`, `loginServer`, `testProcessor`)
- Interfaces: noun or `-er` suffix (`Store`, `DomainLister`, `Processor`)
- Struct tags: backtick-quoted with snake_case JSON keys: `` `json:"user_id"` ``

**Go Functions:**
- Exported: PascalCase (`NewService`, `Authenticate`, `RegisterRoutes`)
- Unexported: camelCase (`hashPassword`, `verifyPassword`, `setupTestRedis`)
- DTO conversion: `to[Entity]DTO` and `to[Entity]DTOs` (`toMessageDTO`, `toCalendarDTO`)
- Handler helpers: short names like `ctx(r)` to extract domain/user from request context

**Go Variables:**
- Short names in tight scopes (`m` for miniredis, `c` for client, `h` for handler)
- Constants for queue names: `queueJobs`, `queueDelayed`, `queueDead`

**JavaScript/React:**
- Components: PascalCase filename matching component name (`MessageView.jsx`, `RichEditor.jsx`)
- Default exports for page/components
- Named exports for utilities (`export function parseRecipients`, `export function htmlToText`)
- API functions: camelCase matching endpoint semantics (`getMessages`, `patchMessage`, `createDraft`)
- Props: camelCase (`htmlBody`, `onChange`)
- State variables: camelCase (`selectedIds`, `loading`, `currentLabel`)

## Code Style

**Formatting:**
- Go: Standard `gofmt` (no custom config detected)
- JavaScript: No explicit Prettier or ESLint config present
- Go imports are grouped: stdlib, third-party, internal module

**Go Import Order:**
```go
import (
    "context"
    "encoding/json"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/google/uuid"

    "github.com/go-postnest/postnest/internal/api"
    "github.com/go-postnest/postnest/internal/models"
)
```

**Tailwind CSS:**
- Custom color palette defined in `web/tailwind.config.js`
  - `primary-50` through `primary-700` (blue family)
  - `surface-50` through `surface-900` (gray family)
- Utility-first class names in JSX

## Error Handling

**Go Backend:**
- Unified `AppError` type in `internal/api/errors.go`
- Sentinel errors as package-level vars:
  ```go
  var (
      ErrNotFound     = &AppError{Code: "not_found", Message: "Resource not found", StatusCode: http.StatusNotFound}
      ErrUnauthorized = &AppError{Code: "unauthorized", Message: "Authentication required", StatusCode: http.StatusUnauthorized}
  )
  ```
- Validation errors: `NewValidationError([]FieldError{{Field: "domain_id", Issue: "required"}})`
- HTTP response pattern: `api.WriteError(w, err)` in all handlers
- Custom `As` wrapper around `errors.As` for error type assertion
- Panic recovery via middleware: `defer` + `recover()` logs stack trace and returns 500

**Frontend:**
- Axios response interceptor redirects to `/login` on 401 (excluding auth endpoints)
- Console logging for non-critical fetch failures (`console.error`)
- Inline error display via `notice` state variables in components

## Logging

**Framework:** `log/slog` (structured JSON)
- Logger creation: `internal/logger/logger.go`
- Pattern: key-value pairs after message string
  ```go
  logger.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start).String())
  slog.Error("job failed", "type", job.Type, "error", err)
  ```
- Request logging middleware records method, path, duration, and request ID

## Comments

**When to Comment:**
- Exported functions and types: brief purpose description
- Non-obvious logic: `// Accept the frontend contract (title/start/end) with backend aliases`
- Security-critical: `// Allow plaintext IMAP/SMTP auth without TLS`

**JSDoc/TSDoc:**
- Not used

## Function Design

**Go:**
- Constructor pattern: `New[Type](...)` returns pointer
- Service structs hold dependencies as fields (`pool`, `store`, `auth`)
- Handlers receive store/auth via constructor, not globals
- Short functions preferred; `ctx(r)` helper encapsulates repeated context extraction

**React:**
- Functional components with hooks
- Custom hook pattern not observed; direct `useState`/`useEffect` in components
- Callback memoization with `useCallback` for fetch functions

## Module Design

**Go:**
- No barrel files
- Each package is self-contained
- Interfaces defined in consumer packages or shared `internal/mailstore`
- `internal/` prefix enforced by Go module structure

**JavaScript:**
- `api.js` centralizes all API calls as named exports
- Components live in `web/src/components/`
- No index.js re-exports observed

---

*Convention analysis: 2026-05-18*

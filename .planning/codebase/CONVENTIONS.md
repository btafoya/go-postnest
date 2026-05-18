# Coding Conventions

**Analysis Date:** 2026-05-18

## Naming Patterns

**Go Files:**
- All lowercase, no underscores: `auth.go`, `webmail_test.go`, `dto.go`
- Test suffix: `*_test.go`, co-located with source
- Package name matches directory: `package webmail` in `internal/webmail/`

**Go Types:**
- Exported: PascalCase (e.g., `Service`, `Handler`, `AppError`, `MessagePatch`)
- Unexported: camelCase (e.g., `loginServer`, `addrDTO`, `labelOut`)
- DTO structs: suffix `DTO` (e.g., `addrDTO`, `messageDTO`)
- Interface names: describe capability (e.g., `DomainLister`, `Store`)

**Go Functions:**
- Exported: PascalCase (e.g., `NewService`, `Authenticate`, `WriteError`)
- Unexported: camelCase (e.g., `hashPassword`, `verifyPassword`, `snippet`, `extractAddresses`)
- Constructor pattern: `New{Type}` (e.g., `NewService`, `NewHandler`, `NewPool`)

**Go Variables:**
- Short names in tight scope (e.g., `u`, `did`, `ctx`)
- Descriptive names for fields and package-level vars (e.g., `maxAttachmentSize`, `sessionKey`)
- Error variables: `Err{Description}` prefix (e.g., `ErrNotFound`, `ErrUnauthorized`, `ErrInvalidCredentials`)

**Go Constants:**
- camelCase for unexported string constants (e.g., `queueJobs`, `queueDelayed`, `queueDead`)
- Context keys use typed string constants: `type ctxKey string; const ctxKeyUser ctxKey = "user"`

**Frontend Files:**
- Components: PascalCase with `.jsx` extension (e.g., `MessageView.jsx`, `RichEditor.jsx`)
- Utilities: camelCase with `.js` extension (e.g., `api.js`, `sse.js`)

**Frontend Components/Functions:**
- Components: PascalCase, default export (e.g., `export default function MessageView()`)
- API functions: camelCase (e.g., `getMessage`, `patchMessage`, `parseRecipients`)
- Hook variables: destructured from React imports (e.g., `useState`, `useEffect`)

## Code Style

**Formatting:**
- Go: standard `gofmt` (no custom formatter config detected)
- Frontend: no explicit formatter config; Vite handles build

**Linting:**
- No `.golangci.yml`, `.eslintrc`, or `biome.json` detected
- Only lint override observed: `// eslint-disable-next-line react-hooks/exhaustive-deps` in `RichEditor.jsx`

## Import Organization

**Go Import Order:**
1. Standard library
2. Third-party packages
3. Internal packages (full module path)

Example from `internal/webmail/webmail.go`:
```go
import (
    "encoding/json"
    "fmt"
    "context"
    // ... stdlib

    "github.com/google/uuid"
    "github.com/go-chi/chi/v5"
    // ... third-party

    "github.com/go-postnest/postnest/internal/api"
    "github.com/go-postnest/postnest/internal/mailstore"
    "github.com/go-postnest/postnest/internal/models"
    // ... internal
)
```

**Frontend Imports:**
- React and hooks first
- Third-party libraries (axios, DOMPurify, lucide-react)
- Local modules with relative paths (`../api`, `../components/MessageView`)

## Error Handling

**Go Patterns:**
- Return `error` as last value from every function that can fail
- Use sentinel errors for known conditions:
  ```go
  var ErrNotFound = &AppError{Code: "not_found", Message: "Resource not found", StatusCode: http.StatusNotFound}
  ```
- Custom error type: `api.AppError` with `Code`, `Message`, `Details`, `StatusCode`
- HTTP handler pattern: check error, call `api.WriteError(w, err)`, return immediately
  ```go
  msg, err := h.store.GetMessage(r.Context(), did, u.ID, id)
  if err != nil {
      api.WriteError(w, err)
      return
  }
  ```
- Database: check `pgx.ErrNoRows` explicitly to convert to domain errors
- No `panic` in production code; recovery middleware catches panics in HTTP layer

**Frontend Patterns:**
- API interceptor redirects to `/login` on 401 for non-auth endpoints
- Component errors logged via `console.error` (e.g., `.catch(console.error)`)
- DOMPurify for HTML sanitization before rendering

## Logging

**Framework:** `log/slog` (structured JSON logging)

**Patterns:**
- Use `slog.New(slog.NewJSONHandler(os.Stdout, ...))` for production
- Key-value pairs: `logger.Info("request", "method", r.Method, "path", r.URL.Path)`
- Error logging: `logger.Error("job failed", "type", job.Type, "error", err)`
- Panic recovery logs stack trace via `debug.Stack()`

## Comments

**When to Comment:**
- Every exported type and function gets a Go-doc comment
- Comments explain purpose, not mechanics
- Inline comments for non-obvious SQL or business logic

**Examples:**
```go
// Service handles authentication, sessions, and password hashing.
type Service struct { ... }

// hashPassword creates an Argon2id hash.
func (s *Service) hashPassword(password string) (string, error) { ... }
```

## Function Design

**Size:**
- Handlers are moderate (20-60 lines); large files split by concern
- Largest file: `internal/mailstore/pgstore.go` (~700 lines)

**Parameters:**
- `context.Context` is always first parameter when needed
- Pointer receivers for stateful types (`*Service`, `*Handler`, `*Pool`)
- Value receivers for pure functions (e.g., `parseAddr`)

**Return Values:**
- `(result, error)` pattern universally
- Tuple returns for related data (e.g., `(*models.AuthSession, string, error)`)

## Module Design

**Exports:**
- Packages export a small public surface: constructor + methods + interfaces
- Interfaces defined in the consumer package when possible (e.g., `DomainLister` in `webmail`, `Store` in `mailstore`)

**Barrel Files:**
- Not used in Go
- Frontend: `api.js` is a flat barrel of all API functions; `sse.js` is standalone

## Frontend Specific Conventions

**Component Structure:**
- Default export at bottom for single-export files
- Hooks at top of component body
- JSX uses TailwindCSS utility classes exclusively
- Event handlers use inline arrow functions or named async functions

**API Pattern:**
- All API calls go through `axios` instance in `web/src/api.js`
- CSRF token attached via request interceptor from `csrf` cookie
- `withCredentials: true` on all requests
- Response interceptor handles global 401 redirect

**Security:**
- HTML email rendered in sandboxed iframe (`sandbox=""`)
- `DOMPurify.sanitize()` before injecting HTML
- `DOMParser` for deriving plain text from HTML (no live DOM assignment)

---

*Convention analysis: 2026-05-18*

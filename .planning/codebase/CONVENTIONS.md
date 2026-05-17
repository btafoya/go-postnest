# Coding Conventions

**Analysis Date:** 2026-05-17

## Naming Patterns

**Files:**
- Descriptive lowercase names for Go source files (auth.go, middleware.go, pgstore.go)
- `*_test.go` alongside source files in the same package
- Package directories use lowercase, single-word names when possible (redis, smtp, webhook)

**Functions:**
- Exported: PascalCase (Authenticate, CreateSession, NewPool)
- Unexported: camelCase (hashPassword, verifyPassword, extractToken)
- Constructors: `New` or `New<Type>` (NewService, NewPool, NewPGStore, NewHandler)
- Middleware constructors: named after behavior (CORS, Recovery, RequestID)
- HTTP handlers: unexported methods on handler structs (handleInbound, createDraft, updateDraft)

**Variables:**
- camelCase for local variables
- Short names accepted in tight scopes (s for store, tx for transaction, ctx for context)
- Package-level constants: normal camelCase or PascalCase
- No ALL_CAPS constants observed

**Types:**
- Exported structs: PascalCase (User, Domain, Message, Config)
- Interfaces: PascalCase, defined in consumer packages (Store, Processor)
- Custom types for context keys: `type ctxKey string` with const declarations
- Options structs: PascalCase with Options or Patch suffix (ListOptions, SearchOptions, MessagePatch)

## Code Style

**Formatting:**
- Standard gofmt (no custom formatter configured)
- Tab indentation (Go default)
- No explicit line length limit; keep readable
- Go version 1.25.0

**Linting:**
- No golangci-lint or custom lint config detected
- Relies on `go vet` and `go build` for static analysis

## Import Organization

**Order:**
1. Standard library packages
2. Third-party packages (github.com/emersion/*, github.com/jackc/*, etc.)
3. Internal packages (github.com/go-postnest/postnest/internal/*)

**Grouping:**
- Single blank line between each group
- No blank line within a group
- No import aliases unless required for disambiguation (e.g., goredis for go-redis in tests)

**Path Aliases:**
- No custom module path aliases; all imports use full module path

## Error Handling

**Patterns:**
- Custom application error type: `AppError` with Code, Message, Details, StatusCode, Err fields
- Sentinel errors as package-level vars (ErrNotFound, ErrUnauthorized, ErrValidation, etc.)
- Wrap errors with `fmt.Errorf("...: %w", err)` at service boundaries
- Database-specific: check `pgx.ErrNoRows` and map to domain errors
- Panics recovered at HTTP middleware boundary (`Recovery` middleware logs and returns 500)

**Error Types:**
- Validation errors carry `[]FieldError` with Field, Issue, Param
- Use `api.As(err, &target)` wrapper around `errors.As`
- Return errors; do not log-and-swallow in low-level functions

## Logging

**Framework:**
- Standard library `log/slog` with JSON handler
- `slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))`

**Patterns:**
- Structured key-value pairs: `logger.Info("request", "method", r.Method, "path", r.URL.Path)`
- Log at service boundaries (HTTP requests, job processing, server startup/shutdown)
- No `fmt.Println` or `log.Println` in production code
- Error level for failures: `logger.Error("dequeue error", "error", err)`
- Warn level for recoverable issues: `logger.Warn("no processor for job type", "type", job.Type)`

## Comments

**When to Comment:**
- Package-level comments for exported types and functions
- Explain "why" for non-obvious logic (e.g., legacy env var mapping, periodic cleanup reasons)
- Avoid obvious comments that repeat what the code says
- No JSDoc/TSDoc equivalent beyond standard Go doc comments

**TODO Comments:**
- Rare; use standard `// TODO: description` if needed

## Function Design

**Size:**
- Prefer functions under ~50 lines; extract helpers for complex logic
- Some accepted exceptions for HTTP setup and protocol parsers (SMTP Data, server main)

**Parameters:**
- `context.Context` as first parameter for any I/O or long-running function
- Options objects for complex updates (MessagePatch with pointer fields for optional updates)
- Max ~4-5 positional parameters; use structs for more

**Return Values:**
- Return `(value, error)` pairs
- Guard clauses with early returns for error cases
- Use `coalesce` in SQL for optional field updates instead of multiple query variants

## Module Design

**Package Structure:**
- One package per domain concern under `internal/` (auth, api, mailstore, workers, etc.)
- `cmd/` contains `main` packages (server, worker, migrate)
- `internal/models/` holds shared domain structs (User, Message, Domain, etc.)

**Constructor Pattern:**
- Export constructor functions that return concrete types
- Types implement interfaces implicitly (no `implements` keyword)

**Interfaces:**
- Define interfaces in consumer packages (mailstore.Store defined in mailstore, consumed by webmail)
- This enables easy manual mocking in tests

**State Management:**
- Services hold dependencies as struct fields (pool, logger, config values)
- Prefer dependency injection via constructors over package-level globals
- Context values used for request-scoped data (user, domain_id, request_id)

---

*Convention analysis: 2026-05-17*
*Update when patterns change*

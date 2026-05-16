# PostNest Testing Patterns

This document describes the current testing state, conventions, and gaps in the PostNest Go 1.25 codebase.

---

## Test Framework

- **Primary**: Standard Go `testing` package (`go test`).
- **Assertions**: Manual `if` checks with `t.Fatalf` / `t.Errorf`. No third-party assertion libraries (e.g., testify) are currently used.
- **Subtests**: Not yet employed; each scenario is a top-level function.

```go
// internal/config/loader_test.go
func TestLoader_Load_FromFile(t *testing.T) {
    // ...
    if cfg.HTTPAddr != ":9090" {
        t.Errorf("HTTPAddr = %q, want :9090", cfg.HTTPAddr)
    }
}
```

---

## Test File Locations and Naming

- **Location**: Tests live alongside source code in the same package (`*_test.go` in the same directory).
- **Naming**: `Test<Struct>_<Method>_<Scenario>`.

| File | Package | Coverage |
|------|---------|----------|
| `internal/config/loader_test.go` | `config` | Loader TOML parsing, env override, legacy env, missing required fields |

**Only one test file exists in the entire repository.** This is a significant coverage gap.

---

## Testing Patterns Observed

### Unit Testing with Temporary Files
The existing test uses `t.TempDir()` for filesystem isolation:

```go
// internal/config/loader_test.go
func TestLoader_Load_FromFile(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "postnest.conf")

    content := `
config_version = 1
[server]
http_addr = ":9090"
...`
    if err := os.WriteFile(path, []byte(content), 0640); err != nil {
        t.Fatalf("write config: %v", err)
    }

    loader := NewLoader(path)
    cfg, err := loader.Load()
    if err != nil {
        t.Fatalf("load: %v", err)
    }
    // assertions ...
}
```

### Environment Variable Isolation
Tests manipulate env vars via `t.Setenv`, which automatically cleans up:

```go
// internal/config/loader_test.go
func TestLoader_Load_EnvOverride(t *testing.T) {
    t.Setenv("POSTNEST_DATABASE_DSN", "postgres://env@localhost/env")
    t.Setenv("POSTNEST_SECURITY_SESSION_KEY", "env-secret")
    // ...
}
```

### Error Testing
Missing required fields are tested by asserting a non-nil error is returned:

```go
// internal/config/loader_test.go
func TestLoader_Load_MissingRequired(t *testing.T) {
    loader := NewLoader("/nonexistent/path.conf")
    _, err := loader.Load()
    if err == nil {
        t.Fatal("expected error for missing required fields")
    }
}
```

---

## Mocking Approach

- **No generated mocks** are present.
- **No manual stub types** exist in test files.
- **Interfaces are defined** (`mailstore.Store`, `contacts.Store`, `workers.Processor`), which would allow manual mocks or a tool like `mockery`/`gomock` if adopted.

Because there is only one test file, mocking has not yet been exercised in practice.

---

## Database Testing

- **No database tests** exist.
- **No testcontainers** or in-memory PostgreSQL wrappers are configured.
- **No transaction rollback pattern** is used for test isolation.

The `internal/db/db.go` package creates a real `pgxpool.Pool` against a live DSN. A future test suite could:

1. Spin up a test Postgres instance (e.g., `testcontainers-go`).
2. Use `pgx.BeginTx` + `defer tx.Rollback(ctx)` for per-test isolation.
3. Run `internal/migrate.Up(dsn)` to schema the test database before tests.

---

## Test Coverage Areas and Gaps

### Covered
- `internal/config/loader.go`: TOML loading, env override precedence, legacy env mapping, validation of required fields.

### Not Covered (Major Gaps)

| Package | What's Untested |
|---------|-----------------|
| `internal/api` | `AppError` serialization, `WriteError`, all middleware (`RequestID`, `Recovery`, `CORS`, `RequireSession`, `RequireDomainAdmin`) |
| `internal/auth` | Password hashing, session creation/validation, API key validation, user CRUD, domain admin checks |
| `internal/mailstore` | All `PGStore` methods: `CreateMessage`, `GetMessage`, `ListMessages`, `UpdateMessage`, `DeleteMessage`, `ApplyLabels`, `Search`, thread operations |
| `internal/contacts` | `PGStore` CRUD and upsert (`ON CONFLICT`) behavior |
| `internal/postmark` | `SendEmail`, `ParseInbound`, attachment handling |
| `internal/redis` | `Enqueue`, `Dequeue`, `Publish` |
| `internal/workers` | `Pool.Start`, `Pool.worker`, retry logic, `Processor` implementations (`InboundProcessor`, `BounceProcessor`, `DeliveryProcessor`) |
| `internal/imap` | IMAP backend session handling, mailbox operations, message listing, flag updates |
| `internal/smtp` | SMTP session auth, mail parsing, outbound relay, attachment extraction |
| `internal/webhook` | Route registration, signature verification, enqueue behavior |
| `internal/webmail` | REST handlers: labels, messages, threads, drafts, search |
| `internal/dav` | CardDAV/CalDAV route handling |
| `internal/certmanager` | ACME registration, certificate lifecycle, renewal loop |
| `internal/db` | Connection pool creation, ping behavior, close logic |
| `internal/logger` | JSON output, level filtering |
| `cmd/server` | Main wiring, graceful shutdown, TLS strategy switching |
| `cmd/worker` | Worker startup and processor registration |
| `cmd/migrate` | Migration up/down/version/force commands |

---

## How to Run Tests

```bash
# Run all tests
cd /home/btafoya/projects/go-postnest
go test ./...

# Run with verbose output
go test -v ./...

# Run a specific package
go test -v ./internal/config/...

# Run a specific test
go test -v -run TestLoader_Load_FromFile ./internal/config/...

# Run with race detector
go test -race ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

---

## Recommendations

1. **Add testify** (`assert` / `require`) to reduce boilerplate in new tests.
2. **Introduce table-driven tests** for CRUD operations and middleware scenarios.
3. **Create interface-based mocks** for `mailstore.Store`, `auth.Service`, and `redis.Client` using `gomock` or hand-written stubs.
4. **Set up `testcontainers-go`** for integration tests against a real PostgreSQL instance.
5. **Add HTTP handler tests** using `httptest.NewRecorder` and `chi.NewRouter` to exercise `webmail`, `webhook`, and middleware without starting a server.
6. **Target coverage** for critical paths: authentication, message CRUD, and webhook ingestion before expanding to protocol servers (IMAP/SMTP).

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	appredis "github.com/go-postnest/postnest/internal/redis"
	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/logging"
)

func init() {
	logging.Disable()
}

// mockAuthCounter implements authCounter for testing.
type mockAuthCounter struct {
	users   int64
	domains int64
	err     error
}

func (m *mockAuthCounter) CountUsers(ctx context.Context) (int64, error)   { return m.users, m.err }
func (m *mockAuthCounter) CountDomains(ctx context.Context) (int64, error) { return m.domains, m.err }

// mockMailCounter implements mailCounter for testing.
type mockMailCounter struct {
	today int64
	err   error
}

func (m *mockMailCounter) CountMessagesToday(ctx context.Context) (int64, error) { return m.today, m.err }

func TestHealthHandler_NewHealthHandler(t *testing.T) {
	h := NewHealthHandler(nil, nil, "", "", nil, nil)
	if h == nil {
		t.Fatal("NewHealthHandler returned nil")
	}
}

func TestHealthHandler_RegisterRoutes(t *testing.T) {
	h := NewHealthHandler(nil, nil, "", "", nil, nil)
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	found := false
	chi.Walk(r, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		if method == "GET" && route == "/admin/api/v1/health" {
			found = true
		}
		return nil
	})
	if !found {
		t.Error("health route not registered")
	}
}

func TestHealthHandler_NilDepsDoesNotPanicOnRegister(t *testing.T) {
	h := NewHealthHandler(nil, nil, "", "", nil, nil)
	r := chi.NewRouter()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RegisterRoutes panicked with nil deps: %v", r)
		}
	}()
	h.RegisterRoutes(r)
}

func TestHealthHandler_Handle(t *testing.T) {
	// Create a real PostgreSQL pool.
	pgCfg, err := pgxpool.ParseConfig("postgres://postnest:changeme@localhost:5432/postnest?sslmode=disable")
	if err != nil {
		t.Skipf("skip: unable to parse postgres DSN: %v", err)
	}
	pgPool, err := pgxpool.NewWithConfig(context.Background(), pgCfg)
	if err != nil {
		t.Skipf("skip: unable to create postgres pool: %v", err)
	}
	defer pgPool.Close()

	// Use a Redis client pointing to a bad address so it returns errors (no panic).
	redisClient := &appredis.Client{UniversalClient: redis.NewClient(&redis.Options{Addr: "localhost:9999", MaxRetries: 0, DialTimeout: 100 * time.Millisecond, PoolSize: 1, MinIdleConns: 0})}
	defer redisClient.Close()

	authSvc := &mockAuthCounter{users: 5, domains: 2}
	mailSvc := &mockMailCounter{today: 42}

	h := &HealthHandler{
		pgPool:      pgPool,
		redisClient: redisClient,
		imapAddr:    "",
		smtpAddr:    "",
		authService: authSvc,
		mailStore:   mailSvc,
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/health", nil)
	h.handle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	requiredKeys := []string{"database", "redis", "imap", "smtp", "worker_queue", "active_users", "messages_today", "total_domains"}
	for _, key := range requiredKeys {
		if _, ok := body[key]; !ok {
			t.Errorf("missing key %q in response", key)
		}
	}

	wq, ok := body["worker_queue"].(map[string]any)
	if !ok {
		t.Fatalf("worker_queue is not an object: %T", body["worker_queue"])
	}
	if _, ok := wq["depth"]; !ok {
		t.Error("worker_queue missing depth")
	}
	if _, ok := wq["dead"]; !ok {
		t.Error("worker_queue missing dead")
	}

	// Verify active_users, messages_today, total_domains values.
	if body["active_users"] != float64(5) {
		t.Errorf("active_users = %v, want 5", body["active_users"])
	}
	if body["messages_today"] != float64(42) {
		t.Errorf("messages_today = %v, want 42", body["messages_today"])
	}
	if body["total_domains"] != float64(2) {
		t.Errorf("total_domains = %v, want 2", body["total_domains"])
	}
}

func TestHealthHandler_DatabaseDown(t *testing.T) {
	// Create a pool pointing to a non-existent port so Ping fails.
	pgCfg, err := pgxpool.ParseConfig("postgres://postnest:changeme@localhost:9999/postnest?sslmode=disable")
	if err != nil {
		t.Skipf("skip: unable to parse postgres DSN: %v", err)
	}
	pgPool, err := pgxpool.NewWithConfig(context.Background(), pgCfg)
	if err != nil {
		t.Skipf("skip: unable to create postgres pool: %v", err)
	}
	defer pgPool.Close()

	redisClient := &appredis.Client{UniversalClient: redis.NewClient(&redis.Options{Addr: "localhost:9999", MaxRetries: 0, DialTimeout: 100 * time.Millisecond, PoolSize: 1, MinIdleConns: 0})}
	defer redisClient.Close()

	authSvc := &mockAuthCounter{users: 1, domains: 1}
	mailSvc := &mockMailCounter{today: 0}

	h := &HealthHandler{
		pgPool:      pgPool,
		redisClient: redisClient,
		imapAddr:    "",
		smtpAddr:    "",
		authService: authSvc,
		mailStore:   mailSvc,
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/health", nil)
	h.handle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	db, ok := body["database"].(map[string]any)
	if !ok {
		t.Fatalf("database is not an object: %T", body["database"])
	}
	if db["status"] != "down" {
		t.Errorf("database.status = %v, want down", db["status"])
	}
	if _, ok := db["error"]; !ok {
		t.Error("database.error missing when DB is down")
	}
}

func TestHealthHandler_RedisDown(t *testing.T) {
	pgCfg, err := pgxpool.ParseConfig("postgres://postnest:changeme@localhost:5432/postnest?sslmode=disable")
	if err != nil {
		t.Skipf("skip: unable to parse postgres DSN: %v", err)
	}
	pgPool, err := pgxpool.NewWithConfig(context.Background(), pgCfg)
	if err != nil {
		t.Skipf("skip: unable to create postgres pool: %v", err)
	}
	defer pgPool.Close()

	redisClient := &appredis.Client{UniversalClient: redis.NewClient(&redis.Options{Addr: "localhost:9999", MaxRetries: 0, DialTimeout: 100 * time.Millisecond, PoolSize: 1, MinIdleConns: 0})}
	defer redisClient.Close()

	authSvc := &mockAuthCounter{users: 1, domains: 1}
	mailSvc := &mockMailCounter{today: 0}

	h := &HealthHandler{
		pgPool:      pgPool,
		redisClient: redisClient,
		imapAddr:    "",
		smtpAddr:    "",
		authService: authSvc,
		mailStore:   mailSvc,
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/health", nil)
	h.handle(rec, req)

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	redis, ok := body["redis"].(map[string]any)
	if !ok {
		t.Fatalf("redis is not an object: %T", body["redis"])
	}
	if redis["status"] != "down" {
		t.Errorf("redis.status = %v, want down", redis["status"])
	}
	if _, ok := redis["error"]; !ok {
		t.Error("redis.error missing when Redis is down")
	}
}

func TestHealthHandler_HandleWithNilPool(t *testing.T) {
	// Verify handle panics with nil pool so caller knows not to do this in production.
	redisClient := &appredis.Client{UniversalClient: redis.NewClient(&redis.Options{Addr: "localhost:9999", MaxRetries: 0, DialTimeout: 100 * time.Millisecond, PoolSize: 1, MinIdleConns: 0})}
	defer redisClient.Close()

	authSvc := &mockAuthCounter{users: 1, domains: 1}
	mailSvc := &mockMailCounter{today: 0}

	h := &HealthHandler{
		pgPool:      nil,
		redisClient: redisClient,
		imapAddr:    "",
		smtpAddr:    "",
		authService: authSvc,
		mailStore:   mailSvc,
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/health", nil)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic with nil pgPool, but did not panic")
		}
	}()
	h.handle(rec, req)
}

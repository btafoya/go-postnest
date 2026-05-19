package admin

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/mailstore"
	"github.com/go-postnest/postnest/internal/redis"
)

type authCounter interface {
	CountUsers(ctx context.Context) (int64, error)
	CountDomains(ctx context.Context) (int64, error)
}

type mailCounter interface {
	CountMessagesToday(ctx context.Context) (int64, error)
}

// HealthHandler provides the admin health endpoint.
type HealthHandler struct {
	pgPool      *pgxpool.Pool
	redisClient *redis.Client
	imapAddr    *string
	smtpAddr    *string
	authService authCounter
	mailStore   mailCounter
}

// NewHealthHandler creates a HealthHandler with explicit dependencies.
// imapAddr/smtpAddr are pointers because the listener addresses are resolved
// by the TLS strategy after this handler is constructed; the probe reads
// them at request time.
func NewHealthHandler(pgPool *pgxpool.Pool, redisClient *redis.Client, imapAddr, smtpAddr *string, authService *auth.Service, mailStore mailstore.Store) *HealthHandler {
	return &HealthHandler{
		pgPool:      pgPool,
		redisClient: redisClient,
		imapAddr:    imapAddr,
		smtpAddr:    smtpAddr,
		authService: authService,
		mailStore:   mailStore,
	}
}

// RegisterRoutes wires the health route.
func (h *HealthHandler) RegisterRoutes(r chi.Router) {
	r.Get("/admin/api/v1/health", h.handle)
}

func (h *HealthHandler) handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	comp := func(probe func() error) map[string]any {
		start := time.Now()
		if err := probe(); err != nil {
			return map[string]any{"status": "down", "error": err.Error()}
		}
		return map[string]any{"status": "up", "latency_ms": time.Since(start).Milliseconds()}
	}
	dialTCP := func(addr string) func() error {
		return func() error {
			c, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err != nil {
				return err
			}
			return c.Close()
		}
	}
	depth, _ := h.redisClient.UniversalClient.LLen(ctx, "queue:jobs").Result()
	dead, _ := h.redisClient.UniversalClient.LLen(ctx, "queue:jobs:dead").Result()
	activeUsers, _ := h.authService.CountUsers(ctx)
	msgToday, _ := h.mailStore.CountMessagesToday(ctx)
	totalDomains, _ := h.authService.CountDomains(ctx)
	writeJSON(w, http.StatusOK, map[string]any{
		"database":       comp(func() error { return h.pgPool.Ping(ctx) }),
		"redis":          comp(func() error { return h.redisClient.Ping(ctx).Err() }),
		"imap":           comp(dialTCP(*h.imapAddr)),
		"smtp":           comp(dialTCP(*h.smtpAddr)),
		"worker_queue":   map[string]any{"depth": depth, "dead": dead},
		"active_users":   activeUsers,
		"messages_today": msgToday,
		"total_domains":  totalDomains,
	})
}

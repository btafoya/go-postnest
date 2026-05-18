package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-postnest/postnest/internal/api"
	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/calendar"
	"github.com/go-postnest/postnest/internal/certmanager"
	"github.com/go-postnest/postnest/internal/config"
	"github.com/go-postnest/postnest/internal/contacts"
	"github.com/go-postnest/postnest/internal/dav"
	"github.com/go-postnest/postnest/internal/db"
	"github.com/go-postnest/postnest/internal/imap"
	"github.com/go-postnest/postnest/internal/logger"
	"github.com/go-postnest/postnest/internal/mailstore"
	"github.com/go-postnest/postnest/internal/postmark"
	"github.com/go-postnest/postnest/internal/redis"
	"github.com/go-postnest/postnest/internal/smtp"
	"github.com/go-postnest/postnest/internal/webhook"
	"github.com/go-postnest/postnest/internal/webmail"
)

func main() {
	log := logger.New()
	cfg, err := config.Load()
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	pgPool, err := db.New(cfg.PostgresDSN, cfg.MaxDBConns)
	if err != nil {
		log.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer pgPool.Close()

	redisClient, err := redis.New(cfg.RedisURL)
	if err != nil {
		log.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}

	authService := auth.NewService(pgPool.Pool, cfg.Argon2idTime, cfg.Argon2idMemory, cfg.Argon2idThreads, cfg.SessionKey)
	mailStore := mailstore.NewPGStore(pgPool.Pool)
	contactsStore := contacts.NewPGStore(pgPool.Pool)
	calendarStore := calendar.NewPGStore(pgPool.Pool)
	postmarkClient := postmark.NewClient()
	_ = postmarkClient

	webmailHandler := webmail.NewHandler(mailStore, authService, redisClient, cfg.MaxAttachmentSize)
	webhookHandler := webhook.NewHandler(redisClient, cfg.PostmarkWebhookSecret)

	r := chi.NewRouter()
	// Middleware
	r.Use(api.RequestID)
	r.Use(api.StructuredLogger(log))
	r.Use(api.Recovery)
	r.Use(api.CORS(cfg.AllowedOrigins))
	rateLimiter := api.NewRateLimiter(100, time.Minute)
	r.Use(rateLimiter.Handler)

	// Public health
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		status := "ok"
		httpStatus := http.StatusOK

		// Check database
		if err := pgPool.Ping(r.Context()); err != nil {
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
			log.Error("health check: database ping failed", "error", err)
		}

		// Check redis
		if err := redisClient.Ping(r.Context()).Err(); err != nil {
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
			log.Error("health check: redis ping failed", "error", err)
		}

		writeJSON(w, httpStatus, map[string]string{"status": status})
	})

	// Webhook routes (public but secret-verified)
	webhookHandler.RegisterRoutes(r)

	// Public auth routes
	authHandler := api.NewAuthHandler(authService, cfg.SessionKey, cfg.SessionExpiry)
	authHandler.RegisterRoutes(r)

	// Listener addresses resolved by the TLS strategy below; declared early so
	// the health endpoint closure can probe them.
	var imapAddr, smtpAddr string

	// Authenticated API routes
	r.Group(func(r chi.Router) {
		r.Use(api.RequireSession(authService))
		r.Use(api.CSRF)
		webmailHandler.RegisterRoutes(r)
		calendar.NewHandler(calendarStore, authService).RegisterRoutes(r)
	})

	// Admin API routes
	r.Group(func(r chi.Router) {
		r.Use(api.RequireSession(authService))
		r.Use(api.CSRF)
		r.Use(api.RequireDomainAdmin(authService))
		r.Get("/admin/api/v1/domains", func(w http.ResponseWriter, r *http.Request) {
			user := api.UserFromContext(r.Context())
			doms, err := authService.GetUserDomains(r.Context(), user.ID)
			if err != nil {
				api.WriteError(w, api.ErrInternal)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"domains": doms})
		})
		r.Get("/admin/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
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
			depth, _ := redisClient.UniversalClient.LLen(ctx, "queue:jobs").Result()
			dead, _ := redisClient.UniversalClient.LLen(ctx, "queue:jobs:dead").Result()
			writeJSON(w, http.StatusOK, map[string]any{
				"database":     comp(func() error { return pgPool.Ping(ctx) }),
				"redis":        comp(func() error { return redisClient.Ping(ctx).Err() }),
				"imap":         comp(dialTCP(imapAddr)),
				"smtp":         comp(dialTCP(smtpAddr)),
				"worker_queue": map[string]any{"depth": depth, "dead": dead},
			})
		})
	})

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      r,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	go func() {
		log.Info("http server starting", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server error", "error", err)
		}
	}()

	// Determine TLS strategy and listener configuration.
	var (
		tlsConfig         *tls.Config
		allowInsecureAuth bool
		certMgr           *certmanager.Manager
	)

	switch {
	case cfg.ACMEEnabled:
		cmCfg := certmanager.Config{
			Email:         cfg.ACMEEmail,
			Domain:        cfg.ACMEDomain,
			Directory:     cfg.ACMEDirectory,
			CertDir:       cfg.ACMECertDir,
			DNSProvider:   cfg.ACMEDNSProvider,
			RenewInterval: cfg.ACMERenewInterval,
			RenewBefore:   cfg.ACMERenewBefore,
		}
		certMgr, err = certmanager.NewManager(cmCfg, log)
		if err != nil {
			log.Error("failed to create certificate manager", "error", err)
			os.Exit(1)
		}
		certMgrCtx, certMgrCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer certMgrCancel()
		if err := certMgr.Start(certMgrCtx); err != nil {
			log.Error("failed to start certificate manager", "error", err)
			os.Exit(1)
		}

		tlsConfig = &tls.Config{
			GetCertificate: certMgr.GetCertificate,
		}
		allowInsecureAuth = false
		imapAddr = cfg.IMAPSAddr
		smtpAddr = cfg.SMTPSAddr
		log.Info("ACME TLS configured", "domain", cfg.ACMEDomain, "directory", cfg.ACMEDirectory)

	case cfg.TLSCertPath != "" && cfg.TLSKeyPath != "":
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertPath, cfg.TLSKeyPath)
		if err != nil {
			log.Error("failed to load TLS certificates", "error", err)
			os.Exit(1)
		}
		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
		allowInsecureAuth = false
		imapAddr = cfg.IMAPSAddr
		smtpAddr = cfg.SMTPSAddr
		log.Info("static TLS configured", "cert", cfg.TLSCertPath)

	default:
		allowInsecureAuth = cfg.AllowInsecureAuth
		imapAddr = cfg.IMAPAddr
		smtpAddr = cfg.SMTPAddr
		if !allowInsecureAuth {
			log.Error("refusing to start: no TLS configured and plaintext auth not explicitly allowed; set POSTNEST_ALLOW_INSECURE_AUTH=true to override (development only)")
			os.Exit(1)
		}
		log.Warn("running without TLS with insecure auth allowed", "imap", imapAddr, "smtp", smtpAddr)
	}

	// IMAP server
	imapSrv := imap.NewServer(imapAddr, tlsConfig, allowInsecureAuth, mailStore, authService, redisClient)
	go func() {
		log.Info("imap server starting", "addr", imapAddr, "tls", tlsConfig != nil)
		if err := imapSrv.Start(); err != nil {
			log.Error("imap server error", "error", err)
		}
	}()

	// SMTP server
	smtpSrv := smtp.NewServer(smtpAddr, tlsConfig, allowInsecureAuth, mailStore, authService, postmarkClient, cfg.MaxMessageSize)
	smtpCtx, smtpCancel := context.WithCancel(context.Background())
	defer smtpCancel()
	go func() {
		log.Info("smtp server starting", "addr", smtpAddr, "tls", tlsConfig != nil)
		if err := smtpSrv.Start(smtpCtx); err != nil {
			log.Error("smtp server error", "error", err)
		}
	}()

	// DAV routes
	davHandler := dav.NewHandler(authService, contactsStore, mailStore, calendarStore)
	davHandler.RegisterRoutes(r)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown HTTP server
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown error", "error", err)
	}

	// Shutdown IMAP server
	if err := imapSrv.Stop(); err != nil {
		log.Error("imap shutdown error", "error", err)
	}

	// Shutdown SMTP server
	smtpCancel()
	if err := smtpSrv.Stop(); err != nil {
		log.Error("smtp shutdown error", "error", err)
	}

	// Shutdown certificate manager
	if certMgr != nil {
		if err := certMgr.Stop(); err != nil {
			log.Error("certificate manager shutdown error", "error", err)
		}
	}

	log.Info("shutdown complete")
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-postnest/postnest/internal/admin"
	"github.com/go-postnest/postnest/internal/api"
	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/autodiscover"
	"github.com/go-postnest/postnest/internal/calendar"
	"github.com/go-postnest/postnest/internal/certmanager"
	"github.com/go-postnest/postnest/internal/config"
	"github.com/go-postnest/postnest/internal/contacts"
	"github.com/go-postnest/postnest/internal/crypto"
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
	webhookHandler := webhook.NewHandler(redisClient, pgPool.Pool)

	adminStore := admin.NewPGStore(pgPool.Pool)
	settingsCache := admin.NewSettingsCache(adminStore, 30*time.Second)

	r := chi.NewRouter()
	// Middleware
	r.Use(api.RequestID)
	r.Use(api.StructuredLogger(log))
	r.Use(api.Recovery)
	r.Use(api.CORS(cfg.AllowedOrigins))
	rateLimiter := api.NewDynamicRateLimiter(func() int {
		v := settingsCache.Get(context.Background(), "rate_limit_requests_per_minute")
		n, _ := strconv.Atoi(v)
		if n <= 0 {
			return 100
		}
		return n
	})
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
		contacts.NewHandler(contactsStore, authService).RegisterRoutes(r)
	})

	adminHandler := admin.NewHandler(adminStore, authService)
	healthHandler := admin.NewHealthHandler(pgPool.Pool, redisClient, &imapAddr, &smtpAddr, authService, mailStore)

	// Admin API routes
	r.Group(func(r chi.Router) {
		r.Use(api.RequireSession(authService))
		r.Use(api.CSRF)
		r.Use(api.RequireSuperAdmin)
		adminHandler.RegisterRoutes(r)
		healthHandler.RegisterRoutes(r)
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

	if cfg.SecretKey != "" {
		cipher, cerr := crypto.NewCipher(cfg.SecretKey)
		if cerr != nil {
			log.Error("POSTNEST_SECRET_KEY is invalid", "error", cerr)
			os.Exit(1)
		}

		// Seed the DB ACME config/domains from TOML on first run so existing
		// deployments keep working before anything is set via the admin UI.
		seedCtx, seedCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := seedACMEFromTOML(seedCtx, adminStore, cfg); err != nil {
			seedCancel()
			log.Error("failed to seed ACME config", "error", err)
			os.Exit(1)
		}
		seedCancel()

		buildCMCfg := func() (certmanager.Config, error) {
			return acmeConfigFromDB(context.Background(), adminStore, cipher, cfg)
		}

		cmCfg, berr := buildCMCfg()
		if berr != nil {
			log.Error("failed to assemble ACME config", "error", berr)
			os.Exit(1)
		}
		certMgr, err = certmanager.NewManager(cmCfg, log)
		if err != nil {
			log.Error("failed to create certificate manager", "error", err)
			os.Exit(1)
		}
		certMgrCtx, certMgrCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer certMgrCancel()
		if err := certMgr.Start(certMgrCtx); err != nil {
			log.Error("certificate manager initial obtain failed; continuing without TLS for IMAP/SMTP", "error", err)
		}

		// Admin UI mutations persist to the DB then call this to hot-reload.
		adminHandler.WithTLS(certMgr, cipher, func() error {
			newCfg, e := buildCMCfg()
			if e != nil {
				return e
			}
			return certMgr.Reload(newCfg)
		})

		tlsConfig = &tls.Config{
			GetCertificate: certMgr.GetCertificate,
		}
		allowInsecureAuth = false
		imapAddr = cfg.IMAPSAddr
		smtpAddr = cfg.SMTPSAddr
		log.Info("ACME TLS manager ready", "enabled", cmCfg.Email != "" && len(cmCfg.Domains) > 0, "domains", cmCfg.Domains, "directory", cmCfg.Directory)
	} else if cfg.TLSCertPath != "" && cfg.TLSKeyPath != "" {
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
	} else {
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

	// SMTP servers
	var smtpSrv, startTLSSrv *smtp.Server
	var smtpCancel, startTLSCancel context.CancelFunc

	if tlsConfig != nil {
		// Implicit TLS on 465
		smtpSrv = smtp.NewServer(cfg.SMTPSAddr, tlsConfig, false, mailStore, authService, postmarkClient, cfg.MaxMessageSize)
		var smtpCtx context.Context
		smtpCtx, smtpCancel = context.WithCancel(context.Background())
		go func() {
			log.Info("smtp server starting", "addr", cfg.SMTPSAddr, "tls", true)
			if err := smtpSrv.Start(smtpCtx); err != nil {
				log.Error("smtp server error", "error", err)
			}
		}()

		// STARTTLS on 587
		startTLSSrv = smtp.NewStartTLSServer(cfg.SMTPAddr, tlsConfig, false, mailStore, authService, postmarkClient, cfg.MaxMessageSize)
		var startTLSCtx context.Context
		startTLSCtx, startTLSCancel = context.WithCancel(context.Background())
		go func() {
			log.Info("smtp server starting", "addr", cfg.SMTPAddr, "starttls", true)
			if err := startTLSSrv.Start(startTLSCtx); err != nil {
				log.Error("smtp starttls server error", "error", err)
			}
		}()
	} else {
		smtpSrv = smtp.NewServer(cfg.SMTPAddr, nil, allowInsecureAuth, mailStore, authService, postmarkClient, cfg.MaxMessageSize)
		var smtpCtx context.Context
		smtpCtx, smtpCancel = context.WithCancel(context.Background())
		go func() {
			log.Info("smtp server starting", "addr", cfg.SMTPAddr, "tls", false)
			if err := smtpSrv.Start(smtpCtx); err != nil {
				log.Error("smtp server error", "error", err)
			}
		}()
	}

	// DAV routes
	davHandler := dav.NewHandler(authService, contactsStore, mailStore, calendarStore)
	davHandler.RegisterRoutes(r)

	// Public autodiscover routes (needs certMgr from TLS strategy above)
	autodiscoverHandler := autodiscover.NewHandler(authService, certMgr, log)
	autodiscoverHandler.RegisterRoutes(r)

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

	// Shutdown SMTP servers
	smtpCancel()
	if err := smtpSrv.Stop(); err != nil {
		log.Error("smtp shutdown error", "error", err)
	}
	if startTLSCancel != nil {
		startTLSCancel()
		if err := startTLSSrv.Stop(); err != nil {
			log.Error("smtp starttls shutdown error", "error", err)
		}
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

// seedACMEFromTOML populates the DB ACME config/domains from legacy TOML/env
// values on first run. It is a one-time bootstrap: existing rows are left
// untouched so the admin UI remains the source of truth thereafter.
func seedACMEFromTOML(ctx context.Context, store *admin.PGStore, cfg *config.Config) error {
	dbCfg, err := store.GetACMEConfig(ctx)
	if err != nil {
		return err
	}

	shouldSeed := false
	enabled := dbCfg.Enabled
	email := dbCfg.Email
	directory := dbCfg.Directory
	dnsProvider := dbCfg.DNSProvider
	credsEnc := dbCfg.CredentialsEnc

	if email == "" && cfg.ACMEEmail != "" {
		shouldSeed = true
		email = cfg.ACMEEmail
		directory = cfg.ACMEDirectory
		if directory != "production" {
			directory = "staging"
		}
		dnsProvider = cfg.ACMEDNSProvider
		if dnsProvider == "" {
			dnsProvider = "cloudflare"
		}
		enabled = cfg.ACMEEnabled
	}
	if !enabled && cfg.ACMEEnabled && email != "" {
		shouldSeed = true
		enabled = true
	}

	if shouldSeed {
		if err := store.SetACMEConfig(ctx, enabled, email, directory, dnsProvider, credsEnc); err != nil {
			return err
		}
	}

	domains, err := store.ListACMEDomains(ctx)
	if err != nil {
		return err
	}
	if len(domains) == 0 && cfg.ACMEDomain != "" {
		for _, d := range strings.Split(cfg.ACMEDomain, ",") {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			if _, err := store.AddACMEDomain(ctx, d); err != nil {
				return err
			}
		}
	}
	return nil
}

// acmeConfigFromDB assembles a certmanager.Config from the DB-persisted ACME
// configuration, decrypting DNS provider credentials with cipher. Renewal
// timings and cert dir still come from TOML/env (operational, not per-tenant).
func acmeConfigFromDB(ctx context.Context, store *admin.PGStore, cipher *crypto.Cipher, cfg *config.Config) (certmanager.Config, error) {
	dbCfg, err := store.GetACMEConfig(ctx)
	if err != nil {
		return certmanager.Config{}, err
	}
	if !dbCfg.Enabled {
		return certmanager.Config{
			CertDir:       cfg.ACMECertDir,
			RenewInterval: cfg.ACMERenewInterval,
			RenewBefore:   cfg.ACMERenewBefore,
		}, nil
	}
	creds := map[string]string{}
	if dbCfg.CredentialsEnc != "" {
		pt, derr := cipher.Decrypt(dbCfg.CredentialsEnc)
		if derr != nil {
			return certmanager.Config{}, derr
		}
		if err := json.Unmarshal([]byte(pt), &creds); err != nil {
			return certmanager.Config{}, err
		}
	}
	rows, err := store.ListACMEDomains(ctx)
	if err != nil {
		return certmanager.Config{}, err
	}
	domains := make([]string, 0, len(rows))
	for _, r := range rows {
		domains = append(domains, r.Domain)
	}
	return certmanager.Config{
		Email:          dbCfg.Email,
		Domains:        domains,
		Directory:      dbCfg.Directory,
		CertDir:        cfg.ACMECertDir,
		DNSProvider:    dbCfg.DNSProvider,
		DNSCredentials: creds,
		RenewInterval:  cfg.ACMERenewInterval,
		RenewBefore:    cfg.ACMERenewBefore,
	}, nil
}

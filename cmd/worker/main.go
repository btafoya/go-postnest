package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/config"
	"github.com/go-postnest/postnest/internal/db"
	"github.com/go-postnest/postnest/internal/logger"
	"github.com/go-postnest/postnest/internal/mailstore"
	"github.com/go-postnest/postnest/internal/redis"
	"github.com/go-postnest/postnest/internal/workers"
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

	pool := workers.NewPool(redisClient, log, cfg.WorkerConcurrency, cfg.WorkerPollInterval)

	// Register processors
	pool.Register("inbound", workers.NewInboundProcessor(mailStore, authService, log))
	pool.Register("bounce", workers.NewBounceProcessor(pgPool, log))
	pool.Register("delivery", workers.NewDeliveryProcessor(pgPool, log))

	ctx, cancel := context.WithCancel(context.Background())
	pool.Start(ctx)
	log.Info("worker pool started", "concurrency", cfg.WorkerConcurrency)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down workers")
	cancel()

	// Give workers time to finish current job
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	<-shutdownCtx.Done()
	log.Info("worker shutdown complete")
}

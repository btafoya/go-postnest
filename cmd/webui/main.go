package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-postnest/postnest/internal/logger"
	"github.com/go-postnest/postnest/internal/webui"
)

func main() {
	log := logger.New()

	// Parse allowed origins from env
	var allowedOrigins []string
	if origins := os.Getenv("WEBUI_ALLOWED_ORIGINS"); origins != "" {
		allowedOrigins = strings.Split(origins, ",")
	}

	webuiCfg := webui.Config{
		Addr:           getEnv("WEBUI_ADDR", ":3000"),
		APIBaseURL:     getEnv("WEBUI_API_BASE_URL", "http://localhost:8080"),
		AllowedOrigins: allowedOrigins,
		RedisURL:       getEnv("WEBUI_REDIS_URL", os.Getenv("POSTNEST_REDIS_URL")),
		Log:            log,
	}

	if webuiCfg.RedisURL == "" {
		webuiCfg.RedisURL = "redis://localhost:6379/0"
	}

	gin.SetMode(gin.ReleaseMode)
	router := webui.NewRouter(webuiCfg)

	srv := &http.Server{
		Addr:         webuiCfg.Addr,
		Handler:      router,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    120 * time.Second,
	}

	go func() {
		log.Info("webui server starting", "addr", webuiCfg.Addr, "api", webuiCfg.APIBaseURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("webui server error", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down webui")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("webui shutdown error", "error", err)
	}
	log.Info("shutdown complete")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

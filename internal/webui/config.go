package webui

import (
	"log/slog"
	"time"
)

// Config holds web UI server configuration.
type Config struct {
	Addr           string
	APIBaseURL     string
	AllowedOrigins []string
	SessionKey     string
	SessionExpiry  time.Duration
	RedisURL       string
	Log            *slog.Logger
}

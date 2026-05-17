package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration sourced from environment variables.
type Config struct {
	// Server
	HTTPAddr     string        `env:"HTTP_ADDR" envDefault:":8080"`
	IMAPAddr     string        `env:"IMAP_ADDR" envDefault:":143"`
	IMAPSAddr    string        `env:"IMAPS_ADDR" envDefault:":993"`
	SMTPAddr     string        `env:"SMTP_ADDR" envDefault:":587"`
	SMTPSAddr    string        `env:"SMTPS_ADDR" envDefault:":465"`
	ReadTimeout  time.Duration `env:"READ_TIMEOUT" envDefault:"30s"`
	WriteTimeout time.Duration `env:"WRITE_TIMEOUT" envDefault:"30s"`
	AllowedOrigins []string

	// TLS
	TLSCertPath string `env:"TLS_CERT_PATH"`
	TLSKeyPath  string `env:"TLS_KEY_PATH"`

	// ACME
	ACMEEnabled       bool          `env:"ACME_ENABLED"`
	ACMEEmail         string        `env:"ACME_EMAIL"`
	ACMEDomain        string        `env:"ACME_DOMAIN"`
	ACMEDirectory     string        `env:"ACME_DIRECTORY"`
	ACMECertDir       string        `env:"ACME_CERT_DIR"`
	ACMEDNSProvider   string        `env:"ACME_DNS_PROVIDER"`
	ACMERenewInterval time.Duration `env:"ACME_RENEW_INTERVAL"`
	ACMERenewBefore   time.Duration `env:"ACME_RENEW_BEFORE"`

	// Database
	PostgresDSN     string `env:"POSTGRES_DSN"`
	PostgresReadDSN string `env:"POSTGRES_READ_DSN"`
	MaxDBConns      int    `env:"MAX_DB_CONNS" envDefault:"25"`

	// Redis
	RedisURL string `env:"REDIS_URL" envDefault:"redis://localhost:6379/0"`

	// Auth
	Argon2idTime    uint32        `env:"ARGON2ID_TIME" envDefault:"3"`
	Argon2idMemory  uint32        `env:"ARGON2ID_MEMORY" envDefault:"65536"`
	Argon2idThreads uint8         `env:"ARGON2ID_THREADS" envDefault:"4"`
	SessionKey      string        `env:"SESSION_KEY"`
	SessionExpiry   time.Duration `env:"SESSION_EXPIRY" envDefault:"168h"`

	// Postmark
	PostmarkWebhookSecret string `env:"POSTMARK_WEBHOOK_SECRET"`

	// Workers
	WorkerConcurrency  int           `env:"WORKER_CONCURRENCY" envDefault:"10"`
	WorkerPollInterval time.Duration `env:"WORKER_POLL_INTERVAL" envDefault:"5s"`

	// Security
	AllowInsecureAuth bool `env:"ALLOW_INSECURE_AUTH" envDefault:"false"` // Allow plaintext IMAP/SMTP auth without TLS

	// Limits
	MaxMessageSize    int64 `env:"MAX_MESSAGE_SIZE" envDefault:"52428800"`    // 50MB
	MaxAttachmentSize int64 `env:"MAX_ATTACHMENT_SIZE" envDefault:"26214400"` // 25MB
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	return NewLoader("").Load()
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}

func parseInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return i
}

func parseDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

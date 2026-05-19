package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const defaultConfigPath = "/etc/postnest/postnest.conf"

// rawConfig mirrors the TOML file shape.
type rawConfig struct {
	ConfigVersion int         `toml:"config_version"`
	Server        rawServer   `toml:"server"`
	Database      rawDatabase `toml:"database"`
	Redis         rawRedis    `toml:"redis"`
	Postmark      rawPostmark `toml:"postmark"`
	TLS           rawTLS      `toml:"tls"`
	Worker        rawWorker   `toml:"worker"`
	Security      rawSecurity `toml:"security"`
	ACME          rawACME     `toml:"acme"`
}
type rawServer struct {
	HTTPAddr     string        `toml:"http_addr"`
	IMAPAddr     string        `toml:"imap_addr"`
	IMAPSAddr    string        `toml:"imaps_addr"`
	SMTPAddr     string        `toml:"smtp_addr"`
	SMTPSAddr    string        `toml:"smtps_addr"`
	ReadTimeout  time.Duration `toml:"read_timeout"`
	WriteTimeout time.Duration `toml:"write_timeout"`
	AllowedOrigins []string `toml:"allowed_origins"`
}

type rawDatabase struct {
	DSN      string `toml:"dsn"`
	ReadDSN  string `toml:"read_dsn"`
	MaxConns int    `toml:"max_conns"`
}

type rawRedis struct {
	URL string `toml:"url"`
}

type rawPostmark struct {
	WebhookSecret string `toml:"webhook_secret"`
}

type rawTLS struct {
	CertPath string `toml:"cert_path"`
	KeyPath  string `toml:"key_path"`
}

type rawWorker struct {
	Concurrency  int           `toml:"concurrency"`
	PollInterval time.Duration `toml:"poll_interval"`
}

type rawSecurity struct {
	SessionKey        string        `toml:"session_key"`
	SessionExpiry     time.Duration `toml:"session_expiry"`
	Argon2idTime      uint32        `toml:"argon2id_time"`
	Argon2idMemory    uint32        `toml:"argon2id_memory"`
	Argon2idThreads   uint8         `toml:"argon2id_threads"`
	MaxMessageSize    int64         `toml:"max_message_size"`
	MaxAttachmentSize int64         `toml:"max_attachment_size"`
	AllowInsecureAuth bool          `toml:"allow_insecure_auth"`
	SecretKey         string        `toml:"secret_key"`
}

type rawACME struct {
	Enabled       bool          `toml:"enabled"`
	Email         string        `toml:"email"`
	Domain        string        `toml:"domain"`
	Directory     string        `toml:"directory"`
	CertDir       string        `toml:"cert_dir"`
	DNSProvider   string        `toml:"dns_provider"`
	RenewInterval time.Duration `toml:"renew_interval"`
	RenewBefore   time.Duration `toml:"renew_before"`
}

// Loader reads TOML configuration and applies environment variable overrides.
type Loader struct {
	Path string
}

// NewLoader creates a loader for the given path. If path is empty, it uses
// POSTNEST_CONFIG_PATH or the default /etc/postnest/postnest.conf.
func NewLoader(path string) *Loader {
	if path == "" {
		path = os.Getenv("POSTNEST_CONFIG_PATH")
		if path == "" {
			path = defaultConfigPath
		}
	}
	return &Loader{Path: path}
}

// Load reads the configuration file (if present), applies env overrides, and
// returns the populated Config. It validates that required fields are present.
func (l *Loader) Load() (*Config, error) {
	raw := rawConfig{
		ConfigVersion: 1,
		Server: rawServer{
			HTTPAddr:     ":8080",
			IMAPAddr:     ":143",
			IMAPSAddr:    ":993",
			SMTPAddr:     ":587",
			SMTPSAddr:    ":465",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			AllowedOrigins: []string{},
		},
		Database: rawDatabase{
			MaxConns: 25,
		},
		Redis: rawRedis{
			URL: "redis://localhost:6379/0",
		},
		Worker: rawWorker{
			Concurrency:  10,
			PollInterval: 5 * time.Second,
		},
		Security: rawSecurity{
			SessionExpiry:     168 * time.Hour,
			Argon2idTime:      3,
			Argon2idMemory:    65536,
			Argon2idThreads:   4,
			MaxMessageSize:    52428800,
			MaxAttachmentSize: 26214400,
		},
		ACME: rawACME{
			Directory:     "staging",
			CertDir:       "/var/lib/postnest/certs",
			DNSProvider:   "cloudflare",
			RenewInterval: 24 * time.Hour,
			RenewBefore:   720 * time.Hour,
		},
	}

	// Load file if it exists.
	if _, err := os.Stat(l.Path); err == nil {
		if _, err := toml.DecodeFile(l.Path, &raw); err != nil {
			return nil, fmt.Errorf("failed to decode config file %s: %w", l.Path, err)
		}
	}

	// Apply env overrides.
	applyEnvOverrides(&raw)

	// Translate to the existing Config struct.
	cfg := &Config{
		HTTPAddr:              raw.Server.HTTPAddr,
		IMAPAddr:              raw.Server.IMAPAddr,
		IMAPSAddr:             raw.Server.IMAPSAddr,
		SMTPAddr:              raw.Server.SMTPAddr,
		SMTPSAddr:             raw.Server.SMTPSAddr,
		ReadTimeout:           raw.Server.ReadTimeout,
		WriteTimeout:          raw.Server.WriteTimeout,
		AllowedOrigins:        raw.Server.AllowedOrigins,
		TLSCertPath:           raw.TLS.CertPath,
		TLSKeyPath:            raw.TLS.KeyPath,
		PostgresDSN:           raw.Database.DSN,
		PostgresReadDSN:       raw.Database.ReadDSN,
		MaxDBConns:            raw.Database.MaxConns,
		RedisURL:              raw.Redis.URL,
		Argon2idTime:          raw.Security.Argon2idTime,
		Argon2idMemory:        raw.Security.Argon2idMemory,
		Argon2idThreads:       raw.Security.Argon2idThreads,
		SessionKey:            raw.Security.SessionKey,
		SessionExpiry:         raw.Security.SessionExpiry,
		PostmarkWebhookSecret: raw.Postmark.WebhookSecret,
		WorkerConcurrency:     raw.Worker.Concurrency,
		WorkerPollInterval:    raw.Worker.PollInterval,
		MaxMessageSize:        raw.Security.MaxMessageSize,
		MaxAttachmentSize:     raw.Security.MaxAttachmentSize,
		AllowInsecureAuth:     raw.Security.AllowInsecureAuth,
		SecretKey:             raw.Security.SecretKey,
		ACMEEnabled:           raw.ACME.Enabled,
		ACMEEmail:             raw.ACME.Email,
		ACMEDomain:            raw.ACME.Domain,
		ACMEDirectory:         raw.ACME.Directory,
		ACMECertDir:           raw.ACME.CertDir,
		ACMEDNSProvider:       raw.ACME.DNSProvider,
		ACMERenewInterval:     raw.ACME.RenewInterval,
		ACMERenewBefore:       raw.ACME.RenewBefore,
	}

	// Validation.
	var missing []string
	if cfg.PostgresDSN == "" {
		missing = append(missing, "database.dsn (set in TOML or POSTNEST_DATABASE_DSN)")
	}
	if cfg.SessionKey == "" {
		missing = append(missing, "security.session_key (set in TOML or POSTNEST_SECURITY_SESSION_KEY)")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("config validation failed:\n  - %s", strings.Join(missing, "\n  - "))
	}

	return cfg, nil
}

// applyEnvOverrides walks the rawConfig struct reflectively and checks for
// POSTNEST_<SECTION>_<KEY> environment variables.
func applyEnvOverrides(raw *rawConfig) {
	v := reflect.ValueOf(raw).Elem()
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		sectionField := v.Field(i)
		sectionType := t.Field(i)

		// Skip non-struct fields at the top level (e.g. ConfigVersion).
		if sectionField.Kind() != reflect.Struct {
			continue
		}

		sectionName := strings.ToUpper(sectionType.Name)
		applySectionOverrides(sectionField, "POSTNEST_"+sectionName+"_")
	}
}

func applySectionOverrides(v reflect.Value, prefix string) {
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)
		key := toScreamingSnakeCase(fieldType.Name)
		envKey := prefix + key

		// Prefer new POSTNEST_* names, fallback to legacy names.
		envVal := os.Getenv(envKey)
		if envVal == "" {
			envVal = legacyEnv(prefix + key)
		}
		if envVal == "" {
			continue
		}

		switch field.Kind() {
		case reflect.String:
			field.SetString(envVal)
		case reflect.Int, reflect.Int64:
			if n, err := strconv.ParseInt(envVal, 10, 64); err == nil {
				field.SetInt(n)
			}
		case reflect.Uint, reflect.Uint32, reflect.Uint8:
			if n, err := strconv.ParseUint(envVal, 10, 64); err == nil {
				field.SetUint(n)
			}
		case reflect.Bool:
			if b, err := strconv.ParseBool(envVal); err == nil {
				field.SetBool(b)
			}
		default:
			if field.Type().String() == "time.Duration" {
				if d, err := time.ParseDuration(envVal); err == nil {
					field.Set(reflect.ValueOf(d))
				}
			}
		}
	}
}

// legacyEnv maps new-style POSTNEST_* env vars to the legacy names used
// before the TOML config system was introduced. This preserves backward
// compatibility for existing deployments.
func legacyEnv(newKey string) string {
	legacy := map[string]string{
		"POSTNEST_SERVER_HTTP_ADDR":             "HTTP_ADDR",
		"POSTNEST_SERVER_IMAP_ADDR":             "IMAP_ADDR",
		"POSTNEST_SERVER_IMAPS_ADDR":            "IMAPS_ADDR",
		"POSTNEST_SERVER_SMTP_ADDR":             "SMTP_ADDR",
		"POSTNEST_SERVER_SMTPS_ADDR":            "SMTPS_ADDR",
		"POSTNEST_SERVER_READ_TIMEOUT":          "READ_TIMEOUT",
		"POSTNEST_SERVER_WRITE_TIMEOUT":         "WRITE_TIMEOUT",
		"POSTNEST_TLS_CERT_PATH":                "TLS_CERT_PATH",
		"POSTNEST_TLS_KEY_PATH":                 "TLS_KEY_PATH",
		"POSTNEST_DATABASE_DSN":                 "POSTGRES_DSN",
		"POSTNEST_DATABASE_READ_DSN":            "POSTGRES_READ_DSN",
		"POSTNEST_DATABASE_MAX_CONNS":           "MAX_DB_CONNS",
		"POSTNEST_REDIS_URL":                    "REDIS_URL",
		"POSTNEST_SECURITY_ARGON2ID_TIME":       "ARGON2ID_TIME",
		"POSTNEST_SECURITY_ARGON2ID_MEMORY":     "ARGON2ID_MEMORY",
		"POSTNEST_SECURITY_ARGON2ID_THREADS":    "ARGON2ID_THREADS",
		"POSTNEST_SECURITY_SESSION_KEY":         "SESSION_KEY",
		"POSTNEST_SECURITY_SESSION_EXPIRY":      "SESSION_EXPIRY",
		"POSTNEST_POSTMARK_WEBHOOK_SECRET":      "POSTMARK_WEBHOOK_SECRET",
		"POSTNEST_WORKER_CONCURRENCY":           "WORKER_CONCURRENCY",
		"POSTNEST_WORKER_POLL_INTERVAL":         "WORKER_POLL_INTERVAL",
		"POSTNEST_SECURITY_MAX_MESSAGE_SIZE":    "MAX_MESSAGE_SIZE",
		"POSTNEST_SECURITY_MAX_ATTACHMENT_SIZE": "MAX_ATTACHMENT_SIZE",
		"POSTNEST_SECURITY_ALLOW_INSECURE_AUTH": "ALLOW_INSECURE_AUTH",
		"POSTNEST_SECURITY_SECRET_KEY":          "POSTNEST_SECRET_KEY",
	}
	if old, ok := legacy[newKey]; ok {
		return os.Getenv(old)
	}
	return ""
}

// toScreamingSnakeCase converts a CamelCase string to SCREAMING_SNAKE_CASE.
func toScreamingSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := rune(s[i-1])
			if prev >= 'a' && prev <= 'z' {
				result = append(result, '_')
			}
		}
		result = append(result, r)
	}
	return strings.ToUpper(string(result))
}

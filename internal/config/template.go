package config

import (
	"fmt"
	"io"
	"strings"
)

// PrintTemplate writes a documented TOML configuration file to w.
func PrintTemplate(w io.Writer) error {
	var b strings.Builder
	b.WriteString("# PostNest Configuration File\n")
	b.WriteString("# Place at /etc/postnest/postnest.conf or set POSTNEST_CONFIG_PATH\n")
	b.WriteString("# All values can be overridden via environment variables:\n")
	b.WriteString("#   POSTNEST_SERVER_HTTP_ADDR=:8080\n")
	b.WriteString("#   POSTNEST_DATABASE_DSN=postgres://...\n")
	b.WriteString("#   etc.\n\n")

	b.WriteString("config_version = 1\n\n")

	b.WriteString("[server]\n")
	b.WriteString("http_addr     = \":8080\"\n")
	b.WriteString("imap_addr     = \":143\"\n")
	b.WriteString("imaps_addr    = \":993\"\n")
	b.WriteString("smtp_addr     = \":587\"\n")
	b.WriteString("smtps_addr    = \":465\"\n")
	b.WriteString("read_timeout  = \"30s\"\n")
	b.WriteString("write_timeout = \"30s\"\n\n")

	b.WriteString("[database]\n")
	b.WriteString("dsn      = \"postgres://postnest:changeme@localhost:5432/postnest?sslmode=disable\"\n")
	b.WriteString("read_dsn = \"\"\n")
	b.WriteString("max_conns = 25\n\n")

	b.WriteString("[redis]\n")
	b.WriteString("url = \"redis://localhost:6379/0\"\n\n")

	b.WriteString("[postmark]\n")
	b.WriteString("webhook_secret = \"\"\n\n")

	b.WriteString("[tls]\n")
	b.WriteString("cert_path = \"\"\n")
	b.WriteString("key_path  = \"\"\n\n")

	b.WriteString("[acme]\n")
	b.WriteString("enabled        = false\n")
	b.WriteString("email          = \"admin@example.com\"\n")
	b.WriteString("domain         = \"mail.example.com\"\n")
	b.WriteString("directory      = \"staging\"\n")
	b.WriteString("cert_dir       = \"/var/lib/postnest/certs\"\n")
	b.WriteString("dns_provider   = \"cloudflare\"\n")
	b.WriteString("renew_interval = \"24h\"\n")
	b.WriteString("renew_before   = \"720h\"\n\n")
	b.WriteString("[worker]\n")
	b.WriteString("concurrency   = 10\n")
	b.WriteString("poll_interval = \"5s\"\n\n")

	b.WriteString("[security]\n")
	b.WriteString("session_key         = \"change-me-in-production\"\n")
	b.WriteString("session_expiry      = \"168h\"\n")
	b.WriteString("argon2id_time       = 3\n")
	b.WriteString("argon2id_memory     = 65536\n")
	b.WriteString("argon2id_threads    = 4\n")
	b.WriteString("max_message_size    = 52428800\n")
	b.WriteString("max_attachment_size = 26214400\n\n")

	_, err := fmt.Fprint(w, b.String())
	return err
}

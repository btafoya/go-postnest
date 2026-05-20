---
name: mailtester
description: AI agent skill for diagnosing SMTP/IMAP connectivity, authentication, and delivery issues using the go-mailtester CLI. Trigger when user mentions SMTP, IMAP, mail servers, email testing, TLS negotiation, STARTTLS, port 25/587/465/993/143, or authentication failures with mail services.
---

# mailtester

AI agent skill for using the `go-mailtester` CLI SMTP testing tool.

## Purpose

This skill enables Claude Code to diagnose SMTP connectivity, authentication, and delivery issues using the `mailtester` binary built from this repository.

## Building

From the project root:

```bash
go build -o mailtester .
```

## Test Modes

| Mode | What it tests |
|------|---------------|
| `connection` | TCP connectivity + EHLO + extension listing |
| `starttls` | STARTTLS negotiation + TLS state inspection |
| `ssl` | Implicit TLS (port 465) connection + TLS state + EHLO + auth |
| `auth` | Authentication with configured credentials |
| `send` | Full manual MAIL FROM → RCPT TO → DATA → QUIT pipeline |
| `sendmail` | High-level `smtp.SendMail()` helper |
| `sendmailtls` | High-level `smtp.SendMailTLS()` helper (implicit TLS) |
| `raw` | Raw textproto SMTP session (no library abstractions) |
| `imap-connection` | Plaintext IMAP dial + greeting + capabilities |
| `imap-starttls` | IMAP STARTTLS upgrade + capabilities |
| `imap-ssl` | Implicit TLS IMAP (port 993) + greeting + capabilities |
| `imap-auth` | IMAP authentication with configured credentials |
| `imap-list` | List all IMAP mailboxes |
| `imap-status` | Select mailbox and report counts |
| `imap-fetch` | Fetch first message envelope and flags |
| `all` | Runs all SMTP tests sequentially |
| `imap-all` | Runs all IMAP tests sequentially |

## Common Flags

```
-host        SMTP/IMAP server hostname (default: localhost)
-port        SMTP/IMAP server port (default: 25)
-from        Sender email address
-to          Recipient(s), comma-separated
-subject     Email subject
-body        Email body text
-user        SMTP/IMAP username
-pass        SMTP/IMAP password
-auth        Auth mechanism: plain, login, none (default: plain)
-tls         Use implicit TLS (port 465 for SMTP, 993 for IMAP)
-starttls    Use STARTTLS
-skip-verify Skip TLS certificate verification
-timeout     Connection timeout (default: 30s)
-helo        EHLO/HELO hostname (default: go-mailtester)
-mode        Test mode (default: all)
-mailbox     IMAP mailbox to test (default: INBOX)
```

## Typical Workflows

**Quick connectivity check:**
```bash
./mailtester -host smtp.example.com -port 587 -mode connection
```

**Test authentication:**
```bash
./mailtester -host smtp.example.com -port 587 -user alice -pass secret -mode auth
```

**Send a test message via STARTTLS + PLAIN auth:**
```bash
./mailtester -host smtp.example.com -port 587 -starttls \
  -from alice@example.com -to bob@example.net \
  -user alice -pass secret -mode send
```

**Test implicit TLS (port 465):**
```bash
./mailtester -host smtp.example.com -port 465 -tls \
  -from alice@example.com -to bob@example.net \
  -user alice -pass secret -mode ssl
```

**Run full diagnostic suite:**
```bash
./mailtester -host smtp.example.com -port 587 -starttls \
  -from alice@example.com -to bob@example.net \
  -user alice -pass secret -mode all
```

**Quick IMAP connectivity check:**
```bash
./mailtester -host imap.example.com -port 993 -tls -mode imap-connection
```

**Test IMAP authentication:**
```bash
./mailtester -host imap.example.com -port 993 -tls \
  -user alice -pass secret -mode imap-auth
```

**List IMAP mailboxes:**
```bash
./mailtester -host imap.example.com -port 993 -tls \
  -user alice -pass secret -mode imap-list
```

**Check I mailbox status:**
```bash
./mailtester -host imap.example.com -port 993 -tls \
  -user alice -pass secret -mode imap-status -mailbox INBOX
```

**Full IMAP diagnostic suite:**
```bash
./mailtester -host imap.example.com -port 993 -tls \
  -user alice -pass secret -mode imap-all
```

## Notes for AI Agents

- Always specify `-mode` explicitly when the user describes a specific test; default `all` requires `-from` and `-to` for SMTP send phases.
- If the user mentions "port 465", add `-tls` and suggest `ssl` mode. If they mention "port 587", add `-starttls`.
- If the user mentions "port 993", add `-tls` and suggest `imap-ssl` or `imap-connection` mode.
- If the user mentions "port 143", suggest `imap-starttls` or `imap-connection` mode.
- `-skip-verify` is useful for self-signed certificates but should only be suggested after a normal TLS failure.
- `-timeout` applies to all connection paths: `net.DialTimeout` for plaintext/STARTTLS, `tls.DialWithDialer` for implicit TLS.
- The tool uses `github.com/emersion/go-smtp` for SMTP and `github.com/emersion/go-imap/v2` for IMAP; errors come from those libraries.
- IMAP `imap-fetch` mode requires at least one message in the selected mailbox (default `INBOX`); use `-mailbox` to target a different mailbox.

package smtp

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/mail"
	"strings"
	"time"

	gomail "github.com/emersion/go-message/mail"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/mailstore"
	"github.com/go-postnest/postnest/internal/models"
	"github.com/go-postnest/postnest/internal/postmark"
	"github.com/google/uuid"
)

// Server wraps the go-smtp server.
type Server struct {
	addr     string
	tlsCfg   *tls.Config
	startTLS bool
	srv      *smtp.Server
	ln       net.Listener
}

// NewServer creates an SMTP server with implicit TLS when tlsCfg is set.
func NewServer(addr string, tlsCfg *tls.Config, allowInsecureAuth bool, store mailstore.Store, auth *auth.Service, pm *postmark.Client, maxMsgSize int64) *Server {
	return newServer(addr, tlsCfg, allowInsecureAuth, false, store, auth, pm, maxMsgSize)
}

// NewStartTLSServer creates an SMTP server that advertises STARTTLS.
func NewStartTLSServer(addr string, tlsCfg *tls.Config, allowInsecureAuth bool, store mailstore.Store, auth *auth.Service, pm *postmark.Client, maxMsgSize int64) *Server {
	return newServer(addr, tlsCfg, allowInsecureAuth, true, store, auth, pm, maxMsgSize)
}

func newServer(addr string, tlsCfg *tls.Config, allowInsecureAuth bool, startTLS bool, store mailstore.Store, auth *auth.Service, pm *postmark.Client, maxMsgSize int64) *Server {
	be := &smtpBackend{store: store, auth: auth, postmark: pm, maxMsgSize: maxMsgSize}
	s := smtp.NewServer(be)
	s.Addr = addr
	s.AllowInsecureAuth = allowInsecureAuth
	if tlsCfg != nil {
		s.TLSConfig = tlsCfg
	}
	return &Server{addr: addr, tlsCfg: tlsCfg, startTLS: startTLS, srv: s}
}

// Start listens for SMTP connections.
func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	if s.tlsCfg != nil && !s.startTLS {
		ln = tls.NewListener(ln, s.tlsCfg)
	}
	s.ln = ln
	go func() {
		<-ctx.Done()
		_ = s.ln.Close()
	}()
	if err := s.srv.Serve(s.ln); err != nil && !isClosedErr(err) {
		return err
	}
	return nil
}

// Stop closes the listener and waits up to 30s for connections to drain.
func (s *Server) Stop() error {
	if s.ln != nil {
		_ = s.ln.Close()
	}
	// go-smtp server does not expose connection tracking.
	// Give in-flight DATA sessions a brief grace period.
	time.Sleep(2 * time.Second)
	return s.srv.Close()
}

func isClosedErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	return strings.Contains(err.Error(), "closed")
}

type smtpBackend struct {
	store       mailstore.Store
	auth        *auth.Service
	postmark    *postmark.Client
	maxMsgSize  int64
}

func (b *smtpBackend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &smtpSession{backend: b}, nil
}

type smtpSession struct {
	backend *smtpBackend
	from    string
	to      []string
	user    *models.User
}

func (s *smtpSession) Reset() {
	s.from = ""
	s.to = nil
}

func (s *smtpSession) Logout() error { return nil }

func (s *smtpSession) Mail(from string, opts *smtp.MailOptions) error {
	if s.user == nil {
		return &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 7, 1},
			Message:      "authentication required",
		}
	}
	parts := strings.Split(from, "@")
	if len(parts) != 2 || parts[1] == "" {
		return &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 1, 2},
			Message:      "invalid sender address",
		}
	}
	domainName := parts[1]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	domain, err := s.backend.auth.GetDomainByName(ctx, domainName)
	if err != nil {
		return &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 7, 1},
			Message:      "unknown domain",
		}
	}

	member, err := s.backend.auth.IsDomainMember(ctx, s.user.ID, domain.ID)
	if err != nil {
		return &smtp.SMTPError{
			Code:         451,
			EnhancedCode: smtp.EnhancedCode{4, 3, 0},
			Message:      "unable to verify domain membership",
		}
	}
	if !member {
		return &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 7, 1},
			Message:      "sender domain not authorized",
		}
	}

	s.from = from
	return nil
}

func (s *smtpSession) Rcpt(to string, opts *smtp.RcptOptions) error {
	if _, err := mail.ParseAddress(to); err != nil {
		return &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 1, 3},
			Message:      "invalid recipient address",
		}
	}
	s.to = append(s.to, to)
	return nil
}

func (s *smtpSession) Data(r io.Reader) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if s.user == nil {
		return &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 7, 1},
			Message:      "authentication required",
		}
	}

	maxSize := s.backend.maxMsgSize
	var raw []byte
	var err error
	if maxSize > 0 {
		raw, err = io.ReadAll(io.LimitReader(r, maxSize+1))
	} else {
		raw, err = io.ReadAll(r)
	}
	if err != nil {
		return &smtp.SMTPError{
			Code:         451,
			EnhancedCode: smtp.EnhancedCode{4, 4, 5},
			Message:      "failed to read message",
		}
	}
	if maxSize > 0 && int64(len(raw)) > maxSize {
		return &smtp.SMTPError{
			Code:         552,
			EnhancedCode: smtp.EnhancedCode{5, 3, 4},
			Message:      "message exceeds maximum size",
		}
	}

	mr, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return &smtp.SMTPError{
			Code:         451,
			EnhancedCode: smtp.EnhancedCode{4, 4, 5},
			Message:      "failed to parse message",
		}
	}

	subject, _ := mr.Header.Subject()
	fromList, _ := mr.Header.AddressList("From")
	ccList, _ := mr.Header.AddressList("Cc")
	bccList, _ := mr.Header.AddressList("Bcc")
	msgID := mr.Header.Get("Message-Id")

	var textBody, htmlBody string
	var attachments []postmark.Attachment

	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			slog.Default().Error("smtp: failed to read part", "error", err)
			break
		}

		var ct string
		switch h := p.Header.(type) {
		case *gomail.InlineHeader:
			ct, _, _ = h.ContentType()
		case *gomail.AttachmentHeader:
			ct, _, _ = h.ContentType()
		default:
			ct = p.Header.Get("Content-Type")
		}

		switch {
		case strings.HasPrefix(ct, "text/plain") && textBody == "":
			b, _ := io.ReadAll(p.Body)
			textBody = string(b)
		case strings.HasPrefix(ct, "text/html") && htmlBody == "":
			b, _ := io.ReadAll(p.Body)
			htmlBody = string(b)
		default:
			b, _ := io.ReadAll(p.Body)
			name := "attachment"
			var disp string
			var params map[string]string
			switch h := p.Header.(type) {
			case *gomail.InlineHeader:
				disp, params, _ = h.ContentDisposition()
			case *gomail.AttachmentHeader:
				disp, params, _ = h.ContentDisposition()
			default:
				disp = p.Header.Get("Content-Disposition")
			}
			if disp == "attachment" || disp == "inline" {
				if n, ok := params["filename"]; ok {
					name = n
				}
			}
			attachments = append(attachments, postmark.Attachment{
				Name:        name,
				ContentType: ct,
				Content:     b,
			})
		}
	}

	domainName := ""
	if len(fromList) > 0 && fromList[0] != nil {
		parts := strings.Split(fromList[0].Address, "@")
		if len(parts) == 2 {
			domainName = parts[1]
		}
	}

	domain, err := s.backend.auth.GetDomainByName(ctx, domainName)
	if err != nil {
		return &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 7, 1},
			Message:      "unknown domain",
		}
	}

	msg := &postmark.OutboundMessage{
		From:          s.from,
		To:            s.to,
		Subject:       subject,
		TextBody:      textBody,
		HTMLBody:      htmlBody,
		MessageStream: domain.PostmarkStream,
		Attachments:   attachments,
	}
	if len(ccList) > 0 {
		msg.Cc = addressesToStrings(ccList)
	}
	if len(bccList) > 0 {
		msg.Bcc = addressesToStrings(bccList)
	}

	res, err := s.backend.postmark.SendEmail(ctx, domain.PostmarkToken, msg)
	if err != nil {
		return &smtp.SMTPError{
			Code:         451,
			EnhancedCode: smtp.EnhancedCode{4, 4, 5},
			Message:      err.Error(),
		}
	}
	if res.ErrorCode != 0 {
		return &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 0, 0},
			Message:      res.Message,
		}
	}

	// Ensure system labels exist before looking up SENT.
	if err := s.backend.store.EnsureSystemLabels(ctx, domain.ID, s.user.ID); err != nil {
		slog.Default().Error("smtp: failed to ensure system labels", "error", err)
	}

	// Store copy in Sent
	sentLabel, err := s.backend.store.GetLabelByName(ctx, domain.ID, s.user.ID, "SENT")
	if err != nil {
		slog.Default().Error("smtp: failed to find SENT label", "error", err)
	}

	m := &models.Message{
		ID:              uuid.Must(uuid.NewV7()),
		DomainID:        domain.ID,
		UserID:          s.user.ID,
		MessageIDHeader: msgID,
		Subject:         subject,
		FromAddress:     s.from,
		ToAddresses:     s.to,
		CcAddresses:     addressesToStrings(ccList),
		BccAddresses:    addressesToStrings(bccList),
		Date:            time.Now(),
		PlainText:       textBody,
		HTMLBody:        htmlBody,
		Source:          raw,
		SizeBytes:       len(raw),
		IsOutbound:      true,
		IsRead:          true,
	}
	var labelIDs []uuid.UUID
	if sentLabel != nil {
		labelIDs = append(labelIDs, sentLabel.ID)
	}
	if err := s.backend.store.CreateMessage(ctx, m, labelIDs, nil); err != nil {
		slog.Default().Error("smtp: failed to store sent message", "error", err)
	}

	return nil
}

func (s *smtpSession) AuthMechanisms() []string {
	return []string{sasl.Plain, "LOGIN"}
}

func (s *smtpSession) Auth(mech string) (sasl.Server, error) {
	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(identity, username, password string) error {
			ctx := context.Background()
			user, err := s.backend.auth.Authenticate(ctx, username, password)
			if err != nil {
				return err
			}
			s.user = user
			return nil
		}), nil
	case "LOGIN":
		return &loginServer{
			authenticate: func(username, password string) error {
				ctx := context.Background()
				user, err := s.backend.auth.Authenticate(ctx, username, password)
				if err != nil {
					return err
				}
				s.user = user
				return nil
			},
		}, nil
	}
	return nil, smtp.ErrAuthUnknownMechanism
}

// loginServer implements the SASL LOGIN server mechanism.
type loginServer struct {
	authenticate func(username, password string) error
	username     string
}

func (ls *loginServer) Next(response []byte) (challenge []byte, done bool, err error) {
	if ls.username == "" {
		if len(response) == 0 {
			return []byte("Username:"), false, nil
		}
		ls.username = string(response)
		return []byte("Password:"), false, nil
	}
	return nil, true, ls.authenticate(ls.username, string(response))
}

func addressesToStrings(addrs []*gomail.Address) []string {
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if a != nil {
			out = append(out, a.Address)
		}
	}
	return out
}

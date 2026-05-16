package smtp

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net"
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
	addr string
	srv  *smtp.Server
}

// NewServer creates an SMTP server.
func NewServer(addr string, tlsCfg *tls.Config, store mailstore.Store, auth *auth.Service, pm *postmark.Client) *Server {
	be := &smtpBackend{store: store, auth: auth, postmark: pm}
	s := smtp.NewServer(be)
	s.Addr = addr
	s.AllowInsecureAuth = true
	if tlsCfg != nil {
		s.TLSConfig = tlsCfg
	}
	return &Server{addr: addr, srv: s}
}

// Start listens for SMTP connections.
func (s *Server) Start(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = s.srv.Close()
	}()
	if err := s.srv.ListenAndServe(); err != nil && !isClosedErr(err) {
		return err
	}
	return nil
}

// Stop shuts down the SMTP server.
func (s *Server) Stop() error {
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
	store    mailstore.Store
	auth     *auth.Service
	postmark *postmark.Client
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
	s.from = from
	return nil
}

func (s *smtpSession) Rcpt(to string, opts *smtp.RcptOptions) error {
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

	mr, err := gomail.CreateReader(r)
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
		PlainText:       textBody,
		HTMLBody:        htmlBody,
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
	return []string{sasl.Plain}
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
	}
	return nil, smtp.ErrAuthUnknownMechanism
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

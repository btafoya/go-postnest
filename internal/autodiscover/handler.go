package autodiscover

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/certmanager"
	"github.com/google/uuid"
)

// Handler serves autodiscover endpoints for Outlook, Thunderbird, and Apple Mail.
type Handler struct {
	authService *auth.Service
	certManager *certmanager.Manager
	log         *slog.Logger
}

// NewHandler creates an autodiscover handler.
func NewHandler(authService *auth.Service, certManager *certmanager.Manager, log *slog.Logger) *Handler {
	return &Handler{
		authService: authService,
		certManager: certManager,
		log:         log,
	}
}

// RegisterRoutes registers public autodiscover routes.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/autodiscover/autodiscover.xml", h.outlookAutodiscover)
	r.Get("/.well-known/autoconfig/mail/config-v1.1.xml", h.thunderbirdAutoconfig)
	r.Get("/mail/config-v1.1.xml", h.thunderbirdAutoconfig)
	r.Get("/email.mobileconfig", h.appleMobileConfig)
}

// autodiscoverData holds the validated account info used by all format templates.
type autodiscoverData struct {
	Email            string
	DisplayName      string
	Domain           string
	Host             string
	IMAPHost         string
	SMTPHost         string
	UUID             string
	DAVUUID          string
	ProfileUUID      string
	DAVWellKnownUUID string
}

// buildData validates the email/domain/user and builds template data.
func (h *Handler) buildData(ctx context.Context, r *http.Request, email string) (*autodiscoverData, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, errors.New("missing email")
	}
	if !strings.Contains(email, "@") {
		return nil, errors.New("invalid email")
	}

	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return nil, errors.New("invalid email")
	}
	localPart := email[:at]
	domainName := email[at+1:]

	// Validate domain exists
	_, err := h.authService.GetDomainByName(ctx, domainName)
	if err != nil {
		return nil, errors.New("invalid request")
	}

	// Validate user exists
	user, err := h.authService.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, errors.New("invalid request")
	}

	// Derive host from request Host header
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	if colon := strings.LastIndex(host, ":"); colon != -1 {
		host = host[:colon]
	}
	if host == "" {
		host = domainName
	}

	data := &autodiscoverData{
		Email:            email,
		DisplayName:      user.DisplayName,
		Domain:           domainName,
		Host:             host,
		IMAPHost:         "imap." + host,
		SMTPHost:         "smtp." + host,
		UUID:             uuid.Must(uuid.NewV7()).String(),
		DAVUUID:          uuid.Must(uuid.NewV7()).String(),
		ProfileUUID:      uuid.Must(uuid.NewV7()).String(),
		DAVWellKnownUUID: uuid.Must(uuid.NewV7()).String(),
	}
	if data.DisplayName == "" {
		data.DisplayName = localPart
	}
	return data, nil
}

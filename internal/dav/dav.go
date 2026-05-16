package dav

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav/carddav"
	"github.com/emersion/go-webdav/caldav"
	"github.com/go-chi/chi/v5"
	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/contacts"
	"github.com/go-postnest/postnest/internal/mailstore"
	"github.com/go-postnest/postnest/internal/models"
	"github.com/google/uuid"
)

type ctxKey string

const userCtxKey ctxKey = "dav-user"

// Handler serves CardDAV/CalDAV/WebDAV.
type Handler struct {
	auth      *auth.Service
	contacts  contacts.Store
	mailstore mailstore.Store
	carddavH  *carddav.Handler
	caldavH   *caldav.Handler
}

// NewHandler creates a DAV handler.
func NewHandler(auth *auth.Service, contactsStore contacts.Store, mailstore mailstore.Store) *Handler {
	h := &Handler{
		auth:      auth,
		contacts:  contactsStore,
		mailstore: mailstore,
	}
	h.carddavH = &carddav.Handler{
		Backend: &carddavBackend{handler: h},
		Prefix:  "/dav/contacts",
	}
	h.caldavH = &caldav.Handler{
		Backend: &caldavBackend{handler: h},
		Prefix:  "/dav/calendar",
	}
	return h
}

// RegisterRoutes mounts DAV routes.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/.well-known/carddav", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dav/contacts/", http.StatusMovedPermanently)
	})
	r.Get("/.well-known/caldav", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dav/calendar/", http.StatusMovedPermanently)
	})

	r.Mount("/dav/contacts", h.authMiddleware(h.carddavH))
	r.Mount("/dav/calendar", h.authMiddleware(h.caldavH))
}

func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="DAV"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		user, err := h.auth.Authenticate(r.Context(), username, password)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userFromContext(ctx context.Context) *models.User {
	u, _ := ctx.Value(userCtxKey).(*models.User)
	return u
}

func domainIDFromUser(ctx context.Context, authSvc *auth.Service, user *models.User) (uuid.UUID, error) {
	domains, err := authSvc.GetUserDomains(ctx, user.ID)
	if err != nil || len(domains) == 0 {
		return uuid.Nil, fmt.Errorf("no domain")
	}
	return domains[0].DomainID, nil
}

// --- CardDAV backend ---

type carddavBackend struct {
	handler *Handler
}

func (b *carddavBackend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	return "/dav/principal/", nil
}

func (b *carddavBackend) AddressBookHomeSetPath(ctx context.Context) (string, error) {
	return "/dav/contacts/", nil
}

func (b *carddavBackend) ListAddressBooks(ctx context.Context) ([]carddav.AddressBook, error) {
	return []carddav.AddressBook{
		{
			Path:        "/dav/contacts/default/",
			Name:        "Contacts",
			Description: "Default address book",
			SupportedAddressData: []carddav.AddressDataType{
				{ContentType: "text/vcard", Version: "3.0"},
				{ContentType: "text/vcard", Version: "4.0"},
			},
		},
	}, nil
}

func (b *carddavBackend) GetAddressBook(ctx context.Context, path string) (*carddav.AddressBook, error) {
	if path == "/dav/contacts/default/" || path == "/dav/contacts/default" {
		ab, _ := b.ListAddressBooks(ctx)
		return &ab[0], nil
	}
	return nil, fmt.Errorf("not found")
}

func (b *carddavBackend) CreateAddressBook(ctx context.Context, ab *carddav.AddressBook) error {
	return fmt.Errorf("create not supported")
}

func (b *carddavBackend) DeleteAddressBook(ctx context.Context, path string) error {
	return fmt.Errorf("delete not supported")
}

func (b *carddavBackend) ListAddressObjects(ctx context.Context, path string, req *carddav.AddressDataRequest) ([]carddav.AddressObject, error) {
	user := userFromContext(ctx)
	if user == nil {
		return nil, fmt.Errorf("unauthorized")
	}
	domainID, err := domainIDFromUser(ctx, b.handler.auth, user)
	if err != nil {
		return nil, err
	}
	list, _, err := b.handler.contacts.List(ctx, domainID, user.ID, 10000, 0)
	if err != nil {
		return nil, err
	}
	var out []carddav.AddressObject
	for _, c := range list {
		out = append(out, contactToAddressObject(c))
	}
	return out, nil
}

func (b *carddavBackend) GetAddressObject(ctx context.Context, path string, req *carddav.AddressDataRequest) (*carddav.AddressObject, error) {
	user := userFromContext(ctx)
	if user == nil {
		return nil, fmt.Errorf("unauthorized")
	}
	domainID, err := domainIDFromUser(ctx, b.handler.auth, user)
	if err != nil {
		return nil, err
	}
	id, err := contactIDFromPath(path)
	if err != nil {
		return nil, err
	}
	c, err := b.handler.contacts.GetByID(ctx, domainID, user.ID, id)
	if err != nil {
		return nil, err
	}
	ao := contactToAddressObject(c)
	return &ao, nil
}

func (b *carddavBackend) QueryAddressObjects(ctx context.Context, path string, query *carddav.AddressBookQuery) ([]carddav.AddressObject, error) {
	return b.ListAddressObjects(ctx, path, &query.DataRequest)
}

func (b *carddavBackend) PutAddressObject(ctx context.Context, path string, card vcard.Card, opts *carddav.PutAddressObjectOptions) (*carddav.AddressObject, error) {
	user := userFromContext(ctx)
	if user == nil {
		return nil, fmt.Errorf("unauthorized")
	}
	domainID, err := domainIDFromUser(ctx, b.handler.auth, user)
	if err != nil {
		return nil, err
	}
	id, err := contactIDFromPath(path)
	if err != nil {
		id = uuid.Must(uuid.NewV7())
	}
	c := &models.Contact{
		ID:           id,
		DomainID:     domainID,
		UserID:       user.ID,
		Name:         card.PreferredValue(vcard.FieldFormattedName),
		Email:        card.PreferredValue(vcard.FieldEmail),
		Phone:        card.PreferredValue(vcard.FieldTelephone),
		Organization: card.PreferredValue(vcard.FieldOrganization),
		VCardData:    vcardToString(card),
	}
	if err := b.handler.contacts.Create(ctx, c); err != nil {
		return nil, err
	}
	ao := contactToAddressObject(c)
	return &ao, nil
}

func (b *carddavBackend) DeleteAddressObject(ctx context.Context, path string) error {
	user := userFromContext(ctx)
	if user == nil {
		return fmt.Errorf("unauthorized")
	}
	domainID, err := domainIDFromUser(ctx, b.handler.auth, user)
	if err != nil {
		return err
	}
	id, err := contactIDFromPath(path)
	if err != nil {
		return err
	}
	return b.handler.contacts.Delete(ctx, domainID, user.ID, id)
}

func contactToAddressObject(c *models.Contact) carddav.AddressObject {
	card := make(vcard.Card)
	card.SetValue(vcard.FieldFormattedName, c.Name)
	card.SetValue(vcard.FieldEmail, c.Email)
	card.SetValue(vcard.FieldTelephone, c.Phone)
	card.SetValue(vcard.FieldOrganization, c.Organization)
	return carddav.AddressObject{
		Path:          fmt.Sprintf("/dav/contacts/default/%s.vcf", c.ID),
		ModTime:       c.UpdatedAt,
		ContentLength: int64(len(vcardToString(card))),
		ETag:          fmt.Sprintf(`"%s"`, c.UpdatedAt.Format(time.RFC3339)),
		Card:          card,
	}
}

func vcardToString(card vcard.Card) string {
	var sb strings.Builder
	enc := vcard.NewEncoder(&sb)
	_ = enc.Encode(card)
	return sb.String()
}

func contactIDFromPath(path string) (uuid.UUID, error) {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	return uuid.Parse(base)
}

// --- CalDAV stub ---

type caldavBackend struct {
	handler *Handler
}

func (b *caldavBackend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	return "/dav/principal/", nil
}

func (b *caldavBackend) CalendarHomeSetPath(ctx context.Context) (string, error) {
	return "/dav/calendar/", nil
}

func (b *caldavBackend) CreateCalendar(ctx context.Context, cal *caldav.Calendar) error {
	return fmt.Errorf("not implemented")
}

func (b *caldavBackend) ListCalendars(ctx context.Context) ([]caldav.Calendar, error) {
	return nil, fmt.Errorf("not implemented")
}

func (b *caldavBackend) GetCalendar(ctx context.Context, path string) (*caldav.Calendar, error) {
	return nil, fmt.Errorf("not implemented")
}

func (b *caldavBackend) GetCalendarObject(ctx context.Context, path string, req *caldav.CalendarCompRequest) (*caldav.CalendarObject, error) {
	return nil, fmt.Errorf("not implemented")
}

func (b *caldavBackend) ListCalendarObjects(ctx context.Context, path string, req *caldav.CalendarCompRequest) ([]caldav.CalendarObject, error) {
	return nil, fmt.Errorf("not implemented")
}

func (b *caldavBackend) QueryCalendarObjects(ctx context.Context, path string, query *caldav.CalendarQuery) ([]caldav.CalendarObject, error) {
	return nil, fmt.Errorf("not implemented")
}

func (b *caldavBackend) PutCalendarObject(ctx context.Context, path string, calendar *ical.Calendar, opts *caldav.PutCalendarObjectOptions) (*caldav.CalendarObject, error) {
	return nil, fmt.Errorf("not implemented")
}

func (b *caldavBackend) DeleteCalendarObject(ctx context.Context, path string) error {
	return fmt.Errorf("not implemented")
}

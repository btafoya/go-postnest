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
	"github.com/go-postnest/postnest/internal/calendar"
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
	calendar  calendar.Store
	carddavH  *carddav.Handler
	caldavH   *caldav.Handler
}

// NewHandler creates a DAV handler.
func NewHandler(auth *auth.Service, contactsStore contacts.Store, mailstore mailstore.Store, calendarStore calendar.Store) *Handler {
	h := &Handler{
		auth:      auth,
		contacts:  contactsStore,
		mailstore: mailstore,
		calendar:  calendarStore,
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

// --- CalDAV backend ---

type caldavBackend struct {
	handler *Handler
}

func (b *caldavBackend) ctx(ctx context.Context) (*models.User, uuid.UUID, error) {
	user := userFromContext(ctx)
	if user == nil {
		return nil, uuid.Nil, fmt.Errorf("unauthorized")
	}
	domainID, err := domainIDFromUser(ctx, b.handler.auth, user)
	if err != nil {
		return nil, uuid.Nil, err
	}
	return user, domainID, nil
}

func (b *caldavBackend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	return "/dav/principal/", nil
}

func (b *caldavBackend) CalendarHomeSetPath(ctx context.Context) (string, error) {
	return "/dav/calendar/", nil
}

func (b *caldavBackend) CreateCalendar(ctx context.Context, cal *caldav.Calendar) error {
	user, domainID, err := b.ctx(ctx)
	if err != nil {
		return err
	}
	return b.handler.calendar.CreateCalendar(ctx, &models.Calendar{
		DomainID: domainID, UserID: user.ID, Name: cal.Name, Description: cal.Description,
	})
}

func (b *caldavBackend) ListCalendars(ctx context.Context) ([]caldav.Calendar, error) {
	user, domainID, err := b.ctx(ctx)
	if err != nil {
		return nil, err
	}
	cals, err := b.handler.calendar.ListCalendars(ctx, domainID, user.ID)
	if err != nil {
		return nil, err
	}
	out := make([]caldav.Calendar, 0, len(cals))
	for _, c := range cals {
		out = append(out, caldav.Calendar{
			Path:                  fmt.Sprintf("/dav/calendar/%s/", c.ID),
			Name:                  c.Name,
			Description:           c.Description,
			SupportedComponentSet: []string{ical.CompEvent},
		})
	}
	return out, nil
}

func (b *caldavBackend) GetCalendar(ctx context.Context, path string) (*caldav.Calendar, error) {
	user, domainID, err := b.ctx(ctx)
	if err != nil {
		return nil, err
	}
	calID, err := calendarIDFromPath(path)
	if err != nil {
		return nil, err
	}
	c, err := b.handler.calendar.GetCalendar(ctx, domainID, user.ID, calID)
	if err != nil {
		return nil, err
	}
	return &caldav.Calendar{
		Path:                  fmt.Sprintf("/dav/calendar/%s/", c.ID),
		Name:                  c.Name,
		Description:           c.Description,
		SupportedComponentSet: []string{ical.CompEvent},
	}, nil
}

func (b *caldavBackend) GetCalendarObject(ctx context.Context, path string, req *caldav.CalendarCompRequest) (*caldav.CalendarObject, error) {
	calID, uid, err := objectPath(path)
	if err != nil {
		return nil, err
	}
	ev, err := b.handler.calendar.GetEvent(ctx, calID, uid)
	if err != nil {
		return nil, err
	}
	return eventToCalendarObject(ev)
}

func (b *caldavBackend) ListCalendarObjects(ctx context.Context, path string, req *caldav.CalendarCompRequest) ([]caldav.CalendarObject, error) {
	calID, err := calendarIDFromPath(path)
	if err != nil {
		return nil, err
	}
	events, err := b.handler.calendar.ListEvents(ctx, calID, time.Time{}, time.Time{})
	if err != nil {
		return nil, err
	}
	out := make([]caldav.CalendarObject, 0, len(events))
	for _, ev := range events {
		obj, err := eventToCalendarObject(ev)
		if err != nil {
			return nil, err
		}
		out = append(out, *obj)
	}
	return out, nil
}

func (b *caldavBackend) QueryCalendarObjects(ctx context.Context, path string, query *caldav.CalendarQuery) ([]caldav.CalendarObject, error) {
	return b.ListCalendarObjects(ctx, path, nil)
}

func (b *caldavBackend) PutCalendarObject(ctx context.Context, path string, cal *ical.Calendar, opts *caldav.PutCalendarObjectOptions) (*caldav.CalendarObject, error) {
	user, domainID, err := b.ctx(ctx)
	if err != nil {
		return nil, err
	}
	calID, uid, err := objectPath(path)
	if err != nil {
		return nil, err
	}
	var buf strings.Builder
	if err := ical.NewEncoder(icalWriter{&buf}).Encode(cal); err != nil {
		return nil, err
	}
	ev, err := calendar.ICSToEvent([]byte(buf.String()))
	if err != nil {
		return nil, err
	}
	ev.CalendarID = calID
	ev.DomainID = domainID
	ev.UserID = user.ID
	if ev.UID == "" {
		ev.UID = uid
	}
	if err := b.handler.calendar.PutEvent(ctx, ev); err != nil {
		return nil, err
	}
	_ = b.handler.calendar.BumpCTag(ctx, calID)
	return eventToCalendarObject(ev)
}

func (b *caldavBackend) DeleteCalendarObject(ctx context.Context, path string) error {
	calID, uid, err := objectPath(path)
	if err != nil {
		return err
	}
	if err := b.handler.calendar.DeleteEvent(ctx, calID, uid); err != nil {
		return err
	}
	return b.handler.calendar.BumpCTag(ctx, calID)
}

type icalWriter struct{ sb *strings.Builder }

func (w icalWriter) Write(p []byte) (int, error) { return w.sb.Write(p) }

func eventToCalendarObject(ev *models.CalendarEvent) (*caldav.CalendarObject, error) {
	data, err := calendar.EventToICS(ev)
	if err != nil {
		return nil, err
	}
	cal, err := ical.NewDecoder(strings.NewReader(string(data))).Decode()
	if err != nil {
		return nil, err
	}
	return &caldav.CalendarObject{
		Path:          fmt.Sprintf("/dav/calendar/%s/%s.ics", ev.CalendarID, ev.UID),
		ModTime:       ev.UpdatedAt,
		ContentLength: int64(len(data)),
		ETag:          ev.ETag,
		Data:          cal,
	}, nil
}

func calendarIDFromPath(path string) (uuid.UUID, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if id, err := uuid.Parse(parts[i]); err == nil {
			return id, nil
		}
	}
	return uuid.Nil, fmt.Errorf("no calendar id in path")
}

func objectPath(path string) (uuid.UUID, string, error) {
	base := filepath.Base(path)
	uid := strings.TrimSuffix(base, filepath.Ext(base))
	dir := filepath.Dir(path)
	calID, err := calendarIDFromPath(dir)
	if err != nil {
		return uuid.Nil, "", err
	}
	return calID, uid, nil
}

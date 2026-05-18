package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-postnest/postnest/internal/api"
	"github.com/go-postnest/postnest/internal/models"
	"github.com/google/uuid"
)

// DomainLister returns domain memberships for a user.
type DomainLister interface {
	GetUserDomains(ctx context.Context, userID uuid.UUID) ([]*models.DomainMember, error)
}

// Handler implements the calendar REST API.
type Handler struct {
	store Store
	auth  DomainLister
}

// NewHandler creates a calendar REST handler.
func NewHandler(store Store, authSvc DomainLister) *Handler {
	return &Handler{store: store, auth: authSvc}
}

// RegisterRoutes mounts calendar routes on a chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/calendars", h.listCalendars)
	r.Post("/api/v1/calendars", h.createCalendar)
	r.Get("/api/v1/calendar/events", h.listEvents)
	r.Post("/api/v1/calendar/events", h.createEvent)
	r.Patch("/api/v1/calendar/events/{id}", h.updateEvent)
	r.Delete("/api/v1/calendar/events/{id}", h.deleteEvent)
}

func (h *Handler) ctx(r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	u := api.UserFromContext(r.Context())
	if u == nil {
		return uuid.Nil, uuid.Nil, false
	}
	did := api.DomainIDFromContext(r.Context())
	if did == uuid.Nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if doms, err := h.auth.GetUserDomains(ctx, u.ID); err == nil && len(doms) > 0 {
			did = doms[0].DomainID
		}
	}
	if did == uuid.Nil {
		return uuid.Nil, uuid.Nil, false
	}
	return did, u.ID, true
}

func (h *Handler) defaultCalendar(r *http.Request, did, uid uuid.UUID) (*models.Calendar, error) {
	cals, err := h.store.ListCalendars(r.Context(), did, uid)
	if err != nil {
		return nil, err
	}
	if len(cals) > 0 {
		return cals[0], nil
	}
	cal := &models.Calendar{DomainID: did, UserID: uid, Name: "Calendar"}
	if err := h.store.CreateCalendar(r.Context(), cal); err != nil {
		return nil, err
	}
	return cal, nil
}

func (h *Handler) listCalendars(w http.ResponseWriter, r *http.Request) {
	did, uid, ok := h.ctx(r)
	if !ok {
		api.WriteError(w, api.ErrUnauthorized)
		return
	}
	cals, err := h.store.ListCalendars(r.Context(), did, uid)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	cdtos := make([]calendarDTO, 0, len(cals))
	for _, c := range cals {
		cdtos = append(cdtos, toCalendarDTO(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"calendars": cdtos})
}

func (h *Handler) createCalendar(w http.ResponseWriter, r *http.Request) {
	did, uid, ok := h.ctx(r)
	if !ok {
		api.WriteError(w, api.ErrUnauthorized)
		return
	}
	var req struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		api.WriteError(w, api.ErrValidation)
		return
	}
	cal := &models.Calendar{DomainID: did, UserID: uid, Name: req.Name, Color: req.Color, Description: req.Description}
	if err := h.store.CreateCalendar(r.Context(), cal); err != nil {
		api.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCalendarDTO(cal))
}

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	did, uid, ok := h.ctx(r)
	if !ok {
		api.WriteError(w, api.ErrUnauthorized)
		return
	}
	cal, err := h.defaultCalendar(r, did, uid)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	var from, to time.Time
	if s := r.URL.Query().Get("start"); s != "" {
		from, _ = time.Parse(time.RFC3339, s)
	}
	if s := r.URL.Query().Get("end"); s != "" {
		to, _ = time.Parse(time.RFC3339, s)
	}
	events, err := h.store.ListEvents(r.Context(), cal.ID, from, to)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": toEventDTOs(events)})
}

func (h *Handler) createEvent(w http.ResponseWriter, r *http.Request) {
	did, uid, ok := h.ctx(r)
	if !ok {
		api.WriteError(w, api.ErrUnauthorized)
		return
	}
	cal, err := h.defaultCalendar(r, did, uid)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	ev, err := decodeEvent(r)
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	ev.CalendarID = cal.ID
	ev.DomainID = did
	ev.UserID = uid
	if ev.UID == "" {
		ev.UID = uuid.Must(uuid.NewV7()).String()
	}
	ev.ETag = computeETag(ev)
	if err := h.store.PutEvent(r.Context(), ev); err != nil {
		api.WriteError(w, err)
		return
	}
	_ = h.store.BumpCTag(r.Context(), cal.ID)
	writeJSON(w, http.StatusCreated, toEventDTO(ev))
}

func (h *Handler) updateEvent(w http.ResponseWriter, r *http.Request) {
	did, uid, ok := h.ctx(r)
	if !ok {
		api.WriteError(w, api.ErrUnauthorized)
		return
	}
	cal, err := h.defaultCalendar(r, did, uid)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	uidParam := chi.URLParam(r, "id")
	existing, err := h.store.GetEvent(r.Context(), cal.ID, uidParam)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			api.WriteError(w, api.ErrNotFound)
		} else {
			api.WriteError(w, err)
		}
		return
	}
	ev, err := decodeEvent(r)
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	ev.ID = existing.ID
	ev.CalendarID = cal.ID
	ev.DomainID = did
	ev.UserID = uid
	ev.UID = uidParam
	ev.Sequence = existing.Sequence + 1
	ev.ETag = computeETag(ev)
	if err := h.store.PutEvent(r.Context(), ev); err != nil {
		api.WriteError(w, err)
		return
	}
	_ = h.store.BumpCTag(r.Context(), cal.ID)
	writeJSON(w, http.StatusOK, toEventDTO(ev))
}

func (h *Handler) deleteEvent(w http.ResponseWriter, r *http.Request) {
	did, uid, ok := h.ctx(r)
	if !ok {
		api.WriteError(w, api.ErrUnauthorized)
		return
	}
	cal, err := h.defaultCalendar(r, did, uid)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	if err := h.store.DeleteEvent(r.Context(), cal.ID, chi.URLParam(r, "id")); err != nil {
		if errors.Is(err, ErrNotFound) {
			api.WriteError(w, api.ErrNotFound)
		} else {
			api.WriteError(w, err)
		}
		return
	}
	_ = h.store.BumpCTag(r.Context(), cal.ID)
	w.WriteHeader(http.StatusNoContent)
}

func decodeEvent(r *http.Request) (*models.CalendarEvent, error) {
	// Accept the frontend contract (title/start/end) with backend aliases
	// (summary/starts_at/ends_at) as fallback.
	var req struct {
		UID         string     `json:"uid"`
		Title       string     `json:"title"`
		Summary     string     `json:"summary"`
		Description string     `json:"description"`
		Location    string     `json:"location"`
		Start       *time.Time `json:"start"`
		End         *time.Time `json:"end"`
		StartsAt    *time.Time `json:"starts_at"`
		EndsAt      *time.Time `json:"ends_at"`
		AllDay      bool       `json:"all_day"`
		RRule       string     `json:"rrule"`
		Status      string     `json:"status"`
		Attendees   []string   `json:"attendees"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	if req.Attendees == nil {
		req.Attendees = []string{}
	}
	summary := req.Title
	if summary == "" {
		summary = req.Summary
	}
	start := req.Start
	if start == nil {
		start = req.StartsAt
	}
	end := req.End
	if end == nil {
		end = req.EndsAt
	}
	if summary == "" || start == nil || end == nil {
		return nil, fmt.Errorf("title, start, and end are required")
	}
	return &models.CalendarEvent{
		UID:         req.UID,
		Summary:     summary,
		Description: req.Description,
		Location:    req.Location,
		StartsAt:    *start,
		EndsAt:      *end,
		AllDay:      req.AllDay,
		RRule:       req.RRule,
		Status:      req.Status,
		Attendees:   req.Attendees,
	}, nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

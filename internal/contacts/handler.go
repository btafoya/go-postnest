package contacts

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/go-chi/chi/v5"
	"github.com/go-postnest/postnest/internal/api"
	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/models"
)

// Handler provides the contacts REST API.
type Handler struct {
	store Store
	auth  *auth.Service
}

// NewHandler creates a contacts handler.
func NewHandler(store Store, authSvc *auth.Service) *Handler {
	return &Handler{store: store, auth: authSvc}
}

// RegisterRoutes mounts routes on a chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/contacts", h.listContacts)
	r.Post("/api/v1/contacts", h.createContact)
	r.Patch("/api/v1/contacts/{id}", h.updateContact)
	r.Delete("/api/v1/contacts/{id}", h.deleteContact)
}

func (h *Handler) currentUser(r *http.Request) *models.User {
	return api.UserFromContext(r.Context())
}

func (h *Handler) resolveDomain(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	u := h.currentUser(r)
	if u == nil {
		api.WriteError(w, api.ErrUnauthorized)
		return uuid.Nil, false
	}
	doms, err := h.auth.GetUserDomains(r.Context(), u.ID)
	if err != nil || len(doms) == 0 {
		api.WriteError(w, &api.AppError{
			Code:       "no_domain",
			Message:    "User is not a member of any domain",
			StatusCode: http.StatusForbidden,
		})
		return uuid.Nil, false
	}
	return doms[0].DomainID, true
}

func (h *Handler) listContacts(w http.ResponseWriter, r *http.Request) {
	u := h.currentUser(r)
	did, ok := h.resolveDomain(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	contacts, total, err := h.store.List(r.Context(), did, u.ID, limit, offset)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"contacts": contacts, "total": total})
}

func (h *Handler) createContact(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email        string `json:"email"`
		Name         string `json:"name"`
		GivenName    string `json:"given_name"`
		FamilyName   string `json:"family_name"`
		Organization string `json:"organization"`
		Phone        string `json:"phone"`
		Notes        string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	if req.Email == "" {
		api.WriteError(w, api.NewValidationError([]api.FieldError{{Field: "email", Issue: "required"}}))
		return
	}
	u := h.currentUser(r)
	did, ok := h.resolveDomain(w, r)
	if !ok {
		return
	}
	c := &models.Contact{
		DomainID:     did,
		UserID:       u.ID,
		Email:        req.Email,
		Name:         req.Name,
		GivenName:    req.GivenName,
		FamilyName:   req.FamilyName,
		Organization: req.Organization,
		Phone:        req.Phone,
	}
	if req.Notes != "" {
		c.VCardData = req.Notes
	}
	if err := h.store.Create(r.Context(), c); err != nil {
		api.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handler) updateContact(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	var req struct {
		Email        string `json:"email"`
		Name         string `json:"name"`
		GivenName    string `json:"given_name"`
		FamilyName   string `json:"family_name"`
		Organization string `json:"organization"`
		Phone        string `json:"phone"`
		Notes        string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	did, ok := h.resolveDomain(w, r)
	if !ok {
		return
	}
	c, err := h.store.GetByID(r.Context(), did, u.ID, id)
	if err != nil {
		api.WriteError(w, api.ErrNotFound)
		return
	}
	c.Email = req.Email
	c.Name = req.Name
	c.GivenName = req.GivenName
	c.FamilyName = req.FamilyName
	c.Organization = req.Organization
	c.Phone = req.Phone
	if req.Notes != "" {
		c.VCardData = req.Notes
	}
	if err := h.store.Create(r.Context(), c); err != nil {
		api.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) deleteContact(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	did, ok := h.resolveDomain(w, r)
	if !ok {
		return
	}
	if err := h.store.Delete(r.Context(), did, u.ID, id); err != nil {
		api.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

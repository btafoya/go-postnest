package admin

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/go-postnest/postnest/internal/api"
	"github.com/go-postnest/postnest/internal/auth"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/argon2"
)

// PasswordHasher creates password hashes.
type PasswordHasher interface {
	hashPassword(password string) (string, error)
}

type hasher struct {
	time    uint32
	memory  uint32
	threads uint8
}

func newHasher(time, memory uint32, threads uint8) PasswordHasher {
	return &hasher{time: time, memory: memory, threads: threads}
}

func (h *hasher) hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, h.time, h.memory, h.threads, 32)
	encoded := base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash)
	return encoded, nil
}

// Handler implements admin REST API.
type Handler struct {
	store  Store
	auth   *auth.Service
	hasher PasswordHasher
}

// NewHandler creates an admin handler.
func NewHandler(store Store, authSvc *auth.Service, argonTime, argonMemory uint32, argonThreads uint8) *Handler {
	return &Handler{
		store:  store,
		auth:   authSvc,
		hasher: newHasher(argonTime, argonMemory, argonThreads),
	}
}

// RegisterRoutes wires admin routes.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/admin/api/v1/domains", h.listDomains)
	r.Post("/admin/api/v1/domains", h.createDomain)
	r.Put("/admin/api/v1/domains/{id}", h.updateDomain)
	r.Delete("/admin/api/v1/domains/{id}", h.deleteDomain)
	r.Patch("/admin/api/v1/domains/{id}/active", h.toggleDomainActive)

	r.Get("/admin/api/v1/users", h.listUsers)
	r.Post("/admin/api/v1/users", h.createUser)
	r.Put("/admin/api/v1/users/{id}", h.updateUser)
	r.Delete("/admin/api/v1/users/{id}", h.deleteUser)
	r.Post("/admin/api/v1/users/{id}/reset-password", h.resetPassword)

	r.Get("/admin/api/v1/settings", h.getSettings)
	r.Put("/admin/api/v1/settings", h.updateSettings)
}

func (h *Handler) listDomains(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	doms, err := h.store.ListDomains(ctx)
	if err != nil {
		api.WriteError(w, mapStoreError(err, "Domain"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"domains": toDomainDTOs(doms)})
}

type createDomainReq struct {
	Name           string `json:"name"`
	PostmarkToken  string `json:"postmark_token"`
	PostmarkStream string `json:"postmark_stream"`
}

func (h *Handler) createDomain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createDomainReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	if req.Name == "" {
		api.WriteError(w, &api.AppError{Code: "validation_failed", Message: "name is required", StatusCode: http.StatusBadRequest})
		return
	}
	d, err := h.store.CreateDomain(ctx, req.Name, req.PostmarkToken, req.PostmarkStream)
	if err != nil {
		api.WriteError(w, mapStoreError(err, "Domain"))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"domain": toDomainDTOFromModel(d)})
}

type updateDomainReq struct {
	Name           string `json:"name"`
	PostmarkToken  string `json:"postmark_token"`
	PostmarkStream string `json:"postmark_stream"`
	IsActive       bool   `json:"is_active"`
}

func (h *Handler) updateDomain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	var req updateDomainReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	if req.Name == "" {
		api.WriteError(w, &api.AppError{Code: "validation_failed", Message: "name is required", StatusCode: http.StatusBadRequest})
		return
	}
	if err := h.store.UpdateDomain(ctx, id, req.Name, req.PostmarkToken, req.PostmarkStream, req.IsActive); err != nil {
		api.WriteError(w, mapStoreError(err, "Domain"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"domain": domainDTO{ID: id, Name: req.Name, PostmarkToken: req.PostmarkToken, PostmarkStream: req.PostmarkStream, IsActive: req.IsActive}})
}

func (h *Handler) deleteDomain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	if err := h.store.DeleteDomain(ctx, id); err != nil {
		api.WriteError(w, mapStoreError(err, "Domain"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (h *Handler) toggleDomainActive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	var body struct{ IsActive bool `json:"is_active"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	if err := h.store.ToggleDomainActive(ctx, id, body.IsActive); err != nil {
		api.WriteError(w, mapStoreError(err, "Domain"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"is_active": body.IsActive})
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	users, err := h.store.ListUsers(ctx, limit, offset)
	if err != nil {
		api.WriteError(w, mapStoreError(err, "User"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": toUserDTOs(users)})
}

type createUserReq struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	DisplayName  string `json:"display_name"`
	IsSuperAdmin bool   `json:"is_super_admin"`
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createUserReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	if req.Email == "" {
		api.WriteError(w, &api.AppError{Code: "validation_failed", Message: "email is required", StatusCode: http.StatusBadRequest})
		return
	}
	if req.Password == "" {
		api.WriteError(w, &api.AppError{Code: "validation_failed", Message: "password is required", StatusCode: http.StatusBadRequest})
		return
	}
	if !strings.Contains(req.Email, "@") {
		api.WriteError(w, &api.AppError{Code: "validation_failed", Message: "email is invalid", StatusCode: http.StatusBadRequest})
		return
	}
	hash, err := h.hasher.hashPassword(req.Password)
	if err != nil {
		api.WriteError(w, api.ErrInternal)
		return
	}
	u, err := h.store.CreateUser(ctx, req.Email, hash, req.DisplayName, req.IsSuperAdmin)
	if err != nil {
		api.WriteError(w, mapStoreError(err, "User"))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": toUserDTO(u, nil)})
}

type updateUserReq struct {
	Email        string `json:"email"`
	DisplayName  string `json:"display_name"`
	IsSuperAdmin bool   `json:"is_super_admin"`
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	var req updateUserReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	if req.Email == "" {
		api.WriteError(w, &api.AppError{Code: "validation_failed", Message: "email is required", StatusCode: http.StatusBadRequest})
		return
	}
	if err := h.store.UpdateUser(ctx, id, req.Email, req.DisplayName, req.IsSuperAdmin); err != nil {
		api.WriteError(w, mapStoreError(err, "User"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": userDTO{ID: id, Email: req.Email, DisplayName: req.DisplayName, IsSuperAdmin: req.IsSuperAdmin, Memberships: []membershipDTO{}}})
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	if err := h.store.DeleteUser(ctx, id); err != nil {
		api.WriteError(w, mapStoreError(err, "User"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	var body struct{ Password string `json:"password"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	if body.Password == "" {
		api.WriteError(w, &api.AppError{Code: "validation_failed", Message: "password is required", StatusCode: http.StatusBadRequest})
		return
	}
	hash, err := h.hasher.hashPassword(body.Password)
	if err != nil {
		api.WriteError(w, api.ErrInternal)
		return
	}
	if err := h.store.ResetPassword(ctx, id, hash); err != nil {
		api.WriteError(w, mapStoreError(err, "User"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reset": true})
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	settings, err := h.store.GetSettings(ctx)
	if err != nil {
		api.WriteError(w, mapStoreError(err, "Settings"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	for k, v := range body {
		if err := h.store.SetSetting(ctx, k, v); err != nil {
			api.WriteError(w, mapStoreError(err, "Settings"))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": true})
}

func mapStoreError(err error, resource string) *api.AppError {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgerrcode.UniqueViolation:
			return &api.AppError{
				Code:       "conflict",
				Message:    resource + " already exists",
				StatusCode: http.StatusConflict,
			}
		}
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
		return &api.AppError{Code: "not_found", Message: "Not found", StatusCode: http.StatusNotFound}
	}
	return api.ErrInternal
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

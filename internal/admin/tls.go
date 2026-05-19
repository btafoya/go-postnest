package admin

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/go-postnest/postnest/internal/api"
	"github.com/go-postnest/postnest/internal/certmanager"
	"github.com/go-postnest/postnest/internal/crypto"
)

// TLSManager is the subset of *certmanager.Manager the admin API needs.
// An interface keeps the handler testable without live ACME.
type TLSManager interface {
	Status() certmanager.Status
	Reload(cfg certmanager.Config) error
	ForceRenew() error
}

// WithTLS attaches the certificate manager and credential cipher so the TLS
// admin endpoints become functional. When cipher is nil the endpoints return
// 503 (POSTNEST_SECRET_KEY not configured).
func (h *Handler) WithTLS(mgr TLSManager, cipher *crypto.Cipher, reload func() error) *Handler {
	h.certMgr = mgr
	h.cipher = cipher
	h.tlsReload = reload
	return h
}

func (h *Handler) registerTLSRoutes(r chi.Router) {
	r.Get("/admin/api/v1/tls/status", h.tlsStatus)
	r.Get("/admin/api/v1/tls/providers", h.tlsProviders)
	r.Get("/admin/api/v1/tls/config", h.getTLSConfig)
	r.Put("/admin/api/v1/tls/config", h.putTLSConfig)
	r.Get("/admin/api/v1/tls/domains", h.listTLSDomains)
	r.Post("/admin/api/v1/tls/domains", h.addTLSDomain)
	r.Delete("/admin/api/v1/tls/domains/{id}", h.deleteTLSDomain)
	r.Post("/admin/api/v1/tls/renew", h.renewTLS)
}

var errTLSUnavailable = &api.AppError{
	Code:       "tls_unavailable",
	Message:    "ACME/TLS management requires POSTNEST_SECRET_KEY to be configured",
	StatusCode: http.StatusServiceUnavailable,
}

func (h *Handler) tlsReady() bool { return h.cipher != nil }

func (h *Handler) tlsStatus(w http.ResponseWriter, r *http.Request) {
	if !h.tlsReady() {
		api.WriteError(w, errTLSUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": h.certMgr.Status()})
}

func (h *Handler) tlsProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"providers": certmanager.ProviderSpecs()})
}

// tlsConfigDTO is the read view. Secret credential fields are never returned;
// instead each key reports whether a value is currently stored.
type tlsConfigDTO struct {
	Enabled     bool            `json:"enabled"`
	Email       string          `json:"email"`
	Directory   string          `json:"directory"`
	DNSProvider string          `json:"dns_provider"`
	CredsSet    map[string]bool `json:"creds_set"`
}

func (h *Handler) getTLSConfig(w http.ResponseWriter, r *http.Request) {
	if !h.tlsReady() {
		api.WriteError(w, errTLSUnavailable)
		return
	}
	cfg, err := h.store.GetACMEConfig(r.Context())
	if err != nil {
		api.WriteError(w, mapStoreError(err, "ACME config"))
		return
	}
	creds, _ := h.decryptCreds(cfg.CredentialsEnc)
	set := make(map[string]bool)
	for k, v := range creds {
		set[k] = v != ""
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": tlsConfigDTO{
		Enabled:     cfg.Enabled,
		Email:       cfg.Email,
		Directory:   cfg.Directory,
		DNSProvider: cfg.DNSProvider,
		CredsSet:    set,
	}})
}

type putTLSConfigReq struct {
	Enabled     bool              `json:"enabled"`
	Email       string            `json:"email"`
	Directory   string            `json:"directory"`
	DNSProvider string            `json:"dns_provider"`
	Credentials map[string]string `json:"credentials"`
}

func (h *Handler) putTLSConfig(w http.ResponseWriter, r *http.Request) {
	if !h.tlsReady() {
		api.WriteError(w, errTLSUnavailable)
		return
	}
	var req putTLSConfigReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	if req.Enabled {
		if req.Email == "" || !certmanager.SupportedProvider(req.DNSProvider) {
			api.WriteError(w, api.ErrValidation)
			return
		}
		if req.Directory != "staging" && req.Directory != "production" {
			api.WriteError(w, api.ErrValidation)
			return
		}
	}

	ctx := r.Context()
	existing, err := h.store.GetACMEConfig(ctx)
	if err != nil {
		api.WriteError(w, mapStoreError(err, "ACME config"))
		return
	}
	current, _ := h.decryptCreds(existing.CredentialsEnc)
	if current == nil {
		current = map[string]string{}
	}
	// Empty submitted value preserves the stored secret (write-only fields).
	for k, v := range req.Credentials {
		if v != "" {
			current[k] = v
		}
	}

	enc, err := h.encryptCreds(current)
	if err != nil {
		api.WriteError(w, api.ErrInternal)
		return
	}
	if err := h.store.SetACMEConfig(ctx, req.Enabled, req.Email, req.Directory, req.DNSProvider, enc); err != nil {
		api.WriteError(w, mapStoreError(err, "ACME config"))
		return
	}
	if err := h.applyTLSReload(); err != nil {
		api.WriteError(w, &api.AppError{
			Code: "tls_reload_failed", Message: err.Error(), StatusCode: http.StatusBadGateway,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": true})
}

func (h *Handler) listTLSDomains(w http.ResponseWriter, r *http.Request) {
	if !h.tlsReady() {
		api.WriteError(w, errTLSUnavailable)
		return
	}
	domains, err := h.store.ListACMEDomains(r.Context())
	if err != nil {
		api.WriteError(w, mapStoreError(err, "ACME domains"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"domains": domains})
}

func (h *Handler) addTLSDomain(w http.ResponseWriter, r *http.Request) {
	if !h.tlsReady() {
		api.WriteError(w, errTLSUnavailable)
		return
	}
	var body struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Domain == "" {
		api.WriteError(w, api.ErrValidation)
		return
	}
	d, err := h.store.AddACMEDomain(r.Context(), body.Domain)
	if err != nil {
		api.WriteError(w, mapStoreError(err, "Domain"))
		return
	}
	if err := h.applyTLSReload(); err != nil {
		api.WriteError(w, &api.AppError{
			Code: "tls_reload_failed", Message: err.Error(), StatusCode: http.StatusBadGateway,
		})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"domain": d})
}

func (h *Handler) deleteTLSDomain(w http.ResponseWriter, r *http.Request) {
	if !h.tlsReady() {
		api.WriteError(w, errTLSUnavailable)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	if err := h.store.DeleteACMEDomain(r.Context(), id); err != nil {
		api.WriteError(w, mapStoreError(err, "Domain"))
		return
	}
	if err := h.applyTLSReload(); err != nil {
		api.WriteError(w, &api.AppError{
			Code: "tls_reload_failed", Message: err.Error(), StatusCode: http.StatusBadGateway,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (h *Handler) renewTLS(w http.ResponseWriter, r *http.Request) {
	if !h.tlsReady() {
		api.WriteError(w, errTLSUnavailable)
		return
	}
	if err := h.certMgr.ForceRenew(); err != nil {
		api.WriteError(w, &api.AppError{
			Code: "renew_failed", Message: err.Error(), StatusCode: http.StatusBadGateway,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"renewed": true})
}

// applyTLSReload re-reads DB config and reloads the manager. The reload
// closure is supplied by main.go where DB access + cipher live.
func (h *Handler) applyTLSReload() error {
	if h.tlsReload == nil {
		return nil
	}
	return h.tlsReload()
}

func (h *Handler) encryptCreds(creds map[string]string) (string, error) {
	b, err := json.Marshal(creds)
	if err != nil {
		return "", err
	}
	return h.cipher.Encrypt(string(b))
}

func (h *Handler) decryptCreds(enc string) (map[string]string, error) {
	if enc == "" {
		return map[string]string{}, nil
	}
	pt, err := h.cipher.Decrypt(enc)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	if err := json.Unmarshal([]byte(pt), &m); err != nil {
		return nil, err
	}
	return m, nil
}

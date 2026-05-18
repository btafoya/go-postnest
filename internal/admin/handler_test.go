package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-postnest/postnest/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

// mockStore is an in-memory store for testing.
type mockStore struct {
	domains       []*DomainRow
	users         []*UserRow
	settings      map[string]string
	listDomainsErr error
	createDomainErr error
	updateDomainErr error
	deleteDomainErr error
	toggleDomainErr error
	listUsersErr    error
	createUserErr   error
	updateUserErr   error
	deleteUserErr   error
	resetPasswordErr error
	getSettingsErr  error
	setSettingErr   error
}

func (m *mockStore) ListDomains(ctx context.Context) ([]*DomainRow, error) {
	if m.listDomainsErr != nil {
		return nil, m.listDomainsErr
	}
	return m.domains, nil
}

func (m *mockStore) CreateDomain(ctx context.Context, name, token, stream string) (*models.Domain, error) {
	if m.createDomainErr != nil {
		return nil, m.createDomainErr
	}
	id := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	return &models.Domain{ID: id, Name: name, PostmarkToken: token, PostmarkStream: stream, CreatedAt: now, UpdatedAt: now}, nil
}

func (m *mockStore) UpdateDomain(ctx context.Context, id uuid.UUID, name, token, stream string, isActive bool) error {
	if m.updateDomainErr != nil {
		return m.updateDomainErr
	}
	return nil
}

func (m *mockStore) DeleteDomain(ctx context.Context, id uuid.UUID) error {
	if m.deleteDomainErr != nil {
		return m.deleteDomainErr
	}
	return nil
}

func (m *mockStore) ToggleDomainActive(ctx context.Context, id uuid.UUID, isActive bool) error {
	if m.toggleDomainErr != nil {
		return m.toggleDomainErr
	}
	return nil
}

func (m *mockStore) ListUsers(ctx context.Context, limit, offset int) ([]*UserRow, error) {
	if m.listUsersErr != nil {
		return nil, m.listUsersErr
	}
	return m.users, nil
}

func (m *mockStore) CreateUser(ctx context.Context, email, passwordHash, displayName string, isSuperAdmin bool) (*models.User, error) {
	if m.createUserErr != nil {
		return nil, m.createUserErr
	}
	id := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	return &models.User{ID: id, Email: email, PasswordHash: passwordHash, DisplayName: displayName, IsSuperAdmin: isSuperAdmin, CreatedAt: now, UpdatedAt: now}, nil
}

func (m *mockStore) UpdateUser(ctx context.Context, id uuid.UUID, email, displayName string, isSuperAdmin bool) error {
	if m.updateUserErr != nil {
		return m.updateUserErr
	}
	return nil
}

func (m *mockStore) DeleteUser(ctx context.Context, id uuid.UUID) error {
	if m.deleteUserErr != nil {
		return m.deleteUserErr
	}
	return nil
}

func (m *mockStore) ResetPassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	if m.resetPasswordErr != nil {
		return m.resetPasswordErr
	}
	return nil
}

func (m *mockStore) GetUserDomainMemberships(ctx context.Context, userID uuid.UUID) ([]*models.DomainMember, error) {
	return nil, nil
}

func (m *mockStore) GetSettings(ctx context.Context) (map[string]string, error) {
	if m.getSettingsErr != nil {
		return nil, m.getSettingsErr
	}
	return m.settings, nil
}

func (m *mockStore) SetSetting(ctx context.Context, key, value string) error {
	if m.setSettingErr != nil {
		return m.setSettingErr
	}
	if m.settings == nil {
		m.settings = make(map[string]string)
	}
	m.settings[key] = value
	return nil
}

func newTestHandler(store Store) *Handler {
	return NewHandler(store, nil, 1, 64*1024, 4)
}

func TestListDomains_ReturnsDTOs(t *testing.T) {
	store := &mockStore{
		domains: []*DomainRow{
			{
				Domain:    models.Domain{ID: uuid.Must(uuid.NewV7()), Name: "example.com"},
				IsActive:  true,
				UserCount: 3,
			},
		},
	}
	h := newTestHandler(store)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/domains", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	domains, ok := resp["domains"].([]any)
	if !ok || len(domains) == 0 {
		t.Fatal("expected domains array")
	}
	first := domains[0].(map[string]any)
	if _, ok := first["id"]; !ok {
		t.Error("expected id key")
	}
	if _, ok := first["name"]; !ok {
		t.Error("expected name key")
	}
	if _, ok := first["is_active"]; !ok {
		t.Error("expected is_active key")
	}
	if _, ok := first["user_count"]; !ok {
		t.Error("expected user_count key")
	}
}

func TestListUsers_NoPasswordHash(t *testing.T) {
	store := &mockStore{
		users: []*UserRow{
			{
				User: models.User{ID: uuid.Must(uuid.NewV7()), Email: "alice@example.com", PasswordHash: "secret", DisplayName: "Alice"},
			},
		},
	}
	h := newTestHandler(store)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/users", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	users, ok := resp["users"].([]any)
	if !ok || len(users) == 0 {
		t.Fatal("expected users array")
	}
	first := users[0].(map[string]any)
	if _, ok := first["password_hash"]; ok {
		t.Error("expected no password_hash key")
	}
	if _, ok := first["memberships"]; !ok {
		t.Error("expected memberships key")
	}
}

func TestCreateDomain_ResponseShape(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]string{"name": "test.com"})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/domains", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusCreated)
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	domain, ok := resp["domain"].(map[string]any)
	if !ok {
		t.Fatal("expected domain key")
	}
	if _, ok := domain["id"]; !ok {
		t.Error("expected domain.id")
	}
	if domain["name"] != "test.com" {
		t.Errorf("name = %v, want test.com", domain["name"])
	}
}

func TestCreateUser_ResponseShape(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]any{"email": "a@b.com", "password": "secret", "display_name": "Alice"})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/users", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusCreated)
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	user, ok := resp["user"].(map[string]any)
	if !ok {
		t.Fatal("expected user key")
	}
	memberships, ok := user["memberships"].([]any)
	if !ok {
		t.Fatal("expected memberships array")
	}
	if len(memberships) != 0 {
		t.Errorf("memberships = %d, want 0", len(memberships))
	}
}

func TestCreateDomain_Duplicate(t *testing.T) {
	store := &mockStore{
		createDomainErr: &pgconn.PgError{Code: pgerrcode.UniqueViolation},
	}
	h := newTestHandler(store)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]string{"name": "test.com"})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/domains", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusConflict)
	}
	var resp map[string]map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"]["message"] != "Domain already exists" {
		t.Errorf("message = %q, want Domain already exists", resp["error"]["message"])
	}
}

func TestUpdateDomain_NotFound(t *testing.T) {
	store := &mockStore{
		updateDomainErr: ErrNotFound,
	}
	h := newTestHandler(store)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	id := uuid.Must(uuid.NewV7()).String()
	body, _ := json.Marshal(map[string]any{"name": "test.com", "is_active": true})
	req := httptest.NewRequest(http.MethodPut, "/admin/api/v1/domains/"+id, bytes.NewReader(body))
	rr := httptest.NewRecorder()

	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
	var resp map[string]map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"]["message"] != "Not found" {
		t.Errorf("message = %q, want Not found", resp["error"]["message"])
	}
}

func TestCreateDomain_EmptyName(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]string{"name": ""})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/domains", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var resp map[string]map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"]["message"] != "name is required" {
		t.Errorf("message = %q, want name is required", resp["error"]["message"])
	}
}

func TestCreateUser_InvalidEmail(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]any{"email": "not-an-email", "password": "secret"})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/users", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var resp map[string]map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"]["message"] != "email is invalid" {
		t.Errorf("message = %q, want email is invalid", resp["error"]["message"])
	}
}

func TestListDomains_DBError(t *testing.T) {
	store := &mockStore{
		listDomainsErr: errors.New("db exploded"),
	}
	h := newTestHandler(store)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/domains", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	var resp map[string]map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"]["message"] != "Internal server error" {
		t.Errorf("message = %q, want Internal server error", resp["error"]["message"])
	}
}

func TestErrorResponse_MessageField(t *testing.T) {
	store := &mockStore{
		listDomainsErr: errors.New("db exploded"),
	}
	h := newTestHandler(store)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/domains", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var resp map[string]map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	msg, ok := resp["error"]["message"]
	if !ok || msg == "" {
		t.Error("expected non-empty error.message")
	}
}

func TestContentType_JSON(t *testing.T) {
	store := &mockStore{
		domains: []*DomainRow{
			{
				Domain:    models.Domain{ID: uuid.Must(uuid.NewV7()), Name: "example.com"},
				IsActive:  true,
				UserCount: 0,
			},
		},
	}
	h := newTestHandler(store)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/domains", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

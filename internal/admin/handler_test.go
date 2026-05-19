package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

// mockStore is an in-memory store for testing.
type mockStore struct {
	domains           []*DomainRow
	users             []*UserRow
	settings          map[string]string
	listDomainsErr    error
	createDomainErr   error
	updateDomainErr   error
	deleteDomainErr   error
	toggleDomainErr   error
	listUsersErr      error
	createUserErr     error
	updateUserErr     error
	deleteUserErr     error
	resetPasswordErr  error
	getSettingsErr    error
	setSettingErr     error
	lastCreateUserHash string
	memberships       []*models.DomainMember
	addMemberErr      error
	updateMemberErr   error
	removeMemberErr   error
	lastAddRole       string
}

func (m *mockStore) ListDomains(ctx context.Context, limit, offset int) ([]*DomainRow, int64, error) {
	if m.listDomainsErr != nil {
		return nil, 0, m.listDomainsErr
	}
	return m.domains, int64(len(m.domains)), nil
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

func (m *mockStore) ListUsers(ctx context.Context, limit, offset int) ([]*UserRow, int64, error) {
	if m.listUsersErr != nil {
		return nil, 0, m.listUsersErr
	}
	return m.users, int64(len(m.users)), nil
}

func (m *mockStore) CreateUser(ctx context.Context, email, passwordHash, displayName string, isSuperAdmin bool) (*models.User, error) {
	if m.createUserErr != nil {
		return nil, m.createUserErr
	}
	m.lastCreateUserHash = passwordHash
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
	return m.memberships, nil
}

func (m *mockStore) AddMember(ctx context.Context, userID, domainID uuid.UUID, role string) (*models.DomainMember, error) {
	if m.addMemberErr != nil {
		return nil, m.addMemberErr
	}
	m.lastAddRole = role
	return &models.DomainMember{DomainID: domainID, DomainName: "example.com", UserID: userID, Role: role, CreatedAt: time.Now().UTC()}, nil
}

func (m *mockStore) UpdateMemberRole(ctx context.Context, userID, domainID uuid.UUID, role string) error {
	return m.updateMemberErr
}

func (m *mockStore) RemoveMember(ctx context.Context, userID, domainID uuid.UUID) error {
	return m.removeMemberErr
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
	authSvc := auth.NewService(nil, 1, 64*1024, 4, "test-session-key")
	return NewHandler(store, authSvc)
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
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error object")
	}
	if errObj["code"] != "validation_failed" {
		t.Errorf("code = %q, want validation_failed", errObj["code"])
	}
	details, ok := errObj["details"].([]any)
	if !ok || len(details) == 0 {
		t.Fatal("expected non-empty details array")
	}
	first := details[0].(map[string]any)
	if first["field"] != "name" {
		t.Errorf("field = %q, want name", first["field"])
	}
	if first["issue"] != "required" {
		t.Errorf("issue = %q, want required", first["issue"])
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
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error object")
	}
	if errObj["code"] != "validation_failed" {
		t.Errorf("code = %q, want validation_failed", errObj["code"])
	}
	details, ok := errObj["details"].([]any)
	if !ok || len(details) == 0 {
		t.Fatal("expected non-empty details array")
	}
	found := false
	for _, d := range details {
		fieldErr := d.(map[string]any)
		if fieldErr["field"] == "email" && fieldErr["issue"] == "email" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected detail with field=email issue=email, got %v", details)
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

func TestCreateDomain_Validation(t *testing.T) {
	tests := []struct {
		name      string
		body      map[string]any
		wantField string
		wantIssue string
	}{
		{"empty name", map[string]any{"name": ""}, "name", "required"},
		{"invalid domain chars", map[string]any{"name": "bad..domain"}, "name", "domainname"},
		{"too long name", map[string]any{"name": strings.Repeat("a", 254)}, "name", "max"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockStore{}
			h := newTestHandler(store)
			r := chi.NewRouter()
			h.RegisterRoutes(r)
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/domains", bytes.NewReader(body))
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
			}
			var resp map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			errObj, ok := resp["error"].(map[string]any)
			if !ok {
				t.Fatal("expected error object")
			}
			if errObj["code"] != "validation_failed" {
				t.Errorf("code = %q, want validation_failed", errObj["code"])
			}
			details, ok := errObj["details"].([]any)
			if !ok || len(details) == 0 {
				t.Fatal("expected non-empty details array")
			}
			found := false
			for _, d := range details {
				fieldErr := d.(map[string]any)
				if fieldErr["field"] == tt.wantField && fieldErr["issue"] == tt.wantIssue {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected detail with field=%q issue=%q, got %v", tt.wantField, tt.wantIssue, details)
			}
		})
	}
}

func TestCreateUser_Validation(t *testing.T) {
	tests := []struct {
		name      string
		body      map[string]any
		wantField string
		wantIssue string
	}{
		{"invalid email", map[string]any{"email": "not-an-email", "password": "secret"}, "email", "email"},
		{"empty password", map[string]any{"email": "a@b.com", "password": ""}, "password", "required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockStore{}
			h := newTestHandler(store)
			r := chi.NewRouter()
			h.RegisterRoutes(r)
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/users", bytes.NewReader(body))
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
			}
			var resp map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			errObj, ok := resp["error"].(map[string]any)
			if !ok {
				t.Fatal("expected error object")
			}
			if errObj["code"] != "validation_failed" {
				t.Errorf("code = %q, want validation_failed", errObj["code"])
			}
			details, ok := errObj["details"].([]any)
			if !ok || len(details) == 0 {
				t.Fatal("expected non-empty details array")
			}
			found := false
			for _, d := range details {
				fieldErr := d.(map[string]any)
				if fieldErr["field"] == tt.wantField && fieldErr["issue"] == tt.wantIssue {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected detail with field=%q issue=%q, got %v", tt.wantField, tt.wantIssue, details)
			}
		})
	}
}

func TestCreateUser_HashDelegation(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]any{"email": "a@b.com", "password": "secret123", "display_name": "Alice"})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/users", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusCreated)
	}
	if store.lastCreateUserHash == "" {
		t.Fatal("expected password hash to be passed to store")
	}
	if store.lastCreateUserHash == "secret123" {
		t.Error("expected password hash to differ from raw password (delegation to auth.Service)")
	}
}

func TestCreateUser_StrongPassword(t *testing.T) {
	store := &mockStore{settings: map[string]string{"require_strong_passwords": "true"}}
	h := newTestHandler(store)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]any{"email": "a@b.com", "password": "short"})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/users", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error object")
	}
	if errObj["code"] != "validation_failed" {
		t.Errorf("code = %q, want validation_failed", errObj["code"])
	}
	details, ok := errObj["details"].([]any)
	if !ok || len(details) == 0 {
		t.Fatal("expected non-empty details array")
	}
	found := false
	for _, d := range details {
		fieldErr := d.(map[string]any)
		if fieldErr["field"] == "password" && fieldErr["issue"] == "gte" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected detail with field=password issue=gte, got %v", details)
	}
}

func TestCreateUser_WeakPasswordAllowed(t *testing.T) {
	store := &mockStore{settings: map[string]string{"require_strong_passwords": "false"}}
	h := newTestHandler(store)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]any{"email": "a@b.com", "password": "short"})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/users", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusCreated)
	}
}

func TestResetPassword_StrongPassword(t *testing.T) {
	store := &mockStore{settings: map[string]string{"require_strong_passwords": "true"}}
	h := newTestHandler(store)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	id := uuid.Must(uuid.NewV7()).String()
	body, _ := json.Marshal(map[string]any{"password": "short"})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/users/"+id+"/reset-password", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error object")
	}
	if errObj["code"] != "validation_failed" {
		t.Errorf("code = %q, want validation_failed", errObj["code"])
	}
	details, ok := errObj["details"].([]any)
	if !ok || len(details) == 0 {
		t.Fatal("expected non-empty details array")
	}
	found := false
	for _, d := range details {
		fieldErr := d.(map[string]any)
		if fieldErr["field"] == "password" && fieldErr["issue"] == "gte" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected detail with field=password issue=gte, got %v", details)
	}
}

func TestListDomains_PaginationMeta(t *testing.T) {
	store := &mockStore{
		domains: []*DomainRow{
			{Domain: models.Domain{ID: uuid.Must(uuid.NewV7()), Name: "example.com"}, IsActive: true, UserCount: 1},
			{Domain: models.Domain{ID: uuid.Must(uuid.NewV7()), Name: "test.com"}, IsActive: false, UserCount: 2},
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
	if resp["total"] != float64(2) {
		t.Errorf("total = %v, want 2", resp["total"])
	}
	if resp["limit"] != float64(20) {
		t.Errorf("limit = %v, want 20", resp["limit"])
	}
	if resp["offset"] != float64(0) {
		t.Errorf("offset = %v, want 0", resp["offset"])
	}
}

func TestListUsers_PaginationMeta(t *testing.T) {
	store := &mockStore{
		users: []*UserRow{
			{User: models.User{ID: uuid.Must(uuid.NewV7()), Email: "alice@example.com"}},
			{User: models.User{ID: uuid.Must(uuid.NewV7()), Email: "bob@example.com"}},
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
	if resp["total"] != float64(2) {
		t.Errorf("total = %v, want 2", resp["total"])
	}
	if resp["limit"] != float64(20) {
		t.Errorf("limit = %v, want 20", resp["limit"])
	}
	if resp["offset"] != float64(0) {
		t.Errorf("offset = %v, want 0", resp["offset"])
	}
}

func TestListUsers_Memberships(t *testing.T) {
	store := &mockStore{
		users: []*UserRow{
			{
				User: models.User{ID: uuid.Must(uuid.NewV7()), Email: "alice@example.com"},
				Memberships: []*models.DomainMember{
					{DomainID: uuid.Must(uuid.NewV7()), UserID: uuid.Must(uuid.NewV7()), Role: "admin"},
				},
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
	memberships, ok := first["memberships"].([]any)
	if !ok {
		t.Fatal("expected memberships array")
	}
	if len(memberships) != 1 {
		t.Errorf("memberships = %d, want 1", len(memberships))
	}
}

func TestListUsers_InvalidPagination(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		wantField string
		wantIssue string
	}{
		{"limit zero", "limit=0", "Limit", "gte"},
		{"limit too high", "limit=200", "Limit", "lte"},
		{"offset negative", "offset=-1", "Offset", "gte"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockStore{}
			h := newTestHandler(store)
			r := chi.NewRouter()
			h.RegisterRoutes(r)
			req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/users?"+tt.query, nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
			}
			var resp map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			errObj, ok := resp["error"].(map[string]any)
			if !ok {
				t.Fatal("expected error object")
			}
			details, ok := errObj["details"].([]any)
			if !ok || len(details) == 0 {
				t.Fatal("expected non-empty details array")
			}
			found := false
			for _, d := range details {
				fieldErr := d.(map[string]any)
				if fieldErr["field"] == tt.wantField && fieldErr["issue"] == tt.wantIssue {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected detail with field=%q issue=%q, got %v", tt.wantField, tt.wantIssue, details)
			}
		})
	}
}

func TestAddUserDomain_Success(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	userID := uuid.Must(uuid.NewV7())
	domainID := uuid.Must(uuid.NewV7())
	body, _ := json.Marshal(map[string]string{"domain_id": domainID.String(), "role": "admin"})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/users/"+userID.String()+"/domains", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (%s)", rr.Code, http.StatusCreated, rr.Body.String())
	}
	if store.lastAddRole != "admin" {
		t.Errorf("lastAddRole = %q, want admin", store.lastAddRole)
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	m, ok := resp["membership"].(map[string]any)
	if !ok {
		t.Fatal("expected membership key")
	}
	if m["role"] != "admin" {
		t.Errorf("role = %v, want admin", m["role"])
	}
	if m["domain_name"] != "example.com" {
		t.Errorf("domain_name = %v, want example.com", m["domain_name"])
	}
}

func TestAddUserDomain_InvalidRole(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	userID := uuid.Must(uuid.NewV7())
	domainID := uuid.Must(uuid.NewV7())
	body, _ := json.Marshal(map[string]string{"domain_id": domainID.String(), "role": "owner"})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/users/"+userID.String()+"/domains", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (%s)", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

func TestAddUserDomain_BadUserID(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]string{"domain_id": uuid.Must(uuid.NewV7()).String(), "role": "user"})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/users/not-a-uuid/domains", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestUpdateUserDomainRole_Success(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	userID := uuid.Must(uuid.NewV7())
	domainID := uuid.Must(uuid.NewV7())
	body, _ := json.Marshal(map[string]string{"role": "readonly"})
	req := httptest.NewRequest(http.MethodPut, "/admin/api/v1/users/"+userID.String()+"/domains/"+domainID.String(), bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestUpdateUserDomainRole_NotFound(t *testing.T) {
	store := &mockStore{updateMemberErr: ErrNotFound}
	h := newTestHandler(store)
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	userID := uuid.Must(uuid.NewV7())
	domainID := uuid.Must(uuid.NewV7())
	body, _ := json.Marshal(map[string]string{"role": "user"})
	req := httptest.NewRequest(http.MethodPut, "/admin/api/v1/users/"+userID.String()+"/domains/"+domainID.String(), bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestRemoveUserDomain_Success(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	userID := uuid.Must(uuid.NewV7())
	domainID := uuid.Must(uuid.NewV7())
	req := httptest.NewRequest(http.MethodDelete, "/admin/api/v1/users/"+userID.String()+"/domains/"+domainID.String(), nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestRemoveUserDomain_NotFound(t *testing.T) {
	store := &mockStore{removeMemberErr: ErrNotFound}
	h := newTestHandler(store)
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	userID := uuid.Must(uuid.NewV7())
	domainID := uuid.Must(uuid.NewV7())
	req := httptest.NewRequest(http.MethodDelete, "/admin/api/v1/users/"+userID.String()+"/domains/"+domainID.String(), nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestUpdateUser_ReturnsRealMemberships(t *testing.T) {
	domainID := uuid.Must(uuid.NewV7())
	store := &mockStore{
		memberships: []*models.DomainMember{
			{DomainID: domainID, DomainName: "example.com", Role: "user"},
		},
	}
	h := newTestHandler(store)
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	userID := uuid.Must(uuid.NewV7())
	body, _ := json.Marshal(map[string]any{"email": "u@example.com", "display_name": "U", "is_super_admin": false})
	req := httptest.NewRequest(http.MethodPut, "/admin/api/v1/users/"+userID.String(), bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rr.Code, http.StatusOK, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	u := resp["user"].(map[string]any)
	mems, ok := u["memberships"].([]any)
	if !ok || len(mems) != 1 {
		t.Fatalf("expected 1 membership, got %v", u["memberships"])
	}
	if mems[0].(map[string]any)["domain_name"] != "example.com" {
		t.Errorf("domain_name = %v, want example.com", mems[0])
	}
}

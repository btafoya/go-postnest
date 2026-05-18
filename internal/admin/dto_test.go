package admin

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/go-postnest/postnest/internal/models"
)

func TestDomainDTO_SnakeCase(t *testing.T) {
	d := domainDTO{
		ID:             uuid.Must(uuid.NewV7()),
		Name:           "example.com",
		PostmarkToken:  "tok",
		PostmarkStream: "outbound",
		IsActive:       true,
		UserCount:      3,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	required := []string{"id", "name", "postmark_token", "postmark_stream", "is_active", "user_count", "created_at", "updated_at"}
	for _, k := range required {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %q", k)
		}
	}

	pascal := []string{"ID", "Name", "IsActive", "UserCount", "PostmarkToken", "PostmarkStream", "CreatedAt", "UpdatedAt"}
	for _, k := range pascal {
		if _, ok := m[k]; ok {
			t.Errorf("unexpected PascalCase key %q", k)
		}
	}
}

func TestUserDTO_NoPasswordHash(t *testing.T) {
	u := userDTO{
		ID:          uuid.Must(uuid.NewV7()),
		Email:       "a@example.com",
		DisplayName: "Alice",
		Memberships: []membershipDTO{},
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	b, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if _, ok := m["password_hash"]; ok {
		t.Error("password_hash must not appear in JSON")
	}

	typ := reflect.TypeOf(userDTO{})
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).Name == "PasswordHash" {
			t.Error("userDTO must not have PasswordHash field")
		}
	}
}

func TestUserDTO_Memberships(t *testing.T) {
	did := uuid.Must(uuid.NewV7())
	u := userDTO{
		ID:          uuid.Must(uuid.NewV7()),
		Email:       "b@example.com",
		DisplayName: "Bob",
		Memberships: []membershipDTO{{DomainID: did, Role: "admin"}},
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	b, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	mems, ok := m["memberships"].([]any)
	if !ok {
		t.Fatalf("memberships is not an array, got %T", m["memberships"])
	}
	if len(mems) != 1 {
		t.Fatalf("expected 1 membership, got %d", len(mems))
	}

	first, ok := mems[0].(map[string]any)
	if !ok {
		t.Fatalf("first membership is not an object, got %T", mems[0])
	}
	if _, ok := first["domain_id"]; !ok {
		t.Error("missing domain_id in first membership")
	}
	if _, ok := first["role"]; !ok {
		t.Error("missing role in first membership")
	}
}

func TestToDomainDTOs(t *testing.T) {
	rows := []*DomainRow{
		{
			Domain:    models.Domain{ID: uuid.Must(uuid.NewV7()), Name: "a.com"},
			IsActive:  true,
			UserCount: 5,
		},
		{
			Domain:    models.Domain{ID: uuid.Must(uuid.NewV7()), Name: "b.com"},
			IsActive:  false,
			UserCount: 0,
		},
	}

	out := toDomainDTOs(rows)
	if len(out) != 2 {
		t.Fatalf("expected 2 DTOs, got %d", len(out))
	}
	if out[0].Name != "a.com" {
		t.Errorf("expected Name a.com, got %s", out[0].Name)
	}
	if out[0].UserCount != 5 {
		t.Errorf("expected UserCount 5, got %d", out[0].UserCount)
	}
}

func TestToUserDTOs(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	rows := []*UserRow{
		{
			User: models.User{
				ID:           uid,
				Email:        "u@example.com",
				DisplayName:  "User",
				PasswordHash: "secret",
				IsSuperAdmin: false,
				CreatedAt:    time.Now().UTC(),
				UpdatedAt:    time.Now().UTC(),
			},
			Memberships: []*models.DomainMember{
				{DomainID: uuid.Must(uuid.NewV7()), UserID: uid, Role: "admin"},
			},
		},
	}

	out := toUserDTOs(rows)
	if len(out) != 1 {
		t.Fatalf("expected 1 DTO, got %d", len(out))
	}
	// Verify PasswordHash is omitted from JSON output
	b, err := json.Marshal(out[0])
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if _, ok := m["password_hash"]; ok {
		t.Error("password_hash must not appear in JSON")
	}
	if len(out[0].Memberships) != 1 {
		t.Fatalf("expected 1 membership, got %d", len(out[0].Memberships))
	}
}

func TestDomainDTOFromModel(t *testing.T) {
	now := time.Now().UTC()
	m := &models.Domain{
		ID:             uuid.Must(uuid.NewV7()),
		Name:           "test.com",
		PostmarkToken:  "token",
		PostmarkStream: "stream",
		IsActive:       true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	d := toDomainDTOFromModel(m)
	if d.ID != m.ID {
		t.Error("ID mismatch")
	}
	if d.Name != m.Name {
		t.Error("Name mismatch")
	}
	if d.PostmarkToken != m.PostmarkToken {
		t.Error("PostmarkToken mismatch")
	}
	if d.PostmarkStream != m.PostmarkStream {
		t.Error("PostmarkStream mismatch")
	}
	if d.IsActive != m.IsActive {
		t.Error("IsActive mismatch")
	}
	if d.UserCount != 0 {
		t.Errorf("expected UserCount 0, got %d", d.UserCount)
	}
	if !d.CreatedAt.Equal(now) {
		t.Error("CreatedAt mismatch")
	}
	if !d.UpdatedAt.Equal(now) {
		t.Error("UpdatedAt mismatch")
	}
}

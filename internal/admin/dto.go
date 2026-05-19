package admin

import (
	"time"

	"github.com/google/uuid"
	"github.com/go-postnest/postnest/internal/models"
)

// domainDTO is the JSON contract for a domain.
type domainDTO struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	PostmarkToken  string    `json:"postmark_token"`
	PostmarkStream string    `json:"postmark_stream"`
	IsActive       bool      `json:"is_active"`
	UserCount      int64     `json:"user_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// membershipDTO is the JSON contract for a domain membership.
type membershipDTO struct {
	DomainID   uuid.UUID `json:"domain_id"`
	DomainName string    `json:"domain_name"`
	Role       string    `json:"role"`
}

// userDTO is the JSON contract for a user.
type userDTO struct {
	ID           uuid.UUID       `json:"id"`
	Email        string          `json:"email"`
	DisplayName  string          `json:"display_name"`
	IsSuperAdmin bool            `json:"is_super_admin"`
	Memberships  []membershipDTO `json:"memberships"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

func toDomainDTO(r *DomainRow) domainDTO {
	return domainDTO{
		ID:             r.ID,
		Name:           r.Name,
		PostmarkToken:  r.PostmarkToken,
		PostmarkStream: r.PostmarkStream,
		IsActive:       r.IsActive,
		UserCount:      r.UserCount,
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
	}
}

func toDomainDTOs(rows []*DomainRow) []domainDTO {
	out := make([]domainDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDomainDTO(r))
	}
	return out
}

func toDomainDTOFromModel(d *models.Domain) domainDTO {
	return domainDTO{
		ID:             d.ID,
		Name:           d.Name,
		PostmarkToken:  d.PostmarkToken,
		PostmarkStream: d.PostmarkStream,
		IsActive:       d.IsActive,
		UserCount:      0,
		CreatedAt:      d.CreatedAt,
		UpdatedAt:      d.UpdatedAt,
	}
}

func toMembershipDTO(m *models.DomainMember) membershipDTO {
	return membershipDTO{
		DomainID:   m.DomainID,
		DomainName: m.DomainName,
		Role:       m.Role,
	}
}

func toMembershipDTOs(mems []*models.DomainMember) []membershipDTO {
	if mems == nil {
		return []membershipDTO{}
	}
	out := make([]membershipDTO, 0, len(mems))
	for _, m := range mems {
		out = append(out, toMembershipDTO(m))
	}
	return out
}

func toUserDTO(u *models.User, mems []*models.DomainMember) userDTO {
	m := toMembershipDTOs(mems)
	return userDTO{
		ID:           u.ID,
		Email:        u.Email,
		DisplayName:  u.DisplayName,
		IsSuperAdmin: u.IsSuperAdmin,
		Memberships:  m,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}

func toUserDTOFromRow(r *UserRow) userDTO {
	return toUserDTO(&r.User, r.Memberships)
}

func toUserDTOs(rows []*UserRow) []userDTO {
	out := make([]userDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toUserDTOFromRow(r))
	}
	return out
}

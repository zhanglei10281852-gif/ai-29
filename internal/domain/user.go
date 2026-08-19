package domain

import (
	"strings"
	"time"
)

type Role string

const (
	RoleMLEngineer        Role = "ml_engineer"
	RoleDataEngineer      Role = "data_engineer"
	RoleRiskReviewer      Role = "risk_reviewer"
	RoleComplianceAuditor Role = "compliance_auditor"
)

type UserStatus string

const (
	UserActive   UserStatus = "active"
	UserDisabled UserStatus = "disabled"
)

type User struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	DisplayName  string     `json:"display_name"`
	PasswordHash string     `json:"-"`
	Role         Role       `json:"role"`
	Status       UserStatus `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	Version      int64      `json:"version"`
}

type Session struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
	RevokedAt *time.Time
}

func (u User) Validate() error {
	email := strings.TrimSpace(strings.ToLower(u.Email))
	if !strings.Contains(email, "@") || strings.HasPrefix(email, "@") || strings.HasSuffix(email, "@") {
		return FieldError{Field: "email", Message: "is invalid"}
	}
	if strings.TrimSpace(u.DisplayName) == "" {
		return FieldError{Field: "display_name", Message: "is required"}
	}
	if strings.TrimSpace(u.PasswordHash) == "" {
		return FieldError{Field: "password_hash", Message: "is required"}
	}
	switch u.Role {
	case RoleMLEngineer, RoleDataEngineer, RoleRiskReviewer, RoleComplianceAuditor:
	default:
		return FieldError{Field: "role", Message: "is invalid"}
	}
	if u.Status != UserActive && u.Status != UserDisabled {
		return FieldError{Field: "status", Message: "is invalid"}
	}
	return nil
}

type Principal struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        Role   `json:"role"`
	SessionID   string `json:"-"`
}

func (s Session) IsActive(now time.Time) bool {
	return s.RevokedAt == nil && s.ExpiresAt.After(now)
}

func (p Principal) Can(roles ...Role) bool {
	for _, role := range roles {
		if p.Role == role {
			return true
		}
	}
	return false
}

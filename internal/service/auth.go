package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/identity"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	dependencies
	sessionTTL time.Duration
}

type LoginInput struct {
	Email    string
	Password string
}

type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	Principal domain.Principal
}

func (s *AuthService) CreateUser(ctx context.Context, email, displayName, password string, role domain.Role) (domain.User, error) {
	principal, _ := principalOrEmpty(ctx)
	if err := requireRole(principal, domain.RoleMLEngineer); err != nil {
		return domain.User{}, err
	}
	if len(password) < 12 || len(password) > 128 {
		return domain.User{}, domain.FieldError{Field: "password", Message: "must be between 12 and 128 characters"}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}
	now := s.clock.Now()
	user := domain.User{ID: identity.New("usr"), Email: strings.TrimSpace(strings.ToLower(email)), DisplayName: strings.TrimSpace(displayName), PasswordHash: string(hash), Role: role, Status: domain.UserActive, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := user.Validate(); err != nil {
		return domain.User{}, err
	}
	err = s.store.WithTx(ctx, func(tx repository.Tx) error {
		if err := tx.InsertUser(ctx, user); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, "user_created", "user", user.ID, "success", map[string]string{"role": string(user.Role)})
	})
	if err != nil {
		return domain.User{}, err
	}
	user.PasswordHash = ""
	return user, nil
}

func (s *AuthService) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	var user domain.User
	err := s.store.Read(ctx, func(reader repository.Reader) error {
		var err error
		user, err = reader.GetUserByEmail(ctx, email)
		return err
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return LoginResult{}, fmt.Errorf("invalid credentials: %w", domain.ErrUnauthenticated)
		}
		return LoginResult{}, fmt.Errorf("authenticate user: %w", err)
	}
	if user.Status != domain.UserActive || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)) != nil {
		return LoginResult{}, fmt.Errorf("invalid credentials: %w", domain.ErrUnauthenticated)
	}
	token, tokenHash, err := newToken()
	if err != nil {
		return LoginResult{}, err
	}
	now := s.clock.Now()
	session := domain.Session{ID: identity.New("ses"), UserID: user.ID, TokenHash: tokenHash, ExpiresAt: now.Add(s.sessionTTL), CreatedAt: now}
	if err := s.store.WithTx(ctx, func(tx repository.Tx) error {
		if err := tx.InsertSession(ctx, session); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, "user_login", "user", user.ID, "success", nil)
	}); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: token, ExpiresAt: session.ExpiresAt, Principal: domain.Principal{UserID: user.ID, Email: user.Email, DisplayName: user.DisplayName, Role: user.Role, SessionID: session.ID}}, nil
}

func (s *AuthService) Authenticate(ctx context.Context, token string) (domain.Principal, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return domain.Principal{}, fmt.Errorf("token is required: %w", domain.ErrUnauthenticated)
	}
	sum := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(sum[:])
	var session domain.Session
	var user domain.User
	err := s.store.Read(ctx, func(reader repository.Reader) error {
		var err error
		session, err = reader.GetSessionByTokenHash(ctx, tokenHash)
		if err != nil {
			return err
		}
		user, err = reader.GetUser(ctx, session.UserID)
		return err
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Principal{}, fmt.Errorf("invalid session: %w", domain.ErrUnauthenticated)
		}
		return domain.Principal{}, fmt.Errorf("authenticate session: %w", err)
	}
	if session.RevokedAt != nil || !session.ExpiresAt.After(s.clock.Now()) || user.Status != domain.UserActive {
		return domain.Principal{}, fmt.Errorf("session is no longer active: %w", domain.ErrUnauthenticated)
	}
	return domain.Principal{UserID: user.ID, Email: user.Email, DisplayName: user.DisplayName, Role: user.Role, SessionID: session.ID}, nil
}

func (s *AuthService) Logout(ctx context.Context, principal domain.Principal) error {
	if principal.SessionID == "" {
		return errors.New("session is required")
	}
	return s.store.WithTx(ctx, func(tx repository.Tx) error {
		if err := tx.RevokeSession(ctx, principal.SessionID, s.clock.Now()); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, "user_logout", "user", principal.UserID, "success", nil)
	})
}

func newToken() (string, string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", fmt.Errorf("generate session token: %w", err)
	}
	token := hex.EncodeToString(raw[:])
	sum := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(sum[:]), nil
}

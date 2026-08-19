package httpapi

import (
	"net/http"

	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/requestmeta"
	"github.com/zhanglei10281852-gif/ai/internal/service"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var input loginRequest
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := s.services.Auth.Login(r.Context(), service.LoginInput{Email: input.Email, Password: input.Password})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": result.Token, "expires_at": result.ExpiresAt, "user": result.Principal})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	principal, _ := requestmeta.Principal(r.Context())
	if err := s.services.Auth.Logout(r.Context(), principal); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

type createUserRequest struct {
	Email       string      `json:"email"`
	DisplayName string      `json:"display_name"`
	Password    string      `json:"password"`
	Role        domain.Role `json:"role"`
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var input createUserRequest
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, r, err)
		return
	}
	user, err := s.services.Auth.CreateUser(r.Context(), input.Email, input.DisplayName, input.Password, input.Role)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

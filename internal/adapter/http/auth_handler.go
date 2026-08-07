package http

import (
	"net/http"

	"wherewhat/internal/domain/usecase/auth/authenticate"
	"wherewhat/internal/domain/usecase/auth/login"
	"wherewhat/internal/domain/usecase/auth/logout"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// Language — the frontend's interface language at login time (detected from the browser, or a
	// previously cached value). Only used to set the user's language if they don't have one yet.
	Language string `json:"language"`
}

// handleLogin handles POST /api/auth/login.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var input loginRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	out, err := s.Auth.Login.Execute(
		r.Context(),
		login.Input{
			Username: input.Username,
			Password: input.Password,
			Language: input.Language,
		},
	)
	if err != nil {
		writeUseCaseError(w, err)
		return
	}

	s.setSessionCookie(w, out.Session)

	writeJSON(w, http.StatusOK, out.User)
}

// handleLogout handles POST /api/auth/logout.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.Auth.Logout.Execute(
		r.Context(),
		logout.Input{
			Token: sessionToken(r),
		},
	)

	s.clearSessionCookie(w)

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleMe handles GET /api/auth/me — reports the current session's user, or 401 if there isn't
// one.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	out, err := s.Auth.Authenticate.Execute(
		r.Context(),
		authenticate.Input{
			Token: sessionToken(r),
		},
	)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	writeJSON(w, http.StatusOK, out.User)
}

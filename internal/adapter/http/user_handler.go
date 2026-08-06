package http

import (
	"net/http"

	"wherewhat/internal/domain/usecase/user/changepassword"
	"wherewhat/internal/domain/usecase/user/updatelanguage"
	"wherewhat/internal/domain/usecase/user/updatelocationfilterdepth"
	"wherewhat/internal/domain/usecase/user/updateusername"
)

type updateLanguageRequest struct {
	Language string `json:"language"`
}

// handleUpdateLanguage handles PATCH /api/user/language.
func (s *Server) handleUpdateLanguage(w http.ResponseWriter, r *http.Request) {
	current := userFromContext(r)
	if current == nil {
		writeError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var input updateLanguageRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	updated, err := s.Users.UpdateLanguage.Execute(r.Context(), updatelanguage.Input{
		User:     *current,
		Language: input.Language,
	})
	if err != nil {
		writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

type updateLocationFilterDepthRequest struct {
	LocationFilterDepth int `json:"locationFilterDepth"`
}

// handleUpdateLocationFilterDepth handles PATCH /api/user/location-filter-depth.
func (s *Server) handleUpdateLocationFilterDepth(w http.ResponseWriter, r *http.Request) {
	current := userFromContext(r)
	if current == nil {
		writeError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var input updateLocationFilterDepthRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	updated, err := s.Users.UpdateLocationFilterDepth.Execute(r.Context(), updatelocationfilterdepth.Input{
		User:  *current,
		Depth: input.LocationFilterDepth,
	})
	if err != nil {
		writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

type updateUsernameRequest struct {
	Username        string `json:"username"`
	CurrentPassword string `json:"currentPassword"`
}

// handleUpdateUsername handles PATCH /api/user/username.
func (s *Server) handleUpdateUsername(w http.ResponseWriter, r *http.Request) {
	current := userFromContext(r)
	if current == nil {
		writeError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var input updateUsernameRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	updated, err := s.Users.UpdateUsername.Execute(r.Context(), updateusername.Input{
		User:            *current,
		Username:        input.Username,
		CurrentPassword: input.CurrentPassword,
	})
	if err != nil {
		writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// handleChangePassword handles PATCH /api/user/password.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	current := userFromContext(r)
	if current == nil {
		writeError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var input changePasswordRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	err := s.Users.ChangePassword.Execute(r.Context(), changepassword.Input{
		User:            *current,
		CurrentPassword: input.CurrentPassword,
		NewPassword:     input.NewPassword,
	})
	if err != nil {
		writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

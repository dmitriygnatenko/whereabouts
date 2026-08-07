package login

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"

	"wherewhat/internal/domain/usecase"
)

// Input is what LoginUser needs to verify credentials and start a session.
type Input struct {
	Username string
	Password string
	// Language is the frontend's detected/cached interface language at login time — adopted as the
	// user's saved language only if they don't have one yet.
	Language string
}

// Validate rejects structurally invalid credentials before any repository lookup
func (i Input) Validate() error {
	return validation.ValidateStruct(&i,
		validation.Field(&i.Username, usecase.UsernameRules()...),
		validation.Field(&i.Password, usecase.PasswordRules()...),
	)
}

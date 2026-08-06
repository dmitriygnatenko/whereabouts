package login

import (
	"fmt"
	"wherewhat/internal/domain/entity"

	validation "github.com/go-ozzo/ozzo-validation/v4"
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
		validation.Field(&i.Username,
			validation.Required.
				Error("Please enter a username"),
			validation.Length(entity.MinUsernameLength, 0).
				Error(fmt.Sprintf("Username must be at least %d characters", entity.MinUsernameLength)),
		),
		validation.Field(&i.Password,
			validation.Required.
				Error("Please enter a password"),
			validation.Length(entity.MinPasswordLength, 0).
				Error(fmt.Sprintf("Password must be at least %d characters", entity.MinPasswordLength)),
		),
	)
}

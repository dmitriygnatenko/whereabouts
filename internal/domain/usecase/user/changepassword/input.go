package changepassword

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"

	"wherewhat/internal/domain/entity"
	"wherewhat/internal/domain/usecase"
)

// Input is what ChangePassword needs — the currently authenticated user, their current password to
// confirm the change, and the new one.
type Input struct {
	User            entity.PublicUser
	CurrentPassword string
	NewPassword     string
}

// Validate rejects a new password that fails the basic format rules before any repository lookup.
func (i Input) Validate() error {
	return validation.ValidateStruct(&i,
		validation.Field(&i.NewPassword, usecase.PasswordRules()...),
	)
}

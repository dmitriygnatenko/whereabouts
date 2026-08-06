package changepassword

import (
	"wherewhat/internal/domain/entity"
)

// Input is what ChangePassword needs — the currently authenticated user, their current password to
// confirm the change, and the new one.
type Input struct {
	User            entity.PublicUser
	CurrentPassword string
	NewPassword     string
}

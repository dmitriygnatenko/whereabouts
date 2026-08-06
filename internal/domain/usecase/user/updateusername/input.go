package updateusername

import (
	"wherewhat/internal/domain/entity"
)

// Input is what UpdateUsername needs — the currently authenticated user, the new username, and
// their current password to confirm the change.
type Input struct {
	User            entity.PublicUser
	Username        string
	CurrentPassword string
}

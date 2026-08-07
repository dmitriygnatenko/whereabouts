package updateusername

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"

	"wherewhat/internal/domain/entity"
	"wherewhat/internal/domain/usecase"
)

// Input is what UpdateUsername needs — the currently authenticated user, the new username, and
// their current password to confirm the change.
type Input struct {
	User            entity.PublicUser
	Username        string
	CurrentPassword string
}

// Validate rejects an invalid new username before any repository lookup.
func (i Input) Validate() error {
	i.Username = usecase.NormalizeUsername(i.Username)

	return validation.ValidateStruct(&i,
		validation.Field(&i.Username, usecase.UsernameRules()...),
	)
}

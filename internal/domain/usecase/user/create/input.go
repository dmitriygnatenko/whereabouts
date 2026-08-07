package create

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"

	"wherewhat/internal/domain/usecase"
)

// Input is what CreateUser needs to create a new account.
type Input struct {
	Username string
	Password string
}

// Validate rejects invalid credentials before any repository lookup.
func (i Input) Validate() error {
	i.Username = usecase.NormalizeUsername(i.Username)

	return validation.ValidateStruct(&i,
		validation.Field(&i.Username, usecase.UsernameRules()...),
		validation.Field(&i.Password, usecase.PasswordRules()...),
	)
}

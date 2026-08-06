package registeruser

import (
	"wherewhat/internal/domain/entity"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Input is what RegisterUser needs to create a new account.
type Input struct {
	Username string
	Password string
}

// Validate checks Username and Password against the same rules enforced at login — every stored
// account must satisfy them, so this only rejects structurally invalid input before an account is
// created.
//
// The struct is validated as a whole (validation.ValidateStruct), not field by field: ozzo-validation
// then reports every violated field in one pass rather than stopping at the first validation.Validate
// call. domainerror.ToValidationError turns that combined result into the error the transport
// renders, which is also what adds sentence punctuation — the messages here stay unpunctuated so the
// frontend can match them against its translation table (see SERVER_ERRORS in web/i18n.js).
func (in Input) Validate() error {
	return validation.ValidateStruct(&in,
		validation.Field(&in.Username,
			validation.Required.Error("Please enter a username"),
			validation.Length(entity.MinUsernameLength, 0).Error("Username must be at least 3 characters"),
		),
		validation.Field(&in.Password,
			// One message for both rules, as at login: an empty password is just the shortest
			// too-short one, and "Please enter a password" has no entry in the translation table.
			validation.Required.Error("Password must be at least 4 characters"),
			validation.Length(entity.MinPasswordLength, 0).Error("Password must be at least 4 characters"),
		),
	)
}

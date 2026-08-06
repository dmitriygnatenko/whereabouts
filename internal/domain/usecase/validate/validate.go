// Package validate holds the ozzo-validation rule sets shared by more than one use case — currently
// username and password, which both RegisterUser (auth) and the profile use cases (user) need
// validated the same way. Each function returns the exact *domainerror.ValidationError the API has
// always returned, just built through ozzo-validation's rule engine instead of hand-rolled
// if-statements.
package validate

import (
	"wherewhat/internal/domain/entity"

	validation "github.com/go-ozzo/ozzo-validation/v4"

	domainerror "wherewhat/internal/domain/error"
)

// Username checks a (already-normalized) username: it must be present, and at least
// domain.MinUsernameLength characters. The two checks stay separate, each with its own message, so
// an empty username reports "please enter one" rather than "too short".
func Username(username string) *domainerror.ValidationError {
	if err := validation.Validate(username, validation.Required.Error("Please enter a username")); err != nil {
		return &domainerror.ValidationError{Message: err.Error()}
	}

	if err := validation.Validate(username, validation.Length(entity.MinUsernameLength, 0).
		Error("Username must be at least 3 characters")); err != nil {
		return &domainerror.ValidationError{Message: err.Error()}
	}

	return nil
}

// Password checks a plaintext password meets the minimum length. Required is chained in front of
// Length with the same message: Length alone treats an empty value as valid (its doc comment says
// as much — "use Required to make sure a value is not empty"), which would let a blank password
// slip through silently.
func Password(password string) *domainerror.ValidationError {
	const msg = "Password must be at least 4 characters"
	if err := validation.Validate(password,
		validation.Required.Error(msg),
		validation.Length(entity.MinPasswordLength, 0).Error(msg),
	); err != nil {
		return &domainerror.ValidationError{Message: err.Error()}
	}

	return nil
}

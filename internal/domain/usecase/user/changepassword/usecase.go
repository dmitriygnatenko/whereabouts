// Package changepassword is the ChangePassword use case: it lets a signed-in user change their
// password, after confirming their current one. It has no output.go — Execute only ever reports
// success or an error.
package changepassword

import (
	"context"
	"errors"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/usecase/user/shared"
	"wherewhat/internal/domain/usecase/validate"
	"wherewhat/internal/port"
)

// UseCase implements ChangePassword.
type UseCase struct {
	Users  port.UserRepository
	Hasher port.PasswordHasher
}

// New builds a UseCase from its dependencies.
func New(users port.UserRepository, hasher port.PasswordHasher) *UseCase {
	return &UseCase{Users: users, Hasher: hasher}
}

// Execute changes the signed-in user's password, after confirming their current one.
func (uc *UseCase) Execute(ctx context.Context, in Input) error {
	if verr := validate.Password(in.NewPassword); verr != nil {
		return verr
	}

	ok, err := shared.CheckCurrentPassword(ctx, uc.Users, uc.Hasher, in.User.ID, in.CurrentPassword)
	if err != nil {
		return errors.New("Failed to verify current password")
	}

	if !ok {
		// Forbidden, not unauthorized — see updateusername for why.
		return &domainerror.ForbiddenError{Message: "Incorrect current password"}
	}

	newHash, err := uc.Hasher.Hash(in.NewPassword)
	if err != nil {
		return errors.New("Failed to process password")
	}

	if err := uc.Users.UpdatePasswordHash(ctx, in.User.ID, newHash); err != nil {
		return errors.New("Failed to update password")
	}

	return nil
}

// Package changepassword is the ChangePassword use case: it lets a signed-in user change their
// password, after confirming their current one. It has no output.go — Execute only ever reports
// success or an error.
package changepassword

import (
	"context"
	"errors"
	"log/slog"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/usecase"
	"wherewhat/internal/port"
)

// UseCase implements ChangePassword.
type UseCase struct {
	userRepository port.UserRepository
	passwordHasher port.PasswordHasher
}

// New builds a UseCase from its dependencies.
func New(
	userRepository port.UserRepository,
	passwordHasher port.PasswordHasher,
) *UseCase {
	return &UseCase{
		userRepository: userRepository,
		passwordHasher: passwordHasher,
	}
}

// Execute changes the signed-in user's password, after confirming their current one.
func (uc *UseCase) Execute(
	ctx context.Context,
	input Input,
) error {
	if err := input.Validate(); err != nil {
		slog.InfoContext(ctx, "change password: validation", "error", err)

		return domainerror.ToValidationError(err)
	}

	ok, err := usecase.CheckCurrentPassword(
		ctx,
		usecase.CheckCurrentPasswordRequest{
			Users:    uc.userRepository,
			Hasher:   uc.passwordHasher,
			UserID:   input.User.ID,
			Password: input.CurrentPassword,
		},
	)
	if err != nil {
		slog.ErrorContext(ctx, "change password: verify current password", "error", err)

		return errors.New("Failed to verify current password")
	}

	if !ok {
		slog.InfoContext(
			ctx,
			"change password: incorrect current password",
			"user_id", input.User.ID,
		)

		// Forbidden, not unauthorized
		return &domainerror.ForbiddenError{
			Message: "Incorrect current password",
		}
	}

	newHash, err := uc.passwordHasher.Hash(input.NewPassword)
	if err != nil {
		slog.ErrorContext(ctx, "change password: hash", "error", err)

		return errors.New("Failed to process password")
	}

	if err = uc.userRepository.UpdatePasswordHash(ctx, input.User.ID, newHash); err != nil {
		slog.ErrorContext(ctx, "change password: save", "user_id", input.User.ID, "error", err)

		return errors.New("Failed to update password")
	}

	return nil
}

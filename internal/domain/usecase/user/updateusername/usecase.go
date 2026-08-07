// Package updateusername is the UpdateUsername use case: it lets a signed-in user change their
// login username, after confirming their current password.
package updateusername

import (
	"context"
	"errors"
	"log/slog"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/usecase"
	"wherewhat/internal/port"
)

// UseCase implements UpdateUsername.
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

// Execute changes the signed-in user's username, after confirming their current password.
func (uc *UseCase) Execute(
	ctx context.Context,
	input Input,
) (Output, error) {
	if err := input.Validate(); err != nil {
		slog.InfoContext(ctx, "update username: validation", "error", err)

		return Output{}, domainerror.ToValidationError(err)
	}

	username := usecase.NormalizeUsername(input.Username)

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
		slog.ErrorContext(ctx, "update username: verify current password", "error", err)

		return Output{}, errors.New("Failed to verify current password")
	}

	if !ok {
		slog.InfoContext(
			ctx,
			"update username: incorrect current password",
			"user_id", input.User.ID,
		)

		// Forbidden, not unauthorized: the session itself is still valid — only the re-entered password
		// was wrong. Treating this as Unauthorized would trip the frontend's global "session expired"
		// handler and log the user out.
		return Output{}, &domainerror.ForbiddenError{Message: "Incorrect current password"}
	}

	if username == input.User.Username {
		return Output{User: input.User}, nil
	}

	if err = uc.userRepository.UpdateUsername(ctx, input.User.ID, username); err != nil {
		var conflict *domainerror.ConflictError
		if errors.As(err, &conflict) {
			slog.InfoContext(ctx, "update username: conflict", "user_id", input.User.ID)

			return Output{}, conflict
		}

		slog.ErrorContext(ctx, "update username: save", "user_id", input.User.ID, "error", err)

		return Output{}, errors.New("Failed to update username")
	}

	updated := input.User
	updated.Username = username

	return Output{
		User: updated,
	}, nil
}

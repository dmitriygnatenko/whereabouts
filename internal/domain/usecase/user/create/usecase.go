// Package create is the CreateUser use case: it creates a new account without starting a
// session. Unlike register (self-service sign-up from the web app), this is meant for
// administrative use — currently the `-create-user` CLI flag.
package create

import (
	"context"
	"errors"
	"log/slog"

	"wherewhat/internal/domain/entity"
	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/usecase"
	"wherewhat/internal/port"
)

// UseCase implements CreateUser.
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

// Execute validates and creates a new account, without starting a session for it.
func (uc *UseCase) Execute(
	ctx context.Context,
	input Input,
) (Output, error) {
	if err := input.Validate(); err != nil {
		slog.InfoContext(ctx, "create user: validation", "error", err)

		return Output{}, domainerror.ToValidationError(err)
	}

	username := usecase.NormalizeUsername(input.Username)

	hash, err := uc.passwordHasher.Hash(input.Password)
	if err != nil {
		slog.ErrorContext(ctx, "create user: hash password", "error", err)

		return Output{}, errors.New("Failed to process password")
	}

	settings := entity.UserSettings{}

	id, err := uc.userRepository.Create(ctx, port.UserCreateRequest{
		Username:     username,
		PasswordHash: hash,
		Settings:     settings,
	})
	if err != nil {
		var conflict *domainerror.ConflictError
		if errors.As(err, &conflict) {
			slog.InfoContext(ctx, "create user: conflict", "username", username)

			return Output{}, conflict
		}

		slog.ErrorContext(ctx, "create user: save", "error", err)

		return Output{}, errors.New("Failed to create user")
	}

	return Output{
		User: entity.PublicUser{
			ID:           id,
			Username:     username,
			UserSettings: settings,
		},
	}, nil
}

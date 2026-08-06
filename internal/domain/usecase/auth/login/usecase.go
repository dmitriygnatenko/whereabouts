package login

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"wherewhat/internal/domain/entity"
	"wherewhat/internal/domain/usecase/auth"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/port"
)

type UseCase struct {
	userRepository    port.UserRepository
	sessionRepository port.SessionRepository
	passwordHasher    port.PasswordHasher
	tokenGenerator    port.TokenGenerator
}

func New(
	userRepository port.UserRepository,
	sessionRepository port.SessionRepository,
	passwordHasher port.PasswordHasher,
	tokenGenerator port.TokenGenerator,
) *UseCase {
	return &UseCase{
		userRepository:    userRepository,
		sessionRepository: sessionRepository,
		passwordHasher:    passwordHasher,
		tokenGenerator:    tokenGenerator,
	}
}

// Execute verifies the given credentials and, on success, starts a new session for the matching user.
func (uc *UseCase) Execute(ctx context.Context, input Input) (Output, error) {
	if err := input.Validate(); err != nil {
		slog.InfoContext(ctx, "login: validation", "error", err)

		return Output{}, domainerror.ToValidationError(err)
	}

	username := auth.NormalizeUsername(input.Username)

	user, err := uc.userRepository.FindByUsername(ctx, username)
	if err != nil || !uc.passwordHasher.Compare(user.PasswordHash, input.Password) {
		return Output{}, &domainerror.UnauthorizedError{
			Message: "Incorrect username or password",
		}
	}

	if user.Settings.Language == "" {
		lang := strings.ToLower(strings.TrimSpace(input.Language))
		if _, ok := entity.SupportedLanguages[lang]; ok {
			if err = uc.userRepository.UpdateLanguage(ctx, user.ID, lang); err != nil {
				slog.ErrorContext(
					ctx,
					"login: failed to save language preference",
					"user_id", user.ID, "error", err,
				)

				return Output{}, errors.New("Failed to save language preference")
			}

			user.Settings.Language = lang
		}
	}

	session, err := auth.NewSession(ctx, uc.sessionRepository, uc.tokenGenerator, user.ID)
	if err != nil {
		slog.ErrorContext(ctx, "login: failed to start a session", "user_id", user.ID, "error", err)

		return Output{}, errors.New("Failed to start a session")
	}

	return Output{
		User:    user.Public(),
		Session: session,
	}, nil
}

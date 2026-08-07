// Package login is the LoginUser use case: it verifies a username/password pair and, on success,
// starts a new session for the matching user.
package login

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"wherewhat/internal/domain/entity"
	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/usecase"
	"wherewhat/internal/port"
)

const sessionDuration = 30 * 24 * time.Hour

// UseCase implements LoginUser.
type UseCase struct {
	userRepository    port.UserRepository
	sessionRepository port.SessionRepository
	passwordHasher    port.PasswordHasher
	tokenGenerator    port.TokenGenerator
}

// New builds a UseCase from its dependencies.
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
func (uc *UseCase) Execute(
	ctx context.Context,
	input Input,
) (Output, error) {
	if err := input.Validate(); err != nil {
		slog.InfoContext(ctx, "login: validation", "error", err)

		return Output{}, domainerror.ToValidationError(err)
	}

	username := usecase.NormalizeUsername(input.Username)

	user, err := uc.userRepository.FindByUsername(ctx, username)

	if err != nil {
		if !domainerror.IsNotFoundError(err) {
			slog.ErrorContext(ctx, "login: find user", "error", err)
		}

		slog.InfoContext(ctx, "login: user is not found")

		return Output{}, &domainerror.UnauthorizedError{
			Message: "Incorrect username or password",
		}
	}

	if !uc.passwordHasher.Compare(user.PasswordHash, input.Password) {
		slog.InfoContext(ctx, "login: incorrect password")

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
					"login: failed to save language",
					"user_id", user.ID, "error", err,
				)

				return Output{}, errors.New("Failed to save language preference")
			}

			user.Settings.Language = lang
		}
	}

	token, err := uc.tokenGenerator.NewToken()
	if err != nil {
		slog.ErrorContext(ctx, "login: failed to generate a token", "error", err)

		return Output{}, errors.New("Failed to generate a token")
	}

	session := entity.Session{
		Token:     token,
		UserID:    user.ID,
		ExpiresAt: time.Now().UTC().Add(sessionDuration),
	}
	if err = uc.sessionRepository.Create(ctx, session); err != nil {
		slog.ErrorContext(ctx, "login: failed to create a session", "error", err)

		return Output{}, errors.New("Failed to start a session")
	}

	return Output{
		User:    user.Public(),
		Session: session,
	}, nil
}

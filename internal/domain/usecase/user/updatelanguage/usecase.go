// Package updatelanguage is the UpdateLanguage use case: it saves the signed-in user's interface
// language, replacing localStorage so the preference follows them across devices.
package updatelanguage

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/port"
)

// UseCase implements UpdateLanguage.
type UseCase struct {
	userRepository port.UserRepository
}

// New builds a UseCase from its dependencies.
func New(
	userRepository port.UserRepository,
) *UseCase {
	return &UseCase{
		userRepository: userRepository,
	}
}

// Execute validates and saves the signed-in user's interface language.
func (uc *UseCase) Execute(
	ctx context.Context,
	input Input,
) (Output, error) {
	if err := input.Validate(); err != nil {
		slog.InfoContext(ctx, "update language: validation", "error", err)

		return Output{}, domainerror.ToValidationError(err)
	}

	lang := strings.ToLower(strings.TrimSpace(input.Language))

	if err := uc.userRepository.UpdateLanguage(ctx, input.User.ID, lang); err != nil {
		slog.ErrorContext(ctx, "update language: save", "user_id", input.User.ID, "error", err)

		return Output{}, errors.New("Failed to save language preference")
	}

	updated := input.User
	updated.Language = lang

	return Output{
		User: updated,
	}, nil
}

// Package updatelanguage is the UpdateLanguage use case: it saves the signed-in user's interface
// language, replacing localStorage so the preference follows them across devices.
package updatelanguage

import (
	"context"
	"errors"
	"strings"
	"wherewhat/internal/domain/entity"
	"wherewhat/internal/port"

	validation "github.com/go-ozzo/ozzo-validation/v4"

	domainerror "wherewhat/internal/domain/error"
)

// supportedLanguageValues is domain.SupportedLanguages' key set, in the []interface{} shape
// validation.In wants — built once at package init so the map stays the single source of truth for
// what's supported.
var supportedLanguageValues = func() []interface{} {
	vals := make([]interface{}, 0, len(entity.SupportedLanguages))
	for lang := range entity.SupportedLanguages {
		vals = append(vals, lang)
	}
	return vals
}()

// UseCase implements UpdateLanguage.
type UseCase struct {
	Users port.UserRepository
}

// New builds a UseCase from its dependencies.
func New(users port.UserRepository) *UseCase {
	return &UseCase{Users: users}
}

// Execute validates and saves the signed-in user's interface language.
func (uc *UseCase) Execute(ctx context.Context, in Input) (Output, error) {
	lang := strings.ToLower(strings.TrimSpace(in.Language))
	// Required is chained in front of In with the same message: In alone treats an empty value as
	// valid (see its doc comment), which would let a blank language slip through instead of being
	// rejected like any other unsupported value.
	const msg = "Unsupported language"
	if err := validation.Validate(lang,
		validation.Required.Error(msg),
		validation.In(supportedLanguageValues...).Error(msg),
	); err != nil {
		return entity.PublicUser{}, &domainerror.ValidationError{Message: err.Error()}
	}

	if err := uc.Users.UpdateLanguage(ctx, in.User.ID, lang); err != nil {
		return entity.PublicUser{}, errors.New("Failed to save language preference")
	}

	updated := in.User
	updated.Language = lang

	return updated, nil
}

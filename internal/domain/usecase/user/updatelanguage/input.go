package updatelanguage

import (
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"

	"wherewhat/internal/domain/entity"
	"wherewhat/internal/domain/usecase"
)

// Input is what UpdateLanguage needs — the currently authenticated user (as resolved by
// authenticate) and the language they want to switch to.
type Input struct {
	User     entity.PublicUser
	Language string
}

// Validate rejects an unsupported language before saving.
func (i Input) Validate() error {
	i.Language = strings.ToLower(strings.TrimSpace(i.Language))

	return validation.ValidateStruct(&i,
		validation.Field(&i.Language, usecase.LanguageRules()...),
	)
}

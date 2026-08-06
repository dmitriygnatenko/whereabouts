package updatelanguage

import (
	"wherewhat/internal/domain/entity"
)

// Input is what UpdateLanguage needs — the currently authenticated user (as resolved by
// authenticatesession) and the language they want to switch to.
type Input struct {
	User     entity.PublicUser
	Language string
}

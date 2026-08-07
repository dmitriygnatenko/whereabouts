package updatelocationfilterdepth

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"

	"wherewhat/internal/domain/entity"
	"wherewhat/internal/domain/usecase"
)

// Input is what UpdateLocationFilterDepth needs — the currently authenticated user and the new
// filter depth. 0 means "no limit".
type Input struct {
	User  entity.PublicUser
	Depth int
}

// Validate rejects a negative depth before saving.
func (i Input) Validate() error {
	return validation.ValidateStruct(&i,
		validation.Field(&i.Depth, usecase.LocationFilterDepthRules()...),
	)
}

package update

import (
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"

	"wherewhat/internal/domain/usecase"
)

// Input is what UpdateLocation needs to rename/recolor a location.
type Input struct {
	ID    uint64
	Name  string
	Color string
}

// Validate rejects a blank name before any repository lookup.
func (i Input) Validate() error {
	i.Name = strings.TrimSpace(i.Name)

	return validation.ValidateStruct(&i,
		validation.Field(&i.Name, usecase.LocationNameRules()...),
	)
}

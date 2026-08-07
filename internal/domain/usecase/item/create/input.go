package create

import (
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"

	"wherewhat/internal/domain/usecase"
)

// Input is what CreateItem needs to create a new item.
type Input struct {
	Name       string
	LocationID uint64
	Notes      string
	Images     []string
}

// Validate rejects a blank name or a missing location before any repository lookup.
func (i Input) Validate() error {
	i.Name = strings.TrimSpace(i.Name)

	return validation.ValidateStruct(&i,
		validation.Field(&i.Name, usecase.ItemNameRules()...),
		validation.Field(&i.LocationID, usecase.LocationIDRules()...),
	)
}

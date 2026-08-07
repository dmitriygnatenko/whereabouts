package create

import (
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"

	"wherewhat/internal/domain/usecase"
)

// Input is what CreateLocation needs to create a new location, optionally nested under a parent. A
// ParentID that is non-nil but not a positive id is treated as "no parent" — this mirrors the
// original handler's behaviour exactly.
type Input struct {
	Name     string
	Color    string
	ParentID *uint64
}

// Validate rejects a blank name before any repository lookup.
func (i Input) Validate() error {
	i.Name = strings.TrimSpace(i.Name)

	return validation.ValidateStruct(&i,
		validation.Field(&i.Name, usecase.LocationNameRules()...),
	)
}

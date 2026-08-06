// Package listlocations is the ListLocations use case: it returns every location. It has no
// input.go — Execute takes nothing beyond a context.
package listlocations

import (
	"context"
	"errors"
	"wherewhat/internal/domain/entity"

	"wherewhat/internal/port"
)

// UseCase implements ListLocations.
type UseCase struct {
	Locations port.LocationRepository
}

// New builds a UseCase from its dependencies.
func New(locations port.LocationRepository) *UseCase {
	return &UseCase{Locations: locations}
}

// Execute returns every location.
func (uc *UseCase) Execute(ctx context.Context) (Output, error) {
	locs, err := uc.Locations.List(ctx)
	if err != nil {
		return nil, errors.New("Failed to load locations")
	}

	if locs == nil {
		locs = []entity.Location{}
	}

	return locs, nil
}

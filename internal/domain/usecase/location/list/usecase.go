// Package list is the ListLocations use case: it returns every location. It has no
// input.go — Execute takes nothing beyond a context.
package list

import (
	"context"
	"errors"
	"log/slog"

	"wherewhat/internal/domain/entity"
	"wherewhat/internal/port"
)

// UseCase implements ListLocations.
type UseCase struct {
	locationRepository port.LocationRepository
}

// New builds a UseCase from its dependencies.
func New(
	locationRepository port.LocationRepository,
) *UseCase {
	return &UseCase{
		locationRepository: locationRepository,
	}
}

// Execute returns every location.
func (uc *UseCase) Execute(ctx context.Context) (Output, error) {
	locations, err := uc.locationRepository.List(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "list locations: load", "error", err)

		return Output{}, errors.New("Failed to load locations")
	}

	if locations == nil {
		locations = []entity.Location{}
	}

	return Output{
		Locations: locations,
	}, nil
}

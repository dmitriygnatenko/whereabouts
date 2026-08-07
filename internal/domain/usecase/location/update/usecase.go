// Package update is the UpdateLocation use case: it renames/ recolors a location. Its
// parent can't be changed this way (matching the original API).
package update

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"wherewhat/internal/domain/entity"
	domainError "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/usecase"
	"wherewhat/internal/port"
)

// UseCase implements UpdateLocation.
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

// Execute renames/recolors an existing location.
func (uc *UseCase) Execute(
	ctx context.Context,
	input Input,
) (Output, error) {
	if err := input.Validate(); err != nil {
		slog.InfoContext(ctx, "update location: validation", "error", err)

		return Output{}, domainError.ToValidationError(err)
	}

	name := strings.TrimSpace(input.Name)
	color := usecase.ResolveColor(input.Color)

	found, err := uc.locationRepository.Update(ctx, port.LocationUpdateRequest{
		ID:    input.ID,
		Name:  name,
		Color: color,
	})
	if err != nil {
		slog.ErrorContext(ctx, "update location: update", "id", input.ID, "error", err)

		return Output{}, errors.New("Failed to update location")
	}

	if !found {
		slog.InfoContext(ctx, "update location: not found", "id", input.ID)

		return Output{}, &domainError.NotFoundError{Message: "Location not found"}
	}

	parentID, err := uc.locationRepository.ParentID(ctx, input.ID)
	if err != nil {
		slog.ErrorContext(ctx, "update location: read back parent", "id", input.ID, "error", err)

		return Output{}, errors.New("Location updated, but failed to read it back")
	}

	return Output{
		Location: entity.Location{
			ID:       input.ID,
			Name:     name,
			Color:    color,
			ParentID: parentID,
		},
	}, nil
}

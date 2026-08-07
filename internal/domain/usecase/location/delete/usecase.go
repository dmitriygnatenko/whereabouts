// Package delete is the DeleteLocation use case: it removes a location, refusing to do so
// while it still has nested locations or items in it. It has no output.go — Execute only ever
// reports success or an error.
package delete

import (
	"context"
	"errors"
	"log/slog"

	domainError "wherewhat/internal/domain/error"
	"wherewhat/internal/port"
)

// UseCase implements DeleteLocation.
type UseCase struct {
	locationRepository port.LocationRepository
	itemRepository     port.ItemRepository
}

// New builds a UseCase from its dependencies.
func New(
	locationRepository port.LocationRepository,
	itemRepository port.ItemRepository,
) *UseCase {
	return &UseCase{
		locationRepository: locationRepository,
		itemRepository:     itemRepository,
	}
}

// Execute deletes a location, refusing while it still has nested locations or items in it.
func (uc *UseCase) Execute(
	ctx context.Context,
	input Input,
) error {
	childCount, err := uc.locationRepository.ChildCount(ctx, input.ID)
	if err != nil {
		slog.ErrorContext(ctx, "delete location: check nested locations", "id", input.ID, "error", err)

		return errors.New("Failed to check nested locations")
	}

	if childCount > 0 {
		slog.InfoContext(ctx, "delete location: has nested locations", "id", input.ID)

		return &domainError.ConflictError{
			Message: "This location has nested locations — delete or move them first",
		}
	}

	itemCount, err := uc.itemRepository.CountByLocation(ctx, input.ID)
	if err != nil {
		slog.ErrorContext(ctx, "delete location: check items", "id", input.ID, "error", err)

		return errors.New("Failed to check items in this location")
	}

	if itemCount > 0 {
		slog.InfoContext(ctx, "delete location: has items", "id", input.ID)

		return &domainError.ConflictError{
			Message: "This location has items in it — move them elsewhere first",
		}
	}

	found, err := uc.locationRepository.Delete(ctx, input.ID)
	if err != nil {
		slog.ErrorContext(ctx, "delete location: delete", "id", input.ID, "error", err)

		return errors.New("Failed to delete location")
	}

	if !found {
		slog.InfoContext(ctx, "delete location: not found", "id", input.ID)

		return &domainError.NotFoundError{Message: "Location not found"}
	}

	return nil
}

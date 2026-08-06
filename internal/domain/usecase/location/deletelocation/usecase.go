// Package deletelocation is the DeleteLocation use case: it removes a location, refusing to do so
// while it still has nested locations or items in it. It has no output.go — Execute only ever
// reports success or an error.
package deletelocation

import (
	"context"
	"errors"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/port"
)

// UseCase implements DeleteLocation.
type UseCase struct {
	Locations port.LocationRepository
	Items     port.ItemRepository
}

// New builds a UseCase from its dependencies.
func New(locations port.LocationRepository, items port.ItemRepository) *UseCase {
	return &UseCase{Locations: locations, Items: items}
}

// Execute deletes a location, refusing while it still has nested locations or items in it.
func (uc *UseCase) Execute(ctx context.Context, in Input) error {
	childCount, err := uc.Locations.ChildCount(ctx, in.ID)
	if err != nil {
		return errors.New("Failed to check nested locations")
	}

	if childCount > 0 {
		return &domainerror.ConflictError{Message: "This location has nested locations — delete or move them first"}
	}

	itemCount, err := uc.Items.CountByLocation(ctx, in.ID)
	if err != nil {
		return errors.New("Failed to check items in this location")
	}

	if itemCount > 0 {
		return &domainerror.ConflictError{Message: "This location has items in it — move them elsewhere first"}
	}

	found, err := uc.Locations.Delete(ctx, in.ID)
	if err != nil {
		return errors.New("Failed to delete location")
	}

	if !found {
		return &domainerror.NotFoundError{Message: "Location not found"}
	}

	return nil
}

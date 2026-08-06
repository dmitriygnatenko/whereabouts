// Package updatelocation is the UpdateLocation use case: it renames/ recolors a location. Its
// parent can't be changed this way (matching the original API).
package updatelocation

import (
	"context"
	"errors"
	"strings"
	"wherewhat/internal/domain/entity"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/usecase/location/shared"
	"wherewhat/internal/port"
)

// UseCase implements UpdateLocation.
type UseCase struct {
	Locations port.LocationRepository
}

// New builds a UseCase from its dependencies.
func New(locations port.LocationRepository) *UseCase {
	return &UseCase{Locations: locations}
}

// Execute renames/recolors an existing location.
func (uc *UseCase) Execute(ctx context.Context, in Input) (Output, error) {
	name := strings.TrimSpace(in.Name)
	if verr := shared.ValidateName(name); verr != nil {
		return entity.Location{}, verr
	}

	color := shared.ResolveColor(in.Color)

	found, err := uc.Locations.Update(ctx, in.ID, name, color)
	if err != nil {
		return entity.Location{}, errors.New("Failed to update location")
	}

	if !found {
		return entity.Location{}, &domainerror.NotFoundError{Message: "Location not found"}
	}

	parentID, err := uc.Locations.ParentID(ctx, in.ID)
	if err != nil {
		return entity.Location{}, errors.New("Location updated, but failed to read it back")
	}

	return entity.Location{ID: in.ID, Name: name, Color: color, ParentID: parentID}, nil
}

// Package createlocation is the CreateLocation use case.
package createlocation

import (
	"context"
	"errors"
	"strings"
	"time"
	"wherewhat/internal/domain/entity"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/usecase/location/shared"
	"wherewhat/internal/port"
)

// UseCase implements CreateLocation.
type UseCase struct {
	Locations port.LocationRepository
}

// New builds a UseCase from its dependencies.
func New(locations port.LocationRepository) *UseCase {
	return &UseCase{Locations: locations}
}

// Execute validates and creates a new location, verifying its parent (if any) exists first.
func (uc *UseCase) Execute(ctx context.Context, in Input) (Output, error) {
	name := strings.TrimSpace(in.Name)
	if verr := shared.ValidateName(name); verr != nil {
		return entity.Location{}, verr
	}

	color := shared.ResolveColor(in.Color)

	parentID := in.ParentID
	if parentID != nil && *parentID > 0 {
		exists, err := uc.Locations.Exists(ctx, *parentID)
		if err != nil {
			return entity.Location{}, errors.New("Failed to verify parent location")
		}

		if !exists {
			return entity.Location{}, &domainerror.ValidationError{Message: "Parent location not found"}
		}
	} else {
		parentID = nil
	}

	now := time.Now().UTC()

	loc, err := uc.Locations.Create(ctx, name, color, parentID, now)
	if err != nil {
		return entity.Location{}, errors.New("Failed to save location")
	}

	return loc, nil
}

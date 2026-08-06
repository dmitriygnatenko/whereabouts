// Package updateitem is the UpdateItem use case: it rewrites an existing item's fields and photo
// set.
package updateitem

import (
	"context"
	"errors"
	"strings"
	"time"
	"wherewhat/internal/domain/entity"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/usecase/item/shared"
	"wherewhat/internal/port"
)

// UseCase implements UpdateItem.
type UseCase struct {
	Items     port.ItemRepository
	Locations port.LocationRepository
	Store     port.ImageStorage
	Processor port.ImageProcessor
}

// New builds a UseCase from its dependencies.
func New(
	items port.ItemRepository, locations port.LocationRepository, store port.ImageStorage, proc port.ImageProcessor,
) *UseCase {
	return &UseCase{Items: items, Locations: locations, Store: store, Processor: proc}
}

// Execute rewrites an existing item's fields, verifying its (possibly new) location exists and
// processing (compressing + storing) any freshly-uploaded photos.
func (uc *UseCase) Execute(ctx context.Context, in Input) (Output, error) {
	if verr := shared.ValidateInput(in.Name, in.LocationID); verr != nil {
		return entity.Item{}, verr
	}

	exists, err := uc.Locations.Exists(ctx, in.LocationID)
	if err != nil {
		return entity.Item{}, errors.New("Failed to verify location")
	}

	if !exists {
		return entity.Item{}, &domainerror.ValidationError{Message: "The specified location was not found"}
	}

	images, err := shared.ProcessImages(uc.Store, uc.Processor, in.Images)
	if err != nil {
		return entity.Item{}, &domainerror.ValidationError{Message: err.Error()}
	}

	now := time.Now().UTC()

	found, err := uc.Items.Update(ctx, in.ID, strings.TrimSpace(in.Name), strings.TrimSpace(in.Notes), in.LocationID, now)
	if err != nil {
		return entity.Item{}, errors.New("Failed to update item")
	}

	if !found {
		return entity.Item{}, &domainerror.NotFoundError{Message: "Record not found"}
	}

	removed, err := uc.Items.ReplaceImages(ctx, in.ID, images)
	if err != nil {
		return entity.Item{}, errors.New("Failed to update photos")
	}

	for _, url := range removed {
		uc.Store.Delete(url)
	}

	item, err := uc.Items.GetByID(ctx, in.ID)
	if err != nil {
		return entity.Item{}, errors.New("Item updated, but failed to read it back")
	}

	return item, nil
}

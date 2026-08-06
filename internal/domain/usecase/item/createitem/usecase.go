// Package createitem is the CreateItem use case: it creates a new item, verifying its location
// exists and processing (compressing + storing) any freshly-uploaded photos.
package createitem

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

// UseCase implements CreateItem.
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

// Execute validates and creates a new item, verifying its location exists and processing
// (compressing + storing) any freshly-uploaded photos.
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

	id, err := uc.Items.Create(ctx, strings.TrimSpace(in.Name), strings.TrimSpace(in.Notes), in.LocationID, now)
	if err != nil {
		return entity.Item{}, errors.New("Failed to save item")
	}

	if _, err := uc.Items.ReplaceImages(ctx, id, images); err != nil {
		return entity.Item{}, errors.New("Failed to save photos")
	}

	item, err := uc.Items.GetByID(ctx, id)
	if err != nil {
		return entity.Item{}, errors.New("Item saved, but failed to read it back")
	}

	return item, nil
}

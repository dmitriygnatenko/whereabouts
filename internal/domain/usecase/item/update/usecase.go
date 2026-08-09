// Package update is the UpdateItem use case: it rewrites an existing item's fields and photo
// set.
package update

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/usecase"
	"wherewhat/internal/port"
)

// UseCase implements UpdateItem.
type UseCase struct {
	itemRepository     port.ItemRepository
	locationRepository port.LocationRepository
	imageStorage       port.ImageStorage
	imageProcessor     port.ImageProcessor
}

// New builds a UseCase from its dependencies.
func New(
	itemRepository port.ItemRepository,
	locationRepository port.LocationRepository,
	imageStorage port.ImageStorage,
	imageProcessor port.ImageProcessor,
) *UseCase {
	return &UseCase{
		itemRepository:     itemRepository,
		locationRepository: locationRepository,
		imageStorage:       imageStorage,
		imageProcessor:     imageProcessor,
	}
}

// Execute rewrites an existing item's fields, verifying its (possibly new) location exists and
// processing (compressing + storing) any freshly-uploaded photos.
func (uc *UseCase) Execute(
	ctx context.Context,
	input Input,
) (Output, error) {
	if err := input.Validate(); err != nil {
		slog.InfoContext(ctx, "update item: validation", "error", err)

		return Output{}, domainerror.ToValidationError(err)
	}

	exists, err := uc.locationRepository.Exists(ctx, input.LocationID)
	if err != nil {
		slog.ErrorContext(ctx, "update item: verify location", "error", err)

		return Output{}, errors.New("Failed to verify location")
	}

	if !exists {
		slog.InfoContext(ctx, "update item: location not found", "location_id", input.LocationID)

		return Output{}, &domainerror.ValidationError{Message: "The specified location was not found"}
	}

	images, err := usecase.ProcessImages(uc.imageStorage, uc.imageProcessor, input.Images)
	if err != nil {
		slog.InfoContext(ctx, "update item: process images", "error", err)

		return Output{}, &domainerror.ValidationError{Message: err.Error()}
	}

	now := time.Now().UTC()

	found, err := uc.itemRepository.Update(ctx, port.ItemUpdateRequest{
		ID:         input.ID,
		Name:       strings.TrimSpace(input.Name),
		Notes:      strings.TrimSpace(input.Notes),
		LocationID: input.LocationID,
		Now:        now,
	})
	if err != nil {
		slog.ErrorContext(ctx, "update item: update", "id", input.ID, "error", err)

		return Output{}, errors.New("Failed to update item")
	}

	if !found {
		slog.InfoContext(ctx, "update item: not found", "id", input.ID)

		return Output{}, &domainerror.NotFoundError{Message: "Record not found"}
	}

	removed, err := uc.itemRepository.ReplaceImages(ctx, input.ID, images)
	if err != nil {
		slog.ErrorContext(ctx, "update item: save photos", "id", input.ID, "error", err)

		return Output{}, errors.New("Failed to update photos")
	}

	for _, img := range removed {
		uc.imageStorage.Delete(img.URL)

		if img.ThumbnailURL != "" {
			uc.imageStorage.Delete(img.ThumbnailURL)
		}
	}

	item, err := uc.itemRepository.GetByID(ctx, input.ID)
	if err != nil {
		slog.ErrorContext(ctx, "update item: read back item", "id", input.ID, "error", err)

		return Output{}, errors.New("Item updated, but failed to read it back")
	}

	return Output{Item: item}, nil
}

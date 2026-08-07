// Package create is the CreateItem use case: it creates a new item, verifying its location
// exists and processing (compressing + storing) any freshly-uploaded photos.
package create

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	domainError "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/usecase"
	"wherewhat/internal/port"
)

// UseCase implements CreateItem.
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

// Execute validates and creates a new item, verifying its location exists and processing
// (compressing + storing) any freshly-uploaded photos.
func (uc *UseCase) Execute(
	ctx context.Context,
	input Input,
) (Output, error) {
	if err := input.Validate(); err != nil {
		slog.InfoContext(ctx, "create item: validation", "error", err)

		return Output{}, domainError.ToValidationError(err)
	}

	exists, err := uc.locationRepository.Exists(ctx, input.LocationID)
	if err != nil {
		slog.ErrorContext(ctx, "create item: verify location", "error", err)

		return Output{}, errors.New("Failed to verify location")
	}

	if !exists {
		slog.InfoContext(ctx, "create item: location not found", "location_id", input.LocationID)

		return Output{}, &domainError.ValidationError{Message: "The specified location was not found"}
	}

	images, err := usecase.ProcessImages(uc.imageStorage, uc.imageProcessor, input.Images)
	if err != nil {
		slog.InfoContext(ctx, "create item: process images", "error", err)

		return Output{}, &domainError.ValidationError{Message: err.Error()}
	}

	now := time.Now().UTC()

	id, err := uc.itemRepository.Create(ctx, port.ItemCreateRequest{
		Name:       strings.TrimSpace(input.Name),
		Notes:      strings.TrimSpace(input.Notes),
		LocationID: input.LocationID,
		Now:        now,
	})
	if err != nil {
		slog.ErrorContext(ctx, "create item: save item", "error", err)

		return Output{}, errors.New("Failed to save item")
	}

	if _, err = uc.itemRepository.ReplaceImages(ctx, id, images); err != nil {
		slog.ErrorContext(ctx, "create item: save photos", "id", id, "error", err)

		return Output{}, errors.New("Failed to save photos")
	}

	item, err := uc.itemRepository.GetByID(ctx, id)
	if err != nil {
		slog.ErrorContext(ctx, "create item: read back item", "id", id, "error", err)

		return Output{}, errors.New("Item saved, but failed to read it back")
	}

	return Output{Item: item}, nil
}

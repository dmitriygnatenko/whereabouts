// Package delete is the DeleteItem use case: it removes an item and any photo files it owned.
// It has no output.go — Execute only ever reports success or an error, there's no value to hand
// back.
package delete

import (
	"context"
	"errors"
	"log/slog"

	domainError "wherewhat/internal/domain/error"
	"wherewhat/internal/port"
)

// UseCase implements DeleteItem.
type UseCase struct {
	itemRepository port.ItemRepository
	imageStorage   port.ImageStorage
}

// New builds a UseCase from its dependencies.
func New(
	itemRepository port.ItemRepository,
	imageStorage port.ImageStorage,
) *UseCase {
	return &UseCase{
		itemRepository: itemRepository,
		imageStorage:   imageStorage,
	}
}

// Execute deletes an item and any photo files it owned.
func (uc *UseCase) Execute(
	ctx context.Context,
	input Input,
) error {
	removed, found, err := uc.itemRepository.Delete(ctx, input.ID)
	if err != nil {
		slog.ErrorContext(ctx, "delete item: delete", "id", input.ID, "error", err)

		return errors.New("Failed to delete item")
	}

	if !found {
		slog.InfoContext(ctx, "delete item: not found", "id", input.ID)

		return &domainError.NotFoundError{
			Message: "Record not found",
		}
	}

	for _, url := range removed {
		uc.imageStorage.Delete(url)
	}

	return nil
}

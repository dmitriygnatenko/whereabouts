// Package deleteitem is the DeleteItem use case: it removes an item and any photo files it owned.
// It has no output.go — Execute only ever reports success or an error, there's no value to hand
// back.
package deleteitem

import (
	"context"
	"errors"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/port"
)

// UseCase implements DeleteItem.
type UseCase struct {
	Items port.ItemRepository
	Store port.ImageStorage
}

// New builds a UseCase from its dependencies.
func New(items port.ItemRepository, store port.ImageStorage) *UseCase {
	return &UseCase{Items: items, Store: store}
}

// Execute deletes an item and any photo files it owned.
func (uc *UseCase) Execute(ctx context.Context, in Input) error {
	removed, found, err := uc.Items.Delete(ctx, in.ID)
	if err != nil {
		return errors.New("Failed to delete item")
	}

	if !found {
		return &domainerror.NotFoundError{Message: "Record not found"}
	}

	for _, url := range removed {
		uc.Store.Delete(url)
	}

	return nil
}

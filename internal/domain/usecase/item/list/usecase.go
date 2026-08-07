// Package list is the ListItems use case: it returns every item. It has no input.go — Execute
// takes nothing beyond a context.
package list

import (
	"context"
	"errors"
	"log/slog"

	"wherewhat/internal/domain/entity"
	"wherewhat/internal/port"
)

// UseCase implements ListItems.
type UseCase struct {
	itemRepository port.ItemRepository
}

// New builds a UseCase from its dependencies.
func New(
	itemRepository port.ItemRepository,
) *UseCase {
	return &UseCase{
		itemRepository: itemRepository,
	}
}

// Execute returns every item.
func (uc *UseCase) Execute(ctx context.Context) (Output, error) {
	items, err := uc.itemRepository.List(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "list items: load", "error", err)

		return Output{}, errors.New("Failed to load items")
	}

	if items == nil {
		items = []entity.Item{}
	}

	return Output{Items: items}, nil
}

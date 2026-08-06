// Package listitems is the ListItems use case: it returns every item. It has no input.go — Execute
// takes nothing beyond a context.
package listitems

import (
	"context"
	"errors"
	"wherewhat/internal/domain/entity"

	"wherewhat/internal/port"
)

// UseCase implements ListItems.
type UseCase struct {
	Items port.ItemRepository
}

// New builds a UseCase from its dependencies.
func New(items port.ItemRepository) *UseCase {
	return &UseCase{Items: items}
}

// Execute returns every item.
func (uc *UseCase) Execute(ctx context.Context) (Output, error) {
	items, err := uc.Items.List(ctx)
	if err != nil {
		return nil, errors.New("Failed to load items")
	}

	if items == nil {
		items = []entity.Item{}
	}

	return items, nil
}

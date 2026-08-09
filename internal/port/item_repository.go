// Package port declares the interfaces (outbound ports) that use cases depend on. Adapters under
// internal/adapter implement these interfaces; use cases under internal/usecase only ever see the
// interface, never the concrete adapter.
package port

import (
	"context"
	"time"

	"wherewhat/internal/domain/entity"
)

//go:generate go tool mockgen -source=item_repository.go -destination=mocks/item_repository_mock.go -package=mocks

// ItemCreateRequest bundles the ItemRepository.Create parameters that ride along with the context.
type ItemCreateRequest struct {
	Name       string
	Notes      string
	LocationID uint64
	Now        time.Time
}

// ItemUpdateRequest bundles the ItemRepository.Update parameters that ride along with the context.
type ItemUpdateRequest struct {
	ID         uint64
	Name       string
	Notes      string
	LocationID uint64
	Now        time.Time
}

// ItemRepository persists Items and their photos.
type ItemRepository interface {
	List(ctx context.Context) ([]entity.Item, error)
	GetByID(ctx context.Context, id uint64) (entity.Item, error)

	// Create inserts the item row and returns its new id. It does not touch photos — call
	// ReplaceImages afterwards, mirroring Update.
	Create(ctx context.Context, req ItemCreateRequest) (id uint64, err error)

	// Update rewrites the item row. found is false if no row with this id exists.
	Update(ctx context.Context, req ItemUpdateRequest) (found bool, err error)

	// ReplaceImages swaps an item's photo rows for the given images and reports which
	// previously-stored images are no longer referenced, so the caller can delete their files (both
	// full-size and thumbnail) via port.ImageStorage.
	ReplaceImages(
		ctx context.Context, itemID uint64, images []entity.ItemImage,
	) (removed []entity.ItemImage, err error)

	// Delete removes the item row (photo rows cascade) and reports the photos it used to have, for
	// file cleanup. found is false if no row with this id existed.
	Delete(ctx context.Context, id uint64) (removedImages []entity.ItemImage, found bool, err error)

	// CountByLocation reports how many items currently live in a location — used to guard location
	// deletion.
	CountByLocation(ctx context.Context, locationID uint64) (int, error)
}

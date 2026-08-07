// Package item implements port.ItemRepository on top of the items and item_images tables: it
// converts row models into domain entities, attaches each item's separately-stored photos, and turns
// a missing row into *domainerror.NotFoundError.
package item

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"wherewhat/internal/domain/entity"
	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/port"
	"wherewhat/internal/storage/model"
)

//go:generate go tool mockgen -source=repository.go -destination=mocks/storage_mock.go -package=mocks

// Storage is the slice of a driver adapter this repository uses — the items and item_images tables
// and nothing else. Every driver adapter (internal/adapter/mysql, postgres, sqlite) implements it in
// its own dialect.
type Storage interface {
	// ListItems returns every item row, most recently updated first. Photos are not part of the row —
	// fetch them with ListItemImages.
	ListItems(ctx context.Context) ([]model.Item, error)

	// FindItemByID returns sql.ErrNoRows when no item has this id.
	FindItemByID(ctx context.Context, id uint64) (model.Item, error)

	// ListItemImages returns the photo URLs of several items at once, keyed by item id and in position
	// order within each item — one query instead of N. itemIDs must not be empty.
	ListItemImages(ctx context.Context, itemIDs []uint64) (map[uint64][]string, error)

	// ItemImageURLs returns the photo URLs stored for one item.
	ItemImageURLs(ctx context.Context, itemID uint64) ([]string, error)

	// CreateItem inserts an item row and returns its new id.
	CreateItem(
		ctx context.Context, name, notes string, locationID uint64, now time.Time,
	) (id uint64, err error)

	// UpdateItem rewrites an item row. found is false if no row with this id exists.
	UpdateItem(
		ctx context.Context, id uint64, name, notes string, locationID uint64, now time.Time,
	) (found bool, err error)

	// ReplaceItemImages atomically swaps an item's photo rows for urls, in the order given.
	ReplaceItemImages(ctx context.Context, itemID uint64, urls []string) error

	// DeleteItem removes an item row (its photo rows cascade). found is false if no row with this id
	// existed.
	DeleteItem(ctx context.Context, id uint64) (found bool, err error)

	CountItemsByLocation(ctx context.Context, locationID uint64) (int, error)
}

// Repository implements port.ItemRepository.
type Repository struct {
	storage Storage
}

// New builds a Repository against s.
func New(s Storage) *Repository {
	return &Repository{storage: s}
}

// List returns every item, most recently updated first, with each item's photos attached.
func (r *Repository) List(ctx context.Context) ([]entity.Item, error) {
	models, err := r.storage.ListItems(ctx)
	if err != nil {
		return nil, err
	}

	ids := make([]uint64, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}

	imagesByItem, err := r.imagesFor(ctx, ids)
	if err != nil {
		return nil, err
	}

	items := make([]entity.Item, len(models))
	for i, m := range models {
		items[i] = m.ToEntity(orEmpty(imagesByItem[m.ID]))
	}

	return items, nil
}

// GetByID returns a single item with its photos attached, reporting an unknown id as a
// *domainerror.NotFoundError.
func (r *Repository) GetByID(
	ctx context.Context,
	id uint64,
) (entity.Item, error) {
	m, err := r.storage.FindItemByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return entity.Item{}, &domainerror.NotFoundError{Message: "Item not found"}
		}

		return entity.Item{}, err
	}

	images, err := r.storage.ItemImageURLs(ctx, id)
	if err != nil {
		return entity.Item{}, err
	}

	return m.ToEntity(orEmpty(images)), nil
}

// Create inserts a new item and returns its id.
func (r *Repository) Create(ctx context.Context, req port.ItemCreateRequest) (uint64, error) {
	return r.storage.CreateItem(ctx, req.Name, req.Notes, req.LocationID, req.Now)
}

// Update rewrites an existing item's fields, reporting via the bool whether a row with that id was
// found.
func (r *Repository) Update(ctx context.Context, req port.ItemUpdateRequest) (bool, error) {
	return r.storage.UpdateItem(ctx, req.ID, req.Name, req.Notes, req.LocationID, req.Now)
}

// ReplaceImages overwrites an item's photo set with images (in the given order) and returns
// whichever previously-stored URLs are no longer referenced, so the caller can remove their files
// from disk.
func (r *Repository) ReplaceImages(
	ctx context.Context,
	itemID uint64,
	images []string,
) ([]string, error) {
	oldURLs, err := r.storage.ItemImageURLs(ctx, itemID)
	if err != nil {
		return nil, err
	}

	if err := r.storage.ReplaceItemImages(ctx, itemID, images); err != nil {
		return nil, err
	}

	return removed(oldURLs, images), nil
}

// Delete removes an item, reporting whether it was found and returning the photo URLs it owned (for
// the caller to remove from disk).
func (r *Repository) Delete(
	ctx context.Context,
	id uint64,
) ([]string, bool, error) {
	// Best-effort: read the photo URLs before deleting (item_images rows cascade-delete with the item)
	// so the caller can also remove their files on disk. A lookup failure here shouldn't block the
	// delete.
	urls, _ := r.storage.ItemImageURLs(ctx, id)

	found, err := r.storage.DeleteItem(ctx, id)
	if err != nil {
		return nil, false, err
	}

	if !found {
		return nil, false, nil
	}

	return urls, true, nil
}

// CountByLocation counts the items currently stored in a location — used to refuse deleting a
// location that still has items in it.
func (r *Repository) CountByLocation(
	ctx context.Context,
	locationID uint64,
) (int, error) {
	return r.storage.CountItemsByLocation(ctx, locationID)
}

// imagesFor loads the photos of a batch of items in one round-trip, skipping the query entirely when
// there are no items to ask about.
func (r *Repository) imagesFor(ctx context.Context, itemIDs []uint64) (map[uint64][]string, error) {
	if len(itemIDs) == 0 {
		return map[uint64][]string{}, nil
	}

	return r.storage.ListItemImages(ctx, itemIDs)
}

// removed lists the URLs that were stored before but aren't part of the new photo set.
func removed(oldURLs, newURLs []string) []string {
	kept := make(map[string]bool, len(newURLs))
	for _, url := range newURLs {
		kept[url] = true
	}

	var gone []string

	for _, url := range oldURLs {
		if !kept[url] {
			gone = append(gone, url)
		}
	}

	return gone
}

// orEmpty normalizes a nil photo list into an empty one, so an item without photos serializes as []
// rather than null.
func orEmpty(images []string) []string {
	if images == nil {
		return []string{}
	}

	return images
}

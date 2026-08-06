// Package model holds the row shapes exchanged between the driver adapters (internal/adapter/mysql,
// postgres, sqlite) and the repositories in internal/repository — the persistence-layer
// counterparts of internal/domain/entity, kept separate so a storage schema change (column names,
// JSON encoding, ...) never leaks into the domain entities the rest of the app depends on.
//
// Columns whose wire shape differs per driver (the settings JSON blob, TIMESTAMP values) are typed
// here as sql.Scanner/driver.Valuer implementations, so every adapter can scan and bind them
// straight into these structs without repeating the conversion.
package model

import "wherewhat/internal/domain/entity"

// Item is the shape of a row in the items table (photos live in a separate item_images table and
// aren't part of this row).
type Item struct {
	ID         uint64
	Name       string
	LocationID uint64
	Notes      string
	UpdatedAt  string
}

// ToEntity attaches the given photo URLs, loaded separately, to produce the domain entity.
func (i Item) ToEntity(images []string) entity.Item {
	return entity.Item{
		ID:         i.ID,
		Name:       i.Name,
		LocationID: i.LocationID,
		Notes:      i.Notes,
		Images:     images,
		UpdatedAt:  i.UpdatedAt,
	}
}

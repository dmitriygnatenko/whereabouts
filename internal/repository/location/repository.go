// Package location implements port.LocationRepository on top of the locations table: it converts row
// models into domain entities and decides what a missing row means for each method — "not found" for
// a lookup, a plain false for an existence check.
package location

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"wherewhat/internal/domain/entity"
	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/storage/model"
)

//go:generate go tool mockgen -source=repository.go -destination=mocks/storage_mock.go -package=mocks

// Storage is the slice of a driver adapter this repository uses — the locations table and nothing
// else. Every driver adapter (internal/adapter/mysql, postgres, sqlite) implements it in its own
// dialect.
type Storage interface {
	// ListLocations returns every location row, oldest-created first.
	ListLocations(ctx context.Context) ([]model.Location, error)

	// FindLocationParentID returns a location's parent id (nil for a top-level location), or
	// sql.ErrNoRows when no location has this id — which is also how Exists answers "does this location
	// exist?", so a bare existence check needs no separate query.
	FindLocationParentID(ctx context.Context, id uint64) (*uint64, error)

	// CreateLocation inserts a location row and returns its new id.
	CreateLocation(
		ctx context.Context, name, color string, parentID *uint64, now time.Time,
	) (id uint64, err error)

	// UpdateLocation changes name/color (parent is immutable after creation). found is false if no row
	// with this id exists.
	UpdateLocation(ctx context.Context, id uint64, name, color string) (found bool, err error)

	// CountLocationChildren reports how many locations have this one as their parent.
	CountLocationChildren(ctx context.Context, parentID uint64) (int, error)

	// DeleteLocation removes a location row. found is false if no row with this id existed.
	DeleteLocation(ctx context.Context, id uint64) (found bool, err error)
}

// Repository implements port.LocationRepository.
type Repository struct {
	storage Storage
}

// New builds a Repository against s.
func New(s Storage) *Repository {
	return &Repository{storage: s}
}

// List returns every location, oldest-created first.
func (r *Repository) List(ctx context.Context) ([]entity.Location, error) {
	models, err := r.storage.ListLocations(ctx)
	if err != nil {
		return nil, err
	}

	locations := make([]entity.Location, len(models))
	for i, m := range models {
		locations[i] = m.ToEntity()
	}

	return locations, nil
}

// Exists reports whether a location with this id exists — used to validate an item's or a
// sub-location's parent before saving it. A missing row is the answer here, not an error.
func (r *Repository) Exists(
	ctx context.Context,
	id uint64,
) (bool, error) {
	if _, err := r.storage.FindLocationParentID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// Create inserts a new location and returns it.
func (r *Repository) Create(
	ctx context.Context,
	name string,
	color string,
	parentID *uint64,
	now time.Time,
) (entity.Location, error) {
	id, err := r.storage.CreateLocation(ctx, name, color, parentID, now)
	if err != nil {
		return entity.Location{}, err
	}

	return model.Location{ID: id, Name: name, Color: color, ParentID: parentID}.ToEntity(), nil
}

// Update renames/recolors an existing location, reporting via the bool whether a row with that id
// was found.
func (r *Repository) Update(
	ctx context.Context,
	id uint64,
	name string,
	color string,
) (bool, error) {
	return r.storage.UpdateLocation(ctx, id, name, color)
}

// ParentID returns a location's parent id, or nil for a top-level location. An unknown id is
// reported as a *domainerror.NotFoundError.
func (r *Repository) ParentID(
	ctx context.Context,
	id uint64,
) (*uint64, error) {
	parentID, err := r.storage.FindLocationParentID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &domainerror.NotFoundError{Message: "Location not found"}
		}

		return nil, err
	}

	return parentID, nil
}

// ChildCount counts a location's direct sub-locations — used to refuse deleting a location that
// still has nested locations.
func (r *Repository) ChildCount(
	ctx context.Context,
	id uint64,
) (int, error) {
	return r.storage.CountLocationChildren(ctx, id)
}

// Delete removes a location, reporting whether it was found.
func (r *Repository) Delete(
	ctx context.Context,
	id uint64,
) (bool, error) {
	return r.storage.DeleteLocation(ctx, id)
}

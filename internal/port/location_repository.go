package port

import (
	"context"
	"time"
	"wherewhat/internal/domain/entity"
)

//go:generate go tool mockgen -source=location_repository.go -destination=mocks/location_repository_mock.go -package=mocks

// LocationRepository persists Locations.
type LocationRepository interface {
	List(ctx context.Context) ([]entity.Location, error)
	Exists(ctx context.Context, id uint64) (bool, error)
	Create(ctx context.Context, name, color string, parentID *uint64, now time.Time) (entity.Location, error)

	// Update changes name/color (parent is immutable after creation). found is false if no row with
	// this id exists.
	Update(ctx context.Context, id uint64, name, color string) (found bool, err error)

	// ParentID looks up the current parent of a location — used after Update to build the full
	// response, since Update itself doesn't touch parent_id.
	ParentID(ctx context.Context, id uint64) (*uint64, error)

	// ChildCount reports how many locations have this one as their parent — used to guard deletion.
	ChildCount(ctx context.Context, id uint64) (int, error)

	// Delete removes the location. found is false if no row with this id existed.
	Delete(ctx context.Context, id uint64) (found bool, err error)
}

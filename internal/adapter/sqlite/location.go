package sqlite

import (
	"context"
	"time"

	"wherewhat/internal/storage/model"
)

// ListLocations returns every location row, oldest-created first.
func (s *Storage) ListLocations(ctx context.Context) ([]model.Location, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, title, color, parent_id FROM locations ORDER BY created_at`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var locations []model.Location

	for rows.Next() {
		var m model.Location
		if err := rows.Scan(&m.ID, &m.Name, &m.Color, &m.ParentID); err != nil {
			return nil, err
		}

		locations = append(locations, m)
	}

	return locations, rows.Err()
}

// FindLocationParentID returns a location's parent id, or sql.ErrNoRows if the location doesn't
// exist.
func (s *Storage) FindLocationParentID(ctx context.Context, id uint64) (*uint64, error) {
	var parentID *uint64

	err := s.DB.QueryRowContext(ctx, `SELECT parent_id FROM locations WHERE id = ?`, id).Scan(&parentID)
	if err != nil {
		return nil, err
	}

	return parentID, nil
}

// CreateLocation inserts a location row and returns its new id.
func (s *Storage) CreateLocation(
	ctx context.Context, name, color string, parentID *uint64, now time.Time,
) (uint64, error) {
	return s.insertReturningID(ctx,
		`INSERT INTO locations (title, color, parent_id, created_at) VALUES (?, ?, ?, ?)`,
		name, color, parentID, now,
	)
}

// UpdateLocation changes a location's name and color.
func (s *Storage) UpdateLocation(ctx context.Context, id uint64, name, color string) (bool, error) {
	res, err := s.DB.ExecContext(ctx,
		`UPDATE locations SET title = ?, color = ? WHERE id = ?`, name, color, id,
	)
	if err != nil {
		return false, err
	}

	return affected(res)
}

// CountLocationChildren counts the locations nested directly under this one.
func (s *Storage) CountLocationChildren(ctx context.Context, parentID uint64) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM locations WHERE parent_id = ?`, parentID).Scan(&n)

	return n, err
}

// DeleteLocation removes a location row.
func (s *Storage) DeleteLocation(ctx context.Context, id uint64) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM locations WHERE id = ?`, id)
	if err != nil {
		return false, err
	}

	return affected(res)
}

package postgres

import (
	"context"
	"strconv"
	"strings"
	"time"

	"wherewhat/internal/storage/model"
)

// ListItems returns every item row, most recently updated first.
func (s *Storage) ListItems(ctx context.Context) ([]model.Item, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, title, location_id, notes, updated_at FROM items ORDER BY updated_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.Item

	for rows.Next() {
		var m model.Item
		if err := rows.Scan(&m.ID, &m.Name, &m.LocationID, &m.Notes, &m.UpdatedAt); err != nil {
			return nil, err
		}

		items = append(items, m)
	}

	return items, rows.Err()
}

// FindItemByID looks up a single item row, returning sql.ErrNoRows when there's no match.
func (s *Storage) FindItemByID(ctx context.Context, id uint64) (model.Item, error) {
	var m model.Item

	err := s.DB.QueryRowContext(ctx,
		`SELECT id, title, location_id, notes, updated_at FROM items WHERE id = $1`, id,
	).Scan(&m.ID, &m.Name, &m.LocationID, &m.Notes, &m.UpdatedAt)
	if err != nil {
		return model.Item{}, err
	}

	return m, nil
}

// ListItemImages fetches the photos of several items in one query, to avoid N+1 round-trips when
// listing.
func (s *Storage) ListItemImages(
	ctx context.Context, itemIDs []uint64,
) (map[uint64][]string, error) {
	args := make([]any, len(itemIDs))
	placeholders := make([]string, len(itemIDs))

	for i, id := range itemIDs {
		args[i] = id
		placeholders[i] = "$" + strconv.Itoa(i+1)
	}

	// #nosec G202 -- the interpolated text is a run of "$N" placeholders built from len(itemIDs); the ids
	// themselves are bound as arguments, never spliced into the query.
	rows, err := s.DB.QueryContext(ctx,
		`SELECT item_id, url FROM item_images WHERE item_id IN (`+strings.Join(placeholders, ", ")+`)`+
			` ORDER BY item_id, position`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[uint64][]string, len(itemIDs))

	for rows.Next() {
		var itemID uint64

		var url string
		if err := rows.Scan(&itemID, &url); err != nil {
			return nil, err
		}

		result[itemID] = append(result[itemID], url)
	}

	return result, rows.Err()
}

// ItemImageURLs returns the photos stored for one item, in position order.
func (s *Storage) ItemImageURLs(ctx context.Context, itemID uint64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT url FROM item_images WHERE item_id = $1 ORDER BY position`, itemID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var urls []string

	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, err
		}

		urls = append(urls, url)
	}

	return urls, rows.Err()
}

// CreateItem inserts an item row and returns its new id.
func (s *Storage) CreateItem(
	ctx context.Context, name, notes string, locationID uint64, now time.Time,
) (uint64, error) {
	return s.insertReturningID(ctx,
		`INSERT INTO items (title, location_id, notes, created_at, updated_at)`+
			` VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		name, locationID, notes, now, now,
	)
}

// UpdateItem rewrites an item row.
func (s *Storage) UpdateItem(
	ctx context.Context, id uint64, name, notes string, locationID uint64, now time.Time,
) (bool, error) {
	res, err := s.DB.ExecContext(ctx,
		`UPDATE items SET title = $1, location_id = $2, notes = $3, updated_at = $4 WHERE id = $5`,
		name, locationID, notes, now, id,
	)
	if err != nil {
		return false, err
	}

	return affected(res)
}

// ReplaceItemImages swaps an item's photo rows for urls, in one transaction so a failure part-way
// through can't leave the item with half a photo set.
func (s *Storage) ReplaceItemImages(ctx context.Context, itemID uint64, urls []string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// No-op once the transaction is committed; the deferred call is what rolls it back on every error
	// path below.
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM item_images WHERE item_id = $1`, itemID); err != nil {
		return err
	}

	for position, url := range urls {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO item_images (item_id, url, position) VALUES ($1, $2, $3)`, itemID, url, position,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// DeleteItem removes an item row; its photo rows cascade.
func (s *Storage) DeleteItem(ctx context.Context, id uint64) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM items WHERE id = $1`, id)
	if err != nil {
		return false, err
	}

	return affected(res)
}

// CountItemsByLocation counts the items currently stored in a location.
func (s *Storage) CountItemsByLocation(ctx context.Context, locationID uint64) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM items WHERE location_id = $1`, locationID).Scan(&n)

	return n, err
}

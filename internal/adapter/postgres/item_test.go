package postgres

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"

	"wherewhat/internal/storage/model"
)

// placeholdersQuery mirrors the placeholder-building recipe ListItemImages itself uses (see
// item.go), so a test can compute the exact query text sqlmock has to match for a given number of
// ids without duplicating a hand-typed literal per case.
func placeholdersQuery(n int) string {
	placeholders := make([]string, n)
	for i := range placeholders {
		placeholders[i] = "$" + strconv.Itoa(i+1)
	}

	return `SELECT item_id, url, thumbnail_url FROM item_images WHERE item_id IN (` +
		strings.Join(placeholders, ", ") + `) ORDER BY item_id, position`
}

// TestListItems covers the listing behind the main screen: rows come back scanned into model.Item
// exactly as the driver hands them over — sorting itself is ORDER BY's job, not this method's, so the
// mock rows are already in the order the assertions expect.
func TestListItems(t *testing.T) {
	t.Parallel()

	query := `SELECT id, title, location_id, notes, updated_at FROM items ORDER BY updated_at DESC`

	type row struct {
		id, locationID uint64
		name, notes    string
		updatedAt      time.Time
	}

	fakeRow := func() row {
		return row{
			id:         fakeID(),
			locationID: fakeID(),
			name:       fakeName(),
			notes:      fakeNotes(),
			updatedAt:  fakeTime(),
		}
	}

	tests := []struct {
		name string
		rows []row
	}{
		{name: "an empty result yields no rows"},
		{
			name: "a single item",
			rows: []row{fakeRow()},
		},
		{
			name: "several items are returned in query order",
			rows: []row{
				fakeRow(),
				fakeRow(),
				fakeRow(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)

			mockRows := sqlmock.NewRows([]string{
				"id",
				"title",
				"location_id",
				"notes",
				"updated_at",
			})
			for _, r := range tt.rows {
				mockRows.AddRow(r.id, r.name, r.locationID, r.notes, r.updatedAt)
			}

			mock.ExpectQuery(query).WillReturnRows(mockRows)

			got, err := s.ListItems(context.Background())
			require.NoError(t, err)
			require.Len(t, got, len(tt.rows))

			for i, r := range tt.rows {
				want := model.Item{
					ID:         r.id,
					Name:       r.name,
					LocationID: r.locationID,
					Notes:      r.notes,
					UpdatedAt:  r.updatedAt.Format(time.RFC3339Nano),
				}
				require.Equal(t, want, got[i])
			}
		})
	}

	t.Run("a driver error is propagated", func(t *testing.T) {
		t.Parallel()

		s, mock := newMock(t)
		mock.ExpectQuery(query).WillReturnError(errStub)

		_, err := s.ListItems(context.Background())
		require.ErrorIs(t, err, errStub)
	})

	t.Run("a scan error is propagated", func(t *testing.T) {
		t.Parallel()

		s, mock := newMock(t)
		mock.ExpectQuery(query).WillReturnRows(
			sqlmock.NewRows([]string{
				"id",
				"title",
				"location_id",
				"notes",
				"updated_at",
			}).
				AddRow("not-a-uint64", fakeName(), fakeID(), fakeNotes(), fakeTime()),
		)

		_, err := s.ListItems(context.Background())
		require.Error(t, err, "want the scan failure propagated")
	})
}

// TestFindItemByID covers the single-row lookup, including the timestamp round-trip through
// database/sql's time.Time -> string conversion.
func TestFindItemByID(t *testing.T) {
	t.Parallel()

	query := `SELECT id, title, location_id, notes, updated_at FROM items WHERE id = $1`

	id, locationID := fakeID(), fakeID()
	name, notes := fakeName(), fakeNotes()
	updatedAt := fakeTime()

	tests := []struct {
		name         string
		mock         func(mock sqlmock.Sqlmock)
		assertResult func(t *testing.T, got model.Item)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "finds the row",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(id).WillReturnRows(
					sqlmock.NewRows([]string{
						"id",
						"title",
						"location_id",
						"notes",
						"updated_at",
					}).
						AddRow(id, name, locationID, notes, updatedAt),
				)
			},
			assertResult: func(t *testing.T, got model.Item) {
				want := model.Item{
					ID:         id,
					Name:       name,
					LocationID: locationID,
					Notes:      notes,
					UpdatedAt:  updatedAt.Format(time.RFC3339Nano),
				}
				require.Equal(t, want, got)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:         "an unknown id is sql.ErrNoRows",
			mock:         func(mock sqlmock.Sqlmock) { mock.ExpectQuery(query).WithArgs(id).WillReturnError(sql.ErrNoRows) },
			assertResult: func(t *testing.T, got model.Item) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, sql.ErrNoRows) },
		},
		{
			name:         "a driver error is propagated",
			mock:         func(mock sqlmock.Sqlmock) { mock.ExpectQuery(query).WithArgs(id).WillReturnError(errStub) },
			assertResult: func(t *testing.T, got model.Item) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)
			tt.mock(mock)

			got, err := s.FindItemByID(context.Background(), id)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

// TestListItemImages covers the batched fetch that keeps the listing from doing one query per item:
// the photos come back grouped by item id, and an id without photos simply has no entry.
func TestListItemImages(t *testing.T) {
	t.Parallel()

	kettle, drill := fakeID(), fakeID()
	kettlePhoto1, kettlePhoto2, drillPhoto := fakeURL(), fakeURL(), fakeURL()
	kettleThumb1, kettleThumb2, drillThumb := fakeURL(), fakeURL(), fakeURL()

	type args struct {
		ctx context.Context
		ids []uint64
	}

	tests := []struct {
		name         string
		args         args
		mock         func(mock sqlmock.Sqlmock)
		assertResult func(t *testing.T, got map[uint64][]model.ItemImage)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "groups the photos by item, in position order",
			args: args{
				ctx: context.Background(),
				ids: []uint64{
					kettle,
					drill,
				},
			},
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(placeholdersQuery(2)).WithArgs(kettle, drill).WillReturnRows(
					sqlmock.NewRows([]string{
						"item_id",
						"url",
						"thumbnail_url",
					}).
						AddRow(kettle, kettlePhoto1, kettleThumb1).
						AddRow(kettle, kettlePhoto2, kettleThumb2).
						AddRow(drill, drillPhoto, drillThumb),
				)
			},
			assertResult: func(t *testing.T, got map[uint64][]model.ItemImage) {
				require.Equal(t, map[uint64][]model.ItemImage{
					kettle: {
						{URL: kettlePhoto1, ThumbnailURL: kettleThumb1},
						{URL: kettlePhoto2, ThumbnailURL: kettleThumb2},
					},
					drill: {{URL: drillPhoto, ThumbnailURL: drillThumb}},
				}, got)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "an id without photos has no entry",
			args: args{
				ctx: context.Background(),
				ids: []uint64{drill},
			},
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(placeholdersQuery(1)).WithArgs(drill).WillReturnRows(
					sqlmock.NewRows([]string{
						"item_id",
						"url",
						"thumbnail_url",
					}),
				)
			},
			assertResult: func(t *testing.T, got map[uint64][]model.ItemImage) {
				require.Equal(t, map[uint64][]model.ItemImage{}, got)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			// Postgres, like MySQL, rejects "IN ()" as a syntax error rather than treating it as an empty
			// match. That's exactly why the interface this implements documents itemIDs as "must not be
			// empty" (internal/repository/item/repository.go), and its only caller (imagesFor)
			// short-circuits before ever getting here — this case pins that the adapter still just
			// propagates whatever the driver says about it, rather than silently special-casing it.
			name: "no ids at all propagates the driver's rejection of IN ()",
			args: args{ctx: context.Background()},
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(placeholdersQuery(0)).WillReturnError(pgErr("42601")) // syntax_error
			},
			assertResult: func(t *testing.T, got map[uint64][]model.ItemImage) {},
			assertErr:    func(t *testing.T, err error) { require.Error(t, err) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)
			tt.mock(mock)

			got, err := s.ListItemImages(tt.args.ctx, tt.args.ids)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

// TestItemImages covers the single-item fetch, whose whole job is the position order.
func TestItemImages(t *testing.T) {
	t.Parallel()

	query := `SELECT url, thumbnail_url FROM item_images WHERE item_id = $1 ORDER BY position`
	itemID := fakeID()
	url1, url2 := fakeURL(), fakeURL()
	thumb1, thumb2 := fakeURL(), fakeURL()

	tests := []struct {
		name         string
		mock         func(mock sqlmock.Sqlmock)
		assertResult func(t *testing.T, got []model.ItemImage)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "an item without photos",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(itemID).
					WillReturnRows(sqlmock.NewRows([]string{"url", "thumbnail_url"}))
			},
			assertResult: func(t *testing.T, got []model.ItemImage) { require.Empty(t, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "photos come back in the order the query returns them",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(itemID).WillReturnRows(
					sqlmock.NewRows([]string{"url", "thumbnail_url"}).
						AddRow(url1, thumb1).
						AddRow(url2, thumb2),
				)
			},
			assertResult: func(t *testing.T, got []model.ItemImage) {
				require.Equal(t, []model.ItemImage{
					{URL: url1, ThumbnailURL: thumb1},
					{URL: url2, ThumbnailURL: thumb2},
				}, got)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a driver error is propagated",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(itemID).WillReturnError(errStub)
			},
			assertResult: func(t *testing.T, got []model.ItemImage) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)
			tt.mock(mock)

			got, err := s.ItemImages(context.Background(), itemID)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

// TestCreateItem covers the insert and the id it returns via RETURNING id, plus the one error shape
// the location reference can produce — a driver-reported foreign key violation, simulated here rather
// than enforced by a real server.
func TestCreateItem(t *testing.T) {
	t.Parallel()

	query := `INSERT INTO items (title, location_id, notes, created_at, updated_at)` +
		` VALUES ($1, $2, $3, $4, $5) RETURNING id`

	name, notes := fakeName(), fakeNotes()
	locationID := fakeID()
	now := fakeTime()
	wantID := fakeID()

	tests := []struct {
		name         string
		mock         func(mock sqlmock.Sqlmock)
		assertResult func(t *testing.T, got uint64)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "stores the item and returns its id",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(name, locationID, notes, now, now).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(wantID))
			},
			assertResult: func(t *testing.T, got uint64) { require.Equal(t, wantID, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "an unknown location surfaces the driver's foreign key error",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(name, locationID, notes, now, now).
					WillReturnError(pgErr(pgForeignKeyViolation))
			},
			assertResult: func(t *testing.T, got uint64) {},
			assertErr:    func(t *testing.T, err error) { require.Error(t, err) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)
			tt.mock(mock)

			id, err := s.CreateItem(context.Background(), name, notes, locationID, now)
			tt.assertErr(t, err)
			tt.assertResult(t, id)
		})
	}
}

// TestUpdateItem covers the edit: every column the form can change is bound into the UPDATE, the
// affected-rows count decides "found", and a move to an unknown location surfaces the driver's error.
func TestUpdateItem(t *testing.T) {
	t.Parallel()

	query := `UPDATE items SET title = $1, location_id = $2, notes = $3, updated_at = $4 WHERE id = $5`

	id, locationID := fakeID(), fakeID()
	name, notes := fakeName(), fakeNotes()
	updatedAt := fakeTime()

	tests := []struct {
		name         string
		mock         func(mock sqlmock.Sqlmock)
		assertResult func(t *testing.T, found bool)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "rewrites the row",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(query).WithArgs(name, locationID, notes, updatedAt, id).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			assertResult: func(t *testing.T, found bool) { require.True(t, found) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "an unknown id reports not found",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(query).WithArgs(name, locationID, notes, updatedAt, id).
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			assertResult: func(t *testing.T, found bool) { require.False(t, found) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a move to an unknown location surfaces the driver's foreign key error",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(query).WithArgs(name, locationID, notes, updatedAt, id).
					WillReturnError(pgErr(pgForeignKeyViolation))
			},
			assertResult: func(t *testing.T, found bool) {},
			assertErr:    func(t *testing.T, err error) { require.Error(t, err) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)
			tt.mock(mock)

			found, err := s.UpdateItem(context.Background(), id, name, notes, locationID, updatedAt)
			tt.assertErr(t, err)
			tt.assertResult(t, found)
		})
	}
}

// TestReplaceItemImages covers the transaction the whole photo set is swapped inside: a commit on
// success, and — this is the point of using a transaction at all — a rollback the moment any
// statement inside it fails, whether that's the delete or one of the inserts.
func TestReplaceItemImages(t *testing.T) {
	t.Parallel()

	itemID := fakeID()

	tests := []struct {
		name      string
		images    []model.ItemImage
		mock      func(mock sqlmock.Sqlmock, images []model.ItemImage)
		assertErr func(t *testing.T, err error)
	}{
		{
			name: "sets the photos and commits",
			images: []model.ItemImage{
				{URL: fakeURL(), ThumbnailURL: fakeURL()},
				{URL: fakeURL(), ThumbnailURL: fakeURL()},
			},
			mock: func(mock sqlmock.Sqlmock, images []model.ItemImage) {
				mock.ExpectBegin()
				mock.ExpectExec(`DELETE FROM item_images WHERE item_id = $1`).
					WithArgs(itemID).WillReturnResult(sqlmock.NewResult(0, 0))

				for i, img := range images {
					mock.ExpectExec(`INSERT INTO item_images (item_id, url, thumbnail_url, position) VALUES ($1, $2, $3, $4)`).
						WithArgs(itemID, img.URL, img.ThumbnailURL, i).WillReturnResult(sqlmock.NewResult(int64(fakeID()), 1))
				}

				mock.ExpectCommit()
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "an empty list still deletes the old photos and commits",
			mock: func(mock sqlmock.Sqlmock, images []model.ItemImage) {
				mock.ExpectBegin()
				mock.ExpectExec(`DELETE FROM item_images WHERE item_id = $1`).
					WithArgs(itemID).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectCommit()
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a failed delete rolls back instead of committing",
			mock: func(mock sqlmock.Sqlmock, images []model.ItemImage) {
				mock.ExpectBegin()
				mock.ExpectExec(`DELETE FROM item_images WHERE item_id = $1`).
					WithArgs(itemID).WillReturnError(errStub)
				mock.ExpectRollback()
			},
			assertErr: func(t *testing.T, err error) { require.Error(t, err) },
		},
		{
			name: "a rejected insert rolls back the delete along with it",
			images: []model.ItemImage{
				{URL: fakeURL(), ThumbnailURL: fakeURL()},
				{URL: fakeURL(), ThumbnailURL: fakeURL()},
			},
			mock: func(mock sqlmock.Sqlmock, images []model.ItemImage) {
				mock.ExpectBegin()
				mock.ExpectExec(`DELETE FROM item_images WHERE item_id = $1`).
					WithArgs(itemID).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(`INSERT INTO item_images (item_id, url, thumbnail_url, position) VALUES ($1, $2, $3, $4)`).
					WithArgs(itemID, images[0].URL, images[0].ThumbnailURL, 0).WillReturnError(pgErr(pgForeignKeyViolation))
				mock.ExpectRollback()
			},
			assertErr: func(t *testing.T, err error) { require.Error(t, err) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)
			tt.mock(mock, tt.images)

			err := s.ReplaceItemImages(context.Background(), itemID, tt.images)
			tt.assertErr(t, err)
		})
	}
}

// TestDeleteItem covers the delete: the affected-rows count decides "found", and a driver error is
// propagated untouched. Whether the item's photos actually cascade away is the schema's job (see
// migrate.go's ON DELETE CASCADE), not something a mock can confirm.
func TestDeleteItem(t *testing.T) {
	t.Parallel()

	query := `DELETE FROM items WHERE id = $1`
	id := fakeID()

	tests := []struct {
		name         string
		mock         func(mock sqlmock.Sqlmock)
		assertResult func(t *testing.T, found bool)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "deletes the row",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(query).WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 1))
			},
			assertResult: func(t *testing.T, found bool) { require.True(t, found) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "an unknown id reports not found",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(query).WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 0))
			},
			assertResult: func(t *testing.T, found bool) { require.False(t, found) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:         "a driver error is propagated",
			mock:         func(mock sqlmock.Sqlmock) { mock.ExpectExec(query).WithArgs(id).WillReturnError(errStub) },
			assertResult: func(t *testing.T, found bool) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)
			tt.mock(mock)

			found, err := s.DeleteItem(context.Background(), id)
			tt.assertErr(t, err)
			tt.assertResult(t, found)
		})
	}
}

// TestCountItemsByLocation covers the count that decides whether a location may be deleted.
func TestCountItemsByLocation(t *testing.T) { //nolint:dupl // mirrors TestCountLocationChildren for a different table
	t.Parallel()

	query := `SELECT COUNT(*) FROM items WHERE location_id = $1`
	locationID := fakeID()
	want := gofakeit.Number(0, 500)

	tests := []struct {
		name         string
		mock         func(mock sqlmock.Sqlmock)
		assertResult func(t *testing.T, got int)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "counts the items in the location",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(locationID).WillReturnRows(
					sqlmock.NewRows([]string{"count"}).AddRow(want),
				)
			},
			assertResult: func(t *testing.T, got int) { require.Equal(t, want, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a driver error is propagated",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(locationID).WillReturnError(errStub)
			},
			assertResult: func(t *testing.T, got int) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)
			tt.mock(mock)

			got, err := s.CountItemsByLocation(context.Background(), locationID)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

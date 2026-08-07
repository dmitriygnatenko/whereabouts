package sqlite

import (
	"context"
	"database/sql"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"

	"wherewhat/internal/storage/model"
)

// TestListLocations covers the full listing, which the tree in the UI is built from: every row comes
// back scanned into model.Location — ordering itself is ORDER BY's job, not this method's.
func TestListLocations(t *testing.T) {
	t.Parallel()

	query := `SELECT id, title, color, parent_id FROM locations ORDER BY created_at`

	root := model.Location{
		ID:    fakeID(),
		Name:  fakeName(),
		Color: fakeColor(),
	}
	parent := root.ID
	child := model.Location{
		ID:       fakeID(),
		Name:     fakeName(),
		Color:    fakeColor(),
		ParentID: &parent,
	}

	tests := []struct {
		name string
		rows []model.Location
	}{
		{name: "an empty result yields no rows"},
		{
			name: "a single root location",
			rows: []model.Location{root},
		},
		{
			name: "a nested location carries its parent id",
			rows: []model.Location{
				root,
				child,
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
				"color",
				"parent_id",
			})
			for _, loc := range tt.rows {
				var parentID any
				if loc.ParentID != nil {
					parentID = *loc.ParentID
				}

				mockRows.AddRow(loc.ID, loc.Name, loc.Color, parentID)
			}

			mock.ExpectQuery(query).WillReturnRows(mockRows)

			got, err := s.ListLocations(context.Background())
			require.NoError(t, err)
			require.Len(t, got, len(tt.rows))

			for i, want := range tt.rows {
				require.Equal(t, want.ID, got[i].ID)
				require.Equal(t, want.Name, got[i].Name)
				require.Equal(t, want.Color, got[i].Color)
				require.Equal(t, want.ParentID, got[i].ParentID)
			}
		})
	}

	t.Run("a driver error is propagated", func(t *testing.T) {
		t.Parallel()

		s, mock := newMock(t)
		mock.ExpectQuery(query).WillReturnError(errStub)

		_, err := s.ListLocations(context.Background())
		require.ErrorIs(t, err, errStub)
	})
}

// TestFindLocationParentID covers the single-column lookup the move/nesting checks walk upwards on: a
// NULL parent is a root, and a missing row is sql.ErrNoRows rather than a nil parent — the two must
// not look alike to the caller.
func TestFindLocationParentID(t *testing.T) {
	t.Parallel()

	query := `SELECT parent_id FROM locations WHERE id = ?`
	id, parentID := fakeID(), fakeID()

	tests := []struct {
		name         string
		mock         func(mock sqlmock.Sqlmock)
		assertResult func(t *testing.T, got *uint64)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "a root location has no parent",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
			},
			assertResult: func(t *testing.T, got *uint64) { require.Nil(t, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a nested location reports its parent",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(parentID))
			},
			assertResult: func(t *testing.T, got *uint64) { require.Equal(t, &parentID, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:         "an unknown id is sql.ErrNoRows",
			mock:         func(mock sqlmock.Sqlmock) { mock.ExpectQuery(query).WithArgs(id).WillReturnError(sql.ErrNoRows) },
			assertResult: func(t *testing.T, got *uint64) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, sql.ErrNoRows) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)
			tt.mock(mock)

			got, err := s.FindLocationParentID(context.Background(), id)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

// TestCreateLocation covers the insert, both as a root and nested, plus the one error shape the
// parent reference can produce — a driver-reported foreign key violation.
func TestCreateLocation(t *testing.T) {
	t.Parallel()

	query := `INSERT INTO locations (title, color, parent_id, created_at) VALUES (?, ?, ?, ?)`

	name, color := fakeName(), fakeColor()
	parentID := fakeID()
	createdAt := fakeTime()
	wantID := int64(fakeID())

	tests := []struct {
		name         string
		parent       *uint64
		mock         func(mock sqlmock.Sqlmock, parent any)
		assertResult func(t *testing.T, got uint64)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "a root location",
			mock: func(mock sqlmock.Sqlmock, parent any) {
				mock.ExpectExec(query).WithArgs(name, color, parent, createdAt).WillReturnResult(sqlmock.NewResult(wantID, 1))
			},
			assertResult: func(t *testing.T, got uint64) { require.Equal(t, uint64(wantID), got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:   "nested under an existing location",
			parent: &parentID,
			mock: func(mock sqlmock.Sqlmock, parent any) {
				mock.ExpectExec(query).WithArgs(name, color, parent, createdAt).WillReturnResult(sqlmock.NewResult(wantID, 1))
			},
			assertResult: func(t *testing.T, got uint64) { require.Equal(t, uint64(wantID), got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:   "an unknown parent surfaces the driver's foreign key error",
			parent: &parentID,
			mock: func(mock sqlmock.Sqlmock, parent any) {
				mock.ExpectExec(query).WithArgs(name, color, parent, createdAt).WillReturnError(sqliteForeignKeyErr())
			},
			assertResult: func(t *testing.T, got uint64) {},
			assertErr:    func(t *testing.T, err error) { require.Error(t, err) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)

			var parentArg any
			if tt.parent != nil {
				parentArg = *tt.parent
			}

			tt.mock(mock, parentArg)

			id, err := s.CreateLocation(context.Background(), name, color, tt.parent, createdAt)
			tt.assertErr(t, err)
			tt.assertResult(t, id)
		})
	}
}

// TestUpdateLocation covers the rename: the boolean is how the storage layer answers "was there a row
// to update?", which the repository turns into a not-found error.
func TestUpdateLocation(t *testing.T) {
	t.Parallel()

	query := `UPDATE locations SET title = ?, color = ? WHERE id = ?`
	id := fakeID()
	newName, newColor := fakeName(), fakeColor()

	tests := []struct {
		name         string
		res          sql.Result
		mockErr      error
		assertResult func(t *testing.T, found bool)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name:         "renames and recolors the location",
			res:          sqlmock.NewResult(0, 1),
			assertResult: func(t *testing.T, found bool) { require.True(t, found) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:         "an unknown id is reported as not found",
			res:          sqlmock.NewResult(0, 0),
			assertResult: func(t *testing.T, found bool) { require.False(t, found) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:         "a driver error is propagated",
			mockErr:      errStub,
			assertResult: func(t *testing.T, found bool) { require.False(t, found) },
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)

			exp := mock.ExpectExec(query).WithArgs(newName, newColor, id)
			if tt.mockErr != nil {
				exp.WillReturnError(tt.mockErr)
			} else {
				exp.WillReturnResult(tt.res)
			}

			found, err := s.UpdateLocation(context.Background(), id, newName, newColor)
			tt.assertErr(t, err)
			tt.assertResult(t, found)
		})
	}
}

// TestCountLocationChildren covers the nesting count the delete and depth rules are decided on.
func TestCountLocationChildren(t *testing.T) {
	t.Parallel()

	query := `SELECT COUNT(*) FROM locations WHERE parent_id = ?`
	parentID := fakeID()
	want := gofakeit.Number(0, 50)

	tests := []struct {
		name         string
		mock         func(mock sqlmock.Sqlmock)
		assertResult func(t *testing.T, got int)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "counts the direct children",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(parentID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(want))
			},
			assertResult: func(t *testing.T, got int) { require.Equal(t, want, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:         "a driver error is propagated",
			mock:         func(mock sqlmock.Sqlmock) { mock.ExpectQuery(query).WithArgs(parentID).WillReturnError(errStub) },
			assertResult: func(t *testing.T, got int) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)
			tt.mock(mock)

			got, err := s.CountLocationChildren(context.Background(), parentID)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

// TestDeleteLocation covers the delete and the constraint failure a location still holding items or
// nested locations produces — ON DELETE RESTRICT in migrate.go, simulated here as a driver error
// rather than enforced by a real database file.
func TestDeleteLocation(t *testing.T) {
	t.Parallel()

	query := `DELETE FROM locations WHERE id = ?`
	id := fakeID()

	tests := []struct {
		name         string
		res          sql.Result
		mockErr      error
		assertResult func(t *testing.T, found bool)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name:         "deletes an empty location",
			res:          sqlmock.NewResult(0, 1),
			assertResult: func(t *testing.T, found bool) { require.True(t, found) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:         "an unknown id is reported as not found",
			res:          sqlmock.NewResult(0, 0),
			assertResult: func(t *testing.T, found bool) { require.False(t, found) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:         "a location still holding rows surfaces the driver's constraint error",
			mockErr:      sqliteForeignKeyErr(),
			assertResult: func(t *testing.T, found bool) {},
			assertErr:    func(t *testing.T, err error) { require.Error(t, err) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)

			exp := mock.ExpectExec(query).WithArgs(id)
			if tt.mockErr != nil {
				exp.WillReturnError(tt.mockErr)
			} else {
				exp.WillReturnResult(tt.res)
			}

			found, err := s.DeleteLocation(context.Background(), id)
			tt.assertErr(t, err)
			tt.assertResult(t, found)
		})
	}
}

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/brianvoe/gofakeit/v7"

	"wherewhat/internal/storage/model"
)

// TestListLocations covers the full listing, which the tree in the UI is built from: every row comes
// back scanned into model.Location — ordering itself is ORDER BY's job, not this method's.
func TestListLocations(t *testing.T) {
	query := `SELECT id, title, color, parent_id FROM locations ORDER BY created_at`

	root := model.Location{ID: fakeID(), Name: fakeName(), Color: fakeColor()}
	parent := root.ID
	child := model.Location{ID: fakeID(), Name: fakeName(), Color: fakeColor(), ParentID: &parent}

	tests := []struct {
		name string
		rows []model.Location
	}{
		{name: "an empty result yields no rows"},
		{name: "a single root location", rows: []model.Location{root}},
		{name: "a nested location carries its parent id", rows: []model.Location{root, child}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)

			mockRows := sqlmock.NewRows([]string{"id", "title", "color", "parent_id"})
			for _, loc := range tt.rows {
				var parentID any
				if loc.ParentID != nil {
					parentID = *loc.ParentID
				}

				mockRows.AddRow(loc.ID, loc.Name, loc.Color, parentID)
			}

			mock.ExpectQuery(query).WillReturnRows(mockRows)

			got, err := s.ListLocations(context.Background())
			if err != nil {
				t.Fatalf("ListLocations() error = %v", err)
			}

			if len(got) != len(tt.rows) {
				t.Fatalf("ListLocations() returned %d rows, want %d", len(got), len(tt.rows))
			}

			for i, want := range tt.rows {
				if got[i].ID != want.ID || got[i].Name != want.Name || got[i].Color != want.Color {
					t.Fatalf("ListLocations()[%d] = %+v, want %+v", i, got[i], want)
				}

				switch {
				case want.ParentID == nil && got[i].ParentID != nil:
					t.Fatalf("ListLocations()[%d].ParentID = %d, want nil", i, *got[i].ParentID)
				case want.ParentID != nil && got[i].ParentID == nil:
					t.Fatalf("ListLocations()[%d].ParentID = nil, want %d", i, *want.ParentID)
				case want.ParentID != nil && *got[i].ParentID != *want.ParentID:
					t.Fatalf("ListLocations()[%d].ParentID = %d, want %d", i, *got[i].ParentID, *want.ParentID)
				}
			}
		})
	}

	t.Run("a driver error is propagated", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(query).WillReturnError(errStub)

		if _, err := s.ListLocations(context.Background()); !errors.Is(err, errStub) {
			t.Fatalf("ListLocations() error = %v, want %v", err, errStub)
		}
	})
}

// TestFindLocationParentID covers the single-column lookup the move/nesting checks walk upwards on: a
// NULL parent is a root, and a missing row is sql.ErrNoRows rather than a nil parent — the two must
// not look alike to the caller.
func TestFindLocationParentID(t *testing.T) {
	query := `SELECT parent_id FROM locations WHERE id = $1`
	id, parentID := fakeID(), fakeID()

	tests := []struct {
		name    string
		mock    func(mock sqlmock.Sqlmock)
		want    *uint64
		wantErr error
	}{
		{
			name: "a root location has no parent",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))
			},
		},
		{
			name: "a nested location reports its parent",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(parentID))
			},
			want: &parentID,
		},
		{
			name:    "an unknown id is sql.ErrNoRows",
			mock:    func(mock sqlmock.Sqlmock) { mock.ExpectQuery(query).WithArgs(id).WillReturnError(sql.ErrNoRows) },
			wantErr: sql.ErrNoRows,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)
			tt.mock(mock)

			got, err := s.FindLocationParentID(context.Background(), id)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("FindLocationParentID() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("FindLocationParentID() error = %v", err)
			}

			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("FindLocationParentID() = %d, want nil", *got)
			case tt.want != nil && got == nil:
				t.Fatalf("FindLocationParentID() = nil, want %d", *tt.want)
			case tt.want != nil && *got != *tt.want:
				t.Fatalf("FindLocationParentID() = %d, want %d", *got, *tt.want)
			}
		})
	}
}

// TestCreateLocation covers the insert, both as a root and nested, plus the one error shape the
// parent reference can produce — a driver-reported foreign key violation.
func TestCreateLocation(t *testing.T) {
	query := `INSERT INTO locations (title, color, parent_id, created_at) VALUES ($1, $2, $3, $4) RETURNING id`

	name, color := fakeName(), fakeColor()
	parentID := fakeID()
	createdAt := fakeTime()
	wantID := fakeID()

	tests := []struct {
		name    string
		parent  *uint64
		mock    func(mock sqlmock.Sqlmock, parent any)
		wantErr bool
	}{
		{
			name: "a root location",
			mock: func(mock sqlmock.Sqlmock, parent any) {
				mock.ExpectQuery(query).WithArgs(name, color, parent, createdAt).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(wantID))
			},
		},
		{
			name:   "nested under an existing location",
			parent: &parentID,
			mock: func(mock sqlmock.Sqlmock, parent any) {
				mock.ExpectQuery(query).WithArgs(name, color, parent, createdAt).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(wantID))
			},
		},
		{
			name:   "an unknown parent surfaces the driver's foreign key error",
			parent: &parentID,
			mock: func(mock sqlmock.Sqlmock, parent any) {
				mock.ExpectQuery(query).WithArgs(name, color, parent, createdAt).WillReturnError(pgErr(pgForeignKeyViolation))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)

			var parentArg any
			if tt.parent != nil {
				parentArg = *tt.parent
			}

			tt.mock(mock, parentArg)

			id, err := s.CreateLocation(context.Background(), name, color, tt.parent, createdAt)
			if tt.wantErr {
				if err == nil {
					t.Fatal("CreateLocation() error = nil, want the driver's constraint error propagated")
				}

				return
			}

			if err != nil {
				t.Fatalf("CreateLocation() error = %v", err)
			}

			if id != wantID {
				t.Fatalf("CreateLocation() = %d, want %d", id, wantID)
			}
		})
	}
}

// TestUpdateLocation covers the rename: the boolean is how the storage layer answers "was there a row
// to update?", which the repository turns into a not-found error.
func TestUpdateLocation(t *testing.T) {
	query := `UPDATE locations SET title = $1, color = $2 WHERE id = $3`
	id := fakeID()
	newName, newColor := fakeName(), fakeColor()

	tests := []struct {
		name      string
		res       sql.Result
		mockErr   error
		wantFound bool
		wantErr   bool
	}{
		{name: "renames and recolors the location", res: sqlmock.NewResult(0, 1), wantFound: true},
		{name: "an unknown id is reported as not found", res: sqlmock.NewResult(0, 0)},
		{name: "a driver error is propagated", mockErr: errStub, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)

			exp := mock.ExpectExec(query).WithArgs(newName, newColor, id)
			if tt.mockErr != nil {
				exp.WillReturnError(tt.mockErr)
			} else {
				exp.WillReturnResult(tt.res)
			}

			found, err := s.UpdateLocation(context.Background(), id, newName, newColor)
			if tt.wantErr {
				if !errors.Is(err, errStub) {
					t.Fatalf("UpdateLocation() error = %v, want %v", err, errStub)
				}

				return
			}

			if err != nil {
				t.Fatalf("UpdateLocation() error = %v", err)
			}

			if found != tt.wantFound {
				t.Fatalf("UpdateLocation() = %v, want %v", found, tt.wantFound)
			}
		})
	}
}

// TestCountLocationChildren covers the nesting count the delete and depth rules are decided on.
func TestCountLocationChildren(t *testing.T) {
	query := `SELECT COUNT(*) FROM locations WHERE parent_id = $1`
	parentID := fakeID()
	want := gofakeit.Number(0, 50)

	tests := []struct {
		name    string
		mock    func(mock sqlmock.Sqlmock)
		want    int
		wantErr bool
	}{
		{
			name: "counts the direct children",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(parentID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(want))
			},
			want: want,
		},
		{
			name:    "a driver error is propagated",
			mock:    func(mock sqlmock.Sqlmock) { mock.ExpectQuery(query).WithArgs(parentID).WillReturnError(errStub) },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)
			tt.mock(mock)

			got, err := s.CountLocationChildren(context.Background(), parentID)
			if tt.wantErr {
				if !errors.Is(err, errStub) {
					t.Fatalf("CountLocationChildren() error = %v, want %v", err, errStub)
				}

				return
			}

			if err != nil {
				t.Fatalf("CountLocationChildren() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf("CountLocationChildren() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestDeleteLocation covers the delete and the constraint failure a location still holding items or
// nested locations produces — ON DELETE RESTRICT in migrate.go, simulated here as a driver error
// rather than enforced by a real server.
func TestDeleteLocation(t *testing.T) {
	query := `DELETE FROM locations WHERE id = $1`
	id := fakeID()

	tests := []struct {
		name      string
		res       sql.Result
		mockErr   error
		wantFound bool
		wantErr   bool
	}{
		{name: "deletes an empty location", res: sqlmock.NewResult(0, 1), wantFound: true},
		{name: "an unknown id is reported as not found", res: sqlmock.NewResult(0, 0)},
		{name: "a location still holding rows surfaces the driver's constraint error", mockErr: pgErr(pgForeignKeyViolation), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)

			exp := mock.ExpectExec(query).WithArgs(id)
			if tt.mockErr != nil {
				exp.WillReturnError(tt.mockErr)
			} else {
				exp.WillReturnResult(tt.res)
			}

			found, err := s.DeleteLocation(context.Background(), id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("DeleteLocation() error = nil, want the driver's constraint error propagated")
				}

				return
			}

			if err != nil {
				t.Fatalf("DeleteLocation() error = %v", err)
			}

			if found != tt.wantFound {
				t.Fatalf("DeleteLocation() = %v, want %v", found, tt.wantFound)
			}
		})
	}
}

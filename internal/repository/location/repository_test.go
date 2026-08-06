package location

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"go.uber.org/mock/gomock"

	"wherewhat/internal/domain/entity"
	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/repository/location/mocks"
	"wherewhat/internal/storage/model"
)

// newRepo returns a Repository wired to a fresh MockStorage; any call a test doesn't stub via
// EXPECT() fails it, exactly like an unmet sqlmock expectation would in the adapter tests.
func newRepo(t *testing.T) (*Repository, *mocks.MockStorage) {
	t.Helper()

	mc := gomock.NewController(t)
	t.Cleanup(mc.Finish)

	m := mocks.NewMockStorage(mc)

	return New(m), m
}

// fakeID, fakeName, fakeColor and fakeNow are the field-shaped random values the tests below bind
// into mock expectations and returned rows, so a test failure is never masked by two cases
// accidentally sharing a fixture value.
func fakeID() uint64     { return uint64(gofakeit.Number(1, 1_000_000)) }
func fakeName() string   { return gofakeit.Word() }
func fakeColor() string  { return gofakeit.HexColor() }
func fakeNow() time.Time { return gofakeit.Date().UTC() }

func fakeLocationModel() model.Location {
	return model.Location{
		ID:    fakeID(),
		Name:  fakeName(),
		Color: fakeColor(),
	}
}

// asAny prints a *uint64 as either its pointed-to value or "<nil>", for readable failure messages.
func asAny(p *uint64) any {
	if p == nil {
		return "<nil>"
	}

	return *p
}

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = errors.New(gofakeit.Sentence())

// TestRepository_List covers the row -> entity conversion for every location returned, in query
// order.
func TestRepository_List(t *testing.T) {
	tests := []struct {
		name    string
		mock    func(m *mocks.MockStorage) []entity.Location
		wantErr error
	}{
		{
			name: "no locations",
			mock: func(m *mocks.MockStorage) []entity.Location {
				m.EXPECT().ListLocations(context.Background()).Return([]model.Location{}, nil)

				return []entity.Location{}
			},
		},
		{
			name: "several locations, including a nested one, are converted in query order",
			mock: func(m *mocks.MockStorage) []entity.Location {
				top := fakeLocationModel()
				child := fakeLocationModel()
				child.ParentID = &top.ID

				m.EXPECT().ListLocations(context.Background()).Return([]model.Location{
					top,
					child,
				}, nil)

				return []entity.Location{
					top.ToEntity(),
					child.ToEntity(),
				}
			},
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) []entity.Location {
				m.EXPECT().ListLocations(context.Background()).Return(nil, errStub)

				return nil
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			want := tt.mock(m)

			got, err := r.List(context.Background())
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("List() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("List() error = %v", err)
			}

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("List() = %+v, want %+v", got, want)
			}
		})
	}
}

// TestRepository_Exists covers the existence check that reuses FindLocationParentID: a nil parent
// still counts as "exists" (a top-level location), and only a missing row means it doesn't.
func TestRepository_Exists(t *testing.T) {
	id := fakeID()

	tests := []struct {
		name    string
		mock    func(m *mocks.MockStorage)
		want    bool
		wantErr error
	}{
		{
			name: "a nested location exists",
			mock: func(m *mocks.MockStorage) {
				parentID := fakeID()
				m.EXPECT().FindLocationParentID(context.Background(), id).Return(&parentID, nil)
			},
			want: true,
		},
		{
			name: "a top-level location still exists, even with a nil parent",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().FindLocationParentID(context.Background(), id).Return(nil, nil)
			},
			want: true,
		},
		{
			name: "an unknown id does not exist",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().FindLocationParentID(context.Background(), id).Return(nil, sql.ErrNoRows)
			},
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().FindLocationParentID(context.Background(), id).Return(nil, errStub)
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			tt.mock(m)

			got, err := r.Exists(context.Background(), id)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Exists() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Exists() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf("Exists() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRepository_Create covers the insert, including the entity assembled from the values the
// caller passed in rather than a round-trip read.
func TestRepository_Create(t *testing.T) {
	name, color := fakeName(), fakeColor()
	now := fakeNow()

	type testCase struct {
		name     string
		parentID *uint64
		mock     func(m *mocks.MockStorage) uint64
		wantErr  error
	}

	var tests []testCase

	{
		tests = append(tests, testCase{
			name: "a top-level location",
			mock: func(m *mocks.MockStorage) uint64 {
				wantID := fakeID()
				m.EXPECT().CreateLocation(context.Background(), name, color, (*uint64)(nil), now).Return(wantID, nil)

				return wantID
			},
		})
	}

	{
		parentID := fakeID()
		tests = append(tests, testCase{
			name:     "a nested location",
			parentID: &parentID,
			mock: func(m *mocks.MockStorage) uint64 {
				wantID := fakeID()
				m.EXPECT().CreateLocation(context.Background(), name, color, &parentID, now).Return(wantID, nil)

				return wantID
			},
		})
	}

	{
		tests = append(tests, testCase{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) uint64 {
				m.EXPECT().CreateLocation(context.Background(), name, color, (*uint64)(nil), now).Return(uint64(0), errStub)

				return 0
			},
			wantErr: errStub,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			wantID := tt.mock(m)

			got, err := r.Create(context.Background(), name, color, tt.parentID, now)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Create() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}

			want := model.Location{
				ID:       wantID,
				Name:     name,
				Color:    color,
				ParentID: tt.parentID,
			}.ToEntity()
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Create() = %+v, want %+v", got, want)
			}
		})
	}
}

// TestRepository_Update covers the rename/recolor, whose affected-rows count decides "found".
func TestRepository_Update(t *testing.T) {
	id := fakeID()
	name, color := fakeName(), fakeColor()

	tests := []struct {
		name      string
		mock      func(m *mocks.MockStorage)
		wantFound bool
		wantErr   error
	}{
		{
			name: "renames and recolors the row",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateLocation(context.Background(), id, name, color).Return(true, nil)
			},
			wantFound: true,
		},
		{
			name: "an unknown id reports not found",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateLocation(context.Background(), id, name, color).Return(false, nil)
			},
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateLocation(context.Background(), id, name, color).Return(false, errStub)
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			tt.mock(m)

			found, err := r.Update(context.Background(), id, name, color)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Update() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Update() error = %v", err)
			}

			if found != tt.wantFound {
				t.Fatalf("Update() = %v, want %v", found, tt.wantFound)
			}
		})
	}
}

// TestRepository_ParentID covers the parent lookup, including the id -> NotFoundError translation.
func TestRepository_ParentID(t *testing.T) {
	id := fakeID()

	tests := []struct {
		name         string
		mock         func(m *mocks.MockStorage) *uint64
		wantErr      error
		wantNotFound bool
	}{
		{
			name: "a nested location's parent id",
			mock: func(m *mocks.MockStorage) *uint64 {
				parentID := fakeID()
				m.EXPECT().FindLocationParentID(context.Background(), id).Return(&parentID, nil)

				return &parentID
			},
		},
		{
			name: "a top-level location has a nil parent",
			mock: func(m *mocks.MockStorage) *uint64 {
				m.EXPECT().FindLocationParentID(context.Background(), id).Return(nil, nil)

				return nil
			},
		},
		{
			name: "an unknown id becomes a NotFoundError",
			mock: func(m *mocks.MockStorage) *uint64 {
				m.EXPECT().FindLocationParentID(context.Background(), id).Return(nil, sql.ErrNoRows)

				return nil
			},
			wantNotFound: true,
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) *uint64 {
				m.EXPECT().FindLocationParentID(context.Background(), id).Return(nil, errStub)

				return nil
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			want := tt.mock(m)

			got, err := r.ParentID(context.Background(), id)
			if tt.wantNotFound {
				var notFound *domainerror.NotFoundError
				if !errors.As(err, &notFound) {
					t.Fatalf("ParentID() error = %v, want *domainerror.NotFoundError", err)
				}

				return
			}

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParentID() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParentID() error = %v", err)
			}

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("ParentID() = %v, want %v", asAny(got), asAny(want))
			}
		})
	}
}

// TestRepository_ChildCount covers the plain delegation to storage.
func TestRepository_ChildCount(t *testing.T) {
	id := fakeID()
	want := gofakeit.Number(0, 50)

	tests := []struct {
		name    string
		mock    func(m *mocks.MockStorage)
		want    int
		wantErr error
	}{
		{
			name: "delegates to storage",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().CountLocationChildren(context.Background(), id).Return(want, nil)
			},
			want: want,
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().CountLocationChildren(context.Background(), id).Return(0, errStub)
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			tt.mock(m)

			got, err := r.ChildCount(context.Background(), id)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ChildCount() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ChildCount() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf("ChildCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestRepository_Delete covers the delete, whose affected-rows count decides "found".
func TestRepository_Delete(t *testing.T) {
	id := fakeID()

	tests := []struct {
		name      string
		mock      func(m *mocks.MockStorage)
		wantFound bool
		wantErr   error
	}{
		{
			name: "deletes the row",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().DeleteLocation(context.Background(), id).Return(true, nil)
			},
			wantFound: true,
		},
		{
			name: "an unknown id reports not found",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().DeleteLocation(context.Background(), id).Return(false, nil)
			},
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().DeleteLocation(context.Background(), id).Return(false, errStub)
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			tt.mock(m)

			found, err := r.Delete(context.Background(), id)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Delete() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Delete() error = %v", err)
			}

			if found != tt.wantFound {
				t.Fatalf("Delete() = %v, want %v", found, tt.wantFound)
			}
		})
	}
}

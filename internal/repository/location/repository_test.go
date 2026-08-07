package location

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"wherewhat/internal/domain/entity"
	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/port"
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

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = errors.New(gofakeit.Sentence())

// TestRepository_List covers the row -> entity conversion for every location returned, in query
// order.
func TestRepository_List(t *testing.T) {
	t.Parallel()

	type args struct {
		ctx context.Context
	}

	tests := []struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage) []entity.Location
		assertResult func(t *testing.T, want, got []entity.Location)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "no locations",
			args: args{ctx: context.Background()},
			mock: func(m *mocks.MockStorage) []entity.Location {
				m.EXPECT().ListLocations(context.Background()).Return([]model.Location{}, nil)

				return []entity.Location{}
			},
			assertResult: func(t *testing.T, want, got []entity.Location) {
				require.Equal(t, want, got)
			},
			assertErr: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			name: "several locations, including a nested one, are converted in query order",
			args: args{ctx: context.Background()},
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
			assertResult: func(t *testing.T, want, got []entity.Location) {
				require.Equal(t, want, got)
			},
			assertErr: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			name: "a storage error is propagated",
			args: args{ctx: context.Background()},
			mock: func(m *mocks.MockStorage) []entity.Location {
				m.EXPECT().ListLocations(context.Background()).Return(nil, errStub)

				return nil
			},
			assertResult: func(t *testing.T, want, got []entity.Location) {},
			assertErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, errStub)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			want := tt.mock(m)

			got, err := r.List(tt.args.ctx)
			tt.assertErr(t, err)
			tt.assertResult(t, want, got)
		})
	}
}

// TestRepository_Exists covers the existence check that reuses FindLocationParentID: a nil parent
// still counts as "exists" (a top-level location), and only a missing row means it doesn't.
func TestRepository_Exists(t *testing.T) {
	t.Parallel()

	id := fakeID()

	type args struct {
		ctx context.Context
		id  uint64
	}

	tests := []struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage)
		assertResult func(t *testing.T, got bool)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "a nested location exists",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) {
				parentID := fakeID()
				m.EXPECT().FindLocationParentID(context.Background(), id).Return(&parentID, nil)
			},
			assertResult: func(t *testing.T, got bool) { require.True(t, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a top-level location still exists, even with a nil parent",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().FindLocationParentID(context.Background(), id).Return(nil, nil)
			},
			assertResult: func(t *testing.T, got bool) { require.True(t, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "an unknown id does not exist",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().FindLocationParentID(context.Background(), id).Return(nil, sql.ErrNoRows)
			},
			assertResult: func(t *testing.T, got bool) { require.False(t, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a storage error is propagated",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().FindLocationParentID(context.Background(), id).Return(nil, errStub)
			},
			assertResult: func(t *testing.T, got bool) { require.False(t, got) },
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			tt.mock(m)

			got, err := r.Exists(tt.args.ctx, tt.args.id)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

// TestRepository_Create covers the insert, including the entity assembled from the values the
// caller passed in rather than a round-trip read.
func TestRepository_Create(t *testing.T) {
	t.Parallel()

	name, color := fakeName(), fakeColor()
	now := fakeNow()

	type args struct {
		ctx      context.Context
		name     string
		color    string
		parentID *uint64
		now      time.Time
	}

	type testCase struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage) uint64
		assertResult func(t *testing.T, wantID uint64, parentID *uint64, got entity.Location)
		assertErr    func(t *testing.T, err error)
	}

	var tests []testCase

	{
		tests = append(tests, testCase{
			name: "a top-level location",
			args: args{
				ctx:   context.Background(),
				name:  name,
				color: color,
				now:   now,
			},
			mock: func(m *mocks.MockStorage) uint64 {
				wantID := fakeID()
				m.EXPECT().CreateLocation(context.Background(), name, color, (*uint64)(nil), now).Return(wantID, nil)

				return wantID
			},
			assertResult: func(t *testing.T, wantID uint64, parentID *uint64, got entity.Location) {
				require.Equal(t, model.Location{
					ID:       wantID,
					Name:     name,
					Color:    color,
					ParentID: parentID,
				}.ToEntity(), got)
			},
			assertErr: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		})
	}

	{
		parentID := fakeID()
		tests = append(tests, testCase{
			name: "a nested location",
			args: args{
				ctx:      context.Background(),
				name:     name,
				color:    color,
				parentID: &parentID,
				now:      now,
			},
			mock: func(m *mocks.MockStorage) uint64 {
				wantID := fakeID()
				m.EXPECT().CreateLocation(context.Background(), name, color, &parentID, now).Return(wantID, nil)

				return wantID
			},
			assertResult: func(t *testing.T, wantID uint64, parentID *uint64, got entity.Location) {
				require.Equal(t, model.Location{
					ID:       wantID,
					Name:     name,
					Color:    color,
					ParentID: parentID,
				}.ToEntity(), got)
			},
			assertErr: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		})
	}

	{
		tests = append(tests, testCase{
			name: "a storage error is propagated",
			args: args{
				ctx:   context.Background(),
				name:  name,
				color: color,
				now:   now,
			},
			mock: func(m *mocks.MockStorage) uint64 {
				m.EXPECT().CreateLocation(context.Background(), name, color, (*uint64)(nil), now).Return(uint64(0), errStub)

				return 0
			},
			assertResult: func(
				t *testing.T, wantID uint64, parentID *uint64, got entity.Location,
			) {
			},
			assertErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, errStub)
			},
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			wantID := tt.mock(m)

			got, err := r.Create(tt.args.ctx, port.LocationCreateRequest{
				Name: tt.args.name, Color: tt.args.color, ParentID: tt.args.parentID, Now: tt.args.now,
			})
			tt.assertErr(t, err)
			tt.assertResult(t, wantID, tt.args.parentID, got)
		})
	}
}

// TestRepository_Update covers the rename/recolor, whose affected-rows count decides "found".
func TestRepository_Update(t *testing.T) {
	t.Parallel()

	id := fakeID()
	name, color := fakeName(), fakeColor()

	type args struct {
		ctx   context.Context
		id    uint64
		name  string
		color string
	}

	tests := []struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage)
		assertResult func(t *testing.T, found bool)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "renames and recolors the row",
			args: args{
				ctx:   context.Background(),
				id:    id,
				name:  name,
				color: color,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateLocation(context.Background(), id, name, color).Return(true, nil)
			},
			assertResult: func(t *testing.T, found bool) { require.True(t, found) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "an unknown id reports not found",
			args: args{
				ctx:   context.Background(),
				id:    id,
				name:  name,
				color: color,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateLocation(context.Background(), id, name, color).Return(false, nil)
			},
			assertResult: func(t *testing.T, found bool) { require.False(t, found) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a storage error is propagated",
			args: args{
				ctx:   context.Background(),
				id:    id,
				name:  name,
				color: color,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateLocation(context.Background(), id, name, color).Return(false, errStub)
			},
			assertResult: func(t *testing.T, found bool) { require.False(t, found) },
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			tt.mock(m)

			found, err := r.Update(tt.args.ctx, port.LocationUpdateRequest{
				ID: tt.args.id, Name: tt.args.name, Color: tt.args.color,
			})
			tt.assertErr(t, err)
			tt.assertResult(t, found)
		})
	}
}

// TestRepository_ParentID covers the parent lookup, including the id -> NotFoundError translation.
func TestRepository_ParentID(t *testing.T) {
	t.Parallel()

	id := fakeID()

	type args struct {
		ctx context.Context
		id  uint64
	}

	tests := []struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage) *uint64
		assertResult func(t *testing.T, want, got *uint64)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "a nested location's parent id",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) *uint64 {
				parentID := fakeID()
				m.EXPECT().FindLocationParentID(context.Background(), id).Return(&parentID, nil)

				return &parentID
			},
			assertResult: func(t *testing.T, want, got *uint64) { require.Equal(t, want, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a top-level location has a nil parent",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) *uint64 {
				m.EXPECT().FindLocationParentID(context.Background(), id).Return(nil, nil)

				return nil
			},
			assertResult: func(t *testing.T, want, got *uint64) { require.Nil(t, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "an unknown id becomes a NotFoundError",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) *uint64 {
				m.EXPECT().FindLocationParentID(context.Background(), id).Return(nil, sql.ErrNoRows)

				return nil
			},
			assertResult: func(t *testing.T, want, got *uint64) { require.Nil(t, got) },
			assertErr: func(t *testing.T, err error) {
				var notFound *domainerror.NotFoundError
				require.ErrorAs(t, err, &notFound)
			},
		},
		{
			name: "a storage error is propagated",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) *uint64 {
				m.EXPECT().FindLocationParentID(context.Background(), id).Return(nil, errStub)

				return nil
			},
			assertResult: func(t *testing.T, want, got *uint64) { require.Nil(t, got) },
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			want := tt.mock(m)

			got, err := r.ParentID(tt.args.ctx, tt.args.id)
			tt.assertErr(t, err)
			tt.assertResult(t, want, got)
		})
	}
}

// TestRepository_ChildCount covers the plain delegation to storage.
func TestRepository_ChildCount(t *testing.T) {
	t.Parallel()

	id := fakeID()
	want := gofakeit.Number(0, 50)

	type args struct {
		ctx context.Context
		id  uint64
	}

	tests := []struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage)
		assertResult func(t *testing.T, got int)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "delegates to storage",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().CountLocationChildren(context.Background(), id).Return(want, nil)
			},
			assertResult: func(t *testing.T, got int) { require.Equal(t, want, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a storage error is propagated",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().CountLocationChildren(context.Background(), id).Return(0, errStub)
			},
			assertResult: func(t *testing.T, got int) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			tt.mock(m)

			got, err := r.ChildCount(tt.args.ctx, tt.args.id)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

// TestRepository_Delete covers the delete, whose affected-rows count decides "found".
func TestRepository_Delete(t *testing.T) {
	t.Parallel()

	id := fakeID()

	type args struct {
		ctx context.Context
		id  uint64
	}

	tests := []struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage)
		assertResult func(t *testing.T, found bool)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "deletes the row",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().DeleteLocation(context.Background(), id).Return(true, nil)
			},
			assertResult: func(t *testing.T, found bool) { require.True(t, found) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "an unknown id reports not found",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().DeleteLocation(context.Background(), id).Return(false, nil)
			},
			assertResult: func(t *testing.T, found bool) { require.False(t, found) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a storage error is propagated",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().DeleteLocation(context.Background(), id).Return(false, errStub)
			},
			assertResult: func(t *testing.T, found bool) { require.False(t, found) },
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			tt.mock(m)

			found, err := r.Delete(tt.args.ctx, tt.args.id)
			tt.assertErr(t, err)
			tt.assertResult(t, found)
		})
	}
}

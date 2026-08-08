package item

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
	"wherewhat/internal/repository/item/mocks"
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

// fakeID, fakeName, fakeNotes, fakeURL, fakeTimestamp and fakeNow are the field-shaped random values
// the tests below bind into mock expectations and returned rows, so a test failure is never masked
// by two cases accidentally sharing a fixture value.
func fakeID() uint64        { return uint64(gofakeit.Number(1, 1_000_000)) }
func fakeName() string      { return gofakeit.AppName() }
func fakeNotes() string     { return gofakeit.Sentence() }
func fakeURL() string       { return gofakeit.URL() }
func fakeNow() time.Time    { return gofakeit.Date().UTC() }
func fakeTimestamp() string { return fakeNow().Format(time.RFC3339Nano) }

func fakeItemModel() model.Item {
	return model.Item{
		ID:         fakeID(),
		Name:       fakeName(),
		LocationID: fakeID(),
		Notes:      fakeNotes(),
		UpdatedAt:  fakeTimestamp(),
	}
}

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = errors.New(gofakeit.Sentence())

// TestRepository_List covers the batched photo attachment: an id absent from the images map becomes
// an empty slice rather than nil, and the image lookup is skipped entirely when there's nothing to
// look up.
func TestRepository_List(t *testing.T) {
	t.Parallel()

	type args struct {
		ctx context.Context
	}

	tests := []struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage) []entity.Item
		assertResult func(t *testing.T, want, got []entity.Item)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "no items skips the image lookup entirely",
			args: args{ctx: context.Background()},
			mock: func(m *mocks.MockStorage) []entity.Item {
				m.EXPECT().ListItems(context.Background()).Return([]model.Item{}, nil)
				// ListItemImages deliberately left unstubbed: an empty id list must never reach it.

				return []entity.Item{}
			},
			assertResult: func(
				t *testing.T, want, got []entity.Item,
			) {
				require.Equal(t, want, got)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "items are returned with their photos attached, a missing id becomes an empty slice",
			args: args{ctx: context.Background()},
			mock: func(m *mocks.MockStorage) []entity.Item {
				withPhotos, withoutPhotos := fakeItemModel(), fakeItemModel()
				photos := []string{
					fakeURL(),
					fakeURL(),
				}

				m.EXPECT().ListItems(context.Background()).Return([]model.Item{
					withPhotos,
					withoutPhotos,
				}, nil)
				m.EXPECT().
					ListItemImages(context.Background(), []uint64{
						withPhotos.ID,
						withoutPhotos.ID,
					}).
					Return(map[uint64][]string{withPhotos.ID: photos}, nil)

				return []entity.Item{
					withPhotos.ToEntity(photos),
					withoutPhotos.ToEntity([]string{}),
				}
			},
			assertResult: func(
				t *testing.T, want, got []entity.Item,
			) {
				require.Equal(t, want, got)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a ListItems error is propagated",
			args: args{ctx: context.Background()},
			mock: func(m *mocks.MockStorage) []entity.Item {
				m.EXPECT().ListItems(context.Background()).Return(nil, errStub)

				return nil
			},
			assertResult: func(t *testing.T, want, got []entity.Item) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
		{
			name: "a ListItemImages error is propagated",
			args: args{ctx: context.Background()},
			mock: func(m *mocks.MockStorage) []entity.Item {
				it := fakeItemModel()
				m.EXPECT().ListItems(context.Background()).Return([]model.Item{it}, nil)
				m.EXPECT().ListItemImages(context.Background(), []uint64{it.ID}).Return(nil, errStub)

				return nil
			},
			assertResult: func(t *testing.T, want, got []entity.Item) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
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

// TestRepository_GetByID covers the single-item fetch with its photos attached, and the id ->
// NotFoundError translation.
func TestRepository_GetByID(t *testing.T) {
	t.Parallel()

	id := fakeID()

	type args struct {
		ctx context.Context
		id  uint64
	}

	tests := []struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage) entity.Item
		assertResult func(t *testing.T, want, got entity.Item)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "attaches the item's photos",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) entity.Item {
				it := fakeItemModel()
				it.ID = id
				photos := []string{fakeURL()}

				m.EXPECT().FindItemByID(context.Background(), id).Return(it, nil)
				m.EXPECT().ItemImageURLs(context.Background(), id).Return(photos, nil)

				return it.ToEntity(photos)
			},
			assertResult: func(t *testing.T, want, got entity.Item) { require.Equal(t, want, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "an unknown id becomes a NotFoundError, without looking up photos",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) entity.Item {
				m.EXPECT().FindItemByID(context.Background(), id).Return(model.Item{}, sql.ErrNoRows)
				// ItemImageURLs deliberately left unstubbed.

				return entity.Item{}
			},
			assertResult: func(t *testing.T, want, got entity.Item) {},
			assertErr: func(t *testing.T, err error) {
				var notFound *domainerror.NotFoundError
				require.ErrorAs(t, err, &notFound)
			},
		},
		{
			name: "a FindItemByID error is propagated",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) entity.Item {
				m.EXPECT().FindItemByID(context.Background(), id).Return(model.Item{}, errStub)

				return entity.Item{}
			},
			assertResult: func(t *testing.T, want, got entity.Item) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
		{
			name: "an ItemImageURLs error is propagated",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) entity.Item {
				it := fakeItemModel()
				it.ID = id
				m.EXPECT().FindItemByID(context.Background(), id).Return(it, nil)
				m.EXPECT().ItemImageURLs(context.Background(), id).Return(nil, errStub)

				return entity.Item{}
			},
			assertResult: func(t *testing.T, want, got entity.Item) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			want := tt.mock(m)

			got, err := r.GetByID(tt.args.ctx, tt.args.id)
			tt.assertErr(t, err)
			tt.assertResult(t, want, got)
		})
	}
}

// TestRepository_Create covers the plain delegation to storage.
func TestRepository_Create(t *testing.T) {
	t.Parallel()

	name, notes := fakeName(), fakeNotes()
	locationID := fakeID()
	now := fakeNow()
	wantID := fakeID()

	type args struct {
		ctx        context.Context
		name       string
		notes      string
		locationID uint64
		now        time.Time
	}

	tests := []struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage)
		assertResult func(t *testing.T, got uint64)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "delegates to storage and returns the new id",
			args: args{
				ctx:        context.Background(),
				name:       name,
				notes:      notes,
				locationID: locationID,
				now:        now,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().CreateItem(context.Background(), name, notes, locationID, now).Return(wantID, nil)
			},
			assertResult: func(t *testing.T, got uint64) { require.Equal(t, wantID, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a storage error is propagated",
			args: args{
				ctx:        context.Background(),
				name:       name,
				notes:      notes,
				locationID: locationID,
				now:        now,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().CreateItem(context.Background(), name, notes, locationID, now).Return(uint64(0), errStub)
			},
			assertResult: func(t *testing.T, got uint64) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			tt.mock(m)

			got, err := r.Create(tt.args.ctx, port.ItemCreateRequest{
				Name: tt.args.name, Notes: tt.args.notes, LocationID: tt.args.locationID, Now: tt.args.now,
			})
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

// TestRepository_Update covers the edit, whose affected-rows count decides "found".
func TestRepository_Update(t *testing.T) {
	t.Parallel()

	id, locationID := fakeID(), fakeID()
	name, notes := fakeName(), fakeNotes()
	now := fakeNow()

	type args struct {
		ctx        context.Context
		id         uint64
		name       string
		notes      string
		locationID uint64
		now        time.Time
	}

	tests := []struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage)
		assertResult func(t *testing.T, found bool)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "rewrites the row",
			args: args{
				ctx:        context.Background(),
				id:         id,
				name:       name,
				notes:      notes,
				locationID: locationID,
				now:        now,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateItem(context.Background(), id, name, notes, locationID, now).Return(true, nil)
			},
			assertResult: func(t *testing.T, found bool) { require.True(t, found) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "an unknown id reports not found",
			args: args{
				ctx:        context.Background(),
				id:         id,
				name:       name,
				notes:      notes,
				locationID: locationID,
				now:        now,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateItem(context.Background(), id, name, notes, locationID, now).Return(false, nil)
			},
			assertResult: func(t *testing.T, found bool) { require.False(t, found) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a storage error is propagated",
			args: args{
				ctx:        context.Background(),
				id:         id,
				name:       name,
				notes:      notes,
				locationID: locationID,
				now:        now,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateItem(context.Background(), id, name, notes, locationID, now).Return(false, errStub)
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

			found, err := r.Update(tt.args.ctx, port.ItemUpdateRequest{
				ID: tt.args.id, Name: tt.args.name, Notes: tt.args.notes,
				LocationID: tt.args.locationID, Now: tt.args.now,
			})
			tt.assertErr(t, err)
			tt.assertResult(t, found)
		})
	}
}

// TestRepository_ReplaceImages covers the swap and the previously-stored urls it reports back as no
// longer referenced.
func TestRepository_ReplaceImages(t *testing.T) {
	t.Parallel()

	itemID := fakeID()

	type args struct {
		ctx    context.Context
		itemID uint64
		images []string
	}

	type testCase struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage)
		assertResult func(t *testing.T, got []string)
		assertErr    func(t *testing.T, err error)
	}

	var tests []testCase

	{ // a photo dropped from the new set is reported as removed, kept ones are not
		kept, gone := fakeURL(), fakeURL()
		images := []string{kept}

		tests = append(tests, testCase{
			name: "a photo dropped from the new set is reported as removed, kept ones are not",
			args: args{
				ctx:    context.Background(),
				itemID: itemID,
				images: images,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().ItemImageURLs(context.Background(), itemID).Return([]string{
					kept,
					gone,
				}, nil)
				m.EXPECT().ReplaceItemImages(context.Background(), itemID, images).Return(nil)
			},
			assertResult: func(
				t *testing.T, got []string,
			) {
				require.Equal(t, []string{gone}, got)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		})
	}

	{ // nothing is reported removed when the new set matches the old one exactly
		urls := []string{
			fakeURL(),
			fakeURL(),
		}

		tests = append(tests, testCase{
			name: "nothing removed when the new set matches the old one",
			args: args{
				ctx:    context.Background(),
				itemID: itemID,
				images: urls,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().ItemImageURLs(context.Background(), itemID).Return(urls, nil)
				m.EXPECT().ReplaceItemImages(context.Background(), itemID, urls).Return(nil)
			},
			assertResult: func(t *testing.T, got []string) { require.Empty(t, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		})
	}

	{ // a lookup failure aborts before the swap is attempted
		tests = append(tests, testCase{
			name: "a lookup failure aborts before the swap is attempted",
			args: args{
				ctx:    context.Background(),
				itemID: itemID,
				images: []string{fakeURL()},
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().ItemImageURLs(context.Background(), itemID).Return(nil, errStub)
				// ReplaceItemImages deliberately left unstubbed.
			},
			assertResult: func(t *testing.T, got []string) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		})
	}

	{ // a swap failure is propagated
		old := []string{fakeURL()}
		images := []string{fakeURL()}

		tests = append(tests, testCase{
			name: "a swap failure is propagated",
			args: args{
				ctx:    context.Background(),
				itemID: itemID,
				images: images,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().ItemImageURLs(context.Background(), itemID).Return(old, nil)
				m.EXPECT().ReplaceItemImages(context.Background(), itemID, images).Return(errStub)
			},
			assertResult: func(t *testing.T, got []string) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			tt.mock(m)

			got, err := r.ReplaceImages(tt.args.ctx, tt.args.itemID, tt.args.images)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

// TestRepository_Delete covers the best-effort photo lookup ahead of the delete: a failure there
// must never block it, but the returned urls are only meaningful when the lookup succeeded.
func TestRepository_Delete(t *testing.T) {
	t.Parallel()

	id := fakeID()

	type args struct {
		ctx context.Context
		id  uint64
	}

	type testCase struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage)
		assertResult func(t *testing.T, urls []string, found bool)
		assertErr    func(t *testing.T, err error)
	}

	var tests []testCase

	{
		urls := []string{
			fakeURL(),
			fakeURL(),
		}

		tests = append(tests, testCase{
			name: "deletes the item and returns the photo urls it owned",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().ItemImageURLs(context.Background(), id).Return(urls, nil)
				m.EXPECT().DeleteItem(context.Background(), id).Return(true, nil)
			},
			assertResult: func(t *testing.T, gotURLs []string, found bool) {
				require.Equal(t, urls, gotURLs)
				require.True(t, found)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		})
	}

	{
		tests = append(tests, testCase{
			name: "a failed photo lookup doesn't block the delete",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().ItemImageURLs(context.Background(), id).Return(nil, errStub)
				m.EXPECT().DeleteItem(context.Background(), id).Return(true, nil)
			},
			assertResult: func(t *testing.T, gotURLs []string, found bool) {
				require.Nil(t, gotURLs)
				require.True(t, found)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		})
	}

	{
		urls := []string{fakeURL()}

		tests = append(tests, testCase{
			name: "an unknown id reports not found, with no urls",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().ItemImageURLs(context.Background(), id).Return(urls, nil)
				m.EXPECT().DeleteItem(context.Background(), id).Return(false, nil)
			},
			assertResult: func(t *testing.T, gotURLs []string, found bool) {
				require.Nil(t, gotURLs)
				require.False(t, found)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		})
	}

	{
		tests = append(tests, testCase{
			name: "a delete error is propagated",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().ItemImageURLs(context.Background(), id).Return(nil, nil)
				m.EXPECT().DeleteItem(context.Background(), id).Return(false, errStub)
			},
			assertResult: func(t *testing.T, gotURLs []string, found bool) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			tt.mock(m)

			urls, found, err := r.Delete(tt.args.ctx, tt.args.id)
			tt.assertErr(t, err)
			tt.assertResult(t, urls, found)
		})
	}
}

// TestRepository_CountByLocation covers the plain delegation to storage.
func TestRepository_CountByLocation(t *testing.T) {
	t.Parallel()

	locationID := fakeID()
	want := gofakeit.Number(0, 500)

	type args struct {
		ctx        context.Context
		locationID uint64
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
				ctx:        context.Background(),
				locationID: locationID,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().CountItemsByLocation(context.Background(), locationID).Return(want, nil)
			},
			assertResult: func(t *testing.T, got int) { require.Equal(t, want, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a storage error is propagated",
			args: args{
				ctx:        context.Background(),
				locationID: locationID,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().CountItemsByLocation(context.Background(), locationID).Return(0, errStub)
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

			got, err := r.CountByLocation(tt.args.ctx, tt.args.locationID)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

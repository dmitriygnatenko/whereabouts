package item

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
	tests := []struct {
		name    string
		mock    func(m *mocks.MockStorage) []entity.Item
		wantErr error
	}{
		{
			name: "no items skips the image lookup entirely",
			mock: func(m *mocks.MockStorage) []entity.Item {
				m.EXPECT().ListItems(context.Background()).Return([]model.Item{}, nil)
				// ListItemImages deliberately left unstubbed: an empty id list must never reach it.

				return []entity.Item{}
			},
		},
		{
			name: "items are returned with their photos attached, a missing id becomes an empty slice",
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
		},
		{
			name: "a ListItems error is propagated",
			mock: func(m *mocks.MockStorage) []entity.Item {
				m.EXPECT().ListItems(context.Background()).Return(nil, errStub)

				return nil
			},
			wantErr: errStub,
		},
		{
			name: "a ListItemImages error is propagated",
			mock: func(m *mocks.MockStorage) []entity.Item {
				it := fakeItemModel()
				m.EXPECT().ListItems(context.Background()).Return([]model.Item{it}, nil)
				m.EXPECT().ListItemImages(context.Background(), []uint64{it.ID}).Return(nil, errStub)

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

// TestRepository_GetByID covers the single-item fetch with its photos attached, and the id ->
// NotFoundError translation.
func TestRepository_GetByID(t *testing.T) {
	id := fakeID()

	tests := []struct {
		name         string
		mock         func(m *mocks.MockStorage) entity.Item
		wantErr      error
		wantNotFound bool
	}{
		{
			name: "attaches the item's photos",
			mock: func(m *mocks.MockStorage) entity.Item {
				it := fakeItemModel()
				it.ID = id
				photos := []string{fakeURL()}

				m.EXPECT().FindItemByID(context.Background(), id).Return(it, nil)
				m.EXPECT().ItemImageURLs(context.Background(), id).Return(photos, nil)

				return it.ToEntity(photos)
			},
		},
		{
			name: "an unknown id becomes a NotFoundError, without looking up photos",
			mock: func(m *mocks.MockStorage) entity.Item {
				m.EXPECT().FindItemByID(context.Background(), id).Return(model.Item{}, sql.ErrNoRows)
				// ItemImageURLs deliberately left unstubbed.

				return entity.Item{}
			},
			wantNotFound: true,
		},
		{
			name: "a FindItemByID error is propagated",
			mock: func(m *mocks.MockStorage) entity.Item {
				m.EXPECT().FindItemByID(context.Background(), id).Return(model.Item{}, errStub)

				return entity.Item{}
			},
			wantErr: errStub,
		},
		{
			name: "an ItemImageURLs error is propagated",
			mock: func(m *mocks.MockStorage) entity.Item {
				it := fakeItemModel()
				it.ID = id
				m.EXPECT().FindItemByID(context.Background(), id).Return(it, nil)
				m.EXPECT().ItemImageURLs(context.Background(), id).Return(nil, errStub)

				return entity.Item{}
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			want := tt.mock(m)

			got, err := r.GetByID(context.Background(), id)
			if tt.wantNotFound {
				var notFound *domainerror.NotFoundError
				if !errors.As(err, &notFound) {
					t.Fatalf("GetByID() error = %v, want *domainerror.NotFoundError", err)
				}

				return
			}

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("GetByID() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("GetByID() error = %v", err)
			}

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("GetByID() = %+v, want %+v", got, want)
			}
		})
	}
}

// TestRepository_Create covers the plain delegation to storage.
func TestRepository_Create(t *testing.T) {
	name, notes := fakeName(), fakeNotes()
	locationID := fakeID()
	now := fakeNow()
	wantID := fakeID()

	tests := []struct {
		name    string
		mock    func(m *mocks.MockStorage)
		wantErr error
	}{
		{
			name: "delegates to storage and returns the new id",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().CreateItem(context.Background(), name, notes, locationID, now).Return(wantID, nil)
			},
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().CreateItem(context.Background(), name, notes, locationID, now).Return(uint64(0), errStub)
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			tt.mock(m)

			got, err := r.Create(context.Background(), name, notes, locationID, now)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Create() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}

			if got != wantID {
				t.Fatalf("Create() = %d, want %d", got, wantID)
			}
		})
	}
}

// TestRepository_Update covers the edit, whose affected-rows count decides "found".
func TestRepository_Update(t *testing.T) {
	id, locationID := fakeID(), fakeID()
	name, notes := fakeName(), fakeNotes()
	now := fakeNow()

	tests := []struct {
		name      string
		mock      func(m *mocks.MockStorage)
		wantFound bool
		wantErr   error
	}{
		{
			name: "rewrites the row",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateItem(context.Background(), id, name, notes, locationID, now).Return(true, nil)
			},
			wantFound: true,
		},
		{
			name: "an unknown id reports not found",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateItem(context.Background(), id, name, notes, locationID, now).Return(false, nil)
			},
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateItem(context.Background(), id, name, notes, locationID, now).Return(false, errStub)
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			tt.mock(m)

			found, err := r.Update(context.Background(), id, name, notes, locationID, now)
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

// TestRepository_ReplaceImages covers the swap and the previously-stored urls it reports back as no
// longer referenced.
func TestRepository_ReplaceImages(t *testing.T) {
	itemID := fakeID()

	type testCase struct {
		name    string
		images  []string
		mock    func(m *mocks.MockStorage)
		want    []string
		wantErr error
	}

	var tests []testCase

	{ // a photo dropped from the new set is reported as removed, kept ones are not
		kept, gone := fakeURL(), fakeURL()
		images := []string{kept}

		tests = append(tests, testCase{
			name:   "a photo dropped from the new set is reported as removed, kept ones are not",
			images: images,
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().ItemImageURLs(context.Background(), itemID).Return([]string{
					kept,
					gone,
				}, nil)
				m.EXPECT().ReplaceItemImages(context.Background(), itemID, images).Return(nil)
			},
			want: []string{gone},
		})
	}

	{ // nothing is reported removed when the new set matches the old one exactly
		urls := []string{
			fakeURL(),
			fakeURL(),
		}

		tests = append(tests, testCase{
			name:   "nothing removed when the new set matches the old one",
			images: urls,
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().ItemImageURLs(context.Background(), itemID).Return(urls, nil)
				m.EXPECT().ReplaceItemImages(context.Background(), itemID, urls).Return(nil)
			},
		})
	}

	{ // a lookup failure aborts before the swap is attempted
		tests = append(tests, testCase{
			name:   "a lookup failure aborts before the swap is attempted",
			images: []string{fakeURL()},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().ItemImageURLs(context.Background(), itemID).Return(nil, errStub)
				// ReplaceItemImages deliberately left unstubbed.
			},
			wantErr: errStub,
		})
	}

	{ // a swap failure is propagated
		old := []string{fakeURL()}
		images := []string{fakeURL()}

		tests = append(tests, testCase{
			name:   "a swap failure is propagated",
			images: images,
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().ItemImageURLs(context.Background(), itemID).Return(old, nil)
				m.EXPECT().ReplaceItemImages(context.Background(), itemID, images).Return(errStub)
			},
			wantErr: errStub,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			tt.mock(m)

			got, err := r.ReplaceImages(context.Background(), itemID, tt.images)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ReplaceImages() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ReplaceImages() error = %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ReplaceImages() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRepository_Delete covers the best-effort photo lookup ahead of the delete: a failure there
// must never block it, but the returned urls are only meaningful when the lookup succeeded.
func TestRepository_Delete(t *testing.T) {
	id := fakeID()

	type testCase struct {
		name      string
		mock      func(m *mocks.MockStorage)
		wantURLs  []string
		wantFound bool
		wantErr   error
	}

	var tests []testCase

	{
		urls := []string{
			fakeURL(),
			fakeURL(),
		}
		tests = append(tests, testCase{
			name: "deletes the item and returns the photo urls it owned",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().ItemImageURLs(context.Background(), id).Return(urls, nil)
				m.EXPECT().DeleteItem(context.Background(), id).Return(true, nil)
			},
			wantURLs:  urls,
			wantFound: true,
		})
	}

	{
		tests = append(tests, testCase{
			name: "a failed photo lookup doesn't block the delete",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().ItemImageURLs(context.Background(), id).Return(nil, errStub)
				m.EXPECT().DeleteItem(context.Background(), id).Return(true, nil)
			},
			wantFound: true,
		})
	}

	{
		urls := []string{fakeURL()}
		tests = append(tests, testCase{
			name: "an unknown id reports not found, with no urls",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().ItemImageURLs(context.Background(), id).Return(urls, nil)
				m.EXPECT().DeleteItem(context.Background(), id).Return(false, nil)
			},
		})
	}

	{
		tests = append(tests, testCase{
			name: "a delete error is propagated",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().ItemImageURLs(context.Background(), id).Return(nil, nil)
				m.EXPECT().DeleteItem(context.Background(), id).Return(false, errStub)
			},
			wantErr: errStub,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			tt.mock(m)

			urls, found, err := r.Delete(context.Background(), id)
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
				t.Fatalf("Delete() found = %v, want %v", found, tt.wantFound)
			}

			if !reflect.DeepEqual(urls, tt.wantURLs) {
				t.Fatalf("Delete() urls = %v, want %v", urls, tt.wantURLs)
			}
		})
	}
}

// TestRepository_CountByLocation covers the plain delegation to storage.
func TestRepository_CountByLocation(t *testing.T) {
	locationID := fakeID()
	want := gofakeit.Number(0, 500)

	tests := []struct {
		name    string
		mock    func(m *mocks.MockStorage)
		want    int
		wantErr error
	}{
		{
			name: "delegates to storage",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().CountItemsByLocation(context.Background(), locationID).Return(want, nil)
			},
			want: want,
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().CountItemsByLocation(context.Background(), locationID).Return(0, errStub)
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			tt.mock(m)

			got, err := r.CountByLocation(context.Background(), locationID)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("CountByLocation() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("CountByLocation() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf("CountByLocation() = %d, want %d", got, tt.want)
			}
		})
	}
}

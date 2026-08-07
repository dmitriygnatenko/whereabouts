package update

import (
	"context"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"wherewhat/internal/domain/entity"
	"wherewhat/internal/port"
	"wherewhat/internal/port/mocks"
)

// hasIDAndLocationID matches an ItemUpdateRequest with the given ID and LocationID, ignoring every
// other field.
func hasIDAndLocationID(id, locationID uint64) gomock.Matcher {
	return gomock.Cond(func(r port.ItemUpdateRequest) bool {
		return r.ID == id && r.LocationID == locationID
	})
}

// deps bundles the four mocks a UseCase depends on.
type deps struct {
	items     *mocks.MockItemRepository
	locations *mocks.MockLocationRepository
	storage   *mocks.MockImageStorage
	processor *mocks.MockImageProcessor
}

// newUseCase returns a UseCase wired to fresh mocks; any call a test doesn't stub via EXPECT()
// fails it.
func newUseCase(t *testing.T) (*UseCase, *deps) {
	t.Helper()

	mc := gomock.NewController(t)
	t.Cleanup(mc.Finish)

	d := &deps{
		items:     mocks.NewMockItemRepository(mc),
		locations: mocks.NewMockLocationRepository(mc),
		storage:   mocks.NewMockImageStorage(mc),
		processor: mocks.NewMockImageProcessor(mc),
	}

	return New(d.items, d.locations, d.storage, d.processor), d
}

// fakeID, fakeName, fakeNotes and fakeItemEntity are the field-shaped random values the tests below
// bind into mock expectations and returned rows, so a test failure is never masked by two cases
// accidentally sharing a fixture value.
func fakeID() uint64    { return uint64(gofakeit.Number(1, 1_000_000)) }
func fakeName() string  { return gofakeit.AppName() }
func fakeNotes() string { return gofakeit.Sentence() }

func fakeItemEntity() entity.Item {
	return entity.Item{
		ID:         fakeID(),
		Name:       fakeName(),
		LocationID: fakeID(),
		Notes:      fakeNotes(),
	}
}

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = gofakeit.Error()

// assertZeroOutput checks that Execute handed back a zero-value Output, as it does on every
// failure path.
func assertZeroOutput(t *testing.T, got Output) {
	t.Helper()

	require.Equal(t, Output{}, got)
}

// TestUseCase_Execute covers validation, the location existence check, photo processing, the
// not-found case, deletion of images ReplaceImages reports as no longer referenced, and the happy
// path.
func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	updatedItem := fakeItemEntity()

	tests := []struct {
		name string
		// mock arranges expectations on the mocks and returns the Input to execute.
		mock         func(d *deps) Input
		assertResult func(t *testing.T, got Output)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "blank name fails validation before any lookup",
			mock: func(d *deps) Input {
				return Input{
					ID:         fakeID(),
					Name:       "",
					LocationID: fakeID(),
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Enter the item name")
			},
		},
		{
			name: "zero location id fails validation",
			mock: func(d *deps) Input {
				return Input{
					ID:         fakeID(),
					Name:       fakeName(),
					LocationID: 0,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Choose a location")
			},
		},
		{
			name: "location existence check storage error",
			mock: func(d *deps) Input {
				locationID := fakeID()
				d.locations.EXPECT().Exists(gomock.Any(), locationID).Return(false, errStub)

				return Input{
					ID:         fakeID(),
					Name:       fakeName(),
					LocationID: locationID,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to verify location")
			},
		},
		{
			name: "missing location is rejected",
			mock: func(d *deps) Input {
				locationID := fakeID()
				d.locations.EXPECT().Exists(gomock.Any(), locationID).Return(false, nil)

				return Input{
					ID:         fakeID(),
					Name:       fakeName(),
					LocationID: locationID,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "The specified location was not found")
			},
		},
		{
			name: "malformed photo is rejected",
			mock: func(d *deps) Input {
				locationID := fakeID()
				d.locations.EXPECT().Exists(gomock.Any(), locationID).Return(true, nil)
				d.storage.EXPECT().IsStoredURL("not-a-data-url").Return(false)

				return Input{
					ID:         fakeID(),
					Name:       fakeName(),
					LocationID: locationID,
					Images:     []string{"not-a-data-url"},
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "does not look like a data URL")
			},
		},
		{
			name: "item update failure",
			mock: func(d *deps) Input {
				id, locationID := fakeID(), fakeID()
				d.locations.EXPECT().Exists(gomock.Any(), locationID).Return(true, nil)
				d.items.EXPECT().
					Update(gomock.Any(), hasIDAndLocationID(id, locationID)).
					Return(false, errStub)

				return Input{
					ID:         id,
					Name:       fakeName(),
					LocationID: locationID,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to update item")
			},
		},
		{
			name: "unknown item is not found",
			mock: func(d *deps) Input {
				id, locationID := fakeID(), fakeID()
				d.locations.EXPECT().Exists(gomock.Any(), locationID).Return(true, nil)
				d.items.EXPECT().
					Update(gomock.Any(), hasIDAndLocationID(id, locationID)).
					Return(false, nil)

				return Input{
					ID:         id,
					Name:       fakeName(),
					LocationID: locationID,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Record not found")
			},
		},
		{
			name: "replace images failure",
			mock: func(d *deps) Input {
				id, locationID := fakeID(), fakeID()
				d.locations.EXPECT().Exists(gomock.Any(), locationID).Return(true, nil)
				d.items.EXPECT().
					Update(gomock.Any(), hasIDAndLocationID(id, locationID)).
					Return(true, nil)
				d.items.EXPECT().ReplaceImages(gomock.Any(), id, gomock.Any()).Return(nil, errStub)

				return Input{
					ID:         id,
					Name:       fakeName(),
					LocationID: locationID,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to update photos")
			},
		},
		{
			name: "read back failure",
			mock: func(d *deps) Input {
				id, locationID := fakeID(), fakeID()
				d.locations.EXPECT().Exists(gomock.Any(), locationID).Return(true, nil)
				d.items.EXPECT().
					Update(gomock.Any(), hasIDAndLocationID(id, locationID)).
					Return(true, nil)
				d.items.EXPECT().ReplaceImages(gomock.Any(), id, gomock.Any()).Return(nil, nil)
				d.items.EXPECT().GetByID(gomock.Any(), id).Return(entity.Item{}, errStub)

				return Input{
					ID:         id,
					Name:       fakeName(),
					LocationID: locationID,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Item updated, but failed to read it back")
			},
		},
		{
			name: "trims fields, deletes images no longer referenced, and updates the item",
			mock: func(d *deps) Input {
				id, locationID := fakeID(), fakeID()
				name, notes := fakeName(), fakeNotes()
				removedURL := gofakeit.URL()

				d.locations.EXPECT().Exists(gomock.Any(), locationID).Return(true, nil)
				d.items.EXPECT().Update(gomock.Any(), gomock.Cond(func(r port.ItemUpdateRequest) bool {
					return r.ID == id && r.Name == name && r.Notes == notes && r.LocationID == locationID
				})).
					DoAndReturn(func(_ context.Context, req port.ItemUpdateRequest) (bool, error) {
						require.WithinDuration(t, time.Now().UTC(), req.Now, time.Minute)

						return true, nil
					})
				d.items.EXPECT().ReplaceImages(gomock.Any(), id, gomock.Any()).Return([]string{removedURL}, nil)
				d.storage.EXPECT().Delete(removedURL)
				d.items.EXPECT().GetByID(gomock.Any(), id).Return(updatedItem, nil)

				return Input{
					ID:         id,
					Name:       "  " + name + "  ",
					Notes:      "  " + notes + "  ",
					LocationID: locationID,
				}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, updatedItem, got.Item)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc, d := newUseCase(t)
			input := tt.mock(d)

			got, err := uc.Execute(context.Background(), input)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

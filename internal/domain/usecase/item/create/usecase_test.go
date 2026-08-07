package create

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

// hasLocationID matches an ItemCreateRequest with the given LocationID, ignoring every other field.
func hasLocationID(locationID uint64) gomock.Matcher {
	return gomock.Cond(func(r port.ItemCreateRequest) bool {
		return r.LocationID == locationID
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

// TestUseCase_Execute covers validation (including the combined-errors case), the location
// existence check, photo processing, the three repository writes, and the happy path.
func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	createdItem := fakeItemEntity()

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
			name: "blank name and missing location report both messages",
			mock: func(d *deps) Input {
				return Input{
					Name:       "   ",
					LocationID: 0,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Choose a location. Enter the item name.")
			},
		},
		{
			name: "location existence check storage error",
			mock: func(d *deps) Input {
				locationID := fakeID()
				d.locations.EXPECT().Exists(gomock.Any(), locationID).Return(false, errStub)

				return Input{
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
			name: "item creation failure",
			mock: func(d *deps) Input {
				locationID := fakeID()
				d.locations.EXPECT().Exists(gomock.Any(), locationID).Return(true, nil)
				d.items.EXPECT().Create(gomock.Any(), hasLocationID(locationID)).
					Return(uint64(0), errStub)

				return Input{
					Name:       fakeName(),
					LocationID: locationID,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to save item")
			},
		},
		{
			name: "replace images failure",
			mock: func(d *deps) Input {
				locationID, id := fakeID(), fakeID()
				d.locations.EXPECT().Exists(gomock.Any(), locationID).Return(true, nil)
				d.items.EXPECT().Create(gomock.Any(), hasLocationID(locationID)).
					Return(id, nil)
				d.items.EXPECT().ReplaceImages(gomock.Any(), id, gomock.Any()).Return(nil, errStub)

				return Input{
					Name:       fakeName(),
					LocationID: locationID,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to save photos")
			},
		},
		{
			name: "read back failure",
			mock: func(d *deps) Input {
				locationID, id := fakeID(), fakeID()
				d.locations.EXPECT().Exists(gomock.Any(), locationID).Return(true, nil)
				d.items.EXPECT().Create(gomock.Any(), hasLocationID(locationID)).
					Return(id, nil)
				d.items.EXPECT().ReplaceImages(gomock.Any(), id, gomock.Any()).Return(nil, nil)
				d.items.EXPECT().GetByID(gomock.Any(), id).Return(entity.Item{}, errStub)

				return Input{
					Name:       fakeName(),
					LocationID: locationID,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Item saved, but failed to read it back")
			},
		},
		{
			name: "trims name and notes, processes photos, and creates the item",
			mock: func(d *deps) Input {
				locationID, id := fakeID(), fakeID()
				name, notes := fakeName(), fakeNotes()
				storedURL := gofakeit.URL()

				d.locations.EXPECT().Exists(gomock.Any(), locationID).Return(true, nil)
				d.storage.EXPECT().IsStoredURL(storedURL).Return(true)
				d.items.EXPECT().Create(gomock.Any(), gomock.Cond(func(r port.ItemCreateRequest) bool {
					return r.Name == name && r.Notes == notes && r.LocationID == locationID
				})).
					DoAndReturn(func(_ context.Context, req port.ItemCreateRequest) (uint64, error) {
						require.WithinDuration(t, time.Now().UTC(), req.Now, time.Minute)

						return id, nil
					})
				d.items.EXPECT().ReplaceImages(gomock.Any(), id, []string{storedURL}).Return(nil, nil)
				d.items.EXPECT().GetByID(gomock.Any(), id).Return(createdItem, nil)

				return Input{
					Name:       "  " + name + "  ",
					Notes:      "  " + notes + "  ",
					LocationID: locationID,
					Images:     []string{storedURL},
				}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, createdItem, got.Item)
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

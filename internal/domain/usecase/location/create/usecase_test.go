package create

import (
	"context"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"wherewhat/internal/domain/entity"
	"wherewhat/internal/domain/usecase"
	"wherewhat/internal/port"
	"wherewhat/internal/port/mocks"
)

// hasParentID matches a LocationCreateRequest whose ParentID points to the same value as parentID,
// ignoring every other field.
func hasParentID(parentID uint64) gomock.Matcher {
	return gomock.Cond(func(r port.LocationCreateRequest) bool {
		return r.ParentID != nil && *r.ParentID == parentID
	})
}

// hasNameColorParent matches a LocationCreateRequest with the given name, color, and parent id
// (nil if parentID is nil), ignoring every other field.
func hasNameColorParent(name, color string, parentID *uint64) gomock.Matcher {
	return gomock.Cond(func(r port.LocationCreateRequest) bool {
		if r.Name != name || r.Color != color {
			return false
		}

		if parentID == nil {
			return r.ParentID == nil
		}

		return r.ParentID != nil && *r.ParentID == *parentID
	})
}

// newUseCase returns a UseCase wired to a fresh MockLocationRepository; any call a test doesn't
// stub via EXPECT() fails it.
func newUseCase(t *testing.T) (*UseCase, *mocks.MockLocationRepository) {
	t.Helper()

	mc := gomock.NewController(t)
	t.Cleanup(mc.Finish)

	locations := mocks.NewMockLocationRepository(mc)

	return New(locations), locations
}

// fakeID, fakeName and fakeColor are the field-shaped random values the tests below bind into mock
// expectations and returned rows, so a test failure is never masked by two cases accidentally
// sharing a fixture value.
func fakeID() uint64    { return uint64(gofakeit.Number(1, 1_000_000)) }
func fakeName() string  { return gofakeit.City() }
func fakeColor() string { return gofakeit.HexColor() }

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = gofakeit.Error()

// assertZeroOutput checks that Execute handed back a zero-value Output, as it does on every
// failure path.
func assertZeroOutput(t *testing.T, got Output) {
	t.Helper()

	require.Equal(t, Output{}, got)
}

// TestUseCase_Execute covers validation, the parent-existence check (including the "not a positive
// id means no parent" quirk), color defaulting, and the happy path.
func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	zeroParentLoc := entity.Location{ID: fakeID(), Name: fakeName(), Color: usecase.DefaultColor}
	trimmedLoc := entity.Location{ID: fakeID(), Name: fakeName(), Color: usecase.DefaultColor}
	nestedParentID := fakeID()
	nestedLoc := entity.Location{ID: fakeID(), Name: fakeName(), Color: fakeColor(), ParentID: &nestedParentID}

	tests := []struct {
		name string
		// mock arranges expectations on the repository and returns the Input to execute.
		mock         func(locations *mocks.MockLocationRepository) Input
		assertResult func(t *testing.T, got Output)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "blank name fails validation before any lookup",
			mock: func(locations *mocks.MockLocationRepository) Input {
				return Input{Name: ""}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Please enter a location name")
			},
		},
		{
			name: "parent existence check storage error",
			mock: func(locations *mocks.MockLocationRepository) Input {
				parentID := fakeID()
				locations.EXPECT().Exists(gomock.Any(), parentID).Return(false, errStub)

				return Input{Name: fakeName(), ParentID: &parentID}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to verify parent location")
			},
		},
		{
			name: "missing parent is rejected",
			mock: func(locations *mocks.MockLocationRepository) Input {
				parentID := fakeID()
				locations.EXPECT().Exists(gomock.Any(), parentID).Return(false, nil)

				return Input{Name: fakeName(), ParentID: &parentID}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Parent location not found")
			},
		},
		{
			name: "location creation failure",
			mock: func(locations *mocks.MockLocationRepository) Input {
				parentID := fakeID()
				locations.EXPECT().Exists(gomock.Any(), parentID).Return(true, nil)
				locations.EXPECT().
					Create(gomock.Any(), hasParentID(parentID)).
					Return(entity.Location{}, errStub)

				return Input{Name: fakeName(), ParentID: &parentID}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to save location")
			},
		},
		{
			name: "a zero parent id is treated as no parent",
			mock: func(locations *mocks.MockLocationRepository) Input {
				zero := uint64(0)
				locations.EXPECT().
					Create(gomock.Any(), hasNameColorParent(zeroParentLoc.Name, usecase.DefaultColor, nil)).
					Return(zeroParentLoc, nil)

				return Input{Name: zeroParentLoc.Name, ParentID: &zero}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, zeroParentLoc, got.Location)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "trims the name and defaults a blank color",
			mock: func(locations *mocks.MockLocationRepository) Input {
				locations.EXPECT().
					Create(gomock.Any(), hasNameColorParent(trimmedLoc.Name, usecase.DefaultColor, nil)).
					DoAndReturn(func(_ context.Context, req port.LocationCreateRequest) (entity.Location, error) {
						require.WithinDuration(t, time.Now().UTC(), req.Now, time.Minute)

						return trimmedLoc, nil
					})

				return Input{Name: "  " + trimmedLoc.Name + "  ", Color: "  "}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, trimmedLoc, got.Location)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "creates a location nested under an existing parent",
			mock: func(locations *mocks.MockLocationRepository) Input {
				locations.EXPECT().Exists(gomock.Any(), nestedParentID).Return(true, nil)
				locations.EXPECT().
					Create(gomock.Any(), hasNameColorParent(nestedLoc.Name, nestedLoc.Color, &nestedParentID)).
					Return(nestedLoc, nil)

				return Input{Name: nestedLoc.Name, Color: nestedLoc.Color, ParentID: &nestedParentID}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, nestedLoc, got.Location)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc, locations := newUseCase(t)
			input := tt.mock(locations)

			got, err := uc.Execute(context.Background(), input)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

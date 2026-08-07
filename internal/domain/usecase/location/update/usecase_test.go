package update

import (
	"context"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"wherewhat/internal/domain/usecase"
	"wherewhat/internal/port"
	"wherewhat/internal/port/mocks"
)

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

// TestUseCase_Execute covers validation, color defaulting, the not-found case, the read-back
// failure, and the happy path.
func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	trimmedID := fakeID()
	trimmedName := fakeName()

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
				return Input{
					ID:   fakeID(),
					Name: "",
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Please enter a location name")
			},
		},
		{
			name: "update failure is reported",
			mock: func(locations *mocks.MockLocationRepository) Input {
				id, name := fakeID(), fakeName()
				locations.EXPECT().
					Update(gomock.Any(), port.LocationUpdateRequest{ID: id, Name: name, Color: usecase.DefaultColor}).
					Return(false, errStub)

				return Input{
					ID:   id,
					Name: name,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to update location")
			},
		},
		{
			name: "unknown location is not found",
			mock: func(locations *mocks.MockLocationRepository) Input {
				id, name := fakeID(), fakeName()
				locations.EXPECT().
					Update(gomock.Any(), port.LocationUpdateRequest{ID: id, Name: name, Color: usecase.DefaultColor}).
					Return(false, nil)

				return Input{
					ID:   id,
					Name: name,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Location not found")
			},
		},
		{
			name: "read back failure",
			mock: func(locations *mocks.MockLocationRepository) Input {
				id, name := fakeID(), fakeName()
				locations.EXPECT().
					Update(gomock.Any(), port.LocationUpdateRequest{ID: id, Name: name, Color: usecase.DefaultColor}).
					Return(true, nil)
				locations.EXPECT().ParentID(gomock.Any(), id).Return(nil, errStub)

				return Input{
					ID:   id,
					Name: name,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Location updated, but failed to read it back")
			},
		},
		{
			name: "trims the name, defaults a blank color, and returns the parent",
			mock: func(locations *mocks.MockLocationRepository) Input {
				parentID := fakeID()
				locations.EXPECT().
					Update(gomock.Any(), port.LocationUpdateRequest{
						ID: trimmedID, Name: trimmedName, Color: usecase.DefaultColor,
					}).
					Return(true, nil)
				locations.EXPECT().ParentID(gomock.Any(), trimmedID).Return(&parentID, nil)

				return Input{
					ID:    trimmedID,
					Name:  "  " + trimmedName + "  ",
					Color: " ",
				}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, trimmedID, got.Location.ID)
				require.Equal(t, trimmedName, got.Location.Name)
				require.Equal(t, usecase.DefaultColor, got.Location.Color)
				require.NotNil(t, got.Location.ParentID)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "updates name and color, keeping the parent unchanged",
			mock: func(locations *mocks.MockLocationRepository) Input {
				id, name, color := fakeID(), fakeName(), fakeColor()
				locations.EXPECT().
					Update(gomock.Any(), port.LocationUpdateRequest{ID: id, Name: name, Color: color}).
					Return(true, nil)
				locations.EXPECT().ParentID(gomock.Any(), id).Return(nil, nil)

				return Input{
					ID:    id,
					Name:  name,
					Color: color,
				}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Nil(t, got.Location.ParentID)
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

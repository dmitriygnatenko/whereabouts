package list

import (
	"context"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"wherewhat/internal/domain/entity"
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

func fakeID() uint64   { return uint64(gofakeit.Number(1, 1_000_000)) }
func fakeName() string { return gofakeit.City() }

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = gofakeit.Error()

// TestUseCase_Execute covers a repository failure, the nil-to-empty-slice normalization, and
// returning every location as-is.
func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// mock arranges expectations on the repository.
		mock         func(locations *mocks.MockLocationRepository)
		assertResult func(t *testing.T, got Output)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "storage error is reported",
			mock: func(locations *mocks.MockLocationRepository) {
				locations.EXPECT().List(gomock.Any()).Return(nil, errStub)
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, Output{}, got)
			},
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to load locations")
			},
		},
		{
			name: "nil result becomes an empty slice",
			mock: func(locations *mocks.MockLocationRepository) {
				locations.EXPECT().List(gomock.Any()).Return(nil, nil)
			},
			assertResult: func(t *testing.T, got Output) {
				require.NotNil(t, got.Locations)
				require.Empty(t, got.Locations)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "returns every location",
			mock: func(locations *mocks.MockLocationRepository) {
				locations.EXPECT().List(gomock.Any()).Return([]entity.Location{
					{ID: fakeID(), Name: fakeName()},
					{ID: fakeID(), Name: fakeName()},
				}, nil)
			},
			assertResult: func(t *testing.T, got Output) {
				require.Len(t, got.Locations, 2)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc, locations := newUseCase(t)
			tt.mock(locations)

			got, err := uc.Execute(context.Background())
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

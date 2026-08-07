package delete

import (
	"context"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/port/mocks"
)

// deps bundles the two mocks a UseCase depends on.
type deps struct {
	locations *mocks.MockLocationRepository
	items     *mocks.MockItemRepository
}

// newUseCase returns a UseCase wired to fresh mocks; any call a test doesn't stub via EXPECT()
// fails it.
func newUseCase(t *testing.T) (*UseCase, *deps) {
	t.Helper()

	mc := gomock.NewController(t)
	t.Cleanup(mc.Finish)

	d := &deps{
		locations: mocks.NewMockLocationRepository(mc),
		items:     mocks.NewMockItemRepository(mc),
	}

	return New(d.locations, d.items), d
}

func fakeID() uint64 { return uint64(gofakeit.Number(1, 1_000_000)) }

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = gofakeit.Error()

// TestUseCase_Execute covers the nested-locations guard, the items-in-location guard, a storage
// failure at each step, the not-found case, and the happy path.
func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// mock arranges expectations on the mocks and returns the Input to execute.
		mock      func(d *deps) Input
		assertErr func(t *testing.T, err error)
	}{
		{
			name: "nested-locations check storage error",
			mock: func(d *deps) Input {
				id := fakeID()
				d.locations.EXPECT().ChildCount(gomock.Any(), id).Return(0, errStub)

				return Input{ID: id}
			},
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to check nested locations")
			},
		},
		{
			name: "refuses to delete a location with nested locations",
			mock: func(d *deps) Input {
				id := fakeID()
				d.locations.EXPECT().ChildCount(gomock.Any(), id).Return(1, nil)

				return Input{ID: id}
			},
			assertErr: func(t *testing.T, err error) {
				var conflict *domainerror.ConflictError

				require.ErrorAs(t, err, &conflict)
				require.Equal(t, "This location has nested locations — delete or move them first", conflict.Message)
			},
		},
		{
			name: "items-in-location check storage error",
			mock: func(d *deps) Input {
				id := fakeID()
				d.locations.EXPECT().ChildCount(gomock.Any(), id).Return(0, nil)
				d.items.EXPECT().CountByLocation(gomock.Any(), id).Return(0, errStub)

				return Input{ID: id}
			},
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to check items in this location")
			},
		},
		{
			name: "refuses to delete a location with items",
			mock: func(d *deps) Input {
				id := fakeID()
				d.locations.EXPECT().ChildCount(gomock.Any(), id).Return(0, nil)
				d.items.EXPECT().CountByLocation(gomock.Any(), id).Return(1, nil)

				return Input{ID: id}
			},
			assertErr: func(t *testing.T, err error) {
				var conflict *domainerror.ConflictError

				require.ErrorAs(t, err, &conflict)
				require.Equal(t, "This location has items in it — move them elsewhere first", conflict.Message)
			},
		},
		{
			name: "delete failure is reported",
			mock: func(d *deps) Input {
				id := fakeID()
				d.locations.EXPECT().ChildCount(gomock.Any(), id).Return(0, nil)
				d.items.EXPECT().CountByLocation(gomock.Any(), id).Return(0, nil)
				d.locations.EXPECT().Delete(gomock.Any(), id).Return(false, errStub)

				return Input{ID: id}
			},
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to delete location")
			},
		},
		{
			name: "unknown location is not found",
			mock: func(d *deps) Input {
				id := fakeID()
				d.locations.EXPECT().ChildCount(gomock.Any(), id).Return(0, nil)
				d.items.EXPECT().CountByLocation(gomock.Any(), id).Return(0, nil)
				d.locations.EXPECT().Delete(gomock.Any(), id).Return(false, nil)

				return Input{ID: id}
			},
			assertErr: func(t *testing.T, err error) {
				var notFound *domainerror.NotFoundError

				require.ErrorAs(t, err, &notFound)
				require.Equal(t, "Location not found", notFound.Message)
			},
		},
		{
			name: "deletes an empty leaf location",
			mock: func(d *deps) Input {
				id := fakeID()
				d.locations.EXPECT().ChildCount(gomock.Any(), id).Return(0, nil)
				d.items.EXPECT().CountByLocation(gomock.Any(), id).Return(0, nil)
				d.locations.EXPECT().Delete(gomock.Any(), id).Return(true, nil)

				return Input{ID: id}
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc, d := newUseCase(t)
			input := tt.mock(d)

			err := uc.Execute(context.Background(), input)
			tt.assertErr(t, err)
		})
	}
}

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
	items   *mocks.MockItemRepository
	storage *mocks.MockImageStorage
}

// newUseCase returns a UseCase wired to fresh mocks; any call a test doesn't stub via EXPECT()
// fails it.
func newUseCase(t *testing.T) (*UseCase, *deps) {
	t.Helper()

	mc := gomock.NewController(t)
	t.Cleanup(mc.Finish)

	d := &deps{
		items:   mocks.NewMockItemRepository(mc),
		storage: mocks.NewMockImageStorage(mc),
	}

	return New(d.items, d.storage), d
}

func fakeID() uint64 { return uint64(gofakeit.Number(1, 1_000_000)) }

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = gofakeit.Error()

// TestUseCase_Execute covers a repository failure, a missing item, and the happy path where every
// photo the item owned gets deleted from storage.
func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// mock arranges expectations on the mocks and returns the Input to execute.
		mock      func(d *deps) Input
		assertErr func(t *testing.T, err error)
	}{
		{
			name: "delete failure is reported",
			mock: func(d *deps) Input {
				id := fakeID()
				d.items.EXPECT().Delete(gomock.Any(), id).Return(nil, false, errStub)

				return Input{ID: id}
			},
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to delete item")
			},
		},
		{
			name: "unknown item is not found",
			mock: func(d *deps) Input {
				id := fakeID()
				d.items.EXPECT().Delete(gomock.Any(), id).Return(nil, false, nil)

				return Input{ID: id}
			},
			assertErr: func(t *testing.T, err error) {
				var notFound *domainerror.NotFoundError

				require.ErrorAs(t, err, &notFound)
				require.Equal(t, "Record not found", notFound.Message)
			},
		},
		{
			name: "deletes every photo the item owned",
			mock: func(d *deps) Input {
				id := fakeID()
				urls := []string{gofakeit.URL(), gofakeit.URL()}
				d.items.EXPECT().Delete(gomock.Any(), id).Return(urls, true, nil)
				d.storage.EXPECT().Delete(urls[0])
				d.storage.EXPECT().Delete(urls[1])

				return Input{ID: id}
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "an item with no photos deletes cleanly",
			mock: func(d *deps) Input {
				id := fakeID()
				d.items.EXPECT().Delete(gomock.Any(), id).Return(nil, true, nil)

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

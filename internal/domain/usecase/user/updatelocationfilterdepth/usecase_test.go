package updatelocationfilterdepth

import (
	"context"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"wherewhat/internal/domain/entity"
	"wherewhat/internal/port/mocks"
)

// newUseCase returns a UseCase wired to a fresh MockUserRepository; any call a test doesn't stub
// via EXPECT() fails it.
func newUseCase(t *testing.T) (*UseCase, *mocks.MockUserRepository) {
	t.Helper()

	mc := gomock.NewController(t)
	t.Cleanup(mc.Finish)

	users := mocks.NewMockUserRepository(mc)

	return New(users), users
}

func fakeID() uint64 { return uint64(gofakeit.Number(1, 1_000_000)) }

func fakePublicUser() entity.PublicUser {
	return entity.PublicUser{ID: fakeID(), Username: gofakeit.Username()}
}

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = gofakeit.Error()

// TestUseCase_Execute covers rejecting a negative depth, accepting zero ("no limit"), a storage
// failure, and the happy path.
func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// mock arranges expectations on the repository and returns the Input to execute.
		mock         func(users *mocks.MockUserRepository) Input
		assertResult func(t *testing.T, got Output)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "negative depth is rejected",
			mock: func(users *mocks.MockUserRepository) Input {
				return Input{User: fakePublicUser(), Depth: -1}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, Output{}, got)
			},
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Location filter depth must be 0 (show all) or at least 1")
			},
		},
		{
			name: "save failure",
			mock: func(users *mocks.MockUserRepository) Input {
				user := fakePublicUser()
				users.EXPECT().UpdateLocationFilterDepth(gomock.Any(), user.ID, 3).Return(errStub)

				return Input{User: user, Depth: 3}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, Output{}, got)
			},
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to save location filter depth")
			},
		},
		{
			name: "zero means no limit and is accepted",
			mock: func(users *mocks.MockUserRepository) Input {
				user := fakePublicUser()
				users.EXPECT().UpdateLocationFilterDepth(gomock.Any(), user.ID, 0).Return(nil)

				return Input{User: user, Depth: 0}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, 0, got.User.LocationFilterDepth)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "saves a positive depth",
			mock: func(users *mocks.MockUserRepository) Input {
				user := fakePublicUser()
				users.EXPECT().UpdateLocationFilterDepth(gomock.Any(), user.ID, 5).Return(nil)

				return Input{User: user, Depth: 5}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, 5, got.User.LocationFilterDepth)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc, users := newUseCase(t)
			input := tt.mock(users)

			got, err := uc.Execute(context.Background(), input)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

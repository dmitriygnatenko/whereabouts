package updatelanguage

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
	return entity.PublicUser{
		ID:       fakeID(),
		Username: gofakeit.Username(),
	}
}

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = gofakeit.Error()

// TestUseCase_Execute covers rejecting an unsupported/blank language, a storage failure, and the
// happy path (including normalizing case and whitespace).
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
			name: "blank language is rejected",
			mock: func(users *mocks.MockUserRepository) Input {
				return Input{
					User:     fakePublicUser(),
					Language: "",
				}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, Output{}, got)
			},
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Unsupported language")
			},
		},
		{
			name: "unsupported language is rejected",
			mock: func(users *mocks.MockUserRepository) Input {
				return Input{
					User:     fakePublicUser(),
					Language: "xx",
				}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, Output{}, got)
			},
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Unsupported language")
			},
		},
		{
			name: "save failure",
			mock: func(users *mocks.MockUserRepository) Input {
				user := fakePublicUser()
				users.EXPECT().UpdateLanguage(gomock.Any(), user.ID, "ru").Return(errStub)

				return Input{
					User:     user,
					Language: "ru",
				}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, Output{}, got)
			},
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to save language preference")
			},
		},
		{
			name: "normalizes case and whitespace, then saves",
			mock: func(users *mocks.MockUserRepository) Input {
				user := fakePublicUser()
				users.EXPECT().UpdateLanguage(gomock.Any(), user.ID, "ru").Return(nil)

				return Input{
					User:     user,
					Language: "  RU  ",
				}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, "ru", got.User.Language)
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

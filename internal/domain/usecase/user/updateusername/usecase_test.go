package updateusername

import (
	"context"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"wherewhat/internal/domain/entity"
	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/usecase"
	"wherewhat/internal/port/mocks"
)

// deps bundles the two mocks a UseCase depends on.
type deps struct {
	users  *mocks.MockUserRepository
	hasher *mocks.MockPasswordHasher
}

// newUseCase returns a UseCase wired to fresh mocks; any call a test doesn't stub via EXPECT()
// fails it.
func newUseCase(t *testing.T) (*UseCase, *deps) {
	t.Helper()

	mc := gomock.NewController(t)
	t.Cleanup(mc.Finish)

	d := &deps{
		users:  mocks.NewMockUserRepository(mc),
		hasher: mocks.NewMockPasswordHasher(mc),
	}

	return New(d.users, d.hasher), d
}

func fakeID() uint64       { return uint64(gofakeit.Number(1, 1_000_000)) }
func fakeUsername() string { return gofakeit.Username() }
func fakePassword() string { return gofakeit.Password(true, true, true, false, false, 10) }
func fakeHash() string     { return gofakeit.LetterN(60) }

func fakePublicUser() entity.PublicUser {
	return entity.PublicUser{
		ID:       fakeID(),
		Username: fakeUsername(),
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

// TestUseCase_Execute covers validation, the current-password check (storage error and mismatch),
// the same-username short circuit, a duplicate-username conflict, a generic storage error, and the
// happy path.
func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	unchangedUser := fakePublicUser()
	unchangedUser.Username = usecase.NormalizeUsername(unchangedUser.Username)
	renamedUser := fakePublicUser()
	renamedTo := "  " + fakeUsername() + "  "
	normalizedRename := usecase.NormalizeUsername(renamedTo)

	tests := []struct {
		name string
		// mock arranges expectations on the mocks and returns the Input to execute.
		mock         func(d *deps) Input
		assertResult func(t *testing.T, got Output)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "empty new username fails validation before any lookup",
			mock: func(d *deps) Input {
				return Input{
					User:            fakePublicUser(),
					Username:        "",
					CurrentPassword: fakePassword(),
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Please enter a username")
			},
		},
		{
			name: "current password check storage error",
			mock: func(d *deps) Input {
				user := fakePublicUser()
				d.users.EXPECT().FindByID(gomock.Any(), user.ID).Return(entity.User{}, errStub)

				return Input{
					User:            user,
					Username:        fakeUsername(),
					CurrentPassword: fakePassword(),
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to verify current password")
			},
		},
		{
			name: "incorrect current password is forbidden",
			mock: func(d *deps) Input {
				user := fakePublicUser()
				current := fakePassword()
				stored := entity.User{
					ID:           user.ID,
					PasswordHash: fakeHash(),
				}
				d.users.EXPECT().FindByID(gomock.Any(), user.ID).Return(stored, nil)
				d.hasher.EXPECT().Compare(stored.PasswordHash, current).Return(false)

				return Input{
					User:            user,
					Username:        fakeUsername(),
					CurrentPassword: current,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				var forbidden *domainerror.ForbiddenError

				require.ErrorAs(t, err, &forbidden)
				require.Equal(t, "Incorrect current password", forbidden.Message)
			},
		},
		{
			name: "unchanged username short-circuits without a repository write",
			mock: func(d *deps) Input {
				current := fakePassword()
				stored := entity.User{
					ID:           unchangedUser.ID,
					PasswordHash: fakeHash(),
				}
				d.users.EXPECT().FindByID(gomock.Any(), unchangedUser.ID).Return(stored, nil)
				d.hasher.EXPECT().Compare(stored.PasswordHash, current).Return(true)

				return Input{
					User:            unchangedUser,
					Username:        unchangedUser.Username,
					CurrentPassword: current,
				}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, unchangedUser, got.User)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "duplicate username is reported as a conflict",
			mock: func(d *deps) Input {
				user := fakePublicUser()
				current, newUsername := fakePassword(), fakeUsername()
				normalized := usecase.NormalizeUsername(newUsername)
				stored := entity.User{
					ID:           user.ID,
					PasswordHash: fakeHash(),
				}
				conflict := &domainerror.ConflictError{Message: "username already exists"}
				d.users.EXPECT().FindByID(gomock.Any(), user.ID).Return(stored, nil)
				d.hasher.EXPECT().Compare(stored.PasswordHash, current).Return(true)
				d.users.EXPECT().UpdateUsername(gomock.Any(), user.ID, normalized).Return(conflict)

				return Input{
					User:            user,
					Username:        newUsername,
					CurrentPassword: current,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				var got *domainerror.ConflictError

				require.ErrorAs(t, err, &got)
				require.Equal(t, "username already exists", got.Message)
			},
		},
		{
			name: "generic save failure",
			mock: func(d *deps) Input {
				user := fakePublicUser()
				current, newUsername := fakePassword(), fakeUsername()
				normalized := usecase.NormalizeUsername(newUsername)
				stored := entity.User{
					ID:           user.ID,
					PasswordHash: fakeHash(),
				}
				d.users.EXPECT().FindByID(gomock.Any(), user.ID).Return(stored, nil)
				d.hasher.EXPECT().Compare(stored.PasswordHash, current).Return(true)
				d.users.EXPECT().UpdateUsername(gomock.Any(), user.ID, normalized).Return(errStub)

				return Input{
					User:            user,
					Username:        newUsername,
					CurrentPassword: current,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to update username")
			},
		},
		{
			name: "updates the username after confirming the current password",
			mock: func(d *deps) Input {
				current := fakePassword()
				stored := entity.User{
					ID:           renamedUser.ID,
					PasswordHash: fakeHash(),
				}
				d.users.EXPECT().FindByID(gomock.Any(), renamedUser.ID).Return(stored, nil)
				d.hasher.EXPECT().Compare(stored.PasswordHash, current).Return(true)
				d.users.EXPECT().UpdateUsername(gomock.Any(), renamedUser.ID, normalizedRename).Return(nil)

				return Input{
					User:            renamedUser,
					Username:        renamedTo,
					CurrentPassword: current,
				}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, normalizedRename, got.User.Username)
				require.Equal(t, renamedUser.ID, got.User.ID)
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

package changepassword

import (
	"context"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"wherewhat/internal/domain/entity"
	domainerror "wherewhat/internal/domain/error"
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
func fakePassword() string { return gofakeit.Password(true, true, true, false, false, 10) }
func fakeHash() string     { return gofakeit.LetterN(60) }

func fakePublicUser() entity.PublicUser {
	return entity.PublicUser{
		ID:       fakeID(),
		Username: gofakeit.Username(),
	}
}

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = gofakeit.Error()

// TestUseCase_Execute covers validation, the current-password check (storage error and mismatch),
// hashing failure, the save failure, and the happy path.
func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// mock arranges expectations on the mocks and returns the Input to execute.
		mock      func(d *deps) Input
		assertErr func(t *testing.T, err error)
	}{
		{
			name: "blank new password fails validation before any lookup",
			mock: func(d *deps) Input {
				return Input{
					User:            fakePublicUser(),
					CurrentPassword: fakePassword(),
					NewPassword:     "",
				}
			},
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Please enter a password")
			},
		},
		{
			name: "current password check storage error",
			mock: func(d *deps) Input {
				user := fakePublicUser()
				d.users.EXPECT().FindByID(gomock.Any(), user.ID).Return(entity.User{}, errStub)

				return Input{
					User:            user,
					CurrentPassword: fakePassword(),
					NewPassword:     fakePassword(),
				}
			},
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
					CurrentPassword: current,
					NewPassword:     fakePassword(),
				}
			},
			assertErr: func(t *testing.T, err error) {
				var forbidden *domainerror.ForbiddenError

				require.ErrorAs(t, err, &forbidden)
				require.Equal(t, "Incorrect current password", forbidden.Message)
			},
		},
		{
			name: "new password hashing failure",
			mock: func(d *deps) Input {
				user := fakePublicUser()
				current, newPassword := fakePassword(), fakePassword()
				stored := entity.User{
					ID:           user.ID,
					PasswordHash: fakeHash(),
				}
				d.users.EXPECT().FindByID(gomock.Any(), user.ID).Return(stored, nil)
				d.hasher.EXPECT().Compare(stored.PasswordHash, current).Return(true)
				d.hasher.EXPECT().Hash(newPassword).Return("", errStub)

				return Input{
					User:            user,
					CurrentPassword: current,
					NewPassword:     newPassword,
				}
			},
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to process password")
			},
		},
		{
			name: "save failure",
			mock: func(d *deps) Input {
				user := fakePublicUser()
				current, newPassword, newHash := fakePassword(), fakePassword(), fakeHash()
				stored := entity.User{
					ID:           user.ID,
					PasswordHash: fakeHash(),
				}
				d.users.EXPECT().FindByID(gomock.Any(), user.ID).Return(stored, nil)
				d.hasher.EXPECT().Compare(stored.PasswordHash, current).Return(true)
				d.hasher.EXPECT().Hash(newPassword).Return(newHash, nil)
				d.users.EXPECT().UpdatePasswordHash(gomock.Any(), user.ID, newHash).Return(errStub)

				return Input{
					User:            user,
					CurrentPassword: current,
					NewPassword:     newPassword,
				}
			},
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to update password")
			},
		},
		{
			name: "changes the password after confirming the current one",
			mock: func(d *deps) Input {
				user := fakePublicUser()
				current, newPassword, newHash := fakePassword(), fakePassword(), fakeHash()
				stored := entity.User{
					ID:           user.ID,
					PasswordHash: fakeHash(),
				}
				d.users.EXPECT().FindByID(gomock.Any(), user.ID).Return(stored, nil)
				d.hasher.EXPECT().Compare(stored.PasswordHash, current).Return(true)
				d.hasher.EXPECT().Hash(newPassword).Return(newHash, nil)
				d.users.EXPECT().UpdatePasswordHash(gomock.Any(), user.ID, newHash).Return(nil)

				return Input{
					User:            user,
					CurrentPassword: current,
					NewPassword:     newPassword,
				}
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

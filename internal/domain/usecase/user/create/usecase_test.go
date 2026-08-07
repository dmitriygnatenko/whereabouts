package create

import (
	"context"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"wherewhat/internal/domain/entity"
	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/usecase"
	"wherewhat/internal/port"
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

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = gofakeit.Error()

// assertZeroOutput checks that Execute handed back a zero-value Output, as it does on every
// failure path.
func assertZeroOutput(t *testing.T, got Output) {
	t.Helper()

	require.Equal(t, Output{}, got)
}

// TestUseCase_Execute covers validation, password hashing, a duplicate-username conflict, a
// generic storage error, and the happy path.
func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	conflict := &domainerror.ConflictError{Message: "username already exists"}
	rawUsername, password := "  "+fakeUsername()+"  ", fakePassword()
	normalizedUsername := usecase.NormalizeUsername(rawUsername)
	createdID := fakeID()

	tests := []struct {
		name string
		// mock arranges expectations on the mocks and returns the Input to execute.
		mock         func(d *deps) Input
		assertResult func(t *testing.T, got Output)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "empty username fails validation before any lookup",
			mock: func(d *deps) Input {
				return Input{
					Username: "",
					Password: fakePassword(),
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Please enter a username")
			},
		},
		{
			name: "empty password fails validation before any lookup",
			mock: func(d *deps) Input {
				return Input{
					Username: fakeUsername(),
					Password: "",
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Please enter a password")
			},
		},
		{
			name: "password hashing failure",
			mock: func(d *deps) Input {
				password := fakePassword()
				d.hasher.EXPECT().Hash(password).Return("", errStub)

				return Input{
					Username: fakeUsername(),
					Password: password,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to process password")
			},
		},
		{
			name: "duplicate username is reported as a conflict",
			mock: func(d *deps) Input {
				username, password := fakeUsername(), fakePassword()
				normalized := usecase.NormalizeUsername(username)
				hash := fakeHash()
				d.hasher.EXPECT().Hash(password).Return(hash, nil)
				d.users.EXPECT().
					Create(gomock.Any(), port.UserCreateRequest{
						Username: normalized, PasswordHash: hash, Settings: entity.UserSettings{},
					}).
					Return(uint64(0), conflict)

				return Input{
					Username: username,
					Password: password,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				var got *domainerror.ConflictError

				require.ErrorAs(t, err, &got)
				require.Equal(t, conflict.Message, got.Message)
			},
		},
		{
			name: "generic creation failure",
			mock: func(d *deps) Input {
				username, password := fakeUsername(), fakePassword()
				normalized := usecase.NormalizeUsername(username)
				hash := fakeHash()
				d.hasher.EXPECT().Hash(password).Return(hash, nil)
				d.users.EXPECT().
					Create(gomock.Any(), port.UserCreateRequest{
						Username: normalized, PasswordHash: hash, Settings: entity.UserSettings{},
					}).
					Return(uint64(0), errStub)

				return Input{
					Username: username,
					Password: password,
				}
			},
			assertResult: assertZeroOutput,
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to create user")
			},
		},
		{
			name: "creates the account with a normalized username",
			mock: func(d *deps) Input {
				hash := fakeHash()
				d.hasher.EXPECT().Hash(password).Return(hash, nil)
				d.users.EXPECT().
					Create(gomock.Any(), port.UserCreateRequest{
						Username: normalizedUsername, PasswordHash: hash, Settings: entity.UserSettings{},
					}).
					Return(createdID, nil)

				return Input{
					Username: rawUsername,
					Password: password,
				}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, createdID, got.User.ID)
				require.Equal(t, normalizedUsername, got.User.Username)
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

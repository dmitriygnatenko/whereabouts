package login

import (
	"context"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"wherewhat/internal/domain/entity"
	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/usecase"
	"wherewhat/internal/port/mocks"
)

// deps bundles the four mocks a UseCase depends on.
type deps struct {
	user    *mocks.MockUserRepository
	session *mocks.MockSessionRepository
	hasher  *mocks.MockPasswordHasher
	token   *mocks.MockTokenGenerator
}

// newUseCase returns a UseCase wired to fresh mocks; any call a test doesn't stub via EXPECT()
// fails it.
func newUseCase(t *testing.T) (*UseCase, *deps) {
	t.Helper()

	mc := gomock.NewController(t)
	t.Cleanup(mc.Finish)

	d := &deps{
		user:    mocks.NewMockUserRepository(mc),
		session: mocks.NewMockSessionRepository(mc),
		hasher:  mocks.NewMockPasswordHasher(mc),
		token:   mocks.NewMockTokenGenerator(mc),
	}

	return New(d.user, d.session, d.hasher, d.token), d
}

// fakeID, fakeUsername, fakePassword and fakeHash are the field-shaped random values the tests
// below bind into mock expectations and returned rows, so a test failure is never masked by two
// cases accidentally sharing a fixture value.
func fakeID() uint64       { return uint64(gofakeit.Number(1, 1_000_000)) }
func fakeUsername() string { return gofakeit.Username() }
func fakePassword() string { return gofakeit.Password(true, true, true, false, false, 10) }
func fakeHash() string     { return gofakeit.LetterN(60) }

func fakeUserEntity(username string) entity.User {
	return entity.User{
		ID:           fakeID(),
		Username:     username,
		PasswordHash: fakeHash(),
	}
}

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = gofakeit.Error()

// assertIncorrectCredentials checks that err is the shared "wrong username or password"
// UnauthorizedError, which login returns for both an unknown username and a bad password —
// deliberately not distinguishing the two to callers.
func assertIncorrectCredentials(t *testing.T, err error) {
	t.Helper()

	var unauthorized *domainerror.UnauthorizedError

	require.ErrorAs(t, err, &unauthorized)
	require.Equal(t, "Incorrect username or password", unauthorized.Message)
}

// TestUseCase_Execute covers validation, credential checks, the language-adoption side effect,
// token/session creation failures, and the happy path.
func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

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
			assertResult: func(
				t *testing.T, got Output,
			) {
				require.Equal(t, Output{}, got)
			},
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
			assertResult: func(
				t *testing.T, got Output,
			) {
				require.Equal(t, Output{}, got)
			},
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Please enter a password")
			},
		},
		{
			name: "unknown username is reported as incorrect credentials",
			mock: func(d *deps) Input {
				username, password := fakeUsername(), fakePassword()
				normalized := usecase.NormalizeUsername(username)
				d.user.EXPECT().FindByUsername(gomock.Any(), normalized).
					Return(entity.User{}, &domainerror.NotFoundError{Message: "not found"})

				return Input{
					Username: username,
					Password: password,
				}
			},
			assertResult: func(
				t *testing.T, got Output,
			) {
				require.Equal(t, Output{}, got)
			},
			assertErr: assertIncorrectCredentials,
		},
		{
			name: "user lookup storage error is reported as incorrect credentials",
			mock: func(d *deps) Input {
				username, password := fakeUsername(), fakePassword()
				normalized := usecase.NormalizeUsername(username)
				d.user.EXPECT().FindByUsername(gomock.Any(), normalized).Return(entity.User{}, errStub)

				return Input{
					Username: username,
					Password: password,
				}
			},
			assertResult: func(
				t *testing.T, got Output,
			) {
				require.Equal(t, Output{}, got)
			},
			assertErr: assertIncorrectCredentials,
		},
		{
			name: "wrong password is reported as incorrect credentials",
			mock: func(d *deps) Input {
				username, password := fakeUsername(), fakePassword()
				normalized := usecase.NormalizeUsername(username)
				user := fakeUserEntity(normalized)
				d.user.EXPECT().FindByUsername(gomock.Any(), normalized).Return(user, nil)
				d.hasher.EXPECT().Compare(user.PasswordHash, password).Return(false)

				return Input{
					Username: username,
					Password: password,
				}
			},
			assertResult: func(
				t *testing.T, got Output,
			) {
				require.Equal(t, Output{}, got)
			},
			assertErr: assertIncorrectCredentials,
		},
		{
			name: "adopts the given language when the user has none saved",
			mock: func(d *deps) Input {
				username, password := fakeUsername(), fakePassword()
				normalized := usecase.NormalizeUsername(username)
				user := fakeUserEntity(normalized)
				d.user.EXPECT().FindByUsername(gomock.Any(), normalized).Return(user, nil)
				d.hasher.EXPECT().Compare(user.PasswordHash, password).Return(true)
				d.user.EXPECT().UpdateLanguage(gomock.Any(), user.ID, "ru").Return(nil)
				d.token.EXPECT().NewToken().Return("tok-123", nil)
				d.session.EXPECT().Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, s entity.Session) error {
						require.Equal(t, "tok-123", s.Token)
						require.Equal(t, user.ID, s.UserID)
						require.WithinDuration(t, time.Now().UTC().Add(sessionDuration), s.ExpiresAt, time.Minute)

						return nil
					})

				return Input{
					Username: username,
					Password: password,
					Language: "ru",
				}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, "ru", got.User.Language)
				require.Equal(t, "tok-123", got.Session.Token)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "failing to save the adopted language aborts the login",
			mock: func(d *deps) Input {
				username, password := fakeUsername(), fakePassword()
				normalized := usecase.NormalizeUsername(username)
				user := fakeUserEntity(normalized)
				d.user.EXPECT().FindByUsername(gomock.Any(), normalized).Return(user, nil)
				d.hasher.EXPECT().Compare(user.PasswordHash, password).Return(true)
				d.user.EXPECT().UpdateLanguage(gomock.Any(), user.ID, "ru").Return(errStub)

				return Input{
					Username: username,
					Password: password,
					Language: "ru",
				}
			},
			assertResult: func(
				t *testing.T, got Output,
			) {
				require.Equal(t, Output{}, got)
			},
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to save language preference")
			},
		},
		{
			name: "an already-set language is never overwritten",
			mock: func(d *deps) Input {
				username, password := fakeUsername(), fakePassword()
				normalized := usecase.NormalizeUsername(username)
				user := fakeUserEntity(normalized)
				user.Settings.Language = "en"
				d.user.EXPECT().FindByUsername(gomock.Any(), normalized).Return(user, nil)
				d.hasher.EXPECT().Compare(user.PasswordHash, password).Return(true)
				d.token.EXPECT().NewToken().Return("tok-123", nil)
				d.session.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

				return Input{
					Username: username,
					Password: password,
					Language: "fr",
				}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, "en", got.User.Language)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "an unsupported language is ignored",
			mock: func(d *deps) Input {
				username, password := fakeUsername(), fakePassword()
				normalized := usecase.NormalizeUsername(username)
				user := fakeUserEntity(normalized)
				d.user.EXPECT().FindByUsername(gomock.Any(), normalized).Return(user, nil)
				d.hasher.EXPECT().Compare(user.PasswordHash, password).Return(true)
				d.token.EXPECT().NewToken().Return("tok-123", nil)
				d.session.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

				return Input{
					Username: username,
					Password: password,
					Language: "xx",
				}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Empty(t, got.User.Language)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "token generation failure is reported",
			mock: func(d *deps) Input {
				username, password := fakeUsername(), fakePassword()
				normalized := usecase.NormalizeUsername(username)
				user := fakeUserEntity(normalized)
				d.user.EXPECT().FindByUsername(gomock.Any(), normalized).Return(user, nil)
				d.hasher.EXPECT().Compare(user.PasswordHash, password).Return(true)
				d.token.EXPECT().NewToken().Return("", errStub)

				return Input{
					Username: username,
					Password: password,
				}
			},
			assertResult: func(
				t *testing.T, got Output,
			) {
				require.Equal(t, Output{}, got)
			},
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to generate a token")
			},
		},
		{
			name: "session creation failure is reported",
			mock: func(d *deps) Input {
				username, password := fakeUsername(), fakePassword()
				normalized := usecase.NormalizeUsername(username)
				user := fakeUserEntity(normalized)
				d.user.EXPECT().FindByUsername(gomock.Any(), normalized).Return(user, nil)
				d.hasher.EXPECT().Compare(user.PasswordHash, password).Return(true)
				d.token.EXPECT().NewToken().Return("tok-123", nil)
				d.session.EXPECT().Create(gomock.Any(), gomock.Any()).Return(errStub)

				return Input{
					Username: username,
					Password: password,
				}
			},
			assertResult: func(
				t *testing.T, got Output,
			) {
				require.Equal(t, Output{}, got)
			},
			assertErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "Failed to start a session")
			},
		},
		{
			name: "successful login normalizes the username and starts a session",
			mock: func(d *deps) Input {
				username, password := "  "+fakeUsername()+"  ", fakePassword()
				normalized := usecase.NormalizeUsername(username)
				user := fakeUserEntity(normalized)
				user.Settings.Language = "en"
				d.user.EXPECT().FindByUsername(gomock.Any(), normalized).Return(user, nil)
				d.hasher.EXPECT().Compare(user.PasswordHash, password).Return(true)
				d.token.EXPECT().NewToken().Return("tok-abc", nil)
				d.session.EXPECT().Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, s entity.Session) error {
						require.Equal(t, "tok-abc", s.Token)
						require.Equal(t, user.ID, s.UserID)
						require.WithinDuration(t, time.Now().UTC().Add(sessionDuration), s.ExpiresAt, time.Minute)

						return nil
					})

				return Input{
					Username: username,
					Password: password,
				}
			},
			assertResult: func(t *testing.T, got Output) {
				require.Equal(t, "tok-abc", got.Session.Token)
				require.Equal(t, "en", got.User.Language)
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

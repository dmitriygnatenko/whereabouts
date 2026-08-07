package authenticate

import (
	"context"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"wherewhat/internal/domain/entity"
	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/port/mocks"
)

// newUseCase returns a UseCase wired to fresh mocks; any call a test doesn't stub via EXPECT()
// fails it.
func newUseCase(t *testing.T) (
	*UseCase,
	*mocks.MockSessionRepository,
	*mocks.MockUserRepository,
) {
	t.Helper()

	mc := gomock.NewController(t)
	t.Cleanup(mc.Finish)

	session := mocks.NewMockSessionRepository(mc)
	user := mocks.NewMockUserRepository(mc)

	return New(session, user), session, user
}

// fakeID, fakeToken, fakeUsername and fakeHash are the field-shaped random values the tests below
// bind into mock expectations and returned rows, so a test failure is never masked by two cases
// accidentally sharing a fixture value.
func fakeID() uint64       { return uint64(gofakeit.Number(1, 1_000_000)) }
func fakeToken() string    { return gofakeit.UUID() }
func fakeUsername() string { return gofakeit.Username() }
func fakeHash() string     { return gofakeit.LetterN(60) }

func fakeUserEntity() entity.User {
	return entity.User{
		ID:           fakeID(),
		Username:     fakeUsername(),
		PasswordHash: fakeHash(),
	}
}

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = gofakeit.Error()

// assertUnauthorized checks that err is the package's shared unauthorized sentinel, which every
// failure mode of Execute returns regardless of its underlying cause.
func assertUnauthorized(t *testing.T, err error) {
	t.Helper()

	var unauthorized *domainerror.UnauthorizedError

	require.ErrorAs(t, err, &unauthorized)
	require.Equal(t, "authentication required", unauthorized.Message)
}

// assertZeroOutput checks that Execute handed back a zero-value Output, as it does on every
// failure path.
func assertZeroOutput(t *testing.T, got Output) {
	t.Helper()

	require.Equal(t, Output{}, got)
}

// TestUseCase_Execute covers every branch of resolving a token to its user: an empty token, an
// unknown token, a storage error, an expired session (with both outcomes of deleting it), a
// dangling user row, and the happy path.
func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// mock arranges expectations on the two repositories and returns the Input to execute.
		mock func(
			session *mocks.MockSessionRepository, user *mocks.MockUserRepository,
		) Input
		assertResult func(t *testing.T, got Output)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "empty token is rejected without a lookup",
			mock: func(
				session *mocks.MockSessionRepository, user *mocks.MockUserRepository,
			) Input {
				return Input{Token: ""}
			},
			assertResult: assertZeroOutput,
			assertErr:    assertUnauthorized,
		},
		{
			name: "unknown token is unauthorized",
			mock: func(
				session *mocks.MockSessionRepository, user *mocks.MockUserRepository,
			) Input {
				token := fakeToken()
				session.EXPECT().FindByToken(gomock.Any(), token).
					Return(entity.Session{}, &domainerror.NotFoundError{Message: "not found"})

				return Input{Token: token}
			},
			assertResult: assertZeroOutput,
			assertErr:    assertUnauthorized,
		},
		{
			name: "session lookup storage error is unauthorized",
			mock: func(
				session *mocks.MockSessionRepository, user *mocks.MockUserRepository,
			) Input {
				token := fakeToken()
				session.EXPECT().FindByToken(gomock.Any(), token).Return(entity.Session{}, errStub)

				return Input{Token: token}
			},
			assertResult: assertZeroOutput,
			assertErr:    assertUnauthorized,
		},
		{
			name: "expired session is deleted and rejected",
			mock: func(
				session *mocks.MockSessionRepository, user *mocks.MockUserRepository,
			) Input {
				token := fakeToken()
				expired := entity.Session{
					Token:     token,
					UserID:    fakeID(),
					ExpiresAt: time.Now().UTC().Add(-time.Hour),
				}
				session.EXPECT().FindByToken(gomock.Any(), token).Return(expired, nil)
				session.EXPECT().Delete(gomock.Any(), token).Return(nil)

				return Input{Token: token}
			},
			assertResult: assertZeroOutput,
			assertErr:    assertUnauthorized,
		},
		{
			name: "expired session delete failure is still rejected",
			mock: func(
				session *mocks.MockSessionRepository, user *mocks.MockUserRepository,
			) Input {
				token := fakeToken()
				expired := entity.Session{
					Token:     token,
					UserID:    fakeID(),
					ExpiresAt: time.Now().UTC().Add(-time.Hour),
				}
				session.EXPECT().FindByToken(gomock.Any(), token).Return(expired, nil)
				session.EXPECT().Delete(gomock.Any(), token).Return(errStub)

				return Input{Token: token}
			},
			assertResult: assertZeroOutput,
			assertErr:    assertUnauthorized,
		},
		{
			name: "dangling user is unauthorized",
			mock: func(
				session *mocks.MockSessionRepository, user *mocks.MockUserRepository,
			) Input {
				token := fakeToken()
				valid := entity.Session{
					Token:     token,
					UserID:    fakeID(),
					ExpiresAt: time.Now().UTC().Add(time.Hour),
				}
				session.EXPECT().FindByToken(gomock.Any(), token).Return(valid, nil)
				user.EXPECT().FindByID(gomock.Any(), valid.UserID).Return(entity.User{}, errStub)

				return Input{Token: token}
			},
			assertResult: assertZeroOutput,
			assertErr:    assertUnauthorized,
		},
		{
			name: "valid session resolves to its user",
			mock: func(
				session *mocks.MockSessionRepository, user *mocks.MockUserRepository,
			) Input {
				u := fakeUserEntity()
				token := fakeToken()
				valid := entity.Session{
					Token:     token,
					UserID:    u.ID,
					ExpiresAt: time.Now().UTC().Add(time.Hour),
				}
				session.EXPECT().FindByToken(gomock.Any(), token).Return(valid, nil)
				user.EXPECT().FindByID(gomock.Any(), valid.UserID).Return(u, nil)

				return Input{Token: token}
			},
			assertResult: func(t *testing.T, got Output) {
				require.NotZero(t, got.User.ID)
				require.NotEmpty(t, got.User.Username)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc, session, user := newUseCase(t)
			input := tt.mock(session, user)

			got, err := uc.Execute(context.Background(), input)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

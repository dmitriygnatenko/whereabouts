package logout

import (
	"context"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"go.uber.org/mock/gomock"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/port/mocks"
)

// newUseCase returns a UseCase wired to a fresh MockSessionRepository; any call a test doesn't
// stub via EXPECT() fails it.
func newUseCase(t *testing.T) (*UseCase, *mocks.MockSessionRepository) {
	t.Helper()

	mc := gomock.NewController(t)
	t.Cleanup(mc.Finish)

	session := mocks.NewMockSessionRepository(mc)

	return New(session), session
}

func fakeToken() string { return gofakeit.UUID() }

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = gofakeit.Error()

// TestUseCase_Execute covers logout's best-effort delete: an empty token never reaches the
// repository, and neither a not-found nor a storage error changes the caller-visible behaviour —
// Execute has no return value, so every case is really just checking which calls happen.
func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mock func(session *mocks.MockSessionRepository) Input
	}{
		{
			name: "empty token never reaches the repository",
			mock: func(session *mocks.MockSessionRepository) Input {
				return Input{Token: ""}
			},
		},
		{
			name: "deletes the session behind the token",
			mock: func(session *mocks.MockSessionRepository) Input {
				token := fakeToken()
				session.EXPECT().Delete(gomock.Any(), token).Return(nil)

				return Input{Token: token}
			},
		},
		{
			name: "a not-found error is swallowed",
			mock: func(session *mocks.MockSessionRepository) Input {
				token := fakeToken()
				session.EXPECT().Delete(gomock.Any(), token).
					Return(&domainerror.NotFoundError{Message: "not found"})

				return Input{Token: token}
			},
		},
		{
			name: "a storage error is swallowed",
			mock: func(session *mocks.MockSessionRepository) Input {
				token := fakeToken()
				session.EXPECT().Delete(gomock.Any(), token).Return(errStub)

				return Input{Token: token}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc, session := newUseCase(t)
			input := tt.mock(session)

			uc.Execute(context.Background(), input)
		})
	}
}

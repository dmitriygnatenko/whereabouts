package session

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"wherewhat/internal/domain/entity"
	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/repository/session/mocks"
	"wherewhat/internal/storage/model"
)

// newRepo returns a Repository wired to a fresh MockStorage; any call a test doesn't stub via
// EXPECT() fails it, exactly like an unmet sqlmock expectation would in the adapter tests.
func newRepo(t *testing.T) (*Repository, *mocks.MockStorage) {
	t.Helper()

	mc := gomock.NewController(t)
	t.Cleanup(mc.Finish)

	m := mocks.NewMockStorage(mc)

	return New(m), m
}

// fakeID, fakeToken and fakeNow are the field-shaped random values the tests below bind into mock
// expectations and returned rows, so a test failure is never masked by two cases accidentally
// sharing a fixture value.
func fakeID() uint64     { return uint64(gofakeit.Number(1, 1_000_000)) }
func fakeToken() string  { return gofakeit.UUID() }
func fakeNow() time.Time { return gofakeit.Date().UTC() }

func fakeSessionEntity() entity.Session {
	return entity.Session{
		Token:     fakeToken(),
		UserID:    fakeID(),
		ExpiresAt: fakeNow(),
	}
}

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = errors.New(gofakeit.Sentence())

// TestRepository_Create covers the entity -> row conversion and the createdAt stamp, which the
// repository generates itself rather than taking from the caller.
func TestRepository_Create(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mock      func(m *mocks.MockStorage, session entity.Session)
		assertErr func(t *testing.T, err error)
	}{
		{
			name: "converts the entity and stamps it with the current time in UTC",
			mock: func(m *mocks.MockStorage, session entity.Session) {
				m.EXPECT().CreateSession(context.Background(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(
						_ context.Context, got model.Session, createdAt time.Time,
					) error {
						want := model.SessionFromEntity(session)
						require.Equal(t, want.Token, got.Token)
						require.Equal(t, want.UserID, got.UserID)
						require.True(t, got.ExpiresAt.Equal(want.ExpiresAt.Time))

						require.Equal(t, time.UTC, createdAt.Location())

						since := time.Since(createdAt)
						require.GreaterOrEqual(t, since, time.Duration(0))
						require.LessOrEqual(t, since, time.Minute)

						return nil
					})
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage, session entity.Session) {
				m.EXPECT().CreateSession(context.Background(), gomock.Any(), gomock.Any()).Return(errStub)
			},
			assertErr: func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			session := fakeSessionEntity()
			tt.mock(m, session)

			err := r.Create(context.Background(), session)
			tt.assertErr(t, err)
		})
	}
}

// TestRepository_FindByToken covers the lookup, including the token -> NotFoundError translation.
func TestRepository_FindByToken(t *testing.T) {
	t.Parallel()

	token := fakeToken()

	type args struct {
		ctx   context.Context
		token string
	}

	tests := []struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage) model.Session
		assertResult func(t *testing.T, want, got entity.Session)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "finds the session",
			args: args{
				ctx:   context.Background(),
				token: token,
			},
			mock: func(m *mocks.MockStorage) model.Session {
				sess := model.Session{
					Token:     token,
					UserID:    fakeID(),
					ExpiresAt: model.NewTime(fakeNow()),
				}
				m.EXPECT().FindSessionByToken(context.Background(), token).Return(sess, nil)

				return sess
			},
			assertResult: func(t *testing.T, want, got entity.Session) {
				require.Equal(t, want.Token, got.Token)
				require.Equal(t, want.UserID, got.UserID)
				require.True(t, got.ExpiresAt.Equal(want.ExpiresAt))
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "an unknown token becomes a NotFoundError",
			args: args{
				ctx:   context.Background(),
				token: token,
			},
			mock: func(m *mocks.MockStorage) model.Session {
				m.EXPECT().FindSessionByToken(context.Background(), token).Return(model.Session{}, sql.ErrNoRows)

				return model.Session{}
			},
			assertResult: func(t *testing.T, want, got entity.Session) {},
			assertErr: func(t *testing.T, err error) {
				var notFound *domainerror.NotFoundError
				require.ErrorAs(t, err, &notFound)
			},
		},
		{
			name: "a storage error is propagated",
			args: args{
				ctx:   context.Background(),
				token: token,
			},
			mock: func(m *mocks.MockStorage) model.Session {
				m.EXPECT().FindSessionByToken(context.Background(), token).Return(model.Session{}, errStub)

				return model.Session{}
			},
			assertResult: func(t *testing.T, want, got entity.Session) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			wantModel := tt.mock(m)

			got, err := r.FindByToken(tt.args.ctx, tt.args.token)
			tt.assertErr(t, err)
			tt.assertResult(t, entity.Session{
				Token:     wantModel.Token,
				UserID:    wantModel.UserID,
				ExpiresAt: wantModel.ExpiresAt.Time,
			}, got)
		})
	}
}

// TestRepository_Delete covers the plain delegation to storage.
func TestRepository_Delete(t *testing.T) {
	t.Parallel()

	token := fakeToken()

	type args struct {
		ctx   context.Context
		token string
	}

	tests := []struct {
		name      string
		args      args
		mock      func(m *mocks.MockStorage)
		assertErr func(t *testing.T, err error)
	}{
		{
			name: "delegates to storage",
			args: args{
				ctx:   context.Background(),
				token: token,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().DeleteSession(context.Background(), token).Return(nil)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a storage error is propagated",
			args: args{
				ctx:   context.Background(),
				token: token,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().DeleteSession(context.Background(), token).Return(errStub)
			},
			assertErr: func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			tt.mock(m)

			err := r.Delete(tt.args.ctx, tt.args.token)
			tt.assertErr(t, err)
		})
	}
}

// TestRepository_DeleteExpired covers the plain delegation to storage, including the deleted-rows
// count it hands back.
func TestRepository_DeleteExpired(t *testing.T) {
	t.Parallel()

	now := fakeNow()
	want := int64(gofakeit.Number(0, 500))

	type args struct {
		ctx context.Context
		now time.Time
	}

	tests := []struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage)
		assertResult func(t *testing.T, got int64)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "delegates to storage",
			args: args{
				ctx: context.Background(),
				now: now,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().DeleteExpiredSessions(context.Background(), now).Return(want, nil)
			},
			assertResult: func(t *testing.T, got int64) { require.Equal(t, want, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a storage error is propagated",
			args: args{
				ctx: context.Background(),
				now: now,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().DeleteExpiredSessions(context.Background(), now).Return(int64(0), errStub)
			},
			assertResult: func(t *testing.T, got int64) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			tt.mock(m)

			got, err := r.DeleteExpired(tt.args.ctx, tt.args.now)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

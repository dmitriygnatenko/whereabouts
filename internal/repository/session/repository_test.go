package session

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
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
	return entity.Session{Token: fakeToken(), UserID: fakeID(), ExpiresAt: fakeNow()}
}

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = errors.New(gofakeit.Sentence())

// TestRepository_Create covers the entity -> row conversion and the createdAt stamp, which the
// repository generates itself rather than taking from the caller.
func TestRepository_Create(t *testing.T) {
	tests := []struct {
		name    string
		mock    func(m *mocks.MockStorage, session entity.Session)
		wantErr error
	}{
		{
			name: "converts the entity and stamps it with the current time in UTC",
			mock: func(m *mocks.MockStorage, session entity.Session) {
				m.EXPECT().CreateSession(context.Background(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, got model.Session, createdAt time.Time) error {
						want := model.SessionFromEntity(session)
						if got.Token != want.Token || got.UserID != want.UserID || !got.ExpiresAt.Equal(want.ExpiresAt.Time) {
							t.Fatalf("CreateSession() session = %+v, want %+v", got, want)
						}

						if createdAt.Location() != time.UTC {
							t.Fatalf("CreateSession() createdAt = %v, want UTC", createdAt)
						}

						if since := time.Since(createdAt); since < 0 || since > time.Minute {
							t.Fatalf("CreateSession() createdAt = %v, want ~now", createdAt)
						}

						return nil
					})
			},
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage, session entity.Session) {
				m.EXPECT().CreateSession(context.Background(), gomock.Any(), gomock.Any()).Return(errStub)
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			session := fakeSessionEntity()
			tt.mock(m, session)

			err := r.Create(context.Background(), session)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Create() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		})
	}
}

// TestRepository_FindByToken covers the lookup, including the token -> NotFoundError translation.
func TestRepository_FindByToken(t *testing.T) {
	token := fakeToken()

	tests := []struct {
		name         string
		mock         func(m *mocks.MockStorage) model.Session
		wantErr      error
		wantNotFound bool
	}{
		{
			name: "finds the session",
			mock: func(m *mocks.MockStorage) model.Session {
				sess := model.Session{Token: token, UserID: fakeID(), ExpiresAt: model.NewTime(fakeNow())}
				m.EXPECT().FindSessionByToken(context.Background(), token).Return(sess, nil)

				return sess
			},
		},
		{
			name: "an unknown token becomes a NotFoundError",
			mock: func(m *mocks.MockStorage) model.Session {
				m.EXPECT().FindSessionByToken(context.Background(), token).Return(model.Session{}, sql.ErrNoRows)

				return model.Session{}
			},
			wantNotFound: true,
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) model.Session {
				m.EXPECT().FindSessionByToken(context.Background(), token).Return(model.Session{}, errStub)

				return model.Session{}
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			want := tt.mock(m)

			got, err := r.FindByToken(context.Background(), token)
			if tt.wantNotFound {
				var notFound *domainerror.NotFoundError
				if !errors.As(err, &notFound) {
					t.Fatalf("FindByToken() error = %v, want *domainerror.NotFoundError", err)
				}

				return
			}

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("FindByToken() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("FindByToken() error = %v", err)
			}

			if got.Token != want.Token || got.UserID != want.UserID || !got.ExpiresAt.Equal(want.ExpiresAt.Time) {
				t.Fatalf("FindByToken() = %+v, want %+v", got, want)
			}
		})
	}
}

// TestRepository_Delete covers the plain delegation to storage.
func TestRepository_Delete(t *testing.T) {
	token := fakeToken()

	tests := []struct {
		name    string
		mock    func(m *mocks.MockStorage)
		wantErr error
	}{
		{
			name: "delegates to storage",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().DeleteSession(context.Background(), token).Return(nil)
			},
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().DeleteSession(context.Background(), token).Return(errStub)
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			tt.mock(m)

			err := r.Delete(context.Background(), token)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Delete() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Delete() error = %v", err)
			}
		})
	}
}

// TestRepository_DeleteExpired covers the plain delegation to storage, including the deleted-rows
// count it hands back.
func TestRepository_DeleteExpired(t *testing.T) {
	now := fakeNow()
	want := int64(gofakeit.Number(0, 500))

	tests := []struct {
		name    string
		mock    func(m *mocks.MockStorage)
		want    int64
		wantErr error
	}{
		{
			name: "delegates to storage",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().DeleteExpiredSessions(context.Background(), now).Return(want, nil)
			},
			want: want,
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().DeleteExpiredSessions(context.Background(), now).Return(int64(0), errStub)
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			tt.mock(m)

			got, err := r.DeleteExpired(context.Background(), now)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("DeleteExpired() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("DeleteExpired() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf("DeleteExpired() = %d, want %d", got, tt.want)
			}
		})
	}
}

package user

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"go.uber.org/mock/gomock"

	"wherewhat/internal/domain/entity"
	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/repository/user/mocks"
	storageError "wherewhat/internal/storage/error"
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

// fakeID, fakeUsername, fakeHash and fakeLang are the field-shaped random values the tests below
// bind into mock expectations and returned rows, so a test failure is never masked by two cases
// accidentally sharing a fixture value.
func fakeID() uint64       { return uint64(gofakeit.Number(1, 1_000_000)) }
func fakeUsername() string { return gofakeit.Username() }
func fakeHash() string     { return gofakeit.LetterN(60) }
func fakeLang() string     { return gofakeit.LanguageAbbreviation() }
func fakeDepth() int       { return gofakeit.Number(0, 10) }

func fakeUserModel() model.User {
	return model.User{
		ID: fakeID(), Username: fakeUsername(), PasswordHash: fakeHash(),
		Settings: model.UserSettings{Language: fakeLang(), LocationFilterDepth: fakeDepth()},
	}
}

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = errors.New(gofakeit.Sentence())

// TestRepository_FindByUsername covers the lookup, including the username -> NotFoundError
// translation.
func TestRepository_FindByUsername(t *testing.T) {
	username := fakeUsername()

	tests := []struct {
		name         string
		mock         func(m *mocks.MockStorage) entity.User
		wantErr      error
		wantNotFound bool
	}{
		{
			name: "finds the user",
			mock: func(m *mocks.MockStorage) entity.User {
				u := fakeUserModel()
				u.Username = username
				m.EXPECT().FindUserByUsername(context.Background(), username).Return(u, nil)

				return u.ToEntity()
			},
		},
		{
			name: "an unknown username becomes a NotFoundError",
			mock: func(m *mocks.MockStorage) entity.User {
				m.EXPECT().FindUserByUsername(context.Background(), username).Return(model.User{}, sql.ErrNoRows)

				return entity.User{}
			},
			wantNotFound: true,
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) entity.User {
				m.EXPECT().FindUserByUsername(context.Background(), username).Return(model.User{}, errStub)

				return entity.User{}
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			want := tt.mock(m)

			got, err := r.FindByUsername(context.Background(), username)
			if tt.wantNotFound {
				var notFound *domainerror.NotFoundError
				if !errors.As(err, &notFound) {
					t.Fatalf("FindByUsername() error = %v, want *domainerror.NotFoundError", err)
				}

				return
			}

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("FindByUsername() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("FindByUsername() error = %v", err)
			}

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("FindByUsername() = %+v, want %+v", got, want)
			}
		})
	}
}

// TestRepository_FindByID covers the lookup, including the id -> NotFoundError translation.
func TestRepository_FindByID(t *testing.T) {
	id := fakeID()

	tests := []struct {
		name         string
		mock         func(m *mocks.MockStorage) entity.User
		wantErr      error
		wantNotFound bool
	}{
		{
			name: "finds the user",
			mock: func(m *mocks.MockStorage) entity.User {
				u := fakeUserModel()
				u.ID = id
				m.EXPECT().FindUserByID(context.Background(), id).Return(u, nil)

				return u.ToEntity()
			},
		},
		{
			name: "an unknown id becomes a NotFoundError",
			mock: func(m *mocks.MockStorage) entity.User {
				m.EXPECT().FindUserByID(context.Background(), id).Return(model.User{}, sql.ErrNoRows)

				return entity.User{}
			},
			wantNotFound: true,
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) entity.User {
				m.EXPECT().FindUserByID(context.Background(), id).Return(model.User{}, errStub)

				return entity.User{}
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			want := tt.mock(m)

			got, err := r.FindByID(context.Background(), id)
			if tt.wantNotFound {
				var notFound *domainerror.NotFoundError
				if !errors.As(err, &notFound) {
					t.Fatalf("FindByID() error = %v, want *domainerror.NotFoundError", err)
				}

				return
			}

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("FindByID() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("FindByID() error = %v", err)
			}

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("FindByID() = %+v, want %+v", got, want)
			}
		})
	}
}

// TestRepository_Create covers the insert and the username -> ConflictError translation, the one
// error shape this repository is allowed to recognize.
func TestRepository_Create(t *testing.T) {
	username, hash := fakeUsername(), fakeHash()
	settings := entity.UserSettings{Language: fakeLang(), LocationFilterDepth: fakeDepth()}

	type testCase struct {
		name         string
		mock         func(m *mocks.MockStorage) uint64
		wantErr      error
		wantConflict bool
	}

	var tests []testCase

	{
		tests = append(tests, testCase{
			name: "stores the user and returns its id",
			mock: func(m *mocks.MockStorage) uint64 {
				wantID := fakeID()
				m.EXPECT().
					CreateUser(context.Background(), username, hash, model.UserSettingsFromEntity(settings)).
					Return(wantID, nil)

				return wantID
			},
		})
	}

	{
		tests = append(tests, testCase{
			name: "a taken username becomes a ConflictError",
			mock: func(m *mocks.MockStorage) uint64 {
				m.EXPECT().
					CreateUser(context.Background(), username, hash, model.UserSettingsFromEntity(settings)).
					Return(uint64(0), storageError.UniqueViolationError)

				return 0
			},
			wantConflict: true,
		})
	}

	{
		tests = append(tests, testCase{
			name: "any other storage error is propagated",
			mock: func(m *mocks.MockStorage) uint64 {
				m.EXPECT().
					CreateUser(context.Background(), username, hash, model.UserSettingsFromEntity(settings)).
					Return(uint64(0), errStub)

				return 0
			},
			wantErr: errStub,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			wantID := tt.mock(m)

			got, err := r.Create(context.Background(), username, hash, settings)
			if tt.wantConflict {
				var conflict *domainerror.ConflictError
				if !errors.As(err, &conflict) {
					t.Fatalf("Create() error = %v, want *domainerror.ConflictError", err)
				}

				return
			}

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Create() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}

			if got != wantID {
				t.Fatalf("Create() = %d, want %d", got, wantID)
			}
		})
	}
}

// TestRepository_UpdateUsername covers the rename and the username -> ConflictError translation.
func TestRepository_UpdateUsername(t *testing.T) {
	id := fakeID()
	username := fakeUsername()

	tests := []struct {
		name         string
		mock         func(m *mocks.MockStorage)
		wantConflict bool
		wantErr      error
	}{
		{
			name: "renames the user",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUsername(context.Background(), id, username).Return(nil)
			},
		},
		{
			name: "a taken username becomes a ConflictError",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUsername(context.Background(), id, username).Return(storageError.UniqueViolationError)
			},
			wantConflict: true,
		},
		{
			name: "any other storage error is propagated",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUsername(context.Background(), id, username).Return(errStub)
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			tt.mock(m)

			err := r.UpdateUsername(context.Background(), id, username)
			if tt.wantConflict {
				var conflict *domainerror.ConflictError
				if !errors.As(err, &conflict) {
					t.Fatalf("UpdateUsername() error = %v, want *domainerror.ConflictError", err)
				}

				return
			}

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("UpdateUsername() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("UpdateUsername() error = %v", err)
			}
		})
	}
}

// TestRepository_UpdatePasswordHash covers the plain delegation to storage.
func TestRepository_UpdatePasswordHash(t *testing.T) {
	id := fakeID()
	hash := fakeHash()

	tests := []struct {
		name    string
		mock    func(m *mocks.MockStorage)
		wantErr error
	}{
		{
			name: "delegates to storage",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUserPasswordHash(context.Background(), id, hash).Return(nil)
			},
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUserPasswordHash(context.Background(), id, hash).Return(errStub)
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			tt.mock(m)

			err := r.UpdatePasswordHash(context.Background(), id, hash)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("UpdatePasswordHash() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("UpdatePasswordHash() error = %v", err)
			}
		})
	}
}

// TestRepository_UpdateLanguage covers the plain delegation to storage.
func TestRepository_UpdateLanguage(t *testing.T) {
	id := fakeID()
	lang := fakeLang()

	tests := []struct {
		name    string
		mock    func(m *mocks.MockStorage)
		wantErr error
	}{
		{
			name: "delegates to storage",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUserLanguage(context.Background(), id, lang).Return(nil)
			},
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUserLanguage(context.Background(), id, lang).Return(errStub)
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			tt.mock(m)

			err := r.UpdateLanguage(context.Background(), id, lang)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("UpdateLanguage() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("UpdateLanguage() error = %v", err)
			}
		})
	}
}

// TestRepository_UpdateLocationFilterDepth covers the plain delegation to storage.
func TestRepository_UpdateLocationFilterDepth(t *testing.T) {
	id := fakeID()
	depth := fakeDepth()

	tests := []struct {
		name    string
		mock    func(m *mocks.MockStorage)
		wantErr error
	}{
		{
			name: "delegates to storage",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUserLocationFilterDepth(context.Background(), id, depth).Return(nil)
			},
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUserLocationFilterDepth(context.Background(), id, depth).Return(errStub)
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			tt.mock(m)

			err := r.UpdateLocationFilterDepth(context.Background(), id, depth)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("UpdateLocationFilterDepth() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("UpdateLocationFilterDepth() error = %v", err)
			}
		})
	}
}

// TestRepository_Count covers the plain delegation to storage.
func TestRepository_Count(t *testing.T) {
	want := gofakeit.Number(0, 500)

	tests := []struct {
		name    string
		mock    func(m *mocks.MockStorage)
		want    int
		wantErr error
	}{
		{
			name: "delegates to storage",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().CountUsers(context.Background()).Return(want, nil)
			},
			want: want,
		},
		{
			name: "a storage error is propagated",
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().CountUsers(context.Background()).Return(0, errStub)
			},
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, m := newRepo(t)
			tt.mock(m)

			got, err := r.Count(context.Background())
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Count() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Count() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf("Count() = %d, want %d", got, tt.want)
			}
		})
	}
}

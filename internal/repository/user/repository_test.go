package user

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"wherewhat/internal/domain/entity"
	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/port"
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
		ID:           fakeID(),
		Username:     fakeUsername(),
		PasswordHash: fakeHash(),
		Settings: model.UserSettings{
			Language:            fakeLang(),
			LocationFilterDepth: fakeDepth(),
		},
	}
}

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = errors.New(gofakeit.Sentence())

// TestRepository_FindByUsername covers the lookup, including the username -> NotFoundError
// translation.
func TestRepository_FindByUsername(t *testing.T) {
	t.Parallel()

	username := fakeUsername()

	type args struct {
		ctx      context.Context
		username string
	}

	tests := []struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage) entity.User
		assertResult func(t *testing.T, want, got entity.User)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "finds the user",
			args: args{
				ctx:      context.Background(),
				username: username,
			},
			mock: func(m *mocks.MockStorage) entity.User {
				u := fakeUserModel()
				u.Username = username
				m.EXPECT().FindUserByUsername(context.Background(), username).Return(u, nil)

				return u.ToEntity()
			},
			assertResult: func(t *testing.T, want, got entity.User) { require.Equal(t, want, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "an unknown username becomes a NotFoundError",
			args: args{
				ctx:      context.Background(),
				username: username,
			},
			mock: func(m *mocks.MockStorage) entity.User {
				m.EXPECT().FindUserByUsername(context.Background(), username).Return(model.User{}, sql.ErrNoRows)

				return entity.User{}
			},
			assertResult: func(t *testing.T, want, got entity.User) {},
			assertErr: func(t *testing.T, err error) {
				var notFound *domainerror.NotFoundError
				require.ErrorAs(t, err, &notFound)
			},
		},
		{
			name: "a storage error is propagated",
			args: args{
				ctx:      context.Background(),
				username: username,
			},
			mock: func(m *mocks.MockStorage) entity.User {
				m.EXPECT().FindUserByUsername(context.Background(), username).Return(model.User{}, errStub)

				return entity.User{}
			},
			assertResult: func(t *testing.T, want, got entity.User) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			want := tt.mock(m)

			got, err := r.FindByUsername(tt.args.ctx, tt.args.username)
			tt.assertErr(t, err)
			tt.assertResult(t, want, got)
		})
	}
}

// TestRepository_FindByID covers the lookup, including the id -> NotFoundError translation.
func TestRepository_FindByID(t *testing.T) {
	t.Parallel()

	id := fakeID()

	type args struct {
		ctx context.Context
		id  uint64
	}

	tests := []struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage) entity.User
		assertResult func(t *testing.T, want, got entity.User)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "finds the user",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) entity.User {
				u := fakeUserModel()
				u.ID = id
				m.EXPECT().FindUserByID(context.Background(), id).Return(u, nil)

				return u.ToEntity()
			},
			assertResult: func(t *testing.T, want, got entity.User) { require.Equal(t, want, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "an unknown id becomes a NotFoundError",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) entity.User {
				m.EXPECT().FindUserByID(context.Background(), id).Return(model.User{}, sql.ErrNoRows)

				return entity.User{}
			},
			assertResult: func(t *testing.T, want, got entity.User) {},
			assertErr: func(t *testing.T, err error) {
				var notFound *domainerror.NotFoundError
				require.ErrorAs(t, err, &notFound)
			},
		},
		{
			name: "a storage error is propagated",
			args: args{
				ctx: context.Background(),
				id:  id,
			},
			mock: func(m *mocks.MockStorage) entity.User {
				m.EXPECT().FindUserByID(context.Background(), id).Return(model.User{}, errStub)

				return entity.User{}
			},
			assertResult: func(t *testing.T, want, got entity.User) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			want := tt.mock(m)

			got, err := r.FindByID(tt.args.ctx, tt.args.id)
			tt.assertErr(t, err)
			tt.assertResult(t, want, got)
		})
	}
}

// TestRepository_Create covers the insert and the username -> ConflictError translation, the one
// error shape this repository is allowed to recognize.
func TestRepository_Create(t *testing.T) {
	t.Parallel()

	username, hash := fakeUsername(), fakeHash()
	settings := entity.UserSettings{
		Language:            fakeLang(),
		LocationFilterDepth: fakeDepth(),
	}

	type args struct {
		ctx      context.Context
		username string
		hash     string
		settings entity.UserSettings
	}

	type testCase struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage) uint64
		assertResult func(t *testing.T, wantID, got uint64)
		assertErr    func(t *testing.T, err error)
	}

	var tests []testCase

	{
		tests = append(tests, testCase{
			name: "stores the user and returns its id",
			args: args{
				ctx:      context.Background(),
				username: username,
				hash:     hash,
				settings: settings,
			},
			mock: func(m *mocks.MockStorage) uint64 {
				wantID := fakeID()
				m.EXPECT().
					CreateUser(context.Background(), username, hash, model.UserSettingsFromEntity(settings)).
					Return(wantID, nil)

				return wantID
			},
			assertResult: func(t *testing.T, wantID, got uint64) { require.Equal(t, wantID, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		})
	}

	{
		tests = append(tests, testCase{
			name: "a taken username becomes a ConflictError",
			args: args{
				ctx:      context.Background(),
				username: username,
				hash:     hash,
				settings: settings,
			},
			mock: func(m *mocks.MockStorage) uint64 {
				m.EXPECT().
					CreateUser(context.Background(), username, hash, model.UserSettingsFromEntity(settings)).
					Return(uint64(0), storageError.UniqueViolationError)

				return 0
			},
			assertResult: func(t *testing.T, wantID, got uint64) {},
			assertErr: func(t *testing.T, err error) {
				var conflict *domainerror.ConflictError
				require.ErrorAs(t, err, &conflict)
			},
		})
	}

	{
		tests = append(tests, testCase{
			name: "any other storage error is propagated",
			args: args{
				ctx:      context.Background(),
				username: username,
				hash:     hash,
				settings: settings,
			},
			mock: func(m *mocks.MockStorage) uint64 {
				m.EXPECT().
					CreateUser(context.Background(), username, hash, model.UserSettingsFromEntity(settings)).
					Return(uint64(0), errStub)

				return 0
			},
			assertResult: func(t *testing.T, wantID, got uint64) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			wantID := tt.mock(m)

			got, err := r.Create(tt.args.ctx, port.UserCreateRequest{
				Username: tt.args.username, PasswordHash: tt.args.hash, Settings: tt.args.settings,
			})
			tt.assertErr(t, err)
			tt.assertResult(t, wantID, got)
		})
	}
}

// TestRepository_UpdateUsername covers the rename and the username -> ConflictError translation.
func TestRepository_UpdateUsername(t *testing.T) {
	t.Parallel()

	id := fakeID()
	username := fakeUsername()

	type args struct {
		ctx      context.Context
		id       uint64
		username string
	}

	tests := []struct {
		name      string
		args      args
		mock      func(m *mocks.MockStorage)
		assertErr func(t *testing.T, err error)
	}{
		{
			name: "renames the user",
			args: args{
				ctx:      context.Background(),
				id:       id,
				username: username,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUsername(context.Background(), id, username).Return(nil)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a taken username becomes a ConflictError",
			args: args{
				ctx:      context.Background(),
				id:       id,
				username: username,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUsername(context.Background(), id, username).Return(storageError.UniqueViolationError)
			},
			assertErr: func(t *testing.T, err error) {
				var conflict *domainerror.ConflictError
				require.ErrorAs(t, err, &conflict)
			},
		},
		{
			name: "any other storage error is propagated",
			args: args{
				ctx:      context.Background(),
				id:       id,
				username: username,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUsername(context.Background(), id, username).Return(errStub)
			},
			assertErr: func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			tt.mock(m)

			err := r.UpdateUsername(tt.args.ctx, tt.args.id, tt.args.username)
			tt.assertErr(t, err)
		})
	}
}

// TestRepository_UpdatePasswordHash covers the plain delegation to storage.
func TestRepository_UpdatePasswordHash(t *testing.T) {
	t.Parallel()

	id := fakeID()
	hash := fakeHash()

	type args struct {
		ctx  context.Context
		id   uint64
		hash string
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
				ctx:  context.Background(),
				id:   id,
				hash: hash,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUserPasswordHash(context.Background(), id, hash).Return(nil)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a storage error is propagated",
			args: args{
				ctx:  context.Background(),
				id:   id,
				hash: hash,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUserPasswordHash(context.Background(), id, hash).Return(errStub)
			},
			assertErr: func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			tt.mock(m)

			err := r.UpdatePasswordHash(tt.args.ctx, tt.args.id, tt.args.hash)
			tt.assertErr(t, err)
		})
	}
}

// TestRepository_UpdateLanguage covers the plain delegation to storage.
func TestRepository_UpdateLanguage(t *testing.T) {
	t.Parallel()

	id := fakeID()
	lang := fakeLang()

	type args struct {
		ctx  context.Context
		id   uint64
		lang string
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
				ctx:  context.Background(),
				id:   id,
				lang: lang,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUserLanguage(context.Background(), id, lang).Return(nil)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a storage error is propagated",
			args: args{
				ctx:  context.Background(),
				id:   id,
				lang: lang,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUserLanguage(context.Background(), id, lang).Return(errStub)
			},
			assertErr: func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			tt.mock(m)

			err := r.UpdateLanguage(tt.args.ctx, tt.args.id, tt.args.lang)
			tt.assertErr(t, err)
		})
	}
}

// TestRepository_UpdateLocationFilterDepth covers the plain delegation to storage.
func TestRepository_UpdateLocationFilterDepth(t *testing.T) {
	t.Parallel()

	id := fakeID()
	depth := fakeDepth()

	type args struct {
		ctx   context.Context
		id    uint64
		depth int
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
				id:    id,
				depth: depth,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUserLocationFilterDepth(context.Background(), id, depth).Return(nil)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a storage error is propagated",
			args: args{
				ctx:   context.Background(),
				id:    id,
				depth: depth,
			},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().UpdateUserLocationFilterDepth(context.Background(), id, depth).Return(errStub)
			},
			assertErr: func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			tt.mock(m)

			err := r.UpdateLocationFilterDepth(tt.args.ctx, tt.args.id, tt.args.depth)
			tt.assertErr(t, err)
		})
	}
}

// TestRepository_Count covers the plain delegation to storage.
func TestRepository_Count(t *testing.T) {
	t.Parallel()

	want := gofakeit.Number(0, 500)

	type args struct {
		ctx context.Context
	}

	tests := []struct {
		name         string
		args         args
		mock         func(m *mocks.MockStorage)
		assertResult func(t *testing.T, got int)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "delegates to storage",
			args: args{ctx: context.Background()},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().CountUsers(context.Background()).Return(want, nil)
			},
			assertResult: func(t *testing.T, got int) { require.Equal(t, want, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a storage error is propagated",
			args: args{ctx: context.Background()},
			mock: func(m *mocks.MockStorage) {
				m.EXPECT().CountUsers(context.Background()).Return(0, errStub)
			},
			assertResult: func(t *testing.T, got int) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, m := newRepo(t)
			tt.mock(m)

			got, err := r.Count(tt.args.ctx)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"

	storageError "wherewhat/internal/storage/error"
	"wherewhat/internal/storage/model"
)

// fakeUserSettings returns a random settings value — the model.UserSettings this package's queries
// bind and scan through the settings JSONB column.
func fakeUserSettings() model.UserSettings {
	return model.UserSettings{
		Language:            gofakeit.LanguageAbbreviation(),
		LocationFilterDepth: gofakeit.Number(0, 10),
	}
}

// mustJSON encodes v the way model.UserSettings.Value does, for building the exact driver.Value a
// mocked expectation has to match. A plain UserSettings struct can't fail to marshal.
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	return b
}

// testFindUser runs the shared "finds the row / miss is sql.ErrNoRows / driver error propagates"
// cases behind TestFindUserByUsername and TestFindUserByID, which differ only in which column they
// filter on.
func testFindUser(
	t *testing.T,
	query string,
	missingCase string,
	arg func(id uint64, username string) any,
	call func(s *Storage, id uint64, username string) (model.User, error),
) {
	t.Helper()

	id := fakeID()
	username := fakeUsername()
	hash := fakeHash()
	settings := fakeUserSettings()
	a := arg(id, username)

	tests := []struct {
		name         string
		mock         func(mock sqlmock.Sqlmock)
		assertResult func(t *testing.T, got model.User)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "finds the row",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(a).WillReturnRows(
					sqlmock.NewRows([]string{
						"id",
						"username",
						"settings",
						"password_hash",
					}).
						AddRow(id, username, mustJSON(settings), hash),
				)
			},
			assertResult: func(t *testing.T, got model.User) {
				require.Equal(t, model.User{
					ID:           id,
					Username:     username,
					PasswordHash: hash,
					Settings:     settings,
				}, got)
			},
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: missingCase,
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(a).WillReturnError(sql.ErrNoRows)
			},
			assertResult: func(t *testing.T, got model.User) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, sql.ErrNoRows) },
		},
		{
			name: "a driver error is propagated",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(a).WillReturnError(errStub)
			},
			assertResult: func(t *testing.T, got model.User) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)
			tt.mock(mock)

			got, err := call(s, id, username)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

// TestFindUserByUsername covers the login lookup: the row comes back whole, and a miss is reported as
// sql.ErrNoRows rather than a zero-valued user.
//
// Not covered here: Postgres's default collation on this column makes the real lookup (and its
// UNIQUE constraint, see TestCreateUser) case-sensitive, like SQLite's and unlike MySQL's
// utf8mb4_unicode_ci. That behavior belongs to the server, not this adapter's Go code, so a mock —
// which just returns whatever row it's told to — can't exercise it; it'd need an integration test
// against a real server.
func TestFindUserByUsername(t *testing.T) {
	t.Parallel()

	testFindUser(t,
		`SELECT id, username, settings, password_hash FROM users WHERE username = $1`,
		"an unknown username is sql.ErrNoRows",
		func(_ uint64, username string) any { return username },
		func(s *Storage, _ uint64, username string) (model.User, error) {
			return s.FindUserByUsername(context.Background(), username)
		},
	)
}

// TestFindUserByID covers the same lookup by primary key, which is what every authenticated request
// goes through.
func TestFindUserByID(t *testing.T) {
	t.Parallel()

	testFindUser(t,
		`SELECT id, username, settings, password_hash FROM users WHERE id = $1`,
		"an unknown id is sql.ErrNoRows",
		func(id uint64, _ string) any { return id },
		func(s *Storage, id uint64, _ string) (model.User, error) {
			return s.FindUserByID(context.Background(), id)
		},
	)
}

// TestCreateUser covers the insert, the settings JSONB binding, and the one error the repositories
// act on: a taken username, which has to arrive as storageError.UniqueViolationError and not as a raw
// driver error.
func TestCreateUser(t *testing.T) {
	t.Parallel()

	query := `INSERT INTO users (username, password_hash, settings) VALUES ($1, $2, $3) RETURNING id`

	username, hash := fakeUsername(), fakeHash()
	settings := fakeUserSettings()
	wantID := fakeID()

	tests := []struct {
		name         string
		mock         func(mock sqlmock.Sqlmock)
		assertResult func(t *testing.T, got uint64)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "stores the settings alongside the credentials",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(username, hash, mustJSON(settings)).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(wantID))
			},
			assertResult: func(t *testing.T, got uint64) { require.Equal(t, wantID, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a taken username is a unique violation",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(username, hash, mustJSON(settings)).
					WillReturnError(pgErr(pgUniqueViolation))
			},
			assertResult: func(t *testing.T, got uint64) {},
			assertErr: func(
				t *testing.T, err error,
			) {
				require.ErrorIs(t, err, storageError.UniqueViolationError)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)
			tt.mock(mock)

			id, err := s.CreateUser(context.Background(), username, hash, settings)
			tt.assertErr(t, err)
			tt.assertResult(t, id)
		})
	}
}

// TestUpdateUsername covers the rename: a taken username has to come back as
// storageError.UniqueViolationError, and an id that doesn't exist is a no-op rather than an error.
func TestUpdateUsername(t *testing.T) {
	t.Parallel()

	query := `UPDATE users SET username = $1 WHERE id = $2`
	id := fakeID()
	newUsername := fakeUsername()

	tests := []struct {
		name      string
		res       sql.Result
		mockErr   error
		assertErr func(t *testing.T, err error)
	}{
		{
			name:      "renames the user",
			res:       sqlmock.NewResult(0, 1),
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:      "an unknown id is not an error",
			res:       sqlmock.NewResult(0, 0),
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:    "a taken username is a unique violation",
			mockErr: pgErr(pgUniqueViolation),
			assertErr: func(
				t *testing.T, err error,
			) {
				require.ErrorIs(t, err, storageError.UniqueViolationError)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)

			exp := mock.ExpectExec(query).WithArgs(newUsername, id)
			if tt.mockErr != nil {
				exp.WillReturnError(tt.mockErr)
			} else {
				exp.WillReturnResult(tt.res)
			}

			err := s.UpdateUsername(context.Background(), id, newUsername)
			tt.assertErr(t, err)
		})
	}
}

// TestUpdateUserPasswordHash covers the password change; like every other update here, an id that
// doesn't exist is a no-op rather than an error.
func TestUpdateUserPasswordHash(t *testing.T) {
	t.Parallel()

	query := `UPDATE users SET password_hash = $1 WHERE id = $2`
	id := fakeID()
	hash := fakeHash()

	tests := []struct {
		name      string
		res       sql.Result
		mockErr   error
		assertErr func(t *testing.T, err error)
	}{
		{
			name:      "overwrites the stored hash",
			res:       sqlmock.NewResult(0, 1),
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:      "an unknown id changes nothing, not an error",
			res:       sqlmock.NewResult(0, 0),
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:      "a driver error is propagated",
			mockErr:   errStub,
			assertErr: func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)

			exp := mock.ExpectExec(query).WithArgs(hash, id)
			if tt.mockErr != nil {
				exp.WillReturnError(tt.mockErr)
			} else {
				exp.WillReturnResult(tt.res)
			}

			err := s.UpdateUserPasswordHash(context.Background(), id, hash)
			tt.assertErr(t, err)
		})
	}
}

// TestUpdateUserLanguage covers the jsonb_set update: the right path ('{language}') and value —
// explicitly cast to text via to_jsonb($1::text), since jsonb_set needs a jsonb replacement value, not
// a bare parameter — are bound into the query. Whether jsonb_set actually merges into the existing
// settings object rather than clobbering it is Postgres's own behavior — verifying the merge itself
// needs a real server, out of reach for a mock that only ever reports "the statement ran".
func TestUpdateUserLanguage(t *testing.T) {
	t.Parallel()

	query := `UPDATE users SET settings = jsonb_set(settings, '{language}', to_jsonb($1::text)) WHERE id = $2`
	id := fakeID()
	lang := gofakeit.LanguageAbbreviation()

	tests := []struct {
		name      string
		mockErr   error
		assertErr func(t *testing.T, err error)
	}{
		{
			name:      "sets the language",
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:      "a driver error is propagated",
			mockErr:   errStub,
			assertErr: func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)

			exp := mock.ExpectExec(query).WithArgs(lang, id)
			if tt.mockErr != nil {
				exp.WillReturnError(tt.mockErr)
			} else {
				exp.WillReturnResult(sqlmock.NewResult(0, 1))
			}

			err := s.UpdateUserLanguage(context.Background(), id, lang)
			tt.assertErr(t, err)
		})
	}
}

// TestUpdateUserLocationFilterDepth is the same jsonb_set path for the numeric setting — worth its
// own test because the value is cast to_jsonb($1::int), where the language is cast ::text.
func TestUpdateUserLocationFilterDepth(t *testing.T) {
	t.Parallel()

	query := `UPDATE users SET settings = jsonb_set(settings, '{locationFilterDepth}', to_jsonb($1::int)) WHERE id = $2`
	id := fakeID()
	depth := gofakeit.Number(0, 10)

	tests := []struct {
		name      string
		mockErr   error
		assertErr func(t *testing.T, err error)
	}{
		{
			name:      "sets the depth",
			assertErr: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:      "a driver error is propagated",
			mockErr:   errStub,
			assertErr: func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)

			exp := mock.ExpectExec(query).WithArgs(depth, id)
			if tt.mockErr != nil {
				exp.WillReturnError(tt.mockErr)
			} else {
				exp.WillReturnResult(sqlmock.NewResult(0, 1))
			}

			err := s.UpdateUserLocationFilterDepth(context.Background(), id, depth)
			tt.assertErr(t, err)
		})
	}
}

// TestCountUsers covers the count the first-run seeding decision is made on.
func TestCountUsers(t *testing.T) {
	t.Parallel()

	query := `SELECT COUNT(*) FROM users`
	want := gofakeit.Number(0, 1000)

	tests := []struct {
		name         string
		mock         func(mock sqlmock.Sqlmock)
		assertResult func(t *testing.T, got int)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "counts the users",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(want))
			},
			assertResult: func(t *testing.T, got int) { require.Equal(t, want, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:         "a driver error is propagated",
			mock:         func(mock sqlmock.Sqlmock) { mock.ExpectQuery(query).WillReturnError(errStub) },
			assertResult: func(t *testing.T, got int) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)
			tt.mock(mock)

			got, err := s.CountUsers(context.Background())
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

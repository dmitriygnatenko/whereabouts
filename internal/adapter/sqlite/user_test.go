package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/brianvoe/gofakeit/v7"

	storageError "wherewhat/internal/storage/error"
	"wherewhat/internal/storage/model"
)

// fakeUserSettings returns a random settings value — the model.UserSettings this package's queries
// bind and scan through the settings JSON column.
func fakeUserSettings() model.UserSettings {
	return model.UserSettings{Language: gofakeit.LanguageAbbreviation(), LocationFilterDepth: gofakeit.Number(0, 10)}
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

// TestFindUserByUsername covers the login lookup: the row comes back whole, and a miss is reported as
// sql.ErrNoRows rather than a zero-valued user.
//
// Not covered here: SQLite's binary collation on this column makes the real lookup (and its UNIQUE
// constraint, see TestCreateUser) case-sensitive. That behavior belongs to the driver, not this
// adapter's Go code, so a mock — which just returns whatever row it's told to — can't exercise it;
// it'd need an integration test against a real database file.
func TestFindUserByUsername(t *testing.T) {
	query := `SELECT id, username, settings, password_hash FROM users WHERE username = ?`

	id := fakeID()
	username := fakeUsername()
	hash := fakeHash()
	settings := fakeUserSettings()

	tests := []struct {
		name    string
		mock    func(mock sqlmock.Sqlmock)
		wantErr error
	}{
		{
			name: "finds the row",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(username).WillReturnRows(
					sqlmock.NewRows([]string{"id", "username", "settings", "password_hash"}).
						AddRow(id, username, mustJSON(settings), hash),
				)
			},
		},
		{
			name:    "an unknown username is sql.ErrNoRows",
			mock:    func(mock sqlmock.Sqlmock) { mock.ExpectQuery(query).WithArgs(username).WillReturnError(sql.ErrNoRows) },
			wantErr: sql.ErrNoRows,
		},
		{
			name:    "a driver error is propagated",
			mock:    func(mock sqlmock.Sqlmock) { mock.ExpectQuery(query).WithArgs(username).WillReturnError(errStub) },
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)
			tt.mock(mock)

			got, err := s.FindUserByUsername(context.Background(), username)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("FindUserByUsername() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("FindUserByUsername() error = %v", err)
			}

			want := model.User{ID: id, Username: username, PasswordHash: hash, Settings: settings}
			if got != want {
				t.Fatalf("FindUserByUsername() = %+v, want %+v", got, want)
			}
		})
	}
}

// TestFindUserByID covers the same lookup by primary key, which is what every authenticated request
// goes through.
func TestFindUserByID(t *testing.T) {
	query := `SELECT id, username, settings, password_hash FROM users WHERE id = ?`

	id := fakeID()
	username := fakeUsername()
	hash := fakeHash()
	settings := fakeUserSettings()

	tests := []struct {
		name    string
		mock    func(mock sqlmock.Sqlmock)
		wantErr error
	}{
		{
			name: "finds the row",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(id).WillReturnRows(
					sqlmock.NewRows([]string{"id", "username", "settings", "password_hash"}).
						AddRow(id, username, mustJSON(settings), hash),
				)
			},
		},
		{
			name:    "an unknown id is sql.ErrNoRows",
			mock:    func(mock sqlmock.Sqlmock) { mock.ExpectQuery(query).WithArgs(id).WillReturnError(sql.ErrNoRows) },
			wantErr: sql.ErrNoRows,
		},
		{
			name:    "a driver error is propagated",
			mock:    func(mock sqlmock.Sqlmock) { mock.ExpectQuery(query).WithArgs(id).WillReturnError(errStub) },
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)
			tt.mock(mock)

			got, err := s.FindUserByID(context.Background(), id)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("FindUserByID() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("FindUserByID() error = %v", err)
			}

			want := model.User{ID: id, Username: username, PasswordHash: hash, Settings: settings}
			if got != want {
				t.Fatalf("FindUserByID() = %+v, want %+v", got, want)
			}
		})
	}
}

// TestCreateUser covers the insert, the settings JSON binding, and the one error the repositories act
// on: a taken username, which has to arrive as storageError.UniqueViolationError and not as a raw
// driver error.
func TestCreateUser(t *testing.T) {
	query := `INSERT INTO users (username, password_hash, settings) VALUES (?, ?, ?)`

	username, hash := fakeUsername(), fakeHash()
	settings := fakeUserSettings()
	wantID := int64(fakeID())

	tests := []struct {
		name       string
		mock       func(mock sqlmock.Sqlmock)
		wantUnique bool
	}{
		{
			name: "stores the settings alongside the credentials",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(query).WithArgs(username, hash, mustJSON(settings)).
					WillReturnResult(sqlmock.NewResult(wantID, 1))
			},
		},
		{
			name: "a taken username is a unique violation",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(query).WithArgs(username, hash, mustJSON(settings)).
					WillReturnError(sqliteUniqueErr())
			},
			wantUnique: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)
			tt.mock(mock)

			id, err := s.CreateUser(context.Background(), username, hash, settings)
			if tt.wantUnique {
				if !errors.Is(err, storageError.UniqueViolationError) {
					t.Fatalf("CreateUser() error = %v, want a unique violation", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("CreateUser() error = %v", err)
			}

			if id != uint64(wantID) {
				t.Fatalf("CreateUser() = %d, want %d", id, wantID)
			}
		})
	}
}

// TestUpdateUsername covers the rename: a taken username has to come back as
// storageError.UniqueViolationError, and an id that doesn't exist is a no-op rather than an error.
func TestUpdateUsername(t *testing.T) {
	query := `UPDATE users SET username = ? WHERE id = ?`
	id := fakeID()
	newUsername := fakeUsername()

	tests := []struct {
		name       string
		res        sql.Result
		mockErr    error
		wantUnique bool
	}{
		{name: "renames the user", res: sqlmock.NewResult(0, 1)},
		{name: "an unknown id is not an error", res: sqlmock.NewResult(0, 0)},
		{name: "a taken username is a unique violation", mockErr: sqliteUniqueErr(), wantUnique: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)

			exp := mock.ExpectExec(query).WithArgs(newUsername, id)
			if tt.mockErr != nil {
				exp.WillReturnError(tt.mockErr)
			} else {
				exp.WillReturnResult(tt.res)
			}

			err := s.UpdateUsername(context.Background(), id, newUsername)
			if tt.wantUnique {
				if !errors.Is(err, storageError.UniqueViolationError) {
					t.Fatalf("UpdateUsername() error = %v, want a unique violation", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("UpdateUsername() error = %v", err)
			}
		})
	}
}

// TestUpdateUserPasswordHash covers the password change; like every other update here, an id that
// doesn't exist is a no-op rather than an error.
func TestUpdateUserPasswordHash(t *testing.T) {
	query := `UPDATE users SET password_hash = ? WHERE id = ?`
	id := fakeID()
	hash := fakeHash()

	tests := []struct {
		name    string
		res     sql.Result
		mockErr error
		wantErr bool
	}{
		{name: "overwrites the stored hash", res: sqlmock.NewResult(0, 1)},
		{name: "an unknown id changes nothing, not an error", res: sqlmock.NewResult(0, 0)},
		{name: "a driver error is propagated", mockErr: errStub, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)

			exp := mock.ExpectExec(query).WithArgs(hash, id)
			if tt.mockErr != nil {
				exp.WillReturnError(tt.mockErr)
			} else {
				exp.WillReturnResult(tt.res)
			}

			err := s.UpdateUserPasswordHash(context.Background(), id, hash)
			if tt.wantErr {
				if !errors.Is(err, errStub) {
					t.Fatalf("UpdateUserPasswordHash() error = %v, want %v", err, errStub)
				}

				return
			}

			if err != nil {
				t.Fatalf("UpdateUserPasswordHash() error = %v", err)
			}
		})
	}
}

// TestUpdateUserLanguage covers the JSON_SET update: the right path ('$.language') and value are
// bound into the query. Whether json_set actually merges into the existing settings object rather
// than clobbering it is SQLite's own behavior — verifying the merge itself needs a real database file,
// out of reach for a mock that only ever reports "the statement ran".
func TestUpdateUserLanguage(t *testing.T) {
	query := `UPDATE users SET settings = JSON_SET(settings, '$.language', ?) WHERE id = ?`
	id := fakeID()
	lang := gofakeit.LanguageAbbreviation()

	tests := []struct {
		name    string
		mockErr error
		wantErr bool
	}{
		{name: "sets the language"},
		{name: "a driver error is propagated", mockErr: errStub, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)

			exp := mock.ExpectExec(query).WithArgs(lang, id)
			if tt.mockErr != nil {
				exp.WillReturnError(tt.mockErr)
			} else {
				exp.WillReturnResult(sqlmock.NewResult(0, 1))
			}

			err := s.UpdateUserLanguage(context.Background(), id, lang)
			if tt.wantErr {
				if !errors.Is(err, errStub) {
					t.Fatalf("UpdateUserLanguage() error = %v, want %v", err, errStub)
				}

				return
			}

			if err != nil {
				t.Fatalf("UpdateUserLanguage() error = %v", err)
			}
		})
	}
}

// TestUpdateUserLocationFilterDepth is the same JSON_SET path for the numeric setting — worth its own
// test because the value is bound as a number, where the language is a string.
func TestUpdateUserLocationFilterDepth(t *testing.T) {
	query := `UPDATE users SET settings = JSON_SET(settings, '$.locationFilterDepth', ?) WHERE id = ?`
	id := fakeID()
	depth := gofakeit.Number(0, 10)

	tests := []struct {
		name    string
		mockErr error
		wantErr bool
	}{
		{name: "sets the depth"},
		{name: "a driver error is propagated", mockErr: errStub, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)

			exp := mock.ExpectExec(query).WithArgs(depth, id)
			if tt.mockErr != nil {
				exp.WillReturnError(tt.mockErr)
			} else {
				exp.WillReturnResult(sqlmock.NewResult(0, 1))
			}

			err := s.UpdateUserLocationFilterDepth(context.Background(), id, depth)
			if tt.wantErr {
				if !errors.Is(err, errStub) {
					t.Fatalf("UpdateUserLocationFilterDepth() error = %v, want %v", err, errStub)
				}

				return
			}

			if err != nil {
				t.Fatalf("UpdateUserLocationFilterDepth() error = %v", err)
			}
		})
	}
}

// TestCountUsers covers the count the first-run seeding decision is made on.
func TestCountUsers(t *testing.T) {
	query := `SELECT COUNT(*) FROM users`
	want := gofakeit.Number(0, 1000)

	tests := []struct {
		name    string
		mock    func(mock sqlmock.Sqlmock)
		want    int
		wantErr bool
	}{
		{
			name: "counts the users",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(want))
			},
			want: want,
		},
		{
			name:    "a driver error is propagated",
			mock:    func(mock sqlmock.Sqlmock) { mock.ExpectQuery(query).WillReturnError(errStub) },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)
			tt.mock(mock)

			got, err := s.CountUsers(context.Background())
			if tt.wantErr {
				if !errors.Is(err, errStub) {
					t.Fatalf("CountUsers() error = %v, want %v", err, errStub)
				}

				return
			}

			if err != nil {
				t.Fatalf("CountUsers() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf("CountUsers() = %d, want %d", got, tt.want)
			}
		})
	}
}

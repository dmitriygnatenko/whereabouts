package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/brianvoe/gofakeit/v7"

	"modernc.org/sqlite"

	storageError "wherewhat/internal/storage/error"
)

// newMock returns a Storage backed by sqlmock instead of a real database file: every query the
// adapter issues has to be told what to expect ahead of time, and the test fails on any call that
// wasn't expected or any expectation that went unmet.
func newMock(t *testing.T) (*Storage, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}

	t.Cleanup(func() { _ = db.Close() })
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sqlmock expectations: %v", err)
		}
	})

	return &Storage{DB: db}, mock
}

// sqliteUniqueErr and sqliteForeignKeyErr return real *sqlite.Error values for the two constraint
// violations these tests need to simulate. modernc.org/sqlite doesn't export a constructor for its
// Error type (its fields are private, see error.go) — running a real statement engineered to fail is
// the only way to get one, so primeSQLiteErrs mints both once, against a throwaway in-memory database,
// and every test reuses the result.
var (
	sqliteErrOnce          sync.Once
	sqliteUniqueErrVal     *sqlite.Error
	sqliteForeignKeyErrVal *sqlite.Error
)

func primeSQLiteErrs() {
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE parent (id INTEGER PRIMARY KEY)`); err != nil {
		panic(err)
	}

	if _, err := db.Exec(`CREATE TABLE child (
		id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES parent (id)
	)`); err != nil {
		panic(err)
	}

	if _, err := db.Exec(`CREATE TABLE uniq (val INTEGER UNIQUE)`); err != nil {
		panic(err)
	}

	if _, err := db.Exec(`INSERT INTO child (id, parent_id) VALUES (1, 4242)`); !errors.As(err, &sqliteForeignKeyErrVal) {
		panic("expected a *sqlite.Error for the foreign key violation, got " + errString(err))
	}

	if _, err := db.Exec(`INSERT INTO uniq (val) VALUES (1)`); err != nil {
		panic(err)
	}

	if _, err := db.Exec(`INSERT INTO uniq (val) VALUES (1)`); !errors.As(err, &sqliteUniqueErrVal) {
		panic("expected a *sqlite.Error for the unique violation, got " + errString(err))
	}
}

func errString(err error) string {
	if err == nil {
		return "<nil>"
	}

	return err.Error()
}

// sqliteUniqueErr is the *sqlite.Error a UNIQUE constraint violation surfaces as.
func sqliteUniqueErr() *sqlite.Error {
	sqliteErrOnce.Do(primeSQLiteErrs)

	return sqliteUniqueErrVal
}

// sqliteForeignKeyErr is the *sqlite.Error a foreign key violation surfaces as — both an insert/update
// referencing a row that doesn't exist, and a delete blocked by ON DELETE RESTRICT.
func sqliteForeignKeyErr() *sqlite.Error {
	sqliteErrOnce.Do(primeSQLiteErrs)

	return sqliteForeignKeyErrVal
}

// fakeID returns a random id in a range that survives the uint64->int64 conversion database/sql
// applies to query arguments, so it's safe to bind directly.
func fakeID() uint64 { return uint64(gofakeit.Number(1, 1_000_000)) }

// fakeUsername, fakeHash, fakeName, fakeNotes, fakeColor and fakeToken are the field-shaped random
// values the tests below bind into queries and mocked rows, so a test failure is never masked by two
// cases accidentally sharing a fixture value.
func fakeUsername() string { return gofakeit.Username() }
func fakeHash() string     { return gofakeit.LetterN(60) }
func fakeName() string     { return gofakeit.AppName() }
func fakeNotes() string    { return gofakeit.Sentence() }
func fakeColor() string    { return gofakeit.HexColor() }
func fakeToken() string    { return gofakeit.UUID() }
func fakeURL() string      { return gofakeit.URL() }

// fakeTime returns a random timestamp. Unlike MySQL's TIMESTAMP columns, SQLite's TEXT-backed
// timestamps carry full time.Time precision, so — unlike the mysql adapter's equivalent — there's no
// need to truncate to the second here.
func fakeTime() time.Time { return gofakeit.Date().UTC() }

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = errors.New(gofakeit.Sentence())

// TestAffected pins the translation from a driver Result to the "found?" answer the storage layer
// gives updates and deletes.
func TestAffected(t *testing.T) {
	rowsTouched := int64(gofakeit.Number(1, 1000))

	tests := []struct {
		name    string
		res     sql.Result
		want    bool
		wantErr bool
	}{
		{name: "no rows touched means not found", res: sqlmock.NewResult(0, 0)},
		{name: "rows touched means found", res: sqlmock.NewResult(0, rowsTouched), want: true},
		{name: "the driver error is propagated", res: sqlmock.NewErrorResult(errStub), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := affected(tt.res)
			if tt.wantErr {
				if !errors.Is(err, errStub) {
					t.Fatalf("affected() error = %v, want %v", err, errStub)
				}

				return
			}

			if err != nil {
				t.Fatalf("affected() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf("affected() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestInsertReturningID checks that the generated id comes back from LastInsertId, and that a failing
// statement reports the error instead of a zero id.
func TestInsertReturningID(t *testing.T) {
	query := `INSERT INTO locations (title, color, parent_id, created_at) VALUES (?, ?, ?, ?)`
	name, color, createdAt := fakeName(), fakeColor(), fakeTime()
	wantID := int64(fakeID())

	tests := []struct {
		name    string
		mock    func(mock sqlmock.Sqlmock)
		wantErr bool
	}{
		{
			name: "the id comes back from LastInsertId",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(query).
					WithArgs(name, color, nil, createdAt).
					WillReturnResult(sqlmock.NewResult(wantID, 1))
			},
		},
		{
			name: "a failing statement returns its error",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(query).
					WithArgs(name, color, nil, createdAt).
					WillReturnError(errStub)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)
			tt.mock(mock)

			got, err := s.insertReturningID(context.Background(), query, name, color, nil, createdAt)
			if tt.wantErr {
				if !errors.Is(err, errStub) {
					t.Fatalf("insertReturningID() error = %v, want %v", err, errStub)
				}

				return
			}

			if err != nil {
				t.Fatalf("insertReturningID() error = %v", err)
			}

			if got != uint64(wantID) {
				t.Fatalf("insertReturningID() = %d, want %d", got, wantID)
			}
		})
	}
}

// TestWrapUnique covers the one error shape the repositories are allowed to recognize: a UNIQUE
// violation, normalized to storageError.UniqueViolationError with the driver error still wrapped
// inside. Every other error — including another SQLite constraint failure — has to pass through
// untouched, otherwise a repository would report "already taken" for an unrelated failure.
func TestWrapUnique(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantNil    bool
		wantUnique bool
	}{
		{name: "nil passes through", err: nil, wantNil: true},
		{name: "an unrelated error passes through", err: errStub},
		{name: "another constraint failure passes through", err: sqliteForeignKeyErr()},
		{name: "a unique violation is wrapped", err: sqliteUniqueErr(), wantUnique: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapUnique(tt.err)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("wrapUnique(nil) = %v, want nil", got)
				}

				return
			}

			if !errors.Is(got, tt.err) {
				t.Fatalf("wrapUnique(%v) = %v, dropped the original error", tt.err, got)
			}

			if isUnique := errors.Is(got, storageError.UniqueViolationError); isUnique != tt.wantUnique {
				t.Fatalf("wrapUnique(%v): unique violation = %v, want %v", tt.err, isUnique, tt.wantUnique)
			}

			var driverErr *sqlite.Error
			if tt.wantUnique && !errors.As(got, &driverErr) {
				t.Fatalf("wrapUnique(%v) = %v, want a *sqlite.Error still reachable via errors.As", tt.err, got)
			}
		})
	}
}

// TestEnsureDatabase covers the one thing it promises: the parent directory exists afterwards. The
// database file itself is the driver's job, on first connection.
func TestEnsureDatabase(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, tmp string)
		path    func(tmp string) string
		wantErr bool
	}{
		{
			name: "a missing parent directory is created",
			path: func(tmp string) string { return filepath.Join(tmp, "data", "nested", "app.db") },
		},
		{
			name: "an existing parent directory is left alone",
			path: func(tmp string) string { return filepath.Join(tmp, "app.db") },
		},
		{
			name: "a bare filename has no directory to create",
			path: func(tmp string) string { return "app.db" },
		},
		{
			name: "a file where the parent directory should be is an error",
			setup: func(t *testing.T, tmp string) {
				t.Helper()

				if err := os.WriteFile(filepath.Join(tmp, "data"), nil, 0o600); err != nil {
					t.Fatalf("writing the blocking file: %v", err)
				}
			},
			path:    func(tmp string) string { return filepath.Join(tmp, "data", "app.db") },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			if tt.setup != nil {
				tt.setup(t, tmp)
			}

			path := tt.path(tmp)

			err := EnsureDatabase(Config{Path: path})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("EnsureDatabase(%q) error = nil, want an error", path)
				}

				return
			}

			if err != nil {
				t.Fatalf("EnsureDatabase(%q) error = %v", path, err)
			}

			info, statErr := os.Stat(filepath.Dir(path))
			if statErr != nil || !info.IsDir() {
				t.Fatalf("EnsureDatabase(%q): parent directory not usable: %v", path, statErr)
			}
		})
	}
}

// TestOpen checks that a usable pool comes back for a workable path, that the single-connection cap
// is applied, and that an unusable path fails at Open rather than at the first query — Open pings,
// which is where a lazily-connecting driver would otherwise stay silent.
func TestOpen(t *testing.T) {
	tests := []struct {
		name    string
		path    func(tmp string) string
		wantErr bool
	}{
		{
			name: "creates and opens the database file",
			path: func(tmp string) string { return filepath.Join(tmp, "app.db") },
		},
		{
			name:    "a directory in place of the file is an error",
			path:    func(tmp string) string { return tmp },
			wantErr: true,
		},
		{
			name:    "a missing parent directory is an error",
			path:    func(tmp string) string { return filepath.Join(tmp, "missing", "app.db") },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.path(t.TempDir())

			s, err := Open(Config{Path: path, ConnMaxLifetime: time.Minute})
			if tt.wantErr {
				if err == nil {
					_ = s.Close()

					t.Fatalf("Open(%q) error = nil, want an error", path)
				}

				return
			}

			if err != nil {
				t.Fatalf("Open(%q) error = %v", path, err)
			}

			defer func() { _ = s.Close() }()

			if got := s.Stats().MaxOpenConnections; got != 1 {
				t.Fatalf("Open(%q) MaxOpenConnections = %d, want 1", path, got)
			}

			if _, statErr := os.Stat(path); statErr != nil {
				t.Fatalf("Open(%q) did not create the file: %v", path, statErr)
			}
		})
	}
}

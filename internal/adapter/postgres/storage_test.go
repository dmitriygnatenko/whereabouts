package postgres

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	storageError "wherewhat/internal/storage/error"
)

// pgForeignKeyViolation is the SQLSTATE for foreign_key_violation. Unlike pgUniqueViolation (see
// storage.go), wrapUnique doesn't treat this one specially; tests use it as "some other constraint
// failure".
const pgForeignKeyViolation = "23503"

// newMock returns a Storage backed by sqlmock instead of a real server: every query the adapter
// issues has to be told what to expect ahead of time, and the test fails on any call that wasn't
// expected or any expectation that went unmet.
func newMock(t *testing.T) (*Storage, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })
	t.Cleanup(func() {
		require.NoError(t, mock.ExpectationsWereMet(), "unmet sqlmock expectations")
	})

	return &Storage{DB: db}, mock
}

// pgErr builds a *pgconn.PgError carrying code — the field every switch in this package keys off —
// with a random message, since production code never inspects the message text.
func pgErr(code string) *pgconn.PgError {
	return &pgconn.PgError{Code: code, Message: gofakeit.Sentence()}
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

// fakeTime returns a random timestamp truncated to the microsecond — Postgres's TIMESTAMP columns
// (see migrate.go) don't carry sub-microsecond precision, so keeping nanosecond precision out of the
// fixture avoids tests asserting a precision the schema doesn't have.
func fakeTime() time.Time { return gofakeit.Date().UTC().Truncate(time.Microsecond) }

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = errors.New(gofakeit.Sentence())

// TestAffected pins the translation from a driver Result to the "found?" answer the storage layer
// gives updates and deletes.
func TestAffected(t *testing.T) {
	t.Parallel()

	rowsTouched := int64(gofakeit.Number(1, 1000))

	type args struct {
		res sql.Result
	}

	tests := []struct {
		name         string
		args         args
		assertResult func(t *testing.T, got bool)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name:         "no rows touched means not found",
			args:         args{res: sqlmock.NewResult(0, 0)},
			assertResult: func(t *testing.T, got bool) { require.False(t, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:         "rows touched means found",
			args:         args{res: sqlmock.NewResult(0, rowsTouched)},
			assertResult: func(t *testing.T, got bool) { require.True(t, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:         "the driver error is propagated",
			args:         args{res: sqlmock.NewErrorResult(errStub)},
			assertResult: func(t *testing.T, got bool) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := affected(tt.args.res)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

// TestInsertReturningID checks that the generated id comes back from the RETURNING id clause —
// Postgres drivers don't support LastInsertId, so insertReturningID scans it out of a query result
// instead of an exec result — and that a failing statement reports the error instead of a zero id.
func TestInsertReturningID(t *testing.T) {
	t.Parallel()

	query := `INSERT INTO locations (title, color, parent_id, created_at) VALUES ($1, $2, $3, $4) RETURNING id`
	name, color, createdAt := fakeName(), fakeColor(), fakeTime()
	wantID := fakeID()

	tests := []struct {
		name         string
		mock         func(mock sqlmock.Sqlmock)
		assertResult func(t *testing.T, got uint64)
		assertErr    func(t *testing.T, err error)
	}{
		{
			name: "the id comes back from the RETURNING clause",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).
					WithArgs(name, color, nil, createdAt).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(wantID))
			},
			assertResult: func(t *testing.T, got uint64) { require.Equal(t, wantID, got) },
			assertErr:    func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "a failing statement returns its error",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).
					WithArgs(name, color, nil, createdAt).
					WillReturnError(errStub)
			},
			assertResult: func(t *testing.T, got uint64) {},
			assertErr:    func(t *testing.T, err error) { require.ErrorIs(t, err, errStub) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, mock := newMock(t)
			tt.mock(mock)

			got, err := s.insertReturningID(context.Background(), query, name, color, nil, createdAt)
			tt.assertErr(t, err)
			tt.assertResult(t, got)
		})
	}
}

// TestWrapUnique covers the one error shape the repositories are allowed to recognize: a
// unique_violation (SQLSTATE 23505), normalized to storageError.UniqueViolationError with the driver
// error still wrapped inside. Every other error — including another constraint failure — has to pass
// through untouched, otherwise a repository would report "already taken" for an unrelated failure.
func TestWrapUnique(t *testing.T) {
	t.Parallel()

	fkViolation := pgErr(pgForeignKeyViolation)
	duplicate := pgErr(pgUniqueViolation)

	type args struct {
		err error
	}

	tests := []struct {
		name         string
		args         args
		assertResult func(t *testing.T, in error, got error)
	}{
		{
			name: "nil passes through",
			args: args{err: nil},
			assertResult: func(t *testing.T, in error, got error) {
				require.NoError(t, got)
			},
		},
		{
			name: "an unrelated error passes through",
			args: args{err: errStub},
			assertResult: func(t *testing.T, in error, got error) {
				require.ErrorIs(t, got, in)
				require.NotErrorIs(t, got, storageError.UniqueViolationError)
			},
		},
		{
			name: "another constraint failure passes through",
			args: args{err: fkViolation},
			assertResult: func(t *testing.T, in error, got error) {
				require.ErrorIs(t, got, in)
				require.NotErrorIs(t, got, storageError.UniqueViolationError)
			},
		},
		{
			name: "a unique violation is wrapped",
			args: args{err: duplicate},
			assertResult: func(t *testing.T, in error, got error) {
				require.ErrorIs(t, got, in)
				require.ErrorIs(t, got, storageError.UniqueViolationError)

				var driverErr *pgconn.PgError
				require.ErrorAs(t, got, &driverErr)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := wrapUnique(tt.args.err)
			tt.assertResult(t, tt.args.err, got)
		})
	}
}

// TestEnsureDatabase covers the one thing it can without a real server to create a database on: the
// name is validated — it's spliced directly into a CREATE DATABASE "%s" statement (see the
// #nosec-worthy comment that would need in storage.go if identifierRE didn't guard it first) — before
// any connection is attempted. A name that isn't a bare identifier has to be rejected at that check,
// distinguished here from a name that passes it and fails later for the mundane reason that nothing
// is listening on the port.
func TestEnsureDatabase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		dbName           string
		wantRejectedName bool
	}{
		{name: "a plain identifier passes the validator", dbName: gofakeit.Word() + "_" + gofakeit.LetterN(6)},
		{name: "a name with a space is rejected", dbName: gofakeit.Word() + " " + gofakeit.Word(), wantRejectedName: true},
		{
			name:             "a name with a semicolon is rejected",
			dbName:           gofakeit.Word() + "; DROP DATABASE postgres",
			wantRejectedName: true,
		},
		{name: "a name with a quote is rejected", dbName: `"` + gofakeit.Word(), wantRejectedName: true},
		{name: "an empty name is rejected", dbName: "", wantRejectedName: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := Config{Host: "127.0.0.1", Port: closedPort(t), Name: tt.dbName}

			err := EnsureDatabase(cfg)
			require.Error(t, err, "want an error either way")
			require.Equal(t, tt.wantRejectedName, strings.Contains(err.Error(), "invalid database name"))
		})
	}
}

// TestOpen checks that an unreachable server is reported at Open (which pings) rather than at the
// first query — a lazily-connecting driver would otherwise stay silent about it. A live server isn't
// needed for this: a closed local port refuses the connection deterministically everywhere.
func TestOpen(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Host: "127.0.0.1", Port: closedPort(t), User: fakeUsername(), Password: fakeHash(), Name: gofakeit.Word(),
	}

	s, err := Open(cfg)
	if err == nil {
		_ = s.Close()
	}

	require.Error(t, err, "want an error for an unreachable server")
}

// closedPort returns a TCP port on localhost that nothing is listening on, by opening then
// immediately closing a listener — deterministic and available in any environment, unlike a
// reserved-looking magic number that might collide with something already running.
func closedPort(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "finding a free port")

	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	return strconv.Itoa(port)
}

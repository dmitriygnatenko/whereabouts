// Package sqlite is the SQLite driver adapter: it implements every operation the repositories in
// internal/repository ask for, in SQLite's own dialect ("?" placeholders, LastInsertId for generated
// ids, JSON_SET for the settings column). The queries live next to this file, one file per table
// group (user.go, item.go, location.go, session.go).
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"modernc.org/sqlite"

	storageError "wherewhat/internal/storage/error"
)

// Storage wraps the connection pool the queries run against. It satisfies each repository's own
// Storage interface; internal/app is where a single one of these is handed to all of them.
type Storage struct {
	*sql.DB
}

// sqliteConstraintUnique is the extended SQLITE_CONSTRAINT_UNIQUE result code (see sqlite3.h).
// modernc.org/sqlite doesn't export this constant from its public package, so it's duplicated here
// as a known literal (it hasn't changed across SQLite versions).
const sqliteConstraintUnique = 2067

// wrapUnique normalizes a UNIQUE constraint violation into storageError.UniqueViolationError, so a
// repository can recognize it without knowing anything about SQLite. Any other error (including nil)
// passes through.
func wrapUnique(err error) error {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqliteConstraintUnique {
		return fmt.Errorf("%w: %w", storageError.UniqueViolationError, err)
	}

	return err
}

// insertReturningID runs an INSERT and returns the new row's id via LastInsertId.
func (s *Storage) insertReturningID(ctx context.Context, query string, args ...any) (uint64, error) {
	res, err := s.DB.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()

	return uint64(id), err
}

// affected reports whether a statement touched any row, which is how the storage layer answers
// "found?" for updates and deletes.
func affected(res sql.Result) (bool, error) {
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	return n > 0, nil
}

// EnsureDatabase creates the parent directory of the database file if it doesn't exist yet — the
// driver creates the file itself on first connection.
func EnsureDatabase(cfg Config) error {
	if dir := filepath.Dir(cfg.Path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating directory %q for the SQLite file: %w", dir, err)
		}
	}

	return nil
}

// Open opens the connection pool for the database file and wraps it in Storage.
func Open(cfg Config) (*Storage, error) {
	raw, err := sql.Open("sqlite", cfg.dsn())
	if err != nil {
		return nil, fmt.Errorf("opening the database: %w", err)
	}

	raw.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	// SQLite doesn't tolerate concurrent writers — multiple simultaneous connections just produce
	// "database is locked" under load.
	raw.SetMaxOpenConns(1)

	if err := raw.Ping(); err != nil {
		return nil, fmt.Errorf("checking the database connection: %w", err)
	}

	return &Storage{DB: raw}, nil
}

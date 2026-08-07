// Package postgres is the Postgres driver adapter: it implements every operation the repositories in
// internal/repository ask for, in Postgres's own dialect ("$1, $2, ..." placeholders, RETURNING id
// for generated ids, jsonb_set for the settings column). The queries live next to this file, one
// file per table group (user.go, item.go, location.go, session.go).
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver used below

	storageError "wherewhat/internal/storage/error"
)

var identifierRE = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// Storage wraps the connection pool the queries run against. It satisfies each repository's own
// Storage interface; internal/app is where a single one of these is handed to all of them.
type Storage struct {
	*sql.DB
}

// pgUniqueViolation is the SQLSTATE for unique_violation.
const pgUniqueViolation = "23505"

// wrapUnique normalizes a UNIQUE constraint violation into storageError.UniqueViolationError, so a
// repository can recognize it without knowing anything about Postgres. Any other error (including nil)
// passes through.
func wrapUnique(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return fmt.Errorf("%w: %w", storageError.UniqueViolationError, err)
	}

	return err
}

// insertReturningID runs an INSERT with a RETURNING id clause — Postgres drivers don't support
// LastInsertId.
func (s *Storage) insertReturningID(
	ctx context.Context, query string, args ...any,
) (uint64, error) {
	var id uint64
	err := s.DB.QueryRowContext(ctx, query, args...).Scan(&id)

	return id, err
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

// EnsureDatabase creates the target database if it doesn't already exist. This needs CREATE
// DATABASE privileges; without them, just make sure the database exists ahead of time and this call
// will fail harmlessly for the caller to log and ignore.
func EnsureDatabase(cfg Config) error {
	if !identifierRE.MatchString(cfg.Name) {
		return fmt.Errorf("invalid database name: %q", cfg.Name)
	}

	root, err := sql.Open("pgx", cfg.serverDSN())
	if err != nil {
		return fmt.Errorf("connecting to the DB server: %w", err)
	}
	defer root.Close()

	if err := root.Ping(); err != nil {
		return fmt.Errorf("reaching the DB server: %w", err)
	}

	var exists bool

	err = root.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, cfg.Name).Scan(&exists)
	if err != nil {
		return fmt.Errorf("checking whether database %q exists: %w", cfg.Name, err)
	}

	if exists {
		return nil
	}
	// CREATE DATABASE doesn't support IF NOT EXISTS or a parameterized name — but cfg.Name is already
	// validated by identifierRE above.
	if _, err := root.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, cfg.Name)); err != nil {
		return fmt.Errorf("creating database %q: %w", cfg.Name, err)
	}

	return nil
}

// Open opens the connection pool for the target database and wraps it in Storage.
func Open(cfg Config) (*Storage, error) {
	raw, err := sql.Open("pgx", cfg.dsn())
	if err != nil {
		return nil, fmt.Errorf("opening the database: %w", err)
	}

	raw.SetMaxOpenConns(cfg.MaxOpenConns)
	raw.SetMaxIdleConns(cfg.MaxIdleConns)
	raw.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := raw.Ping(); err != nil {
		return nil, fmt.Errorf("checking the database connection: %w", err)
	}

	return &Storage{DB: raw}, nil
}

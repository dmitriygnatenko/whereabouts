// Package session implements port.SessionRepository on top of the sessions table: it converts row
// models into domain entities and turns a missing row into *domainerror.NotFoundError.
package session

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"wherewhat/internal/domain/entity"
	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/storage/model"
)

//go:generate go tool mockgen -source=repository.go -destination=mocks/storage_mock.go -package=mocks

// Storage is the slice of a driver adapter this repository uses — the sessions table and nothing
// else. Every driver adapter (internal/adapter/mysql, postgres, sqlite) implements it in its own
// dialect.
type Storage interface {
	// CreateSession inserts a session row stamped with createdAt.
	CreateSession(ctx context.Context, session model.Session, createdAt time.Time) error

	// FindSessionByToken returns sql.ErrNoRows when the token doesn't match a stored session.
	FindSessionByToken(ctx context.Context, token string) (model.Session, error)

	DeleteSession(ctx context.Context, token string) error

	// DeleteExpiredSessions removes every session whose expiry is before now, returning how many rows
	// went away.
	DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error)
}

// Repository implements port.SessionRepository.
type Repository struct {
	storage Storage
}

// New builds a Repository against s.
func New(s Storage) *Repository {
	return &Repository{storage: s}
}

// Create persists a new session row, stamped as created now.
func (r *Repository) Create(
	ctx context.Context,
	session entity.Session,
) error {
	return r.storage.CreateSession(ctx, model.SessionFromEntity(session), time.Now().UTC())
}

// FindByToken looks up a session by its token, reporting an unknown token as a
// *domainerror.NotFoundError.
func (r *Repository) FindByToken(
	ctx context.Context,
	token string,
) (entity.Session, error) {
	m, err := r.storage.FindSessionByToken(ctx, token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return entity.Session{}, &domainerror.NotFoundError{Message: "Session not found"}
		}

		return entity.Session{}, err
	}

	return m.ToEntity(), nil
}

// Delete removes a session by its token.
func (r *Repository) Delete(
	ctx context.Context,
	token string,
) error {
	return r.storage.DeleteSession(ctx, token)
}

// DeleteExpired removes every session whose expiry is before now and returns how many rows were
// deleted.
func (r *Repository) DeleteExpired(
	ctx context.Context,
	now time.Time,
) (int64, error) {
	return r.storage.DeleteExpiredSessions(ctx, now)
}

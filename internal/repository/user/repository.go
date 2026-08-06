// Package user implements port.UserRepository on top of the users table: it converts row models into
// domain entities and turns the storage layer's raw errors into domain errors — a missing row into
// *domainerror.NotFoundError, a username collision into *domainerror.ConflictError.
package user

import (
	"context"
	"database/sql"
	"errors"

	"wherewhat/internal/domain/entity"
	domainerror "wherewhat/internal/domain/error"
	storageError "wherewhat/internal/storage/error"
	"wherewhat/internal/storage/model"
)

//go:generate go tool mockgen -source=repository.go -destination=mocks/storage_mock.go -package=mocks

// Storage is the slice of a driver adapter this repository uses — the users table and nothing else.
// Every driver adapter (internal/adapter/mysql, postgres, sqlite) implements it in its own dialect.
type Storage interface {
	// FindUserByUsername returns sql.ErrNoRows when no user has this (already-normalized) username.
	FindUserByUsername(ctx context.Context, username string) (model.User, error)

	// FindUserByID returns sql.ErrNoRows when no user has this id.
	FindUserByID(ctx context.Context, id uint64) (model.User, error)

	// CreateUser inserts a user row and returns its new id. A taken username comes back wrapped in
	// storageError.UniqueViolationError.
	CreateUser(
		ctx context.Context, username, passwordHash string, settings model.UserSettings,
	) (id uint64, err error)

	// UpdateUsername renames a user. A taken username comes back wrapped in
	// storageError.UniqueViolationError.
	UpdateUsername(ctx context.Context, id uint64, username string) error

	UpdateUserPasswordHash(ctx context.Context, id uint64, hash string) error

	// UpdateUserLanguage writes one field inside the settings JSON column, leaving the rest untouched.
	UpdateUserLanguage(ctx context.Context, id uint64, lang string) error

	// UpdateUserLocationFilterDepth writes one field inside the settings JSON column, leaving the rest
	// untouched.
	UpdateUserLocationFilterDepth(ctx context.Context, id uint64, depth int) error

	CountUsers(ctx context.Context) (int, error)
}

// conflictMessage is what both write paths report when a username is already taken.
const conflictMessage = "A user with this username is already registered"

// Repository implements port.UserRepository.
type Repository struct {
	storage Storage
}

// New builds a Repository against s.
func New(s Storage) *Repository {
	return &Repository{storage: s}
}

// FindByUsername looks up a user by their (already-normalized) username.
func (r *Repository) FindByUsername(
	ctx context.Context,
	username string,
) (entity.User, error) {
	m, err := r.storage.FindUserByUsername(ctx, username)
	if err != nil {
		return entity.User{}, notFound(err)
	}

	return m.ToEntity(), nil
}

// FindByID looks up a user by id.
func (r *Repository) FindByID(
	ctx context.Context,
	id uint64,
) (entity.User, error) {
	m, err := r.storage.FindUserByID(ctx, id)
	if err != nil {
		return entity.User{}, notFound(err)
	}

	return m.ToEntity(), nil
}

// Create inserts a new user and returns its id. A username collision is reported as a
// *domainerror.ConflictError.
func (r *Repository) Create(
	ctx context.Context,
	username string,
	passwordHash string,
	settings entity.UserSettings,
) (uint64, error) {
	id, err := r.storage.CreateUser(ctx, username, passwordHash, model.UserSettingsFromEntity(settings))
	if err != nil {
		return 0, conflict(err)
	}

	return id, nil
}

// UpdateUsername renames a user's login username. A collision with an existing username is reported
// as a *domainerror.ConflictError.
func (r *Repository) UpdateUsername(
	ctx context.Context,
	id uint64,
	username string,
) error {
	return conflict(r.storage.UpdateUsername(ctx, id, username))
}

// UpdatePasswordHash overwrites a user's stored password hash.
func (r *Repository) UpdatePasswordHash(
	ctx context.Context,
	id uint64,
	hash string,
) error {
	return r.storage.UpdateUserPasswordHash(ctx, id, hash)
}

// UpdateLanguage sets a user's interface language, without disturbing the rest of their settings.
func (r *Repository) UpdateLanguage(
	ctx context.Context,
	id uint64,
	lang string,
) error {
	return r.storage.UpdateUserLanguage(ctx, id, lang)
}

// UpdateLocationFilterDepth sets a user's location-filter-depth preference, without disturbing the
// rest of their settings.
func (r *Repository) UpdateLocationFilterDepth(
	ctx context.Context,
	id uint64,
	depth int,
) error {
	return r.storage.UpdateUserLocationFilterDepth(ctx, id, depth)
}

// Count returns the total number of users — used to decide whether the demo user needs seeding on a
// fresh install.
func (r *Repository) Count(ctx context.Context) (int, error) {
	return r.storage.CountUsers(ctx)
}

// notFound translates "no such row" into the domain's NotFoundError, passing any other error
// through.
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return &domainerror.NotFoundError{Message: "User not found"}
	}

	return err
}

// conflict translates a unique-constraint violation into the domain's ConflictError, passing any
// other error (including nil) through.
func conflict(err error) error {
	if errors.Is(err, storageError.UniqueViolationError) {
		return &domainerror.ConflictError{Message: conflictMessage}
	}

	return err
}

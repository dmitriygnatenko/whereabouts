// Package registeruser is the RegisterUser use case: it creates a new account and immediately signs
// it in.
package registeruser

import (
	"context"
	"errors"
	"wherewhat/internal/domain/entity"
	"wherewhat/internal/domain/usecase/auth"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/port"
)

// UseCase implements RegisterUser.
type UseCase struct {
	Users    port.UserRepository
	Sessions port.SessionRepository
	Hasher   port.PasswordHasher
	Tokens   port.TokenGenerator
}

// New builds a UseCase from its dependencies.
func New(
	users port.UserRepository, sessions port.SessionRepository, hasher port.PasswordHasher, tokens port.TokenGenerator,
) *UseCase {
	return &UseCase{Users: users, Sessions: sessions, Hasher: hasher, Tokens: tokens}
}

// Execute validates and creates a new account, then starts a session for it.
func (uc *UseCase) Execute(ctx context.Context, in Input) (Output, error) {
	if verr := in.Validate(); verr != nil {
		return Output{}, verr
	}

	username := auth.NormalizeUsername(in.Username)

	hash, err := uc.Hasher.Hash(in.Password)
	if err != nil {
		return Output{}, errors.New("Failed to process password")
	}

	// Language starts blank: it's adopted from whatever the frontend detects on first successful login
	// (see login), not hardcoded here.
	settings := entity.UserSettings{}

	id, err := uc.Users.Create(ctx, username, hash, settings)
	if err != nil {
		var conflict *domainerror.ConflictError
		if errors.As(err, &conflict) {
			return Output{}, conflict
		}

		return Output{}, errors.New("Failed to create user")
	}

	session, err := auth.NewSession(ctx, uc.Sessions, uc.Tokens, id)
	if err != nil {
		return Output{}, errors.New("User created, but failed to start a session")
	}

	return Output{
		User:    entity.PublicUser{ID: id, Username: username, UserSettings: settings},
		Session: session,
	}, nil
}

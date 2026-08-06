// Package createuser is the CreateUser use case: it creates a new account without starting a
// session. Unlike registeruser (self-service sign-up from the web app), this is meant for
// administrative use — currently the `-create-user` CLI flag.
package createuser

import (
	"context"
	"errors"
	"wherewhat/internal/domain/entity"
	"wherewhat/internal/domain/usecase/auth"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/usecase/validate"
	"wherewhat/internal/port"
)

// UseCase implements CreateUser.
type UseCase struct {
	Users  port.UserRepository
	Hasher port.PasswordHasher
}

// New builds a UseCase from its dependencies.
func New(users port.UserRepository, hasher port.PasswordHasher) *UseCase {
	return &UseCase{Users: users, Hasher: hasher}
}

// Execute validates and creates a new account, without starting a session for it.
func (uc *UseCase) Execute(ctx context.Context, in Input) (Output, error) {
	username := auth.NormalizeUsername(in.Username)
	if verr := validate.Username(username); verr != nil {
		return entity.PublicUser{}, verr
	}

	if verr := validate.Password(in.Password); verr != nil {
		return entity.PublicUser{}, verr
	}

	hash, err := uc.Hasher.Hash(in.Password)
	if err != nil {
		return entity.PublicUser{}, errors.New("Failed to process password")
	}

	settings := entity.UserSettings{}

	id, err := uc.Users.Create(ctx, username, hash, settings)
	if err != nil {
		var conflict *domainerror.ConflictError
		if errors.As(err, &conflict) {
			return entity.PublicUser{}, conflict
		}

		return entity.PublicUser{}, errors.New("Failed to create user")
	}

	return entity.PublicUser{ID: id, Username: username, UserSettings: settings}, nil
}

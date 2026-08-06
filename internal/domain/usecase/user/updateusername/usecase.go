// Package updateusername is the UpdateUsername use case: it lets a signed-in user change their
// login username, after confirming their current password.
package updateusername

import (
	"context"
	"errors"
	"strings"
	"wherewhat/internal/domain/entity"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/usecase/user/shared"
	"wherewhat/internal/domain/usecase/validate"
	"wherewhat/internal/port"
)

// UseCase implements UpdateUsername.
type UseCase struct {
	Users  port.UserRepository
	Hasher port.PasswordHasher
}

// New builds a UseCase from its dependencies.
func New(users port.UserRepository, hasher port.PasswordHasher) *UseCase {
	return &UseCase{Users: users, Hasher: hasher}
}

// Execute changes the signed-in user's username, after confirming their current password.
func (uc *UseCase) Execute(ctx context.Context, in Input) (Output, error) {
	username := strings.ToLower(strings.TrimSpace(in.Username))
	if verr := validate.Username(username); verr != nil {
		return entity.PublicUser{}, verr
	}

	ok, err := shared.CheckCurrentPassword(ctx, uc.Users, uc.Hasher, in.User.ID, in.CurrentPassword)
	if err != nil {
		return entity.PublicUser{}, errors.New("Failed to verify current password")
	}

	if !ok {
		// Forbidden, not unauthorized: the session itself is still valid — only the re-entered password
		// was wrong. Treating this as Unauthorized would trip the frontend's global "session expired"
		// handler and log the user out.
		return entity.PublicUser{}, &domainerror.ForbiddenError{Message: "Incorrect current password"}
	}

	if username == in.User.Username {
		return in.User, nil
	}

	if err := uc.Users.UpdateUsername(ctx, in.User.ID, username); err != nil {
		var conflict *domainerror.ConflictError
		if errors.As(err, &conflict) {
			return entity.PublicUser{}, conflict
		}

		return entity.PublicUser{}, errors.New("Failed to update username")
	}

	updated := in.User
	updated.Username = username

	return updated, nil
}

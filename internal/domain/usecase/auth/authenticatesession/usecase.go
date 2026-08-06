// Package authenticatesession is the AuthenticateSession use case: it resolves a session token into
// the user it belongs to, used both by the requireAuth-style middleware and by "who am I"
// endpoints. It has no input.go — Execute takes a bare token string.
package authenticatesession

import (
	"context"
	"time"
	"wherewhat/internal/domain/entity"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/port"
)

// unauthorized is returned for every failure mode below (missing token, no matching/expired
// session, dangling user row). Callers in the HTTP layer choose their own user-facing wording per
// endpoint, so its message isn't meant to reach the client verbatim.
var unauthorized = &domainerror.UnauthorizedError{Message: "authentication required"}

// UseCase implements AuthenticateSession.
type UseCase struct {
	Sessions port.SessionRepository
	Users    port.UserRepository
}

// New builds a UseCase from its dependencies.
func New(sessions port.SessionRepository, users port.UserRepository) *UseCase {
	return &UseCase{Sessions: sessions, Users: users}
}

// Execute resolves token into the user it belongs to, failing with unauthorized for a
// missing/expired/dangling session.
func (uc *UseCase) Execute(ctx context.Context, token string) (Output, error) {
	if token == "" {
		return entity.PublicUser{}, unauthorized
	}

	session, err := uc.Sessions.FindByToken(ctx, token)
	if err != nil {
		return entity.PublicUser{}, unauthorized
	}

	if session.ExpiresAt.Before(time.Now().UTC()) {
		_ = uc.Sessions.Delete(ctx, token)
		return entity.PublicUser{}, unauthorized
	}

	user, err := uc.Users.FindByID(ctx, session.UserID)
	if err != nil {
		return entity.PublicUser{}, unauthorized
	}

	return user.Public(), nil
}

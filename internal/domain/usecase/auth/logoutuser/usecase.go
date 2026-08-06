// Package logoutuser is the LogoutUser use case: it ends a session. It has no input.go/output.go —
// Execute takes a bare token string and returns nothing; it's best-effort by design, matching the
// original behaviour: an absent or already-gone token is not an error, and the caller always ends
// up logged out client-side regardless.
package logoutuser

import (
	"context"

	"wherewhat/internal/port"
)

// UseCase implements LogoutUser.
type UseCase struct {
	Sessions port.SessionRepository
}

// New builds a UseCase from its dependencies.
func New(sessions port.SessionRepository) *UseCase {
	return &UseCase{Sessions: sessions}
}

// Execute deletes the session behind token, if any.
func (uc *UseCase) Execute(ctx context.Context, token string) {
	if token == "" {
		return
	}

	_ = uc.Sessions.Delete(ctx, token)
}

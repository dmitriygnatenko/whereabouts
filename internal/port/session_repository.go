package port

import (
	"context"
	"time"
	"wherewhat/internal/domain/entity"
)

//go:generate go tool mockgen -source=session_repository.go -destination=mocks/session_repository_mock.go -package=mocks

// SessionRepository persists login sessions.
type SessionRepository interface {
	Create(ctx context.Context, session entity.Session) error
	// FindByToken returns an error (not necessarily *domainerror.NotFoundError) whenever the token
	// doesn't map to a live session — callers treat any error here as "not authenticated" without
	// inspecting it further.
	FindByToken(ctx context.Context, token string) (entity.Session, error)
	Delete(ctx context.Context, token string) error
	// DeleteExpired removes every session whose expiry is before now — sessions are otherwise only
	// ever deleted lazily, the one time an expired token happens to be presented again (see
	// authenticatesession.UseCase), so a periodic sweep is what actually keeps the table from growing
	// forever with sessions nobody ever came back to use.
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}

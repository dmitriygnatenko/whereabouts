// Package auth holds session-creation and username-normalization logic used by more than one auth
// use case (registeruser and login) — exported so those sibling packages can call it, now that
// each use case lives in its own package.
package auth

import (
	"context"
	"strings"
	"time"
	"wherewhat/internal/domain/entity"
	"wherewhat/internal/port"
)

// SessionDuration is how long a session stays valid after login/register.
const SessionDuration = 30 * 24 * time.Hour

// NormalizeUsername lowercases and trims a username the same way everywhere it's accepted — from
// registration, login or a profile change.
func NormalizeUsername(u string) string {
	return strings.ToLower(strings.TrimSpace(u))
}

// NewSession generates a token, computes its expiry and persists it — shared by RegisterUser and
// LoginUser, which both start a session the same way once they've settled on a user id.
func NewSession(
	ctx context.Context,
	sessions port.SessionRepository,
	tokens port.TokenGenerator,
	userID uint64,
) (entity.Session, error) {
	token, err := tokens.NewToken()
	if err != nil {
		return entity.Session{}, err
	}

	session := entity.Session{
		Token:     token,
		UserID:    userID,
		ExpiresAt: time.Now().UTC().Add(SessionDuration),
	}
	if err = sessions.Create(ctx, session); err != nil {
		return entity.Session{}, err
	}

	return session, nil
}

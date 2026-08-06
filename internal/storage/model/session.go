package model

import "wherewhat/internal/domain/entity"

// Session is the shape of a row in the sessions table.
type Session struct {
	Token     string
	UserID    uint64
	ExpiresAt Time
}

// SessionFromEntity converts a domain Session into its row shape.
func SessionFromEntity(s entity.Session) Session {
	return Session{
		Token:     s.Token,
		UserID:    s.UserID,
		ExpiresAt: NewTime(s.ExpiresAt),
	}
}

// ToEntity converts a row-shaped Session into the domain entity.
func (s Session) ToEntity() entity.Session {
	return entity.Session{
		Token:     s.Token,
		UserID:    s.UserID,
		ExpiresAt: s.ExpiresAt.Time,
	}
}

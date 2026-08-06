package entity

import "time"

// Session is a signed-in user's server-side session record.
type Session struct {
	Token     string
	UserID    uint64
	ExpiresAt time.Time
}

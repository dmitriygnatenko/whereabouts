package registeruser

import (
	"wherewhat/internal/domain/entity"
)

// Output is the freshly-created account plus the session it was immediately signed into.
type Output struct {
	User    entity.PublicUser
	Session entity.Session
}

package authenticate

import (
	"wherewhat/internal/domain/entity"
)

// Output is the user the token belongs to.
type Output struct {
	User entity.PublicUser
}

package updatelocationfilterdepth

import (
	"wherewhat/internal/domain/entity"
)

// Output is the user, with the new filter depth applied.
type Output struct {
	User entity.PublicUser
}

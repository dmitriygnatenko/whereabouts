package updatelocationfilterdepth

import (
	"wherewhat/internal/domain/entity"
)

// Input is what UpdateLocationFilterDepth needs — the currently authenticated user and the new
// filter depth. 0 means "no limit".
type Input struct {
	User  entity.PublicUser
	Depth int
}

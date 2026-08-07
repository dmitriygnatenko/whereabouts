package list

import (
	"wherewhat/internal/domain/entity"
)

// Output is every location.
type Output struct {
	Locations []entity.Location
}

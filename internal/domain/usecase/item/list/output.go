package list

import (
	"wherewhat/internal/domain/entity"
)

// Output is every item, most recently updated first.
type Output struct {
	Items []entity.Item
}

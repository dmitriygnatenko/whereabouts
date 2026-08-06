package model

import "wherewhat/internal/domain/entity"

// Location is the shape of a row in the locations table.
type Location struct {
	ID       uint64
	Name     string
	Color    string
	ParentID *uint64
}

// ToEntity converts a row-shaped Location into the domain entity.
func (l Location) ToEntity() entity.Location {
	return entity.Location{
		ID:       l.ID,
		Name:     l.Name,
		Color:    l.Color,
		ParentID: l.ParentID,
	}
}

package updatelocation

// Input is what UpdateLocation needs to rename/recolor a location.
type Input struct {
	ID    uint64
	Name  string
	Color string
}

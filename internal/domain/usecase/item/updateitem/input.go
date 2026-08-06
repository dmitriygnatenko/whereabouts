package updateitem

// Input is what UpdateItem needs to rewrite an existing item.
type Input struct {
	ID         uint64
	Name       string
	LocationID uint64
	Notes      string
	Images     []string
}

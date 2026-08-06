package createitem

// Input is what CreateItem needs to create a new item.
type Input struct {
	Name       string
	LocationID uint64
	Notes      string
	Images     []string
}

package entity

// Item is a thing stored at a Location, with an optional set of photos. UpdatedAt is kept as the
// driver-formatted string read back from storage (see port.ItemRepository) rather than time.Time,
// to match exactly what has always been sent to API clients. json tags live here rather than on a
// separate response DTO: an Item is already exactly what the API returns, with nothing to hide.
type Item struct {
	ID         uint64   `json:"id"`
	Name       string   `json:"name"`
	LocationID uint64   `json:"locationId"`
	Notes      string   `json:"notes"`
	Images     []string `json:"images"`
	UpdatedAt  string   `json:"updatedAt"`
}

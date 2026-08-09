package entity

// Item is a thing stored at a Location, with an optional set of photos. UpdatedAt is kept as the
// driver-formatted string read back from storage (see port.ItemRepository) rather than time.Time,
// to match exactly what has always been sent to API clients. json tags live here rather than on a
// separate response DTO: an Item is already exactly what the API returns, with nothing to hide.
//
// Thumbnails runs parallel to Images (same length, same order): Thumbnails[i] is the small preview
// for Images[i], or "" if none was generated (e.g. an unsupported source format) — the frontend
// falls back to the full image in that case.
type Item struct {
	ID         uint64   `json:"id"`
	Name       string   `json:"name"`
	LocationID uint64   `json:"locationId"`
	Notes      string   `json:"notes"`
	Images     []string `json:"images"`
	Thumbnails []string `json:"thumbnails"`
	UpdatedAt  string   `json:"updatedAt"`
}

// ItemImage pairs a stored photo URL with its thumbnail URL — the shape passed between the image
// use cases and the item repository while saving an item's photo set. It isn't part of the API
// response shape; see Item.Images/Thumbnails for that.
type ItemImage struct {
	URL          string
	ThumbnailURL string
}

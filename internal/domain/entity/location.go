package entity

// Location is a place things can be stored in. ParentID == nil means a top-level location. json
// tags live here rather than on a separate response DTO: a Location is already exactly what the API
// returns.
type Location struct {
	ID       uint64  `json:"id"`
	Name     string  `json:"name"`
	Color    string  `json:"color"`
	ParentID *uint64 `json:"parentId"`
}

package createlocation

// Input is what CreateLocation needs to create a new location, optionally nested under a parent. A
// ParentID that is non-nil but not a positive id is treated as "no parent" — this mirrors the
// original handler's behaviour exactly.
type Input struct {
	Name     string
	Color    string
	ParentID *uint64
}

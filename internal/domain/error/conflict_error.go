package error

// ConflictError signals a request that can't be applied because of the current state of the data
// (duplicate username, location still in use, ...). Maps to 409 Conflict.
type ConflictError struct {
	Message string
}

// Error implements the error interface.
func (e *ConflictError) Error() string {
	return e.Message
}

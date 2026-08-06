package error

// ForbiddenError signals that the request was authenticated but not permitted (e.g. wrong current
// password on a profile change). Maps to 403 Forbidden.
type ForbiddenError struct {
	Message string
}

// Error implements the error interface.
func (e *ForbiddenError) Error() string {
	return e.Message
}

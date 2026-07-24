package domain

// ValidationError represents a field-level validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return e.Field + ": " + e.Message
}

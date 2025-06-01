package ptr

// New creates a new pointer to the given value.
func New[T any](value T) *T {
	return &value
}

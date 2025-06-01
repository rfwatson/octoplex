package optional

// V is a value that may or may not be present.
type V[T any] struct {
	Value   T
	Present bool
}

// Ref returns a pointer to the value if it is present, or nil if it is not.
func (v V[T]) Ref() *T {
	if !v.Present {
		return nil
	}

	return &v.Value
}

// IsZero checks if the optional value is not present.
func (v V[T]) IsZero() bool {
	return !v.Present
}

// IsPresent checks if the optional value is present.
func (v V[T]) IsPresent() bool {
	return v.Present
}

// New creates a new optional value with the given value.
func New[T any](value T) V[T] {
	return V[T]{
		Value:   value,
		Present: true,
	}
}

// Deref creates a new optional value from a pointer.
//
// If the pointer is non-nil, it is dereferenced and set as the value.
func Deref[T any](ptr *T) V[T] {
	if ptr == nil {
		return V[T]{Present: false}
	}

	return V[T]{
		Value:   *ptr,
		Present: true,
	}
}

// Empty creates a new optional value that is not present.
func Empty[T any]() V[T] {
	return V[T]{Present: false}
}

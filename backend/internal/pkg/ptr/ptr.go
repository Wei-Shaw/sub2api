// Package ptr provides a tiny generic helper for taking pointers to values.
package ptr

// Of returns a pointer to v.
func Of[T any](v T) *T {
	return &v
}

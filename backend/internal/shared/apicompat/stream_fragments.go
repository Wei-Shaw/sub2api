package apicompat

import "strings"

// Keep public string snapshots while accumulating fragments amortized linearly.
// Reset releases the builder without overwriting completed event strings. An
// initial value or an externally replaced snapshot remains authoritative.
func appendStreamFragment(buffer *strings.Builder, current, fragment string) string {
	if buffer.String() != current {
		buffer.Reset()
		buffer.Grow(len(current) + len(fragment))
		_, _ = buffer.WriteString(current)
	}
	_, _ = buffer.WriteString(fragment)
	return buffer.String()
}

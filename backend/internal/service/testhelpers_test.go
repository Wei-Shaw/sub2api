//go:build unit

package service

// testPtrFloat64 returns a pointer to the given float64 value.
func testPtrFloat64(v float64) *float64 { return &v }

// testPtrInt returns a pointer to the given int value.
func testPtrInt(v int) *int { return &v }

// testPtrString returns a pointer to the given string value.
func testPtrString(v string) *string { return &v }

// testPtrBool returns a pointer to the given bool value.
func testPtrBool(v bool) *bool { return &v }

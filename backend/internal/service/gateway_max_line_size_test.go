package service

import "testing"

// Guard against reintroducing multi-hundred-MiB SSE line defaults that let
// concurrent Codex streams allocate multi-GiB anonymous memory (#4365).
func TestDefaultMaxLineSizeIsBoundedForLowMemoryHosts(t *testing.T) {
	const maxAcceptableDefault = 64 * 1024 * 1024 // 64 MiB hard ceiling for the built-in default

	if defaultMaxLineSize <= 0 {
		t.Fatalf("defaultMaxLineSize must be positive, got %d", defaultMaxLineSize)
	}
	if defaultMaxLineSize > maxAcceptableDefault {
		t.Fatalf("defaultMaxLineSize=%d exceeds safe ceiling %d (see #4365)", defaultMaxLineSize, maxAcceptableDefault)
	}
	// Documented project default is 40 MiB (config.example.yaml).
	if defaultMaxLineSize != 40*1024*1024 {
		t.Fatalf("defaultMaxLineSize=%d, want 40 MiB to match deploy/config.example.yaml", defaultMaxLineSize)
	}
}

//go:build unit

package tlsfingerprint

import "testing"

// TestSanitizeALPN verifies that only protocols the fingerprint transport can
// speak survive, and that everything else is reported as dropped.
func TestSanitizeALPN(t *testing.T) {
	tests := []struct {
		name        string
		input       []string
		wantKept    []string
		wantDropped []string
	}{
		{
			name:  "nil stays nil so the dialer applies its default",
			input: nil,
		},
		{
			name:     "http/1.1 is preserved",
			input:    []string{"http/1.1"},
			wantKept: []string{"http/1.1"},
		},
		{
			name:        "h2 is dropped",
			input:       []string{"h2"},
			wantDropped: []string{"h2"},
		},
		{
			name:        "mixed list keeps only http/1.1",
			input:       []string{"h2", "http/1.1"},
			wantKept:    []string{"http/1.1"},
			wantDropped: []string{"h2"},
		},
		{
			name:        "h3 and unknown protocols are dropped",
			input:       []string{"h3", "spdy/3.1"},
			wantDropped: []string{"h3", "spdy/3.1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kept, dropped := SanitizeALPN(tc.input)
			assertStrings(t, "kept", kept, tc.wantKept)
			assertStrings(t, "dropped", dropped, tc.wantDropped)
		})
	}
}

// TestSanitizeALPNDoesNotMutateInput guards the caller's slice: the transport
// sanitizes profiles that live in a shared cache.
func TestSanitizeALPNDoesNotMutateInput(t *testing.T) {
	input := []string{"h2", "http/1.1"}
	SanitizeALPN(input)

	if input[0] != "h2" || input[1] != "http/1.1" {
		t.Errorf("input mutated: got %v, want [h2 http/1.1]", input)
	}
}

func assertStrings(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: got %v, want %v", label, got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s: got %v, want %v", label, got, want)
			return
		}
	}
}

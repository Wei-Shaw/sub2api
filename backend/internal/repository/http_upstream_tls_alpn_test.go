package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

// TestSanitizeProfileALPNLeavesCachedProfileIntact is the point of the whole
// helper: TLSFingerprintProfileService hands the same *Profile to every
// transport build, so sanitizing in place would rewrite the shared template.
func TestSanitizeProfileALPNLeavesCachedProfileIntact(t *testing.T) {
	cached := &tlsfingerprint.Profile{
		Name:          "claude_cli_v2",
		ALPNProtocols: []string{"h2", "http/1.1"},
	}

	sanitized := sanitizeProfileALPN(cached)

	if len(cached.ALPNProtocols) != 2 || cached.ALPNProtocols[0] != "h2" {
		t.Errorf("cached profile mutated: got %v, want [h2 http/1.1]", cached.ALPNProtocols)
	}
	if len(sanitized.ALPNProtocols) != 1 || sanitized.ALPNProtocols[0] != "http/1.1" {
		t.Errorf("sanitized ALPN: got %v, want [http/1.1]", sanitized.ALPNProtocols)
	}
	if sanitized.Name != cached.Name {
		t.Errorf("name lost during copy: got %q, want %q", sanitized.Name, cached.Name)
	}
}

// TestSanitizeProfileALPNPassthrough covers the cases that must not allocate a
// copy, including a profile that only advertises the supported protocol.
func TestSanitizeProfileALPNPassthrough(t *testing.T) {
	tests := []struct {
		name    string
		profile *tlsfingerprint.Profile
	}{
		{name: "nil profile"},
		{name: "no ALPN configured", profile: &tlsfingerprint.Profile{Name: "default"}},
		{
			name:    "only http/1.1",
			profile: &tlsfingerprint.Profile{Name: "default", ALPNProtocols: []string{"http/1.1"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeProfileALPN(tc.profile); got != tc.profile {
				t.Errorf("expected the original profile to pass through, got a copy: %+v", got)
			}
		})
	}
}

// TestSanitizeProfileALPNDropsEverythingUnsupported leaves an empty list so the
// dialer falls back to its built-in http/1.1 default rather than sending an
// empty ALPN extension.
func TestSanitizeProfileALPNDropsEverythingUnsupported(t *testing.T) {
	profile := &tlsfingerprint.Profile{Name: "bad", ALPNProtocols: []string{"h2", "h3"}}

	sanitized := sanitizeProfileALPN(profile)

	if len(sanitized.ALPNProtocols) != 0 {
		t.Errorf("expected every protocol dropped, got %v", sanitized.ALPNProtocols)
	}
}

// TestProfileNameForLogging keeps the warning lines readable when a profile is
// absent or unnamed.
func TestProfileNameForLogging(t *testing.T) {
	tests := []struct {
		name    string
		profile *tlsfingerprint.Profile
		want    string
	}{
		{name: "nil", want: "none"},
		{name: "unnamed", profile: &tlsfingerprint.Profile{}, want: "unnamed"},
		{name: "named", profile: &tlsfingerprint.Profile{Name: "claude_cli_v2"}, want: "claude_cli_v2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := profileName(tc.profile); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

package service

import "testing"

func TestIsCloudflareChallengeResponse(t *testing.T) {
	cases := []struct {
		name        string
		cfMitigated string
		body        string
		want        bool
	}{
		{"cf-mitigated challenge header", "challenge", "<html>...</html>", true},
		{"header case-insensitive with spaces", " Challenge ", "", true},
		{"body marker only", "", "<title>Just a moment...</title>", true},
		{"plain json error", "", `{"detail":"Unauthorized"}`, false},
		{"empty", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isCloudflareChallengeResponse(tc.cfMitigated, tc.body); got != tc.want {
				t.Fatalf("isCloudflareChallengeResponse(%q, %q) = %v, want %v", tc.cfMitigated, tc.body, got, tc.want)
			}
		})
	}
}

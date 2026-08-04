//go:build unit

package service

import "testing"

func TestIsSystemToken(t *testing.T) {
	tests := []struct {
		token string
		want  bool
	}{
		{"sat_abc", true},
		{"sat_", false},
		{"sk-abc123", false},
		{"", false},
		{"bearer xyz", false},
	}
	for _, tt := range tests {
		if got := IsSystemToken(tt.token); got != tt.want {
			t.Errorf("IsSystemToken(%q) = %v, want %v", tt.token, got, tt.want)
		}
	}
}

func TestIsValidSystemTokenFormat(t *testing.T) {
	valid := "sat_" + "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	if !IsValidSystemTokenFormat(valid) {
		t.Errorf("expected valid for well-formed token")
	}

	tests := []struct {
		name  string
		token string
	}{
		{"too short", "sat_abc"},
		{"too long", valid + "0"},
		{"uppercase hex", "sat_" + "ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef0123456789"},
		{"non-hex char", "sat_" + "gggggg0123456789abcdef0123456789abcdef0123456789abcdef0123456789"},
		{"wrong prefix", "xxx_" + "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"},
		{"no prefix", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef01234567890123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if IsValidSystemTokenFormat(tt.token) {
				t.Errorf("expected invalid for %q", tt.token)
			}
		})
	}
}

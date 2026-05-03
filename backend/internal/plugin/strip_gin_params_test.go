//go:build unit

package plugin

import "testing"

func TestStripGinParams(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/admin/monitors", "/admin/monitors"},
		{"/admin/monitors/:id", "/admin/monitors/"},
		{"/admin/monitors/:id/run", "/admin/monitors/"},
		{"/admin/monitors/:id/history", "/admin/monitors/"},
		{"/v1/chat/completions", "/v1/chat/completions"},
		{"/api/v1/plugin/foo/*rest", "/api/v1/plugin/foo/"},
		{"/:wildcard", "/"},
		{"/", "/"},
		{"", ""},
	}
	for _, tt := range tests {
		got := stripGinParams(tt.input)
		if got != tt.want {
			t.Errorf("stripGinParams(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

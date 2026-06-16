//go:build unit

package plugin

import (
	"strings"
	"testing"
)

func TestIsValidPluginName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"single_alnum", "a", true},
		{"existing_real_plugin_kebab", "channel-management", true},
		{"existing_real_plugin", "hello-world", true},
		{"underscore_middle", "a_b", true},
		{"digit_start", "9foo", true},
		{"max_len_64", strings.Repeat("a", 64), true},

		{"empty", "", false},
		{"too_long", strings.Repeat("a", 65), false},
		{"dash_prefix", "-foo", false},
		{"dash_suffix", "foo-", false},
		{"underscore_prefix", "_foo", false},
		{"underscore_suffix", "foo_", false},
		{"uppercase", "Foo", false},
		{"mixed_case", "myPlugin", false},
		{"slash", "foo/bar", false},
		{"backslash", "foo\bar", false},
		{"dot", "foo.bar", false},
		{"dotdot", "..", false},
		{"path_traversal", "../../etc/passwd", false},
		{"null_byte", "foo\x00bar", false},
		{"space", "foo bar", false},
		{"newline", "foo\nbar", false},
		{"unicode", "中文", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := IsValidPluginName(tc.in)
			if got != tc.want {
				t.Fatalf("IsValidPluginName(%q) = %v; want %v", tc.in, got, tc.want)
			}
		})
	}
}

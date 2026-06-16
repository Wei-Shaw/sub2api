//go:build unit

package wildcard

import "testing"

func TestSplitSuffix(t *testing.T) {
	cases := []struct {
		in       string
		prefix   string
		wildcard bool
	}{
		{"claude-opus-*", "claude-opus-", true},
		{"claude-opus-4", "claude-opus-4", false},
		{"*", "", true},
		{"", "", false},
		// 大小写敏感: prefix 保留原样
		{"GPT-4*", "GPT-4", true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			gotPrefix, gotWild := SplitSuffix(c.in)
			if gotPrefix != c.prefix || gotWild != c.wildcard {
				t.Fatalf("SplitSuffix(%q) = (%q, %v), want (%q, %v)",
					c.in, gotPrefix, gotWild, c.prefix, c.wildcard)
			}
		})
	}
}

func TestIsWildcard(t *testing.T) {
	if !IsWildcard("foo*") {
		t.Fatal("expected foo* to be wildcard")
	}
	if IsWildcard("foo") {
		t.Fatal("expected foo to be non-wildcard")
	}
	if !IsWildcard("*") {
		t.Fatal("expected * to be wildcard")
	}
	if IsWildcard("") {
		t.Fatal("expected empty string to be non-wildcard")
	}
}

func TestTrimSuffix(t *testing.T) {
	if got := TrimSuffix("claude-*"); got != "claude-" {
		t.Fatalf("TrimSuffix(claude-*) = %q, want claude-", got)
	}
	if got := TrimSuffix("claude"); got != "claude" {
		t.Fatalf("TrimSuffix(claude) = %q, want claude", got)
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern, target string
		want            bool
	}{
		{"*", "anything", true},
		{"*", "", true},
		{"claude-*", "claude-opus-4", true},
		{"claude-*", "gpt-4", false},
		{"claude-opus-4", "claude-opus-4", true},
		{"claude-opus-4", "claude-opus-4o", false},
		{"", "", true},
		{"", "anything", false},
	}
	for _, c := range cases {
		if got := Match(c.pattern, c.target); got != c.want {
			t.Errorf("Match(%q,%q) = %v, want %v", c.pattern, c.target, got, c.want)
		}
	}
}

package claude

import "testing"

func TestParseModelVersion(t *testing.T) {
	cases := []struct {
		in     string
		ok     bool
		family string
		major  int
		minor  int
	}{
		{"claude-opus-5-5", true, "opus", 5, 5},
		{"claude-opus-5", true, "opus", 5, 0},
		{"claude-sonnet-5-5", true, "sonnet", 5, 5},
		{"claude-haiku-5-5", true, "haiku", 5, 5},
		{"claude-fable-5-2", true, "fable", 5, 2},
		{"claude-fable-5", true, "fable", 5, 0},
		{"claude-mythos-5-2", true, "mythos", 5, 2},
		{"us.anthropic.claude-opus-4-8-v1", true, "opus", 4, 8},
		{"anthropic.claude-opus-5-5", true, "opus", 5, 5},
		{"claude-opus-4.7", true, "opus", 4, 7},
		// 日期后缀不能被当作 minor
		{"claude-sonnet-4-20250514", true, "sonnet", 4, 0},
		{"claude-sonnet-4-5-20250929", true, "sonnet", 4, 5},
		{"claude-opus-4-5@20251101", true, "opus", 4, 5},
		{"claude-3-5-sonnet-20241022", false, "", 0, 0},
		{"gpt-5.5", false, "", 0, 0},
	}
	for _, c := range cases {
		v, ok := ParseModelVersion(c.in)
		if ok != c.ok {
			t.Fatalf("%s: ok=%v want %v", c.in, ok, c.ok)
		}
		if !ok {
			continue
		}
		if v.Family != c.family || v.Major != c.major || v.Minor != c.minor {
			t.Fatalf("%s: got %+v want %s %d.%d", c.in, v, c.family, c.major, c.minor)
		}
	}
}

func TestIsGeneration5OrNewer(t *testing.T) {
	for _, m := range []string{"claude-opus-5", "claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5", "claude-fable-5", "claude-fable-5-2", "claude-mythos-5-1"} {
		if !IsGeneration5OrNewer(m) {
			t.Fatalf("%s should be gen5", m)
		}
	}
	for _, m := range []string{"claude-opus-4-8", "claude-sonnet-4-6", "claude-haiku-4-5-20251001", "claude-3-opus-20240229", "claude-sonnet-4-20250514"} {
		if IsGeneration5OrNewer(m) {
			t.Fatalf("%s should not be gen5", m)
		}
	}
}

func TestThinkingPolicyFor(t *testing.T) {
	cases := map[string]ThinkingPolicy{
		"claude-opus-5-5":   {BudgetTokensRemoved: true, DisabledRejected: true},
		"claude-fable-5-2":  {BudgetTokensRemoved: true, DisabledRejected: true},
		"claude-opus-5":     {BudgetTokensRemoved: true, DisabledOnlyUpToHigh: true},
		"claude-haiku-5-5":  {BudgetTokensRemoved: true, DisabledOnlyUpToHigh: true},
		"claude-sonnet-5-5": {BudgetTokensRemoved: true, DisabledUseBetweenTools: true},
		"claude-sonnet-5":   {BudgetTokensRemoved: true},
		"claude-opus-4-8":   {BudgetTokensRemoved: true},
		"claude-opus-4-7":   {BudgetTokensRemoved: true},
		"claude-opus-4-6":   {},
		"claude-sonnet-4-6": {},
		"claude-haiku-4-5":  {},
	}
	for model, want := range cases {
		if got := ThinkingPolicyFor(model); got != want {
			t.Fatalf("%s: got %+v want %+v", model, got, want)
		}
	}
}

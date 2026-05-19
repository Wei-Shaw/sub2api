package kiro

import "testing"

func TestParseModelAndThinking_LongestPrefixFirst(t *testing.T) {
	// "claude-sonnet-4-5" must NOT match "claude-sonnet-4" first.
	got, thinking := ParseModelAndThinking("claude-sonnet-4-5", ThinkingSuffix)
	if got != "claude-sonnet-4.5" || thinking {
		t.Fatalf("got %q, thinking=%v, want claude-sonnet-4.5, false", got, thinking)
	}
}

func TestParseModelAndThinking_ThinkingSuffix(t *testing.T) {
	got, thinking := ParseModelAndThinking("claude-sonnet-4-5-thinking", ThinkingSuffix)
	if got != "claude-sonnet-4.5" || !thinking {
		t.Fatalf("got %q, thinking=%v, want claude-sonnet-4.5, true", got, thinking)
	}
}

func TestParseModelAndThinking_DottedAlreadyMapped(t *testing.T) {
	got, _ := ParseModelAndThinking("claude-opus-4.7", ThinkingSuffix)
	if got != "claude-opus-4.7" {
		t.Fatalf("got %q", got)
	}
}

func TestParseModelAndThinking_BackwardCompatAliases(t *testing.T) {
	cases := map[string]string{
		"claude-3-5-sonnet": "claude-sonnet-4.5",
		"claude-3-opus":     "claude-sonnet-4.5",
		"claude-3-sonnet":   "claude-sonnet-4",
		"claude-3-haiku":    "claude-haiku-4.5",
	}
	for input, want := range cases {
		got, _ := ParseModelAndThinking(input, ThinkingSuffix)
		if got != want {
			t.Errorf("%q -> %q, want %q", input, got, want)
		}
	}
}

func TestParseModelAndThinking_UnknownClaudePassesThrough(t *testing.T) {
	got, _ := ParseModelAndThinking("claude-future-99", ThinkingSuffix)
	if got != "claude-future-99" {
		t.Fatalf("got %q", got)
	}
}

func TestMapModel_StripsThinking(t *testing.T) {
	if MapModel("claude-sonnet-4-6-thinking") != "claude-sonnet-4.6" {
		t.Fatal("MapModel should strip the thinking suffix")
	}
}

func TestEndpointPreference_AutoOrUnknown(t *testing.T) {
	for _, in := range []string{"", "auto", "  ", "weird"} {
		got := EndpointPreference(in)
		if len(got) != 3 || got[0].Name != "Kiro IDE" {
			t.Errorf("input %q: order wrong: %v", in, names(got))
		}
	}
}

func TestEndpointPreference_PutsPreferredFirst(t *testing.T) {
	cases := map[string]string{
		"kiro":          "Kiro IDE",
		"codewhisperer": "CodeWhisperer",
		"amazonq":       "AmazonQ",
	}
	for in, wantFirst := range cases {
		got := EndpointPreference(in)
		if len(got) != 3 {
			t.Errorf("%q: got %d endpoints", in, len(got))
			continue
		}
		if got[0].Name != wantFirst {
			t.Errorf("%q: first endpoint %q, want %q", in, got[0].Name, wantFirst)
		}
	}
}

func TestEndpoints_ReturnsAllThree(t *testing.T) {
	if len(Endpoints()) != 3 {
		t.Fatalf("Endpoints() length = %d", len(Endpoints()))
	}
}

func names(eps []Endpoint) []string {
	out := make([]string, 0, len(eps))
	for _, e := range eps {
		out = append(out, e.Name)
	}
	return out
}

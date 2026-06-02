package service

import (
	"testing"
)

func TestMatchModelRateLimits(t *testing.T) {
	rules := ModelRateLimits{
		{Pattern: "claude-opus-4-7", Limit5h: 10},
		{Pattern: "claude-opus-*", Limit5h: 100},
		{Pattern: "", Limit5h: 999}, // should be ignored
		{Pattern: "gpt-*", Limit5h: 50},
	}

	tests := []struct {
		model   string
		wantPat []string
	}{
		{"claude-opus-4-7", []string{"claude-opus-4-7", "claude-opus-*"}},
		{"claude-opus-4-8", []string{"claude-opus-*"}},
		{"claude-sonnet-4-6", nil},
		{"gpt-4o", []string{"gpt-*"}},
		{"", nil},
	}
	for _, tc := range tests {
		got := MatchModelRateLimits(rules, tc.model)
		if len(got) != len(tc.wantPat) {
			t.Errorf("model=%q: want %d matches, got %d (%v)", tc.model, len(tc.wantPat), len(got), got)
			continue
		}
		for i, r := range got {
			if r.Pattern != tc.wantPat[i] {
				t.Errorf("model=%q: match[%d] pattern = %q, want %q", tc.model, i, r.Pattern, tc.wantPat[i])
			}
		}
	}
}

func TestMatchModelRateLimitsCatchAll(t *testing.T) {
	rules := ModelRateLimits{{Pattern: "*", Limit5h: 5}}
	got := MatchModelRateLimits(rules, "anything-goes")
	if len(got) != 1 || got[0].Pattern != "*" {
		t.Errorf("catch-all should match any model; got %v", got)
	}
}

func TestEffectiveModelRateLimits(t *testing.T) {
	groupRules := ModelRateLimits{{Pattern: "claude-*", Limit5h: 10}}
	keyRules := ModelRateLimits{{Pattern: "claude-opus-*", Limit5h: 20}}

	group := &Group{DefaultModelRateLimits: groupRules}
	keyOverride := &APIKey{ModelRateLimits: keyRules}
	keyEmpty := &APIKey{}

	// Key override wins.
	if got := EffectiveModelRateLimits(keyOverride, group); len(got) != 1 || got[0].Pattern != "claude-opus-*" {
		t.Errorf("key override should win; got %v", got)
	}
	// Empty key inherits group.
	if got := EffectiveModelRateLimits(keyEmpty, group); len(got) != 1 || got[0].Pattern != "claude-*" {
		t.Errorf("empty key should inherit group; got %v", got)
	}
	// Both empty → nil.
	if got := EffectiveModelRateLimits(keyEmpty, &Group{}); got != nil {
		t.Errorf("both empty should return nil; got %v", got)
	}
	// Nil key + nil group → nil.
	if got := EffectiveModelRateLimits(nil, nil); got != nil {
		t.Errorf("nil/nil should return nil; got %v", got)
	}
}

func TestHasAnyLimit(t *testing.T) {
	if (ModelRateLimit{}).HasAnyLimit() {
		t.Error("empty rule should report HasAnyLimit=false")
	}
	if !(ModelRateLimit{Limit5h: 1}).HasAnyLimit() {
		t.Error("rule with Limit5h>0 should report HasAnyLimit=true")
	}
	if !(ModelRateLimit{Limit7d: 0.5}).HasAnyLimit() {
		t.Error("rule with Limit7d>0 should report HasAnyLimit=true")
	}
}

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
	groupRules := ModelRateLimits{{Pattern: "claude-*", Limit5h: 10, Limit1d: 50}}
	keyRules := ModelRateLimits{{Pattern: "claude-opus-*", Limit5h: 20}}

	group := &Group{DefaultModelRateLimits: groupRules}
	keyOverride := &APIKey{ModelRateLimits: keyRules}
	keyEmpty := &APIKey{}

	// Both key and group have rules → merged union (different patterns).
	got := EffectiveModelRateLimits(keyOverride, group)
	if len(got) != 2 {
		t.Fatalf("expected 2 merged rules, got %d: %v", len(got), got)
	}
	// Group rule should be present.
	foundGroup := false
	for _, r := range got {
		if r.Pattern == "claude-*" && r.Limit5h == 10 {
			foundGroup = true
		}
	}
	if !foundGroup {
		t.Errorf("group rule should be present in merged result; got %v", got)
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

func TestMergedModelRateLimits_SamePattern(t *testing.T) {
	groupRules := ModelRateLimits{{Pattern: "claude-*", Limit5h: 10, Limit1d: 50, Limit7d: 100}}
	keyRules := ModelRateLimits{{Pattern: "claude-*", Limit5h: 5, Limit1d: 80, Limit7d: 0}}
	group := &Group{DefaultModelRateLimits: groupRules}
	key := &APIKey{ModelRateLimits: keyRules}

	got := MergedModelRateLimits(key, group)
	if len(got) != 1 {
		t.Fatalf("same pattern should merge to 1 rule; got %d", len(got))
	}
	r := got[0]
	// min(10,5)=5, min(50,80)=50, pickMin(100,0)=100 (0 means unlimited)
	if r.Limit5h != 5 {
		t.Errorf("Limit5h: want 5, got %v", r.Limit5h)
	}
	if r.Limit1d != 50 {
		t.Errorf("Limit1d: want 50, got %v", r.Limit1d)
	}
	if r.Limit7d != 100 {
		t.Errorf("Limit7d: want 100, got %v", r.Limit7d)
	}
}

func TestMergedModelRateLimits_DifferentPatterns(t *testing.T) {
	groupRules := ModelRateLimits{{Pattern: "claude-*", Limit5h: 10}}
	keyRules := ModelRateLimits{{Pattern: "gpt-*", Limit5h: 20}}
	group := &Group{DefaultModelRateLimits: groupRules}
	key := &APIKey{ModelRateLimits: keyRules}

	got := MergedModelRateLimits(key, group)
	if len(got) != 2 {
		t.Fatalf("different patterns should produce union of 2; got %d", len(got))
	}
}

func TestCapLimitsByGroup(t *testing.T) {
	groupLimits := ModelRateLimits{
		{Pattern: "claude-*", Limit5h: 10, Limit1d: 50, Limit7d: 0},
	}

	tests := []struct {
		name       string
		userLimits ModelRateLimits
		want5h     float64
		want1d     float64
		want7d     float64
	}{
		{"user below ceiling", ModelRateLimits{{Pattern: "claude-*", Limit5h: 5, Limit1d: 30, Limit7d: 200}}, 5, 30, 200},
		{"user above ceiling", ModelRateLimits{{Pattern: "claude-*", Limit5h: 20, Limit1d: 80, Limit7d: 50}}, 10, 50, 50},
		{"user zero (no cap) → gets ceiling", ModelRateLimits{{Pattern: "claude-*", Limit5h: 0, Limit1d: 0, Limit7d: 0}}, 10, 50, 0},
		{"no matching pattern → passthrough", ModelRateLimits{{Pattern: "gpt-*", Limit5h: 999}}, 999, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CapLimitsByGroup(tc.userLimits, groupLimits)
			if len(got) != 1 {
				t.Fatalf("expected 1 rule, got %d", len(got))
			}
			r := got[0]
			if r.Limit5h != tc.want5h {
				t.Errorf("Limit5h: want %v, got %v", tc.want5h, r.Limit5h)
			}
			if r.Limit1d != tc.want1d {
				t.Errorf("Limit1d: want %v, got %v", tc.want1d, r.Limit1d)
			}
			if r.Limit7d != tc.want7d {
				t.Errorf("Limit7d: want %v, got %v", tc.want7d, r.Limit7d)
			}
		})
	}

	// Nil group → passthrough.
	orig := ModelRateLimits{{Pattern: "x", Limit5h: 99}}
	if got := CapLimitsByGroup(orig, nil); len(got) != 1 || got[0].Limit5h != 99 {
		t.Errorf("nil group should passthrough; got %v", got)
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

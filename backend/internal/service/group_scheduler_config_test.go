package service

import "testing"

func TestGroupSchedulerConfigDefaultsToLegacy(t *testing.T) {
	cfg := NormalizeGroupSchedulerConfig(GroupSchedulerConfig{})
	if cfg.Strategy != "legacy" || cfg.SelectionMode != "weighted" || cfg.StickyBindingMode != "keep_original" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if err := ValidateGroupSchedulerConfig(cfg); err != nil {
		t.Fatalf("default scheduler config should validate: %v", err)
	}
}

func TestGroupSchedulerConfigAcceptsStrictHealthSelection(t *testing.T) {
	cfg := NormalizeGroupSchedulerConfig(GroupSchedulerConfig{Strategy: "high_availability", SelectionMode: "strict_health"})
	if err := ValidateGroupSchedulerConfig(cfg); err != nil {
		t.Fatalf("strict health scheduler config should validate: %v", err)
	}
}

func TestGroupSchedulerConfigRejectsUnknownSelectionMode(t *testing.T) {
	if err := ValidateGroupSchedulerConfig(GroupSchedulerConfig{SelectionMode: "random"}); err == nil {
		t.Fatal("unknown scheduler selection mode must be rejected")
	}
}

func TestGroupSchedulerConfigRejectsUnknownStrategy(t *testing.T) {
	if err := ValidateGroupSchedulerConfig(GroupSchedulerConfig{Strategy: "random"}); err == nil {
		t.Fatal("unknown scheduler strategy must be rejected")
	}
}

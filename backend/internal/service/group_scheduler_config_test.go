package service

import "testing"

func TestGroupSchedulerConfigDefaultsToLegacy(t *testing.T) {
	cfg := NormalizeGroupSchedulerConfig(GroupSchedulerConfig{})
	if cfg.Strategy != "legacy" || cfg.StickyBindingMode != "keep_original" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if err := ValidateGroupSchedulerConfig(cfg); err != nil {
		t.Fatalf("default scheduler config should validate: %v", err)
	}
}

func TestGroupSchedulerConfigRejectsUnknownStrategy(t *testing.T) {
	if err := ValidateGroupSchedulerConfig(GroupSchedulerConfig{Strategy: "random"}); err == nil {
		t.Fatal("unknown scheduler strategy must be rejected")
	}
}

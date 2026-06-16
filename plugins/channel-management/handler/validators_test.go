//go:build unit

package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/service"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// TestRegisterCustomValidators verifies that all four enum tags get registered
// onto gin's default validator engine and accept exactly the values listed by
// service.* enum helpers (plus the empty string, mirroring the historical
// `omitempty,oneof=...` semantics).
func TestRegisterCustomValidators(t *testing.T) {
	if err := RegisterCustomValidators(); err != nil {
		t.Fatalf("RegisterCustomValidators returned error: %v", err)
	}

	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		t.Fatalf("expected validator engine *validator.Validate, got %T", binding.Validator.Engine())
	}

	checks := []struct {
		tag     string
		allowed []string
	}{
		{"channel_status", service.ChannelStatuses()},
		{"billing_mode", service.BillingModes()},
		{"billing_model_source", service.BillingModelSources()},
		{"platform", service.Platforms()},
	}
	for _, c := range checks {
		// Empty string must always pass (omitempty equivalent).
		if err := v.Var("", c.tag); err != nil {
			t.Errorf("tag %q rejected empty string: %v", c.tag, err)
		}
		// Every documented allowed value must pass.
		for _, val := range c.allowed {
			if err := v.Var(val, c.tag); err != nil {
				t.Errorf("tag %q rejected allowed value %q: %v", c.tag, val, err)
			}
		}
		// A clearly invalid value must be rejected.
		if err := v.Var("__definitely_not_a_valid_enum_value__", c.tag); err == nil {
			t.Errorf("tag %q unexpectedly accepted invalid value", c.tag)
		}
	}
}

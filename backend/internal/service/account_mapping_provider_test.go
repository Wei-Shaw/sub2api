//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

func TestAccountGetModelMapping_AntigravityPreservesExplicitGemini31Aliases(t *testing.T) {
	t.Parallel()

	account := &Account{
		Platform: PlatformAntigravity,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				domain.AntigravityGemini31ProAgentModel: domain.AntigravityGemini31ProAgentModel,
				"gemini-3.1-pro-high":                   "gemini-3.1-pro-high",
				"gemini-3.1-pro-preview":                "gemini-3.1-pro-high",
			},
		},
	}

	mapping := account.GetModelMapping()

	if got := mapping["gemini-3.1-pro"]; got != "" {
		t.Fatalf("unconfigured alias must remain unavailable, got %q", got)
	}
	if got := mapping["gemini-3.1-pro-high"]; got != "gemini-3.1-pro-high" {
		t.Fatalf("expected gemini-3.1-pro-high to map to %q, got %q", domain.AntigravityGemini31ProAgentModel, got)
	}
	if got := mapping["gemini-3.1-pro-preview"]; got != "gemini-3.1-pro-high" {
		t.Fatalf("expected gemini-3.1-pro-preview to map to %q, got %q", domain.AntigravityGemini31ProAgentModel, got)
	}
}

func TestAccountGetModelMapping_AntigravityPreservesGemini31ProOverrides(t *testing.T) {
	t.Parallel()

	account := &Account{
		Platform: PlatformAntigravity,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				domain.AntigravityGemini31ProAgentModel: domain.AntigravityGemini31ProAgentModel,
				"gemini-3.1-pro-high":                   "custom-high",
				"gemini-3.1-pro-preview":                "custom-preview",
			},
		},
	}

	mapping := account.GetModelMapping()

	if got := mapping["gemini-3.1-pro-high"]; got != "custom-high" {
		t.Fatalf("expected gemini-3.1-pro-high override to be preserved, got %q", got)
	}
	if got := mapping["gemini-3.1-pro-preview"]; got != "custom-preview" {
		t.Fatalf("expected gemini-3.1-pro-preview override to be preserved, got %q", got)
	}
	if got := mapping["gemini-3.1-pro"]; got != "" {
		t.Fatalf("unconfigured alias must remain unavailable, got %q", got)
	}
}

func TestAccountGetModelMapping_AntigravityGemini31ProAliasesRespectWildcard(t *testing.T) {
	t.Parallel()

	account := &Account{
		Platform: PlatformAntigravity,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				domain.AntigravityGemini31ProAgentModel: domain.AntigravityGemini31ProAgentModel,
				"gemini-3.1-*":                          "custom-wildcard",
			},
		},
	}

	mapping := account.GetModelMapping()

	if got := mapping["gemini-3.1-pro"]; got != "" {
		t.Fatalf("expected gemini-3.1-pro exact alias to stay unset when wildcard exists, got %q", got)
	}
	if got := mapping["gemini-3.1-pro-high"]; got != "" {
		t.Fatalf("expected gemini-3.1-pro-high exact alias to stay unset when wildcard exists, got %q", got)
	}
	if got := mapping["gemini-3.1-pro-preview"]; got != "" {
		t.Fatalf("expected gemini-3.1-pro-preview exact alias to stay unset when wildcard exists, got %q", got)
	}
}

func TestAccountGetModelMapping_AntigravityDoesNotInjectDefaultPassthroughs(t *testing.T) {
	account := &Account{
		Platform: PlatformAntigravity,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"gemini-3-pro-high": "gemini-3.1-pro-high",
			},
		},
	}

	mapping := account.GetModelMapping()
	if mapping["gemini-3-flash"] != "" {
		t.Fatalf("expected gemini-3-flash unconfigured passthrough to remain absent, got: %q", mapping["gemini-3-flash"])
	}
	if mapping["gemini-3.1-pro-high"] != "" {
		t.Fatalf("expected gemini-3.1-pro-high unconfigured passthrough to remain absent, got: %q", mapping["gemini-3.1-pro-high"])
	}
	if mapping["gemini-3.1-pro-low"] != "" {
		t.Fatalf("expected gemini-3.1-pro-low unconfigured passthrough to remain absent, got: %q", mapping["gemini-3.1-pro-low"])
	}
}

func TestAccountGetModelMapping_GoogleOneUsesConservativeDefaults(t *testing.T) {
	account := &Account{
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"oauth_type": "google_one",
		},
	}

	mapping := account.GetModelMapping()
	for _, model := range []string{"gemini-2.0-flash", "gemini-2.5-flash", "gemini-2.5-pro"} {
		if mapping[model] != model {
			t.Fatalf("expected Google One model %q to map to itself, got %q", model, mapping[model])
		}
	}
	for _, model := range []string{"gemini-2.5-flash-image", "gemini-3.1-flash-image", "gemini-3.5-flash"} {
		if _, ok := mapping[model]; ok {
			t.Fatalf("did not expect unsupported Google One model %q", model)
		}
	}
	if account.IsModelSupported("gemini-3.5-flash") {
		t.Fatal("Google One defaults must not treat unsupported models as eligible")
	}
}

func TestAccountGetModelMapping_GoogleOnePreservesExplicitMapping(t *testing.T) {
	account := &Account{
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"oauth_type": "google_one",
			"model_mapping": map[string]any{
				"custom-model": "gemini-2.5-flash",
			},
		},
	}

	mapping := account.GetModelMapping()
	if mapping["custom-model"] != "gemini-2.5-flash" {
		t.Fatalf("expected explicit Google One mapping to be preserved, got %v", mapping)
	}
	if _, ok := mapping["gemini-2.5-flash"]; ok {
		t.Fatalf("did not expect defaults to overwrite an explicit mapping: %v", mapping)
	}
}

func TestAccountGetModelMapping_AntigravityRespectsWildcardOverride(t *testing.T) {
	account := &Account{
		Platform: PlatformAntigravity,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"gemini-3*": "gemini-3.1-pro-high",
			},
		},
	}

	mapping := account.GetModelMapping()
	if _, exists := mapping["gemini-3-flash"]; exists {
		t.Fatalf("did not expect explicit gemini-3-flash passthrough when wildcard already exists")
	}
	if _, exists := mapping["gemini-3.1-pro-high"]; exists {
		t.Fatalf("did not expect explicit gemini-3.1-pro-high passthrough when wildcard already exists")
	}
	if _, exists := mapping["gemini-3.1-pro-low"]; exists {
		t.Fatalf("did not expect explicit gemini-3.1-pro-low passthrough when wildcard already exists")
	}
	if mapped := account.GetMappedModel("gemini-3-flash"); mapped != "gemini-3.1-pro-high" {
		t.Fatalf("expected wildcard mapping to stay effective, got: %q", mapped)
	}
}

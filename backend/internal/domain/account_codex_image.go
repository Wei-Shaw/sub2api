package domain

import "strings"

// Codex image-generation pure Account methods + their pure helpers.
// Lifted from internal/service/codex_image_generation_bridge.go in Phase 3.

const (
	featureKeyCodexImageGenerationExplicitToolPolicy = "codex_image_generation_explicit_tool_policy"

	// FeatureKeyCodexImageGenerationExplicitToolPolicy is the exported form.
	FeatureKeyCodexImageGenerationExplicitToolPolicy = featureKeyCodexImageGenerationExplicitToolPolicy

	codexImageGenerationExplicitToolPolicyAllow = "allow"
	codexImageGenerationExplicitToolPolicyStrip = "strip"
)

// CodexImageGenerationExplicitToolPolicyAllow/Strip are exported so the
// service layer can re-export the original unexported names.
const (
	CodexImageGenerationExplicitToolPolicyAllow = codexImageGenerationExplicitToolPolicyAllow
	CodexImageGenerationExplicitToolPolicyStrip = codexImageGenerationExplicitToolPolicyStrip
)

// boolOverrideFromMap / stringOverrideFromMap are multi-key Extra readers used
// only by the Codex image-generation methods. (boolOverridePtr already lives
// in channel.go in this package.)
func boolOverrideFromMap(values map[string]any, keys ...string) *bool {
	if values == nil {
		return nil
	}
	for _, key := range keys {
		if v, ok := values[key].(bool); ok {
			return boolOverridePtr(v)
		}
	}
	return nil
}

func stringOverrideFromMap(values map[string]any, keys ...string) (string, bool) {
	if values == nil {
		return "", false
	}
	for _, key := range keys {
		if v, ok := values[key].(string); ok {
			return v, true
		}
	}
	return "", false
}

func normalizeCodexImageGenerationExplicitToolPolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case codexImageGenerationExplicitToolPolicyStrip, "remove", "drop":
		return codexImageGenerationExplicitToolPolicyStrip
	default:
		return codexImageGenerationExplicitToolPolicyAllow
	}
}

// CodexImageGenerationBridgeOverride returns the account-level override for Codex
// image_generation bridge injection. Nil means follow the channel/global policy.
func (a *Account) CodexImageGenerationBridgeOverride() *bool {
	if a == nil || a.Platform != PlatformOpenAI || a.Extra == nil {
		return nil
	}
	if override := boolOverrideFromMap(a.Extra, FeatureKeyCodexImageGenerationBridge, "codex_image_generation_bridge_enabled"); override != nil {
		return override
	}
	openaiConfig, _ := a.Extra[PlatformOpenAI].(map[string]any)
	return boolOverrideFromMap(openaiConfig, FeatureKeyCodexImageGenerationBridge, "codex_image_generation_bridge_enabled")
}

// CodexImageGenerationExplicitToolPolicy returns the account-level policy for
// client-provided Codex /responses image_generation tools. Unknown or unset
// values default to allow to preserve existing behavior.
func (a *Account) CodexImageGenerationExplicitToolPolicy() string {
	if a == nil || a.Platform != PlatformOpenAI || a.Extra == nil {
		return codexImageGenerationExplicitToolPolicyAllow
	}
	if policy, ok := stringOverrideFromMap(a.Extra, featureKeyCodexImageGenerationExplicitToolPolicy); ok {
		return normalizeCodexImageGenerationExplicitToolPolicy(policy)
	}
	openaiConfig, _ := a.Extra[PlatformOpenAI].(map[string]any)
	if policy, ok := stringOverrideFromMap(openaiConfig, featureKeyCodexImageGenerationExplicitToolPolicy); ok {
		return normalizeCodexImageGenerationExplicitToolPolicy(policy)
	}
	return codexImageGenerationExplicitToolPolicyAllow
}

// Package openai provides helpers and types for OpenAI API integration.
package openai

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// Model represents an OpenAI model
type Model struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created"`
	OwnedBy     string `json:"owned_by"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
}

// DefaultModels OpenAI models list
var DefaultModels = []Model{
	{ID: "gpt-5.6-sol", Object: "model", Created: 1780876800, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.6 Sol"},
	{ID: "gpt-6", Object: "model", Created: 1788480000, OwnedBy: "openai", Type: "model", DisplayName: "GPT-6 (Astra)"},
	{ID: "gpt-5.6", Object: "model", Created: 1780876800, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.6 (Sol)"},
	{ID: "gpt-5.6-terra", Object: "model", Created: 1780876800, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.6 Terra"},
	{ID: "gpt-5.6-luna", Object: "model", Created: 1780876800, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.6 Luna"},
	{ID: "gpt-6.2-sol", Object: "model", Created: 1794873600, OwnedBy: "openai", Type: "model", DisplayName: "GPT-6.2 Sol"},
	{ID: "gpt-6.2-astra", Object: "model", Created: 1794873600, OwnedBy: "openai", Type: "model", DisplayName: "GPT-6.2 Astra"},
	{ID: "gpt-6.1-terra", Object: "model", Created: 1792972800, OwnedBy: "openai", Type: "model", DisplayName: "GPT-6.1 Terra"},
	{ID: "gpt-6.1-sol", Object: "model", Created: 1790640000, OwnedBy: "openai", Type: "model", DisplayName: "GPT-6.1 Sol"},
	{ID: "gpt-6-sol", Object: "model", Created: 1790035200, OwnedBy: "openai", Type: "model", DisplayName: "GPT-6 Sol"},
	{ID: "gpt-6-luna", Object: "model", Created: 1790035200, OwnedBy: "openai", Type: "model", DisplayName: "GPT-6 Luna"},
	{ID: "gpt-6-astra", Object: "model", Created: 1788480000, OwnedBy: "openai", Type: "model", DisplayName: "GPT-6 Astra"},
	{ID: "gpt-5.5", Object: "model", Created: 1776873600, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.5"},
	{ID: "gpt-5.4", Object: "model", Created: 1738368000, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.4"},
	{ID: "gpt-5.4-mini", Object: "model", Created: 1738368000, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.4 Mini"},
	{ID: "gpt-5.3-codex-spark", Object: "model", Created: 1735689600, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.3 Codex Spark"},
	{ID: "codex-auto-review", Object: "model", Created: 1776902400, OwnedBy: "openai", Type: "model", DisplayName: "Codex Auto Review"},
	{ID: "gpt-5.2", Object: "model", Created: 1733875200, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.2"},
	{ID: "gpt-image-1", Object: "model", Created: 1733875200, OwnedBy: "openai", Type: "model", DisplayName: "GPT Image 1"},
	{ID: "gpt-image-1.5", Object: "model", Created: 1735689600, OwnedBy: "openai", Type: "model", DisplayName: "GPT Image 1.5"},
	{ID: "gpt-image-2", Object: "model", Created: 1738368000, OwnedBy: "openai", Type: "model", DisplayName: "GPT Image 2"},
	{ID: "gpt-image-2.5-flare", Object: "model", Created: 1788825600, OwnedBy: "openai", Type: "model", DisplayName: "GPT Image 2.5 Flare"},
	{ID: "gpt-image-2.5-sunburst", Object: "model", Created: 1788825600, OwnedBy: "openai", Type: "model", DisplayName: "GPT Image 2.5 Sunburst"},
	{ID: "gpt-image-2.6-flare", Object: "model", Created: 1793750400, OwnedBy: "openai", Type: "model", DisplayName: "GPT Image 2.6 Flare"},
	{ID: "gpt-image-2.6-sunburst", Object: "model", Created: 1793750400, OwnedBy: "openai", Type: "model", DisplayName: "GPT Image 2.6 Sunburst"},
}

// DefaultModelIDs returns the default model ID list
func DefaultModelIDs() []string {
	ids := make([]string, len(DefaultModels))
	for i, m := range DefaultModels {
		ids[i] = m.ID
	}
	return ids
}

// DefaultTestModel default model for testing OpenAI accounts
const DefaultTestModel = "gpt-5.4"

// CodexUsageProbeModel is the model used for OAuth Codex usage probes.
const CodexUsageProbeModel = "codex-auto-review"

// DefaultInstructions default instructions for non-Codex CLI requests.
// 内容为真实 Codex CLI 的 GPT-5-Codex base prompt（codex 系模型默认）。
//
//go:embed instructions.txt
var DefaultInstructions string

// instructionsGPT51 / instructionsGPT52 / instructionsGPT55 / instructionsGPT56 /
// instructionsGPT6Astra / instructionsGPT6Sol / instructionsGPT6Luna are official
// Codex base prompts synced from openai/codex rust-v0.172.0 for model-aware
// default instruction selection. GPT-5.5 remains the fallback for unpinned models.
//
//go:embed instructions_gpt5_1.txt
var instructionsGPT51 string

//go:embed instructions_gpt5_2.txt
var instructionsGPT52 string

//go:embed instructions_gpt5_5.txt
var instructionsGPT55 string

//go:embed instructions_gpt5_6.txt
var instructionsGPT56 string

//go:embed instructions_gpt6_astra.txt
var instructionsGPT6Astra string

//go:embed instructions_gpt6_sol.txt
var instructionsGPT6Sol string

//go:embed instructions_gpt6_luna.txt
var instructionsGPT6Luna string

// GPT-6.2 descriptors preserve catalog metadata from the 2026-11-17 Codex models
// response while syncing instructions_template to openai/codex rust-v0.172.0.
//
//go:embed codex_gpt62_sol.json
var CodexGPT62SolMetadata []byte

//go:embed codex_gpt62_astra.json
var CodexGPT62AstraMetadata []byte

// CodexGPT61SolMetadata / CodexGPT61TerraMetadata are official descriptors from
// openai/codex with instructions_template synced to rust-v0.172.0.
//
//go:embed codex_gpt61_sol.json
var CodexGPT61SolMetadata []byte

//go:embed codex_gpt61_terra.json
var CodexGPT61TerraMetadata []byte

// codexInstructionsTemplate returns the base prompt bundled in an official
// Codex model descriptor.
func codexInstructionsTemplate(descriptor []byte) string {
	var metadata struct {
		ModelMessages struct {
			InstructionsTemplate string `json:"instructions_template"`
		} `json:"model_messages"`
	}
	if err := json.Unmarshal(descriptor, &metadata); err != nil {
		panic(err)
	}
	return metadata.ModelMessages.InstructionsTemplate
}

// latestCodexInstructions 返回当前已知最新版本的 Codex base instructions，
// 当前为 GPT-5.5；若 5.5 prompt 意外为空则回退到 DefaultInstructions 保证非空。
func latestCodexInstructions() string {
	if v := strings.TrimSpace(instructionsGPT55); v != "" {
		return instructionsGPT55
	}
	return DefaultInstructions
}

// CanonicalizeOpenAIModelAliasSpelling normalizes provider prefixes, case,
// separators, and known compact spellings used by OpenAI model aliases.
func CanonicalizeOpenAIModelAliasSpelling(model string) string {
	model = strings.TrimSpace(model)
	if slash := strings.LastIndexByte(model, '/'); slash >= 0 {
		model = strings.TrimSpace(model[slash+1:])
	}
	model = strings.ToLower(model)
	if model == "" {
		return ""
	}

	normalized := strings.ReplaceAll(model, "_", "-")
	normalized = strings.Join(strings.Fields(normalized), "-")
	for strings.Contains(normalized, "--") {
		normalized = strings.ReplaceAll(normalized, "--", "-")
	}

	if strings.HasPrefix(normalized, "gpt5") {
		normalized = "gpt-5" + strings.TrimPrefix(normalized, "gpt5")
	}
	if !strings.HasPrefix(normalized, "gpt-") && !strings.Contains(normalized, "codex") {
		return ""
	}

	replacements := []struct {
		from string
		to   string
	}{
		{"gpt-5.4mini", "gpt-5.4-mini"},
		{"gpt-5.4nano", "gpt-5.4-nano"},
		{"gpt-5.3-codexspark", "gpt-5.3-codex-spark"},
		{"gpt-5.3codexspark", "gpt-5.3-codex-spark"},
		{"gpt-5.3codex", "gpt-5.3-codex"},
	}
	for _, replacement := range replacements {
		normalized = strings.ReplaceAll(normalized, replacement.from, replacement.to)
	}
	return normalized
}

// CodexBaseInstructionsForModel returns the bundled official Codex base prompt
// matching the model family (synced from openai/codex rust-v0.172.0):
//   - gpt-6.2-sol / gpt-6.2-astra -> GPT-6.2 instructions_template
//   - gpt-6.1-sol / gpt-6.1-terra -> GPT-6.1 instructions_template
//   - gpt-6-sol / gpt-6-luna -> GPT-6 Sol / Luna prompt
//   - gpt-6 / gpt-6-astra -> GPT-6 Astra prompt
//   - *codex* models -> GPT-5-Codex prompt
//   - gpt-5.6* models -> GPT-5.6 prompt
//   - gpt-5.5* -> GPT-5.5 prompt
//   - gpt-5.2* / gpt-5.1* -> GPT-5.2 / GPT-5.1 prompt
//   - others -> fallback to latestCodexInstructions (GPT-5.5)
func CodexBaseInstructionsForModel(model string) string {
	canonical := CanonicalizeOpenAIModelAliasSpelling(model)
	switch {
	case GPT62ModelID(canonical) == "gpt-6.2-sol":
		return codexInstructionsTemplate(CodexGPT62SolMetadata)
	case GPT62ModelID(canonical) == "gpt-6.2-astra":
		return codexInstructionsTemplate(CodexGPT62AstraMetadata)
	case IsGPT61SolModelSpelling(canonical):
		return codexInstructionsTemplate(CodexGPT61SolMetadata)
	case IsGPT61TerraModelSpelling(canonical):
		return codexInstructionsTemplate(CodexGPT61TerraMetadata)
	case IsGPT6SolOrLunaModelSpelling(canonical):
		if canonical == "gpt-6-sol" || strings.HasPrefix(canonical, "gpt-6-sol-") {
			if v := strings.TrimSpace(instructionsGPT6Sol); v != "" {
				return instructionsGPT6Sol
			}
		}
		if canonical == "gpt-6-luna" || strings.HasPrefix(canonical, "gpt-6-luna-") {
			if v := strings.TrimSpace(instructionsGPT6Luna); v != "" {
				return instructionsGPT6Luna
			}
		}
	case canonical == "gpt-6" || canonical == "gpt-6-astra" || strings.HasPrefix(canonical, "gpt-6-astra-"):
		if v := strings.TrimSpace(instructionsGPT6Astra); v != "" {
			return instructionsGPT6Astra
		}
	case strings.Contains(canonical, "codex"):
		return DefaultInstructions
	case strings.HasPrefix(canonical, "gpt-5.6"):
		if v := strings.TrimSpace(instructionsGPT56); v != "" {
			return instructionsGPT56
		}
	case strings.HasPrefix(canonical, "gpt-5.5"):
		return latestCodexInstructions()
	case strings.HasPrefix(canonical, "gpt-5.2"):
		if v := strings.TrimSpace(instructionsGPT52); v != "" {
			return instructionsGPT52
		}
	case strings.HasPrefix(canonical, "gpt-5.1"):
		if v := strings.TrimSpace(instructionsGPT51); v != "" {
			return instructionsGPT51
		}
	}
	return latestCodexInstructions()
}

// IsGPT6SolOrLunaModelSpelling recognizes official IDs and existing local effort/compact suffixes.
func IsGPT6SolOrLunaModelSpelling(model string) bool {
	canonical := CanonicalizeOpenAIModelAliasSpelling(model)
	for _, base := range []string{"gpt-6-sol", "gpt-6-luna"} {
		if canonical == base {
			return true
		}
		suffix, ok := strings.CutPrefix(canonical, base+"-")
		if ok {
			switch suffix {
			case "none", "low", "medium", "high", "xhigh", "max", "openai-compact":
				return true
			}
		}
	}
	return false
}

// GPT62ModelID returns the published model ID for local effort and compact
// spellings. Keep disabled effort suffixes identifiable for request validation.
func GPT62ModelID(model string) string {
	canonical := CanonicalizeOpenAIModelAliasSpelling(model)
	for _, base := range []string{"gpt-6.2-sol", "gpt-6.2-astra"} {
		if canonical == base {
			return base
		}
		suffix, ok := strings.CutPrefix(canonical, base+"-")
		if ok {
			switch suffix {
			case "none", "minimal", "low", "medium", "high", "xhigh", "max", "openai-compact":
				return base
			}
		}
	}
	return ""
}

// IsGPT61SolModelSpelling recognizes the published model and local effort/compact
// spellings. Invalid effort suffixes remain identifiable for request validation.
func IsGPT61SolModelSpelling(model string) bool {
	canonical := CanonicalizeOpenAIModelAliasSpelling(model)
	if canonical == "gpt-6.1-sol" {
		return true
	}
	suffix, ok := strings.CutPrefix(canonical, "gpt-6.1-sol-")
	if !ok {
		return false
	}
	switch suffix {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "openai-compact":
		return true
	default:
		return false
	}
}

// IsGPT61TerraModelSpelling recognizes the published model and local effort/compact
// spellings. Invalid effort suffixes remain identifiable for request validation.
func IsGPT61TerraModelSpelling(model string) bool {
	canonical := CanonicalizeOpenAIModelAliasSpelling(model)
	if canonical == "gpt-6.1-terra" {
		return true
	}
	suffix, ok := strings.CutPrefix(canonical, "gpt-6.1-terra-")
	if !ok {
		return false
	}
	switch suffix {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "openai-compact":
		return true
	default:
		return false
	}
}

// RequiresResponsesForToolCalls reports models whose tool calls are only
// available through the Responses API.
func RequiresResponsesForToolCalls(model string) bool {
	return GPT62ModelID(model) != "" || IsGPT61SolModelSpelling(model) || IsGPT61TerraModelSpelling(model)
}

// ValidateGPT61SolReasoningEffort rejects disabled reasoning instead of silently
// increasing the client's requested effort on compatibility paths.
func ValidateGPT61SolReasoningEffort(model, effort string) error {
	if IsGPT61SolModelSpelling(model) {
		switch strings.ToLower(strings.TrimSpace(effort)) {
		case "none", "minimal":
			return fmt.Errorf("gpt-6.1-sol does not support reasoning effort %q; use low, medium, high, xhigh or max", effort)
		}
	}
	return nil
}

// ValidateGPT61TerraReasoningEffort rejects the unpublished minimal level;
// GPT-6.1 Terra accepts none through max.
func ValidateGPT61TerraReasoningEffort(model, effort string) error {
	if IsGPT61TerraModelSpelling(model) && strings.EqualFold(strings.TrimSpace(effort), "minimal") {
		return fmt.Errorf("gpt-6.1-terra does not support reasoning effort %q; use none, low, medium, high, xhigh or max", effort)
	}
	return nil
}

// ValidateGPT61ReasoningEffort applies the published effort scale of each
// GPT-6.1 and GPT-6.2 model.
func ValidateGPT61ReasoningEffort(model, effort string) error {
	if err := ValidateGPT62ReasoningEffort(model, effort); err != nil {
		return err
	}
	if err := ValidateGPT61SolReasoningEffort(model, effort); err != nil {
		return err
	}
	return ValidateGPT61TerraReasoningEffort(model, effort)
}

// ValidateGPT62ReasoningEffort rejects disabled and minimal reasoning on GPT-6.2.
func ValidateGPT62ReasoningEffort(model, effort string) error {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "none", "minimal":
		if base := GPT62ModelID(model); base != "" {
			return fmt.Errorf("%s does not support reasoning effort %q; use low, medium, high, xhigh or max", base, effort)
		}
	}
	return nil
}

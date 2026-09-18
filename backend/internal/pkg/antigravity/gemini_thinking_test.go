package antigravity

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveGeminiThinkingMode(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  GeminiThinkingMode
	}{
		{name: "gemini 3 flash tiered uses level", model: "gemini-3.8-flash-tiered", want: GeminiThinkingLevel},
		{name: "gemini 3 flash low alias uses level", model: "gemini-3.8-flash-low", want: GeminiThinkingLevel},
		{name: "gemini 3 flash medium alias uses level", model: "gemini-3.8-flash-medium", want: GeminiThinkingLevel},
		{name: "gemini 3 flash high alias uses level", model: "gemini-3.8-flash-high", want: GeminiThinkingLevel},
		{name: "gemini 3 pro uses level", model: "models/gemini-3.1-pro", want: GeminiThinkingLevel},
		{name: "case insensitive", model: "GEMINI-3.8-FLASH-HIGH", want: GeminiThinkingLevel},
		{name: "gemini 2.5 flash uses budget", model: "gemini-2.5-flash", want: GeminiThinkingBudget},
		{name: "gemini 2.5 flash thinking alias uses budget", model: "gemini-2.5-flash-thinking", want: GeminiThinkingBudget},
		{name: "gemini 2.5 pro uses budget", model: "gemini-2.5-pro-preview", want: GeminiThinkingBudget},
		{name: "image models are excluded", model: "gemini-3.8-flash-image-preview", want: GeminiThinkingUnsupported},
		{name: "audio models are excluded", model: "gemini-3.1-pro-audio", want: GeminiThinkingUnsupported},
		{name: "live models are excluded", model: "gemini-3.1-flash-live-preview", want: GeminiThinkingUnsupported},
		{name: "embedding models are excluded", model: "gemini-3.1-embedding", want: GeminiThinkingUnsupported},
		{name: "flash lite is not the supported flash family", model: "gemini-2.5-flash-lite", want: GeminiThinkingUnsupported},
		{name: "gemini 2.0 remains unchanged", model: "gemini-2.0-flash", want: GeminiThinkingUnsupported},
		{name: "unknown future generation remains unchanged", model: "gemini-4-flash", want: GeminiThinkingUnsupported},
		{name: "non gemini remains unchanged", model: "claude-sonnet-4-6", want: GeminiThinkingUnsupported},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ResolveGeminiThinkingMode(tt.model))
		})
	}
}

func TestGeminiThinkingSettingsForEffort(t *testing.T) {
	tests := []struct {
		name         string
		model        string
		effort       string
		wantOK       bool
		wantLevel    string
		wantBudget   int
		wantThoughts bool
	}{
		{name: "gemini 3 low", model: "gemini-3.8-flash", effort: "low", wantOK: true, wantLevel: "LOW", wantThoughts: true},
		{name: "gemini 3 medium", model: "gemini-3.8-flash", effort: "medium", wantOK: true, wantLevel: "MEDIUM", wantThoughts: true},
		{name: "gemini 3 high", model: "gemini-3.8-flash", effort: "high", wantOK: true, wantLevel: "HIGH", wantThoughts: true},
		{name: "gemini 3 xhigh caps at high level", model: "gemini-3.8-flash", effort: "xhigh", wantOK: true, wantLevel: "HIGH", wantThoughts: true},
		{name: "gemini 3 max caps at high level", model: "gemini-3.8-flash", effort: "max", wantOK: true, wantLevel: "HIGH", wantThoughts: true},
		{name: "gemini 3 omitted effort defaults low", model: "gemini-3.8-flash", effort: "", wantOK: true, wantLevel: "LOW", wantThoughts: true},
		{name: "gemini 2.5 flash high", model: "gemini-2.5-flash", effort: "high", wantOK: true, wantBudget: 10240, wantThoughts: true},
		{name: "gemini 2.5 flash xhigh is capped", model: "gemini-2.5-flash", effort: "xhigh", wantOK: true, wantBudget: Gemini25FlashThinkingBudgetLimit, wantThoughts: true},
		{name: "gemini 2.5 omitted effort defaults low budget", model: "gemini-2.5-flash", effort: "", wantOK: true, wantBudget: 1024, wantThoughts: true},
		{name: "gemini 2.5 pro max", model: "gemini-2.5-pro", effort: "max", wantOK: true, wantBudget: 32768, wantThoughts: true},
		{name: "unknown effort is ignored", model: "gemini-3.8-flash", effort: "ultra", wantOK: false},
		{name: "unsupported model is ignored", model: "gemini-2.0-flash", effort: "high", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := GeminiThinkingSettingsForEffort(tt.model, tt.effort)
			require.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				return
			}
			require.Equal(t, tt.wantLevel, got.ThinkingLevel)
			require.Equal(t, tt.wantBudget, got.ThinkingBudget)
			require.Equal(t, tt.wantThoughts, got.IncludeThoughts)
		})
	}
}

func TestGeminiThinkingConfigSerialization(t *testing.T) {
	level, err := json.Marshal(GeminiThinkingConfig{IncludeThoughts: true, ThinkingLevel: "HIGH"})
	require.NoError(t, err)
	require.JSONEq(t, `{"includeThoughts":true,"thinkingLevel":"HIGH"}`, string(level))

	budget, err := json.Marshal(GeminiThinkingConfig{IncludeThoughts: true, ThinkingBudget: 1024})
	require.NoError(t, err)
	require.JSONEq(t, `{"includeThoughts":true,"thinkingBudget":1024}`, string(budget))
}

func TestApplyGeminiThinkingConfigPreservesGenerationConfig(t *testing.T) {
	body := []byte(`{"contents":[],"generationConfig":{"maxOutputTokens":4096,"temperature":0.2,"thinkingConfig":{"providerFlag":"keep","thinkingBudget":777}}}`)
	effort := "high"

	got, err := ApplyGeminiThinkingConfig(body, "gemini-3.8-flash-tiered", &effort)
	require.NoError(t, err)
	require.Equal(t, "HIGH", jsonField(t, got, "generationConfig.thinkingConfig.thinkingLevel"))
	require.False(t, jsonFieldExists(got, "generationConfig.thinkingConfig.thinkingBudget"))
	require.Equal(t, "keep", jsonField(t, got, "generationConfig.thinkingConfig.providerFlag"))
	require.Equal(t, int64(4096), jsonIntField(t, got, "generationConfig.maxOutputTokens"))
	require.Equal(t, float64(0.2), jsonFloatField(t, got, "generationConfig.temperature"))
}

func TestApplyGeminiThinkingConfigUsesBudgetAndFlashLimit(t *testing.T) {
	body := []byte(`{"contents":[],"generationConfig":{"thinkingConfig":{"thinkingLevel":"LOW"}}}`)
	effort := "xhigh"

	got, err := ApplyGeminiThinkingConfig(body, "gemini-2.5-flash", &effort)
	require.NoError(t, err)
	require.Equal(t, int64(Gemini25FlashThinkingBudgetLimit), jsonIntField(t, got, "generationConfig.thinkingConfig.thinkingBudget"))
	require.False(t, jsonFieldExists(got, "generationConfig.thinkingConfig.thinkingLevel"))
	require.True(t, jsonBoolField(t, got, "generationConfig.thinkingConfig.includeThoughts"))
}

func TestApplyGeminiThinkingConfigCreatesMissingGenerationConfig(t *testing.T) {
	effort := "medium"

	got, err := ApplyGeminiThinkingConfig([]byte(`{"contents":[]}`), "gemini-3-flash", &effort)
	require.NoError(t, err)
	require.Equal(t, "MEDIUM", jsonField(t, got, "generationConfig.thinkingConfig.thinkingLevel"))
	require.True(t, jsonBoolField(t, got, "generationConfig.thinkingConfig.includeThoughts"))
}

func TestApplyGeminiThinkingConfigRejectsMalformedJSON(t *testing.T) {
	effort := "high"

	_, err := ApplyGeminiThinkingConfig([]byte(`{"contents":`), "gemini-3-flash", &effort)
	require.Error(t, err)
}

func TestApplyGeminiThinkingConfigLeavesUnsupportedRequestsByteStable(t *testing.T) {
	body := []byte(`{"contents":[],"generationConfig":{"maxOutputTokens":4096}}`)

	got, err := ApplyGeminiThinkingConfig(body, "gemini-2.0-flash", nil)
	require.NoError(t, err)
	require.Equal(t, string(body), string(got))

	effort := "ultra"
	got, err = ApplyGeminiThinkingConfig(body, "gemini-3.8-flash", &effort)
	require.NoError(t, err)
	require.Equal(t, string(body), string(got))
}

func TestApplyGeminiThinkingConfigDefaultsToLowestEffort(t *testing.T) {
	body := []byte(`{"contents":[]}`)

	got, err := ApplyGeminiThinkingConfig(body, "gemini-3.8-flash-low", nil)
	require.NoError(t, err)
	require.Equal(t, "LOW", jsonField(t, got, "generationConfig.thinkingConfig.thinkingLevel"))
	require.True(t, jsonBoolField(t, got, "generationConfig.thinkingConfig.includeThoughts"))

	got, err = ApplyGeminiThinkingConfig(body, "gemini-2.5-flash", nil)
	require.NoError(t, err)
	require.Equal(t, int64(1024), jsonIntField(t, got, "generationConfig.thinkingConfig.thinkingBudget"))
	require.True(t, jsonBoolField(t, got, "generationConfig.thinkingConfig.includeThoughts"))
}

func jsonField(t *testing.T, body []byte, path string) string {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	value := any(payload)
	for _, key := range stringsSplitPath(path) {
		object, ok := value.(map[string]any)
		require.True(t, ok, "missing object while reading %s", path)
		value = object[key]
	}
	result, ok := value.(string)
	require.True(t, ok, "expected string at %s", path)
	return result
}

func jsonFieldExists(body []byte, path string) bool {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return false
	}
	value := any(payload)
	for _, key := range stringsSplitPath(path) {
		object, ok := value.(map[string]any)
		if !ok {
			return false
		}
		value, ok = object[key]
		if !ok {
			return false
		}
	}
	return true
}

func jsonIntField(t *testing.T, body []byte, path string) int64 {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	value := jsonValueAtPath(t, payload, path)
	result, ok := value.(float64)
	require.True(t, ok, "expected number at %s", path)
	return int64(result)
}

func jsonFloatField(t *testing.T, body []byte, path string) float64 {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	value := jsonValueAtPath(t, payload, path)
	result, ok := value.(float64)
	require.True(t, ok, "expected number at %s", path)
	return result
}

func jsonBoolField(t *testing.T, body []byte, path string) bool {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	value := jsonValueAtPath(t, payload, path)
	result, ok := value.(bool)
	require.True(t, ok, "expected bool at %s", path)
	return result
}

func jsonValueAtPath(t *testing.T, payload map[string]any, path string) any {
	t.Helper()
	value := any(payload)
	for _, key := range stringsSplitPath(path) {
		object, ok := value.(map[string]any)
		require.True(t, ok, "missing object while reading %s", path)
		value = object[key]
	}
	return value
}

func stringsSplitPath(path string) []string {
	return strings.Split(path, ".")
}

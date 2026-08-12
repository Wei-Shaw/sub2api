package securityaudit

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEventMetadataRoundTrip(t *testing.T) {
	result := &NormalizedResult{
		EngineType: EngineGenericLLM, ScannerVersion: "audit-model", SchemaVersion: GenericSchemaVersion,
		Confidence: 0.91, Stage: "warn", FailurePolicy: "fail_open",
		PromptTokens: 21, CompletionTokens: 8, TotalTokens: 29,
		UnknownCategories: []string{"unknown:future-risk"},
	}
	encoded, err := json.Marshal(eventMetadataFromResult(result))
	require.NoError(t, err)

	var decoded eventMetadata
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	event := &Event{}
	decoded.apply(event)
	require.Equal(t, EngineGenericLLM, event.EngineType)
	require.Equal(t, "audit-model", event.AuditModel)
	require.Equal(t, GenericSchemaVersion, event.SchemaVersion)
	require.InDelta(t, 0.91, event.Confidence, 0.001)
	require.Equal(t, "warn", event.EnforcementStage)
	require.Equal(t, "fail_open", event.FailurePolicy)
	require.Equal(t, 21, event.PromptTokens)
	require.Equal(t, 8, event.CompletionTokens)
	require.Equal(t, 29, event.TotalTokens)
	require.Equal(t, []string{"unknown:future-risk"}, event.UnknownCategories)
}

func TestEmptyEventMetadataIsBackwardCompatible(t *testing.T) {
	var metadata eventMetadata
	require.NoError(t, json.Unmarshal([]byte(`{}`), &metadata))
	event := &Event{ScannerBackend: "qwen3guard-openai", ScannerVersion: "legacy-model", PolicyVersion: 1}
	metadata.apply(event)
	require.Equal(t, "qwen3guard-openai", event.ScannerBackend)
	require.Equal(t, "legacy-model", event.ScannerVersion)
	require.Equal(t, 1, event.PolicyVersion)
	require.Empty(t, event.EngineType)
	require.Zero(t, event.TotalTokens)
}

func TestShadowComparisonMetadataContainsNoPrompt(t *testing.T) {
	metadata := eventMetadata{ShadowComparison: &ShadowComparison{CompositionMode: "combined", KeywordDecision: "allow", LLMDecision: DecisionBlock, Agreement: false}}
	encoded, err := json.Marshal(metadata)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "prompt")
	var decoded eventMetadata
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	event := &Event{}
	decoded.apply(event)
	require.Equal(t, metadata.ShadowComparison, event.ShadowComparison)
}

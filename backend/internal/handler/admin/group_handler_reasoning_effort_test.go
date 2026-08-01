package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateGroupRequestReasoningEffortMappingsTriState(t *testing.T) {
	t.Run("omitted means unchanged", func(t *testing.T) {
		var req UpdateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{}`), &req))
		require.Nil(t, req.ReasoningEffortMappings)
	})

	t.Run("empty array means clear", func(t *testing.T) {
		var req UpdateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{"reasoning_effort_mappings":[]}`), &req))
		require.NotNil(t, req.ReasoningEffortMappings)
		require.Empty(t, *req.ReasoningEffortMappings)
	})

	t.Run("non empty array means replace", func(t *testing.T) {
		var req UpdateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{"reasoning_effort_mappings":[{"from":"max","to":"xhigh"}]}`), &req))
		require.NotNil(t, req.ReasoningEffortMappings)
		require.Len(t, *req.ReasoningEffortMappings, 1)
		require.Equal(t, "max", (*req.ReasoningEffortMappings)[0].From)
		require.Equal(t, "xhigh", (*req.ReasoningEffortMappings)[0].To)
	})
}

func TestUpdateGroupRequestModelReasoningEffortRulesTriState(t *testing.T) {
	t.Run("omitted means unchanged", func(t *testing.T) {
		var req UpdateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{}`), &req))
		require.Nil(t, req.ModelReasoningEffortRules)
	})

	t.Run("empty array means clear", func(t *testing.T) {
		var req UpdateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{"model_reasoning_effort_rules":[]}`), &req))
		require.NotNil(t, req.ModelReasoningEffortRules)
		require.Empty(t, *req.ModelReasoningEffortRules)
	})

	t.Run("non empty array preserves explicit unlimited", func(t *testing.T) {
		var req UpdateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{"model_reasoning_effort_rules":[{"model":"gpt-5.6-luna","max_reasoning_effort":"","reasoning_effort_mappings":[]}]}`), &req))
		require.NotNil(t, req.ModelReasoningEffortRules)
		require.Len(t, *req.ModelReasoningEffortRules, 1)
		require.Equal(t, "gpt-5.6-luna", (*req.ModelReasoningEffortRules)[0].Model)
		require.Empty(t, (*req.ModelReasoningEffortRules)[0].MaxReasoningEffort)
		require.Empty(t, (*req.ModelReasoningEffortRules)[0].ReasoningEffortMappings)
	})
}

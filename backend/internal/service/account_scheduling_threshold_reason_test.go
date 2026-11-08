//go:build unit

package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildAccountSchedulingThresholdReason_UsesSourceAndFallbackMessage(t *testing.T) {
	raw := BuildAccountSchedulingThresholdReason(" \t ")

	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	require.Equal(t, AccountSchedulingThresholdReasonSource, payload["source"])
	require.Equal(t, defaultAccountSchedulingThresholdErrorMessage, payload["error_message"])
	require.True(t, IsAccountSchedulingThresholdReason(raw))
}

func TestIsAccountSchedulingThresholdReason(t *testing.T) {
	require.True(t, IsAccountSchedulingThresholdReason(BuildAccountSchedulingThresholdReason("threshold reached")))
	require.False(t, IsAccountSchedulingThresholdReason(BuildTempUnschedReasonPayload("", "temporary block")))
	require.False(t, IsAccountSchedulingThresholdReason("plain text reason"))
}

func TestTempUnschedStateFromStoredReason_EmptyReasonUsesFallbackErrorMessage(t *testing.T) {
	state := tempUnschedStateFromStoredReason(" \n ", 1735689600)

	require.NotNil(t, state)
	require.Equal(t, int64(1735689600), state.UntilUnix)
	require.Equal(t, defaultTempUnschedReasonErrorMessage, state.ErrorMessage)
}

func TestTempUnschedStateFromStoredReason_MissingRuleIndexIsSystemRule(t *testing.T) {
	state := tempUnschedStateFromStoredReason(`{"error_message":"system cooldown"}`, 123)
	require.Equal(t, -1, state.RuleIndex)
}

func TestTempUnschedStateFromStoredReason_SchedulingThresholdJSONWithoutMessageUsesThresholdFallback(t *testing.T) {
	raw := `{"source":"` + AccountSchedulingThresholdReasonSource + `"}`

	state := tempUnschedStateFromStoredReason(raw, 1735689600)

	require.NotNil(t, state)
	require.Equal(t, int64(1735689600), state.UntilUnix)
	require.Equal(t, defaultAccountSchedulingThresholdErrorMessage, state.ErrorMessage)
}

func TestBuildDetailedAccountSchedulingThresholdReason_IncludesReadableFields(t *testing.T) {
	now := time.Unix(1735689600, 0).UTC()
	until := now.Add(5 * time.Hour)

	raw := BuildDetailedAccountSchedulingThresholdReason(AccountSchedulingThresholdReasonInput{
		Platform:         PlatformOpenAI,
		Window:           "7d",
		ThresholdPercent: 90,
		UsedPercent:      92.5,
		Until:            until,
		Now:              now,
	})

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	require.Equal(t, AccountSchedulingThresholdReasonSource, payload["source"])
	require.Equal(t, PlatformOpenAI, payload["platform"])
	require.Equal(t, "7d", payload["window"])
	require.Equal(t, float64(90), payload["threshold_percent"])
	require.Equal(t, float64(92.5), payload["used_percent"])
	require.Equal(t, float64(until.Unix()), payload["until_unix"])
	require.Equal(t, float64(now.Unix()), payload["triggered_at_unix"])
	require.Contains(t, payload["error_message"], "openai scheduling threshold reached")
	require.Contains(t, payload["error_message"], "92.5% used >= 90%")
}

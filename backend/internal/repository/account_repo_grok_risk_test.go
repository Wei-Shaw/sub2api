package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGrokRiskSnapshotIsSchedulerNeutral(t *testing.T) {
	t.Parallel()

	require.True(t, isSchedulerNeutralExtraKey("grok_risk"))
	require.False(t, shouldEnqueueSchedulerOutboxForExtraUpdates(map[string]any{
		"grok_risk": map[string]any{"verdict": "clean"},
	}))
	require.True(t, shouldEnqueueSchedulerOutboxForExtraUpdates(map[string]any{
		"grok_risk": map[string]any{"verdict": "clean"},
		"status":    "active",
	}))
}

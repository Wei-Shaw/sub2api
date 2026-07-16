package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpstreamBillingProbeExtraIsSchedulerNeutral(t *testing.T) {
	require.True(t, isSchedulerNeutralExtraKey("upstream_billing_probe"))
	require.True(t, isSchedulerNeutralExtraKey("upstream_billing_probe_enabled"))
	require.False(t, shouldEnqueueSchedulerOutboxForExtraUpdates(map[string]any{
		"upstream_billing_probe":         map[string]any{"status": "ok"},
		"upstream_billing_probe_enabled": true,
	}))
}

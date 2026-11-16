//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInflightEstimateQuotaPolicyIncludesPricedTargetAndAccountMapping(t *testing.T) {
	groupID := int64(71)
	originalPrice, targetPrice, upstreamPrice := 1.0, 2.0, 3.0
	key := &APIKey{User: &User{ID: 1}, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformOpenAI, RateMultiplier: 1, ModelPricing: []ChannelModelPricing{
		{Models: []string{"public"}, BillingMode: BillingModePerRequest, PerRequestPrice: &originalPrice},
		{Models: []string{"policy-target"}, BillingMode: BillingModePerRequest, PerRequestPrice: &targetPrice},
		{Models: []string{"actual-upstream"}, BillingMode: BillingModePerRequest, PerRequestPrice: &upstreamPrice},
	}}}
	svc := newInflightEstimateGateway(t, nil)
	d := svc.inflightEstimateDeps()
	calls := 0
	d.accountMappedModels = func(_ context.Context, _ *APIKey, model string) []string {
		calls++
		require.Equal(t, "policy-target", model, "apply static account mapping to the frozen target")
		return []string{"actual-upstream"}
	}
	req := InflightEstimateRequest{Model: "public", BodyBytes: 100}
	baseline, ok := d.estimate(context.Background(), key, req)
	require.True(t, ok)
	require.Equal(t, 1.0, baseline)
	require.Zero(t, calls, "disabled policy must preserve the existing fast path")
	req.AdditionalUpstreamModel = "policy-target"
	reserved, ok := d.estimate(context.Background(), key, req)
	require.True(t, ok)
	require.Equal(t, 3.0, reserved)
	require.Equal(t, 1, calls)
	require.Equal(t, "public", req.Model, "original billing model remains intact")
	d.accountMappedModels = nil
	reserved, ok = d.estimate(context.Background(), key, req)
	require.True(t, ok)
	require.Equal(t, 2.0, reserved)
	req.AdditionalUpstreamModel = "unpriced-target"
	reserved, ok = d.estimate(context.Background(), key, req)
	require.True(t, ok)
	require.Equal(t, baseline, reserved, "additional estimates must never reduce the reservation")
}

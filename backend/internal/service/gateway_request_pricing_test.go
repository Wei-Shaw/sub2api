//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithGatewayTokenRequestPricingMarksOnlyExplicitTokenRequests(t *testing.T) {
	ctx, pricingAt := WithGatewayTokenRequestPricing(context.Background())

	got, ok := gatewayTokenRequestPricingAtFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, pricingAt, got)
	require.Equal(t, pricingAt, GatewayTokenRequestPricingAtFromContext(ctx))
	require.True(t, GatewayTokenRequestPricingAtFromContext(context.Background()).IsZero())
}

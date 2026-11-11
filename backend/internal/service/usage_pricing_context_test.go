package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// 后扣在 worker background ctx 上运行：组合分组必须按 handler 算定的实际平台
// （QuotaPlatform）查渠道定价，而不是按组合分组的回退顺序取到其他平台的价格。
func TestUsagePricingContextScopesCompositeGroupToActualPlatform(t *testing.T) {
	composite := &APIKey{Group: &Group{ID: 1, Platform: PlatformComposite}}

	ctx := usagePricingContext(context.Background(), composite, PlatformKimi)
	require.Equal(t, PlatformKimi, channelLookupPlatform(ctx, PlatformComposite))

	// 未算定具体平台：保持组合分组回退语义。
	ctx = usagePricingContext(context.Background(), composite, PlatformComposite)
	require.Equal(t, PlatformComposite, channelLookupPlatform(ctx, PlatformComposite))
	ctx = usagePricingContext(context.Background(), composite, "")
	require.Equal(t, PlatformComposite, channelLookupPlatform(ctx, PlatformComposite))

	// 请求期已解析的目标平台不被覆盖。
	resolved := WithResolvedTargetPlatform(context.Background(), PlatformOpenAI)
	ctx = usagePricingContext(resolved, composite, PlatformKimi)
	require.Equal(t, PlatformOpenAI, channelLookupPlatform(ctx, PlatformComposite))

	// 非组合分组不受影响。
	concrete := &APIKey{Group: &Group{ID: 2, Platform: PlatformKimi}}
	base := context.Background()
	require.Equal(t, base, usagePricingContext(base, concrete, PlatformOpenAI))
	require.Equal(t, base, usagePricingContext(base, &APIKey{}, PlatformOpenAI))
}

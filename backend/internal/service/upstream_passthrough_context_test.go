package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSetUpstreamPolicyInContext_RoundTrip(t *testing.T) {
	policy := EffectiveUpstreamPolicy{
		Category:               CategoryRelay,
		ProfileApplied:         ProfileTransparent,
		ForwardClientHeaders:   true,
		ForwardUserNetworkInfo: true,
		AccountConcurrency:     7,
	}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)

	got, ok := GetUpstreamPolicyFromContext(ctx)
	require.True(t, ok)
	require.NotNil(t, got)
	require.Equal(t, CategoryRelay, got.Category)
	require.Equal(t, ProfileTransparent, got.ProfileApplied)
	require.True(t, got.ForwardClientHeaders)
	require.Equal(t, 7, got.AccountConcurrency)
}

func TestGetUpstreamPolicyFromContext_AbsentReturnsFalse(t *testing.T) {
	got, ok := GetUpstreamPolicyFromContext(context.Background())
	require.False(t, ok)
	require.Nil(t, got)
}

func TestSetUpstreamPolicyInContext_NilPolicyStillSetsKey(t *testing.T) {
	// Setting nil should not register the key — Get must return (nil, false).
	ctx := SetUpstreamPolicyInContext(context.Background(), nil)
	got, ok := GetUpstreamPolicyFromContext(ctx)
	require.False(t, ok, "nil policy should be treated as 'not set' by Get")
	require.Nil(t, got)
}

func TestRequireUpstreamPolicyFromContext_FallbackOnAbsent(t *testing.T) {
	// Returns a safe Protected fallback when policy is absent
	got := RequireUpstreamPolicyFromContext(context.Background())
	require.Equal(t, ProfileProtected, got.ProfileApplied)
	require.Equal(t, CategoryOfficial, got.Category)
}

func TestRequireUpstreamPolicyFromContext_ReturnsActualWhenPresent(t *testing.T) {
	policy := EffectiveUpstreamPolicy{Category: CategoryReverse, ProfileApplied: ProfileStrict}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	got := RequireUpstreamPolicyFromContext(ctx)
	require.Equal(t, CategoryReverse, got.Category)
	require.Equal(t, ProfileStrict, got.ProfileApplied)
}

func TestResolveAndStorePolicy_FeatureFlagOffReturnsOriginalCtx(t *testing.T) {
	repo := &fakeSettingRepoForUpstream{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	// FeatureFlag is absent → default false
	a := &Account{Type: AccountTypeAPIKey, Platform: PlatformAnthropic}

	origCtx := context.Background()
	got := ResolveAndStorePolicy(origCtx, a, svc)

	// ctx is returned as-is when FeatureFlag is OFF; no policy attached
	_, ok := GetUpstreamPolicyFromContext(got)
	require.False(t, ok)
}

func TestResolveAndStorePolicy_FeatureFlagOnAttachesPolicy(t *testing.T) {
	resetUpstreamPassthroughDefaultsCache()
	resetUpstreamPassthroughGlobalOverrideCache()

	repo := &fakeSettingRepoForUpstream{values: map[string]string{
		SettingKeyUpstreamPolicyV1Enabled: "true",
	}}
	svc := NewSettingService(repo, &config.Config{})
	a := &Account{Type: AccountTypeOAuth, Platform: PlatformKiro}

	got := ResolveAndStorePolicy(context.Background(), a, svc)

	policy, ok := GetUpstreamPolicyFromContext(got)
	require.True(t, ok)
	require.NotNil(t, policy)
	require.Equal(t, CategoryReverse, policy.Category) // Kiro → reverse
	require.Equal(t, ProfileStrict, policy.ProfileApplied)
}

func TestResolveAndStorePolicy_NilAccountReturnsOriginalCtx(t *testing.T) {
	repo := &fakeSettingRepoForUpstream{values: map[string]string{
		SettingKeyUpstreamPolicyV1Enabled: "true",
	}}
	svc := NewSettingService(repo, &config.Config{})

	origCtx := context.Background()
	got := ResolveAndStorePolicy(origCtx, nil, svc)

	// Defensive: nil account, even with FeatureFlag on, returns ctx unchanged
	_, ok := GetUpstreamPolicyFromContext(got)
	require.False(t, ok)
}

func TestResolveAndStorePolicy_NilSettingServiceReturnsOriginalCtx(t *testing.T) {
	origCtx := context.Background()
	got := ResolveAndStorePolicy(origCtx, &Account{Type: AccountTypeAPIKey, Platform: PlatformAnthropic}, nil)

	// Defensive: nil settingService → no resolution possible, ctx unchanged
	_, ok := GetUpstreamPolicyFromContext(got)
	require.False(t, ok)
}

func TestShouldScrubBody_LegacyWhenAbsent(t *testing.T) {
	// No policy in ctx → return true (do the legacy scrub)
	require.True(t, ShouldScrubBody(context.Background()))
}

func TestShouldScrubBody_PolicyAbsentSkipMatchesLegacy(t *testing.T) {
	// Policy in ctx with SkipBodyScrub=false → do scrub (matches Protected/Strict)
	policy := EffectiveUpstreamPolicy{SkipBodyScrub: false}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	require.True(t, ShouldScrubBody(ctx))
}

func TestShouldScrubBody_PolicyWithSkipReturnsFalse(t *testing.T) {
	// Policy in ctx with SkipBodyScrub=true → skip scrub (matches Transparent)
	policy := EffectiveUpstreamPolicy{SkipBodyScrub: true}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	require.False(t, ShouldScrubBody(ctx))
}

func TestShouldInjectSystemPrompt_LegacyWhenAbsent(t *testing.T) {
	// No policy in ctx → return true (run the legacy maybeInjectClaudeCodeSystemBlocks,
	// which still consults cfg.Gateway.InjectCCSystemBlocks internally)
	require.True(t, ShouldInjectSystemPrompt(context.Background()))
}

func TestShouldInjectSystemPrompt_ProtectedSkipsInject(t *testing.T) {
	// Policy in ctx with SkipSystemPromptInject=true (Protected default) → don't inject
	policy := EffectiveUpstreamPolicy{SkipSystemPromptInject: true}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	require.False(t, ShouldInjectSystemPrompt(ctx))
}

func TestShouldInjectSystemPrompt_StrictRunsInject(t *testing.T) {
	// Policy in ctx with SkipSystemPromptInject=false (Strict) → run the legacy injector,
	// which will inject if cfg.Gateway.InjectCCSystemBlocks is on
	policy := EffectiveUpstreamPolicy{SkipSystemPromptInject: false}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	require.True(t, ShouldInjectSystemPrompt(ctx))
}

func TestShouldForwardClientUA_LegacyWhenAbsent(t *testing.T) {
	require.False(t, ShouldForwardClientUA(context.Background()))
}

func TestShouldForwardClientUA_PolicyWithForwardFalseMatchesLegacy(t *testing.T) {
	policy := EffectiveUpstreamPolicy{ForwardClientUA: false}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	require.False(t, ShouldForwardClientUA(ctx))
}

func TestShouldForwardClientUA_PolicyWithForwardTrueReturnsTrue(t *testing.T) {
	policy := EffectiveUpstreamPolicy{ForwardClientUA: true}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	require.True(t, ShouldForwardClientUA(ctx))
}

func TestShouldForwardUserNetworkInfo_LegacyWhenAbsent(t *testing.T) {
	// No policy in ctx → return false (don't forward; preserves today's behavior)
	require.False(t, ShouldForwardUserNetworkInfo(context.Background()))
}

func TestShouldForwardUserNetworkInfo_PolicyAbsentForwardMatchesLegacy(t *testing.T) {
	// Policy with ForwardUserNetworkInfo=false → don't forward (matches Protected/Strict)
	policy := EffectiveUpstreamPolicy{ForwardUserNetworkInfo: false}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	require.False(t, ShouldForwardUserNetworkInfo(ctx))
}

func TestShouldForwardUserNetworkInfo_PolicyWithForwardReturnsTrue(t *testing.T) {
	// Policy with ForwardUserNetworkInfo=true → forward (matches Transparent)
	policy := EffectiveUpstreamPolicy{ForwardUserNetworkInfo: true}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	require.True(t, ShouldForwardUserNetworkInfo(ctx))
}

func TestForwardUserNetworkInfoHeaders_NoOpWhenPolicyOff(t *testing.T) {
	clientHeaders := http.Header{
		"X-Forwarded-For": []string{"1.2.3.4"},
		"X-Real-Ip":       []string{"1.2.3.4"}, // Go canonical form
	}
	upstreamHeaders := http.Header{}

	// No policy in ctx → no-op
	ForwardUserNetworkInfoHeaders(context.Background(), upstreamHeaders, clientHeaders)
	require.Empty(t, upstreamHeaders, "no policy → must not copy anything")
}

func TestForwardUserNetworkInfoHeaders_CopiesWhenPolicyOn(t *testing.T) {
	clientHeaders := http.Header{
		"X-Forwarded-For":  []string{"1.2.3.4, 5.6.7.8"},
		"X-Real-Ip":        []string{"1.2.3.4"},
		"Forwarded":        []string{"for=1.2.3.4"},
		"Cf-Connecting-Ip": []string{"1.2.3.4"},
		"True-Client-Ip":   []string{"1.2.3.4"},
		"Other-Header":     []string{"should not be copied"},
	}
	upstreamHeaders := http.Header{}

	policy := EffectiveUpstreamPolicy{ForwardUserNetworkInfo: true}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	ForwardUserNetworkInfoHeaders(ctx, upstreamHeaders, clientHeaders)

	require.Equal(t, "1.2.3.4, 5.6.7.8", upstreamHeaders.Get("X-Forwarded-For"))
	require.Equal(t, "1.2.3.4", upstreamHeaders.Get("X-Real-IP"))
	require.Equal(t, "for=1.2.3.4", upstreamHeaders.Get("Forwarded"))
	require.Equal(t, "1.2.3.4", upstreamHeaders.Get("CF-Connecting-IP"))
	require.Equal(t, "1.2.3.4", upstreamHeaders.Get("True-Client-IP"))
	require.Empty(t, upstreamHeaders.Get("Other-Header"))
}

func TestForwardUserNetworkInfoHeaders_AbsentClientHeadersAreSkipped(t *testing.T) {
	clientHeaders := http.Header{
		"X-Forwarded-For": []string{"1.2.3.4"},
		// other 4 absent
	}
	upstreamHeaders := http.Header{}

	policy := EffectiveUpstreamPolicy{ForwardUserNetworkInfo: true}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	ForwardUserNetworkInfoHeaders(ctx, upstreamHeaders, clientHeaders)

	require.Equal(t, "1.2.3.4", upstreamHeaders.Get("X-Forwarded-For"))
	// Absent headers should not appear in upstream headers
	require.Empty(t, upstreamHeaders.Get("X-Real-IP"))
	require.Empty(t, upstreamHeaders.Get("Forwarded"))
}

func TestForwardUserNetworkInfoHeaders_NilClientHeadersSafe(t *testing.T) {
	upstreamHeaders := http.Header{}
	policy := EffectiveUpstreamPolicy{ForwardUserNetworkInfo: true}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	ForwardUserNetworkInfoHeaders(ctx, upstreamHeaders, nil) // must not panic
	require.Empty(t, upstreamHeaders)
}

func TestForwardUserNetworkInfoHeaders_NilUpstreamHeadersSafe(t *testing.T) {
	clientHeaders := http.Header{"X-Forwarded-For": []string{"1.2.3.4"}}
	policy := EffectiveUpstreamPolicy{ForwardUserNetworkInfo: true}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	ForwardUserNetworkInfoHeaders(ctx, nil, clientHeaders) // must not panic
}

package service

import (
	"context"
	"net/http"
	"strings"
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

func TestIsClientHeaderBlacklistedForForward_BlacklistEntries(t *testing.T) {
	for _, key := range []string{"authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "x-goog-api-key"} {
		require.True(t, IsClientHeaderBlacklistedForForward(key), "expected %q to be blacklisted", key)
	}
}

func TestIsClientHeaderBlacklistedForForward_NonBlacklistedKeysPass(t *testing.T) {
	for _, key := range []string{"user-agent", "x-app", "anthropic-beta", "accept-language", "x-custom"} {
		require.False(t, IsClientHeaderBlacklistedForForward(key), "expected %q to NOT be blacklisted", key)
	}
}

func TestShouldForwardClientHeaders_LegacyWhenAbsent(t *testing.T) {
	require.False(t, ShouldForwardClientHeaders(context.Background()))
}

func TestShouldForwardClientHeaders_PolicyWithForwardFalseMatchesLegacy(t *testing.T) {
	policy := EffectiveUpstreamPolicy{ForwardClientHeaders: false}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	require.False(t, ShouldForwardClientHeaders(ctx))
}

func TestShouldForwardClientHeaders_PolicyWithForwardTrueReturnsTrue(t *testing.T) {
	policy := EffectiveUpstreamPolicy{ForwardClientHeaders: true}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	require.True(t, ShouldForwardClientHeaders(ctx))
}

func TestShouldCopyClientHeader_LegacyDelegatesToWhitelist(t *testing.T) {
	whitelist := func(k string) bool { return k == "user-agent" }
	ctx := context.Background()

	require.True(t, ShouldCopyClientHeader(ctx, "user-agent", whitelist))
	require.False(t, ShouldCopyClientHeader(ctx, "x-custom", whitelist))
	// Even auth headers fall back to whitelist when policy absent — preserves
	// legacy behavior at every call site that already strips auth post-loop.
	require.False(t, ShouldCopyClientHeader(ctx, "authorization", whitelist))
}

func TestShouldCopyClientHeader_PolicyTrueIgnoresWhitelistAndUsesBlacklist(t *testing.T) {
	whitelist := func(k string) bool { return false } // legacy would block everything
	policy := EffectiveUpstreamPolicy{ForwardClientHeaders: true}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)

	// User-Agent is not whitelisted by the (hostile) closure but blacklist mode passes it.
	require.True(t, ShouldCopyClientHeader(ctx, "user-agent", whitelist))
	require.True(t, ShouldCopyClientHeader(ctx, "x-custom-header", whitelist))
	// Blacklisted keys are stripped even in forward mode.
	require.False(t, ShouldCopyClientHeader(ctx, "authorization", whitelist))
	require.False(t, ShouldCopyClientHeader(ctx, "cookie", whitelist))
	require.False(t, ShouldCopyClientHeader(ctx, "x-api-key", whitelist))
}

func TestShouldCopyClientHeader_PolicyFalseUsesWhitelist(t *testing.T) {
	whitelist := func(k string) bool { return k == "user-agent" }
	policy := EffectiveUpstreamPolicy{ForwardClientHeaders: false}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)

	require.True(t, ShouldCopyClientHeader(ctx, "user-agent", whitelist))
	require.False(t, ShouldCopyClientHeader(ctx, "x-custom", whitelist))
}

func TestShouldCopyClientHeader_NilWhitelistInLegacyReturnsFalse(t *testing.T) {
	// Defensive: a buggy call site that passes nil whitelist must not panic
	// and must not silently forward everything.
	ctx := context.Background()
	require.False(t, ShouldCopyClientHeader(ctx, "user-agent", nil))
}

// Reproduces the loop pattern at each B-2d call site to lock in the contract
// at the use-site level: legacy whitelist mode passes only known headers;
// forward mode passes everything except the blacklist.
func TestShouldCopyClientHeader_LoopPattern_LegacyVsForward(t *testing.T) {
	clientHeaders := http.Header{
		"User-Agent":    []string{"my-client/1.0"},   // whitelisted
		"X-App":         []string{"app-x"},           // whitelisted
		"X-Custom":      []string{"custom-value"},    // NOT whitelisted
		"Cookie":        []string{"session=secret"},  // blacklisted
		"Authorization": []string{"Bearer secret"},   // blacklisted
		"X-Api-Key":     []string{"client-api-key"},  // blacklisted
	}
	miniWhitelist := func(k string) bool {
		return k == "user-agent" || k == "x-app"
	}

	copyUnderPolicy := func(ctx context.Context) http.Header {
		out := http.Header{}
		for key, values := range clientHeaders {
			lower := strings.ToLower(key)
			if !ShouldCopyClientHeader(ctx, lower, miniWhitelist) {
				continue
			}
			for _, v := range values {
				out.Add(key, v)
			}
		}
		return out
	}

	t.Run("legacy: only whitelisted headers pass", func(t *testing.T) {
		out := copyUnderPolicy(context.Background())
		require.Equal(t, "my-client/1.0", out.Get("User-Agent"))
		require.Equal(t, "app-x", out.Get("X-App"))
		require.Empty(t, out.Get("X-Custom"))
		require.Empty(t, out.Get("Cookie"))
		require.Empty(t, out.Get("Authorization"))
		require.Empty(t, out.Get("X-Api-Key"))
	})

	t.Run("forward mode: blacklist is the only filter", func(t *testing.T) {
		policy := EffectiveUpstreamPolicy{ForwardClientHeaders: true}
		ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
		out := copyUnderPolicy(ctx)
		require.Equal(t, "my-client/1.0", out.Get("User-Agent"))
		require.Equal(t, "app-x", out.Get("X-App"))
		require.Equal(t, "custom-value", out.Get("X-Custom"), "forward mode must pass non-whitelisted headers")
		require.Empty(t, out.Get("Cookie"), "blacklist still applies")
		require.Empty(t, out.Get("Authorization"), "blacklist still applies")
		require.Empty(t, out.Get("X-Api-Key"), "blacklist still applies")
	})
}

func TestShouldSkipModelRewrite_LegacyWhenAbsent(t *testing.T) {
	require.False(t, ShouldSkipModelRewrite(context.Background()))
}

func TestShouldSkipModelRewrite_PolicyFalseMatchesLegacy(t *testing.T) {
	policy := EffectiveUpstreamPolicy{SkipModelRewrite: false}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	require.False(t, ShouldSkipModelRewrite(ctx))
}

func TestShouldSkipModelRewrite_PolicyTrueReturnsTrue(t *testing.T) {
	policy := EffectiveUpstreamPolicy{SkipModelRewrite: true}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	require.True(t, ShouldSkipModelRewrite(ctx))
}

func TestAccount_GetMappedModelForUpstream_LegacyAppliesMapping(t *testing.T) {
	a := &Account{
		Credentials: map[string]any{"model_mapping": map[string]any{"claude-3-7-sonnet-20250219": "claude-3-haiku-20240307"}},
	}
	// No policy → legacy mapping applies
	require.Equal(t, "claude-3-haiku-20240307", a.GetMappedModelForUpstream(context.Background(), "claude-3-7-sonnet-20250219"))
	// Unmapped model → returns raw
	require.Equal(t, "claude-3-5-haiku-20241022", a.GetMappedModelForUpstream(context.Background(), "claude-3-5-haiku-20241022"))
}

func TestAccount_GetMappedModelForUpstream_PolicySkipReturnsRaw(t *testing.T) {
	a := &Account{
		Credentials: map[string]any{"model_mapping": map[string]any{"claude-3-7-sonnet-20250219": "claude-3-haiku-20240307"}},
	}
	policy := EffectiveUpstreamPolicy{SkipModelRewrite: true}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	// With SkipModelRewrite=true, raw model wins even when mapping would match.
	require.Equal(t, "claude-3-7-sonnet-20250219", a.GetMappedModelForUpstream(ctx, "claude-3-7-sonnet-20250219"))
}

func TestAccount_GetMappedModelForUpstream_PolicyFalseAppliesMapping(t *testing.T) {
	a := &Account{
		Credentials: map[string]any{"model_mapping": map[string]any{"claude-3-7-sonnet-20250219": "claude-3-haiku-20240307"}},
	}
	policy := EffectiveUpstreamPolicy{SkipModelRewrite: false}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	require.Equal(t, "claude-3-haiku-20240307", a.GetMappedModelForUpstream(ctx, "claude-3-7-sonnet-20250219"))
}

func TestAccount_GetMappedModelForUpstream_NilSafe(t *testing.T) {
	var a *Account
	require.Equal(t, "anything", a.GetMappedModelForUpstream(context.Background(), "anything"))
}

func TestShouldForwardBetaFlags_LegacyWhenAbsent(t *testing.T) {
	require.False(t, ShouldForwardBetaFlags(context.Background()))
}

func TestShouldForwardBetaFlags_PolicyFalseMatchesLegacy(t *testing.T) {
	policy := EffectiveUpstreamPolicy{ForwardBetaFlags: false}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	require.False(t, ShouldForwardBetaFlags(ctx))
}

func TestShouldForwardBetaFlags_PolicyTrueReturnsTrue(t *testing.T) {
	policy := EffectiveUpstreamPolicy{ForwardBetaFlags: true}
	ctx := SetUpstreamPolicyInContext(context.Background(), &policy)
	require.True(t, ShouldForwardBetaFlags(ctx))
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

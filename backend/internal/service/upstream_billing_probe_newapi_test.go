package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildNewAPIPricingURLStripsOpenAIVersionSuffix(t *testing.T) {
	require.Equal(t, "https://www.sheapi.top/api/pricing", buildNewAPIPricingURL("https://www.sheapi.top/v1"))
	require.Equal(t, "https://www.sheapi.top/api/pricing", buildNewAPIPricingURL("https://www.sheapi.top/v1/"))
	require.Equal(t, "https://www.sheapi.top/api/pricing", buildNewAPIPricingURL("https://www.sheapi.top"))
	require.Equal(t, "https://gateway.example.com/relay/api/pricing", buildNewAPIPricingURL("https://gateway.example.com/relay/v1"))
	require.Equal(t, "https://www.sheapi.top/api/user/self/groups", buildNewAPISiteAPIURL("https://www.sheapi.top/v1", newAPIUserGroupsPath))
}

func TestResolveNewAPIUpstreamGroupPrefersExplicitCredential(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			upstreamGroupCredentialKey: "plus_0720",
		},
	}
	ratios := map[string]float64{
		"openai":    1,
		"plus_0720": 0.05,
		"deepseek":  0.07,
	}
	name, rate, ok := resolveNewAPIUpstreamGroup(account, ratios)
	require.True(t, ok)
	require.Equal(t, "plus_0720", name)
	require.Equal(t, 0.05, rate)
}

func TestResolveNewAPIUpstreamGroupFallsBackToPlatform(t *testing.T) {
	account := &Account{Platform: PlatformDeepseek}
	ratios := map[string]float64{
		"DeepSeek": 0.07,
		"openai":   1,
	}
	name, rate, ok := resolveNewAPIUpstreamGroup(account, ratios)
	require.True(t, ok)
	require.Equal(t, "DeepSeek", name)
	require.Equal(t, 0.07, rate)
}

func TestResolveNewAPIUpstreamGroupUsesUserGroupCandidate(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI}
	ratios := map[string]float64{"deepseek": 0.065, "openai": 1}
	name, rate, ok := resolveNewAPIUpstreamGroup(account, ratios, userGroupCandidates(account, "deepseek")...)
	require.True(t, ok)
	require.Equal(t, "deepseek", name)
	require.Equal(t, 0.065, rate)
}

func TestResolveNewAPIUpstreamGroupSkipsUnknownExplicitAndUsesTokenGroup(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			upstreamGroupCredentialKey: "cs", // token name, not a billing group
		},
	}
	ratios := map[string]float64{"plus_0720": 0.045, "distributor": 1}
	name, rate, ok := resolveNewAPIUpstreamGroup(account, ratios, userGroupCandidates(account, "plus_0720", "distributor")...)
	require.True(t, ok)
	require.Equal(t, "plus_0720", name)
	require.Equal(t, 0.045, rate)
}

func TestMatchNewAPITokenGroupByName(t *testing.T) {
	body := []byte(`{"success":true,"data":{"items":[{"name":"cs","group":"plus_0720","key":"L7V1**********dgSF"},{"name":"deepseek","group":"deepseek","key":"Jqda**********Ui1B"}]}}`)
	group, ok := matchNewAPITokenGroup(body, "cs", "sk-L7V1abcdefghijklmnopdgSF")
	require.True(t, ok)
	require.Equal(t, "plus_0720", group)
}

func TestParseNewAPITokenUsageName(t *testing.T) {
	require.Equal(t, "cs", parseNewAPITokenUsageName([]byte(`{"code":true,"data":{"name":"cs","object":"token_usage"}}`)))
}

func TestMaskNewAPITokenKey(t *testing.T) {
	require.Equal(t, "L7V1**********dgSF", maskNewAPITokenKey("sk-L7V1abcdefghijklmnopdgSF"))
}

func TestResolveNewAPIUpstreamGroupMissing(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI}
	_, _, ok := resolveNewAPIUpstreamGroup(account, map[string]float64{"deepseek": 0.07})
	require.False(t, ok)
}

func TestParseNewAPIPricingProbeResponse(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	account := &Account{Platform: PlatformDeepseek}
	body := []byte(`{"success":true,"group_ratio":{"deepseek":0.07,"openai":1}}`)
	data, err := parseNewAPIPricingProbeResponse(body, account, now, "")
	require.NoError(t, err)
	require.Equal(t, "sub2api.key_billing", data["object"])
	require.Equal(t, "newapi.pricing", data["billing_source"])
	require.Equal(t, "deepseek", data["group_name"])
	require.Equal(t, 0.07, data["group_rate_multiplier"])
	require.Equal(t, 0.07, data["resolved_rate_multiplier"])
	require.Equal(t, 0.07, data["effective_rate_multiplier"])
	require.Equal(t, false, data["peak_rate_enabled"])
}

func TestParseNewAPIPricingProbeResponseMissingGroup(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI}
	body := []byte(`{"success":true,"group_ratio":{"deepseek":0.07}}`)
	_, err := parseNewAPIPricingProbeResponse(body, account, time.Now().UTC(), "")
	require.ErrorIs(t, err, errNewAPIUpstreamGroupMissing)
}

func TestParseNewAPIPricingProbeResponseUnsupported(t *testing.T) {
	_, err := parseNewAPIPricingProbeResponse([]byte(`{"hello":"world"}`), &Account{Platform: PlatformOpenAI}, time.Now().UTC(), "")
	require.ErrorIs(t, err, errNewAPIPricingUnsupported)
}

func TestParseNewAPIUserGroupsResponse(t *testing.T) {
	body := []byte(`{"success":true,"data":{"deepseek":{"ratio":0.065,"desc":"x"},"openai":{"ratio":"自动"}}}`)
	ratios, ok := parseNewAPIUserGroupsResponse(body)
	require.True(t, ok)
	require.Equal(t, 0.065, ratios["deepseek"])
	_, hasOpenAI := ratios["openai"]
	require.False(t, hasOpenAI)
}

func TestParseUpstreamRateMultiplierCredential(t *testing.T) {
	account := &Account{Credentials: map[string]any{upstreamRateMultiplierCredentialKey: "0.065"}}
	rate, ok := parseUpstreamRateMultiplierCredential(account)
	require.True(t, ok)
	require.Equal(t, 0.065, rate)

	account = &Account{Credentials: map[string]any{upstreamRateMultiplierCredentialKey: 0.065}}
	rate, ok = parseUpstreamRateMultiplierCredential(account)
	require.True(t, ok)
	require.Equal(t, 0.065, rate)
}

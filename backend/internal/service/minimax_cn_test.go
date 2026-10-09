package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMiniMaxCNCodingPlanProvider(t *testing.T) {
	for _, base := range []string{"https://api.minimax.cn/v1", "https://API.MINIMAX.CN/anthropic"} {
		a := codingAccount(PlatformMiniMax)
		a.Credentials["base_url"] = base
		require.Equal(t, PlatformMiniMax, a.GetCodingPlanProvider(), base)
	}
	for _, base := range []string{"https://api.minimax.cn.evil.example/v1", "https://evil.example/api.minimaxi.com", "https://api.minimax.io.evil.example/v1", "https://api.minimax.cn@evil.example/v1"} {
		a := codingAccount(PlatformMiniMax)
		a.Credentials["base_url"] = base
		require.Empty(t, a.GetCodingPlanProvider(), base)
	}
}

func TestMiniMaxCNQuotaRejectsMissingWindows(t *testing.T) {
	a := codingAccount(PlatformMiniMax)
	a.Credentials["base_url"] = "https://api.minimax.cn/v1"
	repo := &cnBalanceProbeRepo{account: a}
	upstream := &cnBalanceResponseUpstream{statusCode: http.StatusOK, body: `{"base_resp":{"status_code":0}}`}
	s := NewCNProviderQuotaService(repo, nil, upstream, nil)
	result, err := s.QueryUsageForAccount(context.Background(), a)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.NotEmpty(t, result.Error)
	require.Empty(t, repo.extraWrites)
}

func TestMiniMaxCNQuotaURL(t *testing.T) {
	require.Equal(t, "https://www.minimax.cn/v1/token_plan/remains", minimaxQuotaURL("https://api.minimax.cn/anthropic"))
}

func TestMiniMaxCNQuotaRequestAndPersistence(t *testing.T) {
	a := codingAccount(PlatformMiniMax)
	a.Credentials["base_url"] = "https://api.minimax.cn/v1"
	upstream := &httpUpstreamRecorder{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"base_resp":{"status_code":0},"model_remains":[{"model_name":"general","current_interval_remaining_percent":75,"current_weekly_status":1,"current_weekly_remaining_percent":90}]}`)),
	}}}
	repo := &cnBalanceProbeRepo{account: a}
	s := NewCNProviderQuotaService(repo, nil, upstream, cnProbeAllowlistConfig("www.minimax.cn"))
	result, err := s.QueryUsageForAccount(context.Background(), a)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.True(t, result.Persisted)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://www.minimax.cn/v1/token_plan/remains", upstream.requests[0].URL.String())
	require.Equal(t, "Bearer sk-test", upstream.requests[0].Header.Get("Authorization"))
	require.Len(t, result.Tiers, 2)
	require.Equal(t, 25.0, result.Tiers[0].UsedPercent)
	require.Equal(t, 10.0, result.Tiers[1].UsedPercent)
	require.Len(t, repo.extraWrites, 1)
	require.Equal(t, 25.0, repo.extraWrites[0]["minimax_5h_used_percent"])
}

func TestMiniMaxCNQuotaRejectsBlockedEndpoint(t *testing.T) {
	a := codingAccount(PlatformMiniMax)
	a.Credentials["base_url"] = "https://api.minimax.cn/v1"
	upstream := &recordingHTTPUpstream{}
	s := NewCNProviderQuotaService(&cnBalanceProbeRepo{account: a}, nil, upstream, cnProbeAllowlistConfig("api.minimax.cn"))
	_, err := s.QueryUsageForAccount(context.Background(), a)
	require.ErrorContains(t, err, "CN_QUOTA_URL_REJECTED")
	require.Zero(t, upstream.calls)
}

func TestMiniMaxCNMetadataSyncPersistsCapabilities(t *testing.T) {
	a := codingAccount(PlatformMiniMax)
	a.Credentials["base_url"] = "https://api.minimax.cn/v1"
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"MiniMax-M3"}]}`))},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"minimax-cn":{"id":"minimax-cn","api":"https://api.minimax.cn/anthropic/v1","models":{"MiniMax-M3":{"id":"MiniMax-M3","name":"MiniMax M3","reasoning":true,"reasoning_options":[{"type":"toggle"}],"modalities":{"input":["text","image"]},"limit":{"context":1000000,"output":512000}}}}}`))},
	}}
	repo := &upstreamModelMetadataRepoStub{}
	s := &AccountTestService{cfg: upstreamModelSyncTestConfig(), accountRepo: repo, httpUpstream: upstream}
	catalog, err := s.SyncUpstreamModelCatalog(context.Background(), a)
	require.NoError(t, err)
	require.Empty(t, catalog.Warnings)
	require.Equal(t, int64(1000000), catalog.Metadata["MiniMax-M3"].ContextWindow)
	require.Equal(t, []string{"text", "image"}, catalog.Metadata["MiniMax-M3"].InputModalities)
	require.Equal(t, []string{"none", "high"}, catalog.Metadata["MiniMax-M3"].SupportedReasoningLevels)
	require.NotNil(t, repo.updates[UpstreamModelMetadataExtraKey])
	require.Equal(t, "https://api.minimax.cn/v1/models", upstream.requests[0].URL.String())
}

func TestMiniMaxAnthropicDiscoveryPreservesRegion(t *testing.T) {
	s := &AccountTestService{cfg: upstreamModelSyncTestConfig()}
	for _, host := range []string{"api.minimax.cn", "api.minimax.io", "api.minimaxi.com"} {
		a := codingAccount(PlatformMiniMax)
		a.Credentials["api_protocol"] = APIProtocolAnthropic
		a.Credentials["base_url"] = "https://" + host + "/anthropic"
		req, err := s.buildUpstreamModelsRequest(context.Background(), a)
		require.NoError(t, err)
		require.Equal(t, "https://"+host+"/v1/models", req.URL.String())
	}
}

func TestMiniMaxMetadataMatchesOfficialProtocolVariants(t *testing.T) {
	registry := map[string]modelsDevProvider{}
	for _, id := range []string{"minimax-cn", "minimax"} {
		registry[id] = modelsDevProvider{ID: id, Models: map[string]modelsDevModel{"MiniMax-M3": {ID: "MiniMax-M3"}}}
	}
	for base, want := range map[string]string{
		"https://api.minimax.cn/v1":          "minimax-cn",
		"https://api.minimaxi.com/anthropic": "minimax-cn",
		"https://api.minimax.io/v1":          "minimax",
	} {
		provider, ok := matchModelsDevProvider(registry, base)
		require.True(t, ok, base)
		require.Equal(t, want, provider.ID, base)
	}
	for _, base := range []string{"https://relay.example/v1", "https://api.minimax.cn.evil.example/v1"} {
		_, ok := matchModelsDevProvider(registry, base)
		require.False(t, ok, base)
	}
}

func TestMiniMaxMetadataToggleGates(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider string
		reason   bool
		options  []modelsDevReasoningOption
		levels   []string
	}{
		{"toggle", "minimax-cn", true, []modelsDevReasoningOption{{Type: "toggle"}}, []string{"none", "high"}},
		{"no reasoning", "minimax-cn", false, []modelsDevReasoningOption{{Type: "toggle"}}, []string{}},
		{"explicit effort", "minimax-cn", true, []modelsDevReasoningOption{{Type: "toggle"}, {Type: "effort", Values: []any{"low", "high"}}}, []string{"low", "high"}},
		{"other provider", "other", true, []modelsDevReasoningOption{{Type: "toggle"}}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := codingAccount(PlatformMiniMax)
			a.Credentials["base_url"] = "https://api.minimax.cn/v1"
			s := &AccountTestService{
				httpUpstream:            &httpUpstreamRecorder{},
				modelMetadataRegistryAt: time.Now(),
				modelMetadataRegistry: map[string]modelsDevProvider{tc.provider: {
					ID: tc.provider, API: "https://api.minimax.cn/v1",
					Models: map[string]modelsDevModel{"MiniMax-M3": {ID: "MiniMax-M3", Reasoning: &tc.reason, ReasoningOptions: tc.options}},
				}},
			}
			metadata, err := s.fetchModelsDevMetadata(context.Background(), a, []string{"MiniMax-M3"})
			require.NoError(t, err)
			require.Equal(t, tc.levels, metadata["MiniMax-M3"].SupportedReasoningLevels)
		})
	}
}

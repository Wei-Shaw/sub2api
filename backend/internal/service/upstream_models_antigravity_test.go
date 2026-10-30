package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

func antigravityModelSyncAccount(plan string) *Account {
	return &Account{
		ID: 121, Platform: PlatformAntigravity, Type: AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "model-sync-test-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			"project_id":   "model-sync-test-project",
			"plan_type":    plan,
		},
	}
}

func antigravityModelSyncService() *AccountTestService {
	return &AccountTestService{
		antigravityGatewayService: &AntigravityGatewayService{tokenProvider: &AntigravityTokenProvider{}},
		cfg:                       upstreamModelSyncTestConfig(),
	}
}

func withAntigravityModelSyncEndpoints(t *testing.T, production, daily string) {
	t.Helper()
	previous := antigravity.BaseURLs
	antigravity.BaseURLs = []string{production, daily}
	t.Cleanup(func() { antigravity.BaseURLs = previous })
}

func TestFetchUpstreamSupportedModelsAntigravityUsesGenerationEndpoint(t *testing.T) {
	t.Setenv(antigravityForwardBaseURLEnv, "")
	production := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"models":{"gemini-3.7-flash-high":{}}}`)
	}))
	defer production.Close()
	daily := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"models":{"gemini-3.8-flash-high":{}}}`)
	}))
	defer daily.Close()
	withAntigravityModelSyncEndpoints(t, production.URL, daily.URL)

	for _, tc := range []struct {
		name  string
		plan  string
		mode  string
		model string
	}{
		{name: "Ultra daily", plan: "Ultra", model: "gemini-3.8-flash-high"},
		{name: "Pro daily", plan: "Pro", model: "gemini-3.8-flash-high"},
		{name: "Free production", plan: "Free", model: "gemini-3.7-flash-high"},
		{name: "explicit production", plan: "Ultra", mode: "prod", model: "gemini-3.7-flash-high"},
		{name: "explicit daily", plan: "Free", mode: "daily", model: "gemini-3.8-flash-high"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(antigravityForwardBaseURLEnv, tc.mode)
			models, err := antigravityModelSyncService().FetchUpstreamSupportedModels(context.Background(), antigravityModelSyncAccount(tc.plan))
			require.NoError(t, err)
			require.Equal(t, []string{tc.model}, models, "catalog must come from the endpoint used for generation")
		})
	}
}

func TestSyncUpstreamModelCatalogAntigravityPreservesDirectCapabilities(t *testing.T) {
	t.Setenv(antigravityForwardBaseURLEnv, "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"models":{"gemini-3.8-flash-high":{
			"displayName":"Gemini 3.8 Flash High",
			"supportsImages":true,"supportsThinking":true,"thinkingBudget":24576,
			"maxTokens":1048576,"maxOutputTokens":65536
		}}}`)
	}))
	defer server.Close()
	withAntigravityModelSyncEndpoints(t, server.URL, server.URL)
	account := antigravityModelSyncAccount("Ultra")
	account.Credentials["model_mapping"] = map[string]any{"gemini-3.8-flash-high": "gemini-3.8-flash-high"}
	registry := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader(`{"error":"registry unavailable"}`))}}
	svc := antigravityModelSyncService()
	svc.accountRepo = &upstreamModelMetadataRepoStub{}
	svc.httpUpstream = registry

	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"gemini-3.8-flash-high"}, catalog.Models)
	require.Contains(t, catalog.Metadata, "gemini-3.8-flash-high")
	metadata := catalog.Metadata["gemini-3.8-flash-high"]
	require.Equal(t, "Gemini 3.8 Flash High", metadata.DisplayName)
	require.NotNil(t, metadata.Reasoning)
	require.True(t, *metadata.Reasoning)
	require.Equal(t, []string{"high"}, metadata.SupportedReasoningLevels, "fixed variant must advertise only its own reasoning level")
	require.Equal(t, "high", metadata.DefaultReasoningLevel)
	require.Equal(t, []string{"text", "image"}, metadata.InputModalities)
	require.Equal(t, int64(1048576), metadata.ContextWindow)
	require.Equal(t, int64(65536), metadata.MaxOutputTokens)
	require.Empty(t, registry.requests, "complete upstream capabilities must not need guessed registry capabilities")
	require.Empty(t, catalog.Warnings)
	snapshot := account.GetUpstreamModelMetadataSnapshot()
	require.NotNil(t, snapshot)
	require.Equal(t, "upstream", snapshot.Source)
	serializedMetadata, err := json.Marshal(metadata)
	require.NoError(t, err)
	serializedSnapshotEntry, err := json.Marshal(snapshot.Models["gemini-3.8-flash-high"])
	require.NoError(t, err)
	require.JSONEq(t, string(serializedMetadata), string(serializedSnapshotEntry))
}

func TestSyncUpstreamModelCatalogAntigravityTieredReasoning(t *testing.T) {
	t.Setenv(antigravityForwardBaseURLEnv, "")
	for _, tc := range []struct {
		budget       string
		defaultLevel string
	}{
		{budget: "-1", defaultLevel: "high"},
		{budget: "0", defaultLevel: "low"},
		{budget: "1024", defaultLevel: "low"},
		{budget: "1025", defaultLevel: "medium"},
		{budget: "4000", defaultLevel: "medium"},
		{budget: "8192", defaultLevel: "medium"},
		{budget: "8193", defaultLevel: "high"},
		{defaultLevel: "high"},
	} {
		t.Run("budget="+tc.budget, func(t *testing.T) {
			budgetJSON := ""
			if tc.budget != "" {
				budgetJSON = `"thinkingBudget":` + tc.budget + ","
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, fmt.Sprintf(`{"models":{"gemini-3.8-flash-tiered":{
					"supportsImages":false,"supportsThinking":true,%s
					"maxTokens":1048576,"maxOutputTokens":65536
				}}}`, budgetJSON))
			}))
			defer server.Close()
			withAntigravityModelSyncEndpoints(t, server.URL, server.URL)
			account := antigravityModelSyncAccount("Ultra")
			account.Credentials["model_mapping"] = map[string]any{"gemini-3.8-flash-tiered": "gemini-3.8-flash-tiered"}
			svc := antigravityModelSyncService()
			svc.accountRepo = &upstreamModelMetadataRepoStub{}

			catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
			require.NoError(t, err)
			metadata := catalog.Metadata["gemini-3.8-flash-tiered"]
			require.Equal(t, []string{"low", "medium", "high"}, metadata.SupportedReasoningLevels)
			require.Equal(t, tc.defaultLevel, metadata.DefaultReasoningLevel, "tiered default must follow its upstream thinking budget")
			require.Equal(t, []string{"text"}, metadata.InputModalities)
		})
	}
}

func TestSyncUpstreamModelCatalogAntigravityMissingImageSupportIsTextOnly(t *testing.T) {
	t.Setenv(antigravityForwardBaseURLEnv, "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"models":{"claude-plain":{
			"supportsThinking":false,"maxTokens":200000,"maxOutputTokens":64000
		}}}`)
	}))
	defer server.Close()
	withAntigravityModelSyncEndpoints(t, server.URL, server.URL)
	account := antigravityModelSyncAccount("Ultra")
	account.Credentials["model_mapping"] = map[string]any{"claude-plain": "claude-plain"}
	registry := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader(`{"error":"registry unavailable"}`))}}
	svc := antigravityModelSyncService()
	svc.accountRepo = &upstreamModelMetadataRepoStub{}
	svc.httpUpstream = registry

	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	metadata := catalog.Metadata["claude-plain"]
	require.Equal(t, []string{"text"}, metadata.InputModalities, "missing image declaration must not advertise image input")
	require.NotNil(t, metadata.Reasoning)
	require.False(t, *metadata.Reasoning)
	require.Empty(t, metadata.SupportedReasoningLevels)
	require.Empty(t, registry.requests)
}

func TestSyncUpstreamModelCatalogAntigravityBudgetReasoningLevels(t *testing.T) {
	t.Setenv(antigravityForwardBaseURLEnv, "")
	for _, tc := range []struct {
		model        string
		budget       int
		defaultLevel string
		levels       []string
	}{
		{model: "gemini-2.5-flash", budget: -1, defaultLevel: "high", levels: []string{"low", "medium", "high"}},
		{model: "gemini-2.5-flash-lite", budget: 4000, defaultLevel: "medium", levels: []string{"low", "medium", "high"}},
		{model: "gemini-3.8-flash-low", budget: -1, defaultLevel: "low", levels: []string{"low"}},
		{model: "gemini-3.8-flash-medium", budget: 1000, defaultLevel: "medium", levels: []string{"medium"}},
		{model: "gemini-3.8-flash-high", budget: 4000, defaultLevel: "high", levels: []string{"high"}},
	} {
		t.Run(tc.model, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, fmt.Sprintf(`{"models":{%q:{
					"supportsImages":true,"supportsThinking":true,"thinkingBudget":%d,
					"maxTokens":1048576,"maxOutputTokens":65536
				}}}`, tc.model, tc.budget))
			}))
			defer server.Close()
			withAntigravityModelSyncEndpoints(t, server.URL, server.URL)
			account := antigravityModelSyncAccount("Ultra")
			account.Credentials["model_mapping"] = map[string]any{tc.model: tc.model}
			svc := antigravityModelSyncService()
			svc.accountRepo = &upstreamModelMetadataRepoStub{}

			catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
			require.NoError(t, err)
			metadata := catalog.Metadata[tc.model]
			require.Equal(t, tc.levels, metadata.SupportedReasoningLevels, "default budget must not restrict configurable models to one reasoning level")
			require.Equal(t, tc.defaultLevel, metadata.DefaultReasoningLevel)
			require.Empty(t, catalog.Warnings)
		})
	}
}

func TestConfiguredUpstreamModelsForCapabilitySyncSkipsInjectedAntigravityPassthroughs(t *testing.T) {
	account := &Account{
		Platform: PlatformAntigravity,
		Credentials: map[string]any{"model_mapping": map[string]any{
			"gemini-3.8-flash-high": "gemini-3.8-flash-high",
			"gemini-3.8-flash":      "gemini-3.8-flash-tiered",
			"custom-alias":          "gemini-3.6-flash-low",
		}},
	}
	// 运行时补齐的默认透传仍可调用（存量账号兼容）。
	require.True(t, account.IsModelSupported("gemini-3.7-flash-medium"))

	got := configuredUpstreamModelsForCapabilitySync(account)
	require.ElementsMatch(t, []string{"gemini-3.8-flash-high", "gemini-3.8-flash-tiered", "gemini-3.6-flash-low"}, got)

	// 空映射走默认目录，其中的同名条目不是运行时补齐。
	empty := &Account{Platform: PlatformAntigravity, Credentials: map[string]any{}}
	require.Contains(t, configuredUpstreamModelsForCapabilitySync(empty), "gemini-3.8-flash-low")
}

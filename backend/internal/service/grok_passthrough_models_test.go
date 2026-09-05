package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

func TestParseGrokUpstreamModelsCatalogPreservesEffortMetadata(t *testing.T) {
	body := []byte(`{
		"object":"list",
		"data":[
			{
				"id":"grok-4.20",
				"object":"model",
				"context_window":256000,
				"supportsReasoningEffort":true,
				"reasoningEfforts":[{"value":"xhigh","label":"xHigh"}]
			}
		]
	}`)
	items := parseGrokUpstreamModelsCatalog(body)
	require.Len(t, items, 1)
	item, ok := items[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "grok-4.20", item["id"])
	require.Equal(t, true, item["supportsReasoningEffort"])
	require.EqualValues(t, 256000, item["context_window"])
}

func TestResolveGrokPassthroughModelsUsesUpstreamCatalog(t *testing.T) {
	groupID := int64(88)
	account := Account{
		ID:       1,
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"grok_passthrough": true},
		Credentials: map[string]any{
			"api_key":  "xai-test-key",
			"base_url": xai.DefaultBaseURL,
		},
	}
	repo := &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {account}}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"object":"list","data":[{"id":"grok-4.20","reasoningEfforts":[{"value":"max"}]}]}`)),
	}}
	svc := &GatewayService{
		accountRepo:  repo,
		httpUpstream: upstream,
	}

	resolution := svc.ResolveGrokPassthroughModels(context.Background(), &groupID)
	require.True(t, resolution.Enabled)
	require.Len(t, resolution.RawData, 1)
	item, ok := resolution.RawData[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "grok-4.20", item["id"])
	require.Contains(t, upstream.lastReq.URL.String(), "/models")
	require.Equal(t, "Bearer xai-test-key", upstream.lastReq.Header.Get("Authorization"))
}

func TestResolveGrokPassthroughModelsFallsBackToObservedIDs(t *testing.T) {
	groupID := int64(89)
	account := Account{
		ID:       2,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"grok_passthrough": true,
			grokObservedModelsExtraKey: map[string]any{
				"models":     []any{"grok-observed"},
				"fetched_at": "2026-01-01T00:00:00Z",
			},
		},
		Credentials: map[string]any{
			"access_token": "oauth-token",
			"base_url":     xai.DefaultCLIBaseURL,
		},
	}
	repo := &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {account}}}
	svc := &GatewayService{
		accountRepo:  repo,
		httpUpstream: &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader(`nope`))}},
	}

	resolution := svc.ResolveGrokPassthroughModels(context.Background(), &groupID)
	require.True(t, resolution.Enabled)
	require.Empty(t, resolution.RawData)
	require.Equal(t, []string{"grok-observed"}, resolution.FallbackIDs)
}

func TestResolveGrokPassthroughModelsDisabledWithoutFlag(t *testing.T) {
	groupID := int64(90)
	repo := &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {{
		ID:       3,
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
	}}}}
	svc := &GatewayService{accountRepo: repo}

	resolution := svc.ResolveGrokPassthroughModels(context.Background(), &groupID)
	require.False(t, resolution.Enabled)
}

func TestGetAvailableModels_GrokPassthroughUsesDefaultFallback(t *testing.T) {
	groupID := int64(91)
	repo := &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {{
		ID:       4,
		Platform: PlatformGrok,
		Extra:    map[string]any{"grok_passthrough": true},
		Credentials: map[string]any{
			"model_mapping": map[string]any{"stale-model": "stale-model"},
		},
	}}}}
	svc := &GatewayService{accountRepo: repo}

	require.Nil(t, svc.GetAvailableModels(context.Background(), &groupID, PlatformGrok))
}

func TestFilterGrokPassthroughCatalog(t *testing.T) {
	items := []any{
		map[string]any{"id": "keep"},
		map[string]any{"id": "drop"},
	}
	filtered := FilterGrokPassthroughCatalog(items, []string{"keep"})
	require.Len(t, filtered, 1)
	require.Equal(t, "keep", grokPassthroughCatalogModelID(filtered[0]))
}

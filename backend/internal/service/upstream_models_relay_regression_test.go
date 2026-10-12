package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRelayCapabilitySyncPlatformMatrix(t *testing.T) {
	for _, tc := range []struct{ platform, provider, model string }{
		{PlatformAnthropic, "anthropic", "claude-opus-4-6"},
		{PlatformOpenAI, "anthropic", "claude-opus-4-6"},
		{PlatformGemini, "google", "gemini-2.5-pro"},
		{PlatformAntigravity, "google", "gemini-2.5-pro"},
		{PlatformOpenAI, "deepseek", "deepseek-v4-flash"},
		{PlatformDeepseek, "deepseek", "deepseek-v4-flash"},
		{PlatformKimi, "moonshotai-cn", "kimi-k3"},
		{PlatformZhipu, "zhipuai", "glm-5.2"},
	} {
		t.Run(tc.platform+"/"+tc.provider, func(t *testing.T) {
			model := modelsDevModel{ID: tc.model, Reasoning: boolPtr(false), Modalities: modelsDevModalities{Input: []string{"text"}}, Limit: modelsDevLimit{Context: 128000}}
			registry := map[string]modelsDevProvider{tc.provider: {ID: tc.provider, Models: map[string]modelsDevModel{tc.model: model}}}
			body, err := json.Marshal(registry)
			require.NoError(t, err)
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}}
			svc := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
			result, err := svc.fetchModelsDevMetadata(context.Background(), &Account{Platform: tc.platform, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://relay.example", "api_key": "test"}}, []string{tc.model, "private-model", tc.model + "-unknown"})
			require.NoError(t, err)
			require.Len(t, result, 1)
			require.True(t, upstreamModelMetadataIsComplete(result[tc.model]))
		})
	}
}

func TestRelayCapabilitySyncPreviewReportsPartialAndMissingModels(t *testing.T) {
	for _, id := range []int64{0, 448} {
		t.Run(map[bool]string{true: "preview", false: "saved"}[id == 0], func(t *testing.T) {
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"hy3"},{"id":"hy4"}]}`))},
				{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"tencent-token-plan":{"models":{"hy3":{"id":"hy3","reasoning":false,"modalities":{"input":["text"]},"limit":{"context":128000}},"hy4-preview":{"id":"hy4-preview","reasoning":false,"modalities":{"input":["text"]},"limit":{"context":256000}}}}}`))},
			}}
			repo := &upstreamModelMetadataRepoStub{}
			svc := &AccountTestService{httpUpstream: upstream, accountRepo: repo, cfg: upstreamModelSyncTestConfig()}
			result, err := svc.SyncUpstreamModelCatalog(context.Background(), &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://relay.example", "api_key": "test"}})
			require.NoError(t, err)
			require.Len(t, result.Warnings, 1)
			w := result.Warnings[0]
			require.Equal(t, UpstreamModelMetadataPartialCode, w.Code)
			require.Equal(t, []string{"hy4"}, w.ModelIDs)
			require.Equal(t, 1, w.CompleteCount)
			require.Equal(t, 2, w.TotalCount)
			require.Equal(t, "metadata_missing", w.Reason)
			if id == 0 {
				require.Nil(t, repo.updates)
			} else {
				require.NotNil(t, repo.updates)
			}
		})
	}
}

func TestRelayCapabilitySyncRegistryOutagePreservesSavedSnapshot(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"glm-5.2"}]}`))},
		{StatusCode: 503, Body: io.NopCloser(strings.NewReader(`unavailable`))},
	}}
	repo := &upstreamModelMetadataRepoStub{}
	svc := &AccountTestService{httpUpstream: upstream, accountRepo: repo, cfg: upstreamModelSyncTestConfig()}
	account := &Account{ID: 448, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://relay.example", "api_key": "test"}}
	snapshot := UpstreamModelMetadataSnapshot{Source: "upstream", SyncedAt: "earlier", Models: map[string]UpstreamModelMetadata{"glm-5.2": {ID: "glm-5.2", Reasoning: boolPtr(false), InputModalities: []string{"text"}, ContextWindow: 128000}}}
	account.SetUpstreamModelMetadataSnapshot(snapshot)
	result, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.Nil(t, repo.updates)
	require.Equal(t, &snapshot, account.GetUpstreamModelMetadataSnapshot())
	require.Equal(t, "registry_unavailable", result.Warnings[0].Reason)
	require.Equal(t, []string{"glm-5.2"}, result.Warnings[0].ModelIDs)
}

func TestRelayCapabilityRegistryRefreshesAfterFiveMinutes(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"deepseek":{"models":{"deepseek-v4-flash":{"id":"deepseek-v4-flash","reasoning":false,"modalities":{"input":["text"]},"limit":{"context":128000}}}}}`))}}
	svc := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig(), modelMetadataRegistry: map[string]modelsDevProvider{"deepseek": {ID: "deepseek"}}, modelMetadataRegistryAt: time.Now().Add(-6 * time.Minute)}
	result, err := svc.fetchModelsDevMetadata(context.Background(), &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://relay.example", "api_key": "test"}}, []string{"deepseek-v4-flash"})
	require.NoError(t, err)
	require.Contains(t, result, "deepseek-v4-flash")
	require.Len(t, upstream.requests, 1)
}

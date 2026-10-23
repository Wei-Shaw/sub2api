//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNativeCompactionColdCatalogRefusalCanSelectAnotherAccount(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	accounts := make([]Account, 2)
	for i, path := range []string{"/compact-block/v1", "/compact-pass/v1"} {
		account := commandCodeTestAccount(int64(75001 + i))
		account.Status, account.Schedulable, account.Concurrency, account.Priority = StatusActive, true, 1, i
		base := "https://compact-forward.example.test" + path
		account.Credentials["api_base_urls"] = map[string]any{APIProtocolChatCompletions: base, APIProtocolResponses: base}
		key := modelProtocolCatalogKey(account, buildOpenAIModelsURL(base))
		t.Cleanup(func() {
			upstreamModelProtocols.mu.Lock()
			delete(upstreamModelProtocols.entries, key)
			upstreamModelProtocols.mu.Unlock()
		})
		accounts[i] = *account
	}
	upstream := &commandCodeAlphaUpstream{responses: map[string]commandCodeAlphaResponse{
		"/compact-block/v1/models":   {status: 200, body: `{"data":[{"id":"custom-native","supported_endpoints":["/chat/completions"]}]}`},
		"/compact-pass/v1/models":    {status: 200, body: `{"data":[{"id":"custom-native","supported_endpoints":["/responses"]}]}`},
		"/compact-pass/v1/responses": {status: 200, body: "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_compact_test\",\"status\":\"completed\",\"output\":[{\"id\":\"cmp_test\",\"type\":\"compaction\",\"encrypted_content\":\"opaque-test\"}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"},
	}}
	svc := newOpenAICompactionSchedulerTestService(accounts, false)
	svc.httpUpstream, svc.cfg = upstream, rawChatCompletionsTestConfig()
	groupID := int64(95001)
	selectAccount := func(excluded map[int64]struct{}) *AccountSelectionResult {
		selection, _, err := svc.SelectAccountWithSchedulerForCapability(context.Background(), &groupID, "", "", "custom-native", excluded,
			OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityRemoteCompactionV2, false, false, false, PlatformCommandCode)
		require.NoError(t, err)
		require.NotNil(t, selection)
		return selection
	}
	first := selectAccount(nil)
	require.Equal(t, int64(75001), first.Account.ID)
	require.Empty(t, upstream.requests)
	body := []byte(`{"model":"custom-native","stream":true,"input":[{"type":"compaction_trigger"}]}`)
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses")
	MarkOpenAINativeCompactionV2(c)
	_, err := svc.Forward(context.Background(), c, first.Account, body)
	if first.ReleaseFunc != nil {
		first.ReleaseFunc()
	}
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.True(t, failoverErr.ShouldRetryNextAccount())
	require.False(t, c.Writer.Written())
	upstream.mu.Lock()
	firstPaths := make([]string, len(upstream.requests))
	for i, req := range upstream.requests {
		firstPaths[i] = req.URL.Path
	}
	upstream.mu.Unlock()
	require.Equal(t, []string{"/compact-block/v1/models"}, firstPaths, "unsupported candidate must never send an inference request")
	second := selectAccount(map[int64]struct{}{first.Account.ID: {}})
	require.Equal(t, int64(75002), second.Account.ID)
	result, err := svc.Forward(context.Background(), c, second.Account, body)
	if second.ReleaseFunc != nil {
		second.ReleaseFunc()
	}
	require.NoError(t, err)
	require.NotNil(t, result)
	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	require.Len(t, upstream.requests, 3)
	require.Equal(t, "/compact-pass/v1/responses", upstream.requests[2].URL.Path)
}

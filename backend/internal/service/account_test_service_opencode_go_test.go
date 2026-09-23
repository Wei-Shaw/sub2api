//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func openCodeGoTestAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Name:        "oc",
		Platform:    PlatformOpenCodeGo,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-opencode-go-test",
			"api_protocol": APIProtocolAdaptive,
			"base_url":     "https://opencode.ai/zen/go/v1",
			"api_base_urls": map[string]any{
				APIProtocolChatCompletions: "https://opencode.ai/zen/go/v1",
				APIProtocolAnthropic:       "https://opencode.ai/zen/go",
				APIProtocolResponses:       "https://opencode.ai/zen/go/v1",
			},
		},
	}
}

func TestAccountTestService_OpenCodeGoDeepSeekFlashUsesChatCompletions(t *testing.T) {
	account := openCodeGoTestAccount(401)
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNChatTestResponse())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "deepseek-v4-flash", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://opencode.ai/zen/go/v1/chat/completions", upstream.requests[0].URL.String())
	require.Equal(t, "Bearer sk-opencode-go-test", upstream.requests[0].Header.Get("Authorization"))
	require.NotEmpty(t, upstream.requests[0].Header.Get("X-OpenCode-Session"))
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
	require.NotContains(t, upstream.requests[0].URL.Path, "/messages")
}

func TestAccountTestService_OpenCodeGoGrokUsesResponses(t *testing.T) {
	account := openCodeGoTestAccount(402)
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNResponsesTestResponse())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "grok-4.6", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://opencode.ai/zen/go/v1/responses", upstream.requests[0].URL.String())
	require.Equal(t, "Bearer sk-opencode-go-test", upstream.requests[0].Header.Get("Authorization"))
	require.NotEmpty(t, upstream.requests[0].Header.Get("X-OpenCode-Session"))
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestService_OpenCodeGoMiniMaxUsesAnthropicMessages(t *testing.T) {
	account := openCodeGoTestAccount(403)
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNAnthropicTestResponse())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "minimax-m3", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Equal(t, "https://opencode.ai/zen/go/v1/messages", req.URL.String())
	require.Empty(t, req.URL.RawQuery)
	require.Equal(t, "sk-opencode-go-test", req.Header.Get("x-api-key"))
	require.NotEmpty(t, req.Header.Get("X-OpenCode-Session"))
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestService_OpenCodeGoPinnedChatProtocolIgnoresCatalog(t *testing.T) {
	account := openCodeGoTestAccount(404)
	account.Credentials["api_protocol"] = APIProtocolChatCompletions
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNChatTestResponse())
	c, _ := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "grok-4.6", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://opencode.ai/zen/go/v1/chat/completions", upstream.requests[0].URL.String())
}

func TestAccountTestService_OpenCodeGoAppliesModelMappingOnResponses(t *testing.T) {
	account := openCodeGoTestAccount(406)
	account.Credentials["model_mapping"] = map[string]any{
		"opencode/muse-spark-1.3-contributior-free": "muse-spark-1.3-contributior-free",
	}
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNResponsesTestResponse())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "opencode/muse-spark-1.3-contributior-free", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://opencode.ai/zen/go/v1/responses", upstream.requests[0].URL.String())
	require.Equal(t, "muse-spark-1.3-contributior-free", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Contains(t, recorder.Body.String(), `"model":"muse-spark-1.3-contributior-free"`)
	require.NotContains(t, string(upstream.lastBody), "opencode/muse-spark-1.3-contributior-free")
}

func TestAccountTestService_OpenCodeGoEmptyModelDefaultsToChatCatalogID(t *testing.T) {
	account := openCodeGoTestAccount(405)
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNChatTestResponse())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://opencode.ai/zen/go/v1/chat/completions", upstream.requests[0].URL.String())
	require.True(t, strings.Contains(recorder.Body.String(), DefaultOpenCodeGoTestModel))
}

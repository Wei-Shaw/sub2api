//go:build unit

package handler

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// poolGeminiAccount 是没有 base_url 的 Gemini API Key 账号：若误入 Anthropic 协议
// 转发链，GetBaseURL 会回落 api.anthropic.com，Google 凭据被发往错误主机。
func poolGeminiAccount(id int64, groupID int64, priority int) *service.Account {
	return &service.Account{
		ID: id, Name: fmt.Sprintf("gemini-%d", id), Platform: service.PlatformGemini,
		Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true,
		Concurrency: 1, Priority: priority,
		AccountGroups: []service.AccountGroup{{AccountID: id, GroupID: groupID}},
		Credentials: map[string]any{
			"api_key":       "google-secret-key",
			"model_mapping": map[string]any{"my-model": "gemini-3.1-pro"},
		},
	}
}

func requireNoAnthropicUpstreamCall(t *testing.T, h *compositePoolMessagesHarness) {
	t.Helper()
	require.Empty(t, h.gwUpstream.snapshot(), "Gemini 凭据不得经 Anthropic 协议链发出")
	require.Empty(t, h.openAIUpstream.snapshot())
}

// /v1/messages 池选中 Gemini 账号：交 Gemini 兼容链，不得走 generic Anthropic Forward。
// 夹具未装配 Gemini 兼容服务，请求以错误结束，关键是没有任何 Anthropic 上游请求。
func TestCompositePoolMessagesGeminiAccountNeverUsesAnthropicChain(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{poolGeminiAccount(43101, 41011, 0)})

	c, rec := h.newRequest(t, "/v1/messages", poolMessagesBody(), service.PlatformAnthropic, service.PlatformGemini)
	h.handler.Messages(c)

	require.NotEqual(t, http.StatusOK, rec.Code)
	requireNoAnthropicUpstreamCall(t, h)
}

// /v1/responses 没有 Gemini 转换链：池里只有 Gemini 账号时整平台屏蔽后无号可选，
// 请求失败且没有任何上游请求。
func TestCompositePoolResponsesNeverSendsGeminiCredentials(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{poolGeminiAccount(43201, 41011, 0)})

	c, rec := h.newRequest(t, "/v1/responses", poolResponsesBody(), service.PlatformAnthropic, service.PlatformGemini)
	h.handler.Responses(c)

	require.NotEqual(t, http.StatusOK, rec.Code)
	requireNoAnthropicUpstreamCall(t, h)
}

// 候选平台全部是 Gemini（其余平台已被屏蔽）时明确拒绝，而不是报账号耗尽。
func TestCompositePoolResponsesRejectsWhenOnlyGeminiCandidatesRemain(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{poolGeminiAccount(43251, 41011, 0)})

	c, rec := h.newRequest(t, "/v1/responses", poolResponsesBody(), service.PlatformGemini)
	h.handler.Responses(c)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "/v1/responses")
	requireNoAnthropicUpstreamCall(t, h)
}

// /v1/responses 池同时有 Gemini 与 Anthropic 账号：屏蔽 Gemini 后改选 Anthropic 账号。
func TestCompositePoolResponsesSkipsGeminiAndUsesOtherPlatform(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{
		poolGeminiAccount(43301, 41011, 0),
		poolAnthropicAccount(43302, 41011, 1),
	})
	h.gwUpstream.respond = func(compositePoolUpstreamCall, int) *http.Response {
		return anthropicMessagesSSEStreamResponse()
	}

	c, rec := h.newRequest(t, "/v1/responses", poolResponsesBody(), service.PlatformAnthropic, service.PlatformGemini)
	h.handler.Responses(c)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	calls := h.gwUpstream.snapshot()
	require.Len(t, calls, 1)
	require.NotContains(t, calls[0].Authorization, "google-secret-key")
	selected, ok := c.Get(opsAccountIDKey)
	require.True(t, ok)
	require.EqualValues(t, 43302, selected)
}

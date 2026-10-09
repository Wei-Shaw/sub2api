//go:build unit

package routes

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 跨族账号池上的 Responses 续链：归属账号必然是 OpenAI 兼容族（只有 OpenAI 网关链
// 创建续链绑定），候选池收窄到兼容族后交 OpenAI handler 按归属钉住并校验租户；
// 首轮经 generic 池委派创建的响应同样记录了下游归属，续链可继续。
func TestCompositeCrossFamilyPoolContinuationPinsOwner(t *testing.T) {
	upstream := &captureUpstream{}
	deepseek := newPoolPolicyAccountWithMapping(4301, service.PlatformDeepseek, 0, map[string]any{"xpool-model": "deepseek-chat"})
	deepseek.Credentials["api_protocol"] = service.APIProtocolResponses
	anthropic := newPoolPolicyAccountWithMapping(4302, service.PlatformAnthropic, 5, map[string]any{"xpool-model": "claude-sonnet-4-5"})
	router, _ := newCompositePoolPolicyTestRouter(t, poolPolicyRouterInput{
		group:    newPoolPolicyGroup(nil),
		accounts: []service.Account{deepseek, anthropic},
		upstream: upstream,
	})

	first := postPoolPolicyRequest(t, router, "/v1/responses",
		`{"model":"xpool-model","input":"hi","prompt_cache_key":"sess-x-a","stream":false}`)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	responseID := gjson.GetBytes(first.Body.Bytes(), "id").String()
	require.True(t, strings.HasPrefix(responseID, "resp_"), "got %q", responseID)
	captured := upstream.capturedSnapshot()
	require.Len(t, captured, 1)
	require.Equal(t, "deepseek-pool-fake.test", captured[0].host)

	before := len(upstream.capturedSnapshot())
	second := postPoolPolicyRequest(t, router, "/v1/responses",
		`{"model":"xpool-model","input":"continue","prompt_cache_key":"sess-x-b","previous_response_id":"`+responseID+`","stream":false}`)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	after := upstream.capturedSnapshot()[before:]
	require.NotEmpty(t, after)
	for _, record := range after {
		require.Equal(t, "deepseek-pool-fake.test", record.host, "continuation must stay on the owning account")
	}
}

// 续链请求里优先级更高的 Anthropic 账号绝不能收到带 previous_response_id 的请求。
func TestCompositeCrossFamilyPoolContinuationNeverReachesNonOpenAIAccount(t *testing.T) {
	upstream := &captureUpstream{}
	deepseek := newPoolPolicyAccountWithMapping(4311, service.PlatformDeepseek, 5, map[string]any{
		"ds-only":     "deepseek-chat",
		"xpool-model": "deepseek-chat",
	})
	deepseek.Credentials["api_protocol"] = service.APIProtocolResponses
	anthropic := newPoolPolicyAccountWithMapping(4312, service.PlatformAnthropic, 0, map[string]any{"xpool-model": "claude-sonnet-4-5"})
	router, _ := newCompositePoolPolicyTestRouter(t, poolPolicyRouterInput{
		group:    newPoolPolicyGroup(nil),
		accounts: []service.Account{deepseek, anthropic},
		upstream: upstream,
	})

	first := postPoolPolicyRequest(t, router, "/v1/responses",
		`{"model":"ds-only","input":"hi","prompt_cache_key":"sess-y-a","stream":false}`)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	responseID := gjson.GetBytes(first.Body.Bytes(), "id").String()
	require.True(t, strings.HasPrefix(responseID, "resp_"), "got %q", responseID)

	before := len(upstream.capturedSnapshot())
	second := postPoolPolicyRequest(t, router, "/v1/responses",
		`{"model":"xpool-model","input":"continue","prompt_cache_key":"sess-y-b","previous_response_id":"`+responseID+`","stream":false}`)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	for _, record := range upstream.capturedSnapshot()[before:] {
		require.Equal(t, "deepseek-pool-fake.test", record.host)
	}
}

// 候选池没有 OpenAI 兼容族时续链无法继续：入口明确拒绝，不转发任何请求。
func TestCompositePoolContinuationWithoutOpenAICompatibleCandidateRejected(t *testing.T) {
	upstream := &captureUpstream{}
	router, _ := newCompositePoolPolicyTestRouter(t, poolPolicyRouterInput{
		group: newPoolPolicyGroup(nil),
		accounts: []service.Account{
			newPoolPolicyAccountWithMapping(4321, service.PlatformAnthropic, 0, map[string]any{"np-model": "claude-sonnet-4-5"}),
			newPoolPolicyAccountWithMapping(4322, service.PlatformGemini, 1, map[string]any{"np-model": "gemini-3.1-pro"}),
		},
		upstream: upstream,
	})

	w := postPoolPolicyRequest(t, router, "/v1/responses",
		`{"model":"np-model","input":"continue","previous_response_id":"resp_abc","stream":false}`)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "previous_response_id")
	require.Empty(t, upstream.capturedSnapshot())
}

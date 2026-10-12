//go:build unit

package handler

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 跨族池委派 OpenAI 网关链时，上游 cyber_policy 拒绝须与 OpenAI handler 同口径做事后
// 记录（会话封禁 / 风控事件 / 出错用量行），且透传的上游错误体不再被 generic 兜底
// 追加错误帧。
func TestCompositePoolResponsesDelegatedCyberPolicyIsRecorded(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{poolKimiResponsesAccount(41401, 41011, 0)})
	h.openAIUpstream.respond = func(compositePoolUpstreamCall, int) *http.Response {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"cyber_policy","message":"flagged","type":"invalid_request_error"}}`)),
		}
	}

	c, rec := h.newRequest(t, "/v1/responses", poolResponsesBody(), service.PlatformAnthropic, service.PlatformKimi)
	h.handler.Responses(c)

	require.NotNil(t, service.GetOpsCyberPolicy(c), "委派链必须标记 cyber 命中")
	require.True(t, c.GetBool(cyberPolicyRecordedKey), "generic 池委派路径必须执行 cyber 事后记录")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 1, strings.Count(rec.Body.String(), "cyber_policy"), "上游错误体原样透传一次，不追加兜底错误")
}

// 委派 OpenAI 网关链的流式 response.failed 已由该链透传给客户端：generic handler 不得
// 再追加一个终止错误帧（客户端会收到两个终止事件）。
func TestCompositePoolResponsesDelegatedStreamFailureNotDoubleTerminated(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{poolOpenAIAccount(41402, 41011, 0)})
	h.openAIUpstream.respond = func(compositePoolUpstreamCall, int) *http.Response {
		// 先有语义输出再失败：已写出内容后不可 failover，委派链透传 response.failed。
		sse := `data: {"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"partial"}` + "\n\n" +
			`data: {"type":"response.failed","response":{"id":"resp_pool_failed","object":"response","status":"failed","error":{"code":"server_error","message":"boom"}}}` + "\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(sse)),
		}
	}

	body := `{"model":"my-model","stream":true,"input":"hello"}`
	c, rec := h.newRequest(t, "/v1/responses", body, service.PlatformAnthropic, service.PlatformOpenAI)
	h.handler.Responses(c)

	require.Equal(t, 1, strings.Count(rec.Body.String(), "response.failed"),
		"response.failed 只能出现一次，body=%s", rec.Body.String())
}

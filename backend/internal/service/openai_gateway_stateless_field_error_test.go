//go:build unit

package service

// conversation 400 响应体洁净度回归。
//
// respondOpenAIStatelessFieldError 由服务层直接写出 400 invalid_request_error，但
// 其错误消息（"conversation is not supported ..."）不在 handler 侧
// openAIForwardErrorAlreadyCommunicated 识别的两条消息前缀内：不置位
// ResponseCommitted 的话，Forward 返回错误后 handler 走 ensureForwardErrorResponse，
// 因 Writer.Written() 已为 true 而向完整 400 JSON body 尾部追加 SSE 终止帧
// （responses 入站追加 response.failed），body 变脏、JSON 不完整。
// 修复：写前 MarkResponseCommitted（同 cyber_policy / grok 与 ollama_cloud 定价
// 预检 400 的既有先例），handler 检测到响应已提交便不再追加。

import (
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func TestConversationStatelessFieldError400BodyStaysClean(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{err: errors.New("must not reach upstream")}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	// adaptive 协议 → UsesNativeCNResponses 为真，normalizeDeepSeekResponsesRequestBody 生效。
	account := adaptiveProtocolTestAccount(PlatformOllamaCloud, nil)

	body := []byte(`{"model":"gpt-oss:120b-cloud","conversation":"conv_x","input":"hi","store":true,"stream":false}`)
	c, rec, ctx := pricingPreflightContext("/v1/responses", body, nil)
	_, err := svc.Forward(ctx, c, account, body)

	require.Error(t, err)
	require.Nil(t, upstream.lastReq, "conversation 400 必须发生在任何上游 I/O 之前")
	require.Equal(t, http.StatusBadRequest, rec.Code)

	raw := rec.Body.String()
	require.Equal(t, "invalid_request_error", gjson.Get(raw, "error.type").String())
	require.Contains(t, raw, "conversation is not supported")

	// body 洁净度（核心验收断言）：服务层写出的 400 不得被追加任何 SSE 帧。
	require.NotContains(t, raw, "response.failed", "400 JSON body 不得被追加 response.failed SSE 帧")
	require.NotContains(t, raw, "data:", "400 JSON body 不得被追加任何 SSE 帧")
	require.NotContains(t, raw, "Upstream request failed", "handler fallback 错误不得混入 400 body")
	require.True(t, gjson.Parse(raw).IsObject(), "400 body 必须是单个完整 JSON 对象")

	// 机制断言：响应已提交标记必须置位 —— ensureForwardErrorResponse 的第一道检查
	// 读它，置位即跳过追加 fallback（与定价预检 400 的验收口径一致）。
	require.True(t, IsResponseCommitted(c), "ResponseCommitted 必须置位，handler 才不会追加 fallback 错误")
}

// 三入站路径覆盖：conversation 是 Responses 协议字段，只有 Responses 入站能携带。
// CC / Messages 入站的转换分支构造全新的 Responses body（协议里不存在该字段，
// 转换不会透传未知键），因此不会触发该 400 —— 这里用真实转换路径实证，而非假定：
// 强制原生 Responses 出站后，CC / Messages 各自转换出的 outbound body 均无
// conversation，请求正常走到上游（不存在 400 短路）；Responses 入站为对照组，
// 同字段直接 400。
func TestConversationStateless400OnlyReachableFromResponsesIngress(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newForcedResponsesAccount := func() *Account {
		account := adaptiveProtocolTestAccount(PlatformDeepseek, nil)
		account.Credentials["api_protocol"] = APIProtocolResponses
		account.Credentials["base_url"] = "http://responses.example"
		return account
	}

	// CC / Messages 入站：conversation 被转换丢弃，请求照常出站。
	for _, tc := range cnProtocolIngressCases() {
		if tc.name == "responses" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

			// 向入站 body 硬塞 conversation（客户端异常/恶意形态）。
			body, err := sjson.SetBytes(tc.body, "conversation", "conv_x")
			require.NoError(t, err)

			forwardErr := tc.forward(svc, adaptiveProtocolTestContext(tc.path, body), newForcedResponsesAccount(), body)

			// 到达上游（transport 短路错误）= 未被 conversation 400 拦截。
			var statelessErr *openAIResponsesStatelessFieldError
			require.False(t, errors.As(forwardErr, &statelessErr),
				"%s 入站不得触发 conversation 400", tc.name)
			require.NotNil(t, upstream.lastReq, "%s 入站必须照常出站", tc.name)
			require.False(t, gjson.GetBytes(upstream.lastBody, "conversation").Exists(),
				"%s 入站转换出的 Responses body 不得携带 conversation 字段（即该入站永远到不了这个 400）", tc.name)
		})
	}

	// Responses 入站（对照）：conversation 直达 normalize，显式 400。
	t.Run("responses", func(t *testing.T) {
		upstream := &httpUpstreamRecorder{err: errors.New("must not reach upstream")}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

		body := []byte(`{"model":"deepseek-v4.1-flash","conversation":"conv_x","input":"hi","stream":false}`)
		c, rec, ctx := pricingPreflightContext("/v1/responses", body, nil)
		_, err := svc.Forward(ctx, c, newForcedResponsesAccount(), body)

		var statelessErr *openAIResponsesStatelessFieldError
		require.ErrorAs(t, err, &statelessErr)
		require.Nil(t, upstream.lastReq)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.True(t, IsResponseCommitted(c))
	})
}

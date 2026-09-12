//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// conversationGuardRecorder 保留 *httptest.ResponseRecorder 的引用，以便在 Forward
// 返回后断言 client-visible 状态码与响应体。
type conversationGuardRecorder struct {
	rec *httptest.ResponseRecorder
	c   *gin.Context
}

func newConversationGuardRecorder(path string, body []byte) *conversationGuardRecorder {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return &conversationGuardRecorder{rec: rec, c: c}
}

// TestForwardConversationFieldRejectedWithBadRequest 验证 D2/C37′ 的端到端语义：
// 原生 Responses 账号（ollama_cloud adaptive / deepseek responses）的入站 body 携带
// conversation 时，网关显式应答 400 invalid_request_error（client-visible 状态码断
// 言），不向上游转发、不触发账号 failover；body 不携带 conversation 时同一账号正常
// 出站（证明 400 只由 conversation 触发，而非签名改动引入的通用错误）。
func TestForwardConversationFieldRejectedWithBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-oss:120b-cloud","conversation":"conv_x","input":"hi","stream":false}`)

	t.Run("ollama_cloud adaptive gets 400 and no upstream call", func(t *testing.T) {
		upstream := &httpUpstreamRecorder{err: errors.New("must not reach upstream")}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
		account := adaptiveProtocolTestAccount(PlatformOllamaCloud, nil)

		recorder := newConversationGuardRecorder("/v1/responses", body)
		_, err := svc.Forward(context.Background(), recorder.c, account, body)
		require.Error(t, err)
		require.Nil(t, upstream.lastReq, "conversation 请求不得出站")
		require.Equal(t, http.StatusBadRequest, recorder.rec.Code, "client-visible 状态码必须是 400")
		require.Contains(t, gjson.Get(recorder.rec.Body.String(), "error.type").String(), "invalid_request_error")
		require.Contains(t, recorder.rec.Body.String(), "conversation")
	})

	t.Run("deepseek responses gets 400 too", func(t *testing.T) {
		upstream := &httpUpstreamRecorder{err: errors.New("must not reach upstream")}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
		account := adaptiveProtocolTestAccount(PlatformDeepseek, nil)
		account.Credentials["api_protocol"] = APIProtocolResponses

		recorder := newConversationGuardRecorder("/v1/responses", body)
		_, err := svc.Forward(context.Background(), recorder.c, account, body)
		require.Error(t, err)
		require.Nil(t, upstream.lastReq)
		require.Equal(t, http.StatusBadRequest, recorder.rec.Code)
		require.Contains(t, recorder.rec.Body.String(), "conversation")
	})

	t.Run("same account without conversation is forwarded", func(t *testing.T) {
		upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
		account := adaptiveProtocolTestAccount(PlatformOllamaCloud, nil)

		cleanBody := []byte(`{"model":"gpt-oss:120b-cloud","input":"hi","store":true,"stream":false}`)
		_, err := svc.Forward(context.Background(), adaptiveProtocolTestContext("/v1/responses", cleanBody), account, cleanBody)
		require.Error(t, err)
		require.NotNil(t, upstream.lastReq)
		require.Equal(t, "https://ollama.com/v1/responses", upstream.lastReq.URL.String())
		require.False(t, gjson.GetBytes(upstream.lastBody, "store").Bool())
	})
}

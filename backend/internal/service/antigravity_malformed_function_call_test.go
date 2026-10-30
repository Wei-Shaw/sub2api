//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

// antigravityMalformedStreamLine 模拟上游在 MALFORMED_FUNCTION_CALL 时返回的流：
// candidate 只有一个 thoughtSignature，没有 text / functionCall。
const antigravityMalformedStreamLine = `data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"thoughtSignature":"sig-abc"}]},"finishReason":"MALFORMED_FUNCTION_CALL"}]}}`

type malformedFnTempUnschedCall struct {
	accountID int64
	until     time.Time
	reason    string
}

type malformedFnTempUnschedRepo struct {
	AccountRepository
	calls []malformedFnTempUnschedCall
}

func (r *malformedFnTempUnschedRepo) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	r.calls = append(r.calls, malformedFnTempUnschedCall{accountID: id, until: until, reason: reason})
	return nil
}

func TestAntigravityCompatMalformedFunctionCallIsRequestScoped(t *testing.T) {
	processor := antigravity.NewStreamingProcessor("gemini-3.8-flash-high")
	events := processor.ProcessLine(antigravityMalformedStreamLine)
	require.Contains(t, string(events), "message_start")
	require.True(t, processor.MalformedFunctionCall())
	require.False(t, processor.HasContent(), "仅 thoughtSignature 的残片不应视为有效内容")
	require.Equal(t, "gemini-3.8-flash-high", processor.Model())

	normal := antigravity.NewStreamingProcessor("gemini-3.8-flash-high")
	normal.ProcessLine(`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]},"finishReason":"STOP"}]}}`)
	require.False(t, normal.MalformedFunctionCall())

	session := &antigravityCompatStreamSession{processor: processor}
	err := session.emptyStreamFailoverError()
	failoverErr, ok := err.(*UpstreamFailoverError)
	require.True(t, ok)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.True(t, failoverErr.RetryableOnSameAccount, "允许请求级同号重试")
	require.True(t, failoverErr.RequestScopedTransient, "MALFORMED_FUNCTION_CALL 不得归因到账号")
	require.Equal(t, GatewayFailureScopeRequest, failoverErr.Scope)
	require.Contains(t, string(failoverErr.ResponseBody), "MALFORMED_FUNCTION_CALL")

	repo := &malformedFnTempUnschedRepo{}
	svc := &GatewayService{accountRepo: repo}
	svc.TempUnscheduleRetryableError(context.Background(), 12, failoverErr)
	require.Empty(t, repo.calls, "请求/模型侧失败不得对账号临时封禁")
}

func TestAntigravityCompatMalformedStreamFinishReturnsRequestScopedError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", nil)
	writer := newAntigravityClientWriter(c.Writer, c.Writer, "test")
	session := newAntigravityCompatStreamSession(
		"gemini-3.8-flash-high",
		time.Now(),
		newAntigravityChatStreamAdapter("gemini-3.8-flash-high", false),
		writer,
	)
	session.consume(antigravityMalformedStreamLine)

	result, err := session.finish()
	require.Nil(t, result)
	failoverErr, ok := err.(*UpstreamFailoverError)
	require.True(t, ok)
	require.True(t, failoverErr.RequestScopedTransient)
	require.Equal(t, GatewayFailureReason("antigravity_malformed_function_call"), failoverErr.Reason)
}

func TestAntigravityCompatGenericEmptyStreamStillTempsUnschedules(t *testing.T) {
	session := &antigravityCompatStreamSession{processor: antigravity.NewStreamingProcessor("gemini-3.8-flash-high")}
	err := session.emptyStreamFailoverError()
	failoverErr, ok := err.(*UpstreamFailoverError)
	require.True(t, ok)
	require.False(t, failoverErr.RequestScopedTransient, "普通空流仍按账号健康处理")
	require.Equal(t, "empty_stream", session.emptyStreamReason())

	repo := &malformedFnTempUnschedRepo{}
	svc := &GatewayService{accountRepo: repo}
	svc.TempUnscheduleRetryableError(context.Background(), 12, failoverErr)
	require.Len(t, repo.calls, 1)
	require.Contains(t, repo.calls[0].reason, "empty stream response")
}

// 上游有响应、但只输出了 thinking/签名帧（例如思考吃满输出预算）时，账号本身可用，
// 应作为请求级失败重试，不得对账号临时封禁。
func TestAntigravityCompatThinkingOnlyStreamIsRequestScoped(t *testing.T) {
	processor := antigravity.NewStreamingProcessor("gemini-3.8-flash-high")
	processor.ProcessLine(`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"thoughtSignature":"sig-only"}]},"finishReason":"STOP"}]}}`)
	require.True(t, processor.SawUpstreamResponse())
	require.True(t, processor.SawThinking())

	session := &antigravityCompatStreamSession{processor: processor}
	err := session.emptyStreamFailoverError()
	failoverErr, ok := err.(*UpstreamFailoverError)
	require.True(t, ok)
	require.True(t, failoverErr.RequestScopedTransient, "仅思考/签名帧不得归因到账号")
	require.Equal(t, GatewayFailureScopeRequest, failoverErr.Scope)
	require.Equal(t, GatewayFailureReason("antigravity_empty_stream"), failoverErr.Reason)
	require.Equal(t, "thinking_without_content", session.emptyStreamReason())

	repo := &malformedFnTempUnschedRepo{}
	svc := &GatewayService{accountRepo: repo}
	svc.TempUnscheduleRetryableError(context.Background(), 12, failoverErr)
	require.Empty(t, repo.calls, "请求级空流不得对账号临时封禁")
}

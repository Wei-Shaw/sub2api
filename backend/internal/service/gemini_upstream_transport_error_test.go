//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Gemini 传输层失败（Do 返回非 HTTP 错误）的契约，三条转发路径一致：
//   - 不在同账号上重试，单次尝试后立即返回 *UpstreamFailoverError(502) 交给 handler 换号；
//   - service 不写客户端响应；
//   - 瞬时故障不摘号，持久性故障临时摘号；
//   - 客户端断开原样返回，不换号、不摘号；
//   - countTokens 以本地估算兜底。
// ---------------------------------------------------------------------------

const geminiTransportTestNativeBody = `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`

func newGeminiTransportErrorService(upstreamErr error) (*GeminiMessagesCompatService, *geminiCompatHTTPUpstreamStub, *transportTempUnschedRepoStub) {
	httpStub := &geminiCompatHTTPUpstreamStub{err: upstreamErr}
	repo := &transportTempUnschedRepoStub{}
	svc := &GeminiMessagesCompatService{
		accountRepo:  repo,
		httpUpstream: httpStub,
		cfg:          &config.Config{},
	}
	return svc, httpStub, repo
}

func requireGeminiTransportFailover(t *testing.T, err error) *UpstreamFailoverError {
	t.Helper()
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "传输层错误应换号，got %T: %v", err, err)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Equal(t, string(geminiTransportFailoverBody), string(failoverErr.ResponseBody))
	require.True(t, failoverErr.ShouldRetryNextAccount())
	require.False(t, failoverErr.RetryableOnSameAccount, "传输层错误不做同账号重试")
	return failoverErr
}

func TestGeminiForwardNative_TransientTransportErrorFailsOverWithoutRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, httpStub, repo := newGeminiTransportErrorService(errors.New(`Post "https://upstream/v1beta/models/gemini-2.5-flash:generateContent": EOF`))
	c, rec := newGeminiNativeTestContext(t)

	start := time.Now()
	result, err := svc.ForwardNative(context.Background(), c, geminiPoolModeAPIKeyAccount(),
		"gemini-2.5-flash", "generateContent", false, []byte(geminiTransportTestNativeBody))

	require.Nil(t, result)
	requireGeminiTransportFailover(t, err)
	require.Equal(t, 1, httpStub.calls, "不应在同账号上重试")
	require.Less(t, time.Since(start), geminiRetryBaseDelay, "不应进入退避等待")
	require.Zero(t, repo.calls, "瞬时故障不摘号")
	require.False(t, c.Writer.Written(), "换号场景不应写客户端响应")
	require.Zero(t, rec.Body.Len())

	raw, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok, "应记录 ops 上游错误事件")
	events := raw.([]*OpsUpstreamErrorEvent)
	require.Len(t, events, 1)
	require.Equal(t, "request_error", events[0].Kind)
	require.Equal(t, int64(700), events[0].AccountID)
	require.Zero(t, events[0].UpstreamStatusCode)
}

func TestGeminiForwardNative_PersistentTransportErrorEvictsAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, httpStub, repo := newGeminiTransportErrorService(errors.New(`dial tcp 1.2.3.4:443: connect: connection refused`))
	c, _ := newGeminiNativeTestContext(t)
	account := geminiPoolModeAPIKeyAccount()

	before := time.Now()
	_, err := svc.ForwardNative(context.Background(), c, account,
		"gemini-2.5-flash", "streamGenerateContent", true, []byte(geminiTransportTestNativeBody))

	requireGeminiTransportFailover(t, err)
	require.Equal(t, 1, httpStub.calls)
	require.Equal(t, 1, repo.calls, "持久性故障应临时摘号")
	require.Equal(t, account.ID, repo.lastID)
	require.WithinDuration(t, before.Add(gatewayTransportErrorTempUnschedDuration), repo.lastUntil, 5*time.Second)
	require.False(t, c.Writer.Written())
}

func TestGeminiForwardNative_ClientCanceledReturnsOriginalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, httpStub, repo := newGeminiTransportErrorService(context.Canceled)
	c, _ := newGeminiNativeTestContext(t)

	_, err := svc.ForwardNative(context.Background(), c, geminiPoolModeAPIKeyAccount(),
		"gemini-2.5-flash", "generateContent", false, []byte(geminiTransportTestNativeBody))

	require.ErrorIs(t, err, context.Canceled)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "客户端断开不应换号")
	require.Equal(t, 1, httpStub.calls)
	require.Zero(t, repo.calls, "客户端断开不摘号")
	require.False(t, c.Writer.Written())
}

func TestGeminiForwardNative_CountTokensTransportErrorFallsBackToEstimate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, httpStub, _ := newGeminiTransportErrorService(errors.New("EOF"))
	c, rec := newGeminiNativeTestContext(t)

	result, err := svc.ForwardNative(context.Background(), c, geminiPoolModeAPIKeyAccount(),
		"gemini-2.5-flash", "countTokens", false, []byte(geminiTransportTestNativeBody))

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, httpStub.calls)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"totalTokens"`)
}

func TestGeminiForward_TransportErrorFailsOverWithoutRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, httpStub, repo := newGeminiTransportErrorService(errors.New("read tcp 10.0.0.1:1234->1.2.3.4:443: read: connection reset by peer"))
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))

	result, err := svc.Forward(context.Background(), c, geminiPoolModeAPIKeyAccount(), body)

	require.Nil(t, result)
	requireGeminiTransportFailover(t, err)
	require.Equal(t, 1, httpStub.calls, "不应在同账号上重试")
	require.Zero(t, repo.calls)
	require.False(t, c.Writer.Written(), "换号场景不应写客户端响应")
}

func TestGeminiForwardAsChatCompletions_TransportErrorFailsOverWithoutRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, httpStub, repo := newGeminiTransportErrorService(errors.New("EOF"))
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hi"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, geminiPoolModeAPIKeyAccount(), body)

	require.Nil(t, result)
	requireGeminiTransportFailover(t, err)
	require.Equal(t, 1, httpStub.calls, "不应在同账号上重试")
	require.Zero(t, repo.calls)
	require.False(t, c.Writer.Written(), "换号场景不应写客户端响应")
}

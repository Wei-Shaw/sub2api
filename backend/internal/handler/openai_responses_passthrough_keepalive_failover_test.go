//go:build unit

package handler

// /v1/responses 透传账号的下游心跳换号回归。
//
// 透传流式路径（handleStreamingResponsePassthrough）在每次尝试进入流式循环时
// 都会新建一个 SSE 心跳器。第一次尝试真实写出心跳后遇到可换号的 event: error，
// 第二个账号立即以同样的错误失败，第三个账号成功：前一次尝试写出的心跳字节
// 不能被当成语义输出，否则第二次尝试会放弃换号，把错误直接写给客户端。

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const openAIPassthroughKeepaliveSuccessSSE = "event: response.created\n" +
	`data: {"type":"response.created","response":{"id":"resp_passthrough_ok","status":"in_progress","output":[]}}` + "\n\n" +
	"event: response.completed\n" +
	`data: {"type":"response.completed","response":{"id":"resp_passthrough_ok","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}` + "\n\n"

const openAIPassthroughKeepaliveOverloadedSSE = "event: error\n" +
	`data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded","status":529}}` + "\n\n"

type openAIPassthroughKeepaliveUpstream struct {
	service.HTTPUpstream
	mu        sync.Mutex
	calls     []int64
	responses []*http.Response
}

func (u *openAIPassthroughKeepaliveUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.calls = append(u.calls, accountID)
	if len(u.responses) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	resp := u.responses[0]
	u.responses = u.responses[1:]
	return resp, nil
}

func (u *openAIPassthroughKeepaliveUpstream) callIDs() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.calls...)
}

// openAIPassthroughKeepaliveWriter 在第一拍心跳写出时通知测试，测试据此
// 推进上游，不依赖 sleep 时序。
type openAIPassthroughKeepaliveWriter struct {
	gin.ResponseWriter
	once   sync.Once
	notify chan struct{}
}

func (w *openAIPassthroughKeepaliveWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if bytes.Contains(p, []byte(": keepalive\n\n")) {
		w.once.Do(func() { close(w.notify) })
	}
	return n, err
}

func newOpenAIPassthroughKeepaliveHandler(t *testing.T, upstream service.HTTPUpstream) *OpenAIGatewayHandler {
	t.Helper()
	newAccount := func(id int64, priority int) service.Account {
		return service.Account{
			ID: id, Name: "passthrough-keepalive", Platform: service.PlatformOpenAI,
			Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true,
			Concurrency: 1, Priority: priority,
			Credentials: map[string]any{"access_token": "test-token"},
			Extra:       map[string]any{"openai_passthrough": true},
		}
	}
	accounts := []service.Account{newAccount(1, 0), newAccount(2, 1), newAccount(3, 2)}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.StreamKeepaliveInterval = 1
	gatewayService := service.NewOpenAIGatewayService(
		openAIImagesFailoverAccountRepo{accounts: accounts}, nil, nil, nil, nil, nil, nil, cfg,
		nil, nil, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	h := NewOpenAIGatewayHandler(
		gatewayService,
		service.NewConcurrencyService(nil),
		billing,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil, nil, nil, nil, cfg,
	)
	h.maxAccountSwitches = 2
	return h
}

func newOpenAIPassthroughKeepaliveContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder, *openAIPassthroughKeepaliveWriter) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.1","stream":true,"input":"hello"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	groupID := int64(3135)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID: 99, GroupID: &groupID,
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI},
		User:  &service.User{ID: 100},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 100})
	writer := &openAIPassthroughKeepaliveWriter{ResponseWriter: c.Writer, notify: make(chan struct{})}
	c.Writer = writer
	return c, rec, writer
}

func TestOpenAIGatewayHandlerResponsesPassthroughKeepaliveFailoverAcrossAttempts(t *testing.T) {
	firstBody, firstWriter := io.Pipe()
	sseHeader := func() http.Header { return http.Header{"Content-Type": []string{"text/event-stream"}} }
	upstream := &openAIPassthroughKeepaliveUpstream{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: sseHeader(), Body: firstBody},
		{StatusCode: http.StatusOK, Header: sseHeader(), Body: io.NopCloser(strings.NewReader(openAIPassthroughKeepaliveOverloadedSSE))},
		{StatusCode: http.StatusOK, Header: sseHeader(), Body: io.NopCloser(strings.NewReader(openAIPassthroughKeepaliveSuccessSSE))},
	}}
	h := newOpenAIPassthroughKeepaliveHandler(t, upstream)
	c, rec, writer := newOpenAIPassthroughKeepaliveContext(t)

	done := make(chan struct{})
	go func() { h.Responses(c); close(done) }()
	select {
	case <-writer.notify:
	case <-done:
		t.Fatalf("handler 在第一拍心跳前返回: calls=%v body=%q", upstream.callIDs(), rec.Body.String())
	case <-time.After(15 * time.Second):
		t.Fatal("等待透传流第一拍心跳超时")
	}
	_, err := io.WriteString(firstWriter, openAIPassthroughKeepaliveOverloadedSSE)
	require.NoError(t, err)
	require.NoError(t, firstWriter.Close())
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("handler 未在后续尝试完成后返回")
	}

	body := rec.Body.String()
	require.Equal(t, []int64{1, 2, 3}, upstream.callIDs(), "前一次尝试的心跳字节不得阻止第二次尝试换号: body=%q", body)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, body, ": keepalive\n\n", "第一次尝试必须真实写出心跳")
	require.Equal(t, 1, strings.Count(body, "event: response.completed"), "成功流只允许一个 completed")
	require.NotContains(t, body, "response.failed", "可换号的首输出前错误不得写给客户端")
	require.NotContains(t, body, "Overloaded", "可换号的首输出前错误不得写给客户端")
}

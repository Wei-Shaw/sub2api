//go:build unit

package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const rawCCTruncationChatBody = `{"model":"deepseek-v4-pro","messages":[{"role":"user","content":"hello"}],"stream":true}`

// 维护者对 #6063 的闭环要求：把「部分输出 + 上游截断」接到 Chat Completions
// handler，验证 usage 只提交一次、错误帧符合协议，并保住无输出截断与既有终态路径。

func TestChatCompletions_TruncatedRawStreamAfterOutput_CommitsUsageOnceAndWritesTypedError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newRawCCTruncationHandler(t, []service.Account{
		rawCCTruncationAccount(8101, 1),
	}, map[int64]string{
		8101: rawCCTruncatedAfterOutputSSE(),
	})

	rec, c := h.chatCompletions(t, rawCCTruncationChatBody)
	body := rec.Body.String()

	require.Equal(t, http.StatusOK, rec.Code, "流已写出后 HTTP 状态码保持 200，错误只能走 SSE")
	require.Contains(t, body, `"content":"half an ans"`)
	require.NotContains(t, body, "data: [DONE]")
	require.Equal(t, 1, strings.Count(body, "event: error\n"), "类型化错误帧必须且只能补一次")

	errJSON := lastSSEErrorJSON(t, body)
	require.Equal(t, "upstream_error", gjson.Get(errJSON, "error.type").String())
	require.Equal(t, service.OpenAIUpstreamStreamTruncatedCode, gjson.Get(errJSON, "error.code").String())
	require.NotEmpty(t, gjson.Get(errJSON, "error.message").String())

	logs := h.usage.snapshot()
	require.Len(t, logs, 1, "部分输出截断必须提交且只提交一次 usage")
	require.True(t, logs[0].Stream)
	require.Equal(t, "deepseek-v4-pro", logs[0].Model)

	streamErr, ok := service.GetOpsStreamError(c)
	require.True(t, ok, "已提交的流必须记入 SLA 失败")
	require.True(t, streamErr.CountTowardsSLA)
	require.Equal(t, service.OpenAIUpstreamStreamTruncatedCode, streamErr.Code)
	require.Equal(t, http.StatusBadGateway, streamErr.IntendedStatus)

	require.Equal(t, []int64{8101}, h.upstream.snapshotCalls())
}

func TestChatCompletions_EmptyRawStreamBeforeOutput_FailoversWithoutClientBytes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newRawCCTruncationHandler(t, []service.Account{
		rawCCTruncationAccount(8102, 1),
		rawCCTruncationAccount(8103, 2),
	}, map[int64]string{
		8102: "",
		8103: rawCCCompleteSSE("recovered answer", 11, 6),
	})

	rec, c := h.chatCompletions(t, rawCCTruncationChatBody)
	body := rec.Body.String()

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, body, `"content":"recovered answer"`)
	require.Contains(t, body, "data: [DONE]")
	require.NotContains(t, body, "event: error\n", "无输出截断必须在提交响应头前换号，客户端不得看到错误帧")
	_, hasStreamErr := service.GetOpsStreamError(c)
	require.False(t, hasStreamErr)

	logs := h.usage.snapshot()
	require.Len(t, logs, 1, "空流 failover 不得计费；成功的第二号只提交一次")
	require.Equal(t, 11, logs[0].InputTokens)
	require.Equal(t, 6, logs[0].OutputTokens)
	require.Equal(t, []int64{8102, 8103}, h.upstream.snapshotCalls())
}

func TestChatCompletions_MissingDoneWithUsage_SucceedsAndCommitsUsageOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newRawCCTruncationHandler(t, []service.Account{
		rawCCTruncationAccount(8104, 1),
	}, map[int64]string{
		8104: rawCCUsageWithoutDoneSSE(),
	})

	rec, _ := h.chatCompletions(t, rawCCTruncationChatBody)
	body := rec.Body.String()

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, body, `"content":"ok"`)
	require.NotContains(t, body, "event: error\n")
	require.NotContains(t, body, "data: [DONE]")

	logs := h.usage.snapshot()
	require.Len(t, logs, 1)
	require.Equal(t, 11, logs[0].InputTokens)
	require.Equal(t, 6, logs[0].OutputTokens)
}

func TestChatCompletions_MissingDoneWithFinishReason_SucceedsWithoutErrorFrame(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newRawCCTruncationHandler(t, []service.Account{
		rawCCTruncationAccount(8105, 1),
	}, map[int64]string{
		8105: rawCCFinishReasonWithoutDoneSSE(),
	})

	rec, c := h.chatCompletions(t, rawCCTruncationChatBody)
	body := rec.Body.String()

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, body, `"finish_reason":"stop"`)
	require.NotContains(t, body, "event: error\n")
	_, hasStreamErr := service.GetOpsStreamError(c)
	require.False(t, hasStreamErr)

	logs := h.usage.snapshot()
	require.Len(t, logs, 1, "finish_reason 终态仍按成功计费一次")
}

type rawCCTruncationHandler struct {
	handler  *OpenAIGatewayHandler
	usage    *rawCCTruncationUsageLogRepo
	upstream *rawCCTruncationUpstream
	apiKey   *service.APIKey
}

func (h *rawCCTruncationHandler) chatCompletions(t *testing.T, body string) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware2.ContextKeyAPIKey), h.apiKey)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: h.apiKey.User.ID, Concurrency: 0})
	h.handler.ChatCompletions(c)
	return rec, c
}

func newRawCCTruncationHandler(t *testing.T, accounts []service.Account, bodies map[int64]string) *rawCCTruncationHandler {
	t.Helper()
	groupID := int64(6063)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.MaxAccountSwitches = 3
	cfg.Gateway.MaxBodySize = 1 << 20

	usage := &rawCCTruncationUsageLogRepo{}
	upstream := &rawCCTruncationUpstream{bodies: bodies}
	accountRepo := &openAIWSFailoverHandlerAccountRepoStub{accounts: accounts}
	billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheSvc.Stop)

	gatewaySvc := service.NewOpenAIGatewayService(
		accountRepo,
		usage,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		nil,
		nil,
		service.NewBillingService(cfg, nil),
		nil,
		billingCacheSvc,
		upstream,
		&service.DeferredService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	handler := NewOpenAIGatewayHandler(
		gatewaySvc,
		service.NewConcurrencyService(nil),
		billingCacheSvc,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil,
		nil,
		nil,
		nil,
		cfg,
	)
	return &rawCCTruncationHandler{
		handler:  handler,
		usage:    usage,
		upstream: upstream,
		apiKey: &service.APIKey{
			ID:      60631,
			GroupID: &groupID,
			User:    &service.User{ID: 60632, Status: service.StatusActive},
			Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
		},
	}
}

func rawCCTruncationAccount(id int64, priority int) service.Account {
	return service.Account{
		ID:          id,
		Name:        fmt.Sprintf("raw-cc-%d", id),
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Priority:    priority,
		Concurrency: 0,
		Credentials: map[string]any{
			"api_key":  fmt.Sprintf("sk-%d", id),
			"base_url": "https://raw-cc.example.test/v1",
		},
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesSupported: false,
		},
	}
}

func rawCCTruncatedAfterOutputSSE() string {
	return strings.Join([]string{
		`data: {"id":"chatcmpl_cut","object":"chat.completion.chunk","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"content":"half an ans"},"finish_reason":null}]}`,
		"",
	}, "\n")
}

func rawCCCompleteSSE(content string, promptTokens, completionTokens int) string {
	return strings.Join([]string{
		fmt.Sprintf(`data: {"id":"chatcmpl_ok","object":"chat.completion.chunk","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"content":%q},"finish_reason":null}]}`, content),
		"",
		fmt.Sprintf(`data: {"id":"chatcmpl_ok","object":"chat.completion.chunk","model":"deepseek-v4-pro","choices":[],"usage":{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d}}`, promptTokens, completionTokens, promptTokens+completionTokens),
		"",
		"data: [DONE]",
		"",
	}, "\n")
}

func rawCCUsageWithoutDoneSSE() string {
	return strings.Join([]string{
		`data: {"id":"chatcmpl_nodone","object":"chat.completion.chunk","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"content":"ok"}}]}`,
		"",
		`data: {"id":"chatcmpl_nodone","object":"chat.completion.chunk","model":"deepseek-v4-pro","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":6,"total_tokens":17}}`,
		"",
	}, "\n")
}

func rawCCFinishReasonWithoutDoneSSE() string {
	return strings.Join([]string{
		`data: {"id":"chatcmpl_finish","object":"chat.completion.chunk","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"content":"done"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_finish","object":"chat.completion.chunk","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"",
	}, "\n")
}

func lastSSEErrorJSON(t *testing.T, body string) string {
	t.Helper()
	idx := strings.LastIndex(body, "event: error\n")
	require.GreaterOrEqual(t, idx, 0, "missing SSE error event: %s", body)
	payload := body[idx:]
	dataIdx := strings.Index(payload, "data: ")
	require.GreaterOrEqual(t, dataIdx, 0, "SSE error event missing data line: %s", payload)
	line := strings.TrimSpace(strings.TrimPrefix(payload[dataIdx:], "data: "))
	if nl := strings.IndexByte(line, '\n'); nl >= 0 {
		line = strings.TrimSpace(line[:nl])
	}
	require.True(t, gjson.Valid(line), "SSE error data is not JSON: %s", line)
	return line
}

type rawCCTruncationUpstream struct {
	service.HTTPUpstream
	mu     sync.Mutex
	bodies map[int64]string
	calls  []int64
}

func (u *rawCCTruncationUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.calls = append(u.calls, accountID)
	body := u.bodies[accountID]
	u.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"x-request-id": []string{fmt.Sprintf("rid-%d", accountID)},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}, nil
}

func (u *rawCCTruncationUpstream) snapshotCalls() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.calls...)
}

type rawCCTruncationUsageLogRepo struct {
	service.UsageLogRepository
	mu   sync.Mutex
	logs []*service.UsageLog
}

func (r *rawCCTruncationUsageLogRepo) Create(_ context.Context, log *service.UsageLog) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cloned := *log
	r.logs = append(r.logs, &cloned)
	return true, nil
}

func (r *rawCCTruncationUsageLogRepo) snapshot() []*service.UsageLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*service.UsageLog, len(r.logs))
	copy(out, r.logs)
	return out
}

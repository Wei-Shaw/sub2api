//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// A1-03: Chat Completions / Responses 桥接路径必须识别上游 `event: error`：
// 未向客户端输出前转成 UpstreamFailoverError 以便换号；已输出后写协议内的失败终止帧，
// 且仍返回已累计 usage（计费与修复前一致）。

const a103ErrorJSON = `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`

const a103MessageStartAndText = "event: message_start\n" +
	`data: {"type":"message_start","message":{"id":"msg_a103","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-5","usage":{"input_tokens":7}}}` + "\n\n" +
	"event: content_block_start\n" +
	`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
	"event: content_block_delta\n" +
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}` + "\n\n"

type a103Endpoint struct {
	name string
	path string
	call func(*GatewayService, *gin.Context, *Account, []byte) (*ForwardResult, error)
	body func(stream bool) []byte
}

func a103Endpoints() []a103Endpoint {
	return []a103Endpoint{
		{
			name: "chat_completions",
			path: "/v1/chat/completions",
			call: func(s *GatewayService, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
				return s.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
			},
			body: func(stream bool) []byte {
				b, _ := json.Marshal(map[string]any{"model": "claude-sonnet-4-5", "stream": stream, "messages": []map[string]string{{"role": "user", "content": "hello"}}})
				return b
			},
		},
		{
			name: "responses",
			path: "/v1/responses",
			call: func(s *GatewayService, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
				return s.ForwardAsResponses(context.Background(), c, account, body, nil)
			},
			body: func(stream bool) []byte {
				b, _ := json.Marshal(map[string]any{"model": "claude-sonnet-4-5", "stream": stream, "input": "hello"})
				return b
			},
		},
	}
}

func a103Run(t *testing.T, ep a103Endpoint, stream bool, fixture string) (*ForwardResult, error, *httptest.ResponseRecorder, *gatewayForwardErrorPolicyRepoStub) {
	t.Helper()
	repo := &gatewayForwardErrorPolicyRepoStub{}
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	svc := &GatewayService{
		cfg: cfg,
		httpUpstream: &anthropicHTTPUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(fixture)),
		}},
		rateLimitService:    NewRateLimitService(repo, nil, cfg, nil, nil),
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := ep.body(stream)
	c.Request = httptest.NewRequest(http.MethodPost, ep.path, strings.NewReader(string(body)))
	account := &Account{
		ID: 1, Name: "a103", Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key"},
		Status:      StatusActive, Schedulable: true,
	}
	result, err := ep.call(svc, c, account, body)
	return result, err, rec, repo
}

func TestA103BridgeSSEErrorBeforeOutputFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := "event: error\ndata: " + a103ErrorJSON + "\n\n"
	for _, ep := range a103Endpoints() {
		for _, stream := range []bool{true, false} {
			mode := "buffered"
			if stream {
				mode = "streaming"
			}
			t.Run(ep.name+"/"+mode, func(t *testing.T) {
				result, err, rec, repo := a103Run(t, ep, stream, fixture)
				require.Nil(t, result)
				var failoverErr *UpstreamFailoverError
				require.ErrorAs(t, err, &failoverErr)
				require.Equal(t, 529, failoverErr.StatusCode)
				require.JSONEq(t, a103ErrorJSON, string(failoverErr.ResponseBody))
				require.Empty(t, rec.Body.String(), "未输出前必须保持可 failover")
				require.Equal(t, 1, repo.overloadCalls, "未输出前的 overloaded_error 应与原生路径一样触发过载冷却")
			})
		}
	}
}

func TestA103BridgeSSEErrorAfterOutputWritesFailureTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := a103MessageStartAndText + "event: error\ndata: " + a103ErrorJSON + "\n\n"
	for _, ep := range a103Endpoints() {
		t.Run(ep.name, func(t *testing.T) {
			result, err, rec, repo := a103Run(t, ep, true, fixture)
			require.NoError(t, err)
			require.NotNil(t, result, "已输出场景仍需返回 usage，计费保持不变")
			require.Equal(t, 7, result.Usage.InputTokens)
			require.Zero(t, repo.overloadCalls)
			body := rec.Body.String()
			require.Contains(t, body, "partial")
			if ep.name == "responses" {
				require.Contains(t, body, "event: response.failed")
				require.NotContains(t, body, "response.completed")
			} else {
				require.Contains(t, body, `"error"`)
				require.Contains(t, body, "Overloaded")
				require.NotContains(t, body, "[DONE]")
				require.NotContains(t, body, `"finish_reason":"stop"`)
			}
		})
	}
}

func TestA103BridgeBufferedSSEErrorAfterMessageStartReturnsError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := a103MessageStartAndText + "event: error\ndata: " + a103ErrorJSON + "\n\n"
	for _, ep := range a103Endpoints() {
		t.Run(ep.name, func(t *testing.T) {
			result, err, rec, repo := a103Run(t, ep, false, fixture)
			require.NoError(t, err)
			require.NotNil(t, result, "已消耗 message_start usage，计费保持不变")
			require.Equal(t, 7, result.Usage.InputTokens)
			require.Zero(t, repo.overloadCalls)
			require.GreaterOrEqual(t, rec.Code, http.StatusBadRequest, "截断的响应不能以 200 成功返回")
			require.Contains(t, rec.Body.String(), "Overloaded")
		})
	}
}

//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 2026-10-16 实测：上游在 HTTP 200 首帧返回 error，param=access_programs.cyber。
const (
	codexAccessProgramNotEnabledEvent = `{"type":"error","sequence_number":0,"error":{"type":"invalid_request_error","code":"access_program_not_enabled","message":"The daybreak_blue access program is not enabled for this account.","param":"access_programs.cyber"}}`
	codexInvalidAccessProgramEvent    = `{"type":"error","sequence_number":0,"error":{"type":"invalid_request_error","code":"invalid_access_program","message":"Invalid access program: daybreak_red","param":"access_programs.cyber"}}`
)

func TestIsCodexAccessProgramRejection(t *testing.T) {
	tests := map[string]struct {
		payload string
		want    bool
	}{
		"access_program_not_enabled": {codexAccessProgramNotEnabledEvent, true},
		"invalid_access_program":     {codexInvalidAccessProgramEvent, true},
		"response.failed rewrite":    {`{"type":"response.failed","response":{"status":"failed","error":{"code":"access_program_not_enabled","message":"x","param":"access_programs.cyber"}}}`, true},
		"top-level param":            {`{"error":{"code":"invalid_access_program","message":"x"},"param":"access_programs.cyber"}`, true},
		"plain error event":          {`{"type":"error","error":{"type":"server_error","code":"server_error","message":"boom"}}`, false},
		"cyber policy":               {`{"type":"error","error":{"code":"cyber_policy","message":"blocked"}}`, false},
		"other param":                {`{"type":"error","error":{"code":"invalid_request_error","message":"x","param":"service_tier"}}`, false},
		"non-error event with param": {`{"type":"response.created","error":{"param":"access_programs.cyber"}}`, false},
		"invalid json":               {`{"type":"error"`, false},
		"empty":                      {``, false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, isCodexAccessProgramRejection([]byte(tt.payload)))
		})
	}
}

func newCodexRejectionTestService(t *testing.T) (*OpenAIGatewayService, *codexCapabilityProbeAccountRepo, *atomic.Int32) {
	t.Helper()
	body := &atomic.Value{}
	body.Store(codexCapabilitiesStandardAccountManifest)
	calls, _ := newCodexCapabilityProbeServer(t, body)
	account := newCodexCapabilityProbeAccount(31, AccountTypeOAuth)
	repo := &codexCapabilityProbeAccountRepo{accounts: map[int64]*Account{31: account}, updated: make(chan struct{}, 4)}
	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cfg:         &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
	}
	return svc, repo, calls
}

func waitForCodexReprobe(t *testing.T, repo *codexCapabilityProbeAccountRepo) {
	t.Helper()
	select {
	case <-repo.updated:
	case <-time.After(5 * time.Second):
		t.Fatal("expected an asynchronous capability re-probe after the access program rejection")
	}
}

// 首帧访问项目拒绝：事件照常透传（bare error 改写为 response.failed）、不 failover、
// 不触发账号副作用，并对账号异步触发一次重探测；冷却窗口内再次拒绝不再探测。
func TestObserveCodexCapabilityRejection_StreamPathsReprobeOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stream := "event: error\ndata: " + codexAccessProgramNotEnabledEvent + "\n\n"
	paths := []struct {
		name string
		run  func(*OpenAIGatewayService, *gin.Context, *http.Response, *Account) error
	}{
		{name: "native", run: func(svc *OpenAIGatewayService, c *gin.Context, resp *http.Response, account *Account) error {
			_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-6-sol", "gpt-6-sol")
			return err
		}},
		{name: "passthrough", run: func(svc *OpenAIGatewayService, c *gin.Context, resp *http.Response, account *Account) error {
			_, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "gpt-6-sol", "gpt-6-sol")
			return err
		}},
	}
	for _, path := range paths {
		t.Run(path.name, func(t *testing.T) {
			svc, repo, calls := newCodexRejectionTestService(t)
			account := repo.accounts[31]
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(stream)),
			}

			err := path.run(svc, c, resp, account)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr), "访问项目拒绝不 failover")
			require.Contains(t, rec.Body.String(), "access_program_not_enabled", "错误照常透传给客户端")
			require.Contains(t, rec.Body.String(), "response.failed", "既有 bare error → response.failed 改写不变")
			require.Nil(t, GetOpsCyberPolicy(c))
			require.Equal(t, StatusActive, account.Status, "账号状态不变")
			require.Nil(t, account.RateLimitResetAt)

			waitForCodexReprobe(t, repo)
			require.EqualValues(t, 1, calls.Load())

			// 冷却窗口内再次拒绝：不再发起新的重探测。
			rec2 := httptest.NewRecorder()
			c2, _ := gin.CreateTestContext(rec2)
			c2.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			resp2 := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader("event: error\ndata: " + codexInvalidAccessProgramEvent + "\n\n")),
			}
			_ = path.run(svc, c2, resp2, account)
			require.False(t, svc.getCodexCapabilityProbe().ReprobeAsync(account.ID))
			time.Sleep(50 * time.Millisecond)
			require.EqualValues(t, 1, calls.Load(), "冷却窗口内不再探测")
		})
	}
}

// Chat / Messages 非流式聚合：实测首帧是 bare error 事件（非嵌套 response.failed），
// openAICompatTerminalResponse 重建失败对象时必须保留 param 才能识别。
func TestObserveCodexCapabilityRejection_ChatBufferedBareErrorEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, repo, calls := newCodexRejectionTestService(t)
	svc.cfg = rawChatCompletionsTestConfig()
	account := repo.accounts[31]
	body := []byte(`{"model":"gpt-6-sol","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	svc.httpUpstream = &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("event: error\ndata: " + codexAccessProgramNotEnabledEvent + "\n\n")),
	}}
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.Error(t, err)
	waitForCodexReprobe(t, repo)
	require.EqualValues(t, 1, calls.Load())
}

// Responses 非流式聚合（SSE→JSON，原生与透传）同样挂钩。
func TestObserveCodexCapabilityRejection_SSEToJSONPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sse := "event: error\ndata: " + codexInvalidAccessProgramEvent + "\n\n"
	for _, path := range []struct {
		name string
		run  func(*OpenAIGatewayService, *gin.Context, *http.Response, *Account) error
	}{
		{name: "native", run: func(svc *OpenAIGatewayService, c *gin.Context, resp *http.Response, account *Account) error {
			_, err := svc.handleSSEToJSON(resp, c, account, []byte(sse), "gpt-6-sol", "gpt-6-sol")
			return err
		}},
		{name: "passthrough", run: func(svc *OpenAIGatewayService, c *gin.Context, resp *http.Response, account *Account) error {
			_, err := svc.handlePassthroughSSEToJSON(resp, c, account, []byte(sse), "gpt-6-sol", "gpt-6-sol")
			return err
		}},
	} {
		t.Run(path.name, func(t *testing.T) {
			svc, repo, calls := newCodexRejectionTestService(t)
			account := repo.accounts[31]
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
			err := path.run(svc, c, resp, account)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr), "访问项目拒绝不 failover")
			require.Contains(t, rec.Body.String(), "Invalid access program", "错误照常透传给客户端")
			waitForCodexReprobe(t, repo)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

// 普通 error 事件不触发重探测。
func TestObserveCodexCapabilityRejection_IgnoresOrdinaryErrors(t *testing.T) {
	svc, repo, calls := newCodexRejectionTestService(t)
	account := repo.accounts[31]
	require.False(t, svc.observeCodexCapabilityRejection(account, []byte(`{"type":"error","error":{"type":"server_error","message":"boom"}}`)))
	require.False(t, svc.observeCodexCapabilityRejection(account, []byte(`{"type":"error","error":{"code":"cyber_policy","message":"blocked"}}`)))
	apiKey := &Account{ID: 32, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	require.False(t, svc.observeCodexCapabilityRejection(apiKey, []byte(codexAccessProgramNotEnabledEvent)), "API Key 账号没有探测来源")
	time.Sleep(50 * time.Millisecond)
	require.Zero(t, calls.Load())
	require.Zero(t, repo.updateCount())
}

// Chat Completions 非流式聚合路径同样挂钩。
func TestObserveCodexCapabilityRejection_ChatCompletionsBufferedPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, repo, calls := newCodexRejectionTestService(t)
	svc.cfg = rawChatCompletionsTestConfig()
	account := repo.accounts[31]
	body := []byte(`{"model":"gpt-6-sol","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	forward := func(failed string) error {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		svc.httpUpstream = &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("data: " + failed + "\n\n")),
		}}
		_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
		return err
	}
	// 基线：同形态、同 code 的失败事件但 param 不指向访问项目，failover 判定必须与之一致。
	baseline := forward(`{"type":"response.failed","response":{"id":"resp_err","status":"failed","error":{"code":"access_program_not_enabled","type":"invalid_request_error","message":"not enabled","param":"service_tier"},"output":[],"usage":{"input_tokens":10,"output_tokens":0,"total_tokens":10}}}`)
	require.Error(t, baseline)
	time.Sleep(50 * time.Millisecond)
	require.Zero(t, calls.Load(), "普通失败事件不触发重探测")

	rejection := forward(`{"type":"response.failed","response":{"id":"resp_err","status":"failed","error":{"code":"access_program_not_enabled","type":"invalid_request_error","message":"not enabled","param":"access_programs.cyber"},"output":[],"usage":{"input_tokens":10,"output_tokens":0,"total_tokens":10}}}`)
	require.Error(t, rejection)
	var baselineFailover, rejectionFailover *UpstreamFailoverError
	require.Equal(t, errors.As(baseline, &baselineFailover), errors.As(rejection, &rejectionFailover), "failover 判定不变")
	waitForCodexReprobe(t, repo)
	require.EqualValues(t, 1, calls.Load())
	_ = c
}

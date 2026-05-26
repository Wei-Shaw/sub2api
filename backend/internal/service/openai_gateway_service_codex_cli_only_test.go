package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubCodexRestrictionDetector struct {
	result CodexClientRestrictionDetectionResult
}

func (s *stubCodexRestrictionDetector) Detect(_ *gin.Context, _ *Account) CodexClientRestrictionDetectionResult {
	return s.result
}

func TestOpenAIGatewayService_GetCodexClientRestrictionDetector(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("使用注入的 detector", func(t *testing.T) {
		expected := &stubCodexRestrictionDetector{
			result: CodexClientRestrictionDetectionResult{Enabled: true, Matched: true, Reason: "stub"},
		}
		svc := &OpenAIGatewayService{codexDetector: expected}

		got := svc.getCodexClientRestrictionDetector()
		require.Same(t, expected, got)
	})

	t.Run("service 为 nil 时返回默认 detector", func(t *testing.T) {
		var svc *OpenAIGatewayService
		got := svc.getCodexClientRestrictionDetector()
		require.NotNil(t, got)
	})

	t.Run("service 未注入 detector 时返回默认 detector", func(t *testing.T) {
		svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: true}}}
		got := svc.getCodexClientRestrictionDetector()
		require.NotNil(t, got)

		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.Request.Header.Set("User-Agent", "curl/8.0")
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"codex_cli_only": true}}

		result := got.Detect(c, account)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonForceCodexCLI, result.Reason)
	})
}

func TestGetAPIKeyIDFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("context 为 nil", func(t *testing.T) {
		require.Equal(t, int64(0), getAPIKeyIDFromContext(nil))
	})

	t.Run("上下文没有 api_key", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		require.Equal(t, int64(0), getAPIKeyIDFromContext(c))
	})

	t.Run("api_key 类型错误", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Set("api_key", "not-api-key")
		require.Equal(t, int64(0), getAPIKeyIDFromContext(c))
	})

	t.Run("api_key 指针为空", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		var k *APIKey
		c.Set("api_key", k)
		require.Equal(t, int64(0), getAPIKeyIDFromContext(c))
	})

	t.Run("正常读取 api_key_id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Set("api_key", &APIKey{ID: 12345})
		require.Equal(t, int64(12345), getAPIKeyIDFromContext(c))
	})
}

func TestLogCodexCLIOnlyDetection_NilSafety(t *testing.T) {
	// 不校验日志内容，仅保证在 nil 入参下不会 panic。
	require.NotPanics(t, func() {
		logCodexCLIOnlyDetection(context.TODO(), nil, nil, 0, CodexClientRestrictionDetectionResult{Enabled: true, Matched: false, Reason: "test"}, nil)
		logCodexCLIOnlyDetection(context.Background(), nil, nil, 0, CodexClientRestrictionDetectionResult{Enabled: false, Matched: false, Reason: "disabled"}, nil)
	})
}

func TestLogCodexCLIOnlyDetection_OnlyLogsRejected(t *testing.T) {
	logSink, restore := captureStructuredLog(t)
	defer restore()

	account := &Account{ID: 1001}
	logCodexCLIOnlyDetection(context.Background(), nil, account, 2002, CodexClientRestrictionDetectionResult{
		Enabled: true,
		Matched: true,
		Reason:  CodexClientRestrictionReasonMatchedUA,
	}, nil)
	logCodexCLIOnlyDetection(context.Background(), nil, account, 2002, CodexClientRestrictionDetectionResult{
		Enabled: true,
		Matched: false,
		Reason:  CodexClientRestrictionReasonNotMatchedUA,
	}, nil)

	require.False(t, logSink.ContainsMessage("OpenAI codex_cli_only 允许官方客户端请求"))
	require.True(t, logSink.ContainsMessage("OpenAI codex_cli_only 拒绝非官方客户端请求"))
}

func TestLogCodexCLIOnlyDetection_RejectedIncludesRequestDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, restore := captureStructuredLog(t)
	defer restore()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses?trace=1", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0 (Windows 10.0.19045; x86_64) unknown")
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("OpenAI-Beta", "assistants=v2")

	body := []byte(`{"model":"gpt-5.2","stream":false,"prompt_cache_key":"pc-123","access_token":"secret-token","input":[{"type":"text","text":"hello"}]}`)
	account := &Account{ID: 1001}
	logCodexCLIOnlyDetection(context.Background(), c, account, 2002, CodexClientRestrictionDetectionResult{
		Enabled: true,
		Matched: false,
		Reason:  CodexClientRestrictionReasonNotMatchedUA,
	}, body)

	require.True(t, logSink.ContainsFieldValue("request_user_agent", "codex_cli_rs/0.98.0 (Windows 10.0.19045; x86_64) unknown"))
	require.True(t, logSink.ContainsFieldValue("request_model", "gpt-5.2"))
	require.True(t, logSink.ContainsFieldValue("request_query", "trace=1"))
	require.True(t, logSink.ContainsFieldValue("request_prompt_cache_key_sha256", hashSensitiveValueForLog("pc-123")))
	require.True(t, logSink.ContainsFieldValue("request_headers", "openai-beta"))
	require.True(t, logSink.ContainsField("request_body_size"))
	require.False(t, logSink.ContainsField("request_body_preview"))
}

func TestLogOpenAIInstructionsRequiredDebug_LogsRequestDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, restore := captureStructuredLog(t)
	defer restore()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses?trace=1", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "curl/8.0")
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("OpenAI-Beta", "assistants=v2")

	body := []byte(`{"model":"gpt-5.1-codex","stream":false,"prompt_cache_key":"pc-abc","access_token":"secret-token","input":[{"type":"text","text":"hello"}]}`)
	account := &Account{ID: 1001, Name: "codex max套餐"}

	logOpenAIInstructionsRequiredDebug(
		context.Background(),
		c,
		account,
		http.StatusBadRequest,
		"Instructions are required",
		body,
		[]byte(`{"error":{"message":"Instructions are required","type":"invalid_request_error","param":"instructions","code":"missing_required_parameter"}}`),
	)

	require.True(t, logSink.ContainsMessageAtLevel("OpenAI 上游返回 Instructions are required，已记录请求详情用于排查", "warn"))
	require.True(t, logSink.ContainsFieldValue("request_user_agent", "curl/8.0"))
	require.True(t, logSink.ContainsFieldValue("request_model", "gpt-5.1-codex"))
	require.True(t, logSink.ContainsFieldValue("request_query", "trace=1"))
	require.True(t, logSink.ContainsFieldValue("account_name", "codex max套餐"))
	require.True(t, logSink.ContainsFieldValue("request_headers", "openai-beta"))
	require.True(t, logSink.ContainsField("request_body_size"))
	require.False(t, logSink.ContainsField("request_body_preview"))
}

func TestLogOpenAIInstructionsRequiredDebug_NonTargetErrorSkipped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, restore := captureStructuredLog(t)
	defer restore()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "curl/8.0")
	body := []byte(`{"model":"gpt-5.1-codex","stream":false}`)

	logOpenAIInstructionsRequiredDebug(
		context.Background(),
		c,
		&Account{ID: 1001},
		http.StatusForbidden,
		"forbidden",
		body,
		[]byte(`{"error":{"message":"forbidden"}}`),
	)

	require.False(t, logSink.ContainsMessage("OpenAI 上游返回 Instructions are required，已记录请求详情用于排查"))
}

func TestIsOpenAITransientProcessingError(t *testing.T) {
	require.True(t, isOpenAITransientProcessingError(
		http.StatusBadRequest,
		"An error occurred while processing your request.",
		nil,
	))

	require.True(t, isOpenAITransientProcessingError(
		http.StatusBadRequest,
		"",
		[]byte(`{"error":{"message":"An error occurred while processing your request. You can retry your request, or contact us through our help center at help.openai.com if the error persists. Please include the request ID req_123 in your message."}}`),
	))

	require.False(t, isOpenAITransientProcessingError(
		http.StatusBadRequest,
		"Missing required parameter: 'instructions'",
		[]byte(`{"error":{"message":"Missing required parameter: 'instructions'"}}`),
	))
}

func TestOpenAIGatewayService_Forward_LogsInstructionsRequiredDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, restore := captureStructuredLog(t)
	defer restore()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses?trace=1", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("OpenAI-Beta", "assistants=v2")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusBadRequest,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
				"x-request-id": []string{"rid-upstream"},
			},
			Body: io.NopCloser(strings.NewReader(`{"error":{"message":"Missing required parameter: 'instructions'","type":"invalid_request_error","param":"instructions","code":"missing_required_parameter"}}`)),
		},
	}
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{ForceCodexCLI: false},
		},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:             1001,
		Name:           "codex max套餐",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeAPIKey,
		Concurrency:    1,
		Credentials:    map[string]any{"api_key": "sk-test"},
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
	}
	body := []byte(`{"model":"gpt-5.1-codex","stream":false,"input":[{"type":"text","text":"hello"}],"prompt_cache_key":"pc-forward","access_token":"secret-token"}`)

	_, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, err.Error(), "upstream error: 400")

	require.True(t, logSink.ContainsMessageAtLevel("OpenAI 上游返回 Instructions are required，已记录请求详情用于排查", "warn"))
	require.True(t, logSink.ContainsFieldValue("request_user_agent", "codex_cli_rs/0.1.0"))
	require.True(t, logSink.ContainsFieldValue("request_model", "gpt-5.1-codex"))
	require.True(t, logSink.ContainsFieldValue("request_headers", "openai-beta"))
	require.True(t, logSink.ContainsField("request_body_size"))
	require.False(t, logSink.ContainsField("request_body_preview"))
}

func TestOpenAIGatewayService_Forward_TransientProcessingErrorTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusBadRequest,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
				"x-request-id": []string{"rid-processing-400"},
			},
			Body: io.NopCloser(strings.NewReader(`{"error":{"message":"An error occurred while processing your request. You can retry your request, or contact us through our help center at help.openai.com if the error persists. Please include the request ID req_123 in your message.","type":"invalid_request_error"}}`)),
		},
	}
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{ForceCodexCLI: false},
		},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:             1001,
		Name:           "codex max套餐",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeAPIKey,
		Concurrency:    1,
		Credentials:    map[string]any{"api_key": "sk-test"},
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
	}
	body := []byte(`{"model":"gpt-5.1-codex","stream":false,"input":[{"type":"text","text":"hello"}]}`)

	_, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadRequest, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "An error occurred while processing your request")
	require.False(t, c.Writer.Written(), "service 层应返回 failover 错误给上层换号，而不是直接向客户端写响应")
}

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

type openAIAccountSchedulerStub struct {
	selectFn func(ctx context.Context, req service.OpenAIAccountScheduleRequest) (*service.AccountSelectionResult, service.OpenAIAccountScheduleDecision, error)
}

func (s *openAIAccountSchedulerStub) Select(ctx context.Context, req service.OpenAIAccountScheduleRequest) (*service.AccountSelectionResult, service.OpenAIAccountScheduleDecision, error) {
	if s != nil && s.selectFn != nil {
		return s.selectFn(ctx, req)
	}
	return nil, service.OpenAIAccountScheduleDecision{}, nil
}

func (s *openAIAccountSchedulerStub) ReportResult(accountID int64, success bool, firstTokenMs *int) {}

func (s *openAIAccountSchedulerStub) ReportSwitch() {}

func (s *openAIAccountSchedulerStub) SnapshotMetrics() service.OpenAIAccountSchedulerMetricsSnapshot {
	return service.OpenAIAccountSchedulerMetricsSnapshot{}
}

func setUnexportedFieldForTest(t *testing.T, target any, fieldName string, value any) {
	t.Helper()
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Ptr || targetValue.IsNil() {
		t.Fatalf("target for %s must be a non-nil pointer", fieldName)
	}
	field := targetValue.Elem().FieldByName(fieldName)
	if !field.IsValid() {
		t.Fatalf("field %s not found", fieldName)
	}
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}

func newOpenAIStickyDecisionForTest(t *testing.T, layer string, evalResult string) service.OpenAIAccountScheduleDecision {
	t.Helper()
	decision := service.OpenAIAccountScheduleDecision{Layer: layer}
	decisionValue := reflect.ValueOf(&decision).Elem()
	stickyField := decisionValue.FieldByName("Sticky")
	if !stickyField.IsValid() {
		t.Fatal("missing Sticky field on OpenAIAccountScheduleDecision")
	}
	stickyValue := reflect.New(stickyField.Type().Elem())
	stickyElem := stickyValue.Elem()
	stickyElem.FieldByName("SessionSource").SetString("header_x_session_affinity")
	stickyElem.FieldByName("SessionHashPresent").SetBool(true)
	stickyElem.FieldByName("EvalResult").SetString(evalResult)
	stickyElem.FieldByName("SelectedAccountChanged").SetBool(false)
	stickyElem.FieldByName("ParentSessionPresent").SetBool(false)
	stickyElem.FieldByName("ParentSessionKey").SetString("")
	stickyField.Set(stickyValue)
	return decision
}

func TestOpenAIHandleStreamingAwareError_JSONEscaping(t *testing.T) {
	tests := []struct {
		name    string
		errType string
		message string
	}{
		{
			name:    "包含双引号的消息",
			errType: "server_error",
			message: `upstream returned "invalid" response`,
		},
		{
			name:    "包含反斜杠的消息",
			errType: "server_error",
			message: `path C:\Users\test\file.txt not found`,
		},
		{
			name:    "包含双引号和反斜杠的消息",
			errType: "upstream_error",
			message: `error parsing "key\value": unexpected token`,
		},
		{
			name:    "包含换行符的消息",
			errType: "server_error",
			message: "line1\nline2\ttab",
		},
		{
			name:    "普通消息",
			errType: "upstream_error",
			message: "Upstream service temporarily unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			h := &OpenAIGatewayHandler{}
			h.handleStreamingAwareError(c, http.StatusBadGateway, tt.errType, tt.message, true)

			body := w.Body.String()

			// 验证 SSE 格式：event: error\ndata: {JSON}\n\n
			assert.True(t, strings.HasPrefix(body, "event: error\n"), "应以 'event: error\\n' 开头")
			assert.True(t, strings.HasSuffix(body, "\n\n"), "应以 '\\n\\n' 结尾")

			// 提取 data 部分
			lines := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n")
			require.Len(t, lines, 2, "应有 event 行和 data 行")
			dataLine := lines[1]
			require.True(t, strings.HasPrefix(dataLine, "data: "), "第二行应以 'data: ' 开头")
			jsonStr := strings.TrimPrefix(dataLine, "data: ")

			// 验证 JSON 合法性
			var parsed map[string]any
			err := json.Unmarshal([]byte(jsonStr), &parsed)
			require.NoError(t, err, "JSON 应能被成功解析，原始 JSON: %s", jsonStr)

			// 验证结构
			errorObj, ok := parsed["error"].(map[string]any)
			require.True(t, ok, "应包含 error 对象")
			assert.Equal(t, tt.errType, errorObj["type"])
			assert.Equal(t, tt.message, errorObj["message"])
		})
	}
}

func TestOpenAIHandleStreamingAwareError_NonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h := &OpenAIGatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "test error", false)

	// 非流式应返回 JSON 响应
	assert.Equal(t, http.StatusBadGateway, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorObj["type"])
	assert.Equal(t, "test error", errorObj["message"])
}

func TestOpenAIHandleStreamingAwareError_IncludesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	w.Header().Set("X-Request-Id", "req-err-123")
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h := &OpenAIGatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "test error", false)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "req-err-123", errorObj["request_id"])
}

func TestReadRequestBodyWithPrealloc(t *testing.T) {
	payload := `{"model":"gpt-5","input":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(payload))
	req.ContentLength = int64(len(payload))

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(req)
	require.NoError(t, err)
	require.Equal(t, payload, string(body))
}

func TestReadRequestBodyWithPrealloc_MaxBytesError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(strings.Repeat("x", 8)))
	req.Body = http.MaxBytesReader(rec, req.Body, 4)

	_, err := pkghttputil.ReadRequestBodyWithPrealloc(req)
	require.Error(t, err)
	var maxErr *http.MaxBytesError
	require.ErrorAs(t, err, &maxErr)
}

func TestOpenAIEnsureForwardErrorResponse_WritesFallbackWhenNotWritten(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h := &OpenAIGatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.True(t, wrote)
	require.Equal(t, http.StatusBadGateway, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorObj["type"])
	assert.Equal(t, "Upstream request failed", errorObj["message"])
}

func TestOpenAIEnsureForwardErrorResponse_DoesNotOverrideWrittenResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.String(http.StatusTeapot, "already written")

	h := &OpenAIGatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.False(t, wrote)
	require.Equal(t, http.StatusTeapot, w.Code)
	assert.Equal(t, "already written", w.Body.String())
}

func TestOpenAIErrorResponse_IncludesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	w.Header().Set("X-Request-Id", "req-json-456")
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h := &OpenAIGatewayHandler{}
	h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "bad request")

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "req-json-456", errorObj["request_id"])
}

func TestShouldLogOpenAIForwardFailureAsWarn(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("fallback_written_should_not_downgrade", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		require.False(t, shouldLogOpenAIForwardFailureAsWarn(c, true))
	})

	t.Run("context_nil_should_not_downgrade", func(t *testing.T) {
		require.False(t, shouldLogOpenAIForwardFailureAsWarn(nil, false))
	})

	t.Run("response_not_written_should_not_downgrade", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		require.False(t, shouldLogOpenAIForwardFailureAsWarn(c, false))
	})

	t.Run("response_already_written_should_downgrade", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.String(http.StatusForbidden, "already written")
		require.True(t, shouldLogOpenAIForwardFailureAsWarn(c, false))
	})
}

func TestOpenAIRecoverResponsesPanic_WritesFallbackResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	h := &OpenAIGatewayHandler{}
	streamStarted := false
	require.NotPanics(t, func() {
		func() {
			defer h.recoverResponsesPanic(c, &streamStarted)
			panic("test panic")
		}()
	})

	require.Equal(t, http.StatusBadGateway, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)

	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorObj["type"])
	assert.Equal(t, "Upstream request failed", errorObj["message"])
}

func TestOpenAIRecoverResponsesPanic_NoPanicNoWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	h := &OpenAIGatewayHandler{}
	streamStarted := false
	require.NotPanics(t, func() {
		func() {
			defer h.recoverResponsesPanic(c, &streamStarted)
		}()
	})

	require.False(t, c.Writer.Written())
	assert.Equal(t, "", w.Body.String())
}

func TestOpenAIRecoverResponsesPanic_DoesNotOverrideWrittenResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.String(http.StatusTeapot, "already written")

	h := &OpenAIGatewayHandler{}
	streamStarted := false
	require.NotPanics(t, func() {
		func() {
			defer h.recoverResponsesPanic(c, &streamStarted)
			panic("test panic")
		}()
	})

	require.Equal(t, http.StatusTeapot, w.Code)
	assert.Equal(t, "already written", w.Body.String())
}

func TestOpenAIMissingResponsesDependencies(t *testing.T) {
	t.Run("nil_handler", func(t *testing.T) {
		var h *OpenAIGatewayHandler
		require.Equal(t, []string{"handler"}, h.missingResponsesDependencies())
	})

	t.Run("all_dependencies_missing", func(t *testing.T) {
		h := &OpenAIGatewayHandler{}
		require.Equal(t,
			[]string{"gatewayService", "billingCacheService", "apiKeyService", "concurrencyHelper"},
			h.missingResponsesDependencies(),
		)
	})

	t.Run("all_dependencies_present", func(t *testing.T) {
		h := &OpenAIGatewayHandler{
			gatewayService:      &service.OpenAIGatewayService{},
			billingCacheService: &service.BillingCacheService{},
			apiKeyService:       &service.APIKeyService{},
			concurrencyHelper: &ConcurrencyHelper{
				concurrencyService: &service.ConcurrencyService{},
			},
		}
		require.Empty(t, h.missingResponsesDependencies())
	})
}

func TestOpenAIEnsureResponsesDependencies(t *testing.T) {
	t.Run("missing_dependencies_returns_503", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

		h := &OpenAIGatewayHandler{}
		ok := h.ensureResponsesDependencies(c, nil)

		require.False(t, ok)
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
		var parsed map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &parsed)
		require.NoError(t, err)
		errorObj, exists := parsed["error"].(map[string]any)
		require.True(t, exists)
		assert.Equal(t, "api_error", errorObj["type"])
		assert.Equal(t, "Service temporarily unavailable", errorObj["message"])
	})

	t.Run("already_written_response_not_overridden", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.String(http.StatusTeapot, "already written")

		h := &OpenAIGatewayHandler{}
		ok := h.ensureResponsesDependencies(c, nil)

		require.False(t, ok)
		require.Equal(t, http.StatusTeapot, w.Code)
		assert.Equal(t, "already written", w.Body.String())
	})

	t.Run("dependencies_ready_returns_true_and_no_write", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

		h := &OpenAIGatewayHandler{
			gatewayService:      &service.OpenAIGatewayService{},
			billingCacheService: &service.BillingCacheService{},
			apiKeyService:       &service.APIKeyService{},
			concurrencyHelper: &ConcurrencyHelper{
				concurrencyService: &service.ConcurrencyService{},
			},
		}
		ok := h.ensureResponsesDependencies(c, nil)

		require.True(t, ok)
		require.False(t, c.Writer.Written())
		assert.Equal(t, "", w.Body.String())
	})
}

func TestResolveOpenAIForwardDefaultMappedModel(t *testing.T) {
	t.Run("prefers_explicit_fallback_model", func(t *testing.T) {
		apiKey := &service.APIKey{
			Group: &service.Group{DefaultMappedModel: "gpt-5.4"},
		}
		require.Equal(t, "gpt-5.2", resolveOpenAIForwardDefaultMappedModel(apiKey, " gpt-5.2 "))
	})

	t.Run("uses_group_default_when_explicit_fallback_absent", func(t *testing.T) {
		apiKey := &service.APIKey{
			Group: &service.Group{DefaultMappedModel: "gpt-5.4"},
		}
		require.Equal(t, "gpt-5.4", resolveOpenAIForwardDefaultMappedModel(apiKey, ""))
	})

	t.Run("returns_empty_without_group_default", func(t *testing.T) {
		require.Empty(t, resolveOpenAIForwardDefaultMappedModel(nil, ""))
		require.Empty(t, resolveOpenAIForwardDefaultMappedModel(&service.APIKey{}, ""))
		require.Empty(t, resolveOpenAIForwardDefaultMappedModel(&service.APIKey{
			Group: &service.Group{},
		}, ""))
	})
}

func TestResolveOpenAIMessagesDispatchMappedModel(t *testing.T) {
	t.Run("exact_claude_model_override_wins", func(t *testing.T) {
		apiKey := &service.APIKey{
			Group: &service.Group{
				MessagesDispatchModelConfig: service.OpenAIMessagesDispatchModelConfig{
					SonnetMappedModel: "gpt-5.2",
					ExactModelMappings: map[string]string{
						"claude-sonnet-4-5-20250929": "gpt-5.4-mini-high",
					},
				},
			},
		}
		require.Equal(t, "gpt-5.4-mini", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-sonnet-4-5-20250929"))
	})

	t.Run("uses_family_default_when_no_override", func(t *testing.T) {
		apiKey := &service.APIKey{Group: &service.Group{}}
		require.Equal(t, "gpt-5.4", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-opus-4-6"))
		require.Equal(t, "gpt-5.3-codex", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-sonnet-4-5-20250929"))
		require.Equal(t, "gpt-5.4-mini", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-haiku-4-5-20251001"))
	})

	t.Run("returns_empty_for_non_claude_or_missing_group", func(t *testing.T) {
		require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(nil, "claude-sonnet-4-5-20250929"))
		require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(&service.APIKey{}, "claude-sonnet-4-5-20250929"))
		require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(&service.APIKey{Group: &service.Group{}}, "gpt-5.4"))
	})

	t.Run("does_not_fall_back_to_group_default_mapped_model", func(t *testing.T) {
		apiKey := &service.APIKey{
			Group: &service.Group{
				DefaultMappedModel: "gpt-5.4",
			},
		}
		require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(apiKey, "gpt-5.4"))
		require.Equal(t, "gpt-5.3-codex", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-sonnet-4-5-20250929"))
	})
}

func TestPrepareResponsesRequestForScheduling_FunctionCallOutputUsesExhaustedGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := []byte(`{"model":"gpt-5.4","input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)

	patchedBody, patchedModel, targetGroup, err := prepareResponsesRequestForScheduling(c, body, "gpt-5.4")
	require.NoError(t, err)
	require.Equal(t, "gpt-5.4", patchedModel)
	require.Equal(t, service.TargetGroupExhausted, targetGroup)
	require.JSONEq(t, string(body), string(patchedBody))
}

func TestPrepareResponsesRequestForScheduling_FunctionCallOutputTypeVariantUsesExhaustedGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := []byte(`{"model":"gpt-5.4","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},{"type":" Function_Call_Output ","call_id":"call_2","output":"ok"}]}`)

	patchedBody, patchedModel, targetGroup, err := prepareResponsesRequestForScheduling(c, body, "gpt-5.4")
	require.NoError(t, err)
	require.Equal(t, "gpt-5.4", patchedModel)
	require.Equal(t, service.TargetGroupExhausted, targetGroup)
	require.JSONEq(t, string(body), string(patchedBody))
}

func TestPrepareResponsesRequestForScheduling_SysModelAppendsContinuation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := []byte(`{"model":"gpt-5.4-Sys","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]}`)

	patchedBody, patchedModel, targetGroup, err := prepareResponsesRequestForScheduling(c, body, "gpt-5.4-Sys")
	require.NoError(t, err)
	require.Equal(t, "gpt-5.4", patchedModel)
	require.Equal(t, "gpt-5.4", gjson.GetBytes(patchedBody, "model").String())
	require.Equal(t, service.TargetGroupExhausted, targetGroup)
	require.Contains(t, string(patchedBody), `"type":"function_call"`)
	require.Contains(t, string(patchedBody), `"name":"sub2api_sys_bootstrap"`)
	require.Contains(t, string(patchedBody), `Synthetic bootstrap continuation inserted by sub2api for -Sys routing.`)
}

func TestPrepareResponsesRequestForScheduling_SysModelAppendsContinuationForRoleBasedUserItem(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := []byte(`{"model":"gpt-5.4-Sys","input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}]}`)

	patchedBody, patchedModel, targetGroup, err := prepareResponsesRequestForScheduling(c, body, "gpt-5.4-Sys")
	require.NoError(t, err)
	require.Equal(t, "gpt-5.4", patchedModel)
	require.Equal(t, service.TargetGroupExhausted, targetGroup)
	require.Contains(t, string(patchedBody), `"type":"function_call"`)
	require.Contains(t, string(patchedBody), `"type":"function_call_output"`)
}

func TestPrepareResponsesRequestForScheduling_SysModelStringInputAppendsContinuation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := []byte(`{"model":"gpt-5.4-Sys","tools":[{"type":"function","name":"lookup_number"}],"input":"hello"}`)

	patchedBody, patchedModel, targetGroup, err := prepareResponsesRequestForScheduling(c, body, "gpt-5.4-Sys")
	require.NoError(t, err)
	require.Equal(t, "gpt-5.4", patchedModel)
	require.Equal(t, service.TargetGroupExhausted, targetGroup)
	require.Equal(t, "message", gjson.GetBytes(patchedBody, "input.0.type").String())
	require.Equal(t, "user", gjson.GetBytes(patchedBody, "input.0.role").String())
	require.Equal(t, "hello", gjson.GetBytes(patchedBody, "input.0.content.0.text").String())
	require.Contains(t, string(patchedBody), `"type":"function_call_output"`)
}

func TestPrepareResponsesRequestForScheduling_SysModelStripEmptyReturnsInvalidModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := []byte(`{"model":"-Sys","input":[{"type":"message","role":"user","content":[]}]}`)

	patchedBody, patchedModel, targetGroup, err := prepareResponsesRequestForScheduling(c, body, "-Sys")
	require.Error(t, err)
	require.ErrorIs(t, err, errPrepareResponsesRequestInvalidModel)
	require.Nil(t, patchedBody)
	require.Equal(t, "", patchedModel)
	require.Equal(t, service.TargetGroupActive, targetGroup)
}

func TestResponsesNoAvailableAccountsError(t *testing.T) {
	status, code, message := responsesNoAvailableAccountsError(service.TargetGroupExhausted)
	require.Equal(t, http.StatusTooManyRequests, status)
	require.Equal(t, "rate_limit_exceeded", code)
	require.Equal(t, "No available accounts in target group (exhausted)", message)

	status, code, message = responsesNoAvailableAccountsError(service.TargetGroupActive)
	require.Equal(t, http.StatusServiceUnavailable, status)
	require.Equal(t, "service_unavailable", code)
	require.Equal(t, "No available accounts in target group (active)", message)
}

func TestStoreOpenAIRoutingSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	account := &service.Account{ID: 66, Name: "acc-66"}
	snap := storeOpenAIRoutingSnapshot(c, service.OpenAIRoutingSnapshotInput{
		TargetGroup:    service.TargetGroupExhausted,
		ScheduleLayer:  "load_balance",
		SelectedGroup:  "reserve",
		Account:        account,
		RequestedModel: "gpt-5.4-Sys",
		EffectiveModel: "gpt-5.4",
	})

	require.NotNil(t, snap)
	require.Same(t, snap, getOpenAIRoutingSnapshot(c))
	require.Equal(t, "exhausted", snap.TargetGroup)
	require.Equal(t, "load_balance", snap.ScheduleLayer)
	require.Equal(t, "reserve", snap.SelectedGroup)
	if assert.NotNil(t, snap.SelectedAccountID) {
		assert.Equal(t, int64(66), *snap.SelectedAccountID)
	}
	if assert.NotNil(t, snap.SelectedAccountName) {
		assert.Equal(t, "acc-66", *snap.SelectedAccountName)
	}
}

func TestStoreOpenAIRoutingSnapshot_StickyObservability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	account := &service.Account{ID: 6601, Name: "sticky-acc"}
	decisionType := reflect.TypeOf(service.OpenAIAccountScheduleDecision{})
	stickyField, ok := decisionType.FieldByName("Sticky")
	if !ok {
		t.Fatal("missing Sticky field on OpenAIAccountScheduleDecision")
	}
	stickyValue := reflect.New(stickyField.Type.Elem())
	stickyElem := stickyValue.Elem()
	stickyElem.FieldByName("SessionSource").SetString("header_x_session_affinity")
	stickyElem.FieldByName("SessionHashPresent").SetBool(true)
	stickyElem.FieldByName("EvalResult").SetString("hit")
	stickyElem.FieldByName("SelectedAccountChanged").SetBool(false)
	stickyElem.FieldByName("ParentSessionPresent").SetBool(false)
	stickyElem.FieldByName("ParentSessionKey").SetString("")
	boundAccountID := int64(6601)
	stickyElem.FieldByName("BoundAccountID").Set(reflect.ValueOf(&boundAccountID))

	input := service.OpenAIRoutingSnapshotInput{
		TargetGroup:    service.TargetGroupAny,
		ScheduleLayer:  "session_hash",
		SelectedGroup:  "active",
		Account:        account,
		RequestedModel: "gpt-5.1",
		EffectiveModel: "gpt-5.1",
	}
	inputValue := reflect.ValueOf(&input).Elem()
	stickyInputField := inputValue.FieldByName("Sticky")
	if !stickyInputField.IsValid() {
		t.Fatal("missing Sticky field on OpenAIRoutingSnapshotInput")
	}
	stickyInputField.Set(stickyValue)

	snap := storeOpenAIRoutingSnapshot(c, input)
	require.NotNil(t, snap)

	snapshotStickyField := reflect.ValueOf(snap).Elem().FieldByName("Sticky")
	if !snapshotStickyField.IsValid() {
		t.Fatal("missing Sticky field on OpenAIRoutingSnapshot")
	}
	if snapshotStickyField.IsNil() {
		t.Fatal("snapshot Sticky is nil")
	}
	snapshotStickyValue := snapshotStickyField.Elem()
	require.Equal(t, "hit", snapshotStickyValue.FieldByName("EvalResult").String())
	require.Equal(t, "", snapshotStickyValue.FieldByName("ParentSessionKey").String())
	require.False(t, snapshotStickyValue.FieldByName("SelectedAccountChanged").Bool())
	require.Equal(t, "active", snap.SelectedGroup)
	storedBoundAccountID := snapshotStickyValue.FieldByName("BoundAccountID")
	require.Equal(t, reflect.Ptr, storedBoundAccountID.Kind())
	require.False(t, storedBoundAccountID.IsNil())
	require.Equal(t, account.ID, storedBoundAccountID.Elem().Int())
}

func TestStoreOpenAIRoutingSnapshotFromDecision_SelectedGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	account := &service.Account{ID: 6610, Name: "reserve-acc"}
	snap := storeOpenAIRoutingSnapshotFromDecision(
		c,
		service.TargetGroupExhausted,
		service.OpenAIAccountScheduleDecision{
			Layer:         "load_balance",
			SelectedGroup: "reserve",
		},
		account,
		"gpt-5.4-Sys",
		"gpt-5.4",
	)

	require.NotNil(t, snap)
	require.Equal(t, "exhausted", snap.TargetGroup)
	require.Equal(t, "reserve", snap.SelectedGroup)
	require.Same(t, snap, getOpenAIRoutingSnapshot(c))
}

func TestResolveOpenAIScheduleSessionSource_StickyContentFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)

	source := resolveOpenAIScheduleSessionSource(c, []byte(`{"model":"gpt-5.1","input":[{"type":"input_text","text":"hello"}]}`), "derived_content_hash", "")
	require.Equal(t, openAIScheduleSessionSourceContentFallback, source)
}

func TestResolveOpenAIScheduleSessionSource_StickyFallbackSeed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil)

	source := resolveOpenAIScheduleSessionSource(c, nil, "derived_fallback_hash", openAIScheduleSessionSourceFallbackSeed)
	require.Equal(t, openAIScheduleSessionSourceFallbackSeed, source)
}

func TestOpenAIResponses_FailedSelectionStillStoresStickySnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-5.1","stream":false,"input":[{"type":"input_text","text":"hello"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-session-affinity", "affinity-failure-path")

	groupID := int64(12)
	apiKey := &service.APIKey{
		ID:      101,
		GroupID: &groupID,
		User:    &service.User{ID: 1},
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1, Concurrency: 1})

	billingCfg := &config.Config{RunMode: config.RunModeSimple}
	billingService := service.NewBillingCacheService(nil, nil, nil, nil, billingCfg)
	defer billingService.Stop()

	gatewayService := &service.OpenAIGatewayService{}
	setUnexportedFieldForTest(t, gatewayService, "openaiScheduler", &openAIAccountSchedulerStub{
		selectFn: func(ctx context.Context, req service.OpenAIAccountScheduleRequest) (*service.AccountSelectionResult, service.OpenAIAccountScheduleDecision, error) {
			return nil, newOpenAIStickyDecisionForTest(t, "load_balance", "miss_no_binding"), service.ErrNoAvailableAccounts
		},
	})

	concurrencyCache := &concurrencyCacheMock{
		acquireUserSlotFn: func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
			return true, nil
		},
	}
	h := &OpenAIGatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billingService,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(concurrencyCache), SSEPingFormatNone, time.Second),
	}

	h.Responses(c)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	snapshot := getOpenAIRoutingSnapshot(c)
	require.NotNil(t, snapshot)
	require.Equal(t, "load_balance", snapshot.ScheduleLayer)
	require.Nil(t, snapshot.SelectedAccountID)
	snapshotStickyField := reflect.ValueOf(snapshot).Elem().FieldByName("Sticky")
	require.True(t, snapshotStickyField.IsValid())
	require.False(t, snapshotStickyField.IsNil())
	snapshotSticky := snapshotStickyField.Elem()
	require.Equal(t, "miss_no_binding", snapshotSticky.FieldByName("EvalResult").String())
	require.Equal(t, "header_x_session_affinity", snapshotSticky.FieldByName("SessionSource").String())
	require.True(t, snapshotSticky.FieldByName("SessionHashPresent").Bool())
}

func TestOpenAIMessages_FailedSelectionStillStoresStickySnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-20250514","stream":false,"messages":[{"role":"user","content":"hello"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-session-affinity", "affinity-messages-failure")

	groupID := int64(13)
	apiKey := &service.APIKey{
		ID:      102,
		GroupID: &groupID,
		User:    &service.User{ID: 2},
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 2, Concurrency: 1})

	billingCfg := &config.Config{RunMode: config.RunModeSimple}
	billingService := service.NewBillingCacheService(nil, nil, nil, nil, billingCfg)
	defer billingService.Stop()

	gatewayService := &service.OpenAIGatewayService{}
	setUnexportedFieldForTest(t, gatewayService, "openaiScheduler", &openAIAccountSchedulerStub{
		selectFn: func(ctx context.Context, req service.OpenAIAccountScheduleRequest) (*service.AccountSelectionResult, service.OpenAIAccountScheduleDecision, error) {
			return nil, newOpenAIStickyDecisionForTest(t, "load_balance", "miss_no_binding"), service.ErrNoAvailableAccounts
		},
	})

	concurrencyCache := &concurrencyCacheMock{
		acquireUserSlotFn: func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
			return true, nil
		},
	}
	h := &OpenAIGatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billingService,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(concurrencyCache), SSEPingFormatNone, time.Second),
	}

	h.Messages(c)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	snapshot := getOpenAIRoutingSnapshot(c)
	require.NotNil(t, snapshot)
	require.Equal(t, "load_balance", snapshot.ScheduleLayer)
	require.Nil(t, snapshot.SelectedAccountID)
	snapshotStickyField := reflect.ValueOf(snapshot).Elem().FieldByName("Sticky")
	require.True(t, snapshotStickyField.IsValid())
	require.False(t, snapshotStickyField.IsNil())
	snapshotSticky := snapshotStickyField.Elem()
	require.Equal(t, "miss_no_binding", snapshotSticky.FieldByName("EvalResult").String())
	require.Equal(t, "header_x_session_affinity", snapshotSticky.FieldByName("SessionSource").String())
	require.True(t, snapshotSticky.FieldByName("SessionHashPresent").Bool())
}

func TestOpenAIChatCompletions_FailedSelectionStillStoresStickySnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.1","stream":false,"messages":[{"role":"user","content":"hello"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-session-affinity", "affinity-chat-failure")

	groupID := int64(14)
	apiKey := &service.APIKey{
		ID:      103,
		GroupID: &groupID,
		User:    &service.User{ID: 3},
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 3, Concurrency: 1})

	billingCfg := &config.Config{RunMode: config.RunModeSimple}
	billingService := service.NewBillingCacheService(nil, nil, nil, nil, billingCfg)
	defer billingService.Stop()

	gatewayService := &service.OpenAIGatewayService{}
	setUnexportedFieldForTest(t, gatewayService, "openaiScheduler", &openAIAccountSchedulerStub{
		selectFn: func(ctx context.Context, req service.OpenAIAccountScheduleRequest) (*service.AccountSelectionResult, service.OpenAIAccountScheduleDecision, error) {
			return nil, newOpenAIStickyDecisionForTest(t, "load_balance", "miss_no_binding"), service.ErrNoAvailableAccounts
		},
	})

	concurrencyCache := &concurrencyCacheMock{
		acquireUserSlotFn: func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
			return true, nil
		},
	}
	h := &OpenAIGatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billingService,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(concurrencyCache), SSEPingFormatNone, time.Second),
	}

	h.ChatCompletions(c)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	snapshot := getOpenAIRoutingSnapshot(c)
	require.NotNil(t, snapshot)
	require.Equal(t, "load_balance", snapshot.ScheduleLayer)
	require.Nil(t, snapshot.SelectedAccountID)
	snapshotStickyField := reflect.ValueOf(snapshot).Elem().FieldByName("Sticky")
	require.True(t, snapshotStickyField.IsValid())
	require.False(t, snapshotStickyField.IsNil())
	snapshotSticky := snapshotStickyField.Elem()
	require.Equal(t, "miss_no_binding", snapshotSticky.FieldByName("EvalResult").String())
	require.Equal(t, "header_x_session_affinity", snapshotSticky.FieldByName("SessionSource").String())
	require.True(t, snapshotSticky.FieldByName("SessionHashPresent").Bool())
}

func TestResponsesSelectionFailure_FirstAttemptNoAvailableUsesTargetGroup(t *testing.T) {
	action := classifyResponsesSelectionFailure(
		service.ErrNoAvailableAccounts,
		nil,
		0,
		nil,
	)
	require.Equal(t, responsesSelectionFailureActionTargetGroupAware, action)
}

func TestResponsesSelectionFailure_FailoverExhaustedPreservesFailoverSemantics(t *testing.T) {
	action := classifyResponsesSelectionFailure(
		service.ErrNoAvailableAccounts,
		nil,
		1,
		&service.UpstreamFailoverError{StatusCode: http.StatusBadGateway},
	)
	require.Equal(t, responsesSelectionFailureActionFailoverExhausted, action)
}

func TestResponsesSelectionFailure_NilSelectionUsesFailoverPathWhenAlreadyFailingOver(t *testing.T) {
	action := classifyResponsesSelectionFailure(
		nil,
		nil,
		1,
		&service.UpstreamFailoverError{StatusCode: http.StatusBadGateway},
	)
	require.Equal(t, responsesSelectionFailureActionFailoverExhausted, action)
}

func TestResponsesSelectionFailure_NilSelectionUsesTargetGroupWhenNotFailover(t *testing.T) {
	action := classifyResponsesSelectionFailure(
		nil,
		nil,
		0,
		nil,
	)
	require.Equal(t, responsesSelectionFailureActionTargetGroupAware, action)
}

func TestValidateFunctionCallOutputRequest_PreviousResponseIDUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.4","previous_response_id":"resp_prev_123","input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)

	ok := h.validateFunctionCallOutputRequest(c, body, zap.NewNop())
	require.False(t, ok)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "previous_response_id + function_call_output is not supported")
}

func TestOpenAIResponses_MissingDependencies_ReturnsServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5","stream":false}`))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID:      10,
		GroupID: &groupID,
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:      1,
		Concurrency: 1,
	})

	// 故意使用未初始化依赖，验证快速失败而不是崩溃。
	h := &OpenAIGatewayHandler{}
	require.NotPanics(t, func() {
		h.Responses(c)
	})

	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)

	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "api_error", errorObj["type"])
	assert.Equal(t, "Service temporarily unavailable", errorObj["message"])
}

func TestOpenAIResponses_SetsClientTransportHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-5"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h := &OpenAIGatewayHandler{}
	h.Responses(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Equal(t, service.OpenAIClientTransportHTTP, service.GetOpenAIClientTransport(c))
}

func TestOpenAIResponses_RejectsMessageIDAsPreviousResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(
		`{"model":"gpt-5.1","stream":false,"previous_response_id":"msg_123456","input":[{"type":"input_text","text":"hello"}]}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID:      101,
		GroupID: &groupID,
		User:    &service.User{ID: 1},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:      1,
		Concurrency: 1,
	})

	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	h.Responses(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "previous_response_id must be a response.id")
}

func TestOpenAIResponsesWebSocket_SetsClientTransportWSWhenUpgradeValid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil)
	c.Request.Header.Set("Upgrade", "websocket")
	c.Request.Header.Set("Connection", "Upgrade")

	h := &OpenAIGatewayHandler{}
	h.ResponsesWebSocket(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Equal(t, service.OpenAIClientTransportWS, service.GetOpenAIClientTransport(c))
}

func TestOpenAIResponsesWebSocket_InvalidUpgradeDoesNotSetTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil)

	h := &OpenAIGatewayHandler{}
	h.ResponsesWebSocket(c)

	require.Equal(t, http.StatusUpgradeRequired, w.Code)
	require.Equal(t, service.OpenAIClientTransportUnknown, service.GetOpenAIClientTransport(c))
}

func TestOpenAIResponsesWebSocket_RejectsMessageIDAsPreviousResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	wsServer := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() {
		_ = clientConn.CloseNow()
	}()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(
		`{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"msg_abc123"}`,
	))
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, _, err = clientConn.Read(readCtx)
	cancelRead()
	require.Error(t, err)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusPolicyViolation, closeErr.Code)
	require.Contains(t, strings.ToLower(closeErr.Reason), "previous_response_id")
}

func TestOpenAIResponsesWebSocket_PreviousResponseIDKindLoggedBeforeAcquireFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cache := &concurrencyCacheMock{
		acquireUserSlotFn: func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
			return false, errors.New("user slot unavailable")
		},
	}
	h := newOpenAIHandlerForPreviousResponseIDValidation(t, cache)
	wsServer := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() {
		_ = clientConn.CloseNow()
	}()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(
		`{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"resp_prev_123"}`,
	))
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, _, err = clientConn.Read(readCtx)
	cancelRead()
	require.Error(t, err)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusInternalError, closeErr.Code)
	require.Contains(t, strings.ToLower(closeErr.Reason), "failed to acquire user concurrency slot")
}

func TestSetOpenAIClientTransportHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	setOpenAIClientTransportHTTP(c)
	require.Equal(t, service.OpenAIClientTransportHTTP, service.GetOpenAIClientTransport(c))
}

func TestSetOpenAIClientTransportWS(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	setOpenAIClientTransportWS(c)
	require.Equal(t, service.OpenAIClientTransportWS, service.GetOpenAIClientTransport(c))
}

// TestOpenAIHandler_GjsonExtraction 验证 gjson 从请求体中提取 model/stream 的正确性
func TestOpenAIHandler_GjsonExtraction(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantModel  string
		wantStream bool
	}{
		{"正常提取", `{"model":"gpt-4","stream":true,"input":"hello"}`, "gpt-4", true},
		{"stream false", `{"model":"gpt-4","stream":false}`, "gpt-4", false},
		{"无 stream 字段", `{"model":"gpt-4"}`, "gpt-4", false},
		{"model 缺失", `{"stream":true}`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			modelResult := gjson.GetBytes(body, "model")
			model := ""
			if modelResult.Type == gjson.String {
				model = modelResult.String()
			}
			stream := gjson.GetBytes(body, "stream").Bool()
			require.Equal(t, tt.wantModel, model)
			require.Equal(t, tt.wantStream, stream)
		})
	}
}

// TestOpenAIHandler_GjsonValidation 验证修复后的 JSON 合法性和类型校验
func TestOpenAIHandler_GjsonValidation(t *testing.T) {
	// 非法 JSON 被 gjson.ValidBytes 拦截
	require.False(t, gjson.ValidBytes([]byte(`{invalid json`)))

	// model 为数字 → 类型不是 gjson.String，应被拒绝
	body := []byte(`{"model":123}`)
	modelResult := gjson.GetBytes(body, "model")
	require.True(t, modelResult.Exists())
	require.NotEqual(t, gjson.String, modelResult.Type)

	// model 为 null → 类型不是 gjson.String，应被拒绝
	body2 := []byte(`{"model":null}`)
	modelResult2 := gjson.GetBytes(body2, "model")
	require.True(t, modelResult2.Exists())
	require.NotEqual(t, gjson.String, modelResult2.Type)

	// stream 为 string → 类型既不是 True 也不是 False，应被拒绝
	body3 := []byte(`{"model":"gpt-4","stream":"true"}`)
	streamResult := gjson.GetBytes(body3, "stream")
	require.True(t, streamResult.Exists())
	require.NotEqual(t, gjson.True, streamResult.Type)
	require.NotEqual(t, gjson.False, streamResult.Type)

	// stream 为 int → 同上
	body4 := []byte(`{"model":"gpt-4","stream":1}`)
	streamResult2 := gjson.GetBytes(body4, "stream")
	require.True(t, streamResult2.Exists())
	require.NotEqual(t, gjson.True, streamResult2.Type)
	require.NotEqual(t, gjson.False, streamResult2.Type)
}

// TestOpenAIHandler_InstructionsInjection 验证 instructions 的 gjson/sjson 注入逻辑
func TestOpenAIHandler_InstructionsInjection(t *testing.T) {
	// 测试 1：无 instructions → 注入
	body := []byte(`{"model":"gpt-4"}`)
	existing := gjson.GetBytes(body, "instructions").String()
	require.Empty(t, existing)
	newBody, err := sjson.SetBytes(body, "instructions", "test instruction")
	require.NoError(t, err)
	require.Equal(t, "test instruction", gjson.GetBytes(newBody, "instructions").String())

	// 测试 2：已有 instructions → 不覆盖
	body2 := []byte(`{"model":"gpt-4","instructions":"existing"}`)
	existing2 := gjson.GetBytes(body2, "instructions").String()
	require.Equal(t, "existing", existing2)

	// 测试 3：空白 instructions → 注入
	body3 := []byte(`{"model":"gpt-4","instructions":"   "}`)
	existing3 := strings.TrimSpace(gjson.GetBytes(body3, "instructions").String())
	require.Empty(t, existing3)

	// 测试 4：sjson.SetBytes 返回错误时不应 panic
	// 正常 JSON 不会产生 sjson 错误，验证返回值被正确处理
	validBody := []byte(`{"model":"gpt-4"}`)
	result, setErr := sjson.SetBytes(validBody, "instructions", "hello")
	require.NoError(t, setErr)
	require.True(t, gjson.ValidBytes(result))
}

func newOpenAIHandlerForPreviousResponseIDValidation(t *testing.T, cache *concurrencyCacheMock) *OpenAIGatewayHandler {
	t.Helper()
	if cache == nil {
		cache = &concurrencyCacheMock{
			acquireUserSlotFn: func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
				return true, nil
			},
			acquireAccountSlotFn: func(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
				return true, nil
			},
		}
	}
	return &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: &service.BillingCacheService{},
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second),
	}
}

func newOpenAIWSHandlerTestServer(t *testing.T, h *OpenAIGatewayHandler, subject middleware.AuthSubject) *httptest.Server {
	t.Helper()
	groupID := int64(2)
	apiKey := &service.APIKey{
		ID:      101,
		GroupID: &groupID,
		User:    &service.User{ID: subject.UserID},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), subject)
		c.Next()
	})
	router.GET("/openai/v1/responses", h.ResponsesWebSocket)
	return httptest.NewServer(router)
}

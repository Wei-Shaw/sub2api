package middleware

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type auditRepoStub struct {
	items chan *service.AuditLog
}

func (r *auditRepoStub) Create(ctx context.Context, log *service.AuditLog) error {
	select {
	case r.items <- log:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (r *auditRepoStub) List(context.Context, service.AuditLogFilter) ([]service.AuditLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func TestAuditResponseWriterFlushDelegates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	writer := &auditResponseWriter{
		ResponseWriter: c.Writer,
		limit:          1024,
	}
	c.Writer = writer

	_, err := c.Writer.WriteString("data: hello\n\n")
	require.NoError(t, err)
	c.Writer.Flush()

	require.True(t, rec.Flushed)
	require.Equal(t, "data: hello\n\n", rec.Body.String())
	require.Equal(t, "data: hello\n\n", writer.buf.String())
}

func TestAuditResponseWriterReadFromCapturesAndForwards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	writer := &auditResponseWriter{
		ResponseWriter: c.Writer,
		limit:          1024,
	}

	n, err := writer.ReadFrom(strings.NewReader("stream body"))
	require.NoError(t, err)
	require.Equal(t, int64(len("stream body")), n)
	require.Equal(t, "stream body", rec.Body.String())
	require.Equal(t, "stream body", writer.buf.String())
}

func TestNormalizeAuditResponseBodyAnthropicSSE(t *testing.T) {
	raw := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"content":[]}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hello "}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"world"}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")

	got := normalizeAuditResponseBody([]byte(raw), "text/event-stream")
	require.Equal(t, "hello world", got)
}

func TestNormalizeAuditResponseBodyOpenAIResponsesSSE(t *testing.T) {
	raw := strings.Join([]string{
		`data: {"type":"response.created","response":{"status":"in_progress"}}`,
		``,
		`data: {"type":"response.output_text.delta","delta":"hel"}`,
		``,
		`data: {"type":"response.output_text.delta","delta":"lo"}`,
		``,
		`data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}]}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	got := normalizeAuditResponseBody([]byte(raw), "text/event-stream")
	require.Equal(t, "hello", got)
}

func TestNormalizeAuditResponseBodyLeavesPlainJSON(t *testing.T) {
	body := []byte(`{"message":"ok"}`)
	got := normalizeAuditResponseBody(body, http.DetectContentType(body))
	require.Equal(t, string(body), got)
}

func TestAuditCaptureDetectsLargeStreamingRequestFromFullBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &auditRepoStub{items: make(chan *service.AuditLog, 1)}
	router := gin.New()
	router.Use(AuditCapture(service.NewAuditService(repo)))
	router.POST("/", func(c *gin.Context) {
		c.Set("audit_response_body", "complete response")
		c.Header("Content-Type", "text/event-stream")
		_, _ = c.Writer.WriteString("event: message_stop\n")
		_, _ = c.Writer.WriteString(`data: {"type":"message_stop"}`)
		_, _ = c.Writer.WriteString("\n\n")
	})

	body := `{"messages":[{"role":"user","content":"` + strings.Repeat("x", service.AuditCaptureMaxBytes) + `"}],"stream":true,"model":"claude"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Claude-Code-Session-Id", "session-large")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	select {
	case item := <-repo.items:
		require.False(t, item.RequestTruncated)
		require.False(t, item.ResponseTruncated)
		require.Equal(t, strings.Repeat("x", service.AuditCaptureMaxBytes), item.RequestBody)
		require.Equal(t, "complete response", item.ResponseBody)
		require.Equal(t, "claude", item.Model)
		require.Equal(t, "session-large", item.SessionID)
	case <-time.After(time.Second):
		t.Fatal("audit log was not created")
	}
}

func TestExtractAuditUserInputUsesFullBodyAndLatestUserText(t *testing.T) {
	body := `{"model":"deepseek-v4-pro","messages":[{"role":"user","content":[{"type":"text","text":"<system-reminder>ignore</system-reminder>"},{"type":"text","text":"hi"}]},{"role":"assistant","content":[{"type":"text","text":"` + strings.Repeat("assistant history ", 5000) + `"}]},{"role":"user","content":[{"type":"text","text":"最后的问题"}]}],"system":[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude."}]}`

	got := extractAuditUserInput([]byte(body))
	require.Equal(t, "最后的问题", got)

	captured, raw, truncated := readAndRestoreRequestBody(&gin.Context{Request: httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))}, service.AuditCaptureMaxBytes)
	auditBody, auditTruncated := auditRequestBody(raw, captured, truncated)
	require.True(t, truncated)
	require.False(t, auditTruncated)
	require.Equal(t, "最后的问题", auditBody)
	require.NotContains(t, auditBody, "assistant answer")
	require.NotContains(t, auditBody, "Claude Code")
}

func TestExtractAuditUserInputPrefersLatestUserTextOverSessionTag(t *testing.T) {
	body := `{"model":"deepseek-v4-pro","messages":[{"role":"user","content":[{"type":"text","text":"<session>\n之前的问题\n</session>"}]},{"role":"assistant","content":[{"type":"text","text":"历史回复"}]},{"role":"user","content":[{"type":"text","text":"啊？"}]}],"system":[{"type":"text","text":"x-anthropic-billing-header: cc_entrypoint=cli"}]}`

	got := extractAuditUserInput([]byte(body))
	require.Equal(t, "啊？", got)
}

func TestAuditCaptureRestoresWrappedWriterForOuterMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &auditRepoStub{items: make(chan *service.AuditLog, 1)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Next()
		_, ok := c.Writer.(*auditResponseWriter)
		require.False(t, ok, "audit middleware must restore the original writer before outer middleware resumes")
	})
	router.Use(AuditCapture(service.NewAuditService(repo)))
	router.POST("/", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"claude"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestExtractAuditSessionIDPrefersClaudeCodeHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Request.Header.Set("X-Claude-Code-Session-Id", "claude-session-123")

	got := extractAuditSessionID(c, []byte(`{"metadata":{"user_id":"other"}}`))
	require.Equal(t, "claude-session-123", got)
}

func TestExtractAuditSessionIDFromMetadataUserIDJSON(t *testing.T) {
	got := extractAuditSessionID(nil, []byte(`{"metadata":{"user_id":"{\"device_id\":\"d\",\"session_id\":\"json-session-123\"}"}}`))
	require.Equal(t, "json-session-123", got)
}

func TestExtractAuditSessionIDFromClaudeCodeSessionTag(t *testing.T) {
	got := extractAuditSessionID(nil, []byte(`{"messages":[{"content":[{"text":"<session>\n用python写个彩虹\n</session>"}]}]}`))
	require.NotEmpty(t, got)
	require.True(t, strings.HasPrefix(got, "tag:"))

	again := extractAuditSessionID(nil, []byte(`{"messages":[{"content":[{"text":"<session>\n用python写个彩虹\n</session>"}]}],"model":"claude"}`))
	require.Equal(t, got, again)
}

func TestFallbackAuditSessionIDUsesClaudeCodeSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Request.Header.Set("User-Agent", "claude-cli/2.1.144 (external, cli)")
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"text","text":"hello","signature":"sig-session-123"}]}]}`)

	got := fallbackAuditSessionID(c, body)
	require.NotEmpty(t, got)
	require.True(t, strings.HasPrefix(got, "claude-signature:"))

	again := fallbackAuditSessionID(c, []byte(`{"messages":[{"content":[{"signature":"sig-session-123"}]}],"model":"deepseek-v4-pro"}`))
	require.Equal(t, got, again)
}

var _ io.ReaderFrom = (*auditResponseWriter)(nil)

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func runAnthropicVisibleOutputStream(t *testing.T, payload string) (*streamingResult, error, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	svc := &GatewayService{
		cfg:              &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		rateLimitService: &RateLimitService{},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"attempt-private"}},
		Body:       io.NopCloser(strings.NewReader(payload)),
	}
	defer resp.Body.Close()
	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "public-model", "upstream-model", false)
	return result, err, recorder
}

// 服务端搜索不是客户端答案或待执行的工具调用；其 input delta 不能
// 解除空可见输出保护，阻断账号重试。
func TestAnthropicVisibleOutput_ServerToolInputDoesNotCommitAttempt(t *testing.T) {
	payload := "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":7}}}\n\n" +
		"data: {\"type\":\"content_block_start\",\"index\":2,\"content_block\":{\"type\":\"server_tool_use\",\"id\":\"srvtoolu_1\",\"name\":\"web_search\",\"input\":{\"query\":\"weather\"}}}\n\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":2,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"query\\\":\\\"weather\\\"}\"}}\n\n" +
		"data: {\"type\":\"content_block_stop\",\"index\":2}\n\n" +
		"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":12}}\n\n" +
		"data: {\"type\":\"message_stop\"}\n\n"

	result, err, recorder := runAnthropicVisibleOutputStream(t, payload)

	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Nil(t, result, "failed attempts must not become a second bill when account failover succeeds")
	require.Equal(t, http.StatusBadGateway, failover.StatusCode)
	require.True(t, failover.SafeToFailoverAfterWrite)
	require.Contains(t, string(failover.ResponseBody), "empty_visible_output")
	require.Empty(t, recorder.Body.String(), "do not leak staged server-tool or usage events")
	require.Empty(t, recorder.Header().Values("X-Request-Id"), "failed-attempt headers must remain private")
}

func TestAnthropicVisibleOutput_StagingOverflowReturnsFailoverWithoutPartialOutput(t *testing.T) {
	const oneMiB = 1024 * 1024
	thinking := "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"" + strings.Repeat("x", oneMiB) + "\"}}\n\n"
	payload := "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\"}}\n\n" +
		strings.Repeat(thinking, 9)

	result, err, recorder := runAnthropicVisibleOutputStream(t, payload)

	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadGateway, failover.StatusCode)
	require.True(t, failover.SafeToFailoverAfterWrite)
	require.Contains(t, string(failover.ResponseBody), "first_visible_output_buffer_overflow")
	require.Empty(t, recorder.Body.String(), "overflow must discard the entire attempt, not flush a truncated SSE prefix")
	require.Empty(t, recorder.Header().Values("X-Request-Id"))
}

func TestAnthropicVisibleOutput_TerminalGraceCollectsLateUsageWithoutWaitingForEOF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	svc := &GatewayService{
		cfg:              &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		rateLimitService: &RateLimitService{},
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: reader}
	go func() {
		_, _ = io.WriteString(writer, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"answer\"}}\n\n"+
			"data: {\"type\":\"message_stop\"}\n\n")
		time.Sleep(40 * time.Millisecond)
		_, _ = io.WriteString(writer, "data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":37}}\n\n")
		// 后续心跳和重复终态不能重新计时、延长排水窗口。
		for i := 0; i < 4; i++ {
			time.Sleep(80 * time.Millisecond)
			if _, err := io.WriteString(writer, "data: {\"type\":\"ping\"}\n\n"+"data: {\"type\":\"message_stop\"}\n\n"); err != nil {
				return
			}
		}
	}()
	type outcome struct {
		result *streamingResult
		err    error
	}
	done := make(chan outcome, 1)
	start := time.Now()
	go func() {
		result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, start, "model", "model", false)
		done <- outcome{result, err}
	}()
	select {
	case got := <-done:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.Equal(t, 37, got.result.usage.OutputTokens)
		require.GreaterOrEqual(t, time.Since(start), 450*time.Millisecond, "terminal must leave room for trailing usage")
		require.Less(t, time.Since(start), 800*time.Millisecond, "later ping/stop must not extend the 500ms grace")
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("terminal grace must complete a stream without upstream EOF")
	}
}

type anthropicVisibleOutputObservedBody struct {
	io.ReadCloser
	closed       chan struct{}
	readExited   chan struct{}
	closeOnce    sync.Once
	readExitOnce sync.Once
}

func (b *anthropicVisibleOutputObservedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		// 放大关闭与读协程退出之间的间隔，避免测试只证明 Close 被调用。
		time.Sleep(20 * time.Millisecond)
		b.readExitOnce.Do(func() { close(b.readExited) })
	}
	return n, err
}

func (b *anthropicVisibleOutputObservedBody) Close() error {
	var err error
	b.closeOnce.Do(func() {
		err = b.ReadCloser.Close()
		close(b.closed)
	})
	return err
}

func TestAnthropicVisibleOutput_TerminalExitClosesBodyAndWaitsForReader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	svc := &GatewayService{
		cfg:              &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		rateLimitService: &RateLimitService{},
	}
	reader, writer := io.Pipe()
	body := &anthropicVisibleOutputObservedBody{
		ReadCloser: reader,
		closed:     make(chan struct{}),
		readExited: make(chan struct{}),
	}
	defer body.Close()
	defer writer.Close()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}
	go func() {
		_, _ = io.WriteString(writer, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"answer\"}}\n\n"+
			"data: {\"type\":\"message_stop\"}\n\n")
		// 不关闭 writer：终态窗口退出必须自行终止尚在阻塞的上游读取。
	}()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	require.NoError(t, err)
	require.NotNil(t, result)
	select {
	case <-body.closed:
	default:
		t.Fatal("terminal exit must close its upstream body without waiting for the caller")
	}
	select {
	case <-body.readExited:
	default:
		t.Fatal("terminal exit must join the upstream reader after closing the body")
	}
}

func TestAnthropicVisibleOutput_TerminalDeadlineEndsContinuousPingStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	svc := &GatewayService{
		cfg:              &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		rateLimitService: &RateLimitService{},
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: reader}
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		_, _ = io.WriteString(writer, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"answer\"}}\n\n"+
			"data: {\"type\":\"message_stop\"}\n\n"+
			"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":37}}\n\n")
		// 保持队列持续可读，不能依赖 select 随机挑中 timer 才结束窗口。
		for {
			if _, err := io.WriteString(writer, "data: {\"type\":\"ping\"}\n\n"+"data: {\"type\":\"message_stop\"}\n\n"); err != nil {
				return
			}
		}
	}()

	start := time.Now()
	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, start, "model", "model", false)
	require.NoError(t, err)
	require.Equal(t, 37, result.usage.OutputTokens)
	require.GreaterOrEqual(t, time.Since(start), 450*time.Millisecond)
	require.Less(t, time.Since(start), 800*time.Millisecond, "持续 ping/重复 stop 不能延长固定尾窗口")
	select {
	case <-writerDone:
	case <-time.After(time.Second):
		t.Fatal("upstream producer must stop when terminal deadline closes the body")
	}
}

func TestAnthropicVisibleOutput_BeforeVisibleHeartbeatIsOnlyTransportComment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("User-Agent", "claude-cli/2.1.198 (external, cli)")
	svc := &GatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{
			MaxLineSize:             defaultMaxLineSize,
			StreamKeepaliveInterval: 1,
		}},
		rateLimitService: &RateLimitService{},
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"X-Request-Id": []string{"attempt-private"}}, Body: reader}
	const raw = `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`
	go func() {
		defer writer.Close()
		_, _ = io.WriteString(writer, "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":7}}}\n\n"+
			"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\"}}\n\n")
		time.Sleep(1100 * time.Millisecond)
		_, _ = io.WriteString(writer, "event: error\ndata: "+raw+"\n\n")
	}()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	var streamErr *sseStreamErrorEventError
	require.ErrorAs(t, err, &streamErr)
	require.Nil(t, result)
	require.Equal(t, raw, streamErr.RawData)
	require.True(t, streamErr.SafeToFailoverAfterWrite, "传输注释不能阻断账号重试")
	require.Equal(t, ": ping\n\n", recorder.Body.String(), "首可见输出前不能发 metadata、event:ping 或 noop delta")
	require.Empty(t, recorder.Header().Values("X-Request-Id"))
	require.Empty(t, recorder.Result().Header.Values("X-Request-Id"), "已经提交的头也不能泄露失败账号的 request-id")
}

func TestAnthropicVisibleOutput_InterleavedIndexesDoNotBorrowClientToolType(t *testing.T) {
	payload := "data: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"lookup\",\"input\":{}}}\n\n" +
		"data: {\"type\":\"content_block_start\",\"index\":2,\"content_block\":{\"type\":\"server_tool_use\",\"id\":\"srvtoolu_1\",\"name\":\"web_search\",\"input\":{}}}\n\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":2,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"query\\\":\\\"status\\\"}\"}}\n\n" +
		"data: {\"type\":\"content_block_stop\",\"index\":2}\n\n" +
		"data: {\"type\":\"message_stop\"}\n\n"
	_, err, recorder := runAnthropicVisibleOutputStream(t, payload)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Empty(t, recorder.Body.String(), "server-tool input must not borrow a different index's client-tool type")
}

func TestAnthropicVisibleOutput_ClientTextAndToolReleaseStagedEvents(t *testing.T) {
	cases := []struct {
		name   string
		events string
		want   string
	}{
		{"text", "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"answer\"}}\n\n", "answer"},
		{"tool input", "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"lookup\",\"input\":{}}}\n\n" +
			"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"query\\\":\\\"status\\\"}\"}}\n\n", "input_json_delta"},
		{"zero argument tool", "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"clock\",\"input\":{}}}\n\n" +
			"data: {\"type\":\"content_block_stop\",\"index\":0}\n\n", "clock"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := "data: {\"type\":\"message_start\",\"message\":{\"model\":\"upstream-model\",\"usage\":{\"input_tokens\":7}}}\n\n" +
				tc.events + "data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":11}}\n\n" +
				"data: {\"type\":\"message_stop\"}\n\n"
			result, err, recorder := runAnthropicVisibleOutputStream(t, payload)
			require.NoError(t, err)
			require.NotNil(t, result.firstTokenMs)
			require.Equal(t, 11, result.usage.OutputTokens)
			require.Contains(t, recorder.Body.String(), "message_start")
			require.Contains(t, recorder.Body.String(), tc.want)
			require.Contains(t, recorder.Body.String(), "public-model")
			require.Equal(t, "attempt-private", recorder.Header().Get("X-Request-Id"))
		})
	}
}

func TestAnthropicVisibleOutput_EmptyThinkingAndMetadataDoNotSucceed(t *testing.T) {
	cases := []struct {
		name   string
		events string
	}{
		{"empty 200", ""},
		{"metadata only", "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":7}}}\n\n"},
		{"thinking only", "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\"}}\n\n" +
			"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"plan\"}}\n\n"},
		{"empty text", "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"\"}}\n\n"},
		{"unknown tool index", "data: {\"type\":\"content_block_delta\",\"index\":4,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{}\"}}\n\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := tc.events
			if payload != "" {
				payload += "data: {\"type\":\"message_stop\"}\n\n"
			}
			result, err, recorder := runAnthropicVisibleOutputStream(t, payload)
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.Nil(t, result)
			require.True(t, failover.SafeToFailoverAfterWrite)
			require.Empty(t, recorder.Body.String())
			require.Empty(t, recorder.Header().Values("X-Request-Id"))
		})
	}
}

func TestAnthropicVisibleOutput_LargeLineAfterVisibleOutputUsesConfiguredLimit(t *testing.T) {
	// 8 MiB 仅约束首次可见输出前的缓冲，提交正文后的合法大行仍用 MaxLineSize。
	firstText := strings.Repeat("a", 7*1024*1024)
	laterText := strings.Repeat("b", 9*1024*1024)
	payload := "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"" + firstText + "\"}}\n\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"" + laterText + "\"}}\n\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":37}}\n\n" +
		"data: {\"type\":\"message_stop\"}\n\n"

	result, err, recorder := runAnthropicVisibleOutputStream(t, payload)
	require.NoError(t, err, "读取协程提前扫描后续大行，不能误用首可见输出前的限制")
	require.Equal(t, 37, result.usage.OutputTokens)
	require.Contains(t, recorder.Body.String(), laterText)
}

func TestAnthropicVisibleOutput_OversizedSingleLineIsBoundedBeforeCommit(t *testing.T) {
	payload := "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"" + strings.Repeat("x", 8*1024*1024+1)
	result, err, recorder := runAnthropicVisibleOutputStream(t, payload)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Nil(t, result)
	require.Contains(t, string(failover.ResponseBody), "first_visible_output_buffer_overflow")
	require.True(t, failover.SafeToFailoverAfterWrite)
	require.Empty(t, recorder.Body.String())
}

func TestAnthropicVisibleOutput_SSEErrorsKeepTypedUpstreamClassification(t *testing.T) {
	const raw = `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`
	for _, prefix := range []string{"event: error\n", ""} {
		t.Run(prefix, func(t *testing.T) {
			payload := "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":7}}}\n\n" + prefix + "data: " + raw + "\n\n"
			result, err, recorder := runAnthropicVisibleOutputStream(t, payload)
			var streamErr *sseStreamErrorEventError
			require.ErrorAs(t, err, &streamErr)
			require.Equal(t, raw, streamErr.RawData)
			require.True(t, streamErr.SafeToFailoverAfterWrite)
			require.Nil(t, result)
			require.Empty(t, recorder.Body.String())
		})
	}
}

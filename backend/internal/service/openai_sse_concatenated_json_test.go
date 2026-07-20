package service

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIStreamingRepairsConcatenatedJSONDocumentsInSingleDataLine(t *testing.T) {
	testOpenAIStreamingRepairsConcatenatedJSONDocuments(t, false, 0)
}

func TestOpenAIStreamingAsyncScannerRepairsConcatenatedJSONDocumentsInSingleDataLine(t *testing.T) {
	testOpenAIStreamingRepairsConcatenatedJSONDocuments(t, false, 30)
}

func TestOpenAIStreamingPassthroughRepairsConcatenatedJSONDocumentsInSingleDataLine(t *testing.T) {
	testOpenAIStreamingRepairsConcatenatedJSONDocuments(t, true, 0)
}

func TestOpenAIWSv2StreamingRepairsConcatenatedJSONDocumentsInSingleMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	largeInProgress, outputItemAdded, completed := openAIConcatenatedJSONTestEvents(t)
	captureConn := &openAIWSCaptureConn{events: [][]byte{
		[]byte(largeInProgress + outputItemAdded),
		[]byte(completed),
	}}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: captureConn})
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		cache:            &stubGatewayCache{},
		httpUpstream:     &httpUpstreamRecorder{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		openaiWSPool:     pool,
		toolCorrector:    NewCodexToolCorrector(),
	}
	account := &Account{
		ID:          2,
		Name:        "ws-test",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	groupID := int64(1)
	c.Set("api_key", &APIKey{GroupID: &groupID})

	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, 9, result.Usage.OutputTokens)
	require.Nil(t, result.FirstTokenMs)
	assertOpenAISSEFrames(t, recorder.Body.String(), []string{
		"response.in_progress",
		"response.output_item.added",
		"response.completed",
	})
}

func TestOpenAIWSv2RejectsMalformedTypedEventBeforeWritingDownstream(t *testing.T) {
	largeInProgress, _, _ := openAIConcatenatedJSONTestEvents(t)
	testOpenAIWSv2RejectsMalformedEventBeforeWritingDownstream(t, []byte(largeInProgress+"unexpected-tail"))
}

func TestOpenAIWSv2RejectsMalformedUntypedMessageBeforeWritingDownstream(t *testing.T) {
	testOpenAIWSv2RejectsMalformedEventBeforeWritingDownstream(t, []byte("not-json"))
}

func TestOpenAIWSv2RejectsMalformedEventAfterWritingDownstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	outputTextDelta := `{"type":"response.output_text.delta","delta":"ok","sequence_number":1}`
	malformedMessage := `{"type":"response.in_progress"}unexpected-tail`
	captureConn := &openAIWSCaptureConn{events: [][]byte{
		[]byte(outputTextDelta),
		[]byte(malformedMessage),
	}}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: captureConn})
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		cache:            &stubGatewayCache{},
		httpUpstream:     &httpUpstreamRecorder{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		openaiWSPool:     pool,
		toolCorrector:    NewCodexToolCorrector(),
	}
	account := &Account{
		ID:          5,
		Name:        "ws-malformed-event-after-output",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	groupID := int64(1)
	c.Set("api_key", &APIKey{GroupID: &groupID})

	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "after downstream output")
	require.Nil(t, result)
	require.True(t, captureConn.closed)
	require.Contains(t, recorder.Body.String(), `"delta":"ok"`)
	require.NotContains(t, recorder.Body.String(), "unexpected-tail")
	require.NotContains(t, recorder.Body.String(), "response.in_progress")
	assertOpenAISSEFrames(t, recorder.Body.String(), []string{"response.output_text.delta"})
}

func testOpenAIWSv2RejectsMalformedEventBeforeWritingDownstream(t *testing.T, malformedMessage []byte) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	_, _, completed := openAIConcatenatedJSONTestEvents(t)
	outputTextDelta := `{"type":"response.output_text.delta","delta":"ok","sequence_number":3}`
	captureConn := &openAIWSCaptureConn{events: [][]byte{
		malformedMessage,
		[]byte(outputTextDelta),
		[]byte(completed),
	}}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: captureConn})
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		cache:            &stubGatewayCache{},
		httpUpstream:     &httpUpstreamRecorder{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		openaiWSPool:     pool,
		toolCorrector:    NewCodexToolCorrector(),
	}
	account := &Account{
		ID:          4,
		Name:        "ws-malformed-event",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	groupID := int64(1)
	c.Set("api_key", &APIKey{GroupID: &groupID})

	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	require.Error(t, err)
	var fallbackErr *openAIWSFallbackError
	require.ErrorAs(t, err, &fallbackErr)
	require.Equal(t, "invalid_event_json", fallbackErr.Reason)
	require.Nil(t, result)
	require.Empty(t, recorder.Body.String())
	require.True(t, captureConn.closed)
}

func TestSplitOpenAIConcatenatedJSONDocumentsRejectsPayloadOverRepairLimit(t *testing.T) {
	first := `{"type":"response.in_progress","padding":"` + strings.Repeat("x", 16*1024*1024) + `"}`
	second := `{"type":"response.completed"}`
	payload := first + second

	documents, repaired := splitOpenAIConcatenatedJSONDocuments([]byte(payload))
	require.False(t, repaired)
	require.Nil(t, documents)

	line := "data: " + payload
	scanner := bufio.NewScanner(strings.NewReader(line))
	scanner.Buffer(make([]byte, 1024), len(line)+1)
	documentScanner := newOpenAISSEJSONDocumentScanner(scanner)
	require.True(t, documentScanner.Scan())
	require.Equal(t, line, documentScanner.Text())
	require.False(t, documentScanner.Scan())
	require.NoError(t, documentScanner.Err())
}

func TestOpenAIWSv2StreamingBreaksConnectionWhenTerminalHasTrailingDocument(t *testing.T) {
	gin.SetMode(gin.TestMode)
	completed := `{"type":"response.completed","response":{"id":"resp_terminal_tail","usage":{"input_tokens":2,"output_tokens":1}}}`
	tail := `{"type":"error","error":{"type":"upstream_error","message":"tail"}}`
	captureConn := &openAIWSCaptureConn{events: [][]byte{[]byte(completed + tail)}}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: captureConn})
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		cache:            &stubGatewayCache{},
		httpUpstream:     &httpUpstreamRecorder{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		openaiWSPool:     pool,
		toolCorrector:    NewCodexToolCorrector(),
	}
	account := &Account{
		ID:          3,
		Name:        "ws-terminal-tail",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	groupID := int64(1)
	c.Set("api_key", &APIKey{GroupID: &groupID})

	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, captureConn.closed, "a WS message with data after a terminal event must not return to the pool")
	assertOpenAISSEFrames(t, recorder.Body.String(), []string{"response.completed"})
}

func testOpenAIStreamingRepairsConcatenatedJSONDocuments(t *testing.T, passthrough bool, streamDataIntervalTimeout int) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	largeInProgress, outputItemAdded, completed := openAIConcatenatedJSONTestEvents(t)

	upstreamBody := strings.Join([]string{
		"event: response.in_progress",
		"data: " + largeInProgress + outputItemAdded,
		"",
		"event: response.completed",
		"data: " + completed,
		"",
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{
			MaxLineSize:               defaultMaxLineSize,
			StreamDataIntervalTimeout: streamDataIntervalTimeout,
		}},
		toolCorrector: NewCodexToolCorrector(),
	}
	account := &Account{ID: 1, Name: "test", Platform: PlatformOpenAI}

	var usage *OpenAIUsage
	var err error
	if passthrough {
		result, forwardErr := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "gpt-5.6-sol", "gpt-5.6-sol")
		err = forwardErr
		if result != nil {
			usage = result.usage
		}
	} else {
		result, forwardErr := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-5.6-sol", "gpt-5.6-sol")
		err = forwardErr
		if result != nil {
			usage = result.usage
		}
	}
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 7, usage.InputTokens)
	require.Equal(t, 9, usage.OutputTokens)

	assertOpenAISSEFrames(t, recorder.Body.String(), []string{
		"response.in_progress",
		"response.output_item.added",
		"response.completed",
	})
}

func assertOpenAISSEFrames(t *testing.T, body string, expectedTypes []string) {
	t.Helper()
	var parser openAICompatSSEFrameParser
	var eventTypes []string
	for _, line := range strings.Split(body, "\n") {
		frame, ok := parser.AddLine(strings.TrimSuffix(line, "\r"))
		if !ok {
			continue
		}
		require.True(t, json.Valid([]byte(frame.Data)), "each downstream SSE frame must contain exactly one JSON document")
		var event struct {
			Type string `json:"type"`
		}
		require.NoError(t, json.Unmarshal([]byte(frame.Data), &event))
		if frame.EventType != "" {
			require.Equal(t, event.Type, frame.EventType)
		}
		eventTypes = append(eventTypes, event.Type)
	}
	if frame, ok := parser.Finish(); ok {
		require.True(t, json.Valid([]byte(frame.Data)))
		var event struct {
			Type string `json:"type"`
		}
		require.NoError(t, json.Unmarshal([]byte(frame.Data), &event))
		eventTypes = append(eventTypes, event.Type)
	}
	require.Equal(t, expectedTypes, eventTypes)
}

func openAIConcatenatedJSONTestEvents(t *testing.T) (string, string, string) {
	t.Helper()
	const javascriptErrorPosition = 68106
	prefix := `{"type":"response.in_progress","response":{"id":"resp_large","status":"in_progress","instructions":"`
	suffix := `"},"sequence_number":1}`
	require.Less(t, len(prefix)+len(suffix), javascriptErrorPosition)
	largeInProgress := prefix + strings.Repeat("x", javascriptErrorPosition-len(prefix)-len(suffix)) + suffix
	outputItemAdded := `{"type":"response.output_item.added","output_index":0,"item":{"id":"msg_1","type":"message","role":"assistant","status":"in_progress","content":[]},"sequence_number":2}`
	completed := `{"type":"response.completed","response":{"id":"resp_large","status":"completed","output":[],"usage":{"input_tokens":7,"output_tokens":9}},"sequence_number":3}`
	require.Len(t, largeInProgress, javascriptErrorPosition)
	require.True(t, json.Valid([]byte(largeInProgress)))
	var decoded any
	err := json.Unmarshal([]byte(largeInProgress+outputItemAdded), &decoded)
	var syntaxErr *json.SyntaxError
	require.ErrorAs(t, err, &syntaxErr)
	require.Equal(t, int64(javascriptErrorPosition+1), syntaxErr.Offset)
	return largeInProgress, outputItemAdded, completed
}

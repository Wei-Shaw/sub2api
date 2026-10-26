//go:build unit

package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// disconnectOnMarkerResponseWriter 在首次写入包含 marker 的数据时返回错误，模拟客户端中途断开。
type disconnectOnMarkerResponseWriter struct {
	*httptest.ResponseRecorder
	marker            string
	failed            bool
	writesAfterError  int
	flushesAfterError int
}

func (writer *disconnectOnMarkerResponseWriter) Write(data []byte) (int, error) {
	if writer.failed {
		writer.writesAfterError++
		return 0, errors.New("client disconnected")
	}
	if strings.Contains(string(data), writer.marker) {
		writer.failed = true
		return 0, errors.New("client disconnected")
	}
	return writer.ResponseRecorder.Write(data)
}

func (writer *disconnectOnMarkerResponseWriter) WriteString(data string) (int, error) {
	return writer.Write([]byte(data))
}

func (writer *disconnectOnMarkerResponseWriter) Flush() {
	if writer.failed {
		writer.flushesAfterError++
	}
	writer.ResponseRecorder.Flush()
}

func anthropicStreamWithMidStreamText() string {
	return strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_disc","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","usage":{"input_tokens":20,"cache_read_input_tokens":11,"cache_creation_input_tokens":4,"output_tokens":1}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"first"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"second"}}`,
		``,
		`event: ping`,
		`data: {"type":"ping"}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":42}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
}

// stallingAnthropicUpstream 写完 stream 后保持连接不关闭，模拟上游挂住。
func stallingAnthropicUpstream(t *testing.T, stream string) (*http.Response, *io.PipeWriter) {
	t.Helper()
	pr, pw := io.Pipe()
	go func() {
		_, _ = io.WriteString(pw, stream)
	}()
	t.Cleanup(func() {
		_ = pw.Close()
		_ = pr.Close()
	})
	return &http.Response{Header: http.Header{}, Body: pr}, pw
}

func newCompatStreamIdleTestService(intervalSec int) *GatewayService {
	return &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: intervalSec}}}
}

type compatStreamHandler func(*GatewayService, *http.Response, *gin.Context) (*ForwardResult, error)

func compatStreamHandlers() map[string]compatStreamHandler {
	return map[string]compatStreamHandler{
		"chat_completions": func(svc *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
			return svc.handleCCStreamingFromAnthropic(resp, c, "claude-sonnet-4-6", "claude-sonnet-4-6", nil, time.Now())
		},
		"responses": func(svc *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
			return svc.handleResponsesStreamingResponse(resp, c, "claude-sonnet-4-6", "claude-sonnet-4-6", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
		},
	}
}

// compatStreamTerminalMarkers 是两条兼容流按成功收尾时写出的完成标记。
var compatStreamTerminalMarkers = map[string]string{
	"chat_completions": "[DONE]",
	"responses":        "response.completed",
}

// compatStreamFailureMarkers 是两条兼容流写出终止错误时的标记。
var compatStreamFailureMarkers = map[string]string{
	"chat_completions": `data: {"error":`,
	"responses":        "event: response.failed",
}

// anthropicStreamWithoutMessageStop 以带 stop_reason 的 message_delta 结束，不含 message_stop。
func anthropicStreamWithoutMessageStop() string {
	return strings.TrimSuffix(anthropicStreamWithMidStreamText(), "event: message_stop\ndata: {\"type\":\"message_stop\"}\n")
}

// anthropicStreamTruncatedBeforeMessageDelta 在 message_delta 之前中断，usage 只有 message_start 的部分。
func anthropicStreamTruncatedBeforeMessageDelta() string {
	full := anthropicStreamWithMidStreamText()
	return full[:strings.Index(full, "event: message_delta")]
}

type compatStreamOutcome struct {
	result *ForwardResult
	err    error
}

func runCompatStreamWithDeadline(t *testing.T, handle compatStreamHandler, svc *GatewayService, resp *http.Response, c *gin.Context) compatStreamOutcome {
	t.Helper()
	done := make(chan compatStreamOutcome, 1)
	go func() {
		result, err := handle(svc, resp, c)
		done <- compatStreamOutcome{result: result, err: err}
	}()
	select {
	case got := <-done:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not end after upstream idle interval")
		return compatStreamOutcome{}
	}
}

func TestAnthropicCompatStreaming_IdleUpstreamEndsWithinInterval(t *testing.T) {
	for name, handle := range compatStreamHandlers() {
		for _, disconnect := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/client_disconnected=%t", name, disconnect), func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				marker := "\x00never-written"
				if disconnect {
					marker = "second"
				}
				writer := &disconnectOnMarkerResponseWriter{ResponseRecorder: httptest.NewRecorder(), marker: marker}
				c, _ := gin.CreateTestContext(writer)
				resp, pw := stallingAnthropicUpstream(t, anthropicStreamTruncatedBeforeMessageDelta())

				got := runCompatStreamWithDeadline(t, handle, newCompatStreamIdleTestService(1), resp, c)

				if disconnect {
					require.NoError(t, got.err)
				} else {
					require.ErrorIs(t, got.err, errAnthropicNativeStreamIdle)
					body := writer.Body.String()
					require.NotContains(t, body, compatStreamTerminalMarkers[name], "a stream cut by idle timeout must not be reported as completed")
					require.Contains(t, body, compatStreamFailureMarkers[name])
					require.Contains(t, body, "stream_timeout")
					require.True(t, IsResponseCommitted(c))
				}
				require.NotNil(t, got.result)
				require.Equal(t, disconnect, got.result.ClientDisconnect)
				require.Equal(t, disconnect, writer.failed)
				require.Equal(t, 20, got.result.Usage.InputTokens)
				require.Equal(t, 1, got.result.Usage.OutputTokens)
				require.Zero(t, writer.writesAfterError)
				_, werr := io.WriteString(pw, "event: ping\n")
				require.Error(t, werr, "upstream body must be closed after idle timeout")
			})
		}
	}
}

func TestAnthropicCompatStreaming_IdleAfterTerminalEventCompletes(t *testing.T) {
	for name, handle := range compatStreamHandlers() {
		for streamName, stream := range map[string]string{
			"message_stop": anthropicStreamWithMidStreamText(),
			"stop_reason":  anthropicStreamWithoutMessageStop(),
		} {
			t.Run(name+"/"+streamName, func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				resp, _ := stallingAnthropicUpstream(t, stream)

				got := runCompatStreamWithDeadline(t, handle, newCompatStreamIdleTestService(1), resp, c)

				require.NoError(t, got.err)
				require.NotNil(t, got.result)
				require.Equal(t, 42, got.result.Usage.OutputTokens)
				require.Contains(t, rec.Body.String(), compatStreamTerminalMarkers[name])
				require.NotContains(t, rec.Body.String(), compatStreamFailureMarkers[name])
			})
		}
	}
}

// errAfterReader 读完 data 后返回 err，模拟上游连接中途断开。
type errAfterReader struct {
	data *strings.Reader
	err  error
}

func (r *errAfterReader) Read(p []byte) (int, error) {
	if r.data.Len() > 0 {
		return r.data.Read(p)
	}
	return 0, r.err
}

func TestAnthropicCompatStreaming_ReadErrorBeforeTerminalEventFails(t *testing.T) {
	messageStartOnly := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_disc","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","usage":{"input_tokens":20,"output_tokens":1}}}`,
		``,
	}, "\n") + "\n"
	for name, handle := range compatStreamHandlers() {
		for _, tc := range []struct {
			name       string
			stream     string
			wantErr    bool
			wantResult bool
		}{
			{name: "after_text", stream: anthropicStreamTruncatedBeforeMessageDelta(), wantErr: true, wantResult: true},
			{name: "after_message_start", stream: messageStartOnly, wantErr: true, wantResult: true},
			{name: "before_any_event", stream: "", wantErr: true},
			{name: "after_stop_reason", stream: anthropicStreamWithoutMessageStop(), wantResult: true},
			{name: "after_message_stop_without_stop_reason", stream: strings.Replace(anthropicStreamWithMidStreamText(), `"delta":{"stop_reason":"end_turn"}`, `"delta":{}`, 1), wantResult: true},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				body := io.NopCloser(&errAfterReader{data: strings.NewReader(tc.stream), err: io.ErrUnexpectedEOF})

				result, err := handle(&GatewayService{}, &http.Response{Header: http.Header{}, Body: body}, c)

				if tc.wantErr {
					require.ErrorIs(t, err, io.ErrUnexpectedEOF)
					require.NotErrorIs(t, err, errAnthropicNativeStreamIdle)
					require.NotContains(t, rec.Body.String(), compatStreamTerminalMarkers[name])
					require.Contains(t, rec.Body.String(), compatStreamFailureMarkers[name])
					require.Contains(t, rec.Body.String(), "stream_read_error")
				} else {
					require.NoError(t, err)
					require.Contains(t, rec.Body.String(), compatStreamTerminalMarkers[name])
				}
				if !tc.wantResult {
					require.Nil(t, result)
					return
				}
				require.NotNil(t, result)
				require.False(t, result.ClientDisconnect)
				require.Equal(t, 20, result.Usage.InputTokens)
			})
		}
	}
}

// compatResponsesSSEEvents 解析 Responses SSE 输出中各 data 帧的 type 与 sequence_number。
func compatResponsesSSEEvents(t *testing.T, body string) []gjson.Result {
	t.Helper()
	var events []gjson.Result
	for _, line := range strings.Split(body, "\n") {
		if payload, ok := strings.CutPrefix(line, "data: "); ok {
			require.True(t, gjson.Valid(payload), payload)
			events = append(events, gjson.Parse(payload))
		}
	}
	return events
}

func TestAnthropicCompatStreaming_EOFWithoutTerminalEventFails(t *testing.T) {
	truncatedToolUse := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_disc","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","usage":{"input_tokens":20,"output_tokens":1}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"write_file","input":{}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"/tmp/a\",\"content\":\"hal"}}`,
		``,
	}, "\n") + "\n"
	for name, handle := range compatStreamHandlers() {
		for _, tc := range []struct {
			name        string
			stream      string
			wantErrText string
			wantType    string
		}{
			{name: "truncated", stream: anthropicStreamTruncatedBeforeMessageDelta(), wantErrText: "missing terminal event", wantType: "stream_incomplete"},
			{name: "truncated_tool_use", stream: truncatedToolUse, wantErrText: "missing terminal event", wantType: "stream_incomplete"},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)

				result, err := handle(&GatewayService{}, &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.stream))}, c)

				require.ErrorContains(t, err, tc.wantErrText)
				require.NotNil(t, result)
				require.Equal(t, 20, result.Usage.InputTokens)
				require.True(t, IsResponseCommitted(c))
				body := rec.Body.String()
				require.NotContains(t, body, compatStreamTerminalMarkers[name])
				require.NotContains(t, body, `"finish_reason":"`)
				require.Equal(t, 1, strings.Count(body, compatStreamFailureMarkers[name]))
				if name != "responses" {
					require.Contains(t, body, tc.wantType)
					return
				}
				events := compatResponsesSSEEvents(t, body)
				require.GreaterOrEqual(t, len(events), 2)
				failed := events[len(events)-1]
				require.Equal(t, "response.failed", failed.Get("type").String())
				require.Equal(t, "msg_disc", failed.Get("response.id").String())
				require.Equal(t, "failed", failed.Get("response.status").String())
				require.Equal(t, tc.wantType, failed.Get("response.error.code").String())
				require.Equal(t, events[len(events)-2].Get("sequence_number").Int()+1, failed.Get("sequence_number").Int())
			})
		}
	}
}

func TestAnthropicCompatStreaming_EOFBeforeMessageStartKeepsExistingEnding(t *testing.T) {
	for name, handle := range compatStreamHandlers() {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)

			result, err := handle(&GatewayService{}, &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader("event: ping\ndata: {\"type\":\"ping\"}\n\n"))}, c)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotContains(t, rec.Body.String(), compatStreamFailureMarkers[name])
		})
	}
}

func TestAnthropicCompatStreaming_DoneLineCountsAsTerminalEvent(t *testing.T) {
	for name, handle := range compatStreamHandlers() {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			stream := anthropicStreamTruncatedBeforeMessageDelta() + "data: [DONE]\n\n"

			result, err := handle(&GatewayService{}, &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))}, c)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotContains(t, rec.Body.String(), compatStreamFailureMarkers[name])
		})
	}
}

func anthropicStreamWithInBandError(messageBeforeError, after string) string {
	return strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_disc","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","usage":{"input_tokens":20,"output_tokens":1}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"second"}}`,
		``,
		`event: error`,
		`data: {"type":"error","error":{"type":"overloaded_error","message":` + strconv.Quote(messageBeforeError) + `}}`,
		``,
	}, "\n") + "\n" + after
}

func TestAnthropicCompatStreaming_UpstreamErrorEventEndsStreamWithoutWaiting(t *testing.T) {
	for name, handle := range compatStreamHandlers() {
		for _, disconnect := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/client_disconnected=%t", name, disconnect), func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				marker := "\x00never-written"
				if disconnect {
					marker = "second"
				}
				writer := &disconnectOnMarkerResponseWriter{ResponseRecorder: httptest.NewRecorder(), marker: marker}
				c, _ := gin.CreateTestContext(writer)
				resp, pw := stallingAnthropicUpstream(t, anthropicStreamWithInBandError("Overloaded", ""))

				got := runCompatStreamWithDeadline(t, handle, newCompatStreamIdleTestService(0), resp, c)

				if disconnect {
					require.NoError(t, got.err)
					require.NotNil(t, got.result)
					require.True(t, got.result.ClientDisconnect)
					require.Equal(t, 20, got.result.Usage.InputTokens)
					require.Zero(t, writer.writesAfterError)
				} else {
					var sseErr *sseStreamErrorEventError
					require.ErrorAs(t, got.err, &sseErr)
					require.Nil(t, got.result, "an upstream error event is not billed while the client is online")
					require.Equal(t, 1, strings.Count(writer.Body.String(), compatStreamFailureMarkers[name]))
					require.Contains(t, writer.Body.String(), "Overloaded")
				}
				_, werr := io.WriteString(pw, "event: ping\n")
				require.Error(t, werr, "upstream body must be closed after an upstream error event")
			})
		}
	}
}

func TestAnthropicCompatStreaming_UpstreamErrorEventStopsReadingBufferedEvents(t *testing.T) {
	after := strings.Join([]string{
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"LEAKED"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":9}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n") + "\n"
	for name, handle := range compatStreamHandlers() {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)

			result, err := handle(&GatewayService{}, &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(anthropicStreamWithInBandError("Overloaded", after)))}, c)

			var sseErr *sseStreamErrorEventError
			require.ErrorAs(t, err, &sseErr)
			require.Nil(t, result)
			body := rec.Body.String()
			require.NotContains(t, body, "LEAKED")
			require.NotContains(t, body, compatStreamTerminalMarkers[name])
			require.Equal(t, 1, strings.Count(body, compatStreamFailureMarkers[name]))
		})
	}
}

func TestAnthropicCompatStreaming_UpstreamErrorEventMessageIsSanitized(t *testing.T) {
	for name, handle := range compatStreamHandlers() {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			stream := anthropicStreamWithInBandError("upstream https://example.test/v1?key=SECRET failed", "")

			_, err := handle(&GatewayService{}, &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))}, c)

			require.Error(t, err)
			require.Contains(t, rec.Body.String(), "key=***")
			require.NotContains(t, rec.Body.String(), "SECRET")
		})
	}
}

func TestAnthropicCompatStreaming_UpstreamErrorEventBeforeOutputWritesNothing(t *testing.T) {
	stream := "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Overloaded\"}}\n\n"
	for name, handle := range compatStreamHandlers() {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)

			result, err := handle(&GatewayService{}, &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))}, c)

			var sseErr *sseStreamErrorEventError
			require.ErrorAs(t, err, &sseErr)
			require.Nil(t, result)
			require.Zero(t, rec.Body.Len(), "the handler fails over to another account")
			require.False(t, IsResponseCommitted(c))
		})
	}
}

func TestForwardAsCompatStreaming_UpstreamErrorEventFailsOver(t *testing.T) {
	beforeOutput := "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Overloaded\"}}\n\n"
	for _, endpoint := range []string{"chat_completions", "responses"} {
		for _, tc := range []struct {
			name       string
			stream     string
			wantStatus int
		}{
			{name: "before_output", stream: beforeOutput, wantStatus: 529},
			{name: "after_output", stream: anthropicStreamWithInBandError("Overloaded", ""), wantStatus: http.StatusForbidden},
		} {
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}],"stream":true}`
				path := "/v1/chat/completions"
				if endpoint == "responses" {
					body = `{"model":"claude-sonnet-4-6","input":"hi","stream":true}`
					path = "/v1/responses"
				}
				c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.stream))}
				svc := &GatewayService{
					cfg:              rawChatCompletionsTestConfig(),
					httpUpstream:     &anthropicHTTPUpstreamRecorder{resp: resp},
					rateLimitService: &RateLimitService{accountRepo: &errorPolicyRepoStub{}, cfg: &config.Config{}},
				}
				account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-test"}}

				var result *ForwardResult
				var err error
				if endpoint == "responses" {
					result, err = svc.ForwardAsResponses(context.Background(), c, account, []byte(body), nil)
				} else {
					result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(body), nil)
				}

				var failoverErr *UpstreamFailoverError
				require.ErrorAs(t, err, &failoverErr)
				require.Equal(t, tc.wantStatus, failoverErr.StatusCode)
				require.Nil(t, result)
			})
		}
	}
}

func TestAnthropicCompatStreaming_LineTooLongFails(t *testing.T) {
	for name, handle := range compatStreamHandlers() {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: 128 * 1024}}}
			stream := anthropicStreamTruncatedBeforeMessageDelta() + "event: content_block_delta\ndata: " + strings.Repeat("x", 256*1024) + "\n\n"

			result, err := handle(svc, &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))}, c)

			require.ErrorIs(t, err, bufio.ErrTooLong)
			require.NotNil(t, result)
			require.Contains(t, rec.Body.String(), compatStreamFailureMarkers[name])
			require.Contains(t, rec.Body.String(), "response_too_large")
		})
	}
}

func TestHandleResponsesStreamingResponse_FailureSequenceFollowsRestoredEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_tool","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","usage":{"input_tokens":20,"output_tokens":1}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"apply_patch","input":{}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"input\":"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"*** Begin"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":" Patch"}}`,
		``,
	}, "\n") + "\n"
	mapping := apicompat.ResponsesClientToolMapping{CustomTools: map[string]bool{"apply_patch": true}}

	_, err := (&GatewayService{}).handleResponsesStreamingResponse(
		&http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))},
		c, "claude-sonnet-4-6", "claude-sonnet-4-6", nil, time.Now(), mapping,
	)

	require.ErrorContains(t, err, "missing terminal event")
	events := compatResponsesSSEEvents(t, rec.Body.String())
	require.NotEmpty(t, events)
	require.Equal(t, "response.failed", events[len(events)-1].Get("type").String())
	for i, event := range events {
		require.Equal(t, int64(i), event.Get("sequence_number").Int(), "sequence_number must be contiguous: %s", event.Raw)
	}
}

func TestAnthropicCompatStream_ClientGoneAtStreamEndMarksDisconnected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 泵 goroutine 结束与请求 ctx 取消同时就绪时 select 随机选择，多轮重复以覆盖两种选择。
	for round := 0; round < 64; round++ {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
		stream := newAnthropicCompatStream(nil, c, &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, "test", "req")
		time.Sleep(time.Millisecond)

		_, ok := stream.nextLine()

		require.False(t, ok)
		require.True(t, stream.clientDisconnected, "round %d", round)
		require.Nil(t, stream.failure())
		stream.stop()
	}
}

func TestAnthropicCompatStreaming_RequestContextCanceledDrainsWithoutWriting(t *testing.T) {
	for name, handle := range compatStreamHandlers() {
		for _, delayed := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/upstream_delayed=%t", name, delayed), func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Request = httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
				var body io.ReadCloser = io.NopCloser(strings.NewReader(anthropicStreamWithMidStreamText()))
				if delayed {
					pr, pw := io.Pipe()
					t.Cleanup(func() { _ = pr.Close() })
					go func() {
						time.Sleep(20 * time.Millisecond)
						_, _ = io.WriteString(pw, anthropicStreamWithMidStreamText())
						_ = pw.Close()
					}()
					body = pr
				}

				got := runCompatStreamWithDeadline(t, handle, newCompatStreamIdleTestService(0), &http.Response{Header: http.Header{}, Body: body}, c)

				require.NoError(t, got.err)
				require.NotNil(t, got.result)
				require.True(t, got.result.ClientDisconnect)
				require.Equal(t, 20, got.result.Usage.InputTokens)
				require.Equal(t, 42, got.result.Usage.OutputTokens)
				require.NotContains(t, rec.Body.String(), compatStreamTerminalMarkers[name])
			})
		}
	}
}

func TestAnthropicCompatStream_ClientDisconnectBoundsDrainWhenIntervalDisabled(t *testing.T) {
	for _, tc := range []struct {
		name         string
		intervalSec  int
		wantInterval time.Duration
	}{
		{name: "interval_disabled", intervalSec: 0, wantInterval: anthropicCompatDisconnectDrainInterval},
		{name: "interval_configured", intervalSec: 30, wantInterval: 30 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			pr, pw := io.Pipe()
			t.Cleanup(func() { _ = pw.Close(); _ = pr.Close() })
			cfg := &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: tc.intervalSec}}
			stream := newAnthropicCompatStream(cfg, c, &http.Response{Header: http.Header{}, Body: pr}, "test", "req")
			defer stream.stop()
			require.Equal(t, tc.intervalSec > 0, stream.pump.timer != nil)

			stream.markClientDisconnected()

			require.True(t, stream.clientDisconnected)
			require.NotNil(t, stream.pump.timer)
			require.Equal(t, tc.wantInterval, stream.pump.interval)
		})
	}
}

func TestAnthropicCompatStreaming_DisconnectDrainEndsWhenIntervalDisabled(t *testing.T) {
	original := anthropicCompatDisconnectDrainInterval
	anthropicCompatDisconnectDrainInterval = 200 * time.Millisecond
	t.Cleanup(func() { anthropicCompatDisconnectDrainInterval = original })

	for name, handle := range compatStreamHandlers() {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			writer := &disconnectOnMarkerResponseWriter{ResponseRecorder: httptest.NewRecorder(), marker: "second"}
			c, _ := gin.CreateTestContext(writer)
			resp, pw := stallingAnthropicUpstream(t, anthropicStreamTruncatedBeforeMessageDelta())

			got := runCompatStreamWithDeadline(t, handle, newCompatStreamIdleTestService(0), resp, c)

			require.NoError(t, got.err)
			require.NotNil(t, got.result)
			require.True(t, got.result.ClientDisconnect)
			require.Equal(t, 20, got.result.Usage.InputTokens)
			require.Zero(t, writer.writesAfterError)
			_, werr := io.WriteString(pw, "event: ping\n")
			require.Error(t, werr, "upstream body must be closed after the disconnect drain bound")
		})
	}
}

func TestForwardAsCompatStreaming_IdleTimeoutTriggersStreamTimeoutHandling(t *testing.T) {
	for _, endpoint := range []string{"chat_completions", "responses"} {
		for _, tc := range []struct {
			name        string
			stream      string
			marker      string
			wantErr     bool
			wantOutput  int
			wantPenalty int
		}{
			{name: "client_online", stream: anthropicStreamTruncatedBeforeMessageDelta(), marker: "\x00never-written", wantErr: true, wantOutput: 1, wantPenalty: 1},
			{name: "client_disconnected", stream: anthropicStreamTruncatedBeforeMessageDelta(), marker: "second", wantOutput: 1},
			{name: "after_message_stop", stream: anthropicStreamWithMidStreamText(), marker: "\x00never-written", wantOutput: 42},
			{name: "after_stop_reason", stream: anthropicStreamWithoutMessageStop(), marker: "\x00never-written", wantOutput: 42},
		} {
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				writer := &disconnectOnMarkerResponseWriter{ResponseRecorder: httptest.NewRecorder(), marker: tc.marker}
				c, _ := gin.CreateTestContext(writer)
				body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}],"stream":true}`
				path := "/v1/chat/completions"
				if endpoint == "responses" {
					body = `{"model":"claude-sonnet-4-6","input":"hi","stream":true}`
					path = "/v1/responses"
				}
				c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				resp, _ := stallingAnthropicUpstream(t, tc.stream)
				resp.StatusCode = http.StatusOK
				repo := &errorPolicyRepoStub{}
				settings := NewSettingService(&settingRepoStub{values: map[string]string{
					SettingKeyStreamTimeoutSettings: `{"enabled":true,"action":"temp_unsched","temp_unsched_minutes":1,"threshold_count":1,"threshold_window_minutes":1}`,
				}}, nil)
				cfg := rawChatCompletionsTestConfig()
				cfg.Gateway.StreamDataIntervalTimeout = 1
				svc := &GatewayService{
					cfg:              cfg,
					httpUpstream:     &anthropicHTTPUpstreamRecorder{resp: resp},
					rateLimitService: &RateLimitService{accountRepo: repo, settingService: settings},
				}
				account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-test"}}

				var result *ForwardResult
				var err error
				if endpoint == "responses" {
					result, err = svc.ForwardAsResponses(context.Background(), c, account, []byte(body), nil)
				} else {
					result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(body), nil)
				}

				if tc.wantErr {
					require.ErrorIs(t, err, errAnthropicNativeStreamIdle)
				} else {
					require.NoError(t, err)
				}
				require.NotNil(t, result)
				require.Equal(t, tc.wantOutput, result.Usage.OutputTokens)
				require.Equal(t, tc.wantPenalty, repo.tempCalls)
			})
		}
	}
}

func TestHandleCCStreamingFromAnthropic_ClientDisconnectDrainsUpstreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := &disconnectOnMarkerResponseWriter{ResponseRecorder: httptest.NewRecorder(), marker: "second"}
	c, _ := gin.CreateTestContext(writer)

	result, err := (&GatewayService{}).handleCCStreamingFromAnthropic(
		&http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(anthropicStreamWithMidStreamText()))},
		c, "claude-sonnet-4-6", "claude-sonnet-4-6", nil, time.Now(),
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, writer.failed)
	require.True(t, result.ClientDisconnect)
	require.Zero(t, writer.writesAfterError)
	require.Zero(t, writer.flushesAfterError)

	body := writer.Body.String()
	require.Contains(t, body, `"content":"first"`)
	require.NotContains(t, body, "second")
	require.NotContains(t, body, "[DONE]")

	require.Equal(t, 20, result.Usage.InputTokens)
	require.Equal(t, 42, result.Usage.OutputTokens)
	require.Equal(t, 11, result.Usage.CacheReadInputTokens)
	require.Equal(t, 4, result.Usage.CacheCreationInputTokens)
	require.NotNil(t, result.FirstTokenMs)
}

func TestHandleResponsesStreamingResponse_ClientDisconnectDrainsUpstreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := &disconnectOnMarkerResponseWriter{ResponseRecorder: httptest.NewRecorder(), marker: "second"}
	c, _ := gin.CreateTestContext(writer)

	result, err := (&GatewayService{}).handleResponsesStreamingResponse(
		&http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(anthropicStreamWithMidStreamText()))},
		c, "claude-sonnet-4-6", "claude-sonnet-4-6", nil, time.Now(), apicompat.ResponsesClientToolMapping{},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, writer.failed)
	require.True(t, result.ClientDisconnect)
	require.Zero(t, writer.writesAfterError)
	require.Zero(t, writer.flushesAfterError)

	body := writer.Body.String()
	require.Contains(t, body, `"delta":"first"`)
	require.NotContains(t, body, "second")
	require.NotContains(t, body, "response.completed")

	require.Equal(t, 20, result.Usage.InputTokens)
	require.Equal(t, 42, result.Usage.OutputTokens)
	require.Equal(t, 11, result.Usage.CacheReadInputTokens)
	require.Equal(t, 4, result.Usage.CacheCreationInputTokens)
	require.NotNil(t, result.FirstTokenMs)
}

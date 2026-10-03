//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type gatewayForwardDrainHandler func(*GatewayService, *http.Response, *gin.Context, time.Time) (*ForwardResult, error)

func gatewayForwardDrainHandlers() map[string]gatewayForwardDrainHandler {
	return map[string]gatewayForwardDrainHandler{
		"responses": func(s *GatewayService, resp *http.Response, c *gin.Context, start time.Time) (*ForwardResult, error) {
			return s.handleResponsesStreamingResponse(resp, c, "public-model", "claude-sonnet-4.5", nil, start, apicompat.ResponsesClientToolMapping{})
		},
		"chat_completions": func(s *GatewayService, resp *http.Response, c *gin.Context, start time.Time) (*ForwardResult, error) {
			return s.handleCCStreamingFromAnthropic(resp, c, "public-model", "claude-sonnet-4.5", nil, start)
		},
	}
}

// 在 HTTP 下游写出边界注入失败，并模拟客户端取消请求。
// 失败后的任何 write/flush 都算回归，包括 finalize 阶段。
type gatewayForwardDrainFailWriter struct {
	*httptest.ResponseRecorder
	failAt             int
	writes             int
	writesAfterFailure int
	flushAfterFailure  int
	failed             bool
	failedCh           chan struct{}
	cancel             context.CancelFunc
}

func (w *gatewayForwardDrainFailWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.failed {
		w.writesAfterFailure++
		return 0, io.ErrClosedPipe
	}
	if w.writes == w.failAt {
		w.failed = true
		close(w.failedCh)
		if w.cancel != nil {
			w.cancel()
		}
		return 0, io.ErrClosedPipe
	}
	return w.ResponseRecorder.Write(p)
}

func (w *gatewayForwardDrainFailWriter) Flush() {
	if w.failed {
		w.flushAfterFailure++
	}
	w.ResponseRecorder.Flush()
}

// 只有下游实际写失败后才允许读取上游尾部，防止预读 fixture 掩盖早退漏计费。
type gatewayForwardDrainGatedBody struct {
	prefix *strings.Reader
	tail   *strings.Reader
	gate   <-chan struct{}
	closed chan struct{}
	once   sync.Once
}

func (b *gatewayForwardDrainGatedBody) Read(p []byte) (int, error) {
	if b.prefix != nil {
		n, err := b.prefix.Read(p)
		if err != io.EOF {
			return n, err
		}
		b.prefix = nil
	}
	select {
	case <-b.gate:
	case <-b.closed:
		return 0, io.ErrClosedPipe
	}
	return b.tail.Read(p)
}

func (b *gatewayForwardDrainGatedBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

type gatewayForwardDrainCallResult struct {
	result *ForwardResult
	err    error
}

func runGatewayForwardDrainHandler(t *testing.T, handle gatewayForwardDrainHandler, svc *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
	t.Helper()
	done := make(chan gatewayForwardDrainCallResult, 1)
	start := time.Now()
	go func() {
		result, err := handle(svc, resp, c, start)
		done <- gatewayForwardDrainCallResult{result: result, err: err}
	}()
	select {
	case got := <-done:
		return got.result, got.err
	case <-time.After(4 * time.Second):
		_ = resp.Body.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("stream handler still running after closing its upstream body")
		}
		t.Fatal("stream handler did not finish within its fixture deadline")
		return nil, nil
	}
}

// 直接观测上游 Read 退出，而非用协程总数近似判断；Close 必须解除 Read 阻塞，
// 与 net/http 对响应 body 的要求一致。
type gatewayForwardDrainBlockingBody struct {
	prefix      *strings.Reader
	closed      chan struct{}
	readEntered chan struct{}
	readExited  chan struct{}
	readErr     error
	closeOnce   sync.Once
	readOnce    sync.Once
}

func newGatewayForwardDrainBlockingBody(prefix string) *gatewayForwardDrainBlockingBody {
	return &gatewayForwardDrainBlockingBody{
		prefix:      strings.NewReader(prefix),
		closed:      make(chan struct{}),
		readEntered: make(chan struct{}),
		readExited:  make(chan struct{}),
	}
}

func (b *gatewayForwardDrainBlockingBody) Read(p []byte) (int, error) {
	if b.prefix != nil {
		n, err := b.prefix.Read(p)
		if err != io.EOF {
			return n, err
		}
		b.prefix = nil
	}
	b.readOnce.Do(func() { close(b.readEntered) })
	defer close(b.readExited)
	if b.readErr != nil {
		return 0, b.readErr
	}
	<-b.closed
	return 0, io.ErrClosedPipe
}

func (b *gatewayForwardDrainBlockingBody) Close() error {
	b.closeOnce.Do(func() { close(b.closed) })
	return nil
}

func requireGatewayForwardDrainBodyStopped(t *testing.T, body *gatewayForwardDrainBlockingBody) {
	t.Helper()
	select {
	case <-body.closed:
	default:
		t.Error("handler must close its upstream body before returning")
	}
	select {
	case <-body.readEntered:
		select {
		case <-body.readExited:
		default:
			t.Error("upstream read must exit before the handler returns")
		}
	default:
		// error 事件可能在行泵开始下一次 Read 之前就结束处理。
	}
}

const gatewayForwardDrainStart = "event: message_start\n" +
	`data: {"type":"message_start","message":{"id":"msg_drain","type":"message","role":"assistant","model":"claude-sonnet-4.5","content":[],"usage":{"input_tokens":12,"output_tokens":1,"cache_read_input_tokens":9,"cache_creation_input_tokens":3}}}` + "\n\n"

const gatewayForwardDrainDelta = "event: message_delta\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":27,"cache_read_input_tokens":15,"cache_creation_input_tokens":6}}` + "\n\n"

const gatewayForwardDrainStop = "event: message_stop\n" + `data: {"type":"message_stop"}` + "\n\n"

func TestGatewayForwardStreamDrain_ClientWriteFailurePreservesTailUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range gatewayForwardDrainHandlers() {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			writer := &gatewayForwardDrainFailWriter{
				ResponseRecorder: httptest.NewRecorder(),
				failAt:           1,
				failedCh:         make(chan struct{}),
				cancel:           cancel,
			}
			c, _ := gin.CreateTestContext(writer)
			c.Request = httptest.NewRequest(http.MethodPost, "/fixture", nil).WithContext(ctx)
			body := &gatewayForwardDrainGatedBody{
				prefix: strings.NewReader(gatewayForwardDrainStart),
				tail:   strings.NewReader(gatewayForwardDrainDelta + gatewayForwardDrainStop),
				gate:   writer.failedCh,
				closed: make(chan struct{}),
			}
			defer body.Close()
			resp := &http.Response{Header: http.Header{"X-Request-Id": {"rid_drain"}}, Body: body}

			result, err := runGatewayForwardDrainHandler(t, handle, &GatewayService{}, resp, c)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, writer.failed)
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			require.Equal(t, 27, result.Usage.OutputTokens, "usage must include message_delta arriving after the failed write")
			require.Equal(t, 12, result.Usage.InputTokens)
			require.Equal(t, 15, result.Usage.CacheReadInputTokens)
			require.Equal(t, 6, result.Usage.CacheCreationInputTokens)
			require.True(t, result.ClientDisconnect)
			require.Equal(t, "rid_drain", result.RequestID)
			require.Equal(t, "public-model", result.Model)
			require.Equal(t, "claude-sonnet-4.5", result.UpstreamModel)
			require.NotNil(t, result.FirstTokenMs)
			require.Zero(t, writer.writesAfterFailure)
			require.Zero(t, writer.flushAfterFailure)
		})
	}
}

func TestGatewayForwardStreamDrain_TerminalTailUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range gatewayForwardDrainHandlers() {
		for _, compact := range []bool{false, true} {
			t.Run(name+"/compact="+fmt.Sprint(compact), func(t *testing.T) {
				// terminal 自带 usage 和后续 delta 必须在 response.completed /
				// 最终 Chat usage chunk 发出前合并。
				stopWithUsage := "event: message_stop\n" + `data: {"type":"message_stop","usage":{"output_tokens":4,"cache_creation_input_tokens":6}}` + "\n\n"
				lateDelta := "event: message_delta\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":27,"cache_read_input_tokens":15}}` + "\n\n"
				payload := gatewayForwardDrainStart + stopWithUsage + lateDelta
				if compact {
					payload = strings.ReplaceAll(strings.ReplaceAll(payload, "event: ", "event:"), "data: ", "data:")
				}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				resp := &http.Response{Body: io.NopCloser(strings.NewReader(payload))}
				result, err := runGatewayForwardDrainHandler(t, handle, &GatewayService{}, resp, c)
				require.NoError(t, err)
				require.Equal(t, 27, result.Usage.OutputTokens)
				require.Equal(t, 12, result.Usage.InputTokens)
				require.Equal(t, 15, result.Usage.CacheReadInputTokens)
				require.Equal(t, 6, result.Usage.CacheCreationInputTokens)
				require.False(t, result.ClientDisconnect)
				requireGatewayForwardDrainTerminalUsage(t, name, rec.Body.String(), 27)
			})
		}
	}
}

func requireGatewayForwardDrainTerminalUsage(t *testing.T, protocol, payload string, outputTokens int) {
	t.Helper()
	completed, usageChunks, done := 0, 0, 0
	for _, line := range strings.Split(payload, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			done++
			continue
		}
		if protocol == "responses" && gjson.Get(data, "type").String() == "response.completed" {
			completed++
			require.Equal(t, int64(outputTokens), gjson.Get(data, "response.usage.output_tokens").Int())
		}
		if protocol == "chat_completions" && gjson.Get(data, "usage").IsObject() {
			usageChunks++
			require.Equal(t, int64(outputTokens), gjson.Get(data, "usage.completion_tokens").Int())
		}
	}
	if protocol == "responses" {
		require.Equal(t, 1, completed, "terminal conversion must run once after draining usage")
	} else {
		require.Equal(t, 1, usageChunks)
		require.Equal(t, 1, done)
	}
}

func TestGatewayForwardStreamDrain_BoundedTerminalWithDelayedUsageAndPings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range gatewayForwardDrainHandlers() {
		for _, disconnect := range []bool{false, true} {
			t.Run(name+"/disconnect="+fmt.Sprint(disconnect), func(t *testing.T) {
				rec := httptest.NewRecorder()
				var failedWriter *gatewayForwardDrainFailWriter
				var writer http.ResponseWriter = rec
				if disconnect {
					failedWriter = &gatewayForwardDrainFailWriter{ResponseRecorder: rec, failAt: 1, failedCh: make(chan struct{})}
					writer = failedWriter
				}
				c, _ := gin.CreateTestContext(writer)
				pr, pw := io.Pipe()
				defer pr.Close()
				defer pw.Close()
				producerDone := make(chan struct{})
				go func() {
					defer close(producerDone)
					defer pw.Close()
					if _, err := io.WriteString(pw, gatewayForwardDrainStart+gatewayForwardDrainStop); err != nil {
						return
					}
					// 让 usage 确实在 stop 后到达，而非同一次缓冲读取；fixture 的
					// 短延迟远小于 500ms 尾窗口。
					timer := time.NewTimer(50 * time.Millisecond)
					<-timer.C
					if _, err := io.WriteString(pw, gatewayForwardDrainDelta); err != nil {
						return
					}
					// 持续到达的数据不能重置 terminal 总期限。
					ticker := time.NewTicker(20 * time.Millisecond)
					defer ticker.Stop()
					for range ticker.C {
						if _, err := io.WriteString(pw, "event: ping\ndata: {\"type\":\"ping\"}\n\n"); err != nil {
							return
						}
					}
				}()
				svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 3}}}
				start := time.Now()
				result, err := runGatewayForwardDrainHandler(t, handle, svc, &http.Response{Body: pr}, c)
				require.NoError(t, err)
				require.Equal(t, 27, result.Usage.OutputTokens)
				require.Equal(t, 15, result.Usage.CacheReadInputTokens)
				require.Equal(t, 6, result.Usage.CacheCreationInputTokens)
				require.Equal(t, disconnect, result.ClientDisconnect)
				require.Less(t, time.Since(start), 2*time.Second, "pings must not postpone terminal completion to the idle timeout")
				select {
				case <-producerDone:
				case <-time.After(time.Second):
					t.Fatal("upstream producer did not exit after the handler closed the body")
				}
				if disconnect {
					require.Zero(t, failedWriter.writesAfterFailure)
					require.Zero(t, failedWriter.flushAfterFailure)
				} else {
					requireGatewayForwardDrainTerminalUsage(t, name, rec.Body.String(), 27)
				}
			})
		}
	}
}

func TestGatewayForwardStreamDrain_IdleClosesReaderAndPreservesUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range gatewayForwardDrainHandlers() {
		t.Run(name, func(t *testing.T) {
			writer := &gatewayForwardDrainFailWriter{ResponseRecorder: httptest.NewRecorder(), failAt: 1, failedCh: make(chan struct{})}
			c, _ := gin.CreateTestContext(writer)
			body := newGatewayForwardDrainBlockingBody(gatewayForwardDrainStart + gatewayForwardDrainDelta)
			defer body.Close()
			svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1}}}
			result, err := runGatewayForwardDrainHandler(t, handle, svc, &http.Response{Body: body}, c)
			require.ErrorContains(t, err, "stream data interval timeout")
			require.NotNil(t, result)
			require.Equal(t, 27, result.Usage.OutputTokens)
			require.Equal(t, 15, result.Usage.CacheReadInputTokens)
			require.True(t, result.ClientDisconnect)
			require.Zero(t, writer.writesAfterFailure)
			require.Zero(t, writer.flushAfterFailure)
			requireGatewayForwardDrainBodyStopped(t, body)
		})
	}
}

func TestGatewayForwardStreamDrain_ReadErrorClosesReaderAndPreservesUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range gatewayForwardDrainHandlers() {
		for _, terminal := range []bool{false, true} {
			t.Run(name+"/terminal="+fmt.Sprint(terminal), func(t *testing.T) {
				writer := &gatewayForwardDrainFailWriter{ResponseRecorder: httptest.NewRecorder(), failAt: 1, failedCh: make(chan struct{})}
				c, _ := gin.CreateTestContext(writer)
				prefix := gatewayForwardDrainStart + gatewayForwardDrainDelta
				if terminal {
					prefix += gatewayForwardDrainStop
				}
				body := newGatewayForwardDrainBlockingBody(prefix)
				body.readErr = io.ErrUnexpectedEOF
				defer body.Close()
				result, err := runGatewayForwardDrainHandler(t, handle, &GatewayService{}, &http.Response{Body: body}, c)
				if terminal {
					require.NoError(t, err, "a successful terminal must not become a replayable read failure")
				} else {
					require.ErrorIs(t, err, io.ErrUnexpectedEOF)
				}
				require.NotNil(t, result)
				require.Equal(t, 27, result.Usage.OutputTokens)
				require.True(t, result.ClientDisconnect)
				var failover *UpstreamFailoverError
				require.False(t, errors.As(err, &failover), "a disconnected request must never be replayed")
				require.Zero(t, writer.writesAfterFailure)
				require.Zero(t, writer.flushAfterFailure)
				requireGatewayForwardDrainBodyStopped(t, body)
			})
		}
	}
}

func TestGatewayForwardStreamDrain_TerminalWithoutEOFClosesBlockedReader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range gatewayForwardDrainHandlers() {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			body := newGatewayForwardDrainBlockingBody(gatewayForwardDrainStart + gatewayForwardDrainDelta + gatewayForwardDrainStop)
			defer body.Close()
			// fixture 也覆盖未传配置、没有下游 request 的安全路径。
			start := time.Now()
			result, err := runGatewayForwardDrainHandler(t, handle, &GatewayService{}, &http.Response{Body: body}, c)
			require.NoError(t, err)
			require.Equal(t, 27, result.Usage.OutputTokens)
			require.Less(t, time.Since(start), 2*time.Second, "normal terminal must not wait for the active idle timeout or EOF")
			requireGatewayForwardDrainBodyStopped(t, body)
			requireGatewayForwardDrainTerminalUsage(t, name, rec.Body.String(), 27)
		})
	}
}

func TestGatewayForwardStreamDrain_FinalizeWriteFailureSuppressesRemainingOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range gatewayForwardDrainHandlers() {
		t.Run(name, func(t *testing.T) {
			// 保留原有 EOF finalize 语义，但必须复用相同的写失败保护。
			// 两种协议都先写一个初始 chunk，再在 EOF 写 completion。
			writer := &gatewayForwardDrainFailWriter{ResponseRecorder: httptest.NewRecorder(), failAt: 2, failedCh: make(chan struct{})}
			c, _ := gin.CreateTestContext(writer)
			resp := &http.Response{Body: io.NopCloser(strings.NewReader(gatewayForwardDrainStart + gatewayForwardDrainDelta))}
			result, err := runGatewayForwardDrainHandler(t, handle, &GatewayService{}, resp, c)
			require.NoError(t, err)
			require.True(t, writer.failed, "fixture must fail during final output")
			require.True(t, result.ClientDisconnect)
			require.Equal(t, 27, result.Usage.OutputTokens)
			require.Zero(t, writer.writesAfterFailure)
			require.Zero(t, writer.flushAfterFailure)
		})
	}
}

func TestGatewayForwardStreamDrain_UpstreamRequestSurvivesDownstreamCancel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, chat := range []bool{false, true} {
		t.Run("chat="+fmt.Sprint(chat), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			writer := &gatewayForwardDrainFailWriter{
				ResponseRecorder: httptest.NewRecorder(), failAt: 1, failedCh: make(chan struct{}), cancel: cancel,
			}
			c, _ := gin.CreateTestContext(writer)
			c.Request = httptest.NewRequest(http.MethodPost, "/fixture", nil).WithContext(ctx)
			body := &gatewayForwardDrainGatedBody{
				prefix: strings.NewReader(gatewayForwardDrainStart),
				tail:   strings.NewReader(gatewayForwardDrainDelta + gatewayForwardDrainStop),
				gate:   writer.failedCh,
				closed: make(chan struct{}),
			}
			defer body.Close()
			upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Body: body}}
			svc := &GatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture-key"}}
			var result *ForwardResult
			var err error
			if chat {
				result, err = svc.ForwardAsChatCompletions(ctx, c, account, []byte(`{"model":"claude-sonnet-4.5","stream":true,"messages":[{"role":"user","content":"fixture"}]}`), nil)
			} else {
				result, err = svc.ForwardAsResponses(ctx, c, account, []byte(`{"model":"claude-sonnet-4.5","stream":true,"input":"fixture"}`), nil)
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			require.NotNil(t, upstream.lastReq)
			require.NoError(t, upstream.lastReq.Context().Err(), "upstream streaming request must remain detached from the canceled client")
			require.Nil(t, upstream.lastReq.Context().Done())
			require.Equal(t, 27, result.Usage.OutputTokens)
			require.True(t, result.ClientDisconnect)
			require.Zero(t, writer.writesAfterFailure)
			require.Zero(t, writer.flushAfterFailure)
		})
	}
}

func TestGatewayForwardStreamDrain_ErrorEventAfterTerminalPreservesFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range gatewayForwardDrainHandlers() {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			visibleText := "event: content_block_start\n" +
				`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
				"event: content_block_delta\n" +
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"answer"}}` + "\n\n" +
				"event: content_block_stop\n" + `data: {"type":"content_block_stop","index":0}` + "\n\n"
			// 同一有效流的 stop 后、500ms 尾窗口内出现显式上游错误，不能抹成成功。
			body := newGatewayForwardDrainBlockingBody(gatewayForwardDrainStart + visibleText + gatewayForwardDrainDelta + gatewayForwardDrainStop +
				"event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"fixture\"}}\n\n")
			defer body.Close()

			result, err := runGatewayForwardDrainHandler(t, handle, &GatewayService{}, &http.Response{Body: body}, c)
			require.ErrorContains(t, err, "upstream stream error event")
			require.NotNil(t, result, "已提交正文后仍需返回部分 usage")
			require.Equal(t, 27, result.Usage.OutputTokens)
			require.Equal(t, 12, result.Usage.InputTokens)
			require.Equal(t, 15, result.Usage.CacheReadInputTokens)
			require.False(t, result.ClientDisconnect)
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover), "已经提交的请求不能因尾窗口错误而重放")
			require.Contains(t, rec.Body.String(), "answer")
			require.NotContains(t, rec.Body.String(), "response.completed", "错误终态不能补发成功 completion")
			require.NotContains(t, rec.Body.String(), "data: [DONE]", "错误终态不能补发 DONE")
			requireGatewayForwardDrainBodyStopped(t, body)
		})
	}
}

func TestGatewayForwardStreamDrain_ErrorEventStopsDrain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range gatewayForwardDrainHandlers() {
		t.Run(name, func(t *testing.T) {
			writer := &gatewayForwardDrainFailWriter{ResponseRecorder: httptest.NewRecorder(), failAt: 1, failedCh: make(chan struct{})}
			c, _ := gin.CreateTestContext(writer)
			body := newGatewayForwardDrainBlockingBody(gatewayForwardDrainStart + gatewayForwardDrainDelta +
				"event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"fixture\"}}\n\n")
			defer body.Close()
			result, err := runGatewayForwardDrainHandler(t, handle, &GatewayService{}, &http.Response{Body: body}, c)
			require.ErrorContains(t, err, "upstream stream error event")
			require.NotNil(t, result)
			require.Equal(t, 27, result.Usage.OutputTokens)
			require.True(t, result.ClientDisconnect)
			require.Zero(t, writer.writesAfterFailure)
			require.Zero(t, writer.flushAfterFailure)
			requireGatewayForwardDrainBodyStopped(t, body)
		})
	}
}

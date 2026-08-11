package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type passthroughFlushTestWriter struct {
	gin.ResponseWriter
	recorder         *httptest.ResponseRecorder
	failAfterWrites  int
	successfulWrites int
	failedWrites     int
	flushBodyLengths []int
	flushEvents      chan struct{}
}

func (w *passthroughFlushTestWriter) Write(data []byte) (int, error) {
	if w.failAfterWrites >= 0 && w.successfulWrites >= w.failAfterWrites {
		w.failedWrites++
		return 0, errors.New("client disconnected")
	}
	n, err := w.ResponseWriter.Write(data)
	if err == nil {
		w.successfulWrites++
	}
	return n, err
}

func (w *passthroughFlushTestWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func (w *passthroughFlushTestWriter) Flush() {
	w.ResponseWriter.Flush()
	w.flushBodyLengths = append(w.flushBodyLengths, w.recorder.Body.Len())
	if w.flushEvents != nil {
		w.flushEvents <- struct{}{}
	}
}

type passthroughFlushTestErrorBody struct {
	payload []byte
	err     error
	sent    bool
}

func (r *passthroughFlushTestErrorBody) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(p, r.payload), nil
	}
	return 0, r.err
}

func (r *passthroughFlushTestErrorBody) Close() error { return nil }

func runPassthroughFlushTest(
	t *testing.T,
	body io.ReadCloser,
	failAfterWrites int,
	setups ...func(*gin.Context),
) (*openaiStreamingResultPassthrough, *httptest.ResponseRecorder, *passthroughFlushTestWriter, error) {
	return runPassthroughFlushTestWithTimeout(t, body, failAfterWrites, 2, setups...)
}

func runPassthroughFlushTestWithTimeout(
	t *testing.T,
	body io.ReadCloser,
	failAfterWrites int,
	firstEventTimeoutSeconds int,
	setups ...func(*gin.Context),
) (*openaiStreamingResultPassthrough, *httptest.ResponseRecorder, *passthroughFlushTestWriter, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	writer := &passthroughFlushTestWriter{
		ResponseWriter:  c.Writer,
		recorder:        recorder,
		failAfterWrites: failAfterWrites,
		flushEvents:     make(chan struct{}, 16),
	}
	c.Writer = writer
	for _, setup := range setups {
		setup(c)
	}

	svc := &OpenAIGatewayService{cfg: &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize:                             defaultMaxLineSize,
			OpenAIResponsesFirstEventTimeoutSeconds: firstEventTimeoutSeconds,
		},
	}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       body,
	}
	result, err := svc.handleStreamingResponsePassthrough(
		context.Background(),
		resp,
		c,
		&Account{ID: 1, Platform: PlatformOpenAI, Name: "flush-test"},
		time.Now(),
		"",
		"",
	)
	return result, recorder, writer, err
}

func TestOpenAIStreamingPassthroughFlushesAtCompleteEventBoundaries(t *testing.T) {
	firstEvent := "event: response.output_text.delta\n" +
		"id: event-1\n" +
		`data: {"type":"response.output_text.delta","delta":"hello"}` + "\n\n"
	heartbeat := ": keepalive\n\n"
	terminalEvent := "event: response.completed\n" +
		`data: {"type":"response.completed","response":{"id":"resp_flush","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}` + "\n\n"
	upstream := firstEvent + heartbeat + terminalEvent

	result, recorder, writer, err := runPassthroughFlushTest(t, io.NopCloser(strings.NewReader(upstream)), -1)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, upstream, recorder.Body.String())
	require.Equal(t, []int{
		len(firstEvent),
		len(firstEvent) + len(heartbeat),
		len(upstream),
	}, writer.flushBodyLengths)
	require.Equal(t, 3, result.usage.InputTokens)
	require.Equal(t, 2, result.usage.OutputTokens)
}

func TestOpenAIStreamingPassthroughKeepsPreamblePendingUntilFirstOutputBoundary(t *testing.T) {
	preamble := "event: response.created\n" +
		`data: {"type":"response.created","response":{"id":"resp_pending"}}` + "\n\n" +
		": waiting\n\n"
	firstOutput := `data: {"type":"response.output_text.delta","delta":"ready"}` + "\n\n"
	terminalEvent := `data: {"type":"response.completed","response":{"id":"resp_pending","usage":{"input_tokens":4,"output_tokens":1,"total_tokens":5}}}` + "\n\n"
	upstream := preamble + firstOutput + terminalEvent

	_, recorder, writer, err := runPassthroughFlushTest(t, io.NopCloser(strings.NewReader(upstream)), -1)

	require.NoError(t, err)
	require.Equal(t, upstream, recorder.Body.String())
	require.Equal(t, []int{
		len(preamble) + len(firstOutput),
		len(upstream),
	}, writer.flushBodyLengths)
}

func TestOpenAIStreamingPassthroughZeroTimeoutReleasesLifecycleImmediately(t *testing.T) {
	preamble := "event: response.queued\n" +
		`data: {"response":{"id":"resp_zero"}}` + "\n\n"
	terminal := `data: {"type":"response.completed","response":{"id":"resp_zero","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}` + "\n\n"
	allowTerminal := make(chan struct{})
	terminalWaiting := make(chan struct{})
	reader := &stagedOpenAISSEReadCloser{
		segments: [][]byte{[]byte(preamble), []byte(terminal)},
		gates:    []<-chan struct{}{nil, allowTerminal},
		waiting:  []chan struct{}{nil, terminalWaiting},
	}

	type runResult struct {
		result   *openaiStreamingResultPassthrough
		recorder *httptest.ResponseRecorder
		writer   *passthroughFlushTestWriter
		err      error
	}
	resultCh := make(chan runResult, 1)
	writerCh := make(chan *passthroughFlushTestWriter, 1)
	go func() {
		result, recorder, writer, err := runPassthroughFlushTestWithTimeout(t, reader, -1, 0, func(c *gin.Context) {
			writerCh <- c.Writer.(*passthroughFlushTestWriter)
		})
		resultCh <- runResult{result: result, recorder: recorder, writer: writer, err: err}
	}()
	writer := <-writerCh

	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-terminalWaiting:
	case <-timer.C:
		t.Fatal("timed out waiting for terminal event read")
	}
	// The zero-second channel override must make the queued lifecycle event visible
	// while the upstream reader is still blocked before the terminal event.
	select {
	case <-writer.flushEvents:
	case <-timer.C:
		t.Fatal("timed out waiting for lifecycle flush")
	}
	close(allowTerminal)
	run := <-resultCh
	require.NoError(t, run.err)
	require.NotNil(t, run.result)
	require.Equal(t, preamble+terminal, run.recorder.Body.String())
	require.Equal(t, []int{len(preamble), len(preamble) + len(terminal)}, run.writer.flushBodyLengths)
}

func TestOpenAIStreamingPassthroughConfiguredTimeoutFlushesLifecycle(t *testing.T) {
	preamble := "event: response.queued\n" +
		`data: {"response":{"id":"resp_timer"}}` + "\n\n"
	terminal := `data: {"type":"response.completed","response":{"id":"resp_timer","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}` + "\n\n"
	allowTerminal := make(chan struct{})
	terminalWaiting := make(chan struct{})
	reader := &stagedOpenAISSEReadCloser{
		segments: [][]byte{[]byte(preamble), []byte(terminal)},
		gates:    []<-chan struct{}{nil, allowTerminal},
		waiting:  []chan struct{}{nil, terminalWaiting},
	}
	type runResult struct {
		result   *openaiStreamingResultPassthrough
		recorder *httptest.ResponseRecorder
		writer   *passthroughFlushTestWriter
		err      error
	}
	resultCh := make(chan runResult, 1)
	writerCh := make(chan *passthroughFlushTestWriter, 1)
	go func() {
		result, recorder, writer, err := runPassthroughFlushTestWithTimeout(t, reader, -1, 1, func(c *gin.Context) {
			writerCh <- c.Writer.(*passthroughFlushTestWriter)
		})
		resultCh <- runResult{result: result, recorder: recorder, writer: writer, err: err}
	}()
	writer := <-writerCh

	select {
	case <-terminalWaiting:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for terminal event read")
	}
	select {
	case <-writer.flushEvents:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for configured lifecycle flush")
	}
	require.Equal(t, preamble, writer.recorder.Body.String(), "configured timer should flush the lifecycle event before terminal release")
	close(allowTerminal)
	run := <-resultCh
	require.NoError(t, run.err)
	require.NotNil(t, run.result)
	require.Equal(t, preamble+terminal, run.recorder.Body.String())
	require.Equal(t, []int{len(preamble), len(preamble) + len(terminal)}, run.writer.flushBodyLengths)
}

func TestOpenAIStreamingPassthroughFlushesTerminalEventAtEOFWithoutBlankLine(t *testing.T) {
	upstream := "event: response.completed\n" +
		`data: {"type":"response.completed","response":{"id":"resp_eof","usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`
	wantBody := upstream + "\n"

	result, recorder, writer, err := runPassthroughFlushTest(t, io.NopCloser(strings.NewReader(upstream)), -1)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, wantBody, recorder.Body.String())
	require.Equal(t, []int{len(wantBody)}, writer.flushBodyLengths)
	require.Equal(t, 5, result.usage.InputTokens)
	require.Equal(t, 2, result.usage.OutputTokens)
}

func TestOpenAIStreamingPassthroughFailedBeforeOutputCanStillFailOverWithoutFlush(t *testing.T) {
	upstream := "event: response.created\n" +
		`data: {"type":"response.created","response":{"id":"resp_failover"}}` + "\n\n" +
		"event: response.failed\n" +
		`data: {"type":"response.failed","error":{"code":"server_error","message":"upstream processing failed"}}` + "\n\n"

	_, recorder, writer, err := runPassthroughFlushTest(t, io.NopCloser(strings.NewReader(upstream)), -1)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Empty(t, recorder.Body.String())
	require.Empty(t, writer.flushBodyLengths)
}

func TestOpenAIStreamingPassthroughQueuedMetadataBeforeCapacityErrorStillFailsOver(t *testing.T) {
	upstream := "event: response.created\n" +
		`data: {"type":"response.created","response":{"id":"resp_capacity"}}` + "\n\n" +
		"event: response.queued\n" +
		`data: {"response":{"id":"resp_capacity"}}` + "\n\n" +
		"event: keepalive\n" +
		`data: {"sequence_number":2}` + "\n\n" +
		"event: error\n" +
		`data: {"error":{"code":"server_is_overloaded","message":"model capacity exhausted"}}` + "\n\n"

	_, recorder, writer, err := runPassthroughFlushTest(t, io.NopCloser(strings.NewReader(upstream)), -1)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.True(t, failoverErr.RetryableOnSameAccount)
	require.True(t, failoverErr.RequestScopedTransient)
	require.Empty(t, recorder.Body.String())
	require.Empty(t, writer.flushBodyLengths)
}

func TestOpenAIStreamingPassthroughNonRetryableFailedBeforeOutputFlushesAtBoundary(t *testing.T) {
	upstream := "event: response.failed\n" +
		`data: {"type":"response.failed","error":{"code":"content_policy","message":"request blocked by policy"},"usage":{"input_tokens":6,"output_tokens":0,"total_tokens":6}}` + "\n\n"

	result, recorder, writer, err := runPassthroughFlushTest(t, io.NopCloser(strings.NewReader(upstream)), -1)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.NotNil(t, result)
	require.Equal(t, upstream, recorder.Body.String())
	require.Equal(t, []int{len(upstream)}, writer.flushBodyLengths)
	require.Equal(t, 6, result.usage.InputTokens)
	require.Zero(t, result.usage.OutputTokens)
}

func TestOpenAIStreamingPassthroughFailedAfterOutputFlushesAtBoundaryAndKeepsUsage(t *testing.T) {
	firstOutput := `data: {"type":"response.output_text.delta","delta":"partial"}` + "\n\n"
	failedEvent := "event: response.failed\n" +
		`data: {"type":"response.failed","error":{"code":"server_error","message":"upstream processing failed"},"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9}}` + "\n\n"
	upstream := firstOutput + failedEvent

	result, recorder, writer, err := runPassthroughFlushTest(t, io.NopCloser(strings.NewReader(upstream)), -1)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.NotNil(t, result)
	require.Equal(t, upstream, recorder.Body.String())
	require.Equal(t, []int{len(firstOutput), len(upstream)}, writer.flushBodyLengths)
	require.Equal(t, 7, result.usage.InputTokens)
	require.Equal(t, 2, result.usage.OutputTokens)
}

func TestOpenAIStreamingPassthroughClientDisconnectStillDrainsTerminalUsage(t *testing.T) {
	firstOutput := `data: {"type":"response.output_text.delta","delta":"partial"}` + "\n\n"
	terminalEvent := `data: {"type":"response.completed","response":{"id":"resp_drain","usage":{"input_tokens":11,"output_tokens":4,"total_tokens":15}}}` + "\n\n"

	result, recorder, writer, err := runPassthroughFlushTest(
		t,
		io.NopCloser(strings.NewReader(firstOutput+terminalEvent)),
		2,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, firstOutput, recorder.Body.String())
	require.Equal(t, []int{len(firstOutput)}, writer.flushBodyLengths)
	require.Equal(t, 1, writer.failedWrites)
	require.Equal(t, 11, result.usage.InputTokens)
	require.Equal(t, 4, result.usage.OutputTokens)
}

func TestOpenAIStreamingPassthroughScannerErrorFlushesWrittenResidual(t *testing.T) {
	upstream := []byte(`data: {"type":"response.output_text.delta","delta":"partial"}`)
	readErr := errors.New("upstream read failed")

	_, recorder, writer, err := runPassthroughFlushTest(t, &passthroughFlushTestErrorBody{
		payload: upstream,
		err:     readErr,
	}, -1)

	require.ErrorIs(t, err, readErr)
	wantBody := string(upstream) + "\n"
	require.Equal(t, wantBody, recorder.Body.String())
	require.Equal(t, []int{len(wantBody)}, writer.flushBodyLengths)
}

func TestOpenAIStreamingPassthroughNamespaceRestoreErrorFlushesWrittenResidualOnce(t *testing.T) {
	writtenPrefix := `data: {"type":"response.output_text.delta","delta":"prefix"}` + "\n"
	overflowData := `data: {"type":"response.output_text.delta","delta":"not-written","overflow":1e1000}`

	_, recorder, writer, err := runPassthroughFlushTest(
		t,
		io.NopCloser(strings.NewReader(writtenPrefix+overflowData)),
		-1,
		func(c *gin.Context) {
			setOpenAIResponsesNamespaceNames(c, map[string]apicompat.ResponsesNamespaceName{
				"collaboration__spawn_agent": {Namespace: "collaboration", Name: "spawn_agent"},
			})
		},
	)

	require.ErrorContains(t, err, "restore OpenAI passthrough namespace response")
	require.Equal(t, writtenPrefix, recorder.Body.String())
	require.Equal(t, []int{len(writtenPrefix)}, writer.flushBodyLengths)
}

func TestOpenAIStreamingPassthroughBlankWriteFailureDoesNotFlushAndStillDrainsUsage(t *testing.T) {
	writtenDataLine := `data: {"type":"response.output_text.delta","delta":"partial"}` + "\n"
	terminalEvent := `data: {"type":"response.completed","response":{"id":"resp_blank_failure","usage":{"input_tokens":13,"output_tokens":5,"total_tokens":18}}}` + "\n\n"

	result, recorder, writer, err := runPassthroughFlushTest(
		t,
		io.NopCloser(strings.NewReader(writtenDataLine+"\n"+terminalEvent)),
		1,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, writtenDataLine, recorder.Body.String())
	require.Empty(t, writer.flushBodyLengths)
	require.Equal(t, 1, writer.successfulWrites)
	require.Equal(t, 1, writer.failedWrites)
	require.Equal(t, 13, result.usage.InputTokens)
	require.Equal(t, 5, result.usage.OutputTokens)
}

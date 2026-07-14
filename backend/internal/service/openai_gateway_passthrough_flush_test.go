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
REDACTED

func (w *passthroughFlushTestWriter) Write(data []byte) (int, error) {
	if w.failAfterWrites >= 0 && w.successfulWrites >= w.failAfterWrites {
		w.failedWrites++
		return 0, errors.New("client disconnected")
REDACTED
	n, err := w.ResponseWriter.Write(data)
	if err == nil {
		w.successfulWrites++
REDACTED
	return n, err
REDACTED

func (w *passthroughFlushTestWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
REDACTED

func (w *passthroughFlushTestWriter) Flush() {
	w.ResponseWriter.Flush()
	w.flushBodyLengths = append(w.flushBodyLengths, w.recorder.Body.Len())
REDACTED

type passthroughFlushTestErrorBody struct {
	payload []byte
	err     error
	sent    bool
REDACTED

func (r *passthroughFlushTestErrorBody) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(p, r.payload), nil
REDACTED
	return 0, r.err
REDACTED

func (r *passthroughFlushTestErrorBody) Close() error { return nil REDACTED

func runPassthroughFlushTest(
	t *testing.T,
	body io.ReadCloser,
	failAfterWrites int,
	setups ...func(*gin.Context),
) (*openaiStreamingResultPassthrough, *httptest.ResponseRecorder, *passthroughFlushTestWriter, error) {
REDACTED
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	writer := &passthroughFlushTestWriter{
		ResponseWriter:  c.Writer,
		recorder:        recorder,
		failAfterWrites: failAfterWrites,
REDACTED
	c.Writer = writer
	for _, setup := range setups {
		setup(c)
REDACTED

	svc := &OpenAIGatewayService{cfg: &config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED,
REDACTEDREDACTED
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body:       body,
REDACTED
	result, err := svc.handleStreamingResponsePassthrough(
		context.Background(),
		resp,
		c,
		&Account{ID: 1, Platform: PlatformOpenAI, Name: "flush-test"REDACTED,
		time.Now(),
		"",
		"",
	)
	return result, recorder, writer, err
REDACTED

func TestOpenAIStreamingPassthroughFlushesAtCompleteEventBoundaries(t *testing.T) {
	firstEvent := "event: response.output_text.delta\n" +
		"id: event-1\n" +
		`data: {"type":"response.output_text.delta","delta":"hello"REDACTED` + "\n\n"
	heartbeat := ": keepalive\n\n"
	terminalEvent := "event: response.completed\n" +
		`data: {"type":"response.completed","response":{"id":"resp_flush","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5REDACTEDREDACTEDREDACTED` + "\n\n"
	upstream := firstEvent + heartbeat + terminalEvent

	result, recorder, writer, err := runPassthroughFlushTest(t, io.NopCloser(strings.NewReader(upstream)), -1)

REDACTED
	require.NotNil(t, result)
	require.Equal(t, upstream, recorder.Body.String())
	require.Equal(t, []int{
		len(firstEvent),
		len(firstEvent) + len(heartbeat),
		len(upstream),
REDACTED, writer.flushBodyLengths)
	require.Equal(t, 3, result.usage.InputTokens)
	require.Equal(t, 2, result.usage.OutputTokens)
REDACTED

func TestOpenAIStreamingPassthroughKeepsPreamblePendingUntilFirstOutputBoundary(t *testing.T) {
	preamble := "event: response.created\n" +
		`data: {"type":"response.created","response":{"id":"resp_pending"REDACTEDREDACTED` + "\n\n" +
		": waiting\n\n"
	firstOutput := `data: {"type":"response.output_text.delta","delta":"ready"REDACTED` + "\n\n"
	terminalEvent := `data: {"type":"response.completed","response":{"id":"resp_pending","usage":{"input_tokens":4,"output_tokens":1,"total_tokens":5REDACTEDREDACTEDREDACTED` + "\n\n"
	upstream := preamble + firstOutput + terminalEvent

	_, recorder, writer, err := runPassthroughFlushTest(t, io.NopCloser(strings.NewReader(upstream)), -1)

REDACTED
	require.Equal(t, upstream, recorder.Body.String())
	require.Equal(t, []int{
		len(preamble) + len(firstOutput),
		len(upstream),
REDACTED, writer.flushBodyLengths)
REDACTED

func TestOpenAIStreamingPassthroughFlushesTerminalEventAtEOFWithoutBlankLine(t *testing.T) {
	upstream := "event: response.completed\n" +
		`data: {"type":"response.completed","response":{"id":"resp_eof","usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7REDACTEDREDACTEDREDACTED`
	wantBody := upstream + "\n"

	result, recorder, writer, err := runPassthroughFlushTest(t, io.NopCloser(strings.NewReader(upstream)), -1)

REDACTED
	require.NotNil(t, result)
	require.Equal(t, wantBody, recorder.Body.String())
	require.Equal(t, []int{len(wantBody)REDACTED, writer.flushBodyLengths)
	require.Equal(t, 5, result.usage.InputTokens)
	require.Equal(t, 2, result.usage.OutputTokens)
REDACTED

func TestOpenAIStreamingPassthroughFailedBeforeOutputCanStillFailOverWithoutFlush(t *testing.T) {
	upstream := "event: response.created\n" +
		`data: {"type":"response.created","response":{"id":"resp_failover"REDACTEDREDACTED` + "\n\n" +
		"event: response.failed\n" +
		`data: {"type":"response.failed","error":{"code":"server_error","message":"upstream processing failed"REDACTEDREDACTED` + "\n\n"

	_, recorder, writer, err := runPassthroughFlushTest(t, io.NopCloser(strings.NewReader(upstream)), -1)

REDACTED
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Empty(t, recorder.Body.String())
	require.Empty(t, writer.flushBodyLengths)
REDACTED

func TestOpenAIStreamingPassthroughNonRetryableFailedBeforeOutputFlushesAtBoundary(t *testing.T) {
	upstream := "event: response.failed\n" +
		`data: {"type":"response.failed","error":{"code":"content_policy","message":"request blocked by policy"REDACTED,"usage":{"input_tokens":6,"output_tokens":0,"total_tokens":6REDACTEDREDACTED` + "\n\n"

	result, recorder, writer, err := runPassthroughFlushTest(t, io.NopCloser(strings.NewReader(upstream)), -1)

REDACTED
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.NotNil(t, result)
	require.Equal(t, upstream, recorder.Body.String())
	require.Equal(t, []int{len(upstream)REDACTED, writer.flushBodyLengths)
	require.Equal(t, 6, result.usage.InputTokens)
	require.Zero(t, result.usage.OutputTokens)
REDACTED

func TestOpenAIStreamingPassthroughFailedAfterOutputFlushesAtBoundaryAndKeepsUsage(t *testing.T) {
	firstOutput := `data: {"type":"response.output_text.delta","delta":"partial"REDACTED` + "\n\n"
	failedEvent := "event: response.failed\n" +
		`data: {"type":"response.failed","error":{"code":"server_error","message":"upstream processing failed"REDACTED,"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9REDACTEDREDACTED` + "\n\n"
	upstream := firstOutput + failedEvent

	result, recorder, writer, err := runPassthroughFlushTest(t, io.NopCloser(strings.NewReader(upstream)), -1)

REDACTED
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.NotNil(t, result)
	require.Equal(t, upstream, recorder.Body.String())
	require.Equal(t, []int{len(firstOutput), len(upstream)REDACTED, writer.flushBodyLengths)
	require.Equal(t, 7, result.usage.InputTokens)
	require.Equal(t, 2, result.usage.OutputTokens)
REDACTED

func TestOpenAIStreamingPassthroughClientDisconnectStillDrainsTerminalUsage(t *testing.T) {
	firstOutput := `data: {"type":"response.output_text.delta","delta":"partial"REDACTED` + "\n\n"
	terminalEvent := `data: {"type":"response.completed","response":{"id":"resp_drain","usage":{"input_tokens":11,"output_tokens":4,"total_tokens":15REDACTEDREDACTEDREDACTED` + "\n\n"

	result, recorder, writer, err := runPassthroughFlushTest(
		t,
		io.NopCloser(strings.NewReader(firstOutput+terminalEvent)),
		2,
	)

REDACTED
	require.NotNil(t, result)
	require.Equal(t, firstOutput, recorder.Body.String())
	require.Equal(t, []int{len(firstOutput)REDACTED, writer.flushBodyLengths)
	require.Equal(t, 1, writer.failedWrites)
	require.Equal(t, 11, result.usage.InputTokens)
	require.Equal(t, 4, result.usage.OutputTokens)
REDACTED

func TestOpenAIStreamingPassthroughScannerErrorFlushesWrittenResidual(t *testing.T) {
	upstream := []byte(`data: {"type":"response.output_text.delta","delta":"partial"REDACTED`)
	readErr := errors.New("upstream read failed")

	_, recorder, writer, err := runPassthroughFlushTest(t, &passthroughFlushTestErrorBody{
		payload: upstream,
		err:     readErr,
REDACTED, -1)

	require.ErrorIs(t, err, readErr)
	wantBody := string(upstream) + "\n"
	require.Equal(t, wantBody, recorder.Body.String())
	require.Equal(t, []int{len(wantBody)REDACTED, writer.flushBodyLengths)
REDACTED

func TestOpenAIStreamingPassthroughNamespaceRestoreErrorFlushesWrittenResidualOnce(t *testing.T) {
	writtenPrefix := `data: {"type":"response.output_text.delta","delta":"prefix"REDACTED` + "\n"
	overflowData := `data: {"type":"response.output_text.delta","delta":"not-written","overflow":1e1000REDACTED`

	_, recorder, writer, err := runPassthroughFlushTest(
		t,
		io.NopCloser(strings.NewReader(writtenPrefix+overflowData)),
		-1,
		func(c *gin.Context) {
			setOpenAIResponsesNamespaceNames(c, map[string]apicompat.ResponsesNamespaceName{
				"collaboration__spawn_agent": {Namespace: "collaboration", Name: "spawn_agent"REDACTED,
		REDACTED)
	REDACTED,
	)

	require.ErrorContains(t, err, "restore OpenAI passthrough namespace response")
	require.Equal(t, writtenPrefix, recorder.Body.String())
	require.Equal(t, []int{len(writtenPrefix)REDACTED, writer.flushBodyLengths)
REDACTED

func TestOpenAIStreamingPassthroughBlankWriteFailureDoesNotFlushAndStillDrainsUsage(t *testing.T) {
	writtenDataLine := `data: {"type":"response.output_text.delta","delta":"partial"REDACTED` + "\n"
	terminalEvent := `data: {"type":"response.completed","response":{"id":"resp_blank_failure","usage":{"input_tokens":13,"output_tokens":5,"total_tokens":18REDACTEDREDACTEDREDACTED` + "\n\n"

	result, recorder, writer, err := runPassthroughFlushTest(
		t,
		io.NopCloser(strings.NewReader(writtenDataLine+"\n"+terminalEvent)),
		1,
	)

REDACTED
	require.NotNil(t, result)
	require.Equal(t, writtenDataLine, recorder.Body.String())
	require.Empty(t, writer.flushBodyLengths)
	require.Equal(t, 1, writer.successfulWrites)
	require.Equal(t, 1, writer.failedWrites)
	require.Equal(t, 13, result.usage.InputTokens)
	require.Equal(t, 5, result.usage.OutputTokens)
REDACTED

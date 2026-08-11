package service

import (
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

func filterGrokPingTestInput(t *testing.T, input string) string {
REDACTED
	body := newGrokResponsesBillingPingFilterBody(
		io.NopCloser(strings.NewReader(input)),
		&Account{Platform: PlatformGrokREDACTED,
		defaultMaxLineSize,
	)
	output, err := io.ReadAll(body)
REDACTED
	require.NoError(t, body.Close())
	return string(output)
REDACTED

func TestGrokResponsesBillingPingFilter(t *testing.T) {
	input := strings.Join([]string{
		": upstream keepalive",
		"",
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","delta":"hello"REDACTED`,
		"",
		"event: future.vendor_event",
		`data: {"type":"future.vendor_event","value":1REDACTED`,
		"",
		"event: ping",
		`data: {"type":"ping","x-opencode-type":"inference-cost","cost":2.75,"input-tokens":42REDACTED`,
		"",
		"event: ping",
		`data: {"type":"ping","cost":"0"REDACTED`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":3,"output_tokens":5REDACTEDREDACTEDREDACTED`,
		"",
REDACTED, "\n")

	result := filterGrokPingTestInput(t, input)
	require.NotContains(t, result, "event: ping")
	require.NotContains(t, result, `"x-opencode-type":"inference-cost"`)
	require.NotContains(t, result, `{"type":"ping","cost":"0"REDACTED`)
	require.Equal(t, 2, strings.Count(result, ": ping\n\n"))
	require.Contains(t, result, ": upstream keepalive\n\n")
	require.Contains(t, result, "event: response.output_text.delta")
	require.Contains(t, result, `{"type":"response.output_text.delta","delta":"hello"REDACTED`)
	require.Contains(t, result, "event: future.vendor_event")
	require.Contains(t, result, `{"type":"future.vendor_event","value":1REDACTED`)
	require.Contains(t, result, "event: response.completed")
	require.Contains(t, result, `"usage":{"input_tokens":3,"output_tokens":5REDACTED`)
REDACTED

// Every `event: ping` frame is outside the Responses closed event enum and
// breaks strict clients regardless of its payload shape, so all variants are
// rewritten into an SSE comment (issue #5105).
func TestGrokResponsesBillingPingFilterConvertsPingVariants(t *testing.T) {
	frames := []string{
		"event: ping\ndata: {\"type\":\"ping\",\"x-opencode-type\":\"inference-cost\",\"cost\":\"0.06029240\"REDACTED\n\n",
		"event: ping\ndata: {\"type\":\"ping\",\"cost\":\"0\"REDACTED\n\n",
		"event: ping\ndata: {\"type\":\"ping\",\"cost\":\"0.06029240\"REDACTED\n\n",
		"event: ping\ndata: {\"type\":\"ping\",\"kind\":\"keepalive\"REDACTED\n\n",
		"event: ping\ndata: {\"type\":\"ping\"REDACTED\n\n",
		"event: ping\ndata: {\"type\":\"ping\",\"cost\":2REDACTED\n\n",
		"event: ping\ndata: {\"type\":\"ping\",\"cost\":0.0001REDACTED\n\n",
		"event: ping\ndata: {\"type\":\"ping\",\"cost\":\" 0 \"REDACTED\n\n",
		"event: ping\ndata: {\"type\":\"ping\",\"x-opencode-type\":\"keepalive\",\"cost\":0REDACTED\n\n",
		"event: ping\ndata: {\"type\":\"ping\",\"x-opencode-type\":null,\"cost\":0REDACTED\n\n",
		"event: ping\ndata: {\"cost\":\"0\"REDACTED\n\n",
		"event: ping\ndata: {not-jsonREDACTED\n\n",
		"event: ping\n\n",
		"event: ping\n: vendor note\ndata: {\"type\":\"ping\"REDACTED\n\n",
REDACTED
	result := filterGrokPingTestInput(t, strings.Join(frames, ""))
	require.Equal(t, strings.Repeat(": ping\n\n", len(frames)), result)
REDACTED

func TestGrokResponsesBillingPingFilterPreservesNonPingFrames(t *testing.T) {
	input := strings.Join([]string{
		"event: ping",
		`data: {"type":" ping ","cost":0REDACTED`,
		"",
		"event: ping",
		`data: {"type":"response.completed"REDACTED`,
		"",
		"event: custom",
		`data: {"type":"ping","x-opencode-type":"inference-cost"REDACTED`,
		"",
		`data: {"type":"ping","cost":"0"REDACTED`,
		"",
		": keepalive comment",
		"",
		"retry: 1000",
		"",
REDACTED, "\n")

	require.Equal(t, input, filterGrokPingTestInput(t, input))
REDACTED

// A ping candidate that turns out to carry an unexpected SSE field is not a
// vendor billing/keepalive frame; it must be replayed byte for byte.
func TestGrokResponsesBillingPingFilterPassesThroughPingFrameWithUnknownField(t *testing.T) {
	input := "event: ping\nid: 7\ndata: {\"type\":\"ping\",\"cost\":\"0\"REDACTED\n\n"
	require.Equal(t, input, filterGrokPingTestInput(t, input))
REDACTED

// Buffering caps: a ping candidate that grows past the line or byte limit is
// streamed through unchanged instead of accumulating unbounded memory.
func TestGrokResponsesBillingPingFilterPassesThroughOversizedPingFrame(t *testing.T) {
	lines := []string{"event: ping"REDACTED
	for i := 0; i < grokResponsesPingFrameMaxLines; i++ {
		lines = append(lines, ": filler comment")
REDACTED
	lines = append(lines, `data: {"type":"ping","cost":"0"REDACTED`, "")
	byLines := strings.Join(lines, "\n")
	require.Equal(t, byLines, filterGrokPingTestInput(t, byLines))

	byBytes := "event: ping\ndata: {\"type\":\"ping\",\"pad\":\"" +
		strings.Repeat("x", grokResponsesPingFrameMaxBytes) + "\"REDACTED\n\n"
	require.Equal(t, byBytes, filterGrokPingTestInput(t, byBytes))
REDACTED

func TestGrokResponsesBillingPingFilterConvertsMalformedPingFrames(t *testing.T) {
	input := "event: ping\r\ndata: {not-jsonREDACTED\r\n\r\n" +
		"event: ping\r\ndata: {\"type\":\"ping\",\"cost\":\"0\"REDACTED trailing\r\n\r\n" +
		"event: future.response.event\r\ndata: {\"type\":\"future.response.event\"REDACTED"
	want := ": ping\n\n" + ": ping\n\n" +
		"event: future.response.event\r\ndata: {\"type\":\"future.response.event\"REDACTED"
	require.Equal(t, want, filterGrokPingTestInput(t, input))
REDACTED

func TestGrokResponsesBillingPingFilterHandlesBareCRFrames(t *testing.T) {
	input := "event: ping\rdata: {\"type\":\"ping\",\"cost\":\"0\"REDACTED\r\r" +
		"event: future.event\rdata: {\"type\":\"future.event\"REDACTED\r\r"
	want := ": ping\n\n" + "event: future.event\rdata: {\"type\":\"future.event\"REDACTED\r\r"
	require.Equal(t, want, filterGrokPingTestInput(t, input))
REDACTED

func TestGrokResponsesBillingPingFilterConvertsPartialPingFrameAtEOF(t *testing.T) {
	input := "event: ping\ndata: {\"type\":\"ping\",\"cost\":\"0\"REDACTED"
	require.Equal(t, ": ping\n\n", filterGrokPingTestInput(t, input))
REDACTED

func TestGrokResponsesBillingPingFilterDoesNotFilterNonGrokAccounts(t *testing.T) {
	input := "event: ping\ndata: {\"type\":\"ping\",\"cost\":\"0\"REDACTED\n\n"
	source := io.NopCloser(strings.NewReader(input))
	body := newGrokResponsesBillingPingFilterBody(source, &Account{Platform: PlatformOpenAIREDACTED, defaultMaxLineSize)

	output, err := io.ReadAll(body)
REDACTED
	require.NoError(t, body.Close())
	require.Equal(t, input, string(output))
REDACTED

func TestGrokResponsesBillingPingFilterPreservesUsageAndTerminalEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	input := strings.Join([]string{
		"event: ping",
		`data: {"type":"ping","x-opencode-type":"inference-cost","cost":"0"REDACTED`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":3,"output_tokens":5REDACTEDREDACTEDREDACTED`,
		"",
REDACTED, "\n")
	account := &Account{ID: 1, Platform: PlatformGrokREDACTED
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{REDACTED,
		Body: newGrokResponsesBillingPingFilterBody(
			io.NopCloser(strings.NewReader(input)), account, defaultMaxLineSize,
		),
REDACTED
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{
		cfg:           &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTEDREDACTED,
		toolCorrector: NewCodexToolCorrector(),
REDACTED

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "grok-4.5", "grok-4.5")
REDACTED
	require.Equal(t, 3, result.usage.InputTokens)
	require.Equal(t, 5, result.usage.OutputTokens)
	require.Equal(t, "resp_1", result.responseID)
	require.Contains(t, recorder.Body.String(), "response.completed")
	require.NotContains(t, recorder.Body.String(), "inference-cost")
	require.NotContains(t, recorder.Body.String(), "event: ping")
REDACTED

type grokPingFilterTestReadCloser struct {
	reader     io.ReadCloser
	closeCount atomic.Int32
REDACTED

func (r *grokPingFilterTestReadCloser) Read(p []byte) (int, error) { return r.reader.Read(p) REDACTED
func (r *grokPingFilterTestReadCloser) Close() error {
	r.closeCount.Add(1)
	return r.reader.Close()
REDACTED

func TestGrokResponsesBillingPingFilterCloseCancelsSourceOnce(t *testing.T) {
	upstreamReader, upstreamWriter := io.Pipe()
	source := &grokPingFilterTestReadCloser{reader: upstreamReaderREDACTED
	body := newGrokResponsesBillingPingFilterBody(source, &Account{Platform: PlatformGrokREDACTED, defaultMaxLineSize)

	require.NoError(t, body.Close())
	require.Eventually(t, func() bool { return source.closeCount.Load() == 1 REDACTED, time.Second, time.Millisecond)
	_, err := upstreamWriter.Write([]byte("blocked"))
REDACTED
	require.NoError(t, upstreamWriter.Close())
REDACTED

func TestGrokResponsesBillingPingFilterFlushesCompletedFrames(t *testing.T) {
	upstreamReader, upstreamWriter := io.Pipe()
	body := newGrokResponsesBillingPingFilterBody(upstreamReader, &Account{Platform: PlatformGrokREDACTED, defaultMaxLineSize)
	t.Cleanup(func() { require.NoError(t, body.Close()) REDACTED)

	go func() {
		_, _ = io.WriteString(upstreamWriter, "event: future.event\ndata: {\"type\":\"future.event\"REDACTED\n\n")
REDACTED()

	result := make(chan error, 1)
	go func() {
		buffer := make([]byte, 64)
		n, err := body.Read(buffer)
		if err == nil && !strings.Contains(string(buffer[:n]), "future.event") {
			err = errors.New("completed frame was not forwarded")
	REDACTED
		result <- err
REDACTED()
	select {
	case err := <-result:
	REDACTED
	case <-time.After(time.Second):
		t.Fatal("completed frame was buffered until upstream EOF")
REDACTED
REDACTED

func TestGrokResponsesBillingPingFilterReportsOversizedLine(t *testing.T) {
	body := newGrokResponsesBillingPingFilterBody(
		io.NopCloser(strings.NewReader("data: 123456789\n\n")),
		&Account{Platform: PlatformGrokREDACTED,
		8,
	)
	_, err := io.ReadAll(body)
	require.ErrorContains(t, err, "filter Grok Responses billing ping")
	require.NoError(t, body.Close())
REDACTED

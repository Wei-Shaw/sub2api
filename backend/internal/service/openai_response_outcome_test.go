package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// deliveryTestWriter is a downstream ResponseWriter whose writes/flushes fail
// on demand. It deliberately exposes FlushError the way net/http does.
type deliveryTestWriter struct {
	header     http.Header
	mu         sync.Mutex
	buf        bytes.Buffer
	failAfter  int // fail the write that would exceed this many bytes (<0: never)
	shortWrite bool
	flushErr   error
	onFlush    func()
}

func newDeliveryTestWriter() *deliveryTestWriter {
	return &deliveryTestWriter{header: http.Header{}, failAfter: -1}
}

func (w *deliveryTestWriter) Header() http.Header { return w.header }
func (w *deliveryTestWriter) WriteHeader(int)     {}
func (w *deliveryTestWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failAfter >= 0 && w.buf.Len()+len(p) > w.failAfter {
		if w.shortWrite {
			return 0, nil
		}
		return 0, syscall.EPIPE
	}
	return w.buf.Write(p)
}
func (w *deliveryTestWriter) Flush() {}
func (w *deliveryTestWriter) FlushError() error {
	if w.onFlush != nil {
		w.onFlush()
	}
	return w.flushErr
}
func (w *deliveryTestWriter) body() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func newDeliveryTestContext(t *testing.T, ctx context.Context, w http.ResponseWriter) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	return c
}

type closeTrackingPipeBody struct {
	*io.PipeReader
	closed chan struct{}
	once   sync.Once
}

func (b *closeTrackingPipeBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return b.PipeReader.Close()
}

// ctxBoundBody mimics net/http: once the linked request context ends, reads fail.
type ctxBoundBody struct {
	ctx context.Context
}

func (b ctxBoundBody) Read([]byte) (int, error) {
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}
func (b ctxBoundBody) Close() error { return nil }

type scriptedOpenAIUpstream struct {
	respond func(req *http.Request) (*http.Response, error)
}

func (u *scriptedOpenAIUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.respond(req)
}

func (u *scriptedOpenAIUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.respond(req)
}

func deliveryTestOAuthAccount() *Account {
	return &Account{
		ID: 7, Name: "oauth-delivery", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"},
	}
}

func sseResponse(body io.ReadCloser) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: body}
}

func TestOpenAIHTTPUpstreamContextLinksOnlyOpenAIAndKeepsDeadline(t *testing.T) {
	deadline := time.Now().Add(time.Hour)
	parent, cancel := context.WithDeadline(context.Background(), deadline)
	upstreamCtx, release, linked := openAIHTTPUpstreamContext(parent, &Account{Platform: PlatformOpenAI})
	defer release()
	require.True(t, linked)
	gotDeadline, ok := upstreamCtx.Deadline()
	require.True(t, ok)
	require.Equal(t, deadline, gotDeadline)

	otherCtx, otherRelease, otherLinked := openAIHTTPUpstreamContext(parent, &Account{Platform: PlatformGrok})
	defer otherRelease()
	require.False(t, otherLinked)

	cancel()
	require.ErrorIs(t, upstreamCtx.Err(), context.Canceled)
	require.NoError(t, otherCtx.Err(), "other vendors keep the detached upstream context")

	// Releasing a linked context (body close) cancels it even if the request lives on.
	live, liveCancel := context.WithCancel(context.Background())
	defer liveCancel()
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(""))}
	linkedCtx, linkedRelease, _ := openAIHTTPUpstreamContext(live, &Account{Platform: PlatformOpenAI})
	bindOpenAIUpstreamBodyCancel(resp, linkedRelease)
	require.NoError(t, resp.Body.Close())
	require.ErrorIs(t, linkedCtx.Err(), context.Canceled)
	require.NoError(t, live.Err())
}

func TestOpenAIForwardCancelBeforeHeadersIsDeliveryNotFailover(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	upstreamCanceled := make(chan struct{})
	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream: &scriptedOpenAIUpstream{respond: func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			close(upstreamCanceled)
			return nil, req.Context().Err()
		}},
	}
	body := []byte(`{"model":"gpt-5.5","stream":false,"input":"hello"}`)
	w := newDeliveryTestWriter()
	c := newDeliveryTestContext(t, ctx, w)
	time.AfterFunc(30*time.Millisecond, cancel)

	result, err := svc.Forward(ctx, c, deliveryTestOAuthAccount(), body)

	require.Nil(t, result, "no usage observed: no fake zero result")
	deliveryErr, ok := AsOpenAIDeliveryError(err)
	require.True(t, ok, "got %v", err)
	require.Equal(t, ResponseOutcomeClientCancelled, deliveryErr.Outcome)
	require.Equal(t, OpenAIDeliveryPhaseBeforeHeaders, deliveryErr.Phase)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	<-upstreamCanceled
	require.Empty(t, w.body())
}

func TestOpenAIForwardCancelWhileBufferingAbortsUpstream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream: &scriptedOpenAIUpstream{respond: func(req *http.Request) (*http.Response, error) {
			// Headers arrive, then generation keeps buffering (simulated >90s gate).
			return sseResponse(ctxBoundBody{ctx: req.Context()}), nil
		}},
	}
	w := newDeliveryTestWriter()
	c := newDeliveryTestContext(t, ctx, w)
	time.AfterFunc(30*time.Millisecond, cancel)

	started := time.Now()
	result, err := svc.Forward(ctx, c, deliveryTestOAuthAccount(), []byte(`{"model":"gpt-5.5","stream":false,"input":"hello"}`))

	require.Less(t, time.Since(started), 2*time.Second, "cancellation must stop buffering, not drain")
	require.Nil(t, result)
	deliveryErr, ok := AsOpenAIDeliveryError(err)
	require.True(t, ok, "got %v", err)
	require.Equal(t, ResponseOutcomeClientCancelled, deliveryErr.Outcome)
	require.Equal(t, OpenAIDeliveryPhaseBuffering, deliveryErr.Phase)
	require.Empty(t, w.body())
}

const deliveryCompletedSSE = "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_ok\"}}\n\n" +
	"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n" +
	"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_ok\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"hi\"}]}],\"usage\":{\"input_tokens\":11,\"output_tokens\":5}}}\n\n"

func TestOpenAIForwardNonStreamSuccessAfterLongGeneration(t *testing.T) {
	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream: &scriptedOpenAIUpstream{respond: func(*http.Request) (*http.Response, error) {
			return sseResponse(io.NopCloser(strings.NewReader(deliveryCompletedSSE))), nil
		}},
	}
	w := newDeliveryTestWriter()
	c := newDeliveryTestContext(t, context.Background(), w)

	result, err := svc.Forward(context.Background(), c, deliveryTestOAuthAccount(), []byte(`{"model":"gpt-5.5","stream":false,"input":"hello"}`))

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, ResponseOutcomeWritten, result.ResponseOutcome)
	require.Equal(t, 11, result.Usage.InputTokens)
	require.NotNil(t, result.FirstTokenMs, "buffered SSE TTFT comes from the first semantic delta")
	require.Contains(t, w.body(), `"hi"`)

	// A >90s upstream delay is simulated through the observer clock, not a sleep.
	observer := newOpenAIFirstOutputObserver(io.NopCloser(strings.NewReader(deliveryCompletedSSE)), time.Now().Add(-120*time.Second))
	_, _ = io.ReadAll(observer)
	require.NotNil(t, observer.FirstOutputMs())
	require.GreaterOrEqual(t, *observer.FirstOutputMs(), 120000)
}

func TestOpenAIFirstOutputObserverIgnoresHeadersMetadataAndJSON(t *testing.T) {
	metadataOnly := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\"}}\n\ndata: {\"type\":\"response.in_progress\"}\n\n"
	observer := newOpenAIFirstOutputObserver(io.NopCloser(strings.NewReader(metadataOnly)), time.Now())
	_, _ = io.ReadAll(observer)
	require.Nil(t, observer.FirstOutputMs())

	jsonOnly := newOpenAIFirstOutputObserver(io.NopCloser(strings.NewReader(`{"id":"r","output":[],"usage":{"input_tokens":1}}`)), time.Now())
	_, _ = io.ReadAll(jsonOnly)
	require.Nil(t, jsonOnly.FirstOutputMs(), "JSON-only upstream: TTFT unknown, never header time")
}

func TestOpenAINonStreamingDeliveryFailuresKeepObservedUsage(t *testing.T) {
	body := `{"id":"resp_json","object":"response","output":[],"usage":{"input_tokens":20,"output_tokens":3}}`
	for _, tc := range []struct {
		name    string
		setup   func(w *deliveryTestWriter)
		outcome ResponseOutcome
	}{
		{name: "write_error", setup: func(w *deliveryTestWriter) { w.failAfter = 0 }, outcome: ResponseOutcomeWriteFailed},
		{name: "short_write", setup: func(w *deliveryTestWriter) { w.failAfter = 0; w.shortWrite = true }, outcome: ResponseOutcomeWriteFailed},
		{name: "flush_error", setup: func(w *deliveryTestWriter) { w.flushErr = syscall.ECONNRESET }, outcome: ResponseOutcomeWriteFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			w := newDeliveryTestWriter()
			tc.setup(w)
			c := newDeliveryTestContext(t, context.Background(), w)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}

			result, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "gpt-5.5", "gpt-5.5")

			deliveryErr, ok := AsOpenAIDeliveryError(err)
			require.True(t, ok, "got %v", err)
			require.Equal(t, tc.outcome, deliveryErr.Outcome)
			require.Equal(t, OpenAIDeliveryPhaseWriting, deliveryErr.Phase)
			require.NotNil(t, result)
			require.Equal(t, 20, result.usage.InputTokens, "terminal usage stays auditable")
		})
	}
}

func TestOpenAINonStreamingCancelDuringFlushIsNotWritten(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	w := newDeliveryTestWriter()
	w.onFlush = cancel // the client leaves while the native flush runs
	c := newDeliveryTestContext(t, ctx, w)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"id":"r","output":[],"usage":{"input_tokens":2,"output_tokens":1}}`))}

	result, err := svc.handleNonStreamingResponse(ctx, resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "m", "m")

	deliveryErr, ok := AsOpenAIDeliveryError(err)
	require.True(t, ok, "got %v", err)
	require.Equal(t, ResponseOutcomeClientCancelled, deliveryErr.Outcome)
	require.NotNil(t, result)
}

func TestOpenAINonStreamingOtherVendorKeepsUncheckedWrite(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	w := newDeliveryTestWriter()
	w.failAfter = 0
	c := newDeliveryTestContext(t, context.Background(), w)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"id":"r","usage":{"input_tokens":2,"output_tokens":1}}`))}

	result, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformDeepseek, Type: AccountTypeAPIKey}, "m", "m")

	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestOpenAIStreamWriteFailureAfterSemanticOutputStopsUpstream(t *testing.T) {
	pr, pw := io.Pipe()
	body := &closeTrackingPipeBody{PipeReader: pr, closed: make(chan struct{})}
	writerDone := make(chan error, 1)
	go func() {
		_, err := pw.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"))
		if err == nil {
			// Upstream keeps generating; this write only returns once the gateway
			// closes the body instead of draining.
			_, err = pw.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"more\"}\n\n"))
			if err == nil {
				_, err = pw.Write(bytes.Repeat([]byte("x"), 1<<20))
			}
		}
		writerDone <- err
	}()
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
	w := newDeliveryTestWriter()
	w.flushErr = syscall.EPIPE // the first semantic flush hits a broken pipe
	c := newDeliveryTestContext(t, context.Background(), w)

	result, err := svc.handleStreamingResponse(context.Background(), sseResponse(body), c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, time.Now(), "m", "m")

	deliveryErr, ok := AsOpenAIDeliveryError(err)
	require.True(t, ok, "got %v", err)
	require.Equal(t, ResponseOutcomeWriteFailed, deliveryErr.Outcome)
	require.NotNil(t, result)
	require.True(t, result.clientDisconnected)
	select {
	case <-body.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream body was not closed after the downstream write failure")
	}
	select {
	case werr := <-writerDone:
		require.Error(t, werr, "upstream writer must be unblocked by the close, not drained")
	case <-time.After(2 * time.Second):
		t.Fatal("upstream generation kept running (reader goroutine/drain leak)")
	}
}

func TestOpenAIStreamClientCancelAfterSemanticOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"))
		<-ctx.Done()
		_ = pw.CloseWithError(context.Canceled) // linked upstream read aborts
	}()
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
	w := newDeliveryTestWriter()
	c := newDeliveryTestContext(t, ctx, w)
	time.AfterFunc(30*time.Millisecond, cancel)

	result, err := svc.handleStreamingResponse(ctx, sseResponse(pr), c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, time.Now(), "m", "m")

	deliveryErr, ok := AsOpenAIDeliveryError(err)
	require.True(t, ok, "got %v", err)
	require.Equal(t, ResponseOutcomeClientCancelled, deliveryErr.Outcome)
	require.Equal(t, OpenAIDeliveryPhaseStreaming, deliveryErr.Phase)
	require.NotNil(t, result)
	require.Contains(t, w.body(), "hello")
}

func TestOpenAIForwardStreamCompletionIsResponseWritten(t *testing.T) {
	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream: &scriptedOpenAIUpstream{respond: func(*http.Request) (*http.Response, error) {
			return sseResponse(io.NopCloser(strings.NewReader(deliveryCompletedSSE))), nil
		}},
	}
	w := newDeliveryTestWriter()
	c := newDeliveryTestContext(t, context.Background(), w)

	result, err := svc.Forward(context.Background(), c, deliveryTestOAuthAccount(), []byte(`{"model":"gpt-5.5","stream":true,"input":"hello"}`))

	require.NoError(t, err)
	require.Equal(t, ResponseOutcomeWritten, result.ResponseOutcome)
	require.Equal(t, 5, result.Usage.OutputTokens)
	require.Contains(t, w.body(), "response.completed")
}

func TestOpenAIForwardFailoverWithReportedUsageStopsFailover(t *testing.T) {
	failedWithUsage := "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_f\",\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"The server had an error while processing your request\"},\"usage\":{\"input_tokens\":120,\"output_tokens\":7}}}\n\n"
	failedNoUsage := "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_f\",\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"The server had an error while processing your request\"}}}\n\n"
	run := func(t *testing.T, sse string) (*OpenAIForwardResult, error) {
		svc := &OpenAIGatewayService{
			cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
			httpUpstream: &scriptedOpenAIUpstream{respond: func(*http.Request) (*http.Response, error) {
				return sseResponse(io.NopCloser(strings.NewReader(sse))), nil
			}},
		}
		c := newDeliveryTestContext(t, context.Background(), newDeliveryTestWriter())
		return svc.Forward(context.Background(), c, deliveryTestOAuthAccount(), []byte(`{"model":"gpt-5.5","stream":true,"input":"hello"}`))
	}

	t.Run("supplier_usage_stops_failover", func(t *testing.T) {
		result, err := run(t, failedWithUsage)
		require.Error(t, err)
		var failoverErr *UpstreamFailoverError
		require.False(t, errors.As(err, &failoverErr), "must not retry another account after billed usage")
		stopErr, ok := AsOpenAIUpstreamFailedWithUsage(err)
		require.True(t, ok)
		require.NotNil(t, stopErr.Failover)
		require.NotNil(t, result)
		require.Equal(t, ResponseOutcomeUpstreamFailed, result.ResponseOutcome)
		require.Equal(t, 120, result.Usage.InputTokens)
		require.Equal(t, 7, result.Usage.OutputTokens)
	})
	t.Run("no_usage_keeps_failover", func(t *testing.T) {
		result, err := run(t, failedNoUsage)
		require.Nil(t, result)
		var failoverErr *UpstreamFailoverError
		require.True(t, errors.As(err, &failoverErr))
	})
}

func TestApplyOpenAITerminalUsageKeepsObservedProgressiveUsage(t *testing.T) {
	usage := OpenAIUsage{InputTokens: 40, OutputTokens: 9}
	applyOpenAITerminalUsage(&usage, OpenAIUsage{})
	require.Equal(t, OpenAIUsage{InputTokens: 40, OutputTokens: 9}, usage, "zero terminal must not erase supplier usage")
	applyOpenAITerminalUsage(&usage, OpenAIUsage{InputTokens: 41, OutputTokens: 10})
	require.Equal(t, OpenAIUsage{InputTokens: 41, OutputTokens: 10}, usage, "nonzero terminal is authoritative")
}

func TestRawChatStreamShortWriteIsTypedDeliveryFailure(t *testing.T) {
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("data: {\"id\":\"c\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		_, _ = pw.Write([]byte("data: {\"id\":\"c\",\"choices\":[{\"delta\":{\"content\":\" there\"}}]}\n\n"))
		_ = pw.Close()
	}()
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
	w := newDeliveryTestWriter()
	w.failAfter = 0
	w.shortWrite = true
	c := newDeliveryTestContext(t, context.Background(), w)

	result, err := svc.streamRawChatCompletions(c, sseResponse(pr), &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "m", "m", "m", nil, nil, time.Now(), 0)

	deliveryErr, ok := AsOpenAIDeliveryError(err)
	require.True(t, ok, "got %v", err)
	require.Equal(t, ResponseOutcomeWriteFailed, deliveryErr.Outcome)
	require.NotNil(t, result)
	require.Equal(t, ResponseOutcomeWriteFailed, result.ResponseOutcome)
}

func TestOpenAIRecordUsageFailedOutcomeConsumption(t *testing.T) {
	newInput := func(result *OpenAIForwardResult) *OpenAIRecordUsageInput {
		return &OpenAIRecordUsageInput{
			Result:  result,
			APIKey:  &APIKey{ID: 1000, Quota: 100, Group: &Group{RateMultiplier: 1}},
			User:    &User{ID: 2000},
			Account: &Account{ID: 3000, Type: AccountTypeAPIKey, Platform: PlatformOpenAI},
		}
	}
	t.Run("unknown_consumption_writes_nothing", func(t *testing.T) {
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		billingRepo := &openAIRecordUsageBillingRepoStub{}
		svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
		err := svc.RecordUsage(context.Background(), newInput(&OpenAIForwardResult{
			RequestID: "resp_cancel", Model: "gpt-5.1", ResponseOutcome: ResponseOutcomeClientCancelled,
		}))
		require.NoError(t, err)
		require.Zero(t, usageRepo.calls, "no fake zero-consumption row")
		require.Zero(t, billingRepo.calls)
	})
	t.Run("observed_failed_consumption_recorded_once_with_outcome", func(t *testing.T) {
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		billingRepo := &openAIRecordUsageBillingRepoStub{}
		svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
		err := svc.RecordUsage(context.Background(), newInput(&OpenAIForwardResult{
			RequestID: "resp_failed", Model: "gpt-5.1", ResponseOutcome: ResponseOutcomeUpstreamFailed,
			Usage: OpenAIUsage{InputTokens: 120, OutputTokens: 7},
		}))
		require.NoError(t, err)
		require.Equal(t, 1, usageRepo.calls)
		require.Equal(t, 1, billingRepo.calls)
		require.NotNil(t, usageRepo.lastLog.ResponseOutcome)
		require.Equal(t, string(ResponseOutcomeUpstreamFailed), *usageRepo.lastLog.ResponseOutcome)
		require.Equal(t, 7, usageRepo.lastLog.OutputTokens)
	})
	t.Run("legacy_result_keeps_null_outcome", func(t *testing.T) {
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, &openAIRecordUsageBillingRepoStub{}, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
		require.NoError(t, svc.RecordUsage(context.Background(), newInput(&OpenAIForwardResult{RequestID: "resp_legacy", Model: "gpt-5.1"})))
		require.Equal(t, 1, usageRepo.calls)
		require.Nil(t, usageRepo.lastLog.ResponseOutcome)
	})
}

func TestOpenAIRawAdapterPartialFailureNeverUsesLegacyOutcome(t *testing.T) {
	c := newDeliveryTestContext(t, context.Background(), newDeliveryTestWriter())
	account := deliveryTestOAuthAccount()
	result := &OpenAIForwardResult{Usage: OpenAIUsage{InputTokens: 40, OutputTokens: 9}}
	upstreamErr := errors.New("synthetic upstream stream truncated")
	got, err := classifyOpenAIHTTPPartialFailure(c, account, result, upstreamErr)
	require.ErrorIs(t, err, upstreamErr)
	require.Same(t, result, got)
	require.Equal(t, ResponseOutcomeUpstreamFailed, got.ResponseOutcome)
	require.True(t, got.HasObservedConsumption())
	require.False(t, got.ClientDisconnect)

	failoverErr := &UpstreamFailoverError{StatusCode: http.StatusBadGateway}
	got, err = classifyOpenAIHTTPPartialFailure(c, account, result, failoverErr)
	_, stopped := AsOpenAIUpstreamFailedWithUsage(err)
	require.True(t, stopped)
	require.Equal(t, ResponseOutcomeUpstreamFailed, got.ResponseOutcome)
	var retryErr *UpstreamFailoverError
	require.False(t, errors.As(err, &retryErr))

	unknown := &OpenAIForwardResult{}
	_, err = classifyOpenAIHTTPPartialFailure(c, account, unknown, failoverErr)
	require.True(t, errors.As(err, &retryErr), "no observed usage preserves failover")
	other := &OpenAIForwardResult{Usage: OpenAIUsage{InputTokens: 40}}
	otherAccount := &Account{Platform: PlatformGrok}
	got, err = classifyOpenAIHTTPPartialFailure(c, otherAccount, other, upstreamErr)
	require.ErrorIs(t, err, upstreamErr)
	require.Empty(t, got.ResponseOutcome, "other vendor adapter contract remains")
}

type observedUsageResetReader struct{}

func (observedUsageResetReader) Read([]byte) (int, error) { return 0, syscall.ECONNRESET }

func TestOpenAIChatBufferedReadFailurePreservesObservedUsage(t *testing.T) {
	progress := "data: {\"type\":\"response.in_progress\",\"response\":{\"usage\":{\"input_tokens\":42,\"output_tokens\":7}}}\n\n"
	for _, tc := range []struct {
		name string
		tail io.Reader
	}{
		{name: "reset", tail: observedUsageResetReader{}},
		{name: "clean_eof", tail: strings.NewReader("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
			c := newDeliveryTestContext(t, context.Background(), newDeliveryTestWriter())
			resp := sseResponse(io.NopCloser(io.MultiReader(strings.NewReader(progress), tc.tail)))
			result, err := svc.handleChatBufferedStreamingResponse(resp, c, deliveryTestOAuthAccount(), "gpt-5.5", "gpt-5.5", "gpt-5.5", time.Now())
			require.Error(t, err)
			require.NotNil(t, result, "supplier-reported consumption must survive the read failure")
			require.Equal(t, 42, result.Usage.InputTokens)
			require.Equal(t, 7, result.Usage.OutputTokens)
			require.Equal(t, ResponseOutcomeUpstreamFailed, result.ResponseOutcome)
			var retry *UpstreamFailoverError
			require.False(t, errors.As(err, &retry), "billed attempts must not silently fail over")
		})
	}
}

func TestOpenAICompactBridgeDeliveryFailuresKeepObservedUsage(t *testing.T) {
	body := `{"id":"resp_compact","object":"response","output":[{"type":"compaction","encrypted_content":"synthetic"}],"usage":{"input_tokens":42,"output_tokens":7,"total_tokens":49}}`
	for _, tc := range []struct {
		name   string
		setup  func(*deliveryTestWriter)
		cancel bool
	}{
		{name: "write_error", setup: func(w *deliveryTestWriter) { w.failAfter = 0 }},
		{name: "short_write", setup: func(w *deliveryTestWriter) { w.failAfter = 0; w.shortWrite = true }},
		{name: "flush_error", setup: func(w *deliveryTestWriter) { w.flushErr = syscall.EPIPE }},
		{name: "cancelled", setup: func(w *deliveryTestWriter) {}, cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w := newDeliveryTestWriter()
			tc.setup(w)
			c := newDeliveryTestContext(t, ctx, w)
			MarkOpenAICompactClientStream(c)
			if tc.cancel {
				cancel()
			}
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
			result, err := svc.handleNonStreamingResponse(ctx, resp, c, deliveryTestOAuthAccount(), "gpt-5.5", "gpt-5.5")
			require.Error(t, err)
			require.NotNil(t, result)
			require.Equal(t, 42, result.usage.InputTokens)
			require.Equal(t, 7, result.usage.OutputTokens)
			delivery, ok := AsOpenAIDeliveryError(err)
			require.True(t, ok)
			if tc.cancel {
				require.Equal(t, ResponseOutcomeClientCancelled, delivery.Outcome)
			} else {
				require.Equal(t, ResponseOutcomeWriteFailed, delivery.Outcome)
			}
		})
	}
}

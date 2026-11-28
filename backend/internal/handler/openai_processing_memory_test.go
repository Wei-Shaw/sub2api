package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type processingBudgetUpstream struct {
	calls   atomic.Int32
	block   atomic.Bool
	started chan struct{}
	finish  chan struct{}
	// stream, when set, receives the response body writer of the next call
	// so the test can hold the stream open after headers are returned.
	stream chan *io.PipeWriter
}

func (u *processingBudgetUpstream) Do(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls.Add(1)
	_, err := io.Copy(io.Discard, r.Body)
	_ = r.Body.Close()
	if err != nil {
		return nil, err
	}
	if u.stream != nil {
		pr, pw := io.Pipe()
		stream := u.stream
		u.stream = nil
		go func() { stream <- pw }()
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: pr}, nil
	}
	if u.block.Load() {
		close(u.started)
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-u.finish:
			return nil, context.Canceled
		}
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_budget\",\"model\":\"gpt-5\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\ndata: [DONE]\n\n"))}, nil
}
func (u *processingBudgetUpstream) DoWithTLS(r *http.Request, p string, id int64, n int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(r, p, id, n)
}

func newProcessingBudgetHandler(t *testing.T) (*OpenAIGatewayHandler, *processingBudgetUpstream, *openAIWSUsageHandlerUsageLogRepoStub, *openAIWSFailoverHandlerAccountRepoStub) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Gateway.MaxBodySize = 1024
	// A tiny passthrough fixture isolates quota ownership/cancellation from the
	// OAuth default instructions. The bounded incident diagnostic separately
	// exercises the complete OAuth transform with 44MiB image requests.
	accounts := &openAIWSFailoverHandlerAccountRepoStub{accounts: []service.Account{{ID: 1, Name: "synthetic", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Priority: 1, Credentials: map[string]any{"api_key": "synthetic", "base_url": "https://upstream.example.test"}, Extra: map[string]any{"openai_passthrough": true}}}}
	usage := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 8)}
	upstream := &processingBudgetUpstream{started: make(chan struct{}), finish: make(chan struct{})}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	gateway := service.NewOpenAIGatewayService(accounts, usage, nil, nil, nil, nil, nil, cfg, nil, nil, service.NewBillingService(cfg, nil), nil, billing, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billing, service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
	return h, upstream, usage, accounts
}

func runProcessingBudgetRequest(h *OpenAIGatewayHandler, ctx context.Context, budget *httputil.BodyMemoryBudget, body string, unknownLength bool) *httptest.ResponseRecorder {
	ctx, release := httputil.WithRequestBodyMemoryBudget(ctx, budget)
	defer release()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body)).WithContext(ctx)
	if unknownLength {
		c.Request.ContentLength = -1
	}
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1, User: &service.User{ID: 1, Status: service.StatusActive}})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
	h.Responses(c)
	return rec
}

const processingBudgetSmallRequest = `{"model":"gpt-5","input":"hi","stream":false}`

func TestResponsesProcessingBudgetRejectsBeforeUpstreamAndReleases(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "known", true: "unknown"}[unknown], func(t *testing.T) {
			h, u, usage, accounts := newProcessingBudgetHandler(t)
			budget := httputil.NewBodyMemoryBudget(2048)
			large := `{"model":"gpt-5","input":"` + strings.Repeat("x", 250) + `","stream":false}`
			rec := runProcessingBudgetRequest(h, context.Background(), budget, large, unknown)
			require.Equal(t, 413, rec.Code, "a body that can never fit must not be retried as overload")
			require.Empty(t, rec.Header().Get("Retry-After"))
			require.Zero(t, budget.Used())

			busy, releaseBusy := httputil.WithRequestBodyMemoryBudget(context.Background(), budget)
			require.NoError(t, httputil.ReserveRequestBodyMemory(busy, 1500))
			rec = runProcessingBudgetRequest(h, context.Background(), budget, processingBudgetSmallRequest, unknown)
			require.Equal(t, 503, rec.Code)
			require.Equal(t, "1", rec.Header().Get("Retry-After"))
			releaseBusy()
			require.Zero(t, u.calls.Load())
			require.Empty(t, usage.created)
			require.Empty(t, accounts.rateLimitedIDs)
			require.Zero(t, budget.Used())
			rec = runProcessingBudgetRequest(h, context.Background(), budget, processingBudgetSmallRequest, false)
			require.Equal(t, 200, rec.Code, rec.Body.String())
			require.EqualValues(t, 1, u.calls.Load())
			require.Len(t, usage.created, 1)
		})
	}
}

func TestResponsesProcessingBudgetSettlesWhileStreaming(t *testing.T) {
	h, u, usage, _ := newProcessingBudgetHandler(t)
	budget := httputil.NewBodyMemoryBudget(2048)
	streamed := `{"model":"gpt-5","input":"hi","stream":true}`
	stream := make(chan *io.PipeWriter, 1)
	u.stream = stream
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- runProcessingBudgetRequest(h, context.Background(), budget, streamed, false) }()
	var pw *io.PipeWriter
	select {
	case pw = <-stream:
	case <-time.After(3 * time.Second):
		t.Fatal("first request did not reach upstream")
	}
	// Headers are back and the stream is open: only ingress and one outgoing
	// copy stay reserved, so the transform peak no longer blocks the next request.
	require.Less(t, budget.Used(), int64(3*len(streamed)))
	rec := runProcessingBudgetRequest(h, context.Background(), budget, processingBudgetSmallRequest, false)
	require.Equal(t, 200, rec.Code, rec.Body.String())

	_, _ = pw.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_stream\",\"model\":\"gpt-5\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\ndata: [DONE]\n\n"))
	_ = pw.Close()
	select {
	case rec = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("streaming request did not finish")
	}
	require.Equal(t, 200, rec.Code, rec.Body.String())
	require.Zero(t, budget.Used())
	require.EqualValues(t, 2, u.calls.Load())
	require.Len(t, usage.created, 2)
}

func TestResponsesProcessingBudgetPreserves413AndExplicitOptOut(t *testing.T) {
	h, u, usage, _ := newProcessingBudgetHandler(t)
	rec := runProcessingBudgetRequest(h, context.Background(), httputil.NewBodyMemoryBudget(1<<20), strings.Repeat("x", 2048), false)
	require.Equal(t, 413, rec.Code)
	require.Zero(t, u.calls.Load())
	require.Empty(t, usage.created)
	rec = runProcessingBudgetRequest(h, context.Background(), httputil.NewBodyMemoryBudget(0), `{"model":"gpt-5","input":"`+strings.Repeat("x", 250)+`","stream":false}`, false)
	require.Equal(t, 200, rec.Code, rec.Body.String())
	require.EqualValues(t, 1, u.calls.Load())
	require.Len(t, usage.created, 1)
}

func TestResponsesProcessingBudgetHeldUntilCancellationAndReusable(t *testing.T) {
	h, u, usage, accounts := newProcessingBudgetHandler(t)
	budget := httputil.NewBodyMemoryBudget(2048)
	u.block.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		runProcessingBudgetRequest(h, ctx, budget, processingBudgetSmallRequest, false)
	}()
	select {
	case <-u.started:
	case <-time.After(3 * time.Second):
		t.Fatal("first request did not reach upstream")
	}
	rec := runProcessingBudgetRequest(h, context.Background(), budget, processingBudgetSmallRequest, false)
	require.Equal(t, 503, rec.Code)
	require.EqualValues(t, 1, u.calls.Load())
	require.Empty(t, usage.created)
	cancel()
	// The real transport can detach upstream cancellation for bounded usage
	// draining. Quota must stay held until that actual work returns, not merely
	// until the downstream context is cancelled.
	select {
	case <-done:
	default:
		rec = runProcessingBudgetRequest(h, context.Background(), budget, processingBudgetSmallRequest, false)
		require.Equal(t, 503, rec.Code)
	}
	close(u.finish)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled request retained its budget")
	}
	u.block.Store(false)
	rec = runProcessingBudgetRequest(h, context.Background(), budget, processingBudgetSmallRequest, false)
	require.Equal(t, 200, rec.Code, rec.Body.String())
	require.EqualValues(t, 2, u.calls.Load())
	require.Len(t, usage.created, 1)
	require.Empty(t, accounts.rateLimitedIDs)
}

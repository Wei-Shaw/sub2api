//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// slowAccountWriteRepo simulates Postgres latency on the two writes a
// successful Anthropic OAuth response triggers.
type slowAccountWriteRepo struct {
	AccountRepository
	delay time.Duration

	mu                 sync.Mutex
	sessionWindowCalls int
	extraCalls         int
}

func (r *slowAccountWriteRepo) UpdateSessionWindow(context.Context, int64, *time.Time, *time.Time, string) error {
	time.Sleep(r.delay)
	r.mu.Lock()
	r.sessionWindowCalls++
	r.mu.Unlock()
	return nil
}

func (r *slowAccountWriteRepo) UpdateExtra(context.Context, int64, map[string]any) error {
	time.Sleep(r.delay)
	r.mu.Lock()
	r.extraCalls++
	r.mu.Unlock()
	return nil
}

// gatedAccountWriteRepo blocks account writes while a gate is set, so a write
// made on the request goroutine shows up as UpdateSessionWindow not returning.
type gatedAccountWriteRepo struct {
	AccountRepository

	mu                 sync.Mutex
	gate               chan struct{}
	sessionWindowCalls int
	extra              []map[string]any
}

func (r *gatedAccountWriteRepo) setGate(gate chan struct{}) {
	r.mu.Lock()
	r.gate = gate
	r.mu.Unlock()
}

func (r *gatedAccountWriteRepo) waitGate() {
	r.mu.Lock()
	gate := r.gate
	r.mu.Unlock()
	if gate != nil {
		<-gate
	}
}

func (r *gatedAccountWriteRepo) UpdateSessionWindow(context.Context, int64, *time.Time, *time.Time, string) error {
	r.waitGate()
	r.mu.Lock()
	r.sessionWindowCalls++
	r.mu.Unlock()
	return nil
}

func (r *gatedAccountWriteRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.waitGate()
	r.mu.Lock()
	r.extra = append(r.extra, updates)
	r.mu.Unlock()
	return nil
}

func (r *gatedAccountWriteRepo) snapshot() (sessionWindowCalls int, extra []map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessionWindowCalls, append([]map[string]any(nil), r.extra...)
}

func sessionWindowHeaders(status string, windowEnd time.Time, utilization string) http.Header {
	h := http.Header{}
	h.Set("anthropic-ratelimit-unified-5h-status", status)
	h.Set("anthropic-ratelimit-unified-5h-reset", strconv.FormatInt(windowEnd.Unix(), 10))
	if utilization != "" {
		h.Set("anthropic-ratelimit-unified-5h-utilization", utilization)
	}
	return h
}

func TestUpdateSessionWindow_SteadyStateWritesLeaveRequestPath(t *testing.T) {
	repo := &gatedAccountWriteRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	svc.passiveUsage.interval = 50 * time.Millisecond
	windowEnd := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	account := &Account{ID: 7, SessionWindowEnd: &windowEnd}
	ctx := context.Background()

	// The account's first response records its status and samples usage.
	svc.UpdateSessionWindow(ctx, account, sessionWindowHeaders("allowed", windowEnd, "0.10"))
	require.Eventually(t, func() bool { _, extra := repo.snapshot(); return len(extra) == 1 }, 2*time.Second, 5*time.Millisecond)

	// Steady state: every account write blocks, and the response path must not wait on any.
	gate := make(chan struct{})
	repo.setGate(gate)
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		svc.UpdateSessionWindow(ctx, account, sessionWindowHeaders("allowed", windowEnd, "0.20"))
		svc.UpdateSessionWindow(ctx, account, sessionWindowHeaders("allowed", windowEnd, "0.30"))
	}()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		close(gate)
		t.Fatal("UpdateSessionWindow waited on a synchronous DB write")
	}
	close(gate)

	// Both samples coalesce into one deferred write that carries the newest value.
	require.Eventually(t, func() bool { _, extra := repo.snapshot(); return len(extra) == 2 }, 2*time.Second, 5*time.Millisecond)
	sessionWindowCalls, extra := repo.snapshot()
	require.Equal(t, 0.30, extra[1]["session_window_utilization"])
	require.Equal(t, 1, sessionWindowCalls, "an unchanged status is not rewritten")
}

func TestFlushPassiveUsage_WritesPendingSamples(t *testing.T) {
	repo := &gatedAccountWriteRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	svc.passiveUsage.interval = 100 * time.Millisecond
	svc.passiveUsage.lastAt[7] = time.Now() // keep the samples pending until the interval ends
	svc.passiveUsage.enqueue(7, map[string]any{"session_window_utilization": 0.20})
	svc.passiveUsage.enqueue(7, map[string]any{"session_window_utilization": 0.30})

	svc.FlushPassiveUsage(context.Background())
	_, extra := repo.snapshot()
	require.Len(t, extra, 1)
	require.Equal(t, 0.30, extra[0]["session_window_utilization"])

	// The already-scheduled timer finds nothing left and does not write again.
	require.Never(t, func() bool { _, extra := repo.snapshot(); return len(extra) != 1 }, 300*time.Millisecond, 10*time.Millisecond)
}

func TestUpdateSessionWindow_RewritesStatusOnlyWhenChanged(t *testing.T) {
	repo := &anthropicWindowLimitRepo{}
	svc := newRateLimitServiceForTest(repo)
	windowEnd := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	account := &Account{ID: 42, Type: AccountTypeOAuth, Platform: PlatformAnthropic, SessionWindowEnd: &windowEnd}
	ctx := context.Background()

	svc.UpdateSessionWindow(ctx, account, sessionWindowHeaders("allowed", windowEnd, ""))
	svc.UpdateSessionWindow(ctx, account, sessionWindowHeaders("allowed", windowEnd, ""))
	require.Equal(t, 1, repo.sessionWindowCalls, "an unchanged status skips the write")

	// A 429 persists "rejected", so the next "allowed" must be written again.
	rejected := sessionWindowHeaders("allowed", windowEnd, "0.41")
	rejected.Set("anthropic-ratelimit-unified-7d-reset", strconv.FormatInt(windowEnd.Add(72*time.Hour).Unix(), 10))
	rejected.Set("anthropic-ratelimit-unified-7d-status", "allowed")
	rejected.Set("anthropic-ratelimit-unified-7d-utilization", "0.56")
	svc.HandleUpstreamError(ctx, account, http.StatusTooManyRequests, rejected, nil)
	require.Equal(t, 2, repo.sessionWindowCalls)

	svc.UpdateSessionWindow(ctx, account, sessionWindowHeaders("allowed", windowEnd, ""))
	svc.UpdateSessionWindow(ctx, account, sessionWindowHeaders("allowed_warning", windowEnd, ""))
	require.Equal(t, 4, repo.sessionWindowCalls)
}

// ttfbRecorder timestamps the first byte the gateway sends to the client.
type ttfbRecorder struct {
	*httptest.ResponseRecorder
	first time.Time
}

func (r *ttfbRecorder) mark() {
	if r.first.IsZero() {
		r.first = time.Now()
	}
}

func (r *ttfbRecorder) WriteHeader(code int) {
	r.mark()
	r.ResponseRecorder.WriteHeader(code)
}

func (r *ttfbRecorder) Write(p []byte) (int, error) {
	r.mark()
	return r.ResponseRecorder.Write(p)
}

func sessionWindowStreamResponse(windowEnd time.Time) *http.Response {
	sse := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-3-5-sonnet-latest","content":[],"usage":{"input_tokens":11}}}`,
		"",
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
		"",
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		"",
		"",
	}, "\n")
	header := http.Header{}
	header.Set("Content-Type", "text/event-stream")
	header.Set("anthropic-ratelimit-unified-5h-status", "allowed")
	header.Set("anthropic-ratelimit-unified-5h-reset", strconv.FormatInt(windowEnd.Unix(), 10))
	header.Set("anthropic-ratelimit-unified-5h-utilization", "0.25")
	header.Set("anthropic-ratelimit-unified-7d-utilization", "0.40")
	return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(sse))}
}

// BenchmarkForwardTTFB_SlowAccountWrites measures time-to-first-byte of a
// streaming Anthropic OAuth Forward when each account write takes 5ms.
func BenchmarkForwardTTFB_SlowAccountWrites(b *testing.B) {
	gin.SetMode(gin.TestMode)
	repo := &slowAccountWriteRepo{delay: 5 * time.Millisecond}
	svc := newForwardPartialUsageServiceForTest(nil)
	svc.rateLimitService = NewRateLimitService(repo, nil, nil, nil, nil)
	account := newAnthropicOAuthAccountForPartialUsageTest()
	windowEnd := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	account.SessionWindowEnd = &windowEnd
	body := []byte(`{"model":"claude-3-5-sonnet-latest","stream":true,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)

	var ttfb time.Duration
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.httpUpstream = &anthropicHTTPUpstreamRecorder{resp: sessionWindowStreamResponse(windowEnd)}
		rec := &ttfbRecorder{ResponseRecorder: httptest.NewRecorder()}
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
		parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
		require.NoError(b, err)

		start := time.Now()
		_, err = svc.Forward(context.Background(), c, account, parsed)
		require.NoError(b, err)
		require.False(b, rec.first.IsZero())
		ttfb += rec.first.Sub(start)
	}
	b.ReportMetric(float64(ttfb.Microseconds())/1000/float64(b.N), "ttfb-ms")
}

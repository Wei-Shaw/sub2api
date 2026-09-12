//go:build unit

package handler

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

const capacityHandlerError = "data: {\"type\":\"error\",\"error\":{\"code\":\"server_error\",\"type\":\"service_unavailable_error\",\"message\":\"Our servers are currently overloaded. Please try again later.\"}}\n\n"
const capacityHandlerSuccess = "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"response_ok\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"

type capacityHandlerRepo struct {
	openAIImagesFailoverAccountRepo
	changed chan struct{}
}

func (r *capacityHandlerRepo) SetModelRateLimit(_ context.Context, id int64, model string, until time.Time, reason ...string) error {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			r.accounts[i].Extra = map[string]any{"model_rate_limits": map[string]any{
				model: map[string]any{"rate_limit_reset_at": until.UTC().Format(time.RFC3339)},
			}}
		}
	}
	select {
	case r.changed <- struct{}{}:
	default:
	}
	return nil
}

type capacityHandlerSticky struct{ service.GatewayCache }

func (*capacityHandlerSticky) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	return 1, nil
}
func (*capacityHandlerSticky) SetSessionAccountID(context.Context, int64, string, int64, time.Duration) error {
	return nil
}
func (*capacityHandlerSticky) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}
func (*capacityHandlerSticky) DeleteSessionAccountID(context.Context, int64, string) error {
	return nil
}

type capacityHandlerUsage struct {
	service.UsageLogRepository
	calls int
}

func (u *capacityHandlerUsage) Create(context.Context, *service.UsageLog) (bool, error) {
	u.calls++
	return true, nil
}

type capacityHandlerSettings struct{ service.SettingRepository }

func (*capacityHandlerSettings) SetMultiple(context.Context, map[string]string) error { return nil }

func (*capacityHandlerSettings) GetValue(context.Context, string) (string, error) {
	return `{"enabled":true,"window_minutes":10,"failure_threshold":100,"cooldown_minutes":2}`, nil
}

func (*capacityHandlerSettings) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

type capacityHandlerHealth struct {
	service.TempUnschedCache
	calls int
}

func (h *capacityHandlerHealth) RecordOpenAIAPIKeyHealthFailure(context.Context, int64, int, int) (int64, bool, error) {
	h.calls++
	return int64(h.calls), false, nil
}

type capacityHandlerBody struct {
	*io.PipeReader
	closed chan struct{}
	once   sync.Once
}

func (b *capacityHandlerBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return b.PipeReader.Close()
}

type capacityHandlerUpstream struct {
	service.HTTPUpstream
	mode       string
	calls      []int64
	failedBody *capacityHandlerBody
	onError    func()
}

func (u *capacityHandlerUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.calls = append(u.calls, accountID)
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
	if u.mode == "canceled_524" || u.mode == "canceled_only" {
		u.onError()
		if u.mode == "canceled_only" {
			return nil, context.Canceled
		}
		return &http.Response{StatusCode: 524, Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(strings.NewReader("error code: 524"))}, nil
	}
	if u.mode == "healthy" || (accountID == 2 && u.mode != "all_failed") {
		resp.Body = io.NopCloser(strings.NewReader(capacityHandlerSuccess))
		return resp, nil
	}
	if u.mode != "post_output" && u.mode != "keepalive" {
		resp.Body = io.NopCloser(strings.NewReader(capacityHandlerError))
		return resp, nil
	}
	reader, writer := io.Pipe()
	body := &capacityHandlerBody{PipeReader: reader, closed: make(chan struct{})}
	u.failedBody = body
	resp.Body = body
	go func() {
		defer writer.Close()
		preamble := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_attempt\"}}\n\ndata: {\"type\":\"keepalive\",\"sequence_number\":1}\n\n"
		if u.mode == "post_output" {
			preamble += "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"
		}
		_, _ = io.WriteString(writer, preamble+capacityHandlerError)
		<-body.closed
	}()
	return resp, nil
}

func TestOpenAICapacityCanceledRequestStillObservesConfirmedFailure(t *testing.T) {
	for _, mode := range []string{"canceled_524", "canceled_only"} {
		t.Run(mode, func(t *testing.T) {
			handler, upstream, _, slots, health := newCapacityRecoveryHandler(t, mode, false)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			upstream.onError = cancel
			c, _ := newOpenAIResponsesFailoverTestContext(t, ctx)
			handler.Responses(c)
			require.Equal(t, []int64{1}, upstream.calls, "never replay after cancellation")
			require.Equal(t, int32(1), atomic.LoadInt32(&slots.releaseAccountCalled))
			if mode == "canceled_524" {
				require.Equal(t, 1, health.calls, "confirmed upstream failure must be observed exactly once")
			} else {
				require.Zero(t, health.calls, "client cancellation is not an account failure")
			}
		})
	}
}

func newCapacityRecoveryHandler(t *testing.T, mode string, explicitRules bool, stickyCache ...service.GatewayCache) (*OpenAIGatewayHandler, *capacityHandlerUpstream, *capacityHandlerRepo, *concurrencyCacheMock, *capacityHandlerHealth) {
	t.Helper()
	repo := &capacityHandlerRepo{changed: make(chan struct{}, 4)}
	for i := int64(1); i <= 2; i++ {
		rate := 0.2
		credentials := map[string]any{"api_key": "test-only", "pool_mode": true, "pool_mode_retry_count": float64(2)}
		if explicitRules {
			credentials["temp_unschedulable_enabled"] = true
			credentials["temp_unschedulable_rules"] = []any{map[string]any{
				"error_code": float64(503), "keywords": []any{"overloaded"}, "duration_minutes": float64(2),
			}}
		}
		repo.accounts = append(repo.accounts, service.Account{
			ID: i, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
			Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: int(i),
			Credentials: credentials, GroupIDs: []int64{3131}, RateMultiplier: &rate,
		})
	}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.StreamDataIntervalTimeout = 180
	cache := &concurrencyCacheMock{
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
		acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
	}
	concurrency := service.NewConcurrencyService(cache)
	health := &capacityHandlerHealth{}
	rateLimit := service.NewRateLimitService(repo, nil, cfg, nil, nil)
	rateLimit.SetSettingService(service.NewSettingService(&capacityHandlerSettings{}, cfg))
	rateLimit.SetOpenAIAPIKeyHealthCache(health)
	upstream := &capacityHandlerUpstream{mode: mode}
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	var sticky service.GatewayCache = &capacityHandlerSticky{}
	if len(stickyCache) > 0 {
		sticky = stickyCache[0]
	}
	gateway := service.NewOpenAIGatewayService(
		repo, &capacityHandlerUsage{}, nil, nil, nil, nil, sticky, cfg, nil, concurrency,
		service.NewBillingService(cfg, nil), rateLimit, billingCache, upstream,
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	handler := NewOpenAIGatewayHandler(gateway, concurrency, billingCache,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
	handler.maxAccountSwitches = 2
	return handler, upstream, repo, cache, health
}

func TestOpenAIStickySuccessHandlerFailureRecoveryAndNextRequest(t *testing.T) {
	settings := service.NewSettingService(&capacityHandlerSettings{}, &config.Config{})
	require.NoError(t, settings.UpdateSettings(context.Background(), &service.SystemSettings{
		OpenAIAdvancedSchedulerEnabled: true, OpenAIAdvancedSchedulerStickyWeightedEnabled: true,
	}))
	t.Cleanup(func() { require.NoError(t, settings.UpdateSettings(context.Background(), &service.SystemSettings{})) })
	r := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: r.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := repository.NewGatewayCache(client)
	handler, upstream, repo, _, _ := newCapacityRecoveryHandler(t, "keepalive", true, cache)
	group := &service.Group{ID: 3131, Hydrated: true, Status: service.StatusActive, Platform: service.PlatformOpenAI, RateMultiplier: 1, ProfitControlEnabled: true, ProfitMinMargin: 0.5}
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)
	first, rec := newOpenAIResponsesFailoverTestContext(t, ctx)
	first.Request.Header.Set("session_id", "success-session")
	first.Request.Body = io.NopCloser(strings.NewReader(`{"model":"gpt-5.1","stream":true,"input":"hello"}`))
	sessionHash := handler.gatewayService.GenerateSessionHash(first, nil)
	require.NoError(t, cache.SetSessionAccountID(ctx, group.ID, "openai:"+sessionHash, 1, time.Hour))
	handler.Responses(first)
	require.Equal(t, []int64{1, 2}, upstream.calls)
	require.Contains(t, rec.Body.String(), "response_ok")
	successCache, ok := cache.(service.OpenAIStickySuccessCache)
	require.True(t, ok)
	preference, err := successCache.GetOpenAIStickySuccess(ctx, group.ID, sessionHash, "gpt-5.1")
	require.NoError(t, err)
	require.Equal(t, int64(2), preference.AccountID)
	// The old account has recovered. It must not reclaim this model's affinity.
	repo.accounts[0].Extra = nil
	upstream.mode = "healthy"
	next, nextRec := newOpenAIResponsesFailoverTestContext(t, ctx)
	next.Request.Header.Set("session_id", "success-session")
	handler.Responses(next)
	require.Equal(t, []int64{1, 2, 2}, upstream.calls)
	require.Contains(t, nextRec.Body.String(), "response_ok")
}

func TestOpenAICapacityKeepaliveFailsOverWithinRequest(t *testing.T) {
	handler, upstream, repo, slots, health := newCapacityRecoveryHandler(t, "keepalive", true)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, rec := newOpenAIResponsesFailoverTestContext(t, ctx)
	c.Request.Body = io.NopCloser(strings.NewReader(`{"model":"gpt-5.1","stream":true,"input":"hello"}`))
	c.Request.Header.Set("session_id", "same-session")
	handler.Responses(c)
	require.Equal(t, []int64{1, 2}, upstream.calls, "heartbeat-only failure must switch before returning to the client")
	require.False(t, repo.accounts[0].IsSchedulableForModelWithContext(context.Background(), "gpt-5.1"))
	require.Zero(t, health.calls)
	require.Equal(t, int32(2), atomic.LoadInt32(&slots.releaseAccountCalled))
	require.Contains(t, rec.Body.String(), "response_ok")
	require.NotContains(t, rec.Body.String(), "resp_attempt", "failed attempt preamble must remain private")
	require.NotContains(t, rec.Body.String(), "overloaded")
	select {
	case <-upstream.failedBody.closed:
	default:
		t.Fatal("failed upstream connection was not closed")
	}
}

func TestOpenAICapacityClientReconnectAvoidsCooledStickyAccount(t *testing.T) {
	handler, upstream, repo, slots, health := newCapacityRecoveryHandler(t, "post_output", true)
	c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
	c.Request.Body = io.NopCloser(strings.NewReader(`{"model":"gpt-5.1","stream":true,"input":"hello"}`))
	c.Request.Header.Set("session_id", "same-session")
	started := time.Now()
	done := make(chan struct{})
	go func() { defer close(done); handler.Responses(c) }()
	select {
	case <-repo.changed:
	case <-done:
		t.Fatalf("request ended before cooling: status=%d body=%s", rec.Code, rec.Body.String())
	case <-time.After(3 * time.Second):
		t.Fatal("capacity observation did not reach account policy")
	}
	require.Equal(t, []int64{1}, upstream.calls)
	require.False(t, repo.accounts[0].IsSchedulableForModelWithContext(context.Background(), "gpt-5.1"))
	require.Zero(t, health.calls, "an explicit rule must not also count towards whole-account health")
	require.Zero(t, atomic.LoadInt32(&slots.releaseAccountCalled), "slot is held only for bounded usage collection")
	<-done
	require.Less(t, time.Since(started), 5*time.Second)
	require.Equal(t, int32(1), atomic.LoadInt32(&slots.releaseAccountCalled))
	require.Contains(t, rec.Body.String(), "partial")
	require.NotContains(t, rec.Body.String(), "response_ok", "committed stream must not replay")

	retry, retryRec := newOpenAIResponsesFailoverTestContext(t, nil)
	retry.Request.Header.Set("session_id", "same-session")
	handler.Responses(retry)
	require.Equal(t, []int64{1, 2}, upstream.calls, "even a stale sticky cache returning account 1 must not bypass cooling")
	require.Contains(t, retryRec.Body.String(), "response_ok")
	require.Equal(t, int32(2), atomic.LoadInt32(&slots.releaseAccountCalled))
}

func TestOpenAICapacityPoolExhaustionAndRecovery(t *testing.T) {
	handler, upstream, repo, slots, _ := newCapacityRecoveryHandler(t, "all_failed", true)
	c, _ := newOpenAIResponsesFailoverTestContext(t, nil)
	handler.Responses(c)
	require.Equal(t, []int64{1, 2}, upstream.calls, "explicit rules bypass same-account retries")
	require.Equal(t, int32(2), atomic.LoadInt32(&slots.releaseAccountCalled))
	retry, _ := newOpenAIResponsesFailoverTestContext(t, nil)
	handler.Responses(retry)
	require.Equal(t, []int64{1, 2}, upstream.calls, "all-cooled pool must not revive candidates")
	for i := range repo.accounts {
		repo.accounts[i].Extra["model_rate_limits"] = map[string]any{
			"gpt-5.1": map[string]any{"rate_limit_reset_at": time.Now().Add(-time.Second).UTC().Format(time.RFC3339)},
		}
	}
	upstream.mode = "healthy"
	// Administrative suspension remains effective after capacity cooldown expiry.
	repo.accounts[0].Schedulable = false
	recovered, rec := newOpenAIResponsesFailoverTestContext(t, nil)
	handler.Responses(recovered)
	require.Equal(t, []int64{1, 2, 2}, upstream.calls)
	require.Contains(t, rec.Body.String(), "response_ok")
}

func TestOpenAICapacitySameAccountRetryHealthIsTerminal(t *testing.T) {
	handler, upstream, _, _, health := newCapacityRecoveryHandler(t, "pre_output", false)
	c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
	handler.Responses(c)
	require.Equal(t, []int64{1, 1, 1, 2}, upstream.calls)
	require.Equal(t, 1, health.calls, "two retries plus the original attempt count as one terminal account failure")
	require.Contains(t, rec.Body.String(), "response_ok")
}

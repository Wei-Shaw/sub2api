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
	"testing/synctest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const capacityBareFixture = `{"type":"error","error":{"code":"server_error","type":"service_unavailable_error","message":"Our servers are currently overloaded. Please try again later."}}`
const capacityFailedUsageFixture = `{"type":"response.failed","response":{"id":"resp_capacity","status":"failed","error":{"code":"server_error","type":"service_unavailable_error","message":"Our servers are currently overloaded. Please try again later."},"usage":{"input_tokens":11,"output_tokens":2,"total_tokens":13}}}`

type capacityRecoveryRepo struct {
	AccountRepository
	models []string
	until  time.Time
}

func (r *capacityRecoveryRepo) SetModelRateLimit(_ context.Context, _ int64, model string, until time.Time, _ ...string) error {
	r.models = append(r.models, model)
	r.until = until
	return nil
}

type capacityBlockingBody struct {
	*io.PipeReader
	closed chan struct{}
	once   sync.Once
}

func (b *capacityBlockingBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return b.PipeReader.Close()
}

func capacityRuleAccount() *Account {
	a := openAIHealthPoolAccount()
	a.Credentials["temp_unschedulable_enabled"] = true
	a.Credentials["temp_unschedulable_rules"] = []any{map[string]any{
		"error_code": float64(503), "keywords": []any{"overloaded"}, "duration_minutes": float64(2),
	}}
	return a
}

func TestOpenAICapacityBlockingStreamRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, interval := range []int{0, 180} {
			for _, mode := range []string{"silent", "heartbeat", "usage", "failed"} {
				t.Run(fmt.Sprintf("passthrough=%t/interval=%d/%s", passthrough, interval, mode), func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						reader, writer := io.Pipe()
						body := &capacityBlockingBody{PipeReader: reader, closed: make(chan struct{})}
						t.Cleanup(func() { _ = writer.Close(); _ = body.Close() })
						repo := &capacityRecoveryRepo{}
						cfg := &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: interval}}
						svc := &OpenAIGatewayService{cfg: cfg, rateLimitService: &RateLimitService{accountRepo: repo}}
						account := capacityRuleAccount()
						rec := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(rec)
						c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
						resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: body}
						done := make(chan struct{})
						var gotErr error
						var gotUsage *OpenAIUsage
						go func() {
							defer close(done)
							if passthrough {
								result, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "public", "upstream")
								gotErr, gotUsage = err, result.usage
							} else {
								result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "public", "upstream")
								gotErr, gotUsage = err, result.usage
							}
						}()
						_, err := io.WriteString(writer, openAIUpstreamKeepaliveFixture+"data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
						require.NoError(t, err)
						synctest.Wait()
						started := time.Now()
						payload := capacityBareFixture
						if mode == "failed" {
							payload = capacityFailedUsageFixture
						}
						_, err = io.WriteString(writer, "data: "+payload+"\n\n")
						require.NoError(t, err)
						synctest.Wait()
						require.Equal(t, []string{"upstream"}, repo.models, "health action must precede drain timeout")
						require.Equal(t, started.Add(2*time.Minute), repo.until)
						if mode != "failed" {
							select {
							case <-done:
								t.Fatal("bare error closed before usage grace period")
							default:
							}
							time.Sleep(time.Second)
							switch mode {
							case "usage":
								_, err = io.WriteString(writer, "data: "+capacityFailedUsageFixture+"\n\n")
								require.NoError(t, err)
							case "heartbeat":
								_, err = io.WriteString(writer, ": heartbeat\n\n")
								require.NoError(t, err)
							}
						}
						<-done
						var terminal *OpenAIStreamTerminalError
						require.ErrorAs(t, gotErr, &terminal)
						var replay *UpstreamFailoverError
						require.False(t, errors.As(gotErr, &replay))
						select {
						case <-body.closed:
						default:
							t.Fatal("upstream body still open after failure")
						}
						if mode == "usage" || mode == "failed" {
							require.Equal(t, 11, gotUsage.InputTokens)
							require.Equal(t, 2, gotUsage.OutputTokens)
							require.Less(t, time.Since(started), openAICapacityUsageDrainTimeout)
						} else {
							require.Equal(t, openAICapacityUsageDrainTimeout, time.Since(started))
						}
						require.Len(t, repo.models, 1, "error and response.failed must not apply the rule twice")
						require.Equal(t, 1, strings.Count(rec.Body.String(), `"partial"`))
						require.Equal(t, 1, strings.Count(rec.Body.String(), `"type":"service_unavailable_error"`))
					})
				})
			}
		}
	}
}

func TestOpenAICapacityPolicyAndHealth(t *testing.T) {
	payload := []byte(capacityBareFixture)
	t.Run("pool explicit rule skips retries without custom error codes", func(t *testing.T) {
		repo := &capacityRecoveryRepo{}
		svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
		account := capacityRuleAccount()
		err := svc.newOpenAIStreamFailoverErrorWithModel(nil, account, false, "request", payload, "overloaded", "upstream")
		require.False(t, err.RetryableOnSameAccount)
		require.True(t, err.AccountHealthHandled)
		require.Equal(t, []string{"upstream"}, repo.models)
	})
	t.Run("explicit ignore still wins", func(t *testing.T) {
		repo := &capacityRecoveryRepo{}
		svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
		account := capacityRuleAccount()
		account.Credentials["custom_error_codes_enabled"] = true
		account.Credentials["custom_error_codes"] = []any{float64(401)}
		require.False(t, svc.handleOpenAIAccountUpstreamError(context.Background(), account, 503, nil, payload, "upstream"))
		require.Empty(t, repo.models)
	})
	t.Run("terminal capacity is counted only once", func(t *testing.T) {
		settings := NewSettingService(&openAIAPIKeyHealthSettingRepo{value: `{"enabled":true,"window_minutes":10,"failure_threshold":3,"cooldown_minutes":2}`}, &config.Config{})
		cache := &openAIAPIKeyHealthCacheStub{}
		rateLimit := &RateLimitService{accountRepo: &capacityRecoveryRepo{}, settingService: settings, openAIAPIKeyHealth: cache}
		svc := &OpenAIGatewayService{rateLimitService: rateLimit}
		account := openAIHealthPoolAccount()
		err := svc.observeOpenAICapacityTerminalFailure(context.Background(), account, "upstream", nil, payload, "overloaded")
		require.Equal(t, 1, cache.recordCalls)
		svc.ObserveOpenAIAccountHealthFailure(context.Background(), account, err)
		require.Equal(t, 1, cache.recordCalls)
	})
}

func TestOpenAIWeightedStickyCapacityEscape(t *testing.T) {
	svc := &OpenAIGatewayService{}
	stats := newOpenAIAccountRuntimeStats()
	for range 5 {
		stats.report(42, false, nil)
	}
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: stats}
	plan := openAIAccountLoadPlan{topK: 1, candidates: []openAIAccountCandidateScore{
		{account: &Account{ID: 42}, score: 100}, {account: &Account{ID: 43}, score: 1},
	}}
	req := OpenAIAccountScheduleRequest{StickyWeighted: true, StickyAccountID: 42, SessionHash: "same-session"}
	order := scheduler.buildOpenAISelectionOrder(req, plan)
	require.Equal(t, int64(43), order[0].account.ID, "even Top-K=1 must escape an unhealthy sticky candidate")
	plan.candidates = plan.candidates[:1]
	order = scheduler.buildOpenAISelectionOrder(req, plan)
	require.Equal(t, int64(42), order[0].account.ID, "soft escape alone must not disable the only candidate")
}

func TestOpenAICapacityWSV2FirstFailureAndCommittedHealth(t *testing.T) {
	for _, payload := range []string{capacityBareFixture, capacityFailedUsageFixture} {
		for _, committed := range []bool{false, true} {
			t.Run(fmt.Sprintf("failed=%t/committed=%t", strings.Contains(payload, "response.failed"), committed), func(t *testing.T) {
				cfg := newOpenAIWSV2TestConfig()
				cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
				cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 2
				cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 2
				events := [][]byte{[]byte(`{"type":"response.created","response":{"id":"resp_capacity"}}`)}
				if committed {
					events = append(events, []byte(`{"type":"response.output_text.delta","delta":"partial"}`))
				}
				events = append(events, []byte(payload))
				pool := newOpenAIWSConnPool(cfg)
				pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: &openAIWSCaptureConn{events: events}})
				t.Cleanup(pool.Close)
				repo := &capacityRecoveryRepo{}
				svc := &OpenAIGatewayService{cfg: cfg, rateLimitService: &RateLimitService{accountRepo: repo},
					cache: &stubGatewayCache{}, httpUpstream: &httpUpstreamRecorder{},
					openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool}
				account := capacityRuleAccount()
				account.Credentials["api_key"] = "test-only"
				account.Extra = map[string]any{"responses_websockets_v2_enabled": true}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
				result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.1","stream":true,"input":"hello"}`))
				require.Equal(t, []string{"gpt-5.1"}, repo.models)
				var failover *UpstreamFailoverError
				if !committed {
					require.ErrorAs(t, err, &failover)
					require.False(t, failover.RetryableOnSameAccount)
					require.Empty(t, rec.Body.String())
				} else {
					require.False(t, errors.As(err, &failover))
					require.NotNil(t, result)
					require.Equal(t, "response.failed", result.UpstreamTerminalEvent)
					require.Contains(t, rec.Body.String(), "partial")
				}
			})
		}
	}
}

func TestOpenAICapacityWSHTTPBridgeDoesNotUndoConfirmedHealth(t *testing.T) {
	for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
		t.Run(accountType, func(t *testing.T) {
			body := "data: " + capacityBareFixture + "\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_late\",\"status\":\"completed\",\"usage\":{\"input_tokens\":7,\"output_tokens\":1}}}\n\n"
			repo := &capacityRecoveryRepo{}
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, rateLimitService: &RateLimitService{accountRepo: repo}}
			account := capacityRuleAccount()
			account.Type = accountType
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("GET", "/v1/responses", nil)
			payload := []byte(`{"type":"response.create","model":"gpt-5.1","input":"hello"}`)
			var writes []string
			result, err := svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), c, account, "test-only", payload, len(payload), "gpt-5.1", "", "", "", "", 2, func(message []byte) error { writes = append(writes, string(message)); return nil })
			require.NotNil(t, result)
			if accountType == AccountTypeAPIKey {
				require.Error(t, err)
				require.Equal(t, "response.failed", result.UpstreamTerminalEvent)
				require.Equal(t, []string{"gpt-5.1"}, repo.models)
			} else {
				require.NoError(t, err)
				require.Equal(t, "response.completed", result.UpstreamTerminalEvent)
				require.Empty(t, repo.models)
			}
			require.Len(t, writes, 1)
		})
	}
}

func TestOpenAICapacityWSBindingRechecksPause(t *testing.T) {
	account := capacityRuleAccount()
	account.Status, account.Schedulable = StatusActive, true
	repo := &stubOpenAIAccountRepo{accounts: []Account{*account}}
	svc := &OpenAIGatewayService{accountRepo: repo, cfg: &config.Config{RunMode: config.RunModeSimple}}
	require.True(t, svc.OpenAIWSAPIKeyAccountAvailable(context.Background(), account, nil, "gpt-5.1"))
	repo.accounts[0].Extra = map[string]any{"model_rate_limits": map[string]any{"gpt-5.1": map[string]any{"rate_limit_reset_at": time.Now().Add(time.Minute).UTC().Format(time.RFC3339)}}}
	require.False(t, svc.OpenAIWSAPIKeyAccountAvailable(context.Background(), account, nil, "gpt-5.1"))
	require.True(t, svc.OpenAIWSAPIKeyAccountAvailable(context.Background(), account, nil, "gpt-5.2"))
	repo.accounts[0].Schedulable = false
	require.False(t, svc.OpenAIWSAPIKeyAccountAvailable(context.Background(), account, nil, "gpt-5.2"))
}

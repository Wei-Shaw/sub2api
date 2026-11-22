package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type codexQuotaHandlerRepo struct {
	service.AccountRepository
	mu         sync.Mutex
	candidates []service.Account
}

func (r *codexQuotaHandlerRepo) ListModelAvailabilityCandidates(context.Context, *int64, []string, bool) ([]service.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]service.Account(nil), r.candidates...), nil
}

func (r *codexQuotaHandlerRepo) ListSchedulableByPlatform(context.Context, string) ([]service.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var accounts []service.Account
	for _, account := range r.candidates {
		if account.IsSchedulable() {
			accounts = append(accounts, account)
		}
	}
	return accounts, nil
}

func (r *codexQuotaHandlerRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, account := range r.candidates {
		if account.ID == id {
			return &account, nil
		}
	}
	return nil, nil
}

func (r *codexQuotaHandlerRepo) SetCodexQuotaRateLimited(_ context.Context, id int64, cooldown, reset time.Time, explicit bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.candidates {
		account := &r.candidates[i]
		if account.ID == id {
			now := time.Now()
			account.RateLimitedAt, account.RateLimitResetAt = &now, &cooldown
			account.Extra[service.OpenAICodexQuotaCooldownExtraKey] = map[string]any{
				"limited_at": now.Format(time.RFC3339Nano), "cooldown_until": cooldown.Format(time.RFC3339Nano),
				"reset_at": reset.Format(time.RFC3339Nano), "explicit": explicit,
			}
		}
	}
	return nil
}

type codexQuotaHTTPUpstream struct {
	service.HTTPUpstream
	reset            time.Time
	calls            atomic.Int32
	successfulFirst  atomic.Bool
	streamingFailure bool
}

func (u *codexQuotaHTTPUpstream) Do(_ *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	if u.calls.Add(1) == 1 && u.successfulFirst.Load() {
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(`data: {"type":"response.completed","response":{"id":"resp_quota_test","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n")),
		}, nil
	}
	reset := u.reset.Add(time.Duration(id-1) * time.Hour)
	if u.streamingFailure {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":                          {"text/event-stream"},
				"X-Codex-Primary-Used-Percent":          {"100"},
				"X-Codex-Secondary-Used-Percent":        {"92"},
				"X-Codex-Primary-Reset-After-Seconds":   {"3600"},
				"X-Codex-Secondary-Reset-After-Seconds": {"7200"},
				"X-Codex-Turn-State":                    {"upstream-turn-state"},
				"X-Ratelimit-Remaining-Requests":        {"0"},
				"X-Ratelimit-Reset-Requests":            {"60s"},
				"Retry-After":                           {"60"},
				"X-Request-Id":                          {"upstream-request"},
			},
			Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`data: {"type":"error","status":429,"error":{"type":"usage_limit_reached","message":"The usage limit has been reached","resets_at":%d}}`, reset.Unix()) + "\n\n")),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusTooManyRequests, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"error":{"type":"usage_limit_reached","message":"The usage limit has been reached","resets_at":%d}}`, reset.Unix()))),
	}, nil
}

func TestCodexPoolQuotaResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC().Truncate(time.Second)
	reset := now.Add(time.Hour)
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			cfg := &config.Config{RunMode: config.RunModeSimple}
			cfg.Default.RateMultiplier = 1
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.OAuthEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
			cfg.Gateway.OpenAIWS.IngressModeDefault = service.OpenAIWSIngressModePassthrough
			repo := &codexQuotaHandlerRepo{candidates: []service.Account{{
				ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true,
				Extra: map[string]any{
					"openai_oauth_responses_websockets_v2_mode": service.OpenAIWSIngressModePassthrough,
					"auto_pause_5h_threshold":                   1.0,
					"codex_usage_updated_at":                    now.Format(time.RFC3339),
					"codex_5h_used_percent":                     100.0,
					"codex_5h_reset_at":                         reset.Format(time.RFC3339),
				},
			}}}
			billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billing.Stop)
			upstream := &codexQuotaHTTPUpstream{reset: reset, streamingFailure: true}
			rateLimit := service.NewRateLimitService(repo, nil, cfg, nil, nil)
			gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
				service.NewBillingService(cfg, nil), rateLimit, billing, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
			cache := &concurrencyCacheMock{
				acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
				acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
			}
			h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(cache), billing,
				service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
			groupID := int64(42)
			key := &service.APIKey{ID: 7, GroupID: &groupID, User: &service.User{ID: 8, Status: service.StatusActive},
				Group: &service.Group{ID: groupID, Status: service.StatusActive, Platform: service.PlatformOpenAI, AllowImageGeneration: true, CodexPoolQuotaErrorEnabled: enabled}}
			newRouter := func(h *OpenAIGatewayHandler) *gin.Engine {
				router := gin.New()
				router.Use(func(c *gin.Context) {
					c.Set(string(middleware.ContextKeyAPIKey), key)
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 8, Concurrency: 1})
				})
				router.POST("/v1/responses", h.Responses)
				router.GET("/v1/responses", h.ResponsesWebSocket)
				return router
			}
			router := newRouter(h)
			srv := httptest.NewServer(router)
			defer srv.Close()

			for range 2 {
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.1","input":"hello","stream":true}`))
				req.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(rec, req)
				if enabled {
					require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
					require.Equal(t, "usage_limit_reached", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
					require.Equal(t, reset.Unix(), gjson.GetBytes(rec.Body.Bytes(), "error.resets_at").Int())
					require.Empty(t, rec.Header().Get("Retry-After"))
				} else {
					require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
					require.False(t, gjson.GetBytes(rec.Body.Bytes(), "error.resets_at").Exists())
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/responses", nil)
			require.NoError(t, err)
			defer func() { _ = conn.CloseNow() }()
			require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":[{"role":"user","content":"hello"}]}`)))
			_, payload, err := conn.Read(ctx)
			if enabled {
				require.NoError(t, err)
				require.Equal(t, "error", gjson.GetBytes(payload, "type").String())
				require.Equal(t, int64(429), gjson.GetBytes(payload, "status").Int())
				require.Equal(t, "usage_limit_reached", gjson.GetBytes(payload, "error.type").String())
				require.Equal(t, reset.Unix(), gjson.GetBytes(payload, "error.resets_at").Int())
				_, _, err = conn.Read(ctx)
				require.Equal(t, coderws.StatusNormalClosure, coderws.CloseStatus(err))
			} else {
				require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(err))
			}

			if binary := os.Getenv("CODEX_POOL_QUOTA_TEST_CLI"); enabled && binary != "" {
				for _, transport := range []string{"false", "true"} {
					t.Run("codex_ws_"+transport, func(t *testing.T) {
						dir := t.TempDir()
						ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
						defer cancel()
						cmd := exec.CommandContext(ctx, binary, "exec", "--json", "--skip-git-repo-check", "--ephemeral", // #nosec G702 -- The test operator selects the local CLI executable.
							"-c", `model_provider="quota_test"`, "-c", `model="gpt-6.1-sol"`,
							"-c", `model_providers.quota_test.name="Quota test"`,
							"-c", `model_providers.quota_test.env_key="POOL_TEST_KEY"`,
							"-c", `model_providers.quota_test.wire_api="responses"`,
							"-c", `model_providers.quota_test.base_url="`+srv.URL+`/v1"`,
							"-c", "model_providers.quota_test.supports_websockets="+transport, "Reply with OK.")
						cmd.Dir = dir
						cmd.Env = []string{"HOME=" + dir, "CODEX_HOME=" + dir, "POOL_TEST_KEY=local-test", "TZ=UTC", "PATH=" + os.Getenv("PATH")}
						output, err := cmd.CombinedOutput()
						require.IsType(t, &exec.ExitError{}, err, string(output))
						require.Contains(t, string(output), "Try again at "+reset.Format("3:04 PM"))
					})
				}
			}

			if enabled {
				t.Run("explicit_image_size", func(t *testing.T) {
					repo.mu.Lock()
					limited := repo.candidates[0]
					limited.Extra[service.AccountExtraOpenAIImageSizeTiers] = []string{"4K"}
					repo.candidates = []service.Account{
						limited,
						{ID: 2, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true,
							Extra: map[string]any{service.AccountExtraOpenAIImageSizeTiers: []string{"1K"}}},
					}
					repo.mu.Unlock()
					req := httptest.NewRequest(http.MethodPost, "/v1/responses",
						strings.NewReader(`{"model":"gpt-5.1","input":"draw a cat","tools":[{"type":"image_generation","size":"3840x2160"}]}`))
					req.Header.Set("Content-Type", "application/json")
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
					require.Equal(t, reset.Unix(), gjson.GetBytes(rec.Body.Bytes(), "error.resets_at").Int())
					require.Zero(t, upstream.calls.Load())
				})
			}

			if enabled {
				repo.mu.Lock()
				repo.candidates = []service.Account{
					{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1,
						Credentials: map[string]any{"access_token": "mock-token"}, Extra: map[string]any{"openai_passthrough": true}},
					{ID: 2, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1,
						Credentials: map[string]any{"access_token": "mock-token"}, Extra: map[string]any{"openai_passthrough": true}},
				}
				repo.mu.Unlock()
				for attempt := range 2 {
					req := httptest.NewRequest(http.MethodPost, "/v1/responses",
						strings.NewReader(`{"model":"gpt-5.1","instructions":"Be helpful.","input":[{"role":"user","content":"hello"}],"stream":true}`))
					req.Header.Set("Content-Type", "application/json")
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
					require.Equal(t, reset.Unix(), gjson.GetBytes(rec.Body.Bytes(), "error.resets_at").Int(), rec.Body.String())
					require.Equal(t, int32(2), upstream.calls.Load())
					headers := rec.Result().Header
					require.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))
					require.Equal(t, "no-store", headers.Get("Cache-Control"))
					if attempt == 0 {
						require.Equal(t, "upstream-request", headers.Get("X-Request-Id"), "the SSE attempt staged upstream headers")
					}
					for _, name := range []string{
						"X-Codex-Primary-Used-Percent", "X-Codex-Secondary-Used-Percent",
						"X-Codex-Primary-Reset-After-Seconds", "X-Codex-Secondary-Reset-After-Seconds",
						"X-Codex-Turn-State", "X-Ratelimit-Remaining-Requests", "X-Ratelimit-Reset-Requests",
						"Retry-After", "X-Accel-Buffering",
					} {
						require.Empty(t, headers.Values(name), "%s leaked into pool quota response", name)
					}
				}
			}

			if enabled {
				for _, tc := range []struct {
					name  string
					model string
				}{
					{name: "same_model", model: "gpt-5.1"},
					{name: "changed_model", model: "gpt-5.2"},
				} {
					t.Run("http_bridge_"+tc.name, func(t *testing.T) {
						repo.mu.Lock()
						repo.candidates = []service.Account{{
							ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1,
							Credentials: map[string]any{"access_token": "mock-token"},
							Extra:       map[string]any{"openai_oauth_responses_websockets_v2_mode": service.OpenAIWSIngressModeHTTPBridge},
						}}
						repo.mu.Unlock()
						// Use a fresh gateway so runtime cooldowns from other cases do not carry over.
						upstream := &codexQuotaHTTPUpstream{reset: reset}
						upstream.successfulFirst.Store(true)
						bridgeHandler := *h
						gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
							service.NewBillingService(cfg, nil), rateLimit, billing, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
						bridgeHandler.gatewayService = gateway
						router := newRouter(&bridgeHandler)
						bridgeServer := httptest.NewServer(router)
						defer bridgeServer.Close()
						turnCtx, cancelTurns := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancelTurns()
						bridge, _, err := coderws.Dial(turnCtx, "ws"+strings.TrimPrefix(bridgeServer.URL, "http")+"/v1/responses", nil)
						require.NoError(t, err)
						defer func() { _ = bridge.CloseNow() }()
						request := `{"type":"response.create","model":"%s","instructions":"Be helpful.","input":[{"type":"message","role":"user","content":"hello"}]}`
						require.NoError(t, bridge.Write(turnCtx, coderws.MessageText, []byte(fmt.Sprintf(request, "gpt-5.1"))))
						_, frame, err := bridge.Read(turnCtx)
						require.NoError(t, err)
						require.Equal(t, "response.completed", gjson.GetBytes(frame, "type").String(), string(frame))
						require.NoError(t, bridge.Write(turnCtx, coderws.MessageText, []byte(fmt.Sprintf(request, tc.model))))
						_, frame, err = bridge.Read(turnCtx)
						require.Equal(t, int32(2), upstream.calls.Load())
						if tc.model != "gpt-5.1" {
							require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(err), string(frame))
							return
						}
						require.NoError(t, err)
						require.Equal(t, int64(429), gjson.GetBytes(frame, "status").Int(), string(frame))
						require.Equal(t, reset.Unix(), gjson.GetBytes(frame, "error.resets_at").Int())
						_, _, err = bridge.Read(turnCtx)
						require.Equal(t, coderws.StatusNormalClosure, coderws.CloseStatus(err))

						const responseID = "resp_quota_test"
						require.Equal(t, reset, gateway.EarliestCodexPoolQuotaReset(turnCtx, service.CodexPoolQuotaRequest{
							GroupID: &groupID, Model: tc.model, PreviousResponseID: responseID, Transport: service.OpenAIUpstreamTransportAny,
						}).UTC(), "the OAuth account is bound to this response and has a known quota reset")
						require.NoError(t, gateway.BindOpenAIHTTPResponseOwner(turnCtx, groupID, responseID, key.User.ID, key.ID))
						rec := httptest.NewRecorder()
						req := httptest.NewRequest(http.MethodPost, "/v1/responses",
							strings.NewReader(`{"model":"gpt-5.1","previous_response_id":"resp_quota_test","input":"continue","stream":true}`))
						req.Header.Set("Content-Type", "application/json")
						router.ServeHTTP(rec, req)
						require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
						require.False(t, gjson.GetBytes(rec.Body.Bytes(), "error.resets_at").Exists())
						require.Equal(t, int32(2), upstream.calls.Load())
					})
				}
			}

		})
	}
}

type codexQuotaLiveKeyRepo struct {
	service.APIKeyRepository
	key     *service.APIKey
	enabled atomic.Bool
}

func (r *codexQuotaLiveKeyRepo) GetByKeyForAuth(context.Context, string) (*service.APIKey, error) {
	key := *r.key
	group := *key.Group
	group.CodexPoolQuotaErrorEnabled = r.enabled.Load()
	key.Group = &group
	return &key, nil
}

func (r *codexQuotaHandlerRepo) SetRateLimited(_ context.Context, id int64, until time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.candidates {
		if r.candidates[i].ID == id {
			r.candidates[i].RateLimitResetAt = &until
		}
	}
	return nil
}

func TestCodexPoolQuotaNativeWebSocketRefreshesSettingsAfterMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{service.OpenAIWSIngressModeCtxPool, service.OpenAIWSIngressModePassthrough} {
		for _, scenario := range []struct {
			name                                   string
			initiallyEnabled, laterEnabled, output bool
			healthyBackup, toolContinuation        bool
		}{
			{name: "enable_on_existing_connection", laterEnabled: true},
			{name: "disable_on_existing_connection", initiallyEnabled: true},
			{name: "preserve_error_after_semantic_output", initiallyEnabled: true, laterEnabled: true, output: true},
			{name: "reconnect_when_backup_is_healthy", laterEnabled: true, healthyBackup: true},
			{name: "reconnect_when_tool_turn_depends_on_upstream_state", laterEnabled: true, toolContinuation: true},
		} {
			t.Run(mode+"/"+scenario.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				defer cancel()
				now := time.Now().UTC().Truncate(time.Second)
				reset := now.Add(2 * time.Hour)
				upstreamReset := reset.Add(time.Hour)
				var upstreamRequests atomic.Int32
				var inferredContinuation atomic.Bool
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
					if err != nil {
						return
					}
					defer func() { _ = conn.CloseNow() }()
					for _, frames := range [][]string{
						{`{"type":"response.completed","response":{"id":"resp_first","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`},
						{`{"type":"codex.rate_limits"}`, `{"type":"response.created","response":{"id":"resp_second","model":"gpt-5.1"}}`},
					} {
						_, request, err := conn.Read(ctx)
						if err != nil {
							return
						}
						if upstreamRequests.Add(1) == 2 {
							inferredContinuation.Store(gjson.GetBytes(request, "previous_response_id").String() == "resp_first")
						}
						for _, frame := range frames {
							if err := conn.Write(ctx, coderws.MessageText, []byte(frame)); err != nil {
								return
							}
						}
					}
					if scenario.output {
						if err := conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.output_text.delta","response_id":"resp_second","delta":"hello"}`)); err != nil {
							return
						}
					}
					_ = conn.Write(ctx, coderws.MessageText, []byte(fmt.Sprintf(`{"type":"error","status":429,"error":{"type":"usage_limit_reached","message":"The usage limit has been reached","resets_at":%d}}`, upstreamReset.Unix())))
					_, _, _ = conn.Read(ctx)
				}))
				defer upstream.Close()
				repo := &codexQuotaHandlerRepo{candidates: []service.Account{
					{
						ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
						Status: service.StatusActive, Schedulable: true, Concurrency: 1,
						Credentials: map[string]any{"api_key": "mock-token", "base_url": upstream.URL},
						Extra:       map[string]any{"openai_apikey_responses_websockets_v2_mode": mode},
					},
					{
						ID: 2, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
						Status: service.StatusActive, Schedulable: true, Concurrency: 1, RateLimitedAt: &now, RateLimitResetAt: &reset,
						Extra: map[string]any{
							"openai_oauth_responses_websockets_v2_mode": mode,
							service.OpenAICodexQuotaCooldownExtraKey: map[string]any{
								"limited_at": now.Format(time.RFC3339Nano), "cooldown_until": reset.Format(time.RFC3339Nano),
								"reset_at": reset.Format(time.RFC3339Nano), "explicit": true,
							},
						},
					},
				}}
				if scenario.healthyBackup {
					repo.candidates[1].RateLimitedAt = nil
					repo.candidates[1].RateLimitResetAt = nil
					repo.candidates[1].Priority = 10
				}
				cfg := &config.Config{RunMode: config.RunModeSimple}
				cfg.Security.URLAllowlist.AllowInsecureHTTP = true
				cfg.Gateway.OpenAIWS.Enabled = true
				cfg.Gateway.OpenAIWS.OAuthEnabled = true
				cfg.Gateway.OpenAIWS.APIKeyEnabled = true
				cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
				cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
				cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
				cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
				cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
				billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
				t.Cleanup(billing.Stop)
				rateLimit := service.NewRateLimitService(repo, nil, cfg, nil, nil)
				gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
					service.NewBillingService(cfg, nil), rateLimit, billing, nil, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
				groupID := int64(42)
				key := &service.APIKey{ID: 7, Key: "test-gateway-key", GroupID: &groupID, User: &service.User{ID: 8, Status: service.StatusActive},
					Group: &service.Group{ID: groupID, Status: service.StatusActive, Platform: service.PlatformOpenAI, CodexPoolQuotaErrorEnabled: scenario.initiallyEnabled}}
				keys := &codexQuotaLiveKeyRepo{key: key}
				keys.enabled.Store(scenario.initiallyEnabled)
				cache := &concurrencyCacheMock{
					acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
					acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
				}
				h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(cache), billing,
					service.NewAPIKeyService(keys, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
				router := gin.New()
				router.GET("/v1/responses", func(c *gin.Context) {
					c.Set(string(middleware.ContextKeyAPIKey), key)
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 8, Concurrency: 1})
					h.ResponsesWebSocket(c)
				})
				srv := httptest.NewServer(router)
				defer srv.Close()
				conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/responses", nil)
				require.NoError(t, err)
				defer func() { _ = conn.CloseNow() }()
				request := []byte(`{"type":"response.create","model":"gpt-5.1","store":false,"input":[{"type":"message","role":"user","content":"hello"}]}`)
				require.NoError(t, conn.Write(ctx, coderws.MessageText, request))
				_, frame, err := conn.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, "response.completed", gjson.GetBytes(frame, "type").String())
				keys.enabled.Store(scenario.laterEnabled)
				if scenario.toolContinuation {
					request = []byte(`{"type":"response.create","model":"gpt-5.1","store":false,"input":[{"type":"function_call_output","call_id":"call_tool","output":"ok"}]}`)
				}
				require.NoError(t, conn.Write(ctx, coderws.MessageText, request))
				events := []string{"codex.rate_limits", "response.created"}
				if scenario.output {
					events = append(events, "response.output_text.delta")
				}
				expectReconnect := scenario.healthyBackup || scenario.toolContinuation
				if !expectReconnect {
					events = append(events, "error")
				}
				for _, event := range events {
					_, frame, err = conn.Read(ctx)
					require.NoError(t, err)
					require.Equal(t, event, gjson.GetBytes(frame, "type").String())
				}
				if expectReconnect {
					_, frame, err = conn.Read(ctx)
					require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(err), string(frame))
					if scenario.toolContinuation && mode == service.OpenAIWSIngressModeCtxPool {
						require.True(t, inferredContinuation.Load(), "ctx_pool must test the inferred continuation")
					}
				} else if scenario.laterEnabled && !scenario.output {
					require.Equal(t, "usage_limit_reached", gjson.GetBytes(frame, "error.type").String(), string(frame))
					require.Equal(t, reset.Unix(), gjson.GetBytes(frame, "error.resets_at").Int())
					_, _, err := conn.Read(ctx)
					require.Equal(t, coderws.StatusNormalClosure, coderws.CloseStatus(err))
				} else {
					require.Equal(t, upstreamReset.Unix(), gjson.GetBytes(frame, "error.resets_at").Int(), string(frame))
				}
				require.Equal(t, int32(2), upstreamRequests.Load(), "metadata must not permit replay")
			})
		}
	}
}

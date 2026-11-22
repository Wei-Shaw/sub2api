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
	reset           time.Time
	calls           atomic.Int32
	successfulFirst atomic.Bool
}

func (u *codexQuotaHTTPUpstream) Do(_ *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	if u.calls.Add(1) == 1 && u.successfulFirst.Load() {
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(`data: {"type":"response.completed","response":{"id":"resp_quota_test","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n")),
		}, nil
	}
	reset := u.reset.Add(time.Duration(id-1) * time.Hour)
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
				RateLimitedAt: &now, RateLimitResetAt: &reset, Extra: map[string]any{
					"openai_oauth_responses_websockets_v2_mode": service.OpenAIWSIngressModePassthrough,
					service.OpenAICodexQuotaCooldownExtraKey: map[string]any{
						"limited_at": now.Format(time.RFC3339Nano), "cooldown_until": reset.Format(time.RFC3339Nano), "reset_at": reset.Format(time.RFC3339Nano), "explicit": true,
					},
				},
			}}}
			billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billing.Stop)
			upstream := &codexQuotaHTTPUpstream{reset: reset}
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
				Group: &service.Group{ID: groupID, Status: service.StatusActive, Platform: service.PlatformOpenAI, CodexPoolQuotaErrorEnabled: enabled}}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyAPIKey), key)
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 8, Concurrency: 1})
			})
			router.POST("/v1/responses", h.Responses)
			router.GET("/v1/responses", h.ResponsesWebSocket)
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
			defer conn.CloseNow()
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
						cmd := exec.CommandContext(ctx, binary, "exec", "--json", "--skip-git-repo-check", "--ephemeral",
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
				repo.mu.Lock()
				repo.candidates = []service.Account{
					{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1,
						Credentials: map[string]any{"access_token": "mock-token"}, Extra: map[string]any{"openai_passthrough": true}},
					{ID: 2, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1,
						Credentials: map[string]any{"access_token": "mock-token"}, Extra: map[string]any{"openai_passthrough": true}},
				}
				repo.mu.Unlock()
				for range 2 {
					req := httptest.NewRequest(http.MethodPost, "/v1/responses",
						strings.NewReader(`{"model":"gpt-5.1","instructions":"Be helpful.","input":[{"role":"user","content":"hello"}],"stream":true}`))
					req.Header.Set("Content-Type", "application/json")
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
					require.Equal(t, reset.Unix(), gjson.GetBytes(rec.Body.Bytes(), "error.resets_at").Int(), rec.Body.String())
					require.Equal(t, int32(2), upstream.calls.Load())
				}
			}

			if enabled {
				repo.mu.Lock()
				repo.candidates = []service.Account{{
					ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1,
					Credentials: map[string]any{"access_token": "mock-token"},
					Extra:       map[string]any{"openai_oauth_responses_websockets_v2_mode": service.OpenAIWSIngressModeHTTPBridge},
				}}
				repo.mu.Unlock()
				// A new gateway instance drops runtime-only cooldowns while using
				// the same HTTP bridge and account repository.
				upstream.calls.Store(0)
				upstream.successfulFirst.Store(true)
				h.gatewayService = service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
					service.NewBillingService(cfg, nil), rateLimit, billing, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
				turnCtx, cancelTurns := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancelTurns()
				bridge, _, err := coderws.Dial(turnCtx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/responses", nil)
				require.NoError(t, err)
				defer bridge.CloseNow()
				request := []byte(`{"type":"response.create","model":"gpt-5.1","instructions":"Be helpful.","input":[{"type":"message","role":"user","content":"hello"}]}`)
				require.NoError(t, bridge.Write(turnCtx, coderws.MessageText, request))
				_, frame, err := bridge.Read(turnCtx)
				require.NoError(t, err)
				require.Equal(t, "response.completed", gjson.GetBytes(frame, "type").String(), string(frame))
				require.NoError(t, bridge.Write(turnCtx, coderws.MessageText, request))
				_, frame, err = bridge.Read(turnCtx)
				require.NoError(t, err)
				require.Equal(t, int64(429), gjson.GetBytes(frame, "status").Int(), string(frame))
				require.Equal(t, reset.Unix(), gjson.GetBytes(frame, "error.resets_at").Int())
				require.Equal(t, int32(2), upstream.calls.Load())
			}

		})
	}
}

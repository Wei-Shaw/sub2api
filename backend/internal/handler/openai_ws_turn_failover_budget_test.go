package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func TestOpenAIWSTurnFailoverBudget(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModeCtxPool, service.OpenAIWSIngressModeHTTPBridge} {
		t.Run(mode, func(t *testing.T) {
			for _, tc := range []struct {
				name          string
				responses     []string
				clientEvents  []string
				wantAccounts  []int64
				keepCooldown  bool
				wantLastClose bool
			}{
				{
					name:         "successful_turn_restores_budget_and_previous_candidates",
					responses:    []string{"429", "response.completed", "429", "response.completed"},
					clientEvents: []string{"response.completed", "response.completed"},
					wantAccounts: []int64{801, 802, 802, 801},
				},
				{
					name:         "successful_turn_preserves_account_cooldown",
					responses:    []string{"429", "response.completed", "429", "response.completed"},
					clientEvents: []string{"response.completed", "response.completed"},
					wantAccounts: []int64{801, 802, 802, 803},
					keepCooldown: true,
				},
				{
					name:          "consecutive_failures_exhaust_current_turn",
					responses:     []string{"429", "429"},
					clientEvents:  []string{""},
					wantAccounts:  []int64{801, 802},
					wantLastClose: true,
				},
				{
					name:          "incomplete_terminal_does_not_restore_budget",
					responses:     []string{"429", "response.incomplete", "429"},
					clientEvents:  []string{"response.incomplete", ""},
					wantAccounts:  []int64{801, 802, 802},
					wantLastClose: true,
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					conn, repo, attempts := newOpenAIWSTurnBudgetSession(t, mode, tc.responses, nil)
					for turn, eventType := range tc.clientEvents {
						if turn == 1 && tc.keepCooldown {
							require.NoError(t, repo.SetTempUnschedulable(context.Background(), 801, time.Now().Add(time.Hour), "test cooldown"))
						}
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						payload := fmt.Sprintf(`{"type":"response.create","model":"gpt-5.1","instructions":"turn-%d","input":[{"role":"user","content":"hello"}]}`, turn+1)
						require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(payload)))
						_, event, err := conn.Read(ctx)
						cancel()
						if tc.wantLastClose && turn == len(tc.clientEvents)-1 {
							var closeErr coderws.CloseError
							require.ErrorAs(t, err, &closeErr)
						} else {
							require.NoError(t, err, "turn %d should recover on the same client connection", turn+1)
							require.Equal(t, eventType, gjson.GetBytes(event, "type").String())
						}
					}
					gotAccounts, gotTurns, _ := attempts()
					require.Equal(t, tc.wantAccounts, gotAccounts)
					if !tc.wantLastClose {
						require.Equal(t, []string{"turn-1", "turn-1", "turn-2", "turn-2"}, gotTurns, "failover must replay the current turn")
					}
					if tc.keepCooldown {
						account, err := repo.GetByID(context.Background(), 801)
						require.NoError(t, err)
						require.NotNil(t, account.TempUnschedulableUntil)
						require.True(t, account.TempUnschedulableUntil.After(time.Now()))
					}
				})
			}
		})
	}
}

type openAIWSTurnBudgetHTTPClient struct {
	service.HTTPUpstream
}

func (openAIWSTurnBudgetHTTPClient) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

type openAIWSTurnBudgetAccountRepo struct {
	service.AccountRepository
	mu       sync.Mutex
	accounts []service.Account
}

func (r *openAIWSTurnBudgetAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var accounts []service.Account
	for _, account := range r.accounts {
		if account.Platform == platform && account.IsSchedulable() {
			accounts = append(accounts, account)
		}
	}
	return accounts, nil
}

func (r *openAIWSTurnBudgetAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, _ int64, platform string) ([]service.Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}

func (r *openAIWSTurnBudgetAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}

func (r *openAIWSTurnBudgetAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, account := range r.accounts {
		if account.ID == id {
			return &account, nil
		}
	}
	return nil, nil
}

func (r *openAIWSTurnBudgetAccountRepo) SetTempUnschedulable(_ context.Context, id int64, until time.Time, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			r.accounts[i].TempUnschedulableUntil = &until
		}
	}
	return nil
}

func newOpenAIWSTurnBudgetSession(t *testing.T, mode string, responses []string, channelMapping map[string]string, configure ...func(*config.Config)) (*coderws.Conn, *openAIWSTurnBudgetAccountRepo, func() ([]int64, []string, []string)) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var mu sync.Mutex
	var accountIDs []int64
	var turns []string
	var models []string
	responseFor := func(accountID int64, payload []byte) (string, []byte) {
		mu.Lock()
		defer mu.Unlock()
		accountIDs = append(accountIDs, accountID)
		turns = append(turns, gjson.GetBytes(payload, "instructions").String())
		model := gjson.GetBytes(payload, "model").String()
		models = append(models, model)
		attempt := len(accountIDs)
		if attempt > len(responses) {
			t.Errorf("unexpected upstream attempt %d on account %d", attempt, accountID)
			return "429", []byte(`{"type":"error","error":{"code":"rate_limit_exceeded","type":"rate_limit_error","message":"rate limit exceeded"}}`)
		}
		eventType := responses[attempt-1]
		if eventType == "429" {
			return eventType, []byte(`{"type":"error","error":{"code":"rate_limit_exceeded","type":"rate_limit_error","message":"rate limit exceeded"}}`)
		}
		return eventType, []byte(fmt.Sprintf(`{"type":%q,"response":{"id":"resp_budget_%d","model":%q,"output":[],"usage":{"input_tokens":2,"output_tokens":1}}}`, eventType, attempt, model))
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var accountID int64
		if _, err := fmt.Sscanf(r.Header.Get("Authorization"), "Bearer sk-%d", &accountID); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if !isOpenAIWSUpgradeRequest(r) {
			payload, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			eventType, response := responseFor(accountID, payload)
			if eventType == "429" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write(response)
			} else {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(w, "data: %s\n\n", response)
			}
			return
		}
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		for {
			_, payload, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			eventType, response := responseFor(accountID, payload)
			if eventType == "timeout" {
				continue
			}
			if err := conn.Write(r.Context(), coderws.MessageText, response); err != nil {
				return
			}
		}
	}))
	t.Cleanup(upstream.Close)
	repo := &openAIWSTurnBudgetAccountRepo{}
	for i := 0; i < 3; i++ {
		id := int64(801 + i)
		repo.accounts = append(repo.accounts, service.Account{
			ID: id, Name: fmt.Sprintf("ws-budget-%d", id), Platform: service.PlatformOpenAI,
			Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: i + 1,
			Credentials: map[string]any{"api_key": fmt.Sprintf("sk-%d", id), "base_url": upstream.URL},
			Extra: map[string]any{
				"openai_apikey_responses_websockets_v2_enabled": true,
				"openai_apikey_responses_websockets_v2_mode":    mode,
			},
		})
	}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.MaxAccountSwitches = 1
	for _, apply := range configure {
		apply(cfg)
	}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	usage := &openAIWSUsageHandlerUsageLogRepoStub{}
	groupID := int64(4202)
	var channelSvc *service.ChannelService
	if len(channelMapping) > 0 {
		channelSvc = service.NewChannelService(&openAIWSUsageHandlerChannelRepoStub{
			channels: []service.Channel{{
				ID: 7702, Name: "ws-failover-model", Status: service.StatusActive,
				GroupIDs:     []int64{groupID},
				ModelMapping: map[string]map[string]string{service.PlatformOpenAI: channelMapping},
			}},
			groupPlatforms: map[int64]string{groupID: service.PlatformOpenAI},
		}, nil, nil, nil, nil)
	}
	gateway := service.NewOpenAIGatewayService(repo, usage, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billing, openAIWSTurnBudgetHTTPClient{},
		&service.DeferredService{}, nil, nil, nil, channelSvc, nil, nil, nil)
	t.Cleanup(gateway.CloseOpenAIWSPool)
	cache := &concurrencyCacheMock{
		acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
	}
	h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(cache), billing, &service.APIKeyService{}, nil, nil, nil, nil, cfg)
	apiKey := &service.APIKey{
		ID: 1802, GroupID: &groupID, User: &service.User{ID: 1702, Status: service.StatusActive},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.User.ID, Concurrency: 1})
		c.Next()
	})
	router.GET("/openai/v1/responses", h.ResponsesWebSocket)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/openai/v1/responses", nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn, repo, func() ([]int64, []string, []string) {
		mu.Lock()
		defer mu.Unlock()
		return append([]int64(nil), accountIDs...), append([]string(nil), turns...), append([]string(nil), models...)
	}
}

//go:build unit && simulation

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeCapacityPostmanSimulation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	base := strings.TrimRight(os.Getenv("TEST_OPENCODE_CAPACITY_SIMULATION_URL"), "/")
	require.NotEmpty(t, base, "start the opencode-capacity-retry Postman simulation first")
	groupID := int64(24)
	repo := &grokCredentialHandlerRepo{}
	for i := 0; i < 2; i++ {
		repo.accounts = append(repo.accounts, service.Account{
			ID: int64(801 + i), Name: fmt.Sprintf("fixture-%d", i), Platform: service.PlatformOpenCodeGo,
			Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: i + 1,
			Credentials: map[string]any{
				"api_key": fmt.Sprintf("fixture-account-%c", 'a'+i), "base_url": base + "/v1", "api_protocol": service.APIProtocolChatCompletions,
			},
			// 保留未来的配额重置快照，容量 429 若误走配额分支就会阻止后续同账号重试。
			Extra: map[string]any{"opencode_go_5h_reset_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)},
		})
	}
	cfg := &config.Config{RunMode: config.RunModeSimple, Security: config.SecurityConfig{
		URLAllowlist: config.URLAllowlistConfig{AllowInsecureHTTP: true, AllowPrivateHosts: true},
	}}
	cfg.Gateway.MaxAccountSwitches = 3
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	limits := service.NewRateLimitService(repo, nil, cfg, nil, nil)
	gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, &grokMediaSlotBindings{}, cfg, nil, nil,
		service.NewBillingService(cfg, nil), limits, billing, repository.NewHTTPUpstream(cfg), &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
	cache := &concurrencyCacheMock{
		acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
	}
	h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(cache), billing, &service.APIKeyService{}, nil, nil, nil, nil, cfg)
	apiKey := &service.APIKey{ID: 902, GroupID: &groupID, User: &service.User{ID: 903, Status: service.StatusActive},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenCodeGo, Status: service.StatusActive}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.User.ID, Concurrency: 1})
		c.Next()
	})
	router.POST("/v1/chat/completions", h.ChatCompletions)
	router.POST("/v1/responses", h.Responses)
	router.POST("/v1/messages", h.Messages)

	type hit struct {
		Model   string `json:"model"`
		Account string `json:"account"`
		At      int64  `json:"at"`
	}
	readHits := func(model string) []hit {
		res, err := http.Get(base + "/_requests")
		require.NoError(t, err)
		defer res.Body.Close()
		var all, selected []hit
		require.NoError(t, json.NewDecoder(res.Body).Decode(&all))
		for _, row := range all {
			if row.Model == model {
				selected = append(selected, row)
			}
		}
		return selected
	}
	for _, tt := range []struct {
		name, path, content string
		stream              bool
	}{
		{"chat", "/v1/chat/completions", `"messages":[{"role":"user","content":"hello"}]`, false},
		{"stream", "/v1/chat/completions", `"messages":[{"role":"user","content":"hello"}]`, true},
		{"responses", "/v1/responses", `"input":"hello"`, false},
		{"messages", "/v1/messages", `"max_tokens":10,"messages":[{"role":"user","content":"hello"}]`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			model := "deepseek-recover-" + tt.name
			body := fmt.Sprintf(`{"model":%q,"stream":%t,%s}`, model, tt.stream, tt.content)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)
			require.Equal(t, 200, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "Recovered")
			require.NotContains(t, rec.Body.String(), "ttft_admission_refused", "intermediate errors must not reach the client")
			if tt.stream {
				require.Equal(t, 1, strings.Count(rec.Body.String(), "[DONE]"))
			}
			hits := readHits(model)
			require.Len(t, hits, 2)
			require.Equal(t, hits[0].Account, hits[1].Account, "retry must stay on the selected account")
			require.GreaterOrEqual(t, hits[1].At-hits[0].At, int64(1900), "respect the observed two-second admission delay")
		})
	}
	t.Run("exhaust_then_switch", func(t *testing.T) {
		model := "deepseek-exhaust"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"`+model+`","messages":[{"role":"user","content":"hello"}]}`)))
		require.Equal(t, 200, rec.Code, rec.Body.String())
		hits := readHits(model)
		var accounts []string
		for _, row := range hits {
			accounts = append(accounts, row.Account)
		}
		require.Equal(t, []string{"a", "a", "a", "a", "b"}, accounts)
	})
	t.Run("single_account_exhaustion_returns_429", func(t *testing.T) {
		accounts := repo.accounts
		repo.accounts = accounts[:1]
		defer func() { repo.accounts = accounts }()
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-exhaust-single","messages":[{"role":"user","content":"hello"}]}`)))
		require.Equal(t, 429, rec.Code, rec.Body.String())
		hits := readHits("deepseek-exhaust-single")
		require.Len(t, hits, 4)
		for _, row := range hits {
			require.Equal(t, "a", row.Account)
		}
	})
	t.Run("cancel_during_wait", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-cancel","messages":[{"role":"user","content":"hello"}]}`)).WithContext(ctx)
		started := time.Now()
		router.ServeHTTP(httptest.NewRecorder(), req)
		require.Less(t, time.Since(started), time.Second)
		require.Len(t, readHits("deepseek-cancel"), 1)
	})
	t.Run("parameter_error_is_not_retried", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-invalid","messages":[{"role":"user","content":"hello"}]}`)))
		require.Equal(t, 400, rec.Code)
		require.Len(t, readHits("deepseek-invalid"), 1)
	})
	require.Empty(t, repo.rateLimitedAccountIDs(), "capacity errors must not write quota cooldowns")
	require.Empty(t, repo.setTempIDs, "request-scoped capacity must not disable accounts")
}

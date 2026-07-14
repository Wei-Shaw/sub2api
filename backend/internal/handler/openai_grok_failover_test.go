//go:build unit

package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type grok429FailoverAccountRepo struct {
	service.AccountRepository

	mu             sync.Mutex
	accounts       []service.Account
	rateLimitedIDs []int64
	tempUnschedIDs []int64
}

func (r *grok429FailoverAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			account := r.accounts[i]
			return &account, nil
		}
	}
	return nil, service.ErrNoAvailableAccounts
}

func (r *grok429FailoverAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, _ int64, platform string) ([]service.Account, error) {
	return r.accountsForPlatform(platform), nil
}

func (r *grok429FailoverAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	return r.accountsForPlatform(platform), nil
}

func (r *grok429FailoverAccountRepo) ListSchedulableUngroupedByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	return r.accountsForPlatform(platform), nil
}

func (r *grok429FailoverAccountRepo) accountsForPlatform(platform string) []service.Account {
	accounts := make([]service.Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account.Platform == platform {
			accounts = append(accounts, account)
		}
	}
	return accounts
}

func (r *grok429FailoverAccountRepo) UpdateExtra(_ context.Context, _ int64, _ map[string]any) error {
	return nil
}

func (r *grok429FailoverAccountRepo) SetRateLimited(_ context.Context, id int64, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rateLimitedIDs = append(r.rateLimitedIDs, id)
	return nil
}

func (r *grok429FailoverAccountRepo) SetTempUnschedulable(_ context.Context, id int64, _ time.Time, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tempUnschedIDs = append(r.tempUnschedIDs, id)
	return nil
}

func (r *grok429FailoverAccountRepo) rateLimited() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int64(nil), r.rateLimitedIDs...)
}

type grok429FailoverHTTPUpstream struct {
	service.HTTPUpstream

	mu         sync.Mutex
	accountIDs []int64
	statuses   map[int64]int
}

func (u *grok429FailoverHTTPUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.accountIDs = append(u.accountIDs, accountID)
	status := u.statuses[accountID]
	u.mu.Unlock()

	if status == http.StatusTooManyRequests {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
				"Retry-After":  []string{"45"},
			},
			Body: io.NopCloser(bytes.NewBufferString(`{"error":{"message":"Upstream rate limit exceeded, please retry later","type":"rate_limit_error"}}`)),
		}, nil
	}
	if status >= http.StatusBadRequest {
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(`{"error":{"message":"upstream unavailable"}}`)),
		}, nil
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(bytes.NewBufferString(
			`{"id":"chatcmpl-grok-failover","object":"chat.completion","model":"grok-4.5","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":1,"total_tokens":9}}`,
		)),
	}, nil
}

func (u *grok429FailoverHTTPUpstream) calls() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.accountIDs...)
}

func newGrok429FailoverAccount(id int64, priority int) service.Account {
	return service.Account{
		ID:          id,
		Name:        "grok-account",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    priority,
		Credentials: map[string]any{
			"access_token":  "token",
			"refresh_token": "refresh",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
}

func newGrok429FailoverHandler(
	t *testing.T,
	statuses map[int64]int,
) (*OpenAIGatewayHandler, *grok429FailoverAccountRepo, *grok429FailoverHTTPUpstream, *gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	groupID := int64(4238)
	accounts := []service.Account{
		newGrok429FailoverAccount(1, 0),
		newGrok429FailoverAccount(2, 1),
		newGrok429FailoverAccount(3, 2),
	}
	accountRepo := &grok429FailoverAccountRepo{accounts: accounts}
	upstream := &grok429FailoverHTTPUpstream{statuses: statuses}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billingService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingService.Stop)
	gatewayService := service.NewOpenAIGatewayService(
		accountRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		nil,
		nil,
		service.NewBillingService(cfg, nil),
		nil,
		billingService,
		upstream,
		&service.DeferredService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	concurrencyCache := &concurrencyCacheMock{
		acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
	}
	h := NewOpenAIGatewayHandler(
		gatewayService,
		service.NewConcurrencyService(concurrencyCache),
		billingService,
		&service.APIKeyService{},
		nil,
		nil,
		nil,
		nil,
		cfg,
	)
	h.maxAccountSwitches = 10

	body := []byte(`{"model":"grok-4.5","messages":[{"role":"user","content":"1+1="}],"max_tokens":50,"stream":false}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      99,
		GroupID: &groupID,
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformGrok,
			Status:   service.StatusActive,
		},
		User: &service.User{ID: 100, Status: service.StatusActive},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 100, Concurrency: 1})
	return h, accountRepo, upstream, c, rec
}

func TestOpenAIGatewayHandlerChatCompletions_Grok429FailoverIsBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("first 429 rotates to one healthy account", func(t *testing.T) {
		h, accountRepo, upstream, c, rec := newGrok429FailoverHandler(t, map[int64]int{
			1: http.StatusTooManyRequests,
		})

		h.ChatCompletions(c)

		require.Equal(t, []int64{1, 2}, upstream.calls())
		require.Equal(t, []int64{1}, accountRepo.rateLimited())
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, "chatcmpl-grok-failover", gjson.GetBytes(rec.Body.Bytes(), "id").String())
		require.Equal(t, "ok", gjson.GetBytes(rec.Body.Bytes(), "choices.0.message.content").String())
	})

	t.Run("second 429 stops without sweeping third account", func(t *testing.T) {
		h, accountRepo, upstream, c, rec := newGrok429FailoverHandler(t, map[int64]int{
			1: http.StatusTooManyRequests,
			2: http.StatusTooManyRequests,
		})

		h.ChatCompletions(c)

		require.Equal(t, []int64{1, 2}, upstream.calls())
		require.Equal(t, []int64{1, 2}, accountRepo.rateLimited())
		require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
	})

	t.Run("500 followup stops without sweeping third account", func(t *testing.T) {
		h, accountRepo, upstream, c, rec := newGrok429FailoverHandler(t, map[int64]int{
			1: http.StatusTooManyRequests,
			2: http.StatusInternalServerError,
		})

		h.ChatCompletions(c)

		require.Equal(t, []int64{1, 2}, upstream.calls())
		require.Equal(t, []int64{1}, accountRepo.rateLimited())
		require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
	})
}

func TestOpenAIGatewayHandlerGrokMedia429FailoverIsBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)

	runVideoStatus := func(c *gin.Context) {
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/request-1", nil)
		c.Params = gin.Params{{Key: "request_id", Value: "request-1"}}
	}

	t.Run("first 429 rotates to one healthy account", func(t *testing.T) {
		h, accountRepo, upstream, c, rec := newGrok429FailoverHandler(t, map[int64]int{
			1: http.StatusTooManyRequests,
		})
		runVideoStatus(c)

		h.GrokVideoStatus(c)

		require.Equal(t, []int64{1, 2}, upstream.calls())
		require.Equal(t, []int64{1}, accountRepo.rateLimited())
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	})

	t.Run("second 429 stops without sweeping third account", func(t *testing.T) {
		h, accountRepo, upstream, c, rec := newGrok429FailoverHandler(t, map[int64]int{
			1: http.StatusTooManyRequests,
			2: http.StatusTooManyRequests,
		})
		runVideoStatus(c)

		h.GrokVideoStatus(c)

		require.Equal(t, []int64{1, 2}, upstream.calls())
		require.Equal(t, []int64{1, 2}, accountRepo.rateLimited())
		require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
	})
}

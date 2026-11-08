//go:build unit

package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Exercise the actual forwarding handlers with their usage worker blocked. The
// same Gin context then handles another request before the queued charge runs.
// HandleContext makes that reuse deterministic, including under -race (where
// sync.Pool is allowed to randomly discard entries).
func TestGatewayUsageKeepsModelAfterGinContextReuse(t *testing.T) {
	for _, endpoint := range []string{"chat/completions", "responses"} {
		t.Run(endpoint, func(t *testing.T) {
			cfg := &config.Config{}
			groupID := int64(9300)
			group := &service.Group{ID: groupID, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive, RateMultiplier: 1}
			account := &service.Account{
				ID: 9301, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
				Status: service.StatusActive, Schedulable: true, Concurrency: 1,
				Credentials:   map[string]any{"api_key": "test-key"},
				AccountGroups: []service.AccountGroup{{AccountID: 9301, GroupID: groupID}},
			}
			snapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: []*service.Account{account}}, nil, nil, nil, nil)
			usageRepo := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 1)}
			billingRepo := &compatStreamUsageBillingRepo{}
			billingCache := service.NewBillingCacheService(newHandlerInflightCache(100), nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billingCache.Stop)
			upstream := &compatStreamUsageUpstream{response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(compatStreamUsagePartialStream + compatStreamUsageCompletion)),
				Header:     http.Header{"X-Request-Id": []string{"usage-context-reuse"}},
			}}
			gw := service.NewGatewayService(
				nil, &fakeGroupRepo{group: group}, usageRepo, billingRepo, nil, nil, nil, nil, cfg,
				snapshot, nil, service.NewBillingService(cfg, nil), nil, nil, nil, upstream,
				&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			)
			pool := newUsageRecordTestPool(t)
			blocked, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			pool.Submit(func(context.Context) { close(blocked); <-release })
			<-blocked
			h := &GatewayHandler{
				gatewayService: gw, billingCacheService: billingCache, cfg: cfg, maxAccountSwitches: 1,
				usageRecordWorkerPool: pool,
				concurrencyHelper:     NewConcurrencyHelper(service.NewConcurrencyService(&fakeConcurrencyCache{}), SSEPingFormatClaude, 0),
			}
			apiKey := &service.APIKey{ID: 9302, UserID: 9303, GroupID: &groupID, Group: group, User: &service.User{ID: 9303, Balance: 100}}
			var first *gin.Context
			router := gin.New()
			router.POST("/v1/"+endpoint, func(c *gin.Context) {
				first = c
				c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.RequestedPublicModel, "public-model-A"))
				c.Set(string(middleware.ContextKeyAPIKey), apiKey)
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 9303})
				if endpoint == "chat/completions" {
					h.ChatCompletions(c)
				} else {
					h.Responses(c)
				}
			})
			router.GET("/next", func(c *gin.Context) {
				c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.RequestedPublicModel, "public-model-B"))
			})
			body := `{"model":"claude-sonnet-4.5","stream":true,"input":"hello"}`
			if endpoint == "chat/completions" {
				body = `{"model":"claude-sonnet-4.5","stream":true,"messages":[{"role":"user","content":"hello"}]}`
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/"+endpoint, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			require.Equal(t, uint64(1), pool.Stats().WaitingTasks)

			first.Request = httptest.NewRequest(http.MethodGet, "/next", nil)
			router.HandleContext(first)
			unblock()
			select {
			case usage := <-usageRepo.created:
				require.Equal(t, "public-model-A", usage.RequestedModel)
			case <-time.After(2 * time.Second):
				t.Fatal("queued request usage was not recorded")
			}
		})
	}
}

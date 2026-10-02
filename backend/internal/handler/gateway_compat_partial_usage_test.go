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

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// compatUsageLogRepository lets the capture embed the full repository
// interface while only implementing Create.
type compatUsageLogRepository = service.UsageLogRepository

type compatPartialUsageRepo struct {
	compatUsageLogRepository
	mu   sync.Mutex
	logs []service.UsageLog
}

func (r *compatPartialUsageRepo) Create(ctx context.Context, usage *service.UsageLog) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, *usage)
	return true, nil
}

type compatPartialUsageUpstream struct {
	payload string
	cancel  context.CancelFunc
	calls   int
}

func (u *compatPartialUsageUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return u.DoWithTLS(req, proxyURL, accountID, accountConcurrency, nil)
}

func (u *compatPartialUsageUpstream) DoWithTLS(_ *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.calls++
	if u.cancel != nil {
		u.cancel()
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(u.payload))}, nil
}

const compatPartialMessageStartSSE = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_partial\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-sonnet-4-5\",\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n" +
	"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
	"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"partial\"}}\n\n"

// Streams through the Chat Completions and Responses compatibility entries on
// an Anthropic account must persist the upstream-metered usage exactly once
// whether the stream completes, is truncated, or the client left first.
func TestGatewayCompatibleHandlersPreservePartialUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"chat/completions", "responses"} {
		for _, ending := range []string{"complete", "truncated", "disconnected_truncated", "before_start"} {
			t.Run(endpoint+"/"+ending, func(t *testing.T) {
				groupID := int64(9200)
				group := &service.Group{ID: groupID, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive, RateMultiplier: 1}
				account := &service.Account{
					ID: 9201, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
					Status: service.StatusActive, Schedulable: true, Concurrency: 1,
					Credentials:   map[string]any{"api_key": "local-test"},
					AccountGroups: []service.AccountGroup{{AccountID: 9201, GroupID: groupID}},
				}
				payload := compatPartialMessageStartSSE + "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":15}}\n\n"
				switch ending {
				case "complete":
					payload += "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
				case "before_start":
					payload = "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Unavailable\"}}\n\n"
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				upstream := &compatPartialUsageUpstream{payload: payload}
				if ending == "disconnected_truncated" {
					upstream.cancel = cancel
				}
				usageRepo := &compatPartialUsageRepo{}
				cfg := &config.Config{RunMode: config.RunModeSimple}
				snapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: []*service.Account{account}}, nil, nil, nil, nil)
				gateway := service.NewGatewayService(
					nil, &fakeGroupRepo{group: group}, usageRepo, nil, nil, nil, nil, nil, cfg,
					snapshot, nil, service.NewBillingService(cfg, nil), nil, nil, nil, upstream,
					&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				)
				billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
				t.Cleanup(billingCache.Stop)
				pool := newUsageRecordTestPool(t)
				h := &GatewayHandler{
					gatewayService: gateway, billingCacheService: billingCache, cfg: cfg,
					concurrencyHelper:     NewConcurrencyHelper(service.NewConcurrencyService(&fakeConcurrencyCache{}), SSEPingFormatClaude, 0),
					usageRecordWorkerPool: pool,
				}
				apiKey := &service.APIKey{ID: 9202, UserID: 9203, GroupID: &groupID, Group: group, Status: service.StatusActive,
					User: &service.User{ID: 9203, Concurrency: 10, Balance: 100}}
				body := `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}],"stream":true}`
				if endpoint == "responses" {
					body = `{"model":"claude-sonnet-4-5","input":"hello","stream":true}`
				}
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+endpoint, strings.NewReader(body)).WithContext(context.WithValue(ctx, ctxkey.Group, group))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Set(string(middleware.ContextKeyAPIKey), apiKey)
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 10})
				if endpoint == "responses" {
					h.Responses(c)
				} else {
					h.ChatCompletions(c)
				}
				pool.Stop()
				require.Equal(t, 1, upstream.calls, "no replay after a started or cancelled request")
				usageRepo.mu.Lock()
				defer usageRepo.mu.Unlock()
				if ending == "before_start" {
					require.Empty(t, usageRepo.logs, "an upstream failure before any usage must not be billed")
					return
				}
				require.Len(t, usageRepo.logs, 1, "upstream-metered usage must be persisted exactly once; response=%s", recorder.Body.String())
				require.Equal(t, 10, usageRepo.logs[0].InputTokens)
				require.Equal(t, 15, usageRepo.logs[0].OutputTokens)
				require.Equal(t, apiKey.ID, usageRepo.logs[0].APIKeyID)
				require.Equal(t, account.ID, usageRepo.logs[0].AccountID)
			})
		}
	}
}

//go:build unit

package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type compatibilityDisconnectWriter struct {
	gin.ResponseWriter
	writes int
}

func (w *compatibilityDisconnectWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, errors.New("client disconnected")
}

func (w *compatibilityDisconnectWriter) WriteString(string) (int, error) {
	w.writes++
	return 0, errors.New("client disconnected")
}

type compatibilityDisconnectUpstream struct {
	service.HTTPUpstream
	response *http.Response
	calls    int
}

func (u *compatibilityDisconnectUpstream) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	u.calls++
	return u.response, nil
}

type compatibilityDisconnectBillingRepo struct {
	service.UsageBillingRepository
	commands []*service.UsageBillingCommand
	balance  float64
}

func (r *compatibilityDisconnectBillingRepo) Apply(ctx context.Context, cmd *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.commands = append(r.commands, cmd)
	r.balance -= cmd.BalanceCost
	return &service.UsageBillingApplyResult{Applied: true}, nil
}

func TestGatewayCompatibilityHandlers_DisconnectTimeoutSettlesUsageWithoutErrorWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"chat/completions", "responses"} {
		t.Run(endpoint, func(t *testing.T) {
			cfg := &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1}}
			groupID := int64(9200)
			group := &service.Group{ID: groupID, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive, RateMultiplier: 1}
			account := &service.Account{
				ID: 9201, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
				Status: service.StatusActive, Schedulable: true, Concurrency: 1,
				Credentials:   map[string]any{"api_key": "test-key"},
				AccountGroups: []service.AccountGroup{{AccountID: 9201, GroupID: groupID}},
			}
			snapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: []*service.Account{account}}, nil, nil, nil, nil)
			usageRepo := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 1)}
			billingRepo := &compatibilityDisconnectBillingRepo{balance: 100}
			billingCache := service.NewBillingCacheService(newHandlerInflightCache(100), nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billingCache.Stop)
			pr, pw := io.Pipe()
			t.Cleanup(func() { _ = pr.Close(); _ = pw.Close() })
			upstream := &compatibilityDisconnectUpstream{response: &http.Response{
				StatusCode: http.StatusOK, Body: pr,
				Header: http.Header{"X-Request-Id": []string{"compat-disconnect"}},
			}}
			go func() {
				// Keep the upstream body open without further data until the handler
				// closes it on idle timeout after observing a failed downstream write.
				_, _ = io.WriteString(pw, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"role\":\"assistant\",\"content\":[],\"usage\":{\"input_tokens\":12}}}\n\n")
			}()
			gw := service.NewGatewayService(
				nil, &fakeGroupRepo{group: group}, usageRepo, billingRepo, nil, nil, nil, nil, cfg,
				snapshot, nil, service.NewBillingService(cfg, nil), nil, nil, nil, upstream,
				&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			)
			h := &GatewayHandler{
				gatewayService: gw, billingCacheService: billingCache, cfg: cfg, maxAccountSwitches: 1,
				concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(&fakeConcurrencyCache{}), SSEPingFormatClaude, 0),
			}
			body := `{"model":"claude-sonnet-4.5","stream":true,"input":"hello"}`
			if endpoint == "chat/completions" {
				body = `{"model":"claude-sonnet-4.5","stream":true,"messages":[{"role":"user","content":"hello"}]}`
			}
			core, logs := observer.New(zap.DebugLevel)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+endpoint, bytes.NewBufferString(body)).WithContext(logger.IntoContext(context.Background(), zap.New(core)))
			c.Request.Header.Set("Content-Type", "application/json")
			writer := &compatibilityDisconnectWriter{ResponseWriter: c.Writer}
			c.Writer = writer
			apiKey := &service.APIKey{ID: 9202, UserID: 9203, GroupID: &groupID, Group: group, User: &service.User{ID: 9203, Balance: 100}}
			c.Set(string(middleware.ContextKeyAPIKey), apiKey)
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 9203})
			if endpoint == "chat/completions" {
				h.ChatCompletions(c)
			} else {
				h.Responses(c)
			}

			require.Equal(t, 1, upstream.calls, "a disconnected client must not cause account retry")
			require.Equal(t, 1, writer.writes, "only the initial failed stream write is attempted")
			require.Len(t, billingRepo.commands, 1, "partial usage must be settled exactly once")
			require.Equal(t, 12, billingRepo.commands[0].InputTokens)
			require.Positive(t, billingRepo.commands[0].BalanceCost)
			require.Less(t, billingRepo.balance, float64(100))
			require.Len(t, usageRepo.created, 1)
			require.Equal(t, 12, (<-usageRepo.created).InputTokens)
			require.Zero(t, logs.FilterMessage("gateway.cc.forward_failed").Len())
			require.Zero(t, logs.FilterMessage("gateway.responses.forward_failed").Len())
		})
	}
}

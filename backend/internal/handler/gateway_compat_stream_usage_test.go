//go:build unit

package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type compatStreamUsageUpstream struct {
	service.HTTPUpstream
	response *http.Response
	calls    int
}

func (u *compatStreamUsageUpstream) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	u.calls++
	return u.response, nil
}

type compatStreamUsageBillingRepo struct {
	service.UsageBillingRepository
	mu       sync.Mutex
	commands []*service.UsageBillingCommand
}

func (r *compatStreamUsageBillingRepo) Apply(ctx context.Context, cmd *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, cmd)
	return &service.UsageBillingApplyResult{Applied: true}, nil
}

const compatStreamUsagePartialStream = "event: message_start\n" +
	`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","usage":{"input_tokens":12,"output_tokens":1}}}` + "\n\n" +
	"event: content_block_start\n" +
	`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
	"event: content_block_delta\n" +
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}` + "\n\n"

const compatStreamUsageCompletion = "event: content_block_stop\n" +
	`data: {"type":"content_block_stop","index":0}` + "\n\n" +
	"event: message_delta\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}` + "\n\n" +
	"event: message_stop\n" +
	`data: {"type":"message_stop"}` + "\n\n"

type compatStreamUsageRun struct {
	rec         *httptest.ResponseRecorder
	upstream    *compatStreamUsageUpstream
	usageRepo   *openAIWSUsageHandlerUsageLogRepoStub
	billingRepo *compatStreamUsageBillingRepo
}

// runCompatStreamUsageHandler 经 GatewayHandler 转发一次流式请求；上游写完 upstreamStream 后，
// closeUpstream 为 false 时保持连接不再发送数据。
func runCompatStreamUsageHandler(t *testing.T, endpoint, upstreamStream string, closeUpstream bool) compatStreamUsageRun {
	t.Helper()
	cfg := &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1}}
	groupID := int64(9300)
	group := &service.Group{ID: groupID, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive, RateMultiplier: 1}
	account := &service.Account{
		ID: 9301, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		Credentials:   map[string]any{"api_key": "test-key"},
		AccountGroups: []service.AccountGroup{{AccountID: 9301, GroupID: groupID}},
	}
	snapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: []*service.Account{account}}, nil, nil, nil, nil)
	run := compatStreamUsageRun{
		rec:         httptest.NewRecorder(),
		usageRepo:   &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 2)},
		billingRepo: &compatStreamUsageBillingRepo{},
	}
	billingCache := service.NewBillingCacheService(newHandlerInflightCache(100), nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)

	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pr.Close(); _ = pw.Close() })
	go func() {
		_, _ = io.WriteString(pw, upstreamStream)
		if closeUpstream {
			_ = pw.Close()
		}
	}()
	run.upstream = &compatStreamUsageUpstream{response: &http.Response{
		StatusCode: http.StatusOK, Body: pr,
		Header: http.Header{"X-Request-Id": []string{"compat-stream-usage"}},
	}}
	gw := service.NewGatewayService(
		nil, &fakeGroupRepo{group: group}, run.usageRepo, run.billingRepo, nil, nil, nil, nil, cfg,
		snapshot, nil, service.NewBillingService(cfg, nil), nil, nil, nil, run.upstream,
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
	c, _ := gin.CreateTestContext(run.rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+endpoint, bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	apiKey := &service.APIKey{ID: 9302, UserID: 9303, GroupID: &groupID, Group: group, User: &service.User{ID: 9303, Balance: 100}}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 9303})

	if endpoint == "chat/completions" {
		h.ChatCompletions(c)
	} else {
		h.Responses(c)
	}
	return run
}

// compatStreamUsageMarkers 返回成功收尾与终止错误的标记。
func compatStreamUsageMarkers(endpoint string) (success, failure string) {
	if endpoint == "responses" {
		return "event: response.completed", "event: response.failed"
	}
	return "[DONE]", `data: {"error":`
}

func TestGatewayCompatibilityHandlers_StreamUsageRecordedOnceWithTerminalEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"chat/completions", "responses"} {
		for _, tc := range []struct {
			name       string
			completion string
			wantOutput int
		}{
			{name: "complete", completion: compatStreamUsageCompletion, wantOutput: 7},
			{name: "idle_before_message_stop", wantOutput: 1},
		} {
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				successMarker, failureMarker := compatStreamUsageMarkers(endpoint)
				wantMarker, denyMarker := successMarker, failureMarker
				if tc.completion == "" {
					wantMarker, denyMarker = failureMarker, successMarker
				}

				run := runCompatStreamUsageHandler(t, endpoint, compatStreamUsagePartialStream+tc.completion, tc.completion != "")

				require.Equal(t, 1, run.upstream.calls)
				out := run.rec.Body.String()
				require.Contains(t, out, "partial")
				require.Contains(t, out, wantMarker)
				require.NotContains(t, out, denyMarker)
				require.Equal(t, 1, strings.Count(out, wantMarker), "exactly one terminal event")
				require.NotContains(t, out, `data: {"type":"error"`, "the handler must not append a fallback frame")
				if tc.completion == "" {
					require.Contains(t, out, "stream_timeout")
				}

				run.billingRepo.mu.Lock()
				defer run.billingRepo.mu.Unlock()
				require.Len(t, run.billingRepo.commands, 1, "usage must be settled exactly once")
				require.Equal(t, 12, run.billingRepo.commands[0].InputTokens)
				require.Equal(t, tc.wantOutput, run.billingRepo.commands[0].OutputTokens)
				require.Len(t, run.usageRepo.created, 1)
			})
		}
	}
}

func TestGatewayCompatibilityHandlers_IdleBeforeAnyEventWritesStreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"chat/completions", "responses"} {
		t.Run(endpoint, func(t *testing.T) {
			successMarker, failureMarker := compatStreamUsageMarkers(endpoint)

			run := runCompatStreamUsageHandler(t, endpoint, "", false)

			require.Equal(t, 1, run.upstream.calls)
			require.Equal(t, http.StatusOK, run.rec.Code)
			require.Equal(t, "text/event-stream", run.rec.Header().Get("Content-Type"))
			out := run.rec.Body.String()
			require.Equal(t, 1, strings.Count(out, failureMarker), "exactly one terminal event")
			require.NotContains(t, out, `data: {"type":"error"`, "the handler must not append a fallback frame")
			require.Contains(t, out, "stream_timeout")
			require.NotContains(t, out, successMarker)

			run.billingRepo.mu.Lock()
			defer run.billingRepo.mu.Unlock()
			require.Empty(t, run.billingRepo.commands, "no usage was observed")
			require.Empty(t, run.usageRepo.created)
		})
	}
}

func TestGatewayCompatibilityHandlers_UpstreamErrorEventAfterOutputIsNotBilled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamError := "event: error\n" + `data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}` + "\n\n"
	for _, endpoint := range []string{"chat/completions", "responses"} {
		t.Run(endpoint, func(t *testing.T) {
			successMarker, failureMarker := compatStreamUsageMarkers(endpoint)

			run := runCompatStreamUsageHandler(t, endpoint, compatStreamUsagePartialStream+upstreamError, true)

			require.Equal(t, 1, run.upstream.calls)
			out := run.rec.Body.String()
			require.Contains(t, out, "partial")
			require.Equal(t, 1, strings.Count(out, failureMarker), "exactly one terminal event")
			require.NotContains(t, out, `data: {"type":"error"`, "the handler must not append a fallback frame")
			require.Contains(t, out, "Overloaded")
			require.NotContains(t, out, successMarker)

			run.billingRepo.mu.Lock()
			defer run.billingRepo.mu.Unlock()
			require.Empty(t, run.billingRepo.commands, "an upstream error event is not billed while the client is online")
			require.Empty(t, run.usageRepo.created)
		})
	}
}

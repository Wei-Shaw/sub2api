package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIStreamKeyRetryUpstream struct {
	service.HTTPUpstream
	mu                  sync.Mutex
	accountIDs          []int64
	failFallback        bool
	partialFirstOutput  bool
	multilineFirstEvent bool
}

func (u *openAIStreamKeyRetryUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.accountIDs = append(u.accountIDs, accountID)
	attempt := len(u.accountIDs)
	u.mu.Unlock()

	if attempt > 1 && !u.failFallback {
		body := "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_fallback\"}}\n\n" +
			"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"done\"}\n\n" +
			"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_fallback\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n" +
			"data: [DONE]\n\n"
		return openAIStreamKeyRetryResponse(body), nil
	}

	created := "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_first\"}}\n\n"
	if u.multilineFirstEvent && attempt == 1 {
		created = "data: {\ndata: \"type\":\"response.created\",\"response\":{\"id\":\"resp_first\"}}\n\n"
	}
	if u.partialFirstOutput && attempt == 1 {
		created += "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"
	}
	failed := "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed\",\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"upstream gateway failure\"}}}\n\n"
	return openAIStreamKeyRetryResponse(created + failed), nil
}

func (u *openAIStreamKeyRetryUpstream) calls() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.accountIDs...)
}

func openAIStreamKeyRetryResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func runOpenAIStreamKeyRetry(t *testing.T, input string, passthrough, failFallback, partialFirstOutput, multilineFirstEvent bool) (*httptest.ResponseRecorder, []int64) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	groupID := int64(4203)
	accounts := make([]service.Account, 0, 3)
	for i, id := range []int64{9910, 9911, 9912} {
		credentials := map[string]any{
			"api_key":       fmt.Sprintf("test-key-%d", id),
			"base_url":      "https://api.example.test",
			"model_mapping": map[string]any{"gpt-5.2": "gpt-5.2"},
		}
		if i == 0 {
			credentials["pool_mode"] = true
			credentials["pool_mode_retry_count"] = float64(1)
			credentials["pool_mode_retry_status_codes"] = []any{float64(http.StatusBadGateway)}
		}
		accounts = append(accounts, service.Account{
			ID: id, Name: fmt.Sprintf("api-key-%d", id), Platform: service.PlatformOpenAI,
			Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Priority: i + 1,
			GroupIDs: []int64{groupID}, Credentials: credentials,
			Extra: map[string]any{"openai_passthrough": passthrough},
		})
	}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.MaxAccountSwitches = 3
	accountRepo := &openAIWSFailoverHandlerAccountRepoStub{accounts: accounts}
	upstream := &openAIStreamKeyRetryUpstream{failFallback: failFallback, partialFirstOutput: partialFirstOutput, multilineFirstEvent: multilineFirstEvent}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	gateway := service.NewOpenAIGatewayService(
		accountRepo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billing, upstream, &service.DeferredService{},
		nil, nil, nil, nil, nil, nil, nil,
	)
	handler := NewOpenAIGatewayHandler(
		gateway, service.NewConcurrencyService(nil), billing,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg,
	)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(input))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID: 1803, GroupID: &groupID,
		User:  &service.User{ID: 1703, Status: service.StatusActive},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1703, Concurrency: 0})
	handler.Responses(c)
	return recorder, upstream.calls()
}

func TestOpenAIResponsesStreamFailedFrameRetriesOneOtherAPIKey(t *testing.T) {
	for _, tc := range []struct {
		name                string
		passthrough         bool
		multilineFirstEvent bool
	}{
		{name: "native stream"},
		{name: "passthrough stream", passthrough: true},
		{name: "multiline metadata before failure", passthrough: true, multilineFirstEvent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder, calls := runOpenAIStreamKeyRetry(t, `{"model":"gpt-5.2","input":"hello","stream":true}`, tc.passthrough, false, false, tc.multilineFirstEvent)
			require.Equal(t, []int64{9910, 9911}, calls, "response: %s", recorder.Body.String())
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			require.Contains(t, recorder.Body.String(), "done")
			require.Contains(t, recorder.Body.String(), "resp_fallback")
			require.NotContains(t, recorder.Body.String(), "resp_first")
			require.NotContains(t, recorder.Body.String(), "response.failed")
		})
	}
}

func TestOpenAIResponsesStreamFailedFrameRetriesAtMostOneOtherKey(t *testing.T) {
	recorder, calls := runOpenAIStreamKeyRetry(t, `{"model":"gpt-5.2","input":"hello","stream":true}`, true, true, false, false)
	require.Equal(t, []int64{9910, 9911}, calls)
	require.NotContains(t, recorder.Body.String(), "resp_first")
	require.NotContains(t, recorder.Body.String(), "resp_failed")
}

func TestOpenAIResponsesStreamFailedFrameDoesNotReplayUnsafeOrDeliveredRequests(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		partialFirstOutput bool
	}{
		{name: "server-side tool", input: `{"model":"gpt-5.2","input":"hello","stream":true,"tools":[{"type":"web_search"}]}`},
		{name: "partial output delivered", input: `{"model":"gpt-5.2","input":"hello","stream":true}`, partialFirstOutput: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder, calls := runOpenAIStreamKeyRetry(t, tc.input, true, false, tc.partialFirstOutput, false)
			require.Equal(t, []int64{9910}, calls)
			if tc.partialFirstOutput {
				require.Contains(t, recorder.Body.String(), "partial")
			}
		})
	}
}

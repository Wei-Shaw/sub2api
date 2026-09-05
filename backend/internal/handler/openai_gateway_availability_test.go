package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const availability503Error = `{"error":{"type":"server_error","code":"server_is_overloaded","message":"The server is overloaded"}}`

const availability503FailedSSE = "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_a\",\"status\":\"failed\",\"error\":{\"type\":\"server_error\",\"code\":\"server_is_overloaded\",\"message\":\"The server is overloaded\"}}}\n\n"

const availabilityPartialSSE = "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_a\",\"output_index\":0,\"content_index\":0,\"delta\":\"A partial\"}\n\n"

const availabilitySuccessSSE = "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_b\",\"output_index\":0,\"content_index\":0,\"delta\":\"B answer\"}\n\n" +
	"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_b\",\"status\":\"completed\",\"model\":\"gpt-5.4\",\"output\":[{\"id\":\"msg_b\",\"type\":\"message\",\"role\":\"assistant\",\"status\":\"completed\",\"content\":[{\"type\":\"output_text\",\"text\":\"B answer\",\"annotations\":[]}]}],\"usage\":{\"input_tokens\":5,\"output_tokens\":2,\"total_tokens\":7}}}\n\n"

type availabilityHTTPUpstream struct {
	service.HTTPUpstream
	mu            sync.Mutex
	accountIDs    []int64
	failedStatus  int
	failedPayload string
	transportErr  error
	readErr       error
	failSecond    bool
}

type availabilityErrorBody struct {
	reader *strings.Reader
	err    error
}

func (b *availabilityErrorBody) Read(p []byte) (int, error) {
	if b.reader.Len() > 0 {
		return b.reader.Read(p)
	}
	return 0, b.err
}
func (b *availabilityErrorBody) Close() error { return nil }

type availabilityAccountRepo struct {
	*openAIWSFailoverHandlerAccountRepoStub
}

func (r *availabilityAccountRepo) SetError(_ context.Context, id int64, message string) error {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			r.accounts[i].Status = service.StatusError
			r.accounts[i].Schedulable = false
			r.accounts[i].ErrorMessage = message
		}
	}
	return nil
}

func (u *availabilityHTTPUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.accountIDs = append(u.accountIDs, accountID)
	u.mu.Unlock()
	status, payload := http.StatusOK, availabilitySuccessSSE
	contentType := "text/event-stream"
	if accountID == 9910 || (u.failSecond && accountID == 9911) {
		if u.transportErr != nil {
			return nil, u.transportErr
		}
		status, payload = u.failedStatus, u.failedPayload
		if status != http.StatusOK {
			contentType = "application/json"
		}
	}
	body := io.NopCloser(strings.NewReader(payload))
	if accountID == 9910 && u.readErr != nil {
		body = &availabilityErrorBody{reader: strings.NewReader(payload), err: u.readErr}
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}, "Retry-After": []string{"30"}},
		Body:       body,
		Request:    req,
	}, nil
}

func (u *availabilityHTTPUpstream) calls() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.accountIDs...)
}

func availabilityEventCount(t *testing.T, body, eventType string) int {
	t.Helper()
	count := 0
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			continue
		}
		var event struct {
			Type string `json:"type"`
		}
		require.NoError(t, json.Unmarshal([]byte(data), &event), "invalid SSE data: %s", data)
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func TestOpenAIResponses_APIKeyAvailabilityFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{true, false} {
		mode := "native"
		if passthrough {
			mode = "passthrough"
		}
		for _, tc := range []struct {
			name          string
			status        int
			payload       string
			outputStarted bool
			transportErr  error
			readErr       error
			failSecond    bool
		}{
			{name: "http_500", status: 500, payload: `{"error":{"message":"Internal server error"}}`},
			{name: "http_502", status: 502, payload: `{"error":{"message":"Bad gateway"}}`},
			{name: "switch_budget_exhausted", status: 502, payload: `{"error":{"message":"Bad gateway"}}`, failSecond: true},
			{name: "http_503", status: 503, payload: availability503Error},
			{name: "http_504", status: 504, payload: `{"error":{"message":"Gateway timeout"}}`},
			{name: "http_520", status: 520, payload: `{"error":{"message":"Upstream unavailable"}}`},
			{name: "http_521", status: 521, payload: `{"error":{"message":"Upstream unavailable"}}`},
			{name: "http_522", status: 522, payload: `{"error":{"message":"Upstream unavailable"}}`},
			{name: "http_523", status: 523, payload: `{"error":{"message":"Upstream unavailable"}}`},
			{name: "http_524", status: 524, payload: `{"error":{"message":"Upstream unavailable"}}`},
			{name: "http_401_credential", status: 401, payload: `{"error":{"type":"authentication_error","code":"invalid_api_key","message":"Incorrect API key"}}`},
			{name: "http_403_credential", status: 403, payload: `{"error":{"type":"authentication_error","code":"invalid_api_key","message":"Invalid API key"}}`},
			{name: "http_429", status: 429, payload: `{"error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"Rate limit exceeded"}}`},
			{name: "sse_500", status: 200, payload: "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"status_code\":500,\"message\":\"Internal server error\"}}}\n\n"},
			{name: "sse_502", status: 200, payload: "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"status_code\":502,\"message\":\"Bad gateway\"}}}\n\n"},
			{name: "sse_503_before_output", status: 200, payload: availability503FailedSSE},
			{name: "sse_504", status: 200, payload: "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"status_code\":504,\"message\":\"Gateway timeout\"}}}\n\n"},
			{name: "sse_503_after_output", status: 200, payload: availabilityPartialSSE + availability503FailedSSE, outputStarted: true},
			{name: "transport_reset", transportErr: syscall.ECONNRESET},
			{name: "transport_eof", transportErr: io.EOF},
			{name: "transport_timeout", transportErr: context.DeadlineExceeded},
			{name: "stream_read_reset", status: 200, payload: "data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\"}}\n\n", readErr: syscall.ECONNRESET},
			{name: "stream_read_eof", status: 200, payload: "data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\"}}\n\n", readErr: io.EOF},
			{name: "stream_read_timeout", status: 200, payload: "data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\"}}\n\n", readErr: context.DeadlineExceeded},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				cfg := &config.Config{RunMode: config.RunModeSimple}
				cfg.Default.RateMultiplier = 1
				cfg.Security.URLAllowlist.Enabled = false
				cfg.Gateway.MaxAccountSwitches = 1
				cfg.Gateway.OpenAIAPIKeyAvailabilityEnabled = true
				accountRepo := &availabilityAccountRepo{&openAIWSFailoverHandlerAccountRepoStub{accounts: []service.Account{
					{ID: 9910, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Priority: 1, Credentials: map[string]any{"api_key": "sk-fake-a", "base_url": "https://api.example.test"}, Extra: map[string]any{"openai_passthrough": passthrough}},
					{ID: 9911, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Priority: 2, Credentials: map[string]any{"api_key": "sk-fake-b", "base_url": "https://api.example.test"}, Extra: map[string]any{"openai_passthrough": passthrough}},
					{ID: 9912, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Priority: 3, Credentials: map[string]any{"api_key": "sk-fake-c", "base_url": "https://api.example.test"}, Extra: map[string]any{"openai_passthrough": passthrough}},
				}}}
				billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
				t.Cleanup(billingCacheSvc.Stop)
				rateLimitSvc := service.NewRateLimitService(accountRepo, nil, cfg, nil, nil)
				upstream := &availabilityHTTPUpstream{failedStatus: tc.status, failedPayload: tc.payload, transportErr: tc.transportErr, readErr: tc.readErr, failSecond: tc.failSecond}
				gatewaySvc := service.NewOpenAIGatewayService(accountRepo, nil, nil, nil, nil, nil, nil, cfg, nil, nil, service.NewBillingService(cfg, nil), rateLimitSvc, billingCacheSvc, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
				h := NewOpenAIGatewayHandler(gatewaySvc, service.NewConcurrencyService(nil), billingCacheSvc, service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
				groupID := int64(1903)
				request := func() *httptest.ResponseRecorder {
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4","input":"hello","stream":true,"prompt_cache_key":"availability-regression"}`))
					c.Request.Header.Set("Content-Type", "application/json")
					c.Request.Header.Set("session_id", "availability-regression")
					c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1803, GroupID: &groupID, User: &service.User{ID: 1703, Status: service.StatusActive}, Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive}})
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1703, Concurrency: 0})
					h.Responses(c)
					return recorder
				}

				first := request()
				if tc.failSecond {
					// A healthy third account exists, but the configured one-switch
					// budget must end this request after A and B fail.
					require.Equal(t, http.StatusBadGateway, first.Code, first.Body.String())
					require.Equal(t, []int64{9910, 9911}, upstream.calls())
					require.True(t, json.Valid(first.Body.Bytes()))
					require.NotContains(t, first.Body.String(), "B answer")
					return
				}
				require.Equal(t, http.StatusOK, first.Code, first.Body.String())
				if tc.outputStarted {
					// A's real text is already visible: replaying onto B would mix two answers.
					require.Equal(t, []int64{9910}, upstream.calls())
					require.Contains(t, first.Body.String(), "A partial")
					require.NotContains(t, first.Body.String(), "B answer")
					require.Equal(t, 0, availabilityEventCount(t, first.Body.String(), "response.completed"))
					require.Equal(t, 1, availabilityEventCount(t, first.Body.String(), "response.failed"))
				} else {
					// Both upstream attempts belong to this one client request; A is not retried.
					require.Equal(t, []int64{9910, 9911}, upstream.calls())
					require.Contains(t, first.Body.String(), "B answer")
					require.NotContains(t, first.Body.String(), "server_is_overloaded")
					require.Equal(t, 1, availabilityEventCount(t, first.Body.String(), "response.completed"))
					require.Equal(t, 0, availabilityEventCount(t, first.Body.String(), "response.failed"))
				}

				// Keep the same gateway and session. A still has the best priority, but its
				// first upstream failure must keep it out of the next request.
				before := len(upstream.calls())
				second := request()
				require.Equal(t, http.StatusOK, second.Code, second.Body.String())
				require.Equal(t, []int64{9911}, upstream.calls()[before:])
				require.Contains(t, second.Body.String(), "B answer")
				require.NotContains(t, second.Body.String(), "A partial")
				require.Equal(t, 1, availabilityEventCount(t, second.Body.String(), "response.completed"))
			})
		}
	}
}

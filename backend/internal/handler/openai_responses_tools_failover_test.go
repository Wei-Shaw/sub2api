//go:build unit

package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const solToolsTestResponse = `{"id":"resp_tools","object":"response","model":"gpt-6.1-sol","status":"completed","output":[{"id":"fc_lookup","type":"function_call","call_id":"call_lookup","name":"lookup","arguments":"{}","status":"completed"}],"usage":{"input_tokens":1,"output_tokens":1}}`

func TestResponsesSolToolsSwitchAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			accounts := solToolsTestAccounts(false, true)
			before, err := json.Marshal(accounts)
			require.NoError(t, err)
			response := solToolsTestResponse
			contentType := "application/json"
			if stream {
				contentType = "text/event-stream"
				response = "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_tools\",\"status\":\"in_progress\"}}\n\n" +
					"event: response.function_call_arguments.delta\ndata: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_lookup\",\"output_index\":0,\"delta\":\"{}\"}\n\n" +
					"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":" + solToolsTestResponse + "}\n\n"
			}
			upstream := newAstraProCapturedUpstream(&http.Response{
				StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}},
				Body: io.NopCloser(strings.NewReader(response)),
			})
			h := newSolToolsFailoverHandler(t, accounts, upstream)
			body := solToolsTestRequest(stream)
			c, rec := newAstraProFailoverContext(t, body)

			h.Responses(c)

			urls, accountIDs, bodies := upstream.snapshot()
			require.Equal(t, []int64{2}, accountIDs, "the first account is incompatible, not an upstream attempt")
			require.Equal(t, []string{"https://upstream.example/v1/responses"}, urls)
			require.Len(t, bodies, 1)
			require.Equal(t, "gpt-6.1-sol", gjson.GetBytes(bodies[0], "model").String())
			require.JSONEq(t, gjson.Get(body, "tools").Raw, gjson.GetBytes(bodies[0], "tools").Raw)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Body.String(), "function_call")
			require.NotContains(t, rec.Body.String(), "response.failed")
			require.NotContains(t, rec.Body.String(), "only supports Chat Completions")
			metrics := h.gatewayService.SnapshotOpenAIAccountSchedulerMetrics()
			require.EqualValues(t, 1, metrics.AccountSwitchTotal)
			require.Equal(t, 1, metrics.RuntimeStatsAccountCount, "only the successful account gets a health observation")
			after, err := json.Marshal(accounts)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after), "routing must not change capability flags, models or scheduling")
		})
	}
}

func TestResponsesSolToolsFailoverExhausted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name         string
		stream       bool
		started      bool
		maxSwitches  int
		wantSwitches int64
	}{
		{"json", false, false, 10, 2},
		{"stream_not_started", true, false, 10, 2},
		{"stream_keepalive", true, true, 10, 2},
		{"bounded", false, false, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := newAstraProCapturedUpstream()
			h := newSolToolsFailoverHandler(t, solToolsTestAccounts(false, false), upstream)
			h.maxAccountSwitches = tc.maxSwitches
			c, rec := newAstraProFailoverContext(t, solToolsTestRequest(tc.stream))
			if tc.started {
				_, err := c.Writer.WriteString(": keepalive\n\n")
				require.NoError(t, err)
				c.Writer.Flush()
			}

			h.Responses(c)

			_, accountIDs, _ := upstream.snapshot()
			require.Empty(t, accountIDs)
			if tc.started {
				require.Equal(t, 1, strings.Count(rec.Body.String(), "event: response.failed\n"))
				require.True(t, strings.HasPrefix(rec.Body.String(), ": keepalive\n\nevent: response.failed\n"))
			} else {
				require.Equal(t, http.StatusBadRequest, rec.Code)
				require.True(t, json.Valid(rec.Body.Bytes()), "must not append SSE to a JSON error")
				require.Equal(t, "invalid_request_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
			}
			require.Contains(t, rec.Body.String(), "gpt-6.1-sol")
			_, hasUpstreamError := c.Get(service.OpsUpstreamStatusCodeKey)
			require.False(t, hasUpstreamError, "local incompatibility must not be recorded as an upstream HTTP failure")
			metrics := h.gatewayService.SnapshotOpenAIAccountSchedulerMetrics()
			require.Equal(t, tc.wantSwitches, metrics.AccountSwitchTotal)
			require.Zero(t, metrics.RuntimeStatsAccountCount)
		})
	}
}

func TestResponsesSolPlainTextStillUsesChatAccount(t *testing.T) {
	upstream := newAstraProCapturedUpstream(&http.Response{
		StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"id":"chat_sol","model":"gpt-6.1-sol","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)),
	})
	h := newSolToolsFailoverHandler(t, solToolsTestAccounts(false, true), upstream)
	c, rec := newAstraProFailoverContext(t, `{"model":"public-alias","input":"hello","stream":false}`)

	h.Responses(c)

	urls, accountIDs, _ := upstream.snapshot()
	require.Equal(t, []int64{1}, accountIDs)
	require.Equal(t, []string{"https://upstream.example/v1/chat/completions"}, urls)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "ok", gjson.GetBytes(rec.Body.Bytes(), "output.0.content.0.text").String())
	require.Zero(t, h.gatewayService.SnapshotOpenAIAccountSchedulerMetrics().AccountSwitchTotal)
}

func solToolsTestRequest(stream bool) string {
	return fmt.Sprintf(`{"model":"public-alias","input":"hello","stream":%t,"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`, stream)
}

func solToolsTestAccounts(supported ...bool) []service.Account {
	accounts := make([]service.Account, 0, len(supported))
	for i, value := range supported {
		accounts = append(accounts, service.Account{
			ID: int64(i + 1), Name: fmt.Sprintf("responses-account-%d", i+1),
			Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
			Status: service.StatusActive, Schedulable: true, Priority: i,
			Credentials: map[string]any{
				"api_key": "test-key", "base_url": "https://upstream.example",
				"model_mapping": map[string]any{"public-alias": "gpt-6.1-sol"},
			},
			Extra: map[string]any{"openai_responses_supported": value},
		})
	}
	return accounts
}

func newSolToolsFailoverHandler(t *testing.T, accounts []service.Account, upstream service.HTTPUpstream) *OpenAIGatewayHandler {
	t.Helper()
	cfg := &config.Config{RunMode: config.RunModeSimple}
	gateway := service.NewOpenAIGatewayService(
		openAIImagesFailoverAccountRepo{accounts: accounts}, nil, nil, nil, nil, nil, nil, cfg,
		nil, nil, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	h := NewOpenAIGatewayHandler(
		gateway, service.NewConcurrencyService(nil), billing,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil, nil, nil, nil, cfg,
	)
	h.maxAccountSwitches = 10
	return h
}

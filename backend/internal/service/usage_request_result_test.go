package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestFinalizeOpenAIUsageResult(t *testing.T) {
	for _, tt := range []struct {
		name, payload, terminal, outcome, usageStatus string
		usage                                         OpenAIUsage
		err                                           error
		status                                        int
	}{
		{name: "missing usage is not empty input", outcome: "succeeded", usageStatus: "unavailable", status: 200},
		{name: "reported zero", usage: OpenAIUsage{Reported: true}, outcome: "succeeded", usageStatus: "reported", status: 200},
		{name: "failed without usage", payload: `{"error":{"code":"server_is_overloaded","type":"service_unavailable_error"}}`, terminal: "error", outcome: "failed", usageStatus: "unavailable", status: 503},
		{name: "failed with partial usage", payload: `{"error":{"code":"upstream_error","message":"Upstream service temporarily unavailable"}}`, usage: OpenAIUsage{InputTokens: 12}, terminal: "response.failed", outcome: "failed", usageStatus: "reported", status: 502},
		{name: "read failure", err: errors.New("unexpected EOF"), outcome: "failed", usageStatus: "unavailable", status: 502},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Writer.WriteHeaderNow()
			// Failed attempts alone must not turn a successful retry into failure.
			SetOpsUpstreamError(c, 503, "earlier attempt failed", "")
			if tt.payload != "" {
				markOpenAITerminalStreamFailure(c, []byte(tt.payload), "upstream failure")
			}
			result := &OpenAIForwardResult{Stream: true, Usage: tt.usage, UpstreamTerminalEvent: tt.terminal}
			FinalizeOpenAIUsageResult(c, result, tt.err)
			require.Equal(t, tt.outcome, result.RequestResult.Outcome)
			require.Equal(t, tt.usageStatus, result.RequestResult.UsageStatus)
			require.Equal(t, tt.status, result.RequestResult.StatusCode)
			require.Equal(t, http.StatusOK, result.RequestResult.HTTPStatusCode)
			require.Equal(t, tt.usage, result.Usage, "outcome tracking must not change billing")
			_, marked := GetOpsStreamError(c)
			require.Equal(t, tt.outcome == "failed", marked)
		})
	}
}

func TestOpenAIUsageReportedPresence(t *testing.T) {
	for _, tt := range []struct {
		payload  string
		reported bool
	}{
		{`null`, false}, {`{}`, false}, {`{"input_tokens":null}`, false},
		{`{"input_tokens":0,"output_tokens":0}`, true},
		{`{"prompt_tokens":0,"completion_tokens":0}`, true},
		{`{"input_tokens":123}`, true},
	} {
		t.Run(tt.payload, func(t *testing.T) {
			usage, _ := openAIUsageFromGJSON(gjson.Parse(tt.payload))
			require.Equal(t, tt.reported, usage.Reported)
			var merged OpenAIUsage
			mergeOpenAIUsageNonZero(&merged, usage)
			mergeOpenAIUsageNonZero(&merged, OpenAIUsage{})
			require.Equal(t, tt.reported, merged.Reported)
		})
	}
}

func TestChatStreamUsagePresence(t *testing.T) {
	for _, tt := range []struct {
		payload  string
		reported bool
	}{
		{`{}`, false},
		{`null`, false},
		{`{"input_tokens":0,"output_tokens":0}`, true},
	} {
		t.Run(tt.payload, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(`data: {"type":"response.completed","response":{"id":"resp_usage","status":"completed","output":[],"usage":` + tt.payload + "}}\n\n")),
			}
			svc := &OpenAIGatewayService{}
			result, err := svc.handleChatStreamingResponse(resp, c, &Account{Platform: PlatformOpenAI}, "gpt-test", "gpt-test", "gpt-test", time.Now(), 0)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tt.reported, result.Usage.Reported)
		})
	}
}

func TestOpenAIStreamFailureLogicalStatusAndRetry(t *testing.T) {
	account := &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true, "pool_mode_retry_status_codes": []any{float64(502), float64(503)}}}
	for _, tt := range []struct {
		payload, message string
		status           int
	}{
		{`{"type":"error","error":{"code":"upstream_error"}}`, "Upstream service temporarily unavailable", 502},
		{`{"type":"error","error":{"code":"server_is_overloaded","type":"service_unavailable_error"}}`, "Our servers are currently overloaded. Please try again later.", 503},
		{`{"type":"error","error":{"status_code":503,"code":"provider_busy"}}`, "busy", 503},
	} {
		t.Run(tt.message, func(t *testing.T) {
			payload := []byte(tt.payload)
			require.Equal(t, tt.status, openAIStreamFailedEventSemanticStatus(payload, tt.message))
			require.True(t, openAIStreamErrorEventShouldFailover(payload, tt.message))
			require.True(t, openAIStreamFailedEventRetryableOnSameAccount(account, payload, tt.message))
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			svc := &OpenAIGatewayService{}
			err := svc.newOpenAIStreamFailoverError(c, account, false, "req-test", payload, tt.message)
			require.Equal(t, tt.status, err.StatusCode)
			require.True(t, err.RetryableOnSameAccount)
			_, marked := GetOpsStreamError(c)
			require.False(t, marked, "intermediate retry must not mark the final request failed")
		})
	}
}

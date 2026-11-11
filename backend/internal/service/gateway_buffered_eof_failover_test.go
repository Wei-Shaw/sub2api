//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChatBufferedEOFReturnsBoundedFailoverBeforeWriting(t *testing.T) {
	for name, body := range map[string]string{
		"empty":           "",
		"created":         "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_test\",\"status\":\"in_progress\"}}\n\n",
		"partial content": "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\",\"output_index\":0,\"content_index\":0}\n\n",
	} {
		for mode, credentials := range map[string]map[string]any{
			"plain":               nil,
			"pool defaults":       {"pool_mode": true},
			"pool configured 502": {"pool_mode": true, "pool_mode_retry_status_codes": []any{float64(502)}},
		} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				c, rec := bufferedEOFTestContext()
				account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: credentials}
				result, err := (&OpenAIGatewayService{}).handleChatBufferedStreamingResponse(
					bufferedEOFTestResponse(body), c, account, "model", "model", "model", time.Now(),
				)
				require.Nil(t, result, "an incomplete buffered reply must not become billable success")
				var failoverErr *UpstreamFailoverError
				require.ErrorAs(t, err, &failoverErr)
				require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
				require.Equal(t, 1, failoverErr.SameAccountRetryMax)
				require.Equal(t, mode == "pool configured 502", failoverErr.RetryableOnSameAccount)
				_, _, eligible := classifyOpenAIAPIKeyHealthFailure(err)
				require.False(t, eligible, "an incomplete upstream stream is not a credential failure")
				require.False(t, c.Writer.Written())
				require.Empty(t, rec.Body.String())
			})
		}
	}
}

func TestForwardChatBufferedAnthropicEOFReturnsBoundedFailoverBeforeWriting(t *testing.T) {
	for name, stream := range map[string]string{
		"before message start":  "event: ping\ndata: {\"type\":\"ping\"}\n\n",
		"after unmetered start": "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-sonnet-4-5\"}}\n\n",
	} {
		for _, pool := range []bool{false, true} {
			t.Run(name+"/pool="+map[bool]string{false: "false", true: "true"}[pool], func(t *testing.T) {
				c, rec := bufferedEOFTestContext()
				account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
					Credentials: map[string]any{"api_key": "fixture-key", "pool_mode": pool, "pool_mode_retry_status_codes": []any{float64(502)}}}
				svc := &GatewayService{cfg: &config.Config{}, httpUpstream: &anthropicHTTPUpstreamRecorder{resp: bufferedEOFTestResponse(stream)}}
				result, err := svc.ForwardAsChatCompletions(context.Background(), c, account,
					[]byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}],"stream":false}`), nil)
				require.Nil(t, result)
				var failoverErr *UpstreamFailoverError
				require.ErrorAs(t, err, &failoverErr)
				require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
				require.Equal(t, pool, failoverErr.RetryableOnSameAccount)
				require.Equal(t, 1, failoverErr.SameAccountRetryMax)
				require.False(t, c.Writer.Written())
				require.Empty(t, rec.Body.String())
			})
		}
	}
}

func TestChatBufferedEOFKeepsMeteredAnthropicFailure(t *testing.T) {
	c, rec := bufferedEOFTestContext()
	stream := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"content\":[],\"usage\":{\"input_tokens\":5}}}\n\n"
	result, err := (&GatewayService{}).handleCCBufferedFromAnthropic(bufferedEOFTestResponse(stream), c, "model", "model", nil, time.Now())
	var failoverErr *UpstreamFailoverError
	require.NotErrorAs(t, err, &failoverErr)
	require.NotNil(t, result, "preserve settlement of already metered partial attempts")
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.True(t, c.Writer.Written())
}

func TestChatBufferedEOFDoesNotFailoverAfterOutput(t *testing.T) {
	for _, anthropic := range []bool{false, true} {
		c, _ := bufferedEOFTestContext()
		_, _ = c.Writer.WriteString("already sent")
		var err error
		if anthropic {
			_, err = (&GatewayService{}).handleCCBufferedFromAnthropic(bufferedEOFTestResponse(""), c, "model", "model", nil, time.Now())
		} else {
			_, err = (&OpenAIGatewayService{}).handleChatBufferedStreamingResponse(bufferedEOFTestResponse(""), c, &Account{ID: 1}, "model", "model", "model", time.Now())
		}
		var failoverErr *UpstreamFailoverError
		require.Error(t, err)
		require.NotErrorAs(t, err, &failoverErr)
	}
}

func bufferedEOFTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, rec
}

func bufferedEOFTestResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"upstream-test"}},
		Body:   io.NopCloser(strings.NewReader(body))}
}

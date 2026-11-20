package service

import (
	"bytes"
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

// Independent acceptance coverage exercises both scanner modes with a preamble
// larger than bufio.Writer's buffer, and a semantic reasoning boundary.
func TestEncryptedSSEAcceptanceScannerModesAndReasoning(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const failure = "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"id\":\"failed-attempt\",\"error\":{\"code\":\"invalid_encrypted_content\",\"message\":\"rejected\"}}}\n\n"
	const completed = "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"accepted-attempt\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
	for _, timeout := range []int{0, 10} {
		for _, reasoning := range []bool{false, true} {
			name := "sync"
			if timeout != 0 {
				name = "async"
			}
			if reasoning {
				name += "_reasoning_already_seen"
			}
			t.Run(name, func(t *testing.T) {
				body := []byte(`{"model":"gpt-5.5","stream":true,"previous_response_id":"preserve-me","input":{"type":"reasoning","encrypted_content":null,"summary":[{"type":"summary_text","text":"private-summary"}]}}`)
				preamble := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"failed-attempt\",\"padding\":\"" + strings.Repeat("p", 20000) + "\"}}\n\n"
				if reasoning {
					preamble += "data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"semantic reasoning\"}\n\n"
				}
				tracked := &passthroughCloseTrackingReadCloser{Reader: strings.NewReader(preamble + failure)}
				upstream := &httpUpstreamRecorder{responses: []*http.Response{
					{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}, "X-Codex-Turn-State": {"rejected-state"}}, Body: tracked},
					{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}, "X-Codex-Turn-State": {"accepted-state"}}, Body: io.NopCloser(strings.NewReader(completed))},
				}}
				svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAIFirstOutputTimeoutSeconds: timeout}}, httpUpstream: upstream}
				account := &Account{ID: 91, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
				result, err := svc.Forward(context.Background(), c, account, body)
				require.True(t, tracked.closed, "rejected upstream body must be closed")
				if reasoning {
					require.Error(t, err)
					require.Len(t, upstream.bodies, 1)
					require.Contains(t, rec.Body.String(), "semantic reasoning")
					return
				}
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Len(t, upstream.bodies, 2)
				require.NotContains(t, rec.Body.String(), "failed-attempt")
				require.NotContains(t, rec.Body.String(), strings.Repeat("p", 100))
				require.Equal(t, "accepted-state", rec.Header().Get("X-Codex-Turn-State"))
				require.Contains(t, string(upstream.bodies[1]), `"previous_response_id":"preserve-me"`)
				require.Contains(t, string(upstream.bodies[1]), "private-summary")
				require.NotContains(t, string(upstream.bodies[1]), "encrypted_content")
			})
		}
	}
}

// The Forward fallback replays the captured terminal without recovery enabled.
// A cancellation arriving between signal and replay must still produce an error.
func TestEncryptedSSEAcceptanceCanceledTerminalReplay(t *testing.T) {
	for _, timeout := range []int{0, 10} {
		for _, event := range []string{"response.failed", "error"} {
			payload := `{"type":"error","code":"invalid_encrypted_content","message":"rejected"}`
			if event == "response.failed" {
				payload = `{"type":"response.failed","response":{"status":"failed","error":{"type":"invalid_request_error","code":"invalid_encrypted_content","message":"rejected"}}}`
			}
			svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAIFirstOutputTimeoutSeconds: timeout}}}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("event: " + event + "\ndata: " + payload + "\n\n"))}
			_, err := svc.handleStreamingResponseWithReasoning(ctx, resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, time.Now(), "model", "model", "")
			require.Error(t, err, "canceled terminal replay must never return nil error (event=%s timeout=%d)", event, timeout)
			require.Contains(t, rec.Body.String(), "response.failed")
		}
	}
}

func TestEncryptedSSEAcceptanceSecondUntypedFailurePreservesTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.5","stream":true,"input":[{"type":"reasoning","encrypted_content":"opaque-cipher"},{"role":"user","content":"hello"}]}`)
	const terminal = "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"id\":\"original-failure\",\"status\":\"failed\",\"error\":{\"code\":\"invalid_encrypted_content\",\"message\":\"Encrypted reasoning rejected\"}}}\n\n"
	response := func() *http.Response {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(terminal))}
	}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{response(), response()}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 91, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	_, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Len(t, upstream.bodies, 2)
	var failover *UpstreamFailoverError
	require.NotErrorAs(t, err, &failover, "exhausted encrypted-content recovery must preserve the original terminal instead of failing over")
	require.Contains(t, rec.Body.String(), "original-failure")
	require.Contains(t, rec.Body.String(), "invalid_encrypted_content")
}

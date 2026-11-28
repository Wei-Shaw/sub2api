package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestForwardEncryptedContentSSERetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const failed = "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_bad\",\"status\":\"failed\",\"error\":{\"type\":\"invalid_request_error\",\"code\":\"invalid_encrypted_content\",\"message\":\"Encrypted reasoning rejected\"}}}\n\n"
	const created = "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_bad\",\"model\":\"bad-model\"}}\n\n"
	const completed = "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_good\",\"status\":\"completed\",\"model\":\"gpt-5.5\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
	const bare = "event: error\ndata: {\"type\":\"error\",\"code\":\"invalid_encrypted_content\",\"message\":\"Encrypted reasoning rejected\"}\n\n"
	cases := []struct {
		name, first, second                               string
		httpFirst, noCipher, canceled, committed, managed bool
		calls                                             int
		success                                           bool
	}{
		{name: "created then failed", first: created + failed, second: completed, calls: 2, success: true},
		{name: "bare error", first: bare, second: completed, calls: 2, success: true},
		{name: "managed proxy", first: created + failed, second: completed, calls: 2, success: true, managed: true},
		{name: "text already delivered", first: "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n" + failed, calls: 1},
		{name: "tool already delivered", first: "event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"name\":\"run\",\"call_id\":\"call_1\",\"arguments\":\"{}\"}}\n\n" + failed, calls: 1},
		{name: "second SSE fails", first: failed, second: failed, calls: 2},
		{name: "no encrypted item", first: created + failed, noCipher: true, calls: 1},
		{name: "different code", first: strings.ReplaceAll(failed, "invalid_encrypted_content", "invalid_argument"), calls: 1},
		{name: "HTTP then SSE shares budget", first: `{"error":{"code":"invalid_encrypted_content","message":"Encrypted reasoning rejected"}}`, httpFirst: true, second: failed, calls: 2},
		{name: "SSE then HTTP shares budget", first: failed, second: `{"error":{"code":"invalid_encrypted_content","message":"Encrypted reasoning rejected"}}`, calls: 2},
		{name: "canceled", first: failed, canceled: true, calls: 1},
		{name: "already committed", first: failed, committed: true, calls: 1},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			input := `[{"type":"reasoning","id":"rs_1","encrypted_content":"cipher-secret","summary":[{"type":"summary_text","text":"keep-summary"}]},{"role":"user","content":"hello"},{"type":"compaction_trigger"}]`
			if tt.noCipher {
				input = `[{"role":"user","content":"hello"}]`
			}
			body := []byte(`{"model":"gpt-5.5","stream":true,"instructions":"retry-test","input":` + input + `}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			if tt.committed {
				_, err := c.Writer.WriteString(": committed\n\n")
				require.NoError(t, err)
				c.Writer.Flush()
			}
			response := func(data, state string, status int) *http.Response {
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Codex-Turn-State": []string{state}, "X-Request-Id": []string{state}}, Body: io.NopCloser(strings.NewReader(data))}
			}
			status := http.StatusOK
			if tt.httpFirst {
				status = http.StatusBadRequest
			}
			upstream := &httpUpstreamRecorder{responses: []*http.Response{response(tt.first, "bad-state", status)}}
			if tt.second != "" {
				status = http.StatusOK
				if strings.HasPrefix(tt.second, "{") {
					status = http.StatusBadRequest
				}
				upstream.responses = append(upstream.responses, response(tt.second, "good-state", status))
			}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"}, Status: StatusActive, Schedulable: true}
			if tt.managed {
				account, _ = compactFallbackManagedProxyAccount()
			}
			ctx := context.Background()
			if tt.canceled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			result, err := svc.Forward(ctx, c, account, body)
			require.Len(t, upstream.bodies, tt.calls)
			if tt.success {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Contains(t, recorder.Body.String(), "resp_good")
				require.NotContains(t, recorder.Body.String(), "resp_bad")
				require.NotContains(t, recorder.Body.String(), "invalid_encrypted_content")
				require.Equal(t, "good-state", recorder.Header().Get("X-Codex-Turn-State"))
				require.Equal(t, "gpt-5.5", result.UpstreamResponseModel)
			} else {
				require.Error(t, err)
				if !tt.httpFirst && !strings.HasPrefix(tt.second, "{") {
					require.Contains(t, recorder.Body.String(), "response.failed")
				}
			}
			if tt.calls == 2 {
				require.Contains(t, string(upstream.bodies[0]), "cipher-secret")
				require.NotContains(t, string(upstream.bodies[1]), "cipher-secret")
				require.Contains(t, string(upstream.bodies[1]), "keep-summary")
				require.Contains(t, string(upstream.bodies[1]), "compaction_trigger")
				require.NotContains(t, string(upstream.bodies[1]), `"role":"developer"`)
				raw, ok := c.Get(OpsUpstreamErrorsKey)
				require.True(t, ok)
				events := raw.([]*OpsUpstreamErrorEvent)
				require.Equal(t, "retry", events[0].Kind)
				require.Equal(t, "invalid_encrypted_content", events[0].Reason)
				if tt.managed {
					require.NotNil(t, events[0].ProxyID)
					require.Equal(t, *account.ProxyID, *events[0].ProxyID)
					require.Equal(t, account.Proxy.Name, events[0].ProxyName)
				} else {
					require.Nil(t, events[0].ProxyID)
					require.Equal(t, opsProxyNameDirect, events[0].ProxyName)
				}
				require.Empty(t, events[0].Detail)
			}
		})
	}
}

func TestInvalidEncryptedContentSSEStrictClassification(t *testing.T) {
	for _, tt := range []struct {
		code, message string
		want          bool
	}{
		{"invalid_encrypted_content", "", true},
		{"thinking_signature_invalid", "The encrypted content could not be verified; could not be decrypted or parsed", true},
		{"thinking_signature_invalid", "could not be verified", false},
		{"invalid_request_error", "invalid_encrypted_content", false},
	} {
		t.Run(tt.code+tt.message, func(t *testing.T) {
			require.Equal(t, tt.want, isOpenAIInvalidEncryptedContentError(tt.code, tt.message))
		})
	}
}

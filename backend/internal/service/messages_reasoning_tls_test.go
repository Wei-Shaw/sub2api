package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Use the TLS server's trusting client, not a synthetic HTTP response or a
// transport with certificate verification disabled.
type reasoningTLSUpstream struct {
	client *http.Client
}

var _ HTTPUpstream = (*reasoningTLSUpstream)(nil)

func (u *reasoningTLSUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.client.Do(req)
}

func (u *reasoningTLSUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func TestForwardAsAnthropicTLSReasoningRetry(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			const native = "CAQS-native-tls"
			const legacy = "gAAAA-stale-tls"
			const token = "dummy-local-tls-api-key"
			body := []byte(fmt.Sprintf(`{"model":"claude-opus-5-5","max_tokens":32,"stream":%v,"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{}}}],"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":[{"type":"thinking","signature":%q},{"type":"thinking","signature":%q},{"type":"text","text":"visible"},{"type":"tool_use","id":"toolu_tls_1","name":"lookup","input":{"n":9007199254740993}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_tls_1","content":"result"}]}]}`, stream, native, legacy))

			// Handler assertions must be non-fatal: net/http runs the handler in
			// another goroutine. The mutex also makes the final count race-free.
			var mu sync.Mutex
			var requests int
			var firstHeaders http.Header
			var firstHistory []string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				requests++
				assert.NotNil(t, r.TLS, "must traverse a real TLS connection")
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/v1/responses", r.URL.Path)
				assert.Empty(t, r.URL.RawQuery)
				assert.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				sent, err := io.ReadAll(r.Body)
				if !assert.NoError(t, err) {
					http.Error(w, "cannot read test request", http.StatusInternalServerError)
					return
				}
				assert.True(t, gjson.ValidBytes(sent))
				assert.Equal(t, "gpt-6.1-sol", gjson.GetBytes(sent, "model").String())
				assert.True(t, gjson.GetBytes(sent, "stream").Bool(), "upstream is SSE even for a non-streaming client")
				assert.NotContains(t, string(sent), native)

				var history []string
				reasoningCount, callCount, outputCount := 0, 0, 0
				for _, item := range gjson.GetBytes(sent, "input").Array() {
					switch item.Get("type").String() {
					case "reasoning":
						reasoningCount++
						assert.Equal(t, legacy, item.Get("encrypted_content").String())
						assert.Equal(t, "[]", item.Get("summary").Raw)
						continue
					case "function_call":
						callCount++
						assert.Equal(t, "toolu_tls_1", item.Get("call_id").String())
						assert.Equal(t, "lookup", item.Get("name").String())
						assert.JSONEq(t, `{"n":9007199254740993}`, item.Get("arguments").String())
					case "function_call_output":
						outputCount++
						assert.Equal(t, "toolu_tls_1", item.Get("call_id").String())
						assert.Equal(t, "result", item.Get("output").String())
					}
					assert.False(t, item.Get("summary").Exists())
					history = append(history, item.Raw)
				}
				assert.Equal(t, 1, callCount)
				assert.Equal(t, 1, outputCount)
				assert.Contains(t, string(sent), "hello")
				assert.Contains(t, string(sent), "visible")
				assert.Contains(t, string(sent), "9007199254740993")

				if requests == 1 {
					assert.Equal(t, 1, reasoningCount)
					firstHeaders = r.Header.Clone()
					firstHistory = history
					assert.NotEmpty(t, firstHeaders.Get("session_id"))
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadGateway)
					_, _ = io.WriteString(w, `{"error":{"code":"invalid_encrypted_content","message":"could not verify test ciphertext"}}`)
					return
				}
				assert.Equal(t, 2, requests, "exactly one retry is permitted")
				assert.Zero(t, reasoningCount)
				assert.NotContains(t, string(sent), legacy)
				// Retry re-encoding may change JSON HTML escapes, not history.
				assert.JSONEq(t, "["+strings.Join(firstHistory, ",")+"]", "["+strings.Join(history, ",")+"]", "retry must preserve all non-reasoning history in order")
				for _, header := range []string{"session_id", "conversation_id", "User-Agent", "originator", "OpenAI-Beta", "Chatgpt-Account-Id", "x-codex-turn-state", "Authorization"} {
					assert.Equal(t, firstHeaders.Values(header), r.Header.Values(header), header)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("x-request-id", "rid_local_tls")
				_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_local_tls\",\"object\":\"response\",\"model\":\"gpt-6.1-sol\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"id\":\"msg_tls\",\"role\":\"assistant\",\"status\":\"completed\",\"content\":[{\"type\":\"output_text\",\"text\":\"tls-recovered\"}]}],\"usage\":{\"input_tokens\":5,\"output_tokens\":2,\"total_tokens\":7}}}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			client := server.Client()
			client.Timeout = 5 * time.Second
			defer client.CloseIdleConnections()

			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = true
			cfg.Security.URLAllowlist.UpstreamHosts = []string{"127.0.0.1", "localhost"}
			cfg.Security.URLAllowlist.AllowPrivateHosts = true
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: &reasoningTLSUpstream{client: client}}
			// Exercise the same HTTPS validator used when building the actual
			// upstream request; neither the allowlist nor HTTPS safety is bypassed.
			_, err := svc.validateUpstreamBaseURL(server.URL)
			require.NoError(t, err)
			_, err = svc.validateUpstreamBaseURL(strings.Replace(server.URL, "https://", "http://", 1))
			require.Error(t, err)
			_, err = svc.validateUpstreamBaseURL("https://outside-allowlist.invalid")
			require.Error(t, err)

			account := &Account{
				ID: 101, Name: "local-tls-only", Platform: PlatformOpenAI,
				Type: AccountTypeAPIKey, Concurrency: 1,
				Credentials: map[string]any{"api_key": token, "base_url": server.URL},
			}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body)).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("session_id", "tls-client-session")
			c.Request.Header.Set("conversation_id", "tls-client-conversation")
			c.Request.Header.Set("x-codex-turn-state", "tls-stable-turn-state")
			result, err := svc.ForwardAsAnthropic(ctx, c, account, body, "tls-stable-cache-key", "gpt-6.1-sol")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Body.String(), "tls-recovered")
			require.NotContains(t, rec.Body.String(), "invalid_encrypted_content")
			require.NotContains(t, rec.Body.String(), "could not verify test ciphertext")
			if stream {
				require.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")
				require.Contains(t, rec.Body.String(), "event: message_stop")
			} else {
				require.True(t, gjson.ValidBytes(rec.Body.Bytes()))
				require.Equal(t, "message", gjson.GetBytes(rec.Body.Bytes(), "type").String())
				require.Equal(t, "tls-recovered", gjson.GetBytes(rec.Body.Bytes(), "content.0.text").String())
			}
			mu.Lock()
			gotRequests := requests
			mu.Unlock()
			require.Equal(t, 2, gotRequests)
		})
	}
}

//go:build unit

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

// Fixture field numbers are from the pinned upstream devin-proto.ts, independent
// of the production codec, so these tests exercise the actual gateway boundary.
func devinTestString(n protowire.Number, value string) []byte {
	return protowire.AppendString(protowire.AppendTag(nil, n, protowire.BytesType), value)
}
func devinTestInt(n protowire.Number, value uint64) []byte {
	return protowire.AppendVarint(protowire.AppendTag(nil, n, protowire.VarintType), value)
}
func devinTestFrame(flag byte, payload []byte) []byte {
	frame := make([]byte, 5, 5+len(payload))
	frame[0] = flag
	binary.BigEndian.PutUint32(frame[1:], uint32(len(payload)))
	return append(frame, payload...)
}
func devinTestOutput(tool bool, trailer string) []byte {
	msg := append(devinTestString(1, "bot-upstream-id"), devinTestString(3, "Hello from Devin.")...)
	if tool {
		call := append(devinTestString(1, "call_1"), devinTestString(2, "lookup")...)
		call = append(call, devinTestString(3, `{"query":"hello"}`)...)
		msg = append(msg, devinTestString(6, string(call))...)
	}
	usage := append(devinTestInt(2, 10), devinTestInt(3, 4)...)
	usage = append(usage, devinTestInt(4, 2)...)
	usage = append(usage, devinTestInt(5, 3)...)
	msg = append(msg, devinTestString(7, string(usage))...)
	msg = append(msg, devinTestInt(5, 2)...)
	result := devinTestFrame(0, msg)
	if trailer != "" {
		result = append(result, devinTestFrame(2, []byte(trailer))...)
	}
	return result
}

type devinGatewayUpstream struct {
	t       *testing.T
	chat    []byte
	status  int
	calls   []string
	proxies []string
}

func (u *devinGatewayUpstream) Do(req *http.Request, proxy string, id int64, concurrency int) (*http.Response, error) {
	u.calls = append(u.calls, req.URL.Path)
	u.proxies = append(u.proxies, proxy)
	require.Equal(u.t, int64(71), id)
	require.Equal(u.t, 2, concurrency)
	require.Empty(u.t, req.Header.Get("Authorization"), "token belongs in protobuf, not a forwarded Bearer header")
	require.True(u.t, HTTPUpstreamRedirectsDisabled(req.Context()), "protobuf credentials must never be replayed to a redirect target")
	require.True(u.t, HTTPUpstreamPublicHostsOnly(req.Context()), "provider hosts must not resolve to private addresses")
	status := http.StatusOK
	payload := u.chat
	contentType := "application/connect+proto"
	if strings.HasSuffix(req.URL.Path, "/GetUserJwt") {
		payload = devinTestString(1, "user-jwt")
		contentType = "application/proto"
		if u.status != 0 {
			status = u.status
			payload = []byte(`{"error":"do not echo private payload"}`)
		}
	} else {
		require.Equal(u.t, devin.ChatPath, req.URL.Path)
		require.Equal(u.t, "application/connect+proto", req.Header.Get("Content-Type"))
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(bytes.NewReader(payload))}, nil
}
func (u *devinGatewayUpstream) DoWithTLS(req *http.Request, proxy string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, id, concurrency)
}

func devinGatewayTestSetup(t *testing.T, body []byte, path string, output []byte) (*OpenAIGatewayService, *Account, *gin.Context, *httptest.ResponseRecorder, *devinGatewayUpstream) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 23})
	account := &Account{ID: 71, Platform: PlatformDevin, Type: AccountTypeOAuth, Concurrency: 2, Credentials: map[string]any{"access_token": "test-session-token"}}
	upstream := &devinGatewayUpstream{t: t, chat: output}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	svc.devinModelCache.Store(account.ID, devinModelCacheEntry{credential: sha256.Sum256([]byte("test-session-token")), expires: time.Now().Add(time.Minute), models: devin.DefaultModels()})
	return svc, account, c, rec, upstream
}

func TestDevinGatewayProtocols(t *testing.T) {
	for _, protocol := range []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic} {
		for _, stream := range []bool{false, true} {
			for _, tool := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%t/tool=%t", protocol, stream, tool), func(t *testing.T) {
					body := []byte(fmt.Sprintf(`{"model":"swe-1-6","messages":[{"role":"user","content":"hello"}],"stream":%t,"max_tokens":64}`, stream))
					if protocol == APIProtocolResponses {
						body = []byte(fmt.Sprintf(`{"model":"swe-1-6","input":"hello","stream":%t,"max_output_tokens":64}`, stream))
					}
					svc, account, c, rec, upstream := devinGatewayTestSetup(t, body, "/v1/"+protocol, devinTestOutput(tool, `{}`))
					var result *OpenAIForwardResult
					var err error
					switch protocol {
					case APIProtocolResponses:
						result, err = svc.Forward(c.Request.Context(), c, account, body)
					case APIProtocolAnthropic:
						result, err = svc.ForwardAsAnthropic(c.Request.Context(), c, account, body, "", "")
					default:
						result, err = svc.ForwardAsChatCompletions(c.Request.Context(), c, account, body, "", "")
					}
					require.NoError(t, err, rec.Body.String())
					require.Equal(t, http.StatusOK, rec.Code)
					require.Contains(t, rec.Body.String(), "Hello from Devin.")
					if tool {
						require.Contains(t, rec.Body.String(), "lookup")
						require.Contains(t, rec.Body.String(), "call_1")
					}
					require.NotNil(t, result)
					require.Equal(t, 15, result.Usage.InputTokens)
					require.Equal(t, 4, result.Usage.OutputTokens)
					require.Equal(t, 3, result.Usage.CacheReadInputTokens)
					require.Equal(t, 2, result.Usage.CacheCreationInputTokens)
					require.Equal(t, devin.ChatPath, result.UpstreamEndpoint)
					require.Equal(t, devin.ChatPath, GetActualOpenAIUpstreamEndpoint(c))
					require.Len(t, upstream.calls, 2)
					require.NotContains(t, rec.Body.String(), "test-session-token")
				})
			}
		}
	}
}

func TestDevinConnectionTestStreamsWithSSEHeaders(t *testing.T) {
	svc, account, c, rec, upstream := devinGatewayTestSetup(t, nil, "/accounts/71/test", devinTestOutput(false, `{}`))
	tester := &AccountTestService{openaiGatewayService: svc}
	require.NoError(t, tester.testDevinAccountConnection(c, account, "", ""))
	require.Equal(t, "text/event-stream", rec.Result().Header.Get("Content-Type"))
	require.Equal(t, "no-cache", rec.Result().Header.Get("Cache-Control"))
	require.Equal(t, "no", rec.Result().Header.Get("X-Accel-Buffering"))
	require.Contains(t, rec.Body.String(), `"type":"test_complete","success":true`)
	require.Contains(t, rec.Body.String(), "Hello from Devin.")
	require.Len(t, upstream.calls, 2)
}

func TestDevinGatewayFailures(t *testing.T) {
	body := []byte(`{"model":"swe-1-6","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	t.Run("expired credential fails over without network", func(t *testing.T) {
		svc, account, c, _, upstream := devinGatewayTestSetup(t, body, "/v1/chat/completions", nil)
		account.Credentials["access_token"] = "e30.eyJleHAiOjF9.sig"
		_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
		var failover *UpstreamFailoverError
		require.ErrorAs(t, err, &failover)
		require.Equal(t, http.StatusUnauthorized, failover.StatusCode)
		require.Empty(t, upstream.calls)
	})
	t.Run("truncated stream never succeeds", func(t *testing.T) {
		svc, account, c, rec, _ := devinGatewayTestSetup(t, body, "/v1/chat/completions", devinTestOutput(false, ""))
		_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
		require.Error(t, err)
		require.NotContains(t, rec.Body.String(), "[DONE]")
	})
	t.Run("early rate limit trailer fails over", func(t *testing.T) {
		svc, account, c, rec, _ := devinGatewayTestSetup(t, body, "/v1/chat/completions", devinTestFrame(2, []byte(`{"error":{"code":"resource_exhausted","message":"limit"}}`)))
		_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
		var failover *UpstreamFailoverError
		require.ErrorAs(t, err, &failover)
		require.Equal(t, http.StatusTooManyRequests, failover.StatusCode)
		require.NotContains(t, rec.Body.String(), "[DONE]")
	})
	t.Run("unsupported input rejected before network", func(t *testing.T) {
		request := []byte(`{"model":"swe-1-6","previous_response_id":"private-history","input":"hello"}`)
		svc, account, c, rec, upstream := devinGatewayTestSetup(t, request, "/v1/responses", nil)
		_, err := svc.Forward(context.Background(), c, account, request)
		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Empty(t, upstream.calls)
	})
}

func TestDevinGatewayProxyAndLocalSessionKey(t *testing.T) {
	body := []byte(`{"model":"swe-1-6","prompt_cache_key":"client-session","messages":[{"role":"user","content":"hello"}]}`)
	svc, account, c, rec, upstream := devinGatewayTestSetup(t, body, "/v1/chat/completions", devinTestOutput(false, `{}`))
	proxyID := int64(7)
	account.ProxyID = &proxyID
	account.Proxy = &Proxy{Protocol: "http", Host: "proxy.example", Port: 8080, Username: "test", Password: "private"}
	_, err := svc.ForwardAsChatCompletions(c.Request.Context(), c, account, body, "handler-session", "")
	require.NoError(t, err)
	require.Equal(t, []string{account.Proxy.URL(), account.Proxy.URL()}, upstream.proxies)
	require.Equal(t, isolateDevinCascadeID(c, account, "handler-session"), c.GetString(devinCascadeContextKey))
	require.NotContains(t, rec.Body.String(), "private")
}

func TestDevinCascadeIsolationAndURLPolicy(t *testing.T) {
	body := []byte(`{"model":"swe-1-6","messages":[{"role":"user","content":"hello"}]}`)
	_, account, c, _, _ := devinGatewayTestSetup(t, body, "/v1/chat/completions", nil)
	c.Request.Header.Set("session_id", "same-client-session")
	first := devinCascadeID(c, account, body)
	require.Equal(t, first, devinCascadeID(c, account, []byte(`{"messages":[]}`)))
	c.Set("api_key", &APIKey{ID: 24})
	require.NotEqual(t, first, devinCascadeID(c, account, body))
	c.Set("api_key", &APIKey{ID: 23})
	account.ID++
	require.NotEqual(t, first, devinCascadeID(c, account, body))
	for _, raw := range []string{"http://server.codeium.com", "https://127.0.0.1", "https://evil.example", "https://server.codeium.com.evil.example", "https://user:secret@server.codeium.com", "https://server.codeium.com/path", "https://server.codeium.com?token=secret", "https://server.codeium.com:8080"} {
		_, err := validateDevinBaseURL(raw, nil)
		require.Error(t, err, raw)
		require.NotContains(t, err.Error(), "secret")
	}
	value, err := validateDevinBaseURL("https://region.codeium.com/", nil)
	require.NoError(t, err)
	require.Equal(t, "https://region.codeium.com", value)
}

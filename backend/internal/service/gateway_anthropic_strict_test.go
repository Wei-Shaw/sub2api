package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Includes the exact shapes that used to be removed: implicit thinking,
// redacted data, empty text, deferred cache markers and beta-gated fields.
const strictAnthropicBody = `{
 "model":"claude-fable-5-1", "max_tokens":4096,
 "messages":[{"role":"user","content":[{"type":"text","text":""},{"type":"text","text":"hi"}]},{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"opaque_cc_ses_signature"},{"type":"redacted_thinking","data":"opaque_cc_sess_data"},{"type":"tool_use","id":"tool_1","name":"cc_sess_audit","input":{"text":"cc_ses_marker","number":9007199254740993}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool_1","content":[{"type":"text","text":""},{"type":"text","text":"ok"}]}]}],
 "tools":[{"name":"cc_sess_audit","defer_loading":true,"cache_control":{"type":"ephemeral"},"input_schema":{"type":"object"}}],
 "context_management":{"future":"keep"},"fallbacks":["future"],"fallback_credit_token":"opaque","future_extension":{"keep":true}
}`

func TestStrictAnthropicRequestBytesHeadersAndQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"/messages", "/messages/count_tokens"} {
		t.Run(endpoint, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1"+endpoint+"?beta=false&future=a%2Fb&future=two", nil)
			c.Request.Header.Set("Authorization", "Bearer caller-secret")
			c.Request.Header.Set("Accept-Encoding", "gzip, br")
			c.Request.Header.Set("Cookie", "caller-session")
			c.Request.Header.Set("Cookie2", "caller-session")
			c.Request.Header.Set("Connection", "X-Connection-Only")
			c.Request.Header.Set("X-Connection-Only", "remove")
			c.Request.Header["X-Future-Extension"] = []string{"one", "two"}
			c.Request.Header.Set("Anthropic-Beta", "unknown-beta-must-reach-provider")
			svc := &GatewayService{cfg: &config.Config{}}
			account := newAnthropicAPIKeyAccountForTest()
			account.Credentials[credKeyHeaderOverrideEnabled] = true
			account.Credentials[credKeyHeaderOverrides] = map[string]any{"anthropic-beta": "must-not-override", "x-future-extension": "must-not-override"}
			var req *http.Request
			var err error
			if endpoint == "/messages" {
				var wire []byte
				req, wire, err = svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, []byte(strictAnthropicBody), "upstream-secret")
				require.Equal(t, strictAnthropicBody, string(wire))
			} else {
				req, err = svc.buildCountTokensRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, []byte(strictAnthropicBody), "upstream-secret")
			}
			require.NoError(t, err)
			wire, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, strictAnthropicBody, string(wire))
			require.Equal(t, c.Request.URL.RawQuery, req.URL.RawQuery)
			require.True(t, HTTPUpstreamRedirectsDisabled(req.Context()))
			require.Equal(t, "/v1"+endpoint, req.URL.Path)
			require.Equal(t, "identity", req.Header.Get("Accept-Encoding"))
			require.Equal(t, []string{"one", "two"}, req.Header.Values("X-Future-Extension"))
			require.Equal(t, "unknown-beta-must-reach-provider", req.Header.Get("Anthropic-Beta"))
			require.Equal(t, "upstream-secret", getHeaderRaw(req.Header, "x-api-key"))
			require.Empty(t, req.Header.Get("Authorization"))
			require.Empty(t, req.Header.Get("Cookie"))
			require.Empty(t, req.Header.Get("Cookie2"))
			require.Empty(t, req.Header.Get("X-Connection-Only"))
		})
	}
}

func TestStrictAnthropicForwardPreservesImplicitThinkingAndResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "sse"}[stream], func(t *testing.T) {
			payload := `{"type":"message","content":[{"type":"thinking","thinking":"","signature":"cc_ses_opaque"},{"type":"tool_use","name":"cc_sess_audit","input":{"text":"cc_ses_marker"}}],"usage":{"input_tokens":11,"output_tokens":7}}`
			contentType := "application/json"
			if stream {
				contentType = "text/event-stream"
				payload = ": keep this comment\r\nid: 7\r\nevent: message_start\r\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":11}}}\r\n\r\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"signature_delta\",\"signature\":\"cc_ses_opaque\"}}\r\n\r\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":7}}\r\n\r\ndata: {\"type\":\"message_stop\"}"
			}
			upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{
				"Content-Type": {contentType}, "X-Future-Response": {"preserve"}, "X-Request-Id": {"provider-first", "provider-second"},
				"Authorization": {"Bearer provider-secret"}, "X-Api-Key": {"provider-secret"}, "X-Goog-Api-Key": {"provider-secret"},
				"Cookie": {"provider-session"}, "Cookie2": {"provider-session"}, "Set-Cookie": {"provider-session"}, "Set-Cookie2": {"provider-session"},
			}, Body: io.NopCloser(strings.NewReader(payload))}}
			svc := newForwardPartialUsageServiceForTest(upstream)
			account := newAnthropicAPIKeyAccountForTest()
			account.Credentials["model_mapping"] = map[string]any{"claude-fable-5-1": "changed-model"}
			parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(strictAnthropicBody)), PlatformAnthropic)
			require.NoError(t, err)
			parsed.Stream = stream
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			result, err := svc.Forward(context.Background(), c, account, parsed)
			require.NoError(t, err)
			require.Equal(t, strictAnthropicBody, string(upstream.lastBody))
			require.Equal(t, payload, rec.Body.String())
			require.Equal(t, "preserve", rec.Header().Get("X-Future-Response"))
			require.Equal(t, []string{"provider-first", "provider-second"}, rec.Header().Values("X-Request-Id"))
			for _, header := range []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key", "Cookie", "Cookie2", "Set-Cookie", "Set-Cookie2"} {
				require.Empty(t, rec.Header().Get(header), header)
			}
			require.Equal(t, 11, result.Usage.InputTokens)
			require.Equal(t, 7, result.Usage.OutputTokens)
		})
	}
}

func TestStrictAnthropicErrorsRemainProviderErrors(t *testing.T) {
	for _, status := range []int{301, 307, 400, 401, 403, 404, 429, 500, 529} {
		for _, count := range []bool{false, true} {
			payload := "provider error cc_ses_marker\r\n"
			upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"text/plain"}, "Retry-After": {"17"}}, Body: io.NopCloser(strings.NewReader(payload))}}
			svc := newForwardPartialUsageServiceForTest(upstream)
			svc.rateLimitService = nil
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			account := newAnthropicAPIKeyAccountForTest()
			if count {
				require.NoError(t, svc.forwardCountTokensAnthropicAPIKeyPassthrough(context.Background(), c, account, []byte(strictAnthropicBody)))
			} else {
				_, err := svc.forwardAnthropicAPIKeyPassthrough(context.Background(), c, account, []byte(strictAnthropicBody), "claude-fable-5-1", "claude-fable-5-1", false, time.Now())
				require.Error(t, err)
				var failover *UpstreamFailoverError
				require.NotErrorAs(t, err, &failover)
			}
			require.Equal(t, status, rec.Code)
			require.Equal(t, payload, rec.Body.String())
			require.Equal(t, "17", rec.Header().Get("Retry-After"))
		}
	}
}

func TestStrictAnthropicToolRestorationOnlyTouchesMappedNameFields(t *testing.T) {
	body := []byte(`{"content":[{"type":"tool_use","name":"cc_sess_audit","input":{"text":"cc_ses_marker","name":"cc_sess_audit","number":9007199254740993}},{"type":"text","text":"cc_sess_audit"},{"type":"thinking","thinking":"cc_sess_audit","signature":"cc_sess_audit"},{"type":"redacted_thinking","data":"cc_sess_audit"}]}`)
	require.Equal(t, body, restoreToolNamesInBytes(body, nil))
	rw := &ToolNameRewrite{Reverse: map[string]string{"cc_sess_audit": "sessions_audit"}}
	out := restoreToolNamesInBytes(body, rw)
	require.Equal(t, "sessions_audit", gjson.GetBytes(out, "content.0.name").String())
	for _, path := range []string{"content.0.input", "content.1", "content.2", "content.3"} {
		require.Equal(t, gjson.GetBytes(body, path).Raw, gjson.GetBytes(out, path).Raw)
	}
	line := []byte("data: {\"type\":\"content_block_start\",\"content_block\":{\"type\":\"tool_use\",\"name\":\"cc_sess_audit\",\"input\":{\"text\":\"cc_sess_audit\"}}}\r\n")
	want := strings.Replace(string(line), `"name":"cc_sess_audit"`, `"name":"sessions_audit"`, 1)
	require.Equal(t, want, string(restoreToolNamesInBytes(line, rw)))
}

func TestStrictAnthropicAuxiliaryPreservesMultipartAndBinary(t *testing.T) {
	for _, endpoint := range []string{"/models", "/models/claude-fable-5-1", "/files", "/files/file_test/content"} {
		payload := "\x00\xffcc_ses_marker\r\n"
		request := "--boundary\r\nContent-Disposition: form-data; name=\"file\"; filename=\"test.txt\"\r\n\r\ncc_sess_payload\r\n--boundary--\r\n"
		upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/octet-stream"}}, Body: io.NopCloser(strings.NewReader(payload))}}
		svc := newForwardPartialUsageServiceForTest(upstream)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1"+endpoint+"?after_id=a%2Fb", strings.NewReader(request))
		c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
		err := svc.ForwardAnthropicAuxiliary(context.Background(), c, newAnthropicAPIKeyAccountForTest(), endpoint)
		require.NoError(t, err)
		require.Equal(t, request, string(upstream.lastBody))
		require.Equal(t, payload, rec.Body.String())
		require.Equal(t, "after_id=a%2Fb", upstream.lastReq.URL.RawQuery)
		require.Equal(t, "multipart/form-data; boundary=boundary", upstream.lastReq.Header.Get("Content-Type"))
	}
}

type strictConfiguredAccounts struct {
	AccountRepository
	accounts []Account
}

func (r *strictConfiguredAccounts) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]Account, error) {
	return r.accounts, nil
}
func TestStrictAnthropicAuxiliaryDoesNotSwitchNamespacesOnCooldown(t *testing.T) {
	id := int64(1)
	account := newAnthropicAPIKeyAccountForTest()
	second := *account
	second.ID++
	second.Schedulable = false
	repo := &strictConfiguredAccounts{accounts: []Account{*account, second}}
	svc := &GatewayService{accountRepo: repo}
	selected, err := svc.SingleAnthropicPassthroughAccount(context.Background(), &id)
	require.NoError(t, err)
	require.Nil(t, selected)
	repo.accounts = repo.accounts[:1]
	selected, err = svc.SingleAnthropicPassthroughAccount(context.Background(), &id)
	require.NoError(t, err)
	require.Equal(t, account.ID, selected.ID)
	repo.accounts[0].Schedulable = false
	_, err = svc.SingleAnthropicPassthroughAccount(context.Background(), &id)
	require.Error(t, err)
}

func TestStrictAnthropicParserDoesNotNormalizeModelSuffix(t *testing.T) {
	body := []byte(`{ "model":"claude-sonnet-4-6[1m]", "max_tokens":16, "messages":[] }`)
	parsed, err := ParseGatewayRequestPreservingBody(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)
	require.Equal(t, "claude-sonnet-4-6", parsed.Model)
	require.Equal(t, body, parsed.Body.Bytes())
	require.NoError(t, parsed.ReplaceBody(body))
	require.Equal(t, body, parsed.Body.Bytes())
}

func TestStrictAnthropicHeadersReplaceCaseInsensitiveLocalValues(t *testing.T) {
	dst := http.Header{"X-Request-Id": {"local"}, "X-REQUEST-ID": {"stale"}, "X-Multi": {"local"}}
	src := http.Header{"x-request-id": {"provider"}, "x-multi": {"one", "two"}}
	copyAnthropicEndToEndHeaders(dst, src, false)
	require.Equal(t, []string{"provider"}, dst.Values("X-Request-ID"))
	require.Equal(t, []string{"one", "two"}, dst.Values("X-Multi"))
	require.Len(t, dst, 2)
}

func TestStrictAnthropicValidationIgnoresAccountModelMapping(t *testing.T) {
	// An obsolete account mapping must not apply the mapped model's validation
	// rules to a request whose original model and body are sent unchanged.
	body := []byte(`{"model":"claude-sonnet-4-6","max_tokens":16,"thinking":{"type":"disabled"},"messages":[{"role":"user","content":"hello"}]}`)
	for _, count := range []bool{false, true} {
		t.Run(map[bool]string{false: "messages", true: "count_tokens"}[count], func(t *testing.T) {
			payload := `{"type":"message","content":[],"usage":{"input_tokens":2,"output_tokens":1}}`
			if count {
				payload = `{"input_tokens":2}`
			}
			upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(payload))}}
			svc := newForwardPartialUsageServiceForTest(upstream)
			account := newAnthropicAPIKeyAccountForTest()
			account.Credentials["model_mapping"] = map[string]any{"claude-sonnet-4-6": "claude-sonnet-5-5"}
			parsed, err := ParseGatewayRequestPreservingBody(NewRequestBodyRef(body), PlatformAnthropic)
			require.NoError(t, err)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			if count {
				err = svc.ForwardCountTokens(context.Background(), c, account, parsed)
			} else {
				_, err = svc.Forward(context.Background(), c, account, parsed)
			}
			require.NoError(t, err)
			require.Equal(t, body, upstream.lastBody)
			require.Equal(t, payload, rec.Body.String())
		})
	}
}

func TestStrictAnthropicParserRejectsMalformedJSON(t *testing.T) {
	body := []byte("{\"model\":\"claude-test\",\"messages\":[{\"role\":\"user\",\"content\":\"raw\nnewline\"}]}")
	_, err := ParseGatewayRequestPreservingBody(NewRequestBodyRef(body), PlatformAnthropic)
	require.Error(t, err)
}

func TestStrictAnthropicRequestDoesNotForwardQueryCredentials(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages?api_key=caller-secret&key=another-secret&future=a%2Fb&future=two", nil)
	svc := &GatewayService{cfg: &config.Config{}}
	req, _, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), c, newAnthropicAPIKeyAccountForTest(), []byte(strictAnthropicBody), "upstream-secret")
	require.NoError(t, err)
	require.False(t, req.URL.Query().Has("api_key"))
	require.False(t, req.URL.Query().Has("key"))
	require.Equal(t, []string{"a/b", "two"}, req.URL.Query()["future"])
	require.Equal(t, "upstream-secret", req.Header.Get("X-API-Key"))
}

func TestStrictAnthropicModelSuffixForwardingAndCompatibility(t *testing.T) {
	body := []byte(`{ "model":"claude-sonnet-4-6[1m]", "max_tokens":16, "messages":[] }`)
	for _, strict := range []bool{true, false} {
		for _, count := range []bool{false, true} {
			t.Run(fmt.Sprintf("strict=%t/count=%t", strict, count), func(t *testing.T) {
				payload := `{"type":"message","content":[],"usage":{"input_tokens":2,"output_tokens":1}}`
				if count {
					payload = `{"input_tokens":2}`
				}
				upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(payload))}}
				svc := newForwardPartialUsageServiceForTest(upstream)
				account := newAnthropicAPIKeyAccountForTest()
				account.Extra["anthropic_passthrough"] = strict
				parsed, err := ParseGatewayRequestPreservingBody(NewRequestBodyRef(body), PlatformAnthropic)
				require.NoError(t, err)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				if count {
					err = svc.ForwardCountTokens(context.Background(), c, account, parsed)
				} else {
					var result *ForwardResult
					result, err = svc.Forward(context.Background(), c, account, parsed)
					if strict {
						require.NoError(t, err)
						require.Equal(t, "claude-sonnet-4-6[1m]", result.UpstreamModel)
					}
				}
				require.NoError(t, err)
				if strict {
					require.Equal(t, body, upstream.lastBody)
				} else {
					require.Equal(t, "claude-sonnet-4-6", gjson.GetBytes(upstream.lastBody, "model").String())
				}
			})
		}
	}
}

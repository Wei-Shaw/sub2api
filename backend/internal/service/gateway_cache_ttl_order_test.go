package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// requireCacheTTLOrderAccepted fails when a ttl=1h breakpoint follows a 5m one
// in Anthropic's processing order (tools, system, messages). Anthropic rejects
// such requests with HTTP 400.
func requireCacheTTLOrderAccepted(t *testing.T, body []byte) {
	t.Helper()
	_, messagePaths, toolPaths, systemPaths := collectCacheControlPaths(body)
	var paths []string
	paths = append(paths, toolPaths...)
	paths = append(paths, systemPaths...)
	paths = append(paths, messagePaths...)
	shortTTLPath := ""
	for _, path := range paths {
		if gjson.GetBytes(body, path+".ttl").String() == cacheTTLTarget1h {
			require.Empty(t, shortTTLPath, "1h breakpoint %s follows 5m breakpoint %s in %s", path, shortTTLPath, body)
			continue
		}
		if shortTTLPath == "" {
			shortTTLPath = path
		}
	}
}

func TestNormalizeCacheControlTTLOrder_RaisesBreakpointsBeforeLast1h(t *testing.T) {
	body := []byte(`{"tools":[{"name":"t","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}}],` +
		`"system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral","ttl":"5m"}}],` +
		`"messages":[{"role":"user","content":[` +
		`{"type":"text","text":"a","cache_control":{"type":"ephemeral","ttl":"1h"}},` +
		`{"type":"text","text":"b","cache_control":{"type":"ephemeral","ttl":"5m"}}]}]}`)

	out := normalizeCacheControlTTLOrder(body)

	require.Equal(t, "1h", gjson.GetBytes(out, "tools.0.cache_control.ttl").String(), "omitted ttl counts as 5m")
	require.Equal(t, "1h", gjson.GetBytes(out, "system.0.cache_control.ttl").String())
	require.Equal(t, "1h", gjson.GetBytes(out, "messages.0.content.0.cache_control.ttl").String())
	require.Equal(t, "5m", gjson.GetBytes(out, "messages.0.content.1.cache_control.ttl").String(),
		"a 5m breakpoint after the last 1h one is valid and must keep its ttl")
	requireCacheTTLOrderAccepted(t, out)
}

func TestNormalizeCacheControlTTLOrder_TopLevelCacheControlIsLast(t *testing.T) {
	body := []byte(`{"cache_control":{"type":"ephemeral","ttl":"1h"},` +
		`"system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral","ttl":"5m"}}],` +
		`"messages":[{"role":"user","content":"hi"}]}`)

	out := normalizeCacheControlTTLOrder(body)

	require.Equal(t, "1h", gjson.GetBytes(out, "system.0.cache_control.ttl").String())
}

func TestNormalizeCacheControlTTLOrder_KeepsValidRequestsUnchanged(t *testing.T) {
	cases := map[string]string{
		"no breakpoints": `{"system":"s","messages":[{"role":"user","content":"hi"}]}`,
		"all 5m": `{"tools":[{"name":"t","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}}],` +
			`"system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral","ttl":"5m"}}],` +
			`"messages":[{"role":"user","content":[{"type":"text","text":"a","cache_control":{"type":"ephemeral","ttl":"5m"}}]}]}`,
		"1h before 5m": `{"system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral","ttl":"1h"}}],` +
			`"messages":[{"role":"user","content":[{"type":"text","text":"a","cache_control":{"type":"ephemeral"}}]}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, body, string(normalizeCacheControlTTLOrder([]byte(body))))
		})
	}
}

// A non-Claude Code client sends one 1h breakpoint. The mimic path adds 5m
// breakpoints to the injected system block and the last tool, which come first
// in Anthropic's processing order. The outbound body must still be accepted.
func TestEnforceCacheControlLimit_MimicPathKeepsClient1hBreakpointValid(t *testing.T) {
	cases := map[string]string{
		"message 1h": `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":[` +
			`{"type":"text","text":"Reply OK.","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`,
		"system 1h moved into messages": `{"model":"claude-sonnet-4-6",` +
			`"system":[{"type":"text","text":"Be terse.","cache_control":{"type":"ephemeral","ttl":"1h"}}],` +
			`"messages":[{"role":"user","content":"Reply OK."}]}`,
		"tools and message 1h": `{"model":"claude-sonnet-4-6","tools":[{"name":"probe","input_schema":{"type":"object"}}],` +
			`"messages":[{"role":"user","content":[` +
			`{"type":"text","text":"Reply OK.","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			body := []byte(raw)
			var system any
			if sys := gjson.GetBytes(body, "system"); sys.Exists() {
				require.NoError(t, json.Unmarshal([]byte(sys.Raw), &system))
			}
			body = rewriteSystemForNonClaudeCode(body, system)
			body = applyToolsLastCacheBreakpoint(body)

			out := enforceCacheControlLimit(body)

			requireCacheTTLOrderAccepted(t, out)
		})
	}
}

func TestForwardCountTokens_MimicPathKeepsClient1hBreakpointValid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
	c.Request.Header.Set("User-Agent", "third-party-client/1.0")

	// The mimic path adds a 5m breakpoint to tools[-1], ahead of the client's 1h one.
	body := []byte(`{"model":"claude-sonnet-4-6",` +
		`"tools":[{"name":"probe","description":"d","input_schema":{"type":"object"}}],` +
		`"messages":[{"role":"user","content":[{"type":"text","text":"Reply OK.","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`)
	parsed := &ParsedRequest{Body: NewRequestBodyRef(body), Model: "claude-sonnet-4-6"}

	upstream := &anthropicHTTPUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"input_tokens":42}`)),
		},
	}
	svc := &GatewayService{
		cfg:              &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream:     upstream,
		rateLimitService: &RateLimitService{},
	}
	account := &Account{
		ID:          402,
		Name:        "count-tokens-ttl-order-test",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeSetupToken,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-oauth-token"},
		Status:      StatusActive,
		Schedulable: true,
	}

	require.NoError(t, svc.ForwardCountTokens(context.Background(), c, account, parsed))

	require.True(t, gjson.GetBytes(upstream.lastBody, "tools.0.cache_control").Exists(),
		"the mimic tools breakpoint should still be present")
	requireCacheTTLOrderAccepted(t, upstream.lastBody)
}

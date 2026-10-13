//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestStructuredOutputsBetaOAuthMimic(t *testing.T) {
	const beta = "structured-outputs-2025-11-13"
	for _, tc := range []struct {
		name   string
		header string
		drop   map[string]struct{}
		want   bool
	}{
		{"explicit", beta, nil, true},
		{"mixed and duplicate", "custom-beta, " + beta + "," + beta, nil, true},
		{"absent", "custom-beta", nil, false},
		{"similar token", beta + "-other", nil, false},
		{"filtered", beta, map[string]struct{}{beta: {}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestGatewayServiceForBeta(false)
			header := http.Header{}
			header.Set("anthropic-beta", tc.header)
			body := []byte(`{"output_format":{"type":"json_schema","schema":{"type":"object"}}}`)
			got, set := s.computeFinalAnthropicBeta("oauth", true, "claude-sonnet-5", header, body, tc.drop)
			require.True(t, set)
			require.Equal(t, tc.want, containsBetaToken(got, beta))
			require.False(t, containsBetaToken(got, "custom-beta"))
			require.True(t, containsBetaToken(got, "oauth-2025-04-20"))
			if tc.want {
				require.Equal(t, 1, strings.Count(got, beta))
			}
		})
	}
}

func TestBuildUpstreamRequestStructuredOutputsBeta(t *testing.T) {
	const beta = "structured-outputs-2025-11-13"
	for _, filtered := range []bool{false, true} {
		t.Run(map[bool]string{false: "preserved", true: "policy filtered"}[filtered], func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			c.Request.Header.Set("anthropic-beta", beta+",custom-beta")
			if filtered {
				c.Set(betaPolicyFilterSetKey, map[string]struct{}{beta: {}})
			}
			account := &Account{ID: 701, Platform: PlatformAnthropic, Type: AccountTypeOAuth,
				Credentials: map[string]any{"access_token": "test-token"}, Status: StatusActive, Schedulable: true}
			body := []byte(`{"model":"claude-sonnet-5","max_tokens":1024,"output_format":{"type":"json_schema","schema":{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}},"messages":[{"role":"user","content":"Return JSON"}]}`)
			svc := &GatewayService{cfg: &config.Config{}}
			req, _, err := svc.buildUpstreamRequest(context.Background(), c, account, body,
				"test-token", "oauth", "claude-sonnet-5", false, true)
			require.NoError(t, err)
			out := readUpstreamBodyForTest(t, req)
			require.JSONEq(t, gjson.GetBytes(body, "output_format").Raw, gjson.GetBytes(out, "output_format").Raw)
			header := getHeaderRaw(req.Header, "anthropic-beta")
			require.Equal(t, !filtered, containsBetaToken(header, beta))
			require.False(t, containsBetaToken(header, "custom-beta"))
		})
	}
}

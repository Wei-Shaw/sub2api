//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func forwardCompositeThinkingRequest(t *testing.T, publicModel, upstreamModel, clientBody string) (*httptest.ResponseRecorder, *anthropicHTTPUpstreamRecorder, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte(clientBody)
	ctx := context.Background()
	if upstreamModel != publicModel {
		body = RewriteCompositeRequestModel(body, "model", upstreamModel)
		ctx = WithCompositeRouteDecision(ctx, CompositeRouteDecision{
			Matched: true, Source: CompositeRouteSourceExplicit,
			PublicModel: publicModel, TargetPlatform: PlatformAnthropic, UpstreamModel: upstreamModel,
		})
	}
	c.Request = c.Request.WithContext(ctx)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)

	upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"msg_1","type":"message","role":"assistant","model":"` + upstreamModel + `","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)),
	}}
	svc := newForwardPartialUsageServiceForTest(upstream)
	_, err = svc.Forward(ctx, c, newAnthropicOAuthAccountForPartialUsageTest(), parsed)
	return rec, upstream, err
}

// A composite route that rewrites claude-sonnet-5 to claude-opus-5-5 sends the
// client's budget thinking upstream as adaptive thinking instead of failing
// Opus 5.5 validation.
func TestCompositeRewriteToOpus55SendsEnabledThinkingAsAdaptive(t *testing.T) {
	rec, upstream, err := forwardCompositeThinkingRequest(t, "claude-sonnet-5", "claude-opus-5-5",
		`{"model":"claude-sonnet-5","max_tokens":64,"stream":false,"thinking":{"type":"enabled","budget_tokens":4096},"messages":[{"role":"user","content":"hi"}]}`)

	require.NoError(t, err)
	require.NotEqual(t, http.StatusBadRequest, rec.Code)
	require.NotNil(t, upstream.lastReq, "request must reach the upstream")
	require.Equal(t, "claude-opus-5-5", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "adaptive", gjson.GetBytes(upstream.lastBody, "thinking.type").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "thinking.budget_tokens").Exists())
}

func TestCompositeRewriteToSonnet55SendsDisabledThinkingAsBetweenTools(t *testing.T) {
	rec, upstream, err := forwardCompositeThinkingRequest(t, "claude-sonnet-5", "claude-sonnet-5-5",
		`{"model":"claude-sonnet-5","max_tokens":64,"stream":false,"thinking":{"type":"disabled"},"messages":[{"role":"user","content":"hi"}]}`)

	require.NoError(t, err)
	require.NotEqual(t, http.StatusBadRequest, rec.Code)
	require.NotNil(t, upstream.lastReq, "request must reach the upstream")
	require.Equal(t, "between_tools", gjson.GetBytes(upstream.lastBody, "thinking.type").String())
}

// Without a model rewrite the client's own invalid thinking mode is still
// rejected.
func TestDirectOpus55RequestWithEnabledThinkingIsStillRejected(t *testing.T) {
	rec, upstream, err := forwardCompositeThinkingRequest(t, "claude-opus-5-5", "claude-opus-5-5",
		`{"model":"claude-opus-5-5","max_tokens":64,"stream":false,"thinking":{"type":"enabled","budget_tokens":4096},"messages":[{"role":"user","content":"hi"}]}`)

	require.ErrorContains(t, err, "claude-opus-5-5 requires adaptive thinking")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Nil(t, upstream.lastReq)
}

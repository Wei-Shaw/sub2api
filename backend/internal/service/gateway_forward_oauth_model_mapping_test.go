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

func TestGatewayService_Forward_AnthropicSetupTokenAppliesAccountModelMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		requested string
		upstream  string
	}{
		{requested: "claude-opus-4-6", upstream: "claude-opus-4-7"},
		{requested: "claude-sonnet-4-6", upstream: "claude-sonnet-4-6"},
	} {
		t.Run(tc.requested, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

			body := []byte(`{"model":"` + tc.requested + `","stream":false,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
			parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
			require.NoError(t, err)

			upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"msg_1","type":"message","role":"assistant","model":"` + tc.upstream + `","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":1,"output_tokens":1}}`)),
			}}
			svc := newForwardPartialUsageServiceForTest(upstream)
			account := newAnthropicOAuthAccountForPartialUsageTest()
			account.Type = AccountTypeSetupToken
			account.Credentials["model_mapping"] = map[string]any{
				"claude-opus-4-6":   "claude-opus-4-7",
				"claude-sonnet-4-6": "claude-sonnet-4-6",
			}

			_, err = svc.Forward(context.Background(), c, account, parsed)
			require.NoError(t, err)
			require.Equal(t, tc.upstream, gjson.GetBytes(upstream.lastBody, "model").String())
		})
	}
}

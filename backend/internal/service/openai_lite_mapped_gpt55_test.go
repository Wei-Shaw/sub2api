package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMappedGPT55LiteCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name     string
		id       int64
		model    string
		mapped   bool
		wantLite bool
	}{
		{"mapped", 12, "gpt-5.5", true, false},
		{"account13Mapped", 13, "gpt-5.5", true, false},
		{"account14Mapped", 14, "gpt-5.5", true, false},
		{"account13Native", 13, "gpt-6-astra", false, true},
		{"account14Native", 14, "gpt-6-astra", false, true},
		{"nativeSol", 12, "gpt-5.6-sol", true, true},
		{"noMapping", 12, "gpt-5.5", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &Account{ID: tc.id, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{}}
			if tc.mapped {
				a.Credentials["model_mapping"] = map[string]any{"gpt-5.6-sol": "gpt-5.5"}
			}
			body := []byte(`{"model":"` + tc.model + `","input":[{"type":"additional_tools","role":"developer","tools":[]},{"type":"function_call_output","call_id":"call1","output":"12345678901234567890"}],"reasoning":{"context":"all_turns"},"client_metadata":{"ws_request_header_x_openai_internal_codex_responses_lite":"true","keep":"yes"}}`)
			r, err := http.NewRequest("POST", "http://localhost", bytes.NewReader(body))
			require.NoError(t, err)
			r.Header.Set(responsesLiteHeader, "true")
			require.NoError(t, applyMappedGPT55LiteCompatibility(r, a, body))
			require.Equal(t, tc.wantLite, isOpenAIResponsesLiteHeader(r.Header.Get(responsesLiteHeader)))
			got, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			if tc.wantLite {
				require.Equal(t, body, got)
			} else {
				require.False(t, isOpenAIResponsesLiteWebSocketPayload(got))
				require.Equal(t, gjson.GetBytes(body, "input").Raw, gjson.GetBytes(got, "input").Raw)
				require.Equal(t, "all_turns", gjson.GetBytes(got, "reasoning.context").String())
				require.Equal(t, "yes", gjson.GetBytes(got, "client_metadata.keep").String())
				require.Equal(t, int64(len(got)), r.ContentLength)
				replay, err := r.GetBody()
				require.NoError(t, err)
				again, err := io.ReadAll(replay)
				require.NoError(t, err)
				require.Equal(t, got, again)
			}
			require.True(t, isOpenAIResponsesLiteWebSocketPayload(body))
		})
	}
}

func TestMappedGPT55LiteBuildersPreserveIngressForFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
		c.Request.Header.Set(responsesLiteHeader, "true")
		s := &OpenAIGatewayService{cfg: &config.Config{}}
		a := &Account{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.5"}}}
		body := []byte(`{"model":"gpt-5.5","stream":true,"input":[]}`)
		var r *http.Request
		var err error
		if passthrough {
			r, err = s.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, a, body, "test-token")
		} else {
			r, err = s.buildUpstreamRequest(context.Background(), c, a, body, "test-token", true, "", true)
		}
		require.NoError(t, err)
		require.Empty(t, r.Header.Get(responsesLiteHeader))
		require.Equal(t, "true", c.GetHeader(responsesLiteHeader))
		a.ID = 14
		body = []byte(`{"model":"gpt-6-astra","stream":true,"input":[]}`)
		if passthrough {
			r, err = s.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, a, body, "test-token")
		} else {
			r, err = s.buildUpstreamRequest(context.Background(), c, a, body, "test-token", true, "", true)
		}
		require.NoError(t, err)
		require.Equal(t, "true", r.Header.Get(responsesLiteHeader))
	}
}

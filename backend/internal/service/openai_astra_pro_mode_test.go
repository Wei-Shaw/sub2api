//go:build unit

package service

// Integration-level tests for GPT-6 Astra reasoning.mode preservation through
// OpenAIGatewayService.Forward.
//
// These exercise the real forward pipeline through the httpUpstreamRecorder mock
// (no real network / credentials / config), covering the OpenAI OAuth account
// (native Codex transform when passthrough is disabled, and the passthrough
// branch when enabled). Client stream=true and stream=false both receive an
// upstream SSE fixture, because Codex upstreams stream: the stream=false client
// path exercises the real SSE->JSON aggregation instead of a synthetic JSON body.

import (
	"context"
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

const astraProCodexResponsesURL = "https://chatgpt.com/backend-api/codex/responses"

type astraForwardSetup struct {
	upstream *httpUpstreamRecorder
	svc      *OpenAIGatewayService
	c        *gin.Context
	rec      *httptest.ResponseRecorder
	account  *Account
}

// newAstraOAuthSetup builds an OpenAI OAuth account using the minimal
// svc+account harness pattern (mirrors openai_oauth_passthrough_test.go). When
// passthrough is true the account routes through forwardOpenAIPassthrough.
func newAstraOAuthSetup(t *testing.T, passthrough bool) *astraForwardSetup {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
	upstream := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: false}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:             888,
		Name:           "oauth-astra",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
		Extra:          map[string]any{"openai_passthrough": passthrough},
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
	}
	return &astraForwardSetup{upstream: upstream, svc: svc, c: c, rec: rec, account: account}
}

func codexCompletedSSE(inner string) string {
	return "data: {\"type\":\"response.completed\",\"response\":" + inner + "}\n\ndata: [DONE]\n\n"
}

func astraRequestBody(model string, stream bool, mode, effort string) []byte {
	var reasoningParts []string
	if mode != "" {
		reasoningParts = append(reasoningParts, `"mode":"`+mode+`"`)
	}
	if effort != "" {
		reasoningParts = append(reasoningParts, `"effort":"`+effort+`"`)
	}
	reasoning := ""
	if len(reasoningParts) > 0 {
		reasoning = `,"reasoning":{` + strings.Join(reasoningParts, ",") + `}`
	}
	streamJSON := "false"
	if stream {
		streamJSON = "true"
	}
	return []byte(`{"model":"` + model + `","stream":` + streamJSON + `,` +
		`"instructions":"test","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]` +
		reasoning + `}`)
}

// TestForward_AstraOAuth_NonAstraLegacyStillStrips asserts the historical strip
// behavior is preserved for a non-Astra model on an OAuth account.
func TestForward_AstraOAuth_NonAstraLegacyStillStrips(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := newAstraOAuthSetup(t, false)
	inner := `{"id":"resp_test","model":"gpt-5.6-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
	s.upstream.resp = &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(codexCompletedSSE(inner))),
	}
	body := astraRequestBody("gpt-5.6-sol", true, "pro", "")

	result, err := s.svc.Forward(context.Background(), s.c, s.account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, s.upstream.lastReq)
	require.Equal(t, astraProCodexResponsesURL, s.upstream.lastReq.URL.String())

	forwarded := s.upstream.lastBody
	require.False(t, gjson.GetBytes(forwarded, "reasoning.mode").Exists(),
		"non-Astra legacy must strip reasoning.mode")
	require.Equal(t, "max", gjson.GetBytes(forwarded, "reasoning.effort").String(),
		"non-Astra legacy mode=pro without effort injects max")
}

// TestForward_AstraOAuth_ModeMatrix_Preserved runs the pro/standard/missing-mode
// x effort=max matrix across both forward branches and both client stream modes.
// Every attempt receives an upstream SSE fixture; assertions check the final
// upstream request keeps mode/effort and the URL is the Codex responses endpoint.
func TestForward_AstraOAuth_ModeMatrix_Preserved(t *testing.T) {
	cases := []struct {
		name    string
		mode    string
		effort  string
		wantMod string // gjson string value; "" == absent
		wantEff string
	}{
		{name: "pro+max", mode: "pro", effort: "max", wantMod: "pro", wantEff: "max"},
		{name: "standard+max", mode: "standard", effort: "max", wantMod: "standard", wantEff: "max"},
		{name: "missing mode + max", mode: "", effort: "max", wantMod: "", wantEff: "max"},
		{name: "pro+high", mode: "pro", effort: "high", wantMod: "pro", wantEff: "high"},
		{name: "pro no effort", mode: "pro", effort: "", wantMod: "pro", wantEff: ""},
	}
	for _, passthrough := range []bool{false, true} {
		branch := "native-codex"
		if passthrough {
			branch = "passthrough"
		}
		for _, stream := range []bool{true, false} {
			for _, tt := range cases {
				t.Run(tt.name+"/"+branch+"/stream="+map[bool]string{true: "true", false: "false"}[stream], func(t *testing.T) {
					s := newAstraOAuthSetup(t, passthrough)
					inner := `{"id":"resp_test","model":"gpt-6-astra","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
					s.upstream.resp = &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
						Body:       io.NopCloser(strings.NewReader(codexCompletedSSE(inner))),
					}
					body := astraRequestBody("gpt-6-astra", stream, tt.mode, tt.effort)

					result, err := s.svc.Forward(context.Background(), s.c, s.account, body)
					require.NoError(t, err)
					require.NotNil(t, result)
					require.NotNil(t, s.upstream.lastReq)
					require.Equal(t, astraProCodexResponsesURL, s.upstream.lastReq.URL.String(),
						"OAuth account must hit the Codex responses endpoint")

					forwarded := s.upstream.lastBody
					require.Equal(t, "gpt-6-astra", gjson.GetBytes(forwarded, "model").String())

					mode := gjson.GetBytes(forwarded, "reasoning.mode")
					if tt.wantMod == "" {
						require.False(t, mode.Exists(), "mode should be absent for %q", tt.name)
					} else {
						require.True(t, mode.Exists(), "Astra mode must be preserved for %q", tt.name)
						require.Equal(t, tt.wantMod, mode.String())
					}
					eff := gjson.GetBytes(forwarded, "reasoning.effort")
					if tt.wantEff == "" {
						require.False(t, eff.Exists(), "no effort injected when omitted for %q", tt.name)
					} else {
						require.Equal(t, tt.wantEff, eff.String(), "Astra effort preserved for %q", tt.name)
					}
				})
			}
		}
	}
}

// TestForward_AstraOAuth_NonStreamAggregation_PreservesResponseReasoningMode
// drives client stream=false (which is forced to an SSE upstream) and asserts the
// aggregated JSON response object still carries response.reasoning.mode after
// SSE->JSON aggregation.
func TestForward_AstraOAuth_NonStreamAggregation_PreservesResponseReasoningMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := newAstraOAuthSetup(t, false)
	// response.completed.response carries a top-level reasoning:{mode:pro,effort:max}.
	inner := `{"id":"resp_astra","object":"response","created_at":0,"status":"completed","model":"gpt-6-astra","reasoning":{"mode":"pro","effort":"max"},"output":[{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"think"}]},{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"hi"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
	s.upstream.resp = &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(codexCompletedSSE(inner))),
	}
	body := astraRequestBody("gpt-6-astra", false, "pro", "max")

	result, err := s.svc.Forward(context.Background(), s.c, s.account, body)
	require.NoError(t, err)
	require.NotNil(t, result)

	downstream := s.rec.Body.String()
	// Downstream is JSON (SSE->JSON aggregation for the non-streaming client).
	require.True(t, gjson.Valid(downstream), "downstream must be aggregated JSON, got: %s", downstream)
	require.Equal(t, "pro", gjson.Get(downstream, "reasoning.mode").String(),
		"aggregated response object must keep reasoning.mode")
	require.Equal(t, "max", gjson.Get(downstream, "reasoning.effort").String())
}

// TestForward_AstraOAuth_Stream_PreservesResponseReasoningMode drives client
// stream=true and asserts the streamed response.completed event's response object
// still carries response.reasoning.mode.
func TestForward_AstraOAuth_Stream_PreservesResponseReasoningMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := newAstraOAuthSetup(t, false)
	inner := `{"id":"resp_astra","object":"response","created_at":0,"status":"completed","model":"gpt-6-astra","reasoning":{"mode":"pro","effort":"max"},"output":[{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"hi"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
	s.upstream.resp = &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(codexCompletedSSE(inner))),
	}
	body := astraRequestBody("gpt-6-astra", true, "pro", "max")

	result, err := s.svc.Forward(context.Background(), s.c, s.account, body)
	require.NoError(t, err)
	require.NotNil(t, result)

	downstream := s.rec.Body.String()
	require.Contains(t, downstream, "response.completed", "streaming downstream must include completed event")
	// Extract response.completed line and assert response.reasoning.mode inside it.
	var completedJSON string
	for _, line := range strings.Split(downstream, "\n") {
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"type":"response.completed"`) {
			completedJSON = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	require.NotEmpty(t, completedJSON, "no response.completed data line found in: %s", downstream)
	require.Equal(t, "pro", gjson.Get(completedJSON, "response.reasoning.mode").String(),
		"streamed completed response object must keep reasoning.mode")
	require.Equal(t, "max", gjson.Get(completedJSON, "response.reasoning.effort").String())
}

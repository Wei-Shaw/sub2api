//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Record the final HTTP body. A fixed terminal response stops each request at
// the transport boundary, independently of its streaming response protocol.
func captureExtensionRequest(t *testing.T, account *Account, ingress string, body []byte) []byte {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+ingress, bytes.NewReader(body))
	up := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusTeapot, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"fixture complete"}}`)),
	}}
	cfg := structuredInputRetryTestConfig()
	ctx := context.Background()
	var err error
	switch account.Platform {
	case PlatformAnthropic:
		s := &GatewayService{cfg: cfg, httpUpstream: up}
		switch ingress {
		case "chat/completions":
			_, err = s.ForwardAsChatCompletions(ctx, c, account, body, nil)
		case "responses":
			_, err = s.ForwardAsResponses(ctx, c, account, body, nil)
		case "messages":
			parsed, parseErr := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
			require.NoError(t, parseErr)
			_, err = s.Forward(ctx, c, account, parsed)
		}
	case PlatformGemini:
		s := &GeminiMessagesCompatService{cfg: cfg, httpUpstream: up}
		switch ingress {
		case "messages":
			_, err = s.Forward(ctx, c, account, body)
		case "chat/completions":
			_, err = s.ForwardAsChatCompletions(ctx, c, account, body)
		case "responses":
			gateway := &GatewayService{cfg: cfg, httpUpstream: up}
			_, err = gateway.ForwardAsResponses(ctx, c, account, body, nil)
		}
	case PlatformAntigravity:
		s := newAntigravityCompatService(cfg.Gateway, up)
		switch ingress {
		case "chat/completions":
			_, err = s.ForwardAsChatCompletions(ctx, c, account, body, nil)
		case "responses":
			_, err = s.ForwardAsResponses(ctx, c, account, body, nil)
		case "messages":
			_, err = s.Forward(ctx, c, account, body, false)
		}
	default:
		s := &OpenAIGatewayService{cfg: cfg, httpUpstream: up}
		if account.IsGrokOAuth() {
			s.grokTokenProvider = NewGrokTokenProvider(nil, nil)
		}
		switch ingress {
		case "chat/completions":
			_, err = s.ForwardAsChatCompletions(ctx, c, account, body, "", "")
		case "responses":
			_, err = s.Forward(ctx, c, account, body)
		case "messages":
			_, err = s.ForwardAsAnthropic(ctx, c, account, body, "", "")
		}
	}
	require.Len(t, up.bodies, 1, "upstream capture: %v", err)
	return up.lastBody
}

func TestDeepSeekRequestAtSelectedUpstream(t *testing.T) {
	const dsTools = `[{"type":"deepseek_read_url","name":"deepseek_read_url","option":{"future":7}},{"type":"deepseek_web_search","name":"deepseek_web_search"}]`
	const title = `{"enabled":true,"future":17}`
	const safety = `{"level":"high","policy":{"version":2}}`
	routes := []struct{ platform, auth, protocol string }{
		{PlatformDeepseek, AccountTypeAPIKey, APIProtocolResponses},
		{PlatformDeepseek, AccountTypeAPIKey, APIProtocolChatCompletions},
		{PlatformDeepseek, AccountTypeAPIKey, APIProtocolAnthropic},
		{PlatformDeepseek, AccountTypeAPIKey, APIProtocolAdaptive},
		{PlatformOpenAI, AccountTypeAPIKey, APIProtocolResponses},
		{PlatformOpenAI, AccountTypeAPIKey, APIProtocolChatCompletions},
		{PlatformOpenAI, AccountTypeOAuth, APIProtocolResponses},
		{PlatformGrok, AccountTypeAPIKey, APIProtocolResponses},
		{PlatformGrok, AccountTypeOAuth, APIProtocolResponses},
		{PlatformKimi, AccountTypeAPIKey, APIProtocolResponses},
		{PlatformKimi, AccountTypeAPIKey, APIProtocolChatCompletions},
		{PlatformZhipu, AccountTypeAPIKey, APIProtocolChatCompletions},
		{PlatformZhipu, AccountTypeAPIKey, APIProtocolAnthropic},
		{PlatformMiniMax, AccountTypeAPIKey, APIProtocolResponses},
		{PlatformOpenCodeGo, AccountTypeAPIKey, APIProtocolChatCompletions},
		{PlatformOpenCodeGo, AccountTypeAPIKey, APIProtocolResponses},
		{PlatformAnthropic, AccountTypeAPIKey, ""},
		{PlatformGemini, AccountTypeAPIKey, ""},
		{PlatformAntigravity, AccountTypeOAuth, ""},
	}
	for _, route := range routes {
		for _, ingress := range []string{"chat/completions", "responses", "messages"} {
			t.Run(strings.Join([]string{route.platform, route.auth, route.protocol, ingress}, "/"), func(t *testing.T) {
				account := structuredInputRetryTestAccount()
				account.Platform, account.Type = route.platform, route.auth
				account.Credentials["api_protocol"] = route.protocol
				account.Credentials["access_token"] = "fixture"
				account.Credentials["base_url"] = "https://relay.example"
				account.Extra["openai_responses_supported"] = route.protocol == APIProtocolResponses
				model := "test-model"
				if account.IsGrokOAuth() {
					account = healthyGrokOAuthGatewayTestAccount(101, "fixture")
					model = "grok"
				}
				if route.platform == PlatformAntigravity {
					account = newAntigravityCompatAccount(AccountTypeOAuth)
					model = "gemini-3.1-pro-high"
				}
				if route.platform == PlatformAnthropic {
					account.Extra["anthropic_passthrough"] = true
				}
				function := `{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"q":{"type":"string","title":"keep"}}}}}`
				base := `{"model":"` + model + `","messages":[{"role":"user","content":"hi"}]}`
				switch ingress {
				case "responses":
					base = `{"model":"` + model + `","input":"hi"}`
					function = `{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"q":{"type":"string","title":"keep"}}}}`
				case "messages":
					function = `{"name":"lookup","input_schema":{"type":"object","properties":{"q":{"type":"string","title":"keep"}}}}`
				}
				clean, err := sjson.SetRaw(base, "tools", "["+function+"]")
				require.NoError(t, err)
				extended, err := sjson.SetRaw(clean, "tools", dsTools[:len(dsTools)-1]+","+function+"]")
				require.NoError(t, err)
				extended, err = sjson.SetRaw(extended, "title", title)
				require.NoError(t, err)
				extended, err = sjson.SetRaw(extended, "safety_check", safety)
				require.NoError(t, err)
				body := []byte(extended)
				out := captureExtensionRequest(t, account, ingress, body)
				require.Equal(t, extended, string(body), "another account may receive the same input")
				if route.platform == PlatformDeepseek {
					require.JSONEq(t, title, gjson.GetBytes(out, "title").Raw)
					require.JSONEq(t, safety, gjson.GetBytes(out, "safety_check").Raw)
					tools := gjson.GetBytes(out, "tools").Array()
					require.Len(t, tools, 3)
					require.JSONEq(t, gjson.Get(dsTools, "0").Raw, tools[0].Raw)
					require.JSONEq(t, gjson.Get(dsTools, "1").Raw, tools[1].Raw)
					return
				}
				require.False(t, gjson.GetBytes(out, "title").Exists())
				require.False(t, gjson.GetBytes(out, "safety_check").Exists())
				require.NotContains(t, string(out), "deepseek_")
				require.Contains(t, string(out), "lookup")
				baseline := captureExtensionRequest(t, account, ingress, []byte(clean))
				if route.platform == PlatformAntigravity {
					// The Antigravity envelope generates an independent request ID.
					out, _ = sjson.DeleteBytes(out, "requestId")
					baseline, _ = sjson.DeleteBytes(baseline, "requestId")
				}
				require.JSONEq(t, string(baseline), string(out))
			})
		}
	}
}

func TestDeepSeekHistoryForActualMessagesEndpoint(t *testing.T) {
	history := `"messages":[{"role":"assistant","content":[{"type":"server_tool_use","id":"r","name":"deepseek_read_url"},{"type":"web_search_tool_result","tool_use_id":"s"},{"type":"web_fetch_tool_result","tool_use_id":"r","kind":"read_link","signature":"opaque"},{"type":"text","text":"done"}]}]`
	for _, platform := range []string{PlatformDeepseek, PlatformKimi, PlatformMiniMax, PlatformZhipu, PlatformOpenAI, PlatformAnthropic} {
		for _, base := range []string{"https://api.deepseek.com/anthropic", "https://relay.example"} {
			account := &Account{Platform: platform, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_protocol": APIProtocolAnthropic, "base_url": base}}
			for _, decl := range []string{"", `"tools":[{"type":"deepseek_web_search"}],`} {
				body := []byte("{" + decl + history + "}")
				// OpenAI has no native Messages protocol. Anthropic API keys
				// can target a third-party model through their ordinary base URL.
				mappedModel := "model-alias"
				if platform == PlatformAnthropic {
					mappedModel = "glm-4.7"
				}
				out := FilterWebSearchHistoryBlocks(body, mappedModel, account)
				if platform != PlatformOpenAI && strings.Contains(base, "api.deepseek.com") {
					require.Equal(t, body, out)
				} else {
					require.Equal(t, []string{"text"}, collectContentTypes(t, out))
				}
			}
		}
	}
}

func TestDeepSeekChatEndpointReceivesSignedHistory(t *testing.T) {
	body := []byte(`{"model":"deepseek-flash","input":[{"role":"user","content":"read"},{"type":"web_search_call","id":"r","status":"completed","kind":"read_link","action":{"type":"open_page","url":"https://example.com"},"signature":"opaque+/="},{"role":"assistant","content":"done"},{"role":"user","content":"quote it"}]}`)
	for _, platform := range []string{PlatformKimi, PlatformDeepseek} {
		account := structuredInputRetryTestAccount()
		account.Platform = platform
		account.Credentials["api_protocol"] = APIProtocolChatCompletions
		out := captureExtensionRequest(t, account, "responses", body)
		calls := gjson.GetBytes(out, "messages.1.server_tool_calls")
		if platform == PlatformDeepseek {
			require.Equal(t, "opaque+/=", calls.Get("0.calls.0.signature").String())
			require.Equal(t, "read_link", calls.Get("0.calls.0.action.type").String())
		} else {
			require.False(t, calls.Exists())
		}
	}
}

func TestDeepSeekRequestExtensionScopes(t *testing.T) {
	const input = `{
		"title":true,"safety_check":"high",
		"tools":[{"type":"function","name":"deepseek_read_url","parameters":{"title":"schema title"}},{"type":"deepseek_read_url"},{"Type":"deepseek_web_search"}],
		"input":[{"type":"additional_tools","tools":[{"type":"deepseek_web_search"},{"type":"namespace","tools":[{"type":"deepseek_read_url"},{"type":"function","name":"local"}]}]}]
	}`
	body := []byte(input)
	out := filterDeepSeekRequest(body, &Account{Platform: PlatformOpenAI})
	require.False(t, gjson.GetBytes(out, "title").Exists())
	require.False(t, gjson.GetBytes(out, "safety_check").Exists())
	require.JSONEq(t, `[{"type":"function","name":"deepseek_read_url","parameters":{"title":"schema title"}}]`, gjson.GetBytes(out, "tools").Raw)
	require.JSONEq(t, `[{"type":"namespace","tools":[{"type":"function","name":"local"}]}]`, gjson.GetBytes(out, "input.0.tools").Raw)
	otherAttempt := filterDeepSeekRequest(body, &Account{Platform: PlatformDeepseek})
	require.JSONEq(t, gjson.Get(input, "input").Raw, gjson.GetBytes(otherAttempt, "input").Raw)
	require.JSONEq(t, gjson.Get(input, "tools").Raw, gjson.GetBytes(otherAttempt, "tools").Raw)
	require.False(t, gjson.GetBytes(otherAttempt, "title").Exists())
	require.False(t, gjson.GetBytes(otherAttempt, "safety_check").Exists())
	require.Equal(t, input, string(body))

	onlyDeepSeek := []byte(`{"input":[{"type":"additional_tools","tools":[{"type":"deepseek_read_url"}]}]}`)
	out = filterDeepSeekRequest(onlyDeepSeek, &Account{Platform: PlatformOpenAI})
	require.Equal(t, "[]", gjson.GetBytes(out, "input.0.tools").Raw)
	_, _, err := adaptResponsesClientToolsForAnthropic(out)
	require.NoError(t, err)
}

func TestClientDecodeErrorSkipsAccountSchedule(t *testing.T) {
	s := &OpenAIGatewayService{cfg: structuredInputRetryTestConfig()}
	account := structuredInputRetryTestAccount()
	s.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	require.NotNil(t, s.getOpenAIAccountScheduler(context.Background()))
	for _, ingress := range []string{"chat/completions", "responses", "messages"} {
		body := []byte(`{"model":"test-model","messages":true,"max_output_tokens":true,"title":"x"}`)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+ingress, bytes.NewReader(body))
		var err error
		switch ingress {
		case "chat/completions":
			_, err = s.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
		case "responses":
			account.Extra["openai_responses_supported"] = false
			_, err = s.Forward(context.Background(), c, account, body)
		case "messages":
			_, err = s.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
		}
		require.ErrorIs(t, err, errInvalidGatewayRequest)
		var typeError *json.UnmarshalTypeError
		require.ErrorAs(t, err, &typeError)
		require.Equal(t, http.StatusBadRequest, c.Writer.Status())
		s.ReportOpenAIAccountScheduleResult(account, "", false, nil, err)
		require.Zero(t, s.openaiAccountStats.size())
	}
	upstreamErr := json.Unmarshal([]byte("true"), new(string))
	require.Error(t, upstreamErr)
	s.ReportOpenAIAccountScheduleResult(account, "", false, nil, upstreamErr)
	require.Equal(t, 1, s.openaiAccountStats.size())
}

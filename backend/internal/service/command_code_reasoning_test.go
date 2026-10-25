//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const commandCodeChatCompletionsURL = "https://api.commandcode.ai/provider/v1/chat/completions"

// commandCodeOffModels are the Command Code DeepSeek models verified to turn
// thinking off at reasoning_effort "off".
var commandCodeOffModels = []string{
	"deepseek/deepseek-v4-pro",
	"deepseek/deepseek-v4-flash",
	"deepseek/deepseek-v4-flash-vision-exp",
	"deepseek/deepseek-v4.1-flash",
	"deepseek/deepseek-v4.1-flash-fast",
}

func TestNormalizeCommandCodeChatReasoningEffort_RewritesNoneToOff(t *testing.T) {
	for _, model := range commandCodeOffModels {
		t.Run(model, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":%q,"reasoning_effort":"none","messages":[{"role":"user","content":"hello"}],"temperature":0.2}`, model))
			original := bytes.Clone(body)

			got, err := normalizeCommandCodeChatReasoningEffort(commandCodeChatCompletionsURL, body)
			require.NoError(t, err)
			require.Equal(t, "off", gjson.GetBytes(got, "reasoning_effort").String())
			require.Equal(t, model, gjson.GetBytes(got, "model").String())
			require.Equal(t, "hello", gjson.GetBytes(got, "messages.0.content").String())
			// The caller keeps the canonical disable token for retries/failover.
			require.Equal(t, original, body, "input body must not be mutated in place")

			again, err := normalizeCommandCodeChatReasoningEffort(commandCodeChatCompletionsURL, got)
			require.NoError(t, err)
			require.Equal(t, got, again, "second pass must be a no-op")
		})
	}
}

func TestNormalizeCommandCodeChatReasoningEffort_LeavesOtherValuesAlone(t *testing.T) {
	for _, value := range []string{`"off"`, `"minimal"`, `"low"`, `"medium"`, `"high"`, `"xhigh"`, `"max"`, `null`, `false`, `0`, `""`} {
		body := []byte(`{"model":"deepseek/deepseek-v4.1-flash","reasoning_effort":` + value + `}`)
		got, err := normalizeCommandCodeChatReasoningEffort(commandCodeChatCompletionsURL, body)
		require.NoError(t, err)
		require.Equal(t, body, got, value)
	}

	for _, body := range [][]byte{
		nil,
		{},
		[]byte(`{"model":"deepseek/deepseek-v4.1-flash"}`),
		// Responses-shape effort is not the Chat Completions field.
		[]byte(`{"model":"deepseek/deepseek-v4.1-flash","reasoning":{"effort":"none"}}`),
	} {
		got, err := normalizeCommandCodeChatReasoningEffort(commandCodeChatCompletionsURL, body)
		require.NoError(t, err)
		require.Equal(t, body, got)
	}
}

func TestNormalizeCommandCodeChatReasoningEffort_LeavesUnverifiedModelsAlone(t *testing.T) {
	// deepseek-v4-flash-fast refuses "off" as well, and Kimi K3 / GPT models are
	// outside the verified set. Rewriting those would trade one 400 for another.
	for _, model := range []string{
		"deepseek/deepseek-v4-flash-fast",
		"moonshotai/Kimi-K3",
		"gpt-6-sol",
		"zai-org/GLM-5.3",
		"deepseek/deepseek-v4.1-flash-preview",
		"",
	} {
		body := []byte(fmt.Sprintf(`{"model":%q,"reasoning_effort":"none"}`, model))
		got, err := normalizeCommandCodeChatReasoningEffort(commandCodeChatCompletionsURL, body)
		require.NoError(t, err)
		require.Equal(t, body, got, model)
	}
}

func TestNormalizeCommandCodeChatReasoningEffort_LeavesOtherUpstreamsAlone(t *testing.T) {
	// "none" is the disable token those upstreams accept; a shared scheduler pool
	// must not leak Command Code's spelling into them.
	for _, targetURL := range []string{
		"https://opencode.ai/zen/go/v1/chat/completions",
		"https://api.openai.com/v1/chat/completions",
		"https://api.deepseek.com/chat/completions",
		"https://api.commandcode.ai.evil.example/provider/v1/chat/completions",
		"https://proxy.api.commandcode.ai/provider/v1/chat/completions",
		"http://api.commandcode.ai/provider/v1/chat/completions",
		// Go's /alpha/generate is not known to take "off".
		"https://api.commandcode.ai/alpha/generate",
		"https://api.commandcode.ai/provider/v1/responses",
		"",
	} {
		body := []byte(`{"model":"deepseek/deepseek-v4.1-flash","reasoning_effort":"none"}`)
		got, err := normalizeCommandCodeChatReasoningEffort(targetURL, body)
		require.NoError(t, err)
		require.Equal(t, body, got, targetURL)
	}
}

func TestCommandCodeReasonsWithOff(t *testing.T) {
	for _, model := range commandCodeOffModels {
		require.True(t, commandCodeReasonsWithOff(model), model)
		// The same model spelled without the vendor prefix.
		require.True(t, commandCodeReasonsWithOff(strings.TrimPrefix(model, "deepseek/")), model)
	}
	for _, model := range []string{
		"deepseek/deepseek-v4-flash-fast",
		"deepseek/", "deepseek",
		"openai/deepseek-v4.1-flash",
		"moonshotai/deepseek-v4.1-flash",
		"deepseek/deepseek-v4.1-flash-extra",
	} {
		require.False(t, commandCodeReasonsWithOff(model), model)
	}
}

// commandCodeReasoningTestAccount builds a Command Code or comparison account
// that forwards an alias to a concrete DeepSeek upstream model over native Chat
// Completions.
func commandCodeReasoningTestAccount(platform, baseURL string) *Account {
	account := rawChatCompletionsTestAccount()
	account.Platform = platform
	account.Credentials["base_url"] = baseURL
	account.Credentials["api_protocol"] = APIProtocolChatCompletions
	account.Credentials["model_mapping"] = map[string]any{"public-alias": "deepseek/deepseek-v4.1-flash"}
	account.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
	}
	return account
}

func commandCodeReasoningTestResponse(stream bool) (*http.Response, string) {
	payload := `{"id":"chatcmpl_cc","model":"deepseek/deepseek-v4.1-flash","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"delta":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`
	contentType := "application/json"
	if stream {
		payload = "data: " + payload + "\n\ndata: [DONE]\n\n"
		contentType = "text/event-stream"
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(payload)),
	}, payload
}

// TestForwardAsChatCompletions_CommandCodeDisablesThinkingOnTheWire checks the
// end-to-end outbound body for both stream modes, and that the canonical
// "none" is what usage/billing still records.
func TestForwardAsChatCompletions_CommandCodeDisablesThinkingOnTheWire(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"public-alias","messages":[{"role":"user","content":"hello"}],"reasoning_effort":"none","stream":%v}`, stream))
			original := bytes.Clone(body)

			resp, _ := commandCodeReasoningTestResponse(stream)
			upstream := &httpUpstreamRecorder{resp: resp}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			account := commandCodeReasoningTestAccount(PlatformCommandCode, DefaultCommandCodeBaseURL)
			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			require.NoError(t, err)
			require.NotNil(t, result)

			require.Equal(t, "off", gjson.GetBytes(upstream.lastBody, "reasoning_effort").String())
			require.Equal(t, "deepseek/deepseek-v4.1-flash", gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, "/provider/v1/chat/completions", upstream.lastReq.URL.Path)
			require.NotNil(t, result.ReasoningEffort)
			require.Equal(t, "none", *result.ReasoningEffort, "billing keeps the canonical disable bucket")
			require.Equal(t, original, body, "caller body stays canonical for a retry")
			require.Contains(t, rec.Body.String(), "ok")
		})
	}
}

// TestForwardAsChatCompletions_CommandCodeRewriteDoesNotLeakAcrossAccounts
// models the mixed priority pool: the same client body must reach the Command
// Code member as "off" and every other member as "none", in either order, so an
// account retry cannot inherit Command Code's vocabulary.
func TestForwardAsChatCompletions_CommandCodeRewriteDoesNotLeakAcrossAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		platform string
		baseURL  string
		want     string
	}{
		{PlatformCommandCode, DefaultCommandCodeBaseURL, "off"},
		{PlatformOpenCodeGo, "https://opencode.ai/zen/go/v1", "none"},
		{PlatformDeepseek, "https://api.deepseek.com", "none"},
		{PlatformOpenAI, "https://api.openai.com", "none"},
	}

	for _, first := range cases {
		for _, second := range cases {
			name := fmt.Sprintf("%s-then-%s", first.platform, second.platform)
			t.Run(name, func(t *testing.T) {
				body := []byte(`{"model":"public-alias","messages":[{"role":"user","content":"hello"}],"reasoning_effort":"none","stream":false}`)
				original := bytes.Clone(body)

				upstream := &httpUpstreamRecorder{}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

				for _, step := range []struct {
					platform string
					baseURL  string
					want     string
				}{first, second} {
					resp, _ := commandCodeReasoningTestResponse(false)
					upstream.resp = resp
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
					c.Request.Header.Set("Content-Type", "application/json")

					account := commandCodeReasoningTestAccount(step.platform, step.baseURL)
					_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
					require.NoError(t, err)
					require.Equal(t, step.want, gjson.GetBytes(upstream.lastBody, "reasoning_effort").String(), step.platform)
				}
				require.Equal(t, original, body, "the shared client body is never rewritten")
			})
		}
	}
}

// TestForwardAsChatCompletions_CommandCodeKeepsExplicitLevels guards the
// "no global low/off change" requirement: a client asking for real reasoning
// must still get its own level forwarded.
func TestForwardAsChatCompletions_CommandCodeKeepsExplicitLevels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, effort := range []string{"low", "medium", "high", "max"} {
		t.Run(effort, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"public-alias","messages":[{"role":"user","content":"hello"}],"reasoning_effort":%q,"stream":false}`, effort))
			resp, _ := commandCodeReasoningTestResponse(false)
			upstream := &httpUpstreamRecorder{resp: resp}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			account := commandCodeReasoningTestAccount(PlatformCommandCode, DefaultCommandCodeBaseURL)
			_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			require.NoError(t, err)
			require.Equal(t, effort, gjson.GetBytes(upstream.lastBody, "reasoning_effort").String())
		})
	}
}

// TestForwardResponsesViaRawChatCompletions_CommandCodeDisablesThinking covers
// the Responses→Chat bridge, which reaches the same send boundary with the
// effort converted to chatCompletions.reasoning_effort.
func TestForwardResponsesViaRawChatCompletions_CommandCodeDisablesThinking(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"public-alias","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}],"reasoning":{"effort":"none"},"stream":false}`)
	resp, _ := commandCodeReasoningTestResponse(false)
	upstream := &httpUpstreamRecorder{resp: resp}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	account := commandCodeReasoningTestAccount(PlatformCommandCode, DefaultCommandCodeBaseURL)
	result, err := svc.forwardResponsesViaRawChatCompletions(context.Background(), c, account, body, time.Now())
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, "off", gjson.GetBytes(upstream.lastBody, "reasoning_effort").String())
	require.Equal(t, "deepseek/deepseek-v4.1-flash", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "/provider/v1/chat/completions", upstream.lastReq.URL.Path)
}

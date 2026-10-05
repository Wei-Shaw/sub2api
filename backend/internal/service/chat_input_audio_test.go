//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChatInputAudioFullConversionChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for format, mimeType := range map[string]string{"wav": "audio/wav", "mp3": "audio/mpeg", "ogg": "audio/ogg", "flac": "audio/flac", "aac": "audio/aac", "mp4": "audio/mp4", "m4a": "audio/mp4"} {
		for _, route := range []string{"gemini", "antigravity"} {
			t.Run(route+"/"+format, func(t *testing.T) {
				body := []byte(fmt.Sprintf(`{"model":"audio-alias","messages":[{"role":"user","content":[{"type":"text","text":"transcribe"},{"type":"input_audio","input_audio":{"data":"YXVkaW8=","format":%q}},{"type":"text","text":"please"}]}]}`, format))
				c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)
				var sent []byte
				if route == "antigravity" {
					upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
					svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
					account := newAntigravityCompatAccount(AccountTypeOAuth)
					account.Credentials["model_mapping"].(map[string]any)["audio-alias"] = "gemini-3.1-pro-high"
					_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
					require.NoError(t, err)
					require.Len(t, upstream.requestBodies, 1)
					sent = upstream.requestBodies[0]
				} else {
					upstream := &geminiCompatHTTPUpstreamStub{response: antigravityCompatSuccessResponse()}
					svc := &GeminiMessagesCompatService{tokenProvider: &GeminiTokenProvider{}, httpUpstream: upstream, cfg: &config.Config{}}
					account := &Account{ID: 101, Platform: PlatformGemini, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "test-token", "project_id": "test-project"}}
					_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body)
					require.NoError(t, err)
					require.Equal(t, 1, upstream.calls)
					sent, err = io.ReadAll(upstream.lastReq.Body)
					require.NoError(t, err)
				}
				require.Equal(t, http.StatusOK, recorder.Code)
				parts := gjson.GetBytes(sent, "request.contents.0.parts").Array()
				require.Len(t, parts, 3)
				require.Equal(t, "transcribe", parts[0].Get("text").String())
				require.Equal(t, mimeType, parts[1].Get("inlineData.mimeType").String())
				require.Equal(t, "YXVkaW8=", parts[1].Get("inlineData.data").String())
				require.Equal(t, "please", parts[2].Get("text").String())
			})
		}
	}
}

func TestChatInputAudioInvalidOrUnsupportedReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, payload := range []string{
		`{"data":"YXVkaW8=","format":"wav"}`,
		`null`, `{}`, `"audio"`,
		`{"data":123,"format":"wav"}`,
		`{"data":"","format":"wav"}`,
		`{"data":"not base64","format":"wav"}`,
		`{"data":"YXVkaW8=","format":"pcm16"}`,
		`{"data":"YXVkaW8=","format":"audio/wav"}`,
		`{"data":"YXVkaW8="}`,
	} {
		for _, route := range []string{"gemini", "antigravity", "anthropic", "openai-responses", "antigravity-claude"} {
			if payload == `{"data":"YXVkaW8=","format":"wav"}` && (route == "gemini" || route == "antigravity") {
				continue
			}
			t.Run(route+"/"+payload, func(t *testing.T) {
				model := "gemini-3.1-pro-high"
				if route == "antigravity-claude" {
					model = "claude-sonnet-4-5"
				}
				body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":[{"type":"text","text":"transcribe"},{"type":"input_audio","input_audio":%s}]}]}`, model, payload))
				c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)
				var err error
				switch route {
				case "gemini":
					upstream := &geminiCompatHTTPUpstreamStub{}
					_, err = (&GeminiMessagesCompatService{httpUpstream: upstream}).ForwardAsChatCompletions(context.Background(), c, &Account{}, body)
					require.Zero(t, upstream.calls)
				case "anthropic":
					_, err = (&GatewayService{}).ForwardAsChatCompletions(context.Background(), c, &Account{}, body, nil)
				case "openai-responses":
					upstream := &geminiCompatHTTPUpstreamStub{}
					svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
					_, err = svc.ForwardAsChatCompletions(context.Background(), c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body, "", "")
					require.Zero(t, upstream.calls)
				default:
					upstream := &queuedHTTPUpstreamStub{}
					svc := newAntigravityCompatService(config.GatewayConfig{}, upstream)
					_, err = svc.ForwardAsChatCompletions(context.Background(), c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)
					require.Empty(t, upstream.requestBodies)
				}
				require.Error(t, err)
				require.Equal(t, http.StatusBadRequest, recorder.Code)
				require.Equal(t, "invalid_request_error", gjson.Get(recorder.Body.String(), "error.type").String())
				require.Contains(t, recorder.Body.String(), "input_audio")
			})
		}
	}
}

func TestAntigravityChatInputAudioThinkingAwareFamily(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, testCase := range []struct {
		name        string
		highModel   string
		mediumModel string
		allowAudio  bool
	}{
		{"default_gemini_routes_claude", "gemini-3.8-flash-high", "claude-sonnet-4-5", false},
		{"default_claude_routes_gemini", "claude-sonnet-4-5", "gemini-3.8-flash-medium", true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			body := []byte(`{"model":"gemini-3.8-flash","reasoning_effort":"medium","messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"YXVkaW8=","format":"wav"}}]}]}`)
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
			account := newAntigravityCompatAccount(AccountTypeOAuth)
			account.Credentials["model_mapping"] = map[string]any{
				"gemini-3.8-flash-high":   testCase.highModel,
				"gemini-3.8-flash-medium": testCase.mediumModel,
			}
			require.Equal(t, testCase.highModel, svc.getMappedModel(account, "gemini-3.8-flash"))
			_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
			if !testCase.allowAudio {
				require.Error(t, err)
				require.Equal(t, http.StatusBadRequest, recorder.Code)
				require.Equal(t, "invalid_request_error", gjson.Get(recorder.Body.String(), "error.type").String())
				require.Contains(t, recorder.Body.String(), "input_audio")
				require.Empty(t, upstream.requestBodies)
				return
			}
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Len(t, upstream.requestBodies, 1)
			require.Equal(t, testCase.mediumModel, gjson.GetBytes(upstream.requestBodies[0], "model").String())
			require.Contains(t, string(upstream.requestBodies[0]), `"mimeType":"audio/wav"`)
			require.Contains(t, string(upstream.requestBodies[0]), `"data":"YXVkaW8="`)
		})
	}
}

func TestChatInputAudioRawPassthroughUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"audio-model","messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"YXVkaW8=","format":"pcm16"}}]}]}`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"chatcmpl_audio","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := rawChatCompletionsTestAccount()
	account.Extra = map[string]any{"openai_responses_supported": false}
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, gjson.GetBytes(body, "messages").Raw, gjson.GetBytes(upstream.lastBody, "messages").Raw)
}

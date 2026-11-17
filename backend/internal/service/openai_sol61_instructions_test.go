package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSol61InstructionsResponsesForward(t *testing.T) {
	for _, tc := range []struct {
		name    string
		present bool
		value   any
		want    string
	}{
		{name: "absent"},
		{name: "empty", present: true, value: ""},
		{name: "null", present: true},
		{name: "whitespace", present: true, value: " \n\t"},
		{name: "caller text", present: true, value: "  Return JSON only.\n", want: "  Return JSON only.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{
				"model": "public-sol", "input": "Reply with only OK.", "stream": true,
			}
			if tc.present {
				payload["instructions"] = tc.value
			}
			body, err := json.Marshal(payload)
			require.NoError(t, err)
			wire := captureSol61InstructionsForward(t, AccountTypeOAuth, false, body)
			require.Equal(t, "gpt-6.1-sol", gjson.GetBytes(wire, "model").String())
			require.True(t, gjson.GetBytes(wire, "instructions").Exists())
			require.Equal(t, tc.want, gjson.GetBytes(wire, "instructions").String())
			require.Equal(t, 1, strings.Count(string(wire), "Reply with only OK."))
		})
	}
}

func TestSol61InstructionsTransformAliasesAndClientModes(t *testing.T) {
	for _, model := range []string{"gpt-6.1-sol", "openai/gpt-6.1-sol", "GPT-6.1-SOL", "gpt-6.1-sol-xhigh", "gpt-6.1-sol-openai-compact"} {
		for _, cli := range []bool{false, true} {
			t.Run(model+"/"+map[bool]string{false: "api", true: "cli"}[cli], func(t *testing.T) {
				body := map[string]any{
					"model": model,
					"input": []any{map[string]any{"type": "message", "role": "user", "content": "hello"}},
				}
				result := applyCodexOAuthTransformWithOptions(body, codexOAuthTransformOptions{IsCodexCLI: cli})
				require.NoError(t, result.Error)
				require.Equal(t, "", body["instructions"])
				require.False(t, applyInstructions(body, cli), "an already empty default must not be marked as another rewrite")
			})
		}
	}
}

func TestSol61InstructionsChatBridge(t *testing.T) {
	for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
		for _, tc := range []struct {
			name         string
			messages     string
			extra        string
			want         string
			inputCount   int64
			promptCopies int
		}{
			{name: "no system", messages: `{"role":"user","content":"hello"}`, want: "", inputCount: 1},
			{name: "caller system", messages: `{"role":"system","content":"Caller-owned guidance."},{"role":"user","content":"hello"}`, want: "Caller-owned guidance.", inputCount: 1, promptCopies: 1},
			{name: "developer retained", messages: `{"role":"developer","content":"Caller-owned guidance."},{"role":"user","content":"hello"}`, want: "", inputCount: 2, promptCopies: 1},
			{name: "json mode retains guidance", messages: `{"role":"system","content":"Caller-owned guidance. Return JSON."},{"role":"user","content":"hello"}`, extra: `,"response_format":{"type":"json_object"}`, want: "Caller-owned guidance. Return JSON.", inputCount: 2, promptCopies: 2},
			{name: "mixed system keeps image", messages: `{"role":"system","content":[{"type":"text","text":"Caller-owned guidance."},{"type":"image_url","image_url":{"url":"https://example.com/reference.png"}}]},{"role":"user","content":"hello"}`, want: "Caller-owned guidance.", inputCount: 2, promptCopies: 2},
		} {
			t.Run(accountType+"/"+tc.name, func(t *testing.T) {
				body := []byte(`{"model":"public-sol","messages":[` + tc.messages + `],"stream":false` + tc.extra + `}`)
				wire := captureSol61InstructionsForward(t, accountType, true, body)
				require.True(t, gjson.GetBytes(wire, "instructions").Exists())
				require.Equal(t, tc.want, gjson.GetBytes(wire, "instructions").String())
				require.Equal(t, tc.inputCount, gjson.GetBytes(wire, "input.#").Int())
				require.Equal(t, tc.promptCopies, strings.Count(string(wire), "Caller-owned guidance."))
				if tc.name == "mixed system keeps image" {
					require.Contains(t, string(wire), "https://example.com/reference.png")
				}
			})
		}
	}
}

func captureSol61InstructionsForward(t *testing.T, accountType string, chat bool, body []byte) []byte {
	t.Helper()
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{err: errors.New("stop after capturing outbound request")}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 1, Platform: PlatformOpenAI, Type: accountType, Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "fixture-key", "base_url": "https://upstream.example",
			"access_token": "fixture-token", "chatgpt_account_id": "fixture-account",
			"model_mapping": map[string]any{"public-sol": "gpt-6.1-sol"},
		},
		Extra: map[string]any{"openai_responses_supported": true},
	}
	path := "/v1/responses"
	if chat {
		path = "/v1/chat/completions"
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	var err error
	if chat {
		_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	} else {
		_, err = svc.Forward(context.Background(), c, account, body)
	}
	require.Error(t, err)
	require.NotNil(t, upstream.lastReq, "the test must reach the outbound boundary")
	require.NotEmpty(t, upstream.lastBody)
	return upstream.lastBody
}

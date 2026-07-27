package service

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayAccountNoticeText(t *testing.T) {
	rate := 1.25
	account := &Account{ID: 42, Name: "primary\n\taccount", RateMultiplier: &rate}

	require.Equal(t,
		"\x1b[93m[Corgi AI Gateway]\x1b[97m 当前正在使用 \x1b[96m#42 primary account\x1b[97m 账号，计费倍率 \x1b[96m1.25\x1b[0m\n",
		GatewayAccountNoticeText(account),
	)

	zero := 0.0
	require.Contains(t, GatewayAccountNoticeText(&Account{ID: 7, Name: "free", RateMultiplier: &zero}), "\x1b[96m0\x1b[0m\n")
	require.Contains(t, GatewayAccountNoticeText(&Account{ID: 8, Name: "legacy"}), "\x1b[96m1\x1b[0m\n")
	invalid := math.NaN()
	require.Contains(t, GatewayAccountNoticeText(&Account{ID: 9, Name: "invalid", RateMultiplier: &invalid}), "\x1b[96m1\x1b[0m\n")
}

func TestStripGatewayAccountNoticeFromBody(t *testing.T) {
	notice := "[Corgi AI Gateway] banner\\n"
	tests := []struct {
		name     string
		body     string
		expected string
		changed  bool
	}{
		{
			name:     "openai messages preserves user text and answer",
			body:     `{"messages":[{"role":"user","content":"[Corgi AI Gateway] user supplied"},{"role":"assistant","content":"` + notice + `actual answer"}]}`,
			expected: `{"messages":[{"role":"user","content":"[Corgi AI Gateway] user supplied"},{"role":"assistant","content":"actual answer"}]}`,
			changed:  true,
		},
		{
			name:     "pure notification assistant message is removed",
			body:     `{"messages":[{"role":"assistant","content":"` + notice + `"},{"role":"user","content":"next"}]}`,
			expected: `{"messages":[{"role":"user","content":"next"}]}`,
			changed:  true,
		},
		{
			name:     "tool call message remains after text block removal",
			body:     `{"messages":[{"role":"assistant","content":[{"type":"text","text":"` + notice + `"}],"tool_calls":[{"id":"call_1"}]}]}`,
			expected: `{"messages":[{"role":"assistant","tool_calls":[{"id":"call_1"}]}]}`,
			changed:  true,
		},
		{
			name:     "responses input text block",
			body:     `{"input":[{"type":"message","role":"assistant","content":[{"type":"input_text","text":"` + notice + `answer"}]}]}`,
			expected: `{"input":[{"type":"message","role":"assistant","content":[{"type":"input_text","text":"answer"}]}]}`,
			changed:  true,
		},
		{
			name:     "anthropic text block",
			body:     `{"messages":[{"role":"assistant","content":[{"type":"text","text":"[Corgi AI Gateway] old banner\nanswer"}]}]}`,
			expected: `{"messages":[{"role":"assistant","content":[{"type":"text","text":"answer"}]}]}`,
			changed:  true,
		},
		{
			name:     "gemini model parts",
			body:     `{"contents":[{"role":"model","parts":[{"text":"` + notice + `answer"},{"functionCall":{"name":"lookup"}}]}]}`,
			expected: `{"contents":[{"role":"model","parts":[{"text":"answer"},{"functionCall":{"name":"lookup"}}]}]}`,
			changed:  true,
		},
		{
			name:     "unrelated assistant content is unchanged",
			body:     `{"messages":[{"role":"assistant","content":"ordinary answer"}]}`,
			expected: `{"messages":[{"role":"assistant","content":"ordinary answer"}]}`,
			changed:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cleaned, changed := StripGatewayAccountNoticeFromBody([]byte(test.body))
			require.Equal(t, test.changed, changed)
			require.JSONEq(t, test.expected, string(cleaned))
		})
	}

	body, err := json.Marshal(map[string]any{
		"messages": []any{map[string]any{
			"role":    "assistant",
			"content": GatewayAccountNoticeText(&Account{ID: 1, Name: "one"}) + "answer",
		}},
	})
	require.NoError(t, err)
	cleaned, changed := StripGatewayAccountNoticeFromBody(body)
	require.True(t, changed)
	var request map[string]any
	require.NoError(t, json.Unmarshal(cleaned, &request))
	messages := request["messages"].([]any)
	require.Equal(t, "answer", messages[0].(map[string]any)["content"])
}

func TestGatewayAccountNoticeInjectionUsesSelectedProtocol(t *testing.T) {
	state := &gatewayAccountNoticeState{text: "notice\n", mode: GatewayAccountNoticeOpenAIResponses}
	payload := map[string]any{"type": "response.output_text.delta", "delta": "answer"}
	require.True(t, prependGatewayNoticeToPayload(payload, state))
	require.Equal(t, "notice\nanswer", payload["delta"])
	require.True(t, state.injected)

	state = &gatewayAccountNoticeState{text: "notice\n", mode: GatewayAccountNoticeGemini}
	payload = map[string]any{"type": "response.output_text.delta", "delta": "answer"}
	require.False(t, prependGatewayNoticeToPayload(payload, state))
	require.False(t, state.injected)

	state = &gatewayAccountNoticeState{text: "notice\n", mode: GatewayAccountNoticeOpenAIChat}
	var response map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"choices":[{"message":{"role":"assistant","content":"answer"}}]}`), &response))
	require.True(t, prependGatewayNoticeToPayload(response, state))
	choices := response["choices"].([]any)
	require.Equal(t, "notice\nanswer", choices[0].(map[string]any)["message"].(map[string]any)["content"])
}

func TestGatewayAccountNoticeDoesNotInjectIntoEmptyOrToolOnlyOutput(t *testing.T) {
	state := &gatewayAccountNoticeState{text: "notice\n", mode: GatewayAccountNoticeOpenAIChat}
	payload := map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"id": "call_1"}}}}}}
	require.False(t, prependGatewayNoticeToPayload(payload, state))
	require.False(t, state.injected)
}

//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesInlineSystemCapabilityGate(t *testing.T) {
	input := json.RawMessage(`[{"role":"user","content":"q"},{"role":"developer","content":"policy"}]`)
	for _, tc := range []struct {
		name, model, accountType string
		optOut, want             bool
	}{
		{"opus 5.5 api key", "claude-opus-5-5", AccountTypeAPIKey, false, true},
		{"sonnet 5.5 api key", "claude-sonnet-5-5", AccountTypeAPIKey, false, true},
		{"provider prefix", "vendor/claude-opus-5-5", AccountTypeAPIKey, false, true},
		{"claude 4.5 hoists", "claude-sonnet-4-5", AccountTypeAPIKey, false, false},
		{"non-claude model", "k3", AccountTypeAPIKey, false, false},
		{"oauth account", "claude-opus-5-5", AccountTypeOAuth, false, false},
		{"account opt-out", "claude-opus-5-5", AccountTypeAPIKey, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := responsesToAnthropicForAccount(
				&apicompat.ResponsesRequest{Model: tc.model, Input: input},
				&Account{Type: tc.accountType, Extra: map[string]any{"disable_mid_conversation_system": tc.optOut}},
			)
			require.NoError(t, err)
			require.Equal(t, tc.want, out.Messages[len(out.Messages)-1].Role == "system")
		})
	}
}

// Late developer instructions must not mutate the already-sent top-level
// system prompt, tools, or earlier message blocks; otherwise Anthropic prompt
// caching cannot hit on multi-turn conversations. New instructions become an
// inline system message appended at the current conversation boundary.
func TestResponsesInlineSystemKeepsStableOutboundPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	input := []map[string]any{{"role": "system", "content": "stable policy"}, {"role": "user", "content": "first"}}
	var previous []byte
	for round := 0; round < 3; round++ {
		if round > 0 {
			input = append(input,
				map[string]any{"role": "assistant", "content": "pong"},
				map[string]any{"role": "developer", "content": "new runtime policy"},
				map[string]any{"role": "user", "content": "next"},
			)
		}
		body, err := json.Marshal(map[string]any{"model": "public-alias", "stream": true, "input": input})
		require.NoError(t, err)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/responses", strings.NewReader(string(body)))
		account := nativeAnthropicTestAccount()
		account.Platform = PlatformAnthropic
		account.Credentials["model_mapping"] = map[string]any{"public-alias": "claude-opus-5-5"}
		upstream := &httpUpstreamRecorder{resp: nativeAnthropicStreamResponse()}
		svc := &GatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
		_, err = svc.ForwardAsResponses(context.Background(), c, account, body, nil)
		require.NoError(t, err)

		sent := upstream.lastBody
		require.Equal(t, "claude-opus-5-5", gjson.GetBytes(sent, "model").String())
		require.Equal(t, "stable policy", gjson.GetBytes(sent, "system").String())
		messages := gjson.GetBytes(sent, "messages").Array()
		require.NotEmpty(t, messages)
		if round > 0 {
			prevMessages := gjson.GetBytes(previous, "messages").Array()
			require.Len(t, messages, len(prevMessages)+3)
			for i, msg := range prevMessages {
				require.JSONEq(t, msg.Raw, messages[i].Raw)
			}
			last := messages[len(messages)-1]
			require.Equal(t, "system", last.Get("role").String())
			require.Equal(t, "new runtime policy", last.Get("content.0.text").String())
		}
		previous = append([]byte(nil), sent...)
	}
}

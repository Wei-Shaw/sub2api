//go:build unit

package service

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// responsesBodyToAnthropicForCacheTest 复刻 ForwardAsResponses / native anthropic
// 路径上的转换步骤，用于断言真正发往上游的 Anthropic body。
func responsesBodyToAnthropicForCacheTest(t *testing.T, raw string) []byte {
	t.Helper()

	adapted, _, err := adaptResponsesClientToolsForAnthropic([]byte(raw))
	require.NoError(t, err)

	var req apicompat.ResponsesRequest
	require.NoError(t, json.Unmarshal(adapted, &req))

	anthropicReq, err := apicompat.ResponsesToAnthropicRequest(&req)
	require.NoError(t, err)

	body, err := json.Marshal(anthropicReq)
	require.NoError(t, err)

	body = StripEmptyTextBlocks(body)
	body = applyResponsesAnthropicCacheBreakpoints(body, req.Model)
	return enforceCacheControlLimit(body)
}

func countAnthropicCacheBreakpoints(body []byte) int {
	count := 0
	gjson.GetBytes(body, "messages").ForEach(func(_, msg gjson.Result) bool {
		msg.Get("content").ForEach(func(_, block gjson.Result) bool {
			if block.Get("cache_control").Exists() {
				count++
			}
			return true
		})
		return true
	})
	gjson.GetBytes(body, "system").ForEach(func(_, block gjson.Result) bool {
		if block.Get("cache_control").Exists() {
			count++
		}
		return true
	})
	gjson.GetBytes(body, "tools").ForEach(func(_, tool gjson.Result) bool {
		if tool.Get("cache_control").Exists() {
			count++
		}
		return true
	})
	return count
}

// Codex 每轮都会重新发整段 input。断点必须落在“本轮最后一条 message”上，
// 缓存前缀才会随对话增长；否则上游只会写一次 tools 固定前缀。
func TestApplyResponsesAnthropicCacheBreakpoints_FollowsLatestTurn(t *testing.T) {
	t.Parallel()

	firstTurn := responsesBodyToAnthropicForCacheTest(t, `{
		"model":"claude-opus-5-5",
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"turn one"}]}
		],
		"tools":[{"type":"function","name":"exec","description":"run","parameters":{"type":"object","properties":{}}}]
	}`)

	require.Equal(t, "ephemeral", gjson.GetBytes(firstTurn, "messages.0.content.0.cache_control.type").String())
	require.Equal(t, "5m", gjson.GetBytes(firstTurn, "messages.0.content.0.cache_control.ttl").String())

	secondTurn := responsesBodyToAnthropicForCacheTest(t, `{
		"model":"claude-opus-5-5",
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"turn one"}]},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer one"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"turn two"}]},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer two"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"turn three"}]}
		],
		"tools":[{"type":"function","name":"exec","description":"run","parameters":{"type":"object","properties":{}}}]
	}`)

	messages := gjson.GetBytes(secondTurn, "messages").Array()
	require.Len(t, messages, 5)
	lastIdx := len(messages) - 1
	require.Equal(t, "ephemeral", gjson.GetBytes(secondTurn, "messages."+strconv.Itoa(lastIdx)+".content.0.cache_control.type").String())
	// 另一个断点落在倒数第二个 user turn，与 Parrot 语义一致。
	require.Equal(t, "ephemeral", gjson.GetBytes(secondTurn, "messages.2.content.0.cache_control.type").String())
	require.False(t, gjson.GetBytes(secondTurn, "messages.0.content.0.cache_control").Exists())
	require.Equal(t, 2, countAnthropicCacheBreakpoints(secondTurn))
	require.LessOrEqual(t, countAnthropicCacheBreakpoints(secondTurn), 4)
}

func TestApplyResponsesAnthropicCacheBreakpoints_Idempotent(t *testing.T) {
	t.Parallel()

	body := []byte(`{"messages":[
		{"role":"user","content":[{"type":"text","text":"q1"}]},
		{"role":"assistant","content":[{"type":"text","text":"a1"}]},
		{"role":"user","content":[{"type":"text","text":"q2"}]}
	]}`)

	once := applyResponsesAnthropicCacheBreakpoints(body, "claude-opus-5-5")
	require.JSONEq(t, string(once), string(applyResponsesAnthropicCacheBreakpoints(once, "claude-opus-5-5")))
}

// thinking / redacted_thinking 不允许带 cache_control，断点必须回退到前一个 block。
func TestApplyResponsesAnthropicCacheBreakpoints_SkipsThinkingBlocks(t *testing.T) {
	t.Parallel()

	body := []byte(`{"messages":[{"role":"assistant","content":[
		{"type":"text","text":"visible"},
		{"type":"thinking","thinking":"hidden","signature":"sig"}
	]}]}`)

	out := applyResponsesAnthropicCacheBreakpoints(body, "claude-opus-5-5")
	require.Equal(t, "ephemeral", gjson.GetBytes(out, "messages.0.content.0.cache_control.type").String())
	require.False(t, gjson.GetBytes(out, "messages.0.content.1.cache_control").Exists())

	onlyThinking := []byte(`{"messages":[{"role":"assistant","content":[
		{"type":"thinking","thinking":"hidden","signature":"sig"}
	]}]}`)
	require.JSONEq(t, string(onlyThinking), string(applyResponsesAnthropicCacheBreakpoints(onlyThinking, "claude-opus-5-5")))
}

// DeepSeek / Kimi 等 Anthropic 兼容端点不保证接受 cache_control.ttl，保持原样透传。
func TestApplyResponsesAnthropicCacheBreakpoints_NonClaudeModelsUntouched(t *testing.T) {
	t.Parallel()

	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
	for _, model := range []string{"deepseek-v4-flash", "kimi-k2.5", "glm-5.3", ""} {
		require.JSONEq(t, string(body), string(applyResponsesAnthropicCacheBreakpoints(body, model)))
	}
}

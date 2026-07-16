package service

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// 第三方 Claude 兼容上游 (DeepSeek/Kimi/GLM 等) 要求历史 thinking block 原样回传，
// 任何过滤都会破坏「thinking 必须 round-trip」契约。这些测试锁住「mappedModel 命中
// passback-required 时，3 个过滤函数都返回原 body」的行为，避免回归。
// 详见 .pensieve/short-term/knowledge/thinking-block-filter-third-party-upstream-inversion/

const passbackThinkingBody = `{
	"model":"deepseek-v4-pro",
	"thinking":{"type":"enabled","budget_tokens":1024},
	"messages":[
		{"role":"user","content":[{"type":"text","text":"Hi"}]},
		{"role":"assistant","content":[
			{"type":"thinking","thinking":"Let me think..."},
			{"type":"text","text":"Answer"}
		]}
	]
}`

func TestFilterThinkingBlocks_SkipsForPassbackRequired(t *testing.T) {
	in := []byte(passbackThinkingBody)
	out := FilterThinkingBlocks(in, "deepseek-v4-pro")
	// passback-required: 原样回传 body（byte-for-byte 不变）
	require.True(t, bytes.Equal(in, out), "passback-required 上游不应过滤 thinking block")
}

func TestFilterThinkingBlocksForRetry_SkipsForPassbackRequired(t *testing.T) {
	in := []byte(passbackThinkingBody)
	out := FilterThinkingBlocksForRetry(in, "kimi-coding")
	require.True(t, bytes.Equal(in, out), "passback-required 上游 retry 不应剥离 thinking block")
}

func TestFilterSignatureSensitiveBlocksForRetry_SkipsForPassbackRequired(t *testing.T) {
	in := []byte(passbackThinkingBody)
	out := FilterSignatureSensitiveBlocksForRetry(in, "glm-5.1")
	require.True(t, bytes.Equal(in, out), "passback-required 上游不应降级 thinking/tool block")
}

// 反向验证：anthropic-strict 路径仍然按原逻辑剥离无 signature 的 thinking block
func TestFilterThinkingBlocks_StripsForAnthropicStrict(t *testing.T) {
	in := []byte(passbackThinkingBody)
	out := FilterThinkingBlocks(in, "claude-sonnet-4-5")
	// anthropic-strict: thinking.type=enabled 且 thinking block 无 signature → 应剥离
	require.False(t, bytes.Equal(in, out), "anthropic-strict 上游应剥离无 signature 的 thinking block")
	require.NotContains(t, string(out), `"type":"thinking"`)
}

// Unknown 协议族保守不过滤（与 passback-required 一致）
func TestFilterThinkingBlocks_SkipsForUnknownModel(t *testing.T) {
	in := []byte(passbackThinkingBody)
	out := FilterThinkingBlocks(in, "yi-large")
	require.True(t, bytes.Equal(in, out), "unknown 协议族保守不过滤")
}

func TestFilterThinkingBlocks_SkipsForEmptyModel(t *testing.T) {
	in := []byte(passbackThinkingBody)
	out := FilterThinkingBlocks(in, "")
	require.True(t, bytes.Equal(in, out), "空 model 保守不过滤")
}

// retry 路径上 unknown 与 passback-required 行为对称：都返回原 body。
// 避免「预过滤跳过但 retry 仍剥离」的语义裂缝。
func TestFilterThinkingBlocksForRetry_SkipsForUnknownModel(t *testing.T) {
	in := []byte(passbackThinkingBody)
	out := FilterThinkingBlocksForRetry(in, "yi-large")
	require.True(t, bytes.Equal(in, out), "unknown 协议族 retry 不应剥离 thinking block")
}

func TestFilterSignatureSensitiveBlocksForRetry_SkipsForUnknownModel(t *testing.T) {
	in := []byte(passbackThinkingBody)
	out := FilterSignatureSensitiveBlocksForRetry(in, "gpt-5.1")
	require.True(t, bytes.Equal(in, out), "unknown 协议族不应降级 thinking/tool block")
}

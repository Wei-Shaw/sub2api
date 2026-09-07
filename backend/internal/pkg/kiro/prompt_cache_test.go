package kiro

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func promptCacheTestSystemBlock(text string, ttl string) apicompat.AnthropicContentBlock {
	b := apicompat.AnthropicContentBlock{Type: "text", Text: text}
	if ttl != "" {
		b.CacheControl = &apicompat.AnthropicCacheControl{Type: "ephemeral", TTL: ttl}
	}
	return b
}

func promptCacheTestSystemRaw(t *testing.T, blocks ...apicompat.AnthropicContentBlock) []byte {
	t.Helper()
	raw, err := json.Marshal(blocks)
	require.NoError(t, err)
	return raw
}

func promptCacheTestMessage(t *testing.T, role, text string) apicompat.AnthropicMessage {
	t.Helper()
	raw, err := json.Marshal(text)
	require.NoError(t, err)
	return apicompat.AnthropicMessage{Role: role, Content: raw}
}

func TestPromptCacheTrackerComputeAndUpdate(t *testing.T) {
	tracker := NewPromptCacheTracker()
	longSystem := strings.Repeat("You are a helpful coding assistant with deep knowledge of Go, Rust, Python, and TypeScript. ", 80)

	req := &apicompat.AnthropicRequest{
		Model:  "claude-sonnet-4-5",
		System: promptCacheTestSystemRaw(t, promptCacheTestSystemBlock(longSystem, "5m")),
		Messages: []apicompat.AnthropicMessage{
			promptCacheTestMessage(t, "user", "hello world"),
		},
	}

	profile := BuildPromptCacheProfile(req, 120)
	require.NotNil(t, profile, "expected cache profile to be built")

	first := tracker.Compute(1, profile)
	require.Positive(t, first.CacheCreationInputTokens, "first request should create cache tokens")
	require.Zero(t, first.CacheReadInputTokens, "first request should have zero cache reads")

	tracker.Update(1, profile)
	second := tracker.Compute(1, profile)
	require.Positive(t, second.CacheReadInputTokens, "repeated request should read cache tokens")
	require.Zero(t, second.CacheCreationInputTokens, "repeated request should avoid cache creation")
}

func TestPromptCacheStableAcrossBillingHeaderDrift(t *testing.T) {
	tracker := NewPromptCacheTracker()
	mainSystem := strings.Repeat("You are a helpful coding assistant with deep knowledge of Go, Rust, Python, and TypeScript. ", 80)

	build := func(billingHdr string) *apicompat.AnthropicRequest {
		return &apicompat.AnthropicRequest{
			Model: "claude-sonnet-4-5",
			System: promptCacheTestSystemRaw(t,
				promptCacheTestSystemBlock(billingHdr, ""),
				promptCacheTestSystemBlock(mainSystem, "5m"),
			),
			Messages: []apicompat.AnthropicMessage{promptCacheTestMessage(t, "user", "hello world")},
		}
	}

	profile1 := BuildPromptCacheProfile(build("x-anthropic-billing-header: cc_version=2.1.87.1; cch=aaaa;"), 2048)
	require.NotNil(t, profile1)
	first := tracker.Compute(1, profile1)
	require.Zero(t, first.CacheReadInputTokens)
	tracker.Update(1, profile1)

	profile2 := BuildPromptCacheProfile(build("x-anthropic-billing-header: cc_version=2.1.87.42; cch=bbbb; padding=xxyyzz;"), 2048)
	require.NotNil(t, profile2)
	second := tracker.Compute(1, profile2)
	require.Positive(t, second.CacheReadInputTokens, "billing header drift must not break the fingerprint match")
}

func TestPromptCacheImplicitBreakpointAtMessageEnd(t *testing.T) {
	tracker := NewPromptCacheTracker()
	systemText := strings.Repeat("You are a helpful coding assistant with deep knowledge of Go, Rust, Python, and TypeScript. ", 80)
	systemRaw := promptCacheTestSystemRaw(t, promptCacheTestSystemBlock(systemText, "5m"))

	req1 := &apicompat.AnthropicRequest{
		Model:    "claude-sonnet-4-5",
		System:   systemRaw,
		Messages: []apicompat.AnthropicMessage{promptCacheTestMessage(t, "user", "question one")},
	}
	profile1 := BuildPromptCacheProfile(req1, 2048)
	require.NotNil(t, profile1)
	tracker.Update(1, profile1)

	req2 := &apicompat.AnthropicRequest{
		Model:  "claude-sonnet-4-5",
		System: systemRaw,
		Messages: []apicompat.AnthropicMessage{
			promptCacheTestMessage(t, "user", "question one"),
			promptCacheTestMessage(t, "assistant", "answer one"),
			promptCacheTestMessage(t, "user", "follow-up question"),
		},
	}
	profile2 := BuildPromptCacheProfile(req2, 4096)
	require.NotNil(t, profile2)
	result := tracker.Compute(1, profile2)
	require.Positive(t, result.CacheReadInputTokens, "later turns should hit the prefix via an implicit message-end breakpoint")
}

func TestPromptCacheDifferentAccountsDoNotShareState(t *testing.T) {
	tracker := NewPromptCacheTracker()
	systemText := strings.Repeat("shared system prompt content that is long enough to be cacheable. ", 80)
	req := &apicompat.AnthropicRequest{
		Model:    "claude-sonnet-4-5",
		System:   promptCacheTestSystemRaw(t, promptCacheTestSystemBlock(systemText, "5m")),
		Messages: []apicompat.AnthropicMessage{promptCacheTestMessage(t, "user", "hi")},
	}
	profile := BuildPromptCacheProfile(req, 2048)
	require.NotNil(t, profile)

	tracker.Update(1, profile)
	require.NotEmpty(t, tracker.byAccount[1], "sanity check: the fixture must actually be cacheable, or this test proves nothing")

	// A different account must not benefit from account 1's cache entries —
	// Kiro's real (or simulated) cache is isolated per credential.
	result := tracker.Compute(2, profile)
	require.Zero(t, result.CacheReadInputTokens)
}

func TestPromptCacheEntryExpiresAfterTTL(t *testing.T) {
	tracker := NewPromptCacheTracker()
	systemText := strings.Repeat("shared system prompt content that is long enough to be cacheable. ", 80)
	req := &apicompat.AnthropicRequest{
		Model:    "claude-sonnet-4-5",
		System:   promptCacheTestSystemRaw(t, promptCacheTestSystemBlock(systemText, "5m")),
		Messages: []apicompat.AnthropicMessage{promptCacheTestMessage(t, "user", "hi")},
	}
	profile := BuildPromptCacheProfile(req, 2048)
	require.NotNil(t, profile)

	tracker.Update(1, profile)
	// Manually expire the stored entry instead of sleeping 5 minutes in a test.
	for fp, entry := range tracker.byAccount[1] {
		entry.expiresAt = time.Now().Add(-time.Second)
		tracker.byAccount[1][fp] = entry
	}

	result := tracker.Compute(1, profile)
	require.Zero(t, result.CacheReadInputTokens, "expired entries must not count as a cache hit")
}

// TestPromptCacheCreationSplitNeverExceedsCreationTotal 覆盖真实数据复查
// 发现的 bug：85% 上限（maxCacheable）把 lastTokens 从"最新断点的原始
// cumulative"往下裁剪之后，5m/1h 拆分不能继续按未裁剪的
// profile.totalInputTokens 去累加到最新断点——那样算出来的 5m/1h 之和会
// 超过真正报出去的 cache_creation_input_tokens，破坏 Anthropic 协议要求的
// "5m + 1h == cache_creation_input_tokens" 这条守恒不变式（真实账号数据里
// 复现：cache_creation_tokens=0 但 cache_creation_5m_tokens 顶着和
// input_tokens 相同大小的非零值，不是假设性场景）。
//
// 直接手工构造 profile（本文件与被测代码同包，可以直接写未导出字段）而不是
// 靠 BuildPromptCacheProfile 从文本反推——这样才能精确摆出触发条件：一个
// 已经命中过的旧断点 A，和一个原始 cumulative 明显超过 85% 上限、导致
// lastTokens 被下压到低于 A 与"新断点"之间自然差值的最新断点 B。
func TestPromptCacheCreationSplitNeverExceedsCreationTotal(t *testing.T) {
	var fpA, fpB [32]byte
	fpA[0] = 0xAA
	fpB[0] = 0xBB

	tracker := NewPromptCacheTracker()

	// 先用一份只含断点 A 的 profile 做 Update，让它的指纹进入该账号的记录。
	seedProfile := &PromptCacheProfile{
		model:            "claude-sonnet-5",
		totalInputTokens: 8000,
		breakpoints: []promptCacheBreakpoint{
			{fingerprint: fpA, cumulativeTokens: 8000, ttl: 5 * time.Minute},
		},
	}
	tracker.Update(1, seedProfile)

	// 总量 10000，85% 上限 = 8500；断点 B（最新一轮）原始 cumulative 是
	// 9900，明显超过上限——lastTokens 会被下压到 8500。matched 断点 A 的
	// cumulative 是 8000，真正的 creation 只有 8500-8000=500，但未裁剪的
	// walk 会一路走到 9900，算出 1900，远超真正的 creation。
	profile := &PromptCacheProfile{
		model:            "claude-sonnet-5",
		totalInputTokens: 10000,
		breakpoints: []promptCacheBreakpoint{
			{fingerprint: fpA, cumulativeTokens: 8000, ttl: 5 * time.Minute},
			{fingerprint: fpB, cumulativeTokens: 9900, ttl: 5 * time.Minute},
		},
	}
	result := tracker.Compute(1, profile)

	require.Equal(t, 500, result.CacheCreationInputTokens, "85% 上限裁剪后真正的 creation 只有 500")
	require.Equal(t, 8000, result.CacheReadInputTokens)
	require.Equal(t, result.CacheCreationInputTokens, result.CacheCreation5mInputTokens+result.CacheCreation1hInputTokens,
		"5m+1h 必须恒等于 cache_creation_input_tokens，不能超出（bug 修复前这里会算出 1900）")
}

// TestPromptCacheInvalidatesOnModelChange 覆盖对照 Kiro-Go 真实源码复查发现
// 的缺口：prelude 段（model + tool_choice）此前完全没有参与哈希，导致同一
// 账号中途切换模型时，明明处理方式已经不同，却因为其余内容一样而被判定
// 命中——把不该给的缓存折扣算给了账号，是计费公平性问题。
func TestPromptCacheInvalidatesOnModelChange(t *testing.T) {
	tracker := NewPromptCacheTracker()
	longSystem := strings.Repeat("You are a helpful coding assistant with deep knowledge of Go, Rust, Python, and TypeScript. ", 80)

	build := func(model string) *apicompat.AnthropicRequest {
		return &apicompat.AnthropicRequest{
			Model:    model,
			System:   promptCacheTestSystemRaw(t, promptCacheTestSystemBlock(longSystem, "5m")),
			Messages: []apicompat.AnthropicMessage{promptCacheTestMessage(t, "user", "hello world")},
		}
	}

	// 特意用同一档位（都非 opus，minCacheableTokens 阈值相同）的两个模型名——
	// 避免和"opus 阈值更高"这条独立规则混在一起，保证测的确实是 prelude 是否
	// 参与哈希，而不是阈值差异带来的副作用。
	profile1 := BuildPromptCacheProfile(build("claude-sonnet-4-5"), 120)
	require.NotNil(t, profile1)
	tracker.Update(1, profile1)

	profile2 := BuildPromptCacheProfile(build("claude-sonnet-4-6"), 120)
	require.NotNil(t, profile2)
	result := tracker.Compute(1, profile2)
	require.Zero(t, result.CacheReadInputTokens, "switching model mid-conversation must not be reported as a cache hit")
}

// TestPromptCacheInvalidatesOnToolChoiceChange 覆盖同一份缺口的另一半：
// tool_choice 从 auto 切到强制某个工具时，同样不能被判定命中。
func TestPromptCacheInvalidatesOnToolChoiceChange(t *testing.T) {
	tracker := NewPromptCacheTracker()
	longSystem := strings.Repeat("You are a helpful coding assistant with deep knowledge of Go, Rust, Python, and TypeScript. ", 80)

	build := func(toolChoice string) *apicompat.AnthropicRequest {
		return &apicompat.AnthropicRequest{
			Model:      "claude-sonnet-4-5",
			System:     promptCacheTestSystemRaw(t, promptCacheTestSystemBlock(longSystem, "5m")),
			Messages:   []apicompat.AnthropicMessage{promptCacheTestMessage(t, "user", "hello world")},
			ToolChoice: json.RawMessage(toolChoice),
		}
	}

	profile1 := BuildPromptCacheProfile(build(`"auto"`), 120)
	require.NotNil(t, profile1)
	tracker.Update(1, profile1)

	profile2 := BuildPromptCacheProfile(build(`{"type":"tool","name":"Read"}`), 120)
	require.NotNil(t, profile2)
	result := tracker.Compute(1, profile2)
	require.Zero(t, result.CacheReadInputTokens, "switching tool_choice mid-conversation must not be reported as a cache hit")
}

// TestPromptCacheToolSchemaKeyOrderDoesNotBreakMatch 覆盖对照 Kiro-Go 复查
// 发现的第二个缺口：工具 schema 的 JSON key 顺序不同（不同 JSON 库序列化
// map 不保证顺序）不能让本该命中的断点白白错过——schema 语义完全相同，
// 只是 key 顺序不同,规范化（排序 key）之后哈希必须一致。
//
// cache_control 特意放在工具定义上而不是 system 上：这样"工具"本身就是
// 唯一的显式断点来源，让工具断点、以及在它之后隐式产生的消息末尾断点，
// 全都必然把 schema 内容编码进哈希——不给测试留一个"跳过工具、命中更早的
// system-only 断点"的后门，确保测的确实是 schema 规范化这条修复，而不是
// 碰巧从别的断点蒙对的。
func TestPromptCacheToolSchemaKeyOrderDoesNotBreakMatch(t *testing.T) {
	tracker := NewPromptCacheTracker()
	longSystem := strings.Repeat("You are a helpful coding assistant with deep knowledge of Go, Rust, Python, and TypeScript. ", 80)

	build := func(schema string) *apicompat.AnthropicRequest {
		return &apicompat.AnthropicRequest{
			Model:  "claude-sonnet-4-5",
			System: promptCacheTestSystemRaw(t, promptCacheTestSystemBlock(longSystem, "")),
			Tools: []apicompat.AnthropicTool{
				{
					Name:         "Read",
					Description:  "Reads a file",
					InputSchema:  json.RawMessage(schema),
					CacheControl: &apicompat.AnthropicCacheControl{Type: "ephemeral", TTL: "5m"},
				},
			},
			Messages: []apicompat.AnthropicMessage{promptCacheTestMessage(t, "user", "hello world")},
		}
	}

	profile1 := BuildPromptCacheProfile(
		build(`{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}`), 150)
	require.NotNil(t, profile1)
	tracker.Update(1, profile1)

	// 语义相同，key 顺序完全打乱。
	profile2 := BuildPromptCacheProfile(
		build(`{"required":["file_path"],"properties":{"file_path":{"type":"string"}},"type":"object"}`), 150)
	require.NotNil(t, profile2)
	result := tracker.Compute(1, profile2)
	require.Positive(t, result.CacheReadInputTokens, "semantically identical schema with reordered JSON keys must still hit")
}

func TestMinCacheableTokensForModel(t *testing.T) {
	require.Equal(t, promptCacheOpusMinTokens, MinCacheableTokensForModel("claude-opus-4-6"))
	require.Equal(t, promptCacheMinTokens, MinCacheableTokensForModel("claude-sonnet-4-5"))
}

func TestBuildPromptCacheProfileReturnsNilWithoutAnyBreakpoint(t *testing.T) {
	req := &apicompat.AnthropicRequest{
		Model:    "claude-sonnet-4-5",
		Messages: []apicompat.AnthropicMessage{promptCacheTestMessage(t, "user", "hello, no cache_control anywhere")},
	}
	require.Nil(t, BuildPromptCacheProfile(req, 100), "no explicit cache_control anywhere means no breakpoint at all")
}

func TestPromptCacheComputeNilSafe(t *testing.T) {
	var tracker *PromptCacheTracker
	require.Zero(t, tracker.Compute(1, &PromptCacheProfile{}))
	require.NotPanics(t, func() { tracker.Update(1, &PromptCacheProfile{}) })
}

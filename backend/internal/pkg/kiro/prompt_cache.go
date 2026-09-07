package kiro

import (
	"crypto/sha256"
	"encoding/json"
	"hash"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

// 本文件实现的是账号级、进程内的 prompt cache *模拟*——不是在读/转发 Kiro/AWS
// 真实给出的缓存信号。真实调查（2026-09-07，kiro-test 账号 26 轮真实对话 +
// 受控探针）证实 Kiro 的 meteringEvent 在实践中从未给出过非零
// cacheReadInputTokens/cacheCreationInputTokens；交叉核对的两份可读到真实源码
// 的参考实现（Kiro-Go、kiro-go-proxy）分别用"账号级指纹精确匹配"和"完全不做
// 缓存"应对这同一个事实,没有一份代码证明能让 AWS 真正处理得更快。这里移植的
// 是 Kiro-Go（proxy/cache_tracker.go）的保守方案：只有客户端自己发来的
// cache_control 断点、且这次内容与该账号近期确实发过的内容逐段匹配上,才报
// 非零 cache_read——不像 kiro2cc-proxy 那样把"除最后一轮外的全部历史"无条件
// 当作缓存命中（那是启发式假设,不是验证过的复用）。
//
// 影响面：只改变本网关上报/计费用的 usage 数字（cache_creation_input_tokens /
// cache_read_input_tokens，以及按此扣减后的 input_tokens），不改变真正发给
// Kiro 上游的请求内容或大小——不能让 AWS 处理得更快，只能让计费更贴近"重复
// 发送的历史不应按全价计费"这条 Anthropic 式的缓存经济学。

// DefaultPromptCacheTTL 是 Anthropic ephemeral cache 未显式指定 ttl 时的默认时长。
const DefaultPromptCacheTTL = 5 * time.Minute

const (
	// promptCacheMinTokens / promptCacheOpusMinTokens 是 Anthropic 公开文档的
	// 真实缓存最小 token 门槛——低于这个数的前缀本来就不会被记成缓存,报出来
	// 只会是不真实的"100%命中"假象。
	promptCacheMinTokens     = 1024
	promptCacheOpusMinTokens = 4096

	// promptCacheMaxCacheableRatio 保证"这次新增的内容"永远不会被算进缓存——
	// 当前这一轮至少有 15% 视为不可能来自缓存,防止账号第二次请求就被判定
	// 100% 命中这种不现实的结果。
	promptCacheMaxCacheableRatio = 0.85
)

// PromptCacheUsage 是这次请求算出的模拟 prompt cache 用量。
type PromptCacheUsage struct {
	CacheCreationInputTokens   int
	CacheReadInputTokens       int
	CacheCreation5mInputTokens int
	CacheCreation1hInputTokens int
}

// promptCacheBreakpoint 是前缀哈希链上的一个可缓存断点。
type promptCacheBreakpoint struct {
	fingerprint      [32]byte
	cumulativeTokens int
	ttl              time.Duration
}

// PromptCacheProfile 是从一次请求里提取出的、可用于匹配/记录的缓存断点序列。
type PromptCacheProfile struct {
	breakpoints      []promptCacheBreakpoint
	totalInputTokens int
	model            string
}

// MinCacheableTokensForModel 返回该模型的最小可缓存 token 门槛
// （Anthropic 公开文档：Opus 系列 4096，其余 1024）。
func MinCacheableTokensForModel(model string) int {
	if strings.Contains(strings.ToLower(model), "opus") {
		return promptCacheOpusMinTokens
	}
	return promptCacheMinTokens
}

// BuildPromptCacheProfile 从一次 Anthropic 请求里提取缓存断点。
//
// 断点判定规则（移植自 Kiro-Go 的 BuildClaudeProfile）：
//  1. 段本身带显式 cache_control（ephemeral）——该段结束处是一个断点。
//  2. 一旦出现过任意显式断点，此后每条消息的末尾都成为隐式断点——多轮对话
//     里只有最新一轮通常带 cache_control，但历史轮次的末尾也应该能命中更早
//     存下的前缀，不能要求客户端每轮都重复声明。
//
// totalInputTokens 应传调用方已经用 EstimateRequestInput 算出的总估算值,
// 用于 Compute 阶段裁剪"最新一轮"的边界——两者口径不同时,取更大的那个,
// 保证 cumulativeTokens 不会因为口径差异而超过总量。没有任何断点时返回 nil。
func BuildPromptCacheProfile(req *apicompat.AnthropicRequest, totalInputTokens int) *PromptCacheProfile {
	if req == nil {
		return nil
	}

	segments := flattenPromptCacheSegments(req)
	if len(segments) == 0 {
		return nil
	}

	hasher := sha256.New()
	var breakpoints []promptCacheBreakpoint
	cumulative := 0
	var activeTTL time.Duration

	for _, seg := range segments {
		for _, field := range seg.fields {
			writeHashChunk(hasher, field)
		}
		cumulative += seg.tokens

		bpTTL := time.Duration(0)
		switch {
		case seg.ttl > 0:
			bpTTL = seg.ttl
			activeTTL = seg.ttl
		case seg.isMessageEnd && activeTTL > 0:
			bpTTL = activeTTL
		}
		if bpTTL <= 0 {
			continue
		}

		var fp [32]byte
		copy(fp[:], hasher.Sum(nil))
		breakpoints = append(breakpoints, promptCacheBreakpoint{
			fingerprint:      fp,
			cumulativeTokens: cumulative,
			ttl:              bpTTL,
		})
	}

	if len(breakpoints) == 0 {
		return nil
	}
	if totalInputTokens < cumulative {
		totalInputTokens = cumulative
	}
	return &PromptCacheProfile{breakpoints: breakpoints, totalInputTokens: totalInputTokens, model: req.Model}
}

// promptCacheSegment 是构成哈希链的最小单元。fields 里的每一项各自独立做
// 长度前缀哈希（writeHashChunk），因此调用方不需要关心内部分隔符转义。
//
// 有意不携带任何位置序号（消息下标/块下标）——累积哈希链本身已经通过"喂入
// 顺序"编码了位置信息，重复编码只会让同样内容因为位置不同而被判定为不同
// 前缀，反而伤害命中率。
type promptCacheSegment struct {
	fields       []string
	tokens       int
	ttl          time.Duration
	isMessageEnd bool
}

func flattenPromptCacheSegments(req *apicompat.AnthropicRequest) []promptCacheSegment {
	segments := []promptCacheSegment{promptCachePreludeSegment(req)}

	appendSystemCacheSegments(&segments, req.System)

	for _, tool := range req.Tools {
		schema := canonicalCacheJSON(tool.InputSchema)
		segments = append(segments, promptCacheSegment{
			fields: []string{"tool", tool.Name, tool.Description, schema},
			tokens: EstimateText(tool.Name) + EstimateText(tool.Description) + EstimateText(schema),
			ttl:    promptCacheTTLFromControl(tool.CacheControl),
		})
	}

	for _, msg := range req.Messages {
		appendMessageCacheSegments(&segments, msg)
	}

	return segments
}

// promptCachePreludeSegment 是哈希链的第一段：model + tool_choice——移植自
// Kiro-Go 的 buildCachePreludeBlock（复查对照真实源码发现的缺口）。这两个
// 字段变了就不是能安全复用的同一个前缀：模型切换、tool_choice 从 auto
// 切到强制某个工具，都可能让上游对同样的历史内容产生不同处理，必须让
// 哈希链从第一段就分叉，不能因为后面的内容凑巧一样就被判定命中——否则会
// 在客户端中途换模型/换 tool_choice 时，把不该给的缓存折扣算给账号,是计费
// 公平性问题,不是假设性场景。
func promptCachePreludeSegment(req *apicompat.AnthropicRequest) promptCacheSegment {
	toolChoice := canonicalCacheJSON(req.ToolChoice)
	return promptCacheSegment{
		fields: []string{"prelude", req.Model, toolChoice},
		tokens: EstimateText(req.Model) + EstimateText(toolChoice),
	}
}

// canonicalCacheJSON 把一段 JSON 规范化成确定性字符串（对象 key 排序）——
// 移植自 Kiro-Go 的 canonicalizeCacheValue。同一份 schema/tool_choice 只要
// 语义相同，不管序列化时 key 顺序如何都产出相同字符串,避免不同 JSON 库
// 对同一份内容的 key 顺序不一致时,误判"内容变了"而白白错过本该命中的
// 断点（复查对照真实源码发现的缺口:此前直接用客户端原始字节,对 key 顺序
// 敏感）。解析失败（不是合法 JSON，或为空）时退回原始字节的字符串形式——
// 容错优先于精确,不能因为规范化失败就让整个请求的缓存断点计算跟着报错。
func canonicalCacheJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	var buf strings.Builder
	writeCanonicalCacheJSON(&buf, v)
	return buf.String()
}

// strings.Builder 的 Write*/WriteByte 按其接口约定永不返回错误
// （io.Discard 语义），返回值特意丢弃。
func writeCanonicalCacheJSON(buf *strings.Builder, v any) {
	switch val := v.(type) {
	case nil:
		_, _ = buf.WriteString("null")
	case bool:
		if val {
			_, _ = buf.WriteString("true")
		} else {
			_, _ = buf.WriteString("false")
		}
	case string:
		encoded, _ := json.Marshal(val)
		_, _ = buf.Write(encoded)
	case float64:
		encoded, _ := json.Marshal(val)
		_, _ = buf.Write(encoded)
	case []any:
		_ = buf.WriteByte('[')
		for i, item := range val {
			if i > 0 {
				_ = buf.WriteByte(',')
			}
			writeCanonicalCacheJSON(buf, item)
		}
		_ = buf.WriteByte(']')
	case map[string]any:
		_ = buf.WriteByte('{')
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				_ = buf.WriteByte(',')
			}
			encodedKey, _ := json.Marshal(k)
			_, _ = buf.Write(encodedKey)
			_ = buf.WriteByte(':')
			writeCanonicalCacheJSON(buf, val[k])
		}
		_ = buf.WriteByte('}')
	default:
		encoded, _ := json.Marshal(val)
		_, _ = buf.Write(encoded)
	}
}

func appendSystemCacheSegments(segments *[]promptCacheSegment, raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if asString == "" || isPromptCacheBillingHeaderText(asString) {
			return
		}
		*segments = append(*segments, promptCacheSegment{
			fields: []string{"system", asString},
			tokens: EstimateText(asString),
		})
		return
	}

	var blocks []apicompat.AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return
	}
	for _, b := range blocks {
		if b.Type != "text" || b.Text == "" || isPromptCacheBillingHeaderText(b.Text) {
			continue
		}
		*segments = append(*segments, promptCacheSegment{
			fields: []string{"system", "text", b.Text},
			tokens: EstimateText(b.Text),
			ttl:    promptCacheTTLFromControl(b.CacheControl),
		})
	}
}

func appendMessageCacheSegments(segments *[]promptCacheSegment, msg apicompat.AnthropicMessage) {
	var asString string
	if err := json.Unmarshal(msg.Content, &asString); err == nil {
		if asString == "" {
			return
		}
		*segments = append(*segments, promptCacheSegment{
			fields:       []string{"message", msg.Role, "text", asString},
			tokens:       EstimateText(asString),
			isMessageEnd: true,
		})
		return
	}

	var blocks []apicompat.AnthropicContentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return
	}

	lastIdx := -1
	built := make([]promptCacheSegment, 0, len(blocks))
	for _, b := range blocks {
		seg, ok := promptCacheMessageBlockSegment(msg.Role, b)
		if !ok {
			continue
		}
		built = append(built, seg)
		lastIdx = len(built) - 1
	}
	if lastIdx >= 0 {
		built[lastIdx].isMessageEnd = true
	}
	*segments = append(*segments, built...)
}

func promptCacheMessageBlockSegment(role string, b apicompat.AnthropicContentBlock) (promptCacheSegment, bool) {
	switch b.Type {
	case "text":
		if b.Text == "" || isPromptCacheBillingHeaderText(b.Text) {
			return promptCacheSegment{}, false
		}
		return promptCacheSegment{
			fields: []string{"message", role, "text", b.Text},
			tokens: EstimateText(b.Text),
			ttl:    promptCacheTTLFromControl(b.CacheControl),
		}, true

	case "thinking":
		return promptCacheSegment{
			fields: []string{"message", role, "thinking", b.Thinking, b.Signature},
			tokens: EstimateText(b.Thinking) + EstimateText(b.Signature),
			ttl:    promptCacheTTLFromControl(b.CacheControl),
		}, true

	case "image":
		tag := promptCacheImageFingerprint(b.Source)
		return promptCacheSegment{
			fields: []string{"message", role, "image", tag},
			tokens: imageTokenEstimate,
			ttl:    promptCacheTTLFromControl(b.CacheControl),
		}, true

	case "tool_use":
		input := canonicalCacheJSON(b.Input)
		return promptCacheSegment{
			fields: []string{"message", role, "tool_use", b.Name, input},
			tokens: EstimateText(b.Name) + EstimateText(input),
			ttl:    promptCacheTTLFromControl(b.CacheControl),
		}, true

	case "tool_result":
		text, _ := flattenToolResultContent(b.Content)
		return promptCacheSegment{
			fields: []string{"message", role, "tool_result", b.ToolUseID, text},
			tokens: EstimateText(b.ToolUseID) + EstimateText(text),
			ttl:    promptCacheTTLFromControl(b.CacheControl),
		}, true

	default:
		// document / redacted_thinking / server_tool_use 等未建模类型：不计入
		// 缓存断点（也不计入 cumulativeTokens）。BuildPromptCacheProfile 会用
		// EstimateRequestInput 算出的总量兜底裁剪，漏掉这些类型只会让模拟结果
		// 偏保守（少报缓存、不会多报），不影响正确性。
		return promptCacheSegment{}, false
	}
}

// promptCacheImageFingerprint 返回图片的一个廉价身份标记，不用于计费/token
// 估算（那条规则见 imageTokenEstimate 的文档：绝不能拿字节数估算 token）——
// 这里只是给同一张图片重复出现时一个可比较的信号，取媒体类型+字节数+一段
// 定长前缀，不处理整段 base64。
func promptCacheImageFingerprint(src *apicompat.AnthropicImageSource) string {
	if src == nil {
		return ""
	}
	prefixLen := min(32, len(src.Data))
	return src.MediaType + ":" + strconv.Itoa(len(src.Data)) + ":" + src.Data[:prefixLen]
}

// isPromptCacheBillingHeaderText 识别 Claude Code 每次请求都会变化的计费头
// 文本块（形如 "x-anthropic-billing-header: cc_version=...; cch=...;"）。
// 移植自 Kiro-Go 的 isAnthropicBillingHeaderBlock：这段内容与模型语义无关,
// 但字节每次不同,不排除的话会让前缀哈希永远对不上,前面所有的断点匹配都
// 白做。
func isPromptCacheBillingHeaderText(text string) bool {
	trimmed := strings.TrimLeft(text, " \t\r\n")
	return strings.HasPrefix(strings.ToLower(trimmed), "x-anthropic-billing-header:")
}

func promptCacheTTLFromControl(cc *apicompat.AnthropicCacheControl) time.Duration {
	if cc == nil || !strings.EqualFold(cc.Type, "ephemeral") {
		return 0
	}
	if strings.EqualFold(strings.TrimSpace(cc.TTL), "1h") {
		return time.Hour
	}
	return DefaultPromptCacheTTL
}

// writeHashChunk 写入一个长度前缀的块，避免相邻字段拼接产生的边界歧义
// （"ab"+"c" 与 "a"+"bc" 逐字节拼接会相同，长度前缀能区分）。hash.Hash.Write
// 按 hash.Hash 的接口约定永不返回错误，返回值特意丢弃。
func writeHashChunk(h hash.Hash, chunk string) {
	length := strconv.Itoa(len(chunk))
	_, _ = h.Write([]byte(length))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(chunk))
	_, _ = h.Write([]byte{0})
}

// promptCacheEntry 是账号维度存下的一个断点及其过期时间。
type promptCacheEntry struct {
	expiresAt time.Time
	ttl       time.Duration
}

// PromptCacheTracker 按账号维度记录最近见过的缓存断点指纹，用来判断这次
// 请求的前缀是不是"最近真的发过一次"——不是转发 Kiro/AWS 的真实缓存信号
// （见本文件顶部说明）。零值不可用，必须用 NewPromptCacheTracker 构造；
// 所有方法对 nil receiver 安全（Compute 返回零值、Update 空操作），这样
// 未走 NewKiroGatewayService 构造的测试/调用方不会因为忘记初始化而 panic，
// 只是拿不到模拟缓存——与账号级 KiroFakeThinking 开关关闭时的降级路径一致。
type PromptCacheTracker struct {
	mu        sync.Mutex
	byAccount map[int64]map[[32]byte]promptCacheEntry
}

// NewPromptCacheTracker 构造一个空的追踪器。
func NewPromptCacheTracker() *PromptCacheTracker {
	return &PromptCacheTracker{byAccount: make(map[int64]map[[32]byte]promptCacheEntry)}
}

// Compute 返回这次请求应报出的模拟 cache 用量,不修改追踪器状态——写入是
// Update 的职责,两者分开是为了让调用方能在"确认这次请求真的被上游接受"之后
// 再决定要不要记录（forwardUpstream 只在 Kiro 返回 2xx 之后才调用 Update）。
func (t *PromptCacheTracker) Compute(accountID int64, profile *PromptCacheProfile) PromptCacheUsage {
	if t == nil || profile == nil || len(profile.breakpoints) == 0 || accountID == 0 {
		return PromptCacheUsage{}
	}

	minTokens := MinCacheableTokensForModel(profile.model)
	last := profile.breakpoints[len(profile.breakpoints)-1]
	lastTokens := min(last.cumulativeTokens, profile.totalInputTokens)

	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneExpiredLocked(now)

	entries := t.byAccount[accountID]
	if len(entries) == 0 {
		// 该账号第一次见到任何断点：只可能是"这次写入了缓存"，不可能是命中。
		creation := lastTokens
		if creation < minTokens {
			creation = 0
		}
		c5, c1 := promptCacheTTLBreakdown(profile, 0, creation)
		return PromptCacheUsage{CacheCreationInputTokens: creation, CacheCreation5mInputTokens: c5, CacheCreation1hInputTokens: c1}
	}

	maxCacheable := int(float64(profile.totalInputTokens) * promptCacheMaxCacheableRatio)
	if lastTokens > maxCacheable {
		lastTokens = maxCacheable
	}

	matched := 0
	for _, bp := range slices.Backward(profile.breakpoints) {
		if bp.cumulativeTokens < minTokens {
			continue
		}
		entry, ok := entries[bp.fingerprint]
		if !ok || entry.expiresAt.Before(now) {
			continue
		}
		entry.expiresAt = now.Add(entry.ttl)
		entries[bp.fingerprint] = entry
		matched = min(bp.cumulativeTokens, profile.totalInputTokens)
		matched = min(matched, lastTokens)
		break
	}

	creation := max(lastTokens-matched, 0)
	c5, c1 := promptCacheTTLBreakdown(profile, matched, creation)
	return PromptCacheUsage{
		CacheCreationInputTokens:   creation,
		CacheReadInputTokens:       matched,
		CacheCreation5mInputTokens: c5,
		CacheCreation1hInputTokens: c1,
	}
}

// Update 把这次请求里达到最小门槛的断点存入/刷新该账号的记录，供后续请求
// 匹配。与 Compute 是否走了命中分支无关——即便这次没匹配上（比如冷启动），
// 这次的断点仍然要存下来，否则下一轮永远都是"第一次"。
func (t *PromptCacheTracker) Update(accountID int64, profile *PromptCacheProfile) {
	if t == nil || profile == nil || len(profile.breakpoints) == 0 || accountID == 0 {
		return
	}

	minTokens := MinCacheableTokensForModel(profile.model)
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneExpiredLocked(now)

	entries := t.byAccount[accountID]
	if entries == nil {
		entries = make(map[[32]byte]promptCacheEntry)
		t.byAccount[accountID] = entries
	}
	for _, bp := range profile.breakpoints {
		if bp.cumulativeTokens < minTokens {
			continue
		}
		entries[bp.fingerprint] = promptCacheEntry{expiresAt: now.Add(bp.ttl), ttl: bp.ttl}
	}
}

func (t *PromptCacheTracker) pruneExpiredLocked(now time.Time) {
	for accountID, entries := range t.byAccount {
		for fp, e := range entries {
			if !e.expiresAt.After(now) {
				delete(entries, fp)
			}
		}
		if len(entries) == 0 {
			delete(t.byAccount, accountID)
		}
	}
}

// promptCacheTTLBreakdown 把 [matched, matched+creationCap) 这一段按各断点
// 自己的 ttl 拆成 5 分钟档和 1 小时档——Anthropic 的 cache_creation 分
// ephemeral_5m/1h 两档计费，命中(matched)部分不算创建，不参与拆分。
//
// creationCap 必须是调用方已经算出的、真正要报的 cache_creation_input_tokens
// 数值（Compute 里两处调用分别传各自分支算出的 creation），不能让这里自己
// 走到 profile.totalInputTokens——那是没有被 85% 上限（maxCacheable）和
// minTokens 门槛裁剪过的原始总量，两边各自独立计算会导致拆分出的
// fiveMin+oneHour 之和超过（甚至在 creation 被 minTokens 门槛清零时，
// 远大于）真正报出去的 creation 值，破坏 Anthropic 协议要求的
// "cache_creation_5m + cache_creation_1h == cache_creation_input_tokens"
// 这条守恒不变式——这是复查真实数据时发现的真实 bug（cache_read 命中、
// cache_creation 为 0 的行，cache_creation_5m_tokens 却顶着 input_tokens
// 同样大小的非零值），不是假设性场景。
func promptCacheTTLBreakdown(profile *PromptCacheProfile, matched, creationCap int) (fiveMin, oneHour int) {
	if profile == nil || creationCap <= 0 {
		return 0, 0
	}
	upper := matched + creationCap
	prev := matched
	for _, bp := range profile.breakpoints {
		cur := min(bp.cumulativeTokens, upper)
		if cur <= prev {
			continue
		}
		delta := cur - prev
		if bp.ttl >= time.Hour {
			oneHour += delta
		} else {
			fiveMin += delta
		}
		prev = cur
	}
	return fiveMin, oneHour
}

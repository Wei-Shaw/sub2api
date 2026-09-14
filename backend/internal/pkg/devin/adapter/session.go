// session.go 为一次请求派生上游 trajectory/cascade ID 并维护
// 会话内单调递增的 step_index。
//
// ID 派生策略移植 devin2api deriveSessionIDs（sha256 种子 → 两个 UUID，
// 确定性且跨重启稳定，利于上游 prompt 前缀缓存命中）；step_index 计数
// 采用 devin-connect 插件的抓包语义：首请求缺席（0），随后跳 2,3,4…
// （真实 CLI 的 title-gen 调用吃掉序号 1，我们从不发 title-gen，为对齐
// 观测序列直接跳过）。
package adapter

import (
	"crypto/sha256"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
)

// deriveSessionIDs 为一次请求派生上游 trajectory/cascade ID。
// SessionKey（CC metadata.user_id 内含 session_id、Codex prompt_cache_key
// 为线程级）本身即会话级标识，直接做种——压缩改写消息内容也不影响轨迹
// 连续性。无 SessionKey 时退回「系统提示头 4KB + 首条消息文本头 1KB」
// 内容哈希：同一会话多轮回放前缀不变 → 稳定，不同会话 → 自然分散。
func deriveSessionIDs(request llm.RequestMessages) (trajectoryID string, cascadeID string) {
	var seed strings.Builder
	if request.SessionKey != "" {
		seed.WriteString(request.SessionKey)
	} else {
		head := request.SystemPrompt
		if len(head) > 4096 {
			head = head[:4096]
		}
		seed.WriteString(head)
		for _, message := range request.Messages {
			text := firstMessageText(message)
			if text == "" {
				continue
			}
			if len(text) > 1024 {
				text = text[:1024]
			}
			seed.WriteByte(0)
			seed.WriteString(text)
			break
		}
	}
	sum := sha256.Sum256([]byte(seed.String()))
	var a, b [16]byte
	copy(a[:], sum[:16])
	copy(b[:], sum[16:32])
	return devin.FormatUUID(a), devin.FormatUUID(b)
}

// firstMessageText 提取消息的首个文本块，用于会话种子。
func firstMessageText(message llm.Message) string {
	var content []llm.Content
	switch typed := message.(type) {
	case llm.UserMessage:
		content = typed.Content
	case llm.AssistantMessage:
		content = typed.Content
	case llm.ToolResultMessage:
		content = typed.Content
	}
	for _, block := range content {
		if text, ok := block.(llm.TextContent); ok && text.Text != "" {
			return text.Text
		}
	}
	return ""
}

// stepIndexRegistry 按 trajectory_id 记录已发送的上游步数：真实 CLI 每请求
// 发送会话内单调递增的 step_index（抓包实测），不发是残留的 wire 形态
// 差异。计数随进程重启归零，与 CLI 重启行为一致；容量封顶防止会话数
// 长期累积成无界 map，触顶整体清空——轨迹记账字段重置无害。
var stepIndexRegistry = struct {
	sync.Mutex
	counts map[string]int
}{counts: make(map[string]int)}

// nextStepIndex 返回本 trajectory 下一个 step_index。
// 抓包序：用户轮 1 → 缺席（0），title-gen 调用 → 1，用户轮 2 → 2，
// 用户轮 3 → 3…。计数覆盖该 trajectory 上的每一次 GetChatMessage，
// 含一次性 title 调用。我们模拟之：首次请求后计数直接跳到 2。
func nextStepIndex(trajectoryID string) int {
	stepIndexRegistry.Lock()
	defer stepIndexRegistry.Unlock()
	if len(stepIndexRegistry.counts) >= 65536 {
		stepIndexRegistry.counts = make(map[string]int)
	}
	next := stepIndexRegistry.counts[trajectoryID]
	if next == 0 {
		stepIndexRegistry.counts[trajectoryID] = 2
	} else {
		stepIndexRegistry.counts[trajectoryID] = next + 1
	}
	return next
}

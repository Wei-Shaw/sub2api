package service

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// buildReplayTurnPayload 构造第 turn 轮的全量重发 payload：客户端历史逐轮追加
// itemBytes 大小的项（Codex store=false 模式的真实形态）。
func buildReplayTurnPayload(turn, itemBytes int) []byte {
	var b strings.Builder
	_, _ = b.WriteString(`{"type":"response.create","model":"gpt-5.5","stream":true,"input":[`)
	filler := strings.Repeat("x", itemBytes)
	for i := 1; i <= turn; i++ {
		if i > 1 {
			_, _ = b.WriteString(",")
		}
		_, _ = fmt.Fprintf(&b, `{"type":"input_text","text":"turn-%d-%s"}`, i, filler)
	}
	_, _ = b.WriteString(`]}`)
	return []byte(b.String())
}

// TestOpenAIWSReplayStateBuildAllocationBounded 长会话分配回归：128 turn、每轮
// ~10KiB 增量，全量历史逐轮重发。replay 状态构建（build + 历史保存）必须共享
// 正文而非逐轮深拷贝，否则累计分配为 O(T²)（本场景 >160MB）；共享实现的累计
// 分配与客户端重发总量同阶（~85MB payload 本身之外仅头数组与解析开销）。
func TestOpenAIWSReplayStateBuildAllocationBounded(t *testing.T) {
	const (
		turns     = 128
		itemBytes = 10 * 1024
	)

	payloads := make([][]byte, 0, turns)
	for turn := 1; turn <= turns; turn++ {
		payloads = append(payloads, buildReplayTurnPayload(turn, itemBytes))
	}

	var history []json.RawMessage
	historyExists := false
	delta := []json.RawMessage{json.RawMessage(`{"type":"function_call","id":"item_1","call_id":"call_1","name":"exec","arguments":"{}"}`)}

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	for turn := 1; turn <= turns; turn++ {
		items, exists, err := buildOpenAIWSReplayInputSequence(history, historyExists, payloads[turn-1], turn > 1)
		require.NoError(t, err)
		require.True(t, exists)
		require.Len(t, items, turn)
		// 保存历史 + collector 增量合并（与 ingress/bridge 保存点同构）。
		history = combineOpenAIWSReplayItems(items, delta)
		historyExists = true
		// 下一轮 payload 含全部用户项但不含 collector 项，触发 sanitize+merge 路径中
		// 最常见的 prefix 分支比较。
		history = history[:len(history)-1]
	}

	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc

	// 正文共享实现全程约 2.5MB（头数组与 gjson 解析开销）。任何逐轮 O(H) 的
	// 字节级复制（extract 拷贝、正文 clone、保存点深拷贝）都会叠加 ≥84MB
	// （sum(t×10KiB)）的量级，直接击穿此上界。
	const maxAllocatedBytes = 32 * 1024 * 1024
	require.Lessf(
		t,
		allocated,
		uint64(maxAllocatedBytes),
		"replay 状态构建累计分配 %d bytes 超出上界，疑似回归为逐轮深拷贝",
		allocated,
	)
}

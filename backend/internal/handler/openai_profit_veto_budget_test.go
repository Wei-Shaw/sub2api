package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRecordOpenAIProfitVetoBounded 钉死 OpenAI 侧选号循环的利润否决预算：
// 账号被加入排除集，且第 maxProfitVetoAttempts 次否决返回 false 要求终止。
//
// 上限是必需的：acquireResponsesAccountSlot 的 WaitPlan 分支先阻塞排队
// （sticky 45s / fallback 30s）拿到槽位才做利润终检，无上限重选会把单次请求
// 的延迟放大到 N × WaitPlan.Timeout。
func TestRecordOpenAIProfitVetoBounded(t *testing.T) {
	failed := make(map[int64]struct{})
	count := 0

	for i := int64(1); i < int64(maxProfitVetoAttempts); i++ {
		require.True(t, recordOpenAIProfitVeto(failed, i, &count), "第 %d 次否决应允许继续重选", i)
		require.Contains(t, failed, i, "否决的账号必须进入本请求排除集")
	}

	require.False(t,
		recordOpenAIProfitVeto(failed, int64(maxProfitVetoAttempts), &count),
		"第 %d 次否决应耗尽预算并要求终止", maxProfitVetoAttempts)
	require.Equal(t, maxProfitVetoAttempts, count)
	require.Len(t, failed, maxProfitVetoAttempts)
}

// TestOpenAIProfitVetoLoopTerminates 模拟「选号 → 抢槽 → 利润终检否决 → 重选」
// 循环：整池越线时必须在常数步内终止，而不是把候选池逐个排队一遍。
func TestOpenAIProfitVetoLoopTerminates(t *testing.T) {
	failed := make(map[int64]struct{})
	count := 0
	terminated := false
	iterations := 0

	// 候选池远大于否决上限：无上限时会对每个账号各排队一次。
	for accountID := int64(1); accountID <= 500; accountID++ {
		iterations++
		if _, excluded := failed[accountID]; excluded {
			continue
		}
		if !recordOpenAIProfitVeto(failed, accountID, &count) {
			terminated = true
			break
		}
	}

	require.True(t, terminated, "整池越线时循环必须由否决预算终止")
	require.Equal(t, maxProfitVetoAttempts, iterations)
}

// TestProfitVetoBudgetSharedWithFailoverState 钉死两条路径（FailoverState 与
// OpenAI 独立计数器）使用同一上限语义，避免日后单边漂移。
func TestProfitVetoBudgetSharedWithFailoverState(t *testing.T) {
	fs := NewFailoverState(10, false)
	failed := make(map[int64]struct{})
	count := 0

	var fsStoppedAt, openAIStoppedAt int
	for i := int64(1); i <= int64(maxProfitVetoAttempts)+5; i++ {
		if fsStoppedAt == 0 && fs.RecordProfitVeto(i) == FailoverExhausted {
			fsStoppedAt = fs.ProfitVetoCount()
		}
		if openAIStoppedAt == 0 && !recordOpenAIProfitVeto(failed, i, &count) {
			openAIStoppedAt = count
		}
	}

	require.Equal(t, maxProfitVetoAttempts, fsStoppedAt)
	require.Equal(t, fsStoppedAt, openAIStoppedAt, "两条路径的利润否决上限必须一致")
}

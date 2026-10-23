package service

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// openAICodexCreditsSnapshotStaleAfter 是本地积分快照参与 429 判定时允许的最大年龄。
// 余额只在窗口耗尽期被消耗、变化缓慢；但过旧的快照不足以证明"现在仍有积分"，
// 过期后回退到原有行为（按窗口耗尽冻结），避免持续撞上游。
const openAICodexCreditsSnapshotStaleAfter = 8 * time.Hour

// openAICodexCreditsAvailable 报告 429 判定时账号是否仍有可消耗的积分额度
// （Codex credits，与重置卡 reset credit 不同）。
//
// 上游在套餐窗口耗尽后允许继续消耗积分提供服务：此时 429 只代表本次请求被
// 瞬时限流，并不代表账号在窗口级别不可用。若不区分，网关会把这类 429 按
// "窗口耗尽"冻结到窗口重置时刻（数天），账号在积分可用期内完全不可调度，
// 与上游实际能力矛盾。
//
// 判据优先级：
//  1. 429 响应头的 x-codex-credits-has-credits/-unlimited/-balance
//     （上游是否在 429 上携带这些头未证实，故仅作为快速路径）；
//  2. 账号本地 codex_credits_snapshot（/wham/usage 快照），要求新鲜。
func openAICodexCreditsAvailable(account *Account, headers http.Header, now time.Time) bool {
	if account == nil {
		return false
	}
	if hasCredits := strings.TrimSpace(headers.Get("x-codex-credits-has-credits")); hasCredits != "" {
		if !strings.EqualFold(hasCredits, "true") {
			return false
		}
		if strings.EqualFold(strings.TrimSpace(headers.Get("x-codex-credits-unlimited")), "true") {
			return true
		}
		balance := strings.TrimSpace(headers.Get("x-codex-credits-balance"))
		if balance == "" {
			// has_credits=true 且未携带余额（如 unlimited 场景）时按可用处理。
			return true
		}
		return openAICreditsBalancePositive(&balance)
	}

	snapshot := openAICodexCreditsSnapshotFromExtra(account.Extra)
	if snapshot == nil || snapshot.Credits == nil || !snapshot.Credits.HasCredits {
		return false
	}
	if snapshot.Credits.Unlimited {
		return true
	}
	if !openAICodexCreditsSnapshotFresh(snapshot, now) {
		return false
	}
	return openAICreditsBalancePositive(snapshot.Credits.Balance)
}

func openAICodexCreditsSnapshotFromExtra(extra map[string]any) *openAICreditsSnapshot {
	if len(extra) == 0 {
		return nil
	}
	raw, ok := extra[openaiQuotaCreditsKey]
	if !ok || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var snapshot openAICreditsSnapshot
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return nil
	}
	return &snapshot
}

func openAICodexCreditsSnapshotFresh(snapshot *openAICreditsSnapshot, now time.Time) bool {
	if snapshot == nil || snapshot.FetchedAt <= 0 {
		return false
	}
	age := now.Sub(time.Unix(snapshot.FetchedAt, 0))
	return age < openAICodexCreditsSnapshotStaleAfter
}

// openAICreditsBalancePositive 解析上游的十进制字符串余额（可能为空或 "0"）。
func openAICreditsBalancePositive(balance *string) bool {
	if balance == nil {
		return false
	}
	trimmed := strings.TrimSpace(*balance)
	if trimmed == "" {
		return false
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	return err == nil && value > 0
}

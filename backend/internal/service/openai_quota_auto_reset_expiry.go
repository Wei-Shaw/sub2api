package service

import (
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	openAIAutoResetTriggerUsage       = "usage_threshold"
	openAIAutoResetTriggerExpiry      = "credit_expiring"
	openAIAutoResetAttemptPending     = "pending"
	openAIAutoResetAttemptConsumed    = "consumed"
	openAIAutoResetAttemptNotConsumed = "not_consumed"
	openAIAutoResetCardKeyVersion     = "card_v1"
)

// OpenAIAutoResetCreditAttempt 只保存卡指纹及结果；已知未消费也必须留档，避免每分钟
// 为同一张卡重新生成兑换请求。LegacyCycleHash 仅用于升级前已发出的兑换重试。
type OpenAIAutoResetCreditAttempt struct {
	CreditHash      string `json:"credit_hash"`
	CreditExpiresAt string `json:"credit_expires_at,omitempty"`
	LegacyCycleHash string `json:"legacy_cycle_hash,omitempty"`
	Outcome         string `json:"outcome"`
	Code            string `json:"code,omitempty"`
	TriggerReason   string `json:"trigger_reason,omitempty"`
	QuotaCycleHash  string `json:"quota_cycle_hash,omitempty"`
	Generation      string `json:"generation,omitempty"`
}

func openAIAutoResetUsageEnabled(account *Account, config OpenAIAutoResetCreditConfig) bool {
	return isOpenAIAutoResetCreditAccount(account) && config.Enabled && account.IsActive() && account.Schedulable
}

func openAIAutoResetEnabledForAccount(account *Account, config OpenAIAutoResetCreditConfig) bool {
	return isOpenAIAutoResetCreditAccount(account) && (config.ExpiryEnabled || openAIAutoResetUsageEnabled(account, config))
}

func openAIAutoResetCreditExpiring(expiresAt string, hours float64, now time.Time) bool {
	expiry, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || !isValidOpenAIAutoResetExpiryHours(hours) {
		return false
	}
	remaining := expiry.Sub(now)
	return remaining > 0 && remaining <= time.Duration(hours*float64(time.Hour))
}

func openAIAutoResetExpiryQueryDue(state *OpenAIAutoResetCreditState, config OpenAIAutoResetCreditConfig, now time.Time) bool {
	if !config.ExpiryEnabled {
		return false
	}
	if state == nil {
		return true
	}
	checkedAt, err := time.Parse(time.RFC3339, state.CreditsCheckedAt)
	if err != nil || now.Sub(checkedAt) >= openAIAutoResetSnapshotTTL {
		return true
	}
	// 额度样本会持续刷新，所以卡的查询时钟独立计时；临期最多每分钟实时确认一次。
	expiry, expiryErr := time.Parse(time.RFC3339, state.NextCreditExpiresAt)
	return expiryErr == nil && expiry.Sub(now) <= time.Duration(config.ExpiryHours*float64(time.Hour)) && now.Sub(checkedAt) >= openAIAutoResetScanInterval
}

func openAIAutoResetMigrateAttempt(state *OpenAIAutoResetCreditState) {
	if state == nil || state.AttemptCreditHash == "" || state.AttemptKeyVersion == openAIAutoResetCardKeyVersion {
		return
	}
	for _, attempt := range state.Attempts {
		if attempt.CreditHash == state.AttemptCreditHash {
			return
		}
	}
	outcome := openAIAutoResetAttemptPending
	if state.Status == OpenAIAutoResetStatusSuccess {
		outcome = openAIAutoResetAttemptConsumed
	}
	state.Attempts = append(state.Attempts, OpenAIAutoResetCreditAttempt{
		CreditHash: state.AttemptCreditHash, CreditExpiresAt: state.CreditExpiresAt,
		LegacyCycleHash: state.AttemptCycleHash, Outcome: outcome,
	})
}

func openAIAutoResetFindAttempt(state *OpenAIAutoResetCreditState, creditHash string) *OpenAIAutoResetCreditAttempt {
	for i := range state.Attempts {
		if state.Attempts[i].CreditHash == creditHash {
			return &state.Attempts[i]
		}
	}
	return nil
}

func openAIAutoResetHasPending(state *OpenAIAutoResetCreditState) bool {
	if state != nil {
		if state.PostProcessPending {
			return true
		}
		if state.ErrorCode == OpenAIQuotaResetWarningCacheRefreshFailed || state.ErrorCode == OpenAIQuotaResetWarningAccountRecoveryFailed || state.ErrorCode == OpenAIQuotaResetWarningAccountRefreshFailed {
			return true
		}
		for _, attempt := range state.Attempts {
			if attempt.Outcome == openAIAutoResetAttemptPending {
				return true
			}
		}
	}
	return false
}

func pruneOpenAIAutoResetAttempts(state *OpenAIAutoResetCreditState, now time.Time) {
	kept := state.Attempts[:0]
	for _, attempt := range state.Attempts {
		expiry, err := time.Parse(time.RFC3339, attempt.CreditExpiresAt)
		// 未决结果和旧版无到期信息的身份不能靠时间清理；已知结果过期后无需永久积累。
		postProcessPending := state.PostProcessPending && attempt.CreditHash == state.AttemptCreditHash
		if attempt.Outcome != openAIAutoResetAttemptPending && !postProcessPending && err == nil && !expiry.After(now) {
			continue
		}
		kept = append(kept, attempt)
	}
	state.Attempts = kept
}

func openAIAutoResetAttemptKey(accountID int64, attempt OpenAIAutoResetCreditAttempt) string {
	if attempt.LegacyCycleHash != "" {
		return fmt.Sprintf("oarc:%d:%s:%s", accountID, attempt.CreditHash, attempt.LegacyCycleHash)
	}
	// 同账号同卡只有一种消费身份，额度自然换周期或触发原因变化都不会另发兑换。
	key := fmt.Sprintf("oarc:v2:%d:%s", accountID, attempt.CreditHash)
	if attempt.Generation != "" {
		key += ":" + attempt.Generation
	}
	return key
}

func openAIAutoResetSetNextExpiry(state *OpenAIAutoResetCreditState, candidates []openAIAutoResetCreditCandidate, now time.Time) {
	state.NextCreditExpiresAt = ""
	var next time.Time
	for _, candidate := range candidates {
		expiry, err := time.Parse(time.RFC3339, candidate.ExpiresAt)
		if err != nil || !expiry.After(now) {
			continue
		}
		attempt := openAIAutoResetFindAttempt(state, shortOpenAIAutoResetHash(candidate.ID))
		if attempt != nil && attempt.Outcome != openAIAutoResetAttemptPending {
			continue
		}
		if next.IsZero() || expiry.Before(next) {
			next = expiry
			state.NextCreditExpiresAt = candidate.ExpiresAt
		}
	}
}

func selectOpenAIAutoResetEligibleCandidate(usage *OpenAIQuotaUsage, state *OpenAIAutoResetCreditState, config OpenAIAutoResetCreditConfig, usageReached bool, now time.Time) (*openAIAutoResetCreditCandidate, string, error) {
	available := usage.RateLimitResetCredits.AvailableCount
	candidates, err := orderOpenAIAutoResetCandidates(usage.autoResetCandidates, available)
	if err != nil {
		return nil, "", err
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.ID) == "" {
			return nil, "", infraerrors.Conflict("OPENAI_AUTO_RESET_CREDIT_ID_MISSING", "reset credit has no official id")
		}
	}
	// 未知结果必须先确认原卡，不能因用量周期变化、上游列表缺卡而改选下一张。
	for _, attempt := range state.Attempts {
		if attempt.Outcome != openAIAutoResetAttemptPending {
			continue
		}
		for _, candidate := range candidates {
			if shortOpenAIAutoResetHash(candidate.ID) == attempt.CreditHash {
				if reason := openAIAutoResetCandidateReason(candidate, config, usageReached, now); reason != "" {
					return &candidate, reason, nil
				}
				return nil, "", nil
			}
		}
		return nil, "", infraerrors.Conflict("OPENAI_AUTO_RESET_ORIGINAL_CREDIT_UNAVAILABLE", "the original reset credit cannot be confirmed; refusing to switch credits")
	}
	if available <= 0 {
		return nil, "", nil
	}
	for _, candidate := range candidates {
		if attempt := openAIAutoResetFindAttempt(state, shortOpenAIAutoResetHash(candidate.ID)); attempt != nil {
			cycleHash := shortOpenAIAutoResetHash(openAIAutoResetCycleSeed(usage))
			// 到期尝试已知未消费后，真实阈值仍可用卡；同一用量周期的新代次只能产生一次。
			if attempt.Outcome != openAIAutoResetAttemptNotConsumed || !strings.EqualFold(strings.TrimSpace(attempt.Code), "nothing_to_reset") || !usageReached ||
				(attempt.TriggerReason != openAIAutoResetTriggerExpiry && attempt.QuotaCycleHash == cycleHash) {
				continue
			}
		}
		if reason := openAIAutoResetCandidateReason(candidate, config, usageReached, now); reason != "" {
			return &candidate, reason, nil
		}
	}
	return nil, "", nil
}

func openAIAutoResetCandidateReason(candidate openAIAutoResetCreditCandidate, config OpenAIAutoResetCreditConfig, usageReached bool, now time.Time) string {
	expiry, err := time.Parse(time.RFC3339, candidate.ExpiresAt)
	if err != nil || !expiry.After(now) {
		return ""
	}
	if usageReached {
		return openAIAutoResetTriggerUsage
	}
	if config.ExpiryEnabled && openAIAutoResetCreditExpiring(candidate.ExpiresAt, config.ExpiryHours, now) {
		return openAIAutoResetTriggerExpiry
	}
	return ""
}

func openAIAutoResetConsumeOutcome(result openAIAutoResetConsumeResult) string {
	code := strings.ToLower(strings.TrimSpace(result.Code))
	switch code {
	case "nothing_to_reset", "no_credit":
		if result.WindowsReset == 0 {
			return openAIAutoResetAttemptNotConsumed
		}
		return openAIAutoResetAttemptPending
	case "not_limited", "not_available":
		return openAIAutoResetAttemptPending
	}
	if result.WindowsReset > 0 {
		return openAIAutoResetAttemptConsumed
	}
	// HTTP 200 和零窗口都不足以证明已消费；保留原卡身份，等待后续同键结果。
	return openAIAutoResetAttemptPending
}

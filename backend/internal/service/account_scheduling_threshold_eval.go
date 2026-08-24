package service

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

// AccountSchedulingThresholdDecision captures the pure pause decision for one account.
type AccountSchedulingThresholdDecision struct {
	ShouldPause      bool
	Platform         string
	Window           string
	Scope            string
	ThresholdPercent int
	UsedPercent      float64
	Until            *time.Time
}

type accountSchedulingThresholdCandidate struct {
	window      string
	scope       string
	usedPercent float64
	until       *time.Time
}

const accountSchedulingThresholdCredentialKey = "account_scheduling_threshold"

// EvaluateAccountSchedulingThreshold evaluates whether an account should be paused
// based on the current per-platform scheduling threshold snapshot.
func EvaluateAccountSchedulingThreshold(account *Account, thresholds map[string]int, now time.Time) AccountSchedulingThresholdDecision {
	decision := AccountSchedulingThresholdDecision{}
	if account == nil {
		return decision
	}

	decision.Platform = strings.ToLower(strings.TrimSpace(account.Platform))
	if decision.Platform == "" {
		return decision
	}
	if !isAllowedSchedulingThresholdPlatform(decision.Platform) {
		return decision
	}

	threshold, ok := resolveEffectiveAccountSchedulingThreshold(account, thresholds, decision.Platform)
	decision.ThresholdPercent = threshold
	if !ok || threshold >= 100 {
		return decision
	}

	var winner *accountSchedulingThresholdCandidate
	switch decision.Platform {
	case PlatformOpenAI:
		winner = pickLatestResetSchedulingCandidate(openAIThresholdCandidates(account, now), threshold, now)
	case PlatformAnthropic:
		winner = pickLatestResetSchedulingCandidate(anthropicThresholdCandidates(account), threshold, now)
	case PlatformGrok:
		winner = pickLatestResetSchedulingCandidate(grokThresholdCandidates(account), threshold, now)
	case PlatformKimi:
		winner = pickLatestResetSchedulingCandidate(cnProviderThresholdCandidates(account, PlatformKimi), threshold, now)
	case PlatformZhipu:
		winner = pickLatestResetSchedulingCandidate(cnProviderThresholdCandidates(account, PlatformZhipu), threshold, now)
	default:
		return decision
	}

	if winner == nil {
		return decision
	}

	decision.ShouldPause = true
	decision.Window = winner.window
	decision.Scope = winner.scope
	decision.UsedPercent = winner.usedPercent
	decision.Until = winner.until
	return decision
}

func isAllowedSchedulingThresholdPlatform(platform string) bool {
	for _, allowed := range AllowedSchedulingThresholdPlatforms {
		if platform == allowed {
			return true
		}
	}
	return false
}

func resolveEffectiveAccountSchedulingThreshold(account *Account, thresholds map[string]int, platform string) (int, bool) {
	if account != nil {
		if threshold, ok := accountSchedulingThresholdOverride(account); ok {
			return threshold, true
		}
	}
	return lookupAccountSchedulingThreshold(thresholds, platform)
}

func accountSchedulingThresholdOverride(account *Account) (int, bool) {
	if account == nil || len(account.Credentials) == 0 {
		return 0, false
	}
	raw, ok := account.Credentials[accountSchedulingThresholdCredentialKey]
	if !ok {
		return 0, false
	}
	return parseAccountSchedulingThresholdValue(raw)
}

func parseAccountSchedulingThresholdValue(raw any) (int, bool) {
	var value int
	switch v := raw.(type) {
	case int:
		value = v
	case int64:
		value = int(v)
	case float64:
		value = int(math.Round(v))
	case float32:
		value = int(math.Round(float64(v)))
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return 0, false
		}
		value = int(math.Round(parsed))
	case string:
		raw := strings.TrimSpace(v)
		parsed, err := strconv.Atoi(raw)
		if err == nil {
			value = parsed
			break
		}
		parsedFloat, floatErr := strconv.ParseFloat(raw, 64)
		if floatErr != nil {
			return 0, false
		}
		value = int(math.Round(parsedFloat))
	default:
		return 0, false
	}
	if value < 1 || value > 100 {
		return 0, false
	}
	return value, true
}

func lookupAccountSchedulingThreshold(thresholds map[string]int, platform string) (int, bool) {
	if len(thresholds) == 0 {
		return 0, false
	}
	value, ok := thresholds[platform]
	return value, ok
}

func openAIThresholdCandidates(account *Account, now time.Time) []*accountSchedulingThresholdCandidate {
	if account == nil {
		return nil
	}
	if !openAICodexSnapshotIdentityTrusted(account) {
		return nil
	}
	return []*accountSchedulingThresholdCandidate{
		openAIThresholdCandidate(account.Extra, "5h", now),
		openAIThresholdCandidate(account.Extra, "7d", now),
	}
}

func openAICodexSnapshotIdentityTrusted(account *Account) bool {
	if account == nil || !account.IsOpenAIOAuth() || len(account.Extra) == 0 {
		return true
	}

	if identityValuesConflict(
		firstStringValue(account.Credentials, "email"),
		firstStringValue(account.Extra, "email", "email_address"),
	) {
		return false
	}
	if identityValuesConflict(
		firstStringValue(account.Credentials, "chatgpt_account_id"),
		firstStringValue(account.Extra, "chatgpt_account_id", "account_id"),
	) {
		return false
	}
	if identityValuesConflict(
		firstStringValue(account.Credentials, "workspace_id", "chatgpt_workspace_id", "organization_id", "org_id"),
		firstStringValue(account.Extra, "workspace_id", "chatgpt_workspace_id", "organization_id", "org_id"),
	) {
		return false
	}
	return true
}

func identityValuesConflict(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	return left != "" && right != "" && !strings.EqualFold(left, right)
}

// firstStringValue returns the first non-empty string among the given map keys.
// Used by OpenAI codex snapshot identity matching for scheduling thresholds.
func firstStringValue(values map[string]any, keys ...string) string {
	if len(values) == 0 {
		return ""
	}
	for _, key := range keys {
		raw, ok := values[key]
		if !ok || raw == nil {
			continue
		}
		switch typed := raw.(type) {
		case string:
			if v := strings.TrimSpace(typed); v != "" {
				return v
			}
		default:
			if v := strings.TrimSpace(stringValue(raw)); v != "" {
				return v
			}
		}
	}
	return ""
}

func openAIThresholdCandidate(extra map[string]any, window string, now time.Time) *accountSchedulingThresholdCandidate {
	if len(extra) == 0 {
		return nil
	}

	var (
		usedPercentKey string
		resetAtKey     string
	)
	switch window {
	case "5h":
		usedPercentKey = "codex_5h_used_percent"
		resetAtKey = "codex_5h_reset_at"
	case "7d":
		usedPercentKey = "codex_7d_used_percent"
		resetAtKey = "codex_7d_reset_at"
	default:
		return nil
	}

	usedPercent, ok := extra[usedPercentKey]
	if !ok {
		return nil
	}
	if openAIQuotaWindowReset(extra, window, now) || openAICodexSnapshotStaleForPause(extra, now) {
		return nil
	}
	return &accountSchedulingThresholdCandidate{
		window:      window,
		usedPercent: schedulingPercentValue(usedPercent),
		until:       parseSchedulingResetAt(extra[resetAtKey]),
	}
}

func anthropicThresholdCandidates(account *Account) []*accountSchedulingThresholdCandidate {
	if account == nil {
		return nil
	}

	var candidates []*accountSchedulingThresholdCandidate
	if usedPercent := utilizationAsPercent(account.Extra["session_window_utilization"]); usedPercent > 0 {
		candidates = append(candidates, &accountSchedulingThresholdCandidate{
			window:      "5h",
			usedPercent: usedPercent,
			until:       cloneTimePtr(account.SessionWindowEnd),
		})
	}
	if usedPercent := utilizationAsPercent(account.Extra["passive_usage_7d_utilization"]); usedPercent > 0 {
		candidates = append(candidates, &accountSchedulingThresholdCandidate{
			window:      "7d",
			usedPercent: usedPercent,
			until:       parseSchedulingResetAt(account.Extra["passive_usage_7d_reset"]),
		})
	}
	return candidates
}

// NOTE: Gemini / Kiro / Antigravity are intentionally NOT threshold-pausing
// platforms (see AllowedSchedulingThresholdPlatforms and the evaluator switch,
// asserted by TestEvaluateAccountSchedulingThreshold_UnsupportedPlatformsDoNotPause).
// Their former per-platform candidate readers were dead code — never reachable
// from EvaluateAccountSchedulingThreshold — and have been removed to avoid the
// false impression that configuring a threshold for them has any effect. The
// kiro_sched_* / antigravity_sched_* extras are still written purely as
// observability snapshots.

// grokThresholdCandidates uses only header-projected
// grok_sched_utilization / grok_sched_reset_at (rolling quota window, reset
// capped at ~25h when written). Official billing 7d/30d windows are not used
// for auto-pause here.
func grokThresholdCandidates(account *Account) []*accountSchedulingThresholdCandidate {
	if account == nil {
		return nil
	}
	return []*accountSchedulingThresholdCandidate{
		{
			window:      "quota",
			scope:       "grok",
			usedPercent: schedulingPercentValue(account.Extra["grok_sched_utilization"]),
			until:       parseSchedulingResetAt(account.Extra["grok_sched_reset_at"]),
		},
	}
}

// cnProviderThresholdCandidates 读取国产供应商 Coding Plan 账号的 5h / weekly 滚动窗口
// 用量快照（由 CNProviderQuotaService 写入 account.Extra，键形如
// <provider>_5h_used_percent / <provider>_weekly_reset_at）。payg 账号无此快照，
// 候选为空 → 不触发阈值停调（余额型走余额检测）。与 openai 的快照驱动停调一致：
// 仅当用量超阈值且窗口尚未重置时才停调。
func cnProviderThresholdCandidates(account *Account, provider string) []*accountSchedulingThresholdCandidate {
	if account == nil || len(account.Extra) == 0 {
		return nil
	}
	return []*accountSchedulingThresholdCandidate{
		cnThresholdCandidate(account.Extra, provider, "5h"),
		cnThresholdCandidate(account.Extra, provider, "weekly"),
	}
}

func cnThresholdCandidate(extra map[string]any, provider, window string) *accountSchedulingThresholdCandidate {
	var usedKey, resetKey string
	switch window {
	case "5h":
		usedKey = cnExtraKey(provider, cnExtraSuffix5hUsed)
		resetKey = cnExtraKey(provider, cnExtraSuffix5hReset)
	case "weekly":
		usedKey = cnExtraKey(provider, cnExtraSuffixWeeklyUsed)
		resetKey = cnExtraKey(provider, cnExtraSuffixWeeklyReset)
	default:
		return nil
	}
	usedPercent, ok := extra[usedKey]
	if !ok {
		return nil
	}
	return &accountSchedulingThresholdCandidate{
		window:      window,
		scope:       provider,
		usedPercent: schedulingPercentValue(usedPercent),
		until:       parseSchedulingResetAt(extra[resetKey]),
	}
}

func pickLatestResetSchedulingCandidate(candidates []*accountSchedulingThresholdCandidate, threshold int, now time.Time) *accountSchedulingThresholdCandidate {
	var winner *accountSchedulingThresholdCandidate
	for _, candidate := range candidates {
		if !candidateMatchesThreshold(candidate, threshold, now) {
			continue
		}
		if winner == nil || candidate.until.After(*winner.until) {
			winner = candidate
			continue
		}
		if winner.until.Equal(*candidate.until) && candidate.usedPercent > winner.usedPercent {
			winner = candidate
		}
	}
	return winner
}

func candidateMatchesThreshold(candidate *accountSchedulingThresholdCandidate, threshold int, now time.Time) bool {
	if candidate == nil || candidate.until == nil || !candidate.until.After(now) {
		return false
	}
	return candidate.usedPercent >= float64(threshold)
}

func utilizationAsPercent(raw any) float64 {
	switch v := raw.(type) {
	case float64:
		if v >= 0 && v <= 1 {
			return v * 100
		}
		return v
	case float32:
		value := float64(v)
		if value >= 0 && value <= 1 {
			return value * 100
		}
		return value
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		value, err := v.Float64()
		if err != nil {
			return 0
		}
		if strings.Contains(v.String(), ".") && value >= 0 && value <= 1 {
			return value * 100
		}
		return value
	case string:
		trimmed := strings.TrimSpace(v)
		value, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0
		}
		if strings.Contains(trimmed, ".") && value >= 0 && value <= 1 {
			return value * 100
		}
		return value
	default:
		return 0
	}
}

func schedulingPercentValue(raw any) float64 {
	switch v := raw.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		value, err := v.Float64()
		if err != nil {
			return 0
		}
		return value
	case string:
		value, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0
		}
		return value
	default:
		return 0
	}
}

func parseSchedulingResetAt(raw any) *time.Time {
	switch v := raw.(type) {
	case nil:
		return nil
	case time.Time:
		ts := v
		return &ts
	case *time.Time:
		return cloneTimePtr(v)
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil
		}
		ts, err := parseSchedulingTime(trimmed)
		if err != nil {
			return nil
		}
		return &ts
	case json.Number:
		if value, err := v.Int64(); err == nil && value > 0 {
			ts := time.Unix(value, 0)
			return &ts
		}
		if value, err := v.Float64(); err == nil && value > 0 {
			ts := time.Unix(int64(value), 0)
			return &ts
		}
	case float64:
		if v > 0 {
			ts := time.Unix(int64(v), 0)
			return &ts
		}
	case float32:
		if v > 0 {
			ts := time.Unix(int64(v), 0)
			return &ts
		}
	case int:
		if v > 0 {
			ts := time.Unix(int64(v), 0)
			return &ts
		}
	case int64:
		if v > 0 {
			ts := time.Unix(v, 0)
			return &ts
		}
	}
	return nil
}

func parseSchedulingTime(raw string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
	}
	for _, format := range formats {
		if ts, err := time.Parse(format, raw); err == nil {
			return ts, nil
		}
	}
	return time.Time{}, strconv.ErrSyntax
}

func cloneTimePtr(src *time.Time) *time.Time {
	if src == nil {
		return nil
	}
	value := *src
	return &value
}

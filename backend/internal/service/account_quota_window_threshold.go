package service

import (
	"math"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Reuse the existing OpenAI extra keys, including their 0..1 storage unit.
// weekly is the CN snapshot name; the account-facing setting remains 7d.
func schedulingThresholdWindowKey(window string) string {
	switch window {
	case "5h":
		return "5h"
	case "7d", "weekly", "7d_oi":
		return "7d"
	case "monthly":
		return "monthly"
	default:
		return ""
	}
}

func effectiveSchedulingWindowThreshold(account *Account, window string, fallback int, hasFallback bool) (float64, bool) {
	key := schedulingThresholdWindowKey(window)
	if key != "" && account != nil {
		if resolveAccountExtraBool(account.Extra, "auto_pause_"+key+"_disabled") {
			return 0, false
		}
		if ratio, ok := resolveAccountExtraNumber(account.Extra, "auto_pause_"+key+"_threshold"); ok &&
			!math.IsNaN(ratio) && !math.IsInf(ratio, 0) && ratio > 0 && ratio <= 1 {
			return ratio * 100, true
		}
	}
	// Preserve the older all-window setting: 100 disables that fallback.
	return float64(fallback), hasFallback && fallback > 0 && fallback < 100
}

func validateAccountQuotaWindowThresholds(extra map[string]any) error {
	for _, window := range []string{"5h", "7d", "monthly"} {
		key := "auto_pause_" + window + "_threshold"
		if raw, present := extra[key]; present && raw != nil {
			ratio, ok := resolveAccountExtraNumber(extra, key)
			if !ok || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 || ratio > 1 {
				return infraerrors.BadRequest("INVALID_QUOTA_WINDOW_THRESHOLD", "quota window thresholds must be finite ratios between 0 and 1 (0 inherits defaults)")
			}
		}
		if raw, present := extra["auto_pause_"+window+"_disabled"]; present && raw != nil {
			if _, ok := raw.(bool); !ok {
				return infraerrors.BadRequest("INVALID_QUOTA_WINDOW_DISABLED", "quota window disable flags must be booleans")
			}
		}
	}
	return nil
}

// Editing a percentage must not overwrite a newer provider snapshot with the
// stale extra object from when the account dialog was opened.
func preserveCNQuotaRuntimeExtra(current, next map[string]any) map[string]any {
	if next == nil {
		next = make(map[string]any)
	}
	for _, provider := range []string{PlatformKimi, PlatformZhipu, PlatformMiniMax, PlatformOpenCodeGo} {
		for _, suffix := range []string{cnExtraSuffix5hUsed, cnExtraSuffix5hReset, cnExtraSuffixWeeklyUsed, cnExtraSuffixWeeklyReset, cnExtraSuffixMonthlyUsed, cnExtraSuffixMonthlyReset, cnExtraSuffixUsageUpdated} {
			key := cnExtraKey(provider, suffix)
			delete(next, key)
			if value, ok := current[key]; ok {
				next[key] = value
			}
		}
	}
	return next
}

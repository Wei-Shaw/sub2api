package service

import (
	"strconv"
	"strings"
	"time"
)

// AccountQuotaResetSnapshot is the provider-neutral reset signal used by
// schedulers and admin diagnostics. A nil ResetAt means that no trustworthy
// reset sample was available; callers must treat it as unknown, never as soon.
type AccountQuotaResetSnapshot struct {
	Window           string     `json:"window"`
	ResetAt          *time.Time `json:"reset_at,omitempty"`
	RemainingSeconds *int64     `json:"remaining_seconds,omitempty"`
	Utilization      *float64   `json:"utilization,omitempty"`
	SampledAt        *time.Time `json:"sampled_at,omitempty"`
	Source           string     `json:"source"`
	Stale            bool       `json:"stale"`
}

const defaultQuotaResetDataMaxAge = 15 * time.Minute

// ResolveAccountQuotaReset resolves the best reset window for a request. It
// only reads already cached account fields and is safe for request hot paths.
// preferredWindow accepts auto, 5h, 7d, and model_specific.
func ResolveAccountQuotaReset(account *Account, platform, model, preferredWindow string, now time.Time) AccountQuotaResetSnapshot {
	return resolveAccountQuotaResetWithMaxAge(account, platform, model, preferredWindow, now, defaultQuotaResetDataMaxAge)
}

func resolveAccountQuotaResetWithMaxAge(account *Account, platform, model, preferredWindow string, now time.Time, maxAge time.Duration) AccountQuotaResetSnapshot {
	if account == nil {
		return AccountQuotaResetSnapshot{Source: "unavailable"}
	}
	if now.IsZero() {
		now = time.Now()
	}
	mode := strings.ToLower(strings.TrimSpace(preferredWindow))
	if mode == "" {
		mode = "auto"
	}
	windows := []string{mode}
	if mode == "auto" {
		if platform == PlatformAnthropic || platform == "anthropic" {
			if isAnthropicFableModel(model) {
				windows = []string{"7d_oi"}
			} else {
				windows = []string{"7d"}
			}
		} else if platform == PlatformOpenAI || platform == "openai" || platform == "codex" {
			windows = []string{"7d"}
		} else {
			windows = []string{"5h"}
		}
	} else if mode == "model_specific" {
		if isAnthropicFableModel(model) {
			windows = []string{"7d_oi", "7d"}
		} else {
			windows = []string{"7d"}
		}
	}
	var staleSnapshot AccountQuotaResetSnapshot
	for _, window := range windows {
		if snap, ok := quotaResetWindow(account, window, now, maxAge); ok {
			return snap
		} else if snap.Stale {
			staleSnapshot = snap
		}
	}
	if staleSnapshot.Stale {
		return staleSnapshot
	}
	return AccountQuotaResetSnapshot{Window: windows[len(windows)-1], Source: "unavailable"}
}

func quotaResetWindow(account *Account, window string, now time.Time, maxAge time.Duration) (AccountQuotaResetSnapshot, bool) {
	extra := account.Extra
	snap := AccountQuotaResetSnapshot{Window: window}
	var reset *time.Time
	var utilization *float64
	source := ""
	var sampledAt *time.Time
	switch window {
	case "7d":
		if account.Platform == PlatformOpenAI || account.Platform == "openai" || account.Platform == "codex" {
			reset = parseQuotaTime(extraValue(extra, "codex_7d_reset_at"))
			if reset == nil {
				reset = resetAfter(now, extraValue(extra, "codex_7d_reset_after_seconds"))
			}
			utilization = parseFloatPtr(extraValue(extra, "codex_7d_used_percent"), true)
			sampledAt = firstTime(parseQuotaTime(extraValue(extra, "codex_usage_updated_at")), sampledAt)
			source = "active_probe"
		} else {
			reset = parseQuotaTime(extraValue(extra, "passive_usage_7d_reset"))
			utilization = parseFloatPtr(extraValue(extra, "passive_usage_7d_utilization"), false)
			sampledAt = parseQuotaTime(extraValue(extra, "passive_usage_sampled_at"))
			source = "passive_cache"
		}
	case "7d_oi":
		reset = parseQuotaTime(extraValue(extra, "passive_usage_7d_oi_reset"))
		utilization = parseFloatPtr(extraValue(extra, "passive_usage_7d_oi_utilization"), false)
		sampledAt = parseQuotaTime(extraValue(extra, "passive_usage_sampled_at"))
		source = "passive_cache"
	case "5h":
		reset = account.SessionWindowEnd
		if reset == nil {
			reset = parseQuotaTime(extraValue(extra, "codex_5h_reset_at"))
			if reset == nil {
				reset = resetAfter(now, extraValue(extra, "codex_5h_reset_after_seconds"))
			}
		}
		utilization = parseFloatPtr(extraValue(extra, "codex_5h_used_percent"), true)
		source = "session_window"
		if extraValue(extra, "codex_5h_reset_at") != nil || extraValue(extra, "codex_5h_reset_after_seconds") != nil {
			source = "active_probe"
		}
	}
	if reset == nil {
		return snap, false
	}
	if sampledAt != nil {
		snap.SampledAt = sampledAt
		snap.Stale = maxAge > 0 && now.Sub(*sampledAt) > maxAge
	}
	if !reset.After(now) {
		return snap, false
	}
	if snap.Stale {
		return snap, false
	}
	resetCopy := reset.UTC()
	remaining := int64(resetCopy.Sub(now).Seconds())
	snap.ResetAt = &resetCopy
	snap.RemainingSeconds = &remaining
	snap.Utilization = utilization
	snap.Source = source
	return snap, true
}

// compareQuotaResetSnapshots returns -1 when a should be selected before b.
// The ordering intentionally treats whole remaining days as the primary
// window bucket. Within the same day, remaining quota is preferred; hours and
// the exact timestamp are only tie-breakers.
func compareQuotaResetSnapshots(a, b AccountQuotaResetSnapshot, now time.Time) int {
	if a.ResetAt == nil && b.ResetAt == nil {
		return 0
	}
	if a.ResetAt != nil && b.ResetAt == nil {
		return -1
	}
	if a.ResetAt == nil {
		return 1
	}
	aSeconds := int64(a.ResetAt.Sub(now).Seconds())
	bSeconds := int64(b.ResetAt.Sub(now).Seconds())
	if aSeconds < 0 { aSeconds = 0 }
	if bSeconds < 0 { bSeconds = 0 }
	aDays, bDays := aSeconds/(24*60*60), bSeconds/(24*60*60)
	if aDays != bDays {
		if aDays < bDays { return -1 }
		return 1
	}
	if a.Utilization != nil && b.Utilization == nil {
		return -1
	}
	if a.Utilization == nil && b.Utilization != nil {
		return 1
	}
	if a.Utilization != nil && b.Utilization != nil && *a.Utilization != *b.Utilization {
		if *a.Utilization < *b.Utilization { return -1 }
		return 1
	}
	aHours := (aSeconds % (24 * 60 * 60)) / (60 * 60)
	bHours := (bSeconds % (24 * 60 * 60)) / (60 * 60)
	if aHours != bHours {
		if aHours < bHours { return -1 }
		return 1
	}
	if a.ResetAt.Before(*b.ResetAt) { return -1 }
	if a.ResetAt.After(*b.ResetAt) { return 1 }
	return 0
}

func extraValue(extra map[string]any, key string) any {
	if extra == nil {
		return nil
	}
	return extra[key]
}

func firstTime(a, b *time.Time) *time.Time {
	if a != nil { return a }
	return b
}

func parseFloatPtr(value any, percent bool) *float64 {
	var n float64
	switch v := value.(type) {
	case float64: n = v
	case float32: n = float64(v)
	case int: n = float64(v)
	case int64: n = float64(v)
	case string:
		var err error
		n, err = strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil { return nil }
	default: return nil
	}
	if percent && n > 1 { n /= 100 }
	return &n
}

func resetAfter(now time.Time, value any) *time.Time {
	seconds := parseFloatPtr(value, false)
	if seconds == nil || *seconds <= 0 {
		return nil
	}
	t := now.Add(time.Duration(*seconds * float64(time.Second)))
	return &t
}

func parseQuotaTime(value any) *time.Time {
	if value == nil { return nil }
	switch v := value.(type) {
	case time.Time: return &v
	case *time.Time: return v
	case int64: t := time.Unix(v, 0); return &t
	case int: t := time.Unix(int64(v), 0); return &t
	case float64: t := time.Unix(int64(v), 0); return &t
	case string:
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(v)); err == nil { return &t }
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil { t := time.Unix(n, 0); return &t }
	}
	return nil
}

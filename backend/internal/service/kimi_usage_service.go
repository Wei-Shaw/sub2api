package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
)

const (
	kimiUsageRequestTimeout = 15 * time.Second
	kimiUsageBodyLimit      = 1 << 20
)

// kimiUsageTokenProvider keeps AccountUsageService testable while reusing the
// request-path token provider (including refresh-token rotation) in production.
type kimiUsageTokenProvider interface {
	GetAccessToken(ctx context.Context, account *Account) (string, error)
}

type kimiUsageLimit struct {
	Detail map[string]any `json:"detail"`
	Window map[string]any `json:"window"`
}

type kimiUsageResponse struct {
	Usage  map[string]any   `json:"usage"`
	Limits []kimiUsageLimit `json:"limits"`
	User   struct {
		Membership struct {
			Level string `json:"level"`
		} `json:"membership"`
	} `json:"user"`
}

func (s *AccountUsageService) getKimiUsage(ctx context.Context, account *Account, force bool) (*UsageInfo, error) {
	if account == nil || !account.IsKimiOAuth() {
		return nil, fmt.Errorf("account is not a Kimi OAuth account")
	}
	if s.cache == nil {
		return s.fetchKimiUsage(ctx, account)
	}

	if !force {
		if cached, ok := s.cache.kimiUsageCache.Load(account.ID); ok {
			if entry, ok := cached.(*kimiUsageCache); ok {
				age := time.Since(entry.timestamp)
				if entry.err != nil && age < apiErrorCacheTTL {
					return nil, entry.err
				}
				if entry.usageInfo != nil && age < apiCacheTTL {
					return entry.usageInfo, nil
				}
			}
		}
	}

	flightKey := fmt.Sprintf("kimi-usage:%d", account.ID)
	result, err, _ := s.cache.kimiUsageFlight.Do(flightKey, func() (any, error) {
		if !force {
			if cached, ok := s.cache.kimiUsageCache.Load(account.ID); ok {
				if entry, ok := cached.(*kimiUsageCache); ok {
					age := time.Since(entry.timestamp)
					if entry.err != nil && age < apiErrorCacheTTL {
						return nil, entry.err
					}
					if entry.usageInfo != nil && age < apiCacheTTL {
						return entry.usageInfo, nil
					}
				}
			}
		}

		usage, fetchErr := s.fetchKimiUsage(ctx, account)
		s.cache.kimiUsageCache.Store(account.ID, &kimiUsageCache{
			usageInfo: usage,
			err:       fetchErr,
			timestamp: time.Now(),
		})
		if fetchErr != nil {
			return nil, fetchErr
		}
		return usage, nil
	})
	if err != nil {
		return nil, err
	}
	usage, ok := result.(*UsageInfo)
	if !ok || usage == nil {
		return nil, fmt.Errorf("invalid Kimi usage response")
	}
	return usage, nil
}

func (s *AccountUsageService) fetchKimiUsage(ctx context.Context, account *Account) (*UsageInfo, error) {
	if s.kimiTokenProvider == nil {
		return nil, fmt.Errorf("Kimi token provider is not configured")
	}
	if s.httpUpstream == nil {
		return nil, fmt.Errorf("HTTP upstream is not configured")
	}

	accessToken, err := s.kimiTokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("get Kimi access token: %w", err)
	}
	usageURL, err := kimi.BuildUsagesURL(account.GetKimiBaseURL())
	if err != nil {
		return nil, fmt.Errorf("build Kimi usages URL: %w", err)
	}

	requestCtx, cancel := context.WithTimeout(ctx, kimiUsageRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build Kimi usage request: %w", err)
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	kimi.SetFingerprintHeaders(req.Header, account.GetKimiDeviceID())
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, fmt.Errorf("fetch Kimi usage: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, kimiUsageBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("read Kimi usage response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Kimi usages API returned status %d", resp.StatusCode)
	}

	var payload kimiUsageResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode Kimi usage response: %w", err)
	}
	usage := buildKimiUsageInfo(&payload, time.Now())
	if usage.FiveHour == nil && usage.SevenDay == nil {
		return nil, fmt.Errorf("Kimi usages API returned no recognizable usage windows")
	}
	s.persistKimiMembership(account, usage)
	return usage, nil
}

func buildKimiUsageInfo(payload *kimiUsageResponse, now time.Time) *UsageInfo {
	info := &UsageInfo{Source: "active", UpdatedAt: &now}
	if payload == nil {
		return info
	}
	info.SubscriptionTier, info.SubscriptionTierRaw = normalizeKimiMembershipLevel(payload.User.Membership.Level)

	// The official kimi-cli labels the top-level usage object as the weekly limit.
	info.SevenDay = buildKimiUsageProgress(payload.Usage, now)

	var firstLimit *UsageProgress
	for _, limit := range payload.Limits {
		progress := buildKimiUsageProgress(limit.Detail, now)
		if progress == nil {
			continue
		}
		if firstLimit == nil {
			firstLimit = progress
		}
		durationMinutes := kimiWindowDurationMinutes(limit.Window)
		switch durationMinutes {
		case 5 * 60:
			info.FiveHour = progress
		case 7 * 24 * 60:
			if info.SevenDay == nil {
				info.SevenDay = progress
			}
		}
	}
	// Older Kimi responses omitted window metadata but still returned one
	// short-period limit. Keep it visible as the 5h window.
	if info.FiveHour == nil {
		info.FiveHour = firstLimit
	}
	return info
}

func normalizeKimiMembershipLevel(raw string) (normalized, original string) {
	original = strings.TrimSpace(raw)
	if original == "" {
		return "", ""
	}
	normalized = strings.TrimPrefix(strings.ToUpper(original), "LEVEL_")
	if normalized == "" {
		return "", original
	}
	return normalized, original
}

func (s *AccountUsageService) persistKimiMembership(account *Account, usage *UsageInfo) {
	if s == nil || s.accountRepo == nil || account == nil || account.ID <= 0 || usage == nil {
		return
	}
	level := strings.TrimSpace(usage.SubscriptionTierRaw)
	if level == "" {
		return
	}

	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	if existing, _ := account.Extra["kimi_membership_level"].(string); existing == level {
		return
	}
	account.Extra["kimi_membership_level"] = level

	go func(accountID int64) {
		updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.accountRepo.UpdateExtra(updateCtx, accountID, map[string]any{
			"kimi_membership_level": level,
		}); err != nil {
			// Membership display is best-effort and must not make usage probing fail.
			slog.Warn("persist_kimi_membership_failed", "account_id", accountID, "error", err)
		}
	}(account.ID)
}

func buildKimiUsageProgress(metric map[string]any, now time.Time) *UsageProgress {
	if len(metric) == 0 {
		return nil
	}
	limit, hasLimit := kimiInt64(metric["limit"])
	used, hasUsed := kimiInt64(metric["used"])
	if !hasUsed {
		if remaining, ok := kimiInt64(metric["remaining"]); ok && hasLimit {
			used = limit - remaining
			hasUsed = true
		}
	}
	if !hasLimit && !hasUsed {
		return nil
	}
	if used < 0 {
		used = 0
	}

	progress := &UsageProgress{UsedRequests: used, LimitRequests: limit}
	if limit > 0 {
		progress.Utilization = float64(used) / float64(limit) * 100
	}
	if resetAt, ok := kimiResetTime(metric); ok {
		progress.ResetsAt = &resetAt
		progress.RemainingSeconds = max(0, int(resetAt.Sub(now).Seconds()))
	}
	return progress
}

func kimiWindowDurationMinutes(window map[string]any) int64 {
	duration, ok := kimiInt64(window["duration"])
	if !ok || duration <= 0 {
		return 0
	}
	unit := strings.ToUpper(strings.TrimSpace(fmt.Sprint(window["timeUnit"])))
	switch {
	case strings.Contains(unit, "MINUTE"):
		return duration
	case strings.Contains(unit, "HOUR"):
		return duration * 60
	case strings.Contains(unit, "DAY"):
		return duration * 24 * 60
	case strings.Contains(unit, "SECOND"):
		return duration / 60
	default:
		return 0
	}
}

func kimiResetTime(metric map[string]any) (time.Time, bool) {
	for _, key := range []string{"resetTime", "reset_at", "resetAt", "reset_time"} {
		raw, ok := metric[key]
		if !ok {
			continue
		}
		value := strings.TrimSpace(fmt.Sprint(raw))
		if value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func kimiInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		return int64(v), true
	case json.Number:
		parsed, err := v.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

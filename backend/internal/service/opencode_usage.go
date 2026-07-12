package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	httppool "github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

const (
	openCodeUsageURL      = "https://console.opencode.ai/zen/go/v1/usage"
	openCodeUsageTimeout  = 15 * time.Second
	openCodeUsageCacheTTL = 3 * time.Minute
)

const (
	openCodeUsageUpdatedAtKey = "opencode_usage_updated_at"
	openCodeRollingPercentKey = "opencode_rolling_used_percent"
	openCodeRollingResetAtKey = "opencode_rolling_reset_at"
	openCodeWeeklyPercentKey  = "opencode_weekly_used_percent"
	openCodeWeeklyResetAtKey  = "opencode_weekly_reset_at"
	openCodeMonthlyPercentKey = "opencode_monthly_used_percent"
	openCodeMonthlyResetAtKey = "opencode_monthly_reset_at"
)

type openCodeUsageWindow struct {
	Status            string  `json:"status"`
	UsagePercent      float64 `json:"usagePercent"`
	ResetsInSeconds   int64   `json:"resetsInSeconds"`
	ResetInSecondsAlt int64   `json:"resetInSec"`
}

func (w *openCodeUsageWindow) resetSeconds() int64 {
	if w == nil {
		return 0
	}
	if w.ResetsInSeconds > 0 {
		return w.ResetsInSeconds
	}
	return w.ResetInSecondsAlt
}

type openCodeUsageResponse struct {
	Rolling *openCodeUsageWindow `json:"rolling"`
	Weekly  *openCodeUsageWindow `json:"weekly"`
	Monthly *openCodeUsageWindow `json:"monthly"`
}

type openCodeUsageHTTPError struct {
	StatusCode int
}

type openCodeUsageAuthError struct{}

func (*openCodeUsageAuthError) Error() string {
	return "OpenCode quota auth cookie is invalid or expired"
}

func (e *openCodeUsageHTTPError) Error() string {
	if e == nil {
		return "OpenCode usage request failed"
	}
	return fmt.Sprintf("OpenCode usage endpoint returned HTTP %d", e.StatusCode)
}

func (s *AccountUsageService) getOpenCodeUsage(ctx context.Context, account *Account, force bool) (*UsageInfo, error) {
	now := time.Now()
	cached := buildOpenCodeUsageFromExtra(account, now)
	if !force && cached != nil && isOpenCodeUsageSnapshotFresh(account, now) {
		return cached, nil
	}

	authCookie := strings.TrimSpace(account.GetCredential("auth_cookie"))
	if authCookie == "" {
		if cached != nil {
			cached.ErrorCode = "credentials_missing"
			cached.Error = "OpenCode quota auth cookie is not configured"
			return cached, nil
		}
		return &UsageInfo{
			Source:    "active",
			UpdatedAt: &now,
			ErrorCode: "credentials_missing",
			Error:     "OpenCode quota auth cookie is not configured",
		}, nil
	}

	flightKey := fmt.Sprintf("opencode_usage:%d", account.ID)
	var (
		result   any
		fetchErr error
	)
	if s.cache != nil {
		result, fetchErr, _ = s.cache.openCodeUsageFlight.Do(flightKey, func() (any, error) {
			return fetchOpenCodeUsage(ctx, account, openCodeUsageURL)
		})
	} else {
		result, fetchErr = fetchOpenCodeUsage(ctx, account, openCodeUsageURL)
	}
	if fetchErr != nil {
		if cached == nil {
			cached = &UsageInfo{Source: "active", UpdatedAt: &now}
		}
		cached.ErrorCode = classifyOpenCodeUsageError(fetchErr)
		cached.Error = fetchErr.Error()
		return cached, nil
	}

	snapshot, _ := result.(*openCodeUsageResponse)
	usage := buildOpenCodeUsageInfo(snapshot, now)
	if usage.OpenCodeRolling == nil && usage.OpenCodeWeekly == nil && usage.OpenCodeMonthly == nil {
		usage.ErrorCode = "upstream_unavailable"
		usage.Error = "OpenCode usage endpoint returned no active quota windows"
		return usage, nil
	}
	updates := buildOpenCodeUsageExtraUpdates(usage, now)
	if len(updates) > 0 {
		if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
			slog.Warn("opencode_usage_snapshot_persist_failed", "account_id", account.ID, "error", err)
		}
	}
	return usage, nil
}

func fetchOpenCodeUsage(ctx context.Context, account *Account, endpoint string) (*openCodeUsageResponse, error) {
	if account == nil {
		return nil, fmt.Errorf("OpenCode account is nil")
	}
	authCookie := normalizeOpenCodeAuthCookie(account.GetCredential("auth_cookie"))
	if authCookie == "" {
		return nil, fmt.Errorf("OpenCode quota auth cookie is not configured")
	}

	reqCtx, cancel := context.WithTimeout(ctx, openCodeUsageTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create OpenCode usage request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cookie", authCookie)
	req.Header.Set("User-Agent", "sub2api-opencode-usage/1.0")

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	client, err := httppool.GetClient(httppool.Options{
		ProxyURL:              proxyURL,
		Timeout:               openCodeUsageTimeout,
		ResponseHeaderTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("build OpenCode usage client: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenCode usage request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return nil, &openCodeUsageHTTPError{StatusCode: resp.StatusCode}
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "json") {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return nil, &openCodeUsageAuthError{}
	}

	var snapshot openCodeUsageResponse
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 64<<10))
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("decode OpenCode usage response: %w", err)
	}
	if snapshot.Rolling == nil && snapshot.Weekly == nil && snapshot.Monthly == nil {
		return nil, fmt.Errorf("OpenCode usage response did not contain quota windows")
	}
	return &snapshot, nil
}

func normalizeOpenCodeAuthCookie(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "auth=") {
		return value
	}
	return "auth=" + value
}

func buildOpenCodeUsageInfo(snapshot *openCodeUsageResponse, now time.Time) *UsageInfo {
	usage := &UsageInfo{Source: "active", UpdatedAt: &now}
	if snapshot == nil {
		return usage
	}
	usage.OpenCodeRolling = buildOpenCodeUsageProgress(snapshot.Rolling, now)
	usage.OpenCodeWeekly = buildOpenCodeUsageProgress(snapshot.Weekly, now)
	usage.OpenCodeMonthly = buildOpenCodeUsageProgress(snapshot.Monthly, now)
	return usage
}

func buildOpenCodeUsageProgress(window *openCodeUsageWindow, now time.Time) *UsageProgress {
	if window == nil || (window.Status != "" && window.Status != "ok") {
		return nil
	}
	utilization := window.UsagePercent
	if utilization < 0 {
		utilization = 0
	} else if utilization > 100 {
		utilization = 100
	}
	remaining := window.resetSeconds()
	if remaining < 0 {
		remaining = 0
	}
	var resetsAt *time.Time
	if remaining > 0 {
		reset := now.Add(time.Duration(remaining) * time.Second)
		resetsAt = &reset
	}
	return &UsageProgress{
		Utilization:      utilization,
		ResetsAt:         resetsAt,
		RemainingSeconds: int(remaining),
	}
}

func buildOpenCodeUsageExtraUpdates(usage *UsageInfo, now time.Time) map[string]any {
	if usage == nil {
		return nil
	}
	updates := map[string]any{openCodeUsageUpdatedAtKey: now.UTC().Format(time.RFC3339)}
	appendWindow := func(window *UsageProgress, percentKey, resetKey string) {
		if window == nil {
			return
		}
		updates[percentKey] = window.Utilization
		if window.ResetsAt != nil {
			updates[resetKey] = window.ResetsAt.Unix()
		}
	}
	appendWindow(usage.OpenCodeRolling, openCodeRollingPercentKey, openCodeRollingResetAtKey)
	appendWindow(usage.OpenCodeWeekly, openCodeWeeklyPercentKey, openCodeWeeklyResetAtKey)
	appendWindow(usage.OpenCodeMonthly, openCodeMonthlyPercentKey, openCodeMonthlyResetAtKey)
	return updates
}

func buildOpenCodeUsageFromExtra(account *Account, now time.Time) *UsageInfo {
	if account == nil || account.Extra == nil {
		return nil
	}
	rolling := buildOpenCodeUsageProgressFromExtra(account.Extra, openCodeRollingPercentKey, openCodeRollingResetAtKey, now)
	weekly := buildOpenCodeUsageProgressFromExtra(account.Extra, openCodeWeeklyPercentKey, openCodeWeeklyResetAtKey, now)
	monthly := buildOpenCodeUsageProgressFromExtra(account.Extra, openCodeMonthlyPercentKey, openCodeMonthlyResetAtKey, now)
	if rolling == nil && weekly == nil && monthly == nil {
		return nil
	}
	usage := &UsageInfo{
		Source:          "active",
		OpenCodeRolling: rolling,
		OpenCodeWeekly:  weekly,
		OpenCodeMonthly: monthly,
	}
	if raw, ok := account.Extra[openCodeUsageUpdatedAtKey]; ok {
		if parsed, err := parseTime(fmt.Sprint(raw)); err == nil {
			usage.UpdatedAt = &parsed
		}
	}
	return usage
}

func buildOpenCodeUsageProgressFromExtra(extra map[string]any, percentKey, resetKey string, now time.Time) *UsageProgress {
	_, hasPercent := extra[percentKey]
	_, hasReset := extra[resetKey]
	if !hasPercent && !hasReset {
		return nil
	}
	utilization := parseExtraFloat64(extra[percentKey])
	if utilization < 0 {
		utilization = 0
	} else if utilization > 100 {
		utilization = 100
	}
	resetUnix := int64(parseExtraFloat64(extra[resetKey]))
	var resetsAt *time.Time
	remaining := 0
	if resetUnix > 0 {
		reset := time.Unix(resetUnix, 0)
		resetsAt = &reset
		remaining = int(reset.Sub(now).Seconds())
		if remaining < 0 {
			remaining = 0
		}
	}
	return &UsageProgress{Utilization: utilization, ResetsAt: resetsAt, RemainingSeconds: remaining}
}

func isOpenCodeUsageSnapshotFresh(account *Account, now time.Time) bool {
	if account == nil || account.Extra == nil {
		return false
	}
	raw, ok := account.Extra[openCodeUsageUpdatedAtKey]
	if !ok {
		return false
	}
	updatedAt, err := parseTime(fmt.Sprint(raw))
	if err != nil {
		return false
	}
	return now.Sub(updatedAt) < openCodeUsageCacheTTL
}

func classifyOpenCodeUsageError(err error) string {
	if _, ok := err.(*openCodeUsageAuthError); ok {
		return "unauthenticated"
	}
	httpErr, ok := err.(*openCodeUsageHTTPError)
	if !ok {
		return "network_error"
	}
	switch httpErr.StatusCode {
	case http.StatusUnauthorized:
		return "unauthenticated"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusTooManyRequests:
		return "rate_limited"
	default:
		return "network_error"
	}
}

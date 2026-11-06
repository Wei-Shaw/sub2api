package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	httppool "github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	openaipkg "github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

func (s *AccountUsageService) getOpenAIUsage(ctx context.Context, account *Account, force bool) (*UsageInfo, error) {
	now := time.Now()
	usage := &UsageInfo{UpdatedAt: &now}

	if account == nil {
		return usage, nil
	}

	applyExtraToUsage(usage, account.Extra, now)

	if (force || shouldRefreshOpenAICodexSnapshot(account, usage, now)) && s.shouldProbeOpenAICodexSnapshot(account.ID, now, force) {
		if adminaccess.RestrictedFromContext(ctx) != nil {
			// 受限额度读取只调用额度 API，不生成 /responses 合成请求或消费重置次数。
			if s.openAIQuotaService != nil {
				if quotaUsage, err := s.openAIQuotaService.QueryUsage(WithScopedAdminAccountOperation(ctx, "accounts.usage.read"), account.ID); err == nil {
					updates := buildOpenAIAutoResetUsageUpdates(quotaUsage, now)
					if account.IsShadow() {
						updates = buildCodexSparkWindowExtraUpdates(quotaUsage, now)
					}
					if len(updates) > 0 {
						mergeAccountExtra(account, updates)
						s.persistOpenAICodexProbeSnapshotContext(ctx, account.ID, updates)
						applyExtraToUsage(usage, account.Extra, now)
					}
				}
			}
		} else if account.IsShadow() {
			// Spark shadow accounts fetch usage from /wham/usage (bengalfox channel)
			// via the shared OpenAIQuotaService, which resolves credentials from the
			// parent account. The result is written to the shadow row's own codex_*
			// Extra keys and immediately reflected in the returned UsageInfo.
			if s.openAIQuotaService != nil {
				if quotaUsage, err := s.openAIQuotaService.QueryUsage(WithScopedAdminAccountOperation(ctx, "accounts.usage.read"), account.ID); err == nil {
					if updates := buildCodexSparkWindowExtraUpdates(quotaUsage, now); len(updates) > 0 {
						mergeAccountExtra(account, updates)
						s.persistOpenAICodexProbeSnapshotContext(ctx, account.ID, updates)
						if account.ParentAccountID != nil && adminaccess.RestrictedFromContext(ctx) == nil {
							notifyOpenAIAutoReset(*account.ParentAccountID)
						}
						if usage.UpdatedAt == nil {
							usage.UpdatedAt = &now
						}
						applyExtraToUsage(usage, account.Extra, now)
					}
				}
			}
		} else {
			if updates, err := s.probeOpenAICodexSnapshot(ctx, account); err == nil && len(updates) > 0 {
				mergeAccountExtra(account, updates)
				if usage.UpdatedAt == nil {
					usage.UpdatedAt = &now
				}
				applyExtraToUsage(usage, account.Extra, now)
			}
		}
	}

	if s.usageLogRepo == nil {
		return usage, nil
	}

	if stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, account.ID, codexWindowStatsStart(usage.FiveHour, 5*time.Hour, now)); err == nil {
		if usage.FiveHour == nil {
			usage.FiveHour = &UsageProgress{Utilization: 0}
		}
		usage.FiveHour.WindowStats = windowStatsFromAccountStats(stats)
	}

	if stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, account.ID, codexWindowStatsStart(usage.SevenDay, 7*24*time.Hour, now)); err == nil {
		if usage.SevenDay == nil {
			usage.SevenDay = &UsageProgress{Utilization: 0}
		}
		usage.SevenDay.WindowStats = windowStatsFromAccountStats(stats)
	}

	return usage, nil
}

func shouldRefreshOpenAICodexSnapshot(account *Account, usage *UsageInfo, now time.Time) bool {
	if account == nil {
		return false
	}
	if usage == nil {
		return true
	}
	if usage.FiveHour == nil || usage.SevenDay == nil {
		return true
	}
	if account.IsRateLimited() {
		return true
	}
	return isOpenAICodexSnapshotStale(account, now)
}

func isOpenAICodexSnapshotStale(account *Account, now time.Time) bool {
	if account == nil || !account.IsOpenAIOAuth() {
		return false
	}
	if !account.IsShadow() && !account.IsOpenAIResponsesWebSocketV2Enabled() {
		return false
	}
	if account.Extra == nil {
		return true
	}
	raw, ok := account.Extra["codex_usage_updated_at"]
	if !ok {
		return true
	}
	ts, err := parseTime(fmt.Sprint(raw))
	if err != nil {
		return true
	}
	return now.Sub(ts) >= openAIProbeCacheTTL
}

func (s *AccountUsageService) shouldProbeOpenAICodexSnapshot(accountID int64, now time.Time, force ...bool) bool {
	if s == nil || s.cache == nil || accountID <= 0 {
		return true
	}
	forceProbe := len(force) > 0 && force[0]
	if !forceProbe {
		if cached, ok := s.cache.openAIProbeCache.Load(accountID); ok {
			if ts, ok := cached.(time.Time); ok && now.Sub(ts) < openAIProbeCacheTTL {
				return false
			}
		}
	}
	s.cache.openAIProbeCache.Store(accountID, now)
	return true
}

func (s *AccountUsageService) probeOpenAICodexSnapshot(ctx context.Context, account *Account) (map[string]any, error) {
	if account == nil || !account.IsOAuth() {
		return nil, nil
	}
	accessToken := ""
	if !account.IsOpenAIAgentIdentity() {
		accessToken = account.GetOpenAIAccessToken()
	}
	if accessToken == "" && !account.IsOpenAIAgentIdentity() {
		return nil, fmt.Errorf("no access token available")
	}
	modelID := openaipkg.CodexUsageProbeModel
	payload := createOpenAITestPayload(modelID, true)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal openai probe payload: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, chatgptCodexURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create openai probe request: %w", err)
	}
	req.Host = "chatgpt.com"
	req.Header.Set("Content-Type", "application/json")
	if account.IsOpenAIAgentIdentity() {
		authHeaders, authErr := buildAgentIdentityAuthenticationHeaders(ctx, s.accountRepo, s.agentIdentityWS, &s.agentIdentityTaskMu, account)
		if authErr != nil {
			return nil, fmt.Errorf("build Agent Identity authentication: %w", authErr)
		}
		for key, values := range authHeaders {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	} else {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	canonical := resolveCodexOutboundIdentity("")
	req.Header.Set("Originator", canonical.originator)
	req.Header.Set("Version", canonical.version)
	req.Header.Set("User-Agent", canonical.userAgent)
	if s.identityCache != nil {
		if fp, fpErr := s.identityCache.GetFingerprint(reqCtx, account.ID); fpErr == nil && fp != nil && strings.TrimSpace(fp.UserAgent) != "" {
			req.Header.Set("User-Agent", strings.TrimSpace(fp.UserAgent))
		}
	}
	enforceCodexIdentityHeadersWithUA(req.Header, account.GetOpenAIUserAgent())
	setOpenAIChatGPTAccountHeaders(req.Header, account)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	client, err := httppool.GetClient(httppool.Options{
		ProxyURL:              proxyURL,
		Timeout:               15 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("build openai probe client: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai codex probe request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	updates, err := extractOpenAICodexProbeUpdates(resp)
	if err != nil {
		return nil, err
	}
	if len(updates) > 0 {
		s.persistOpenAICodexProbeSnapshotContext(ctx, account.ID, updates)
		return updates, nil
	}
	return nil, nil
}

func (s *AccountUsageService) persistOpenAICodexProbeSnapshot(accountID int64, updates map[string]any) {
	s.persistOpenAICodexProbeSnapshotContext(context.Background(), accountID, updates)
}

func (s *AccountUsageService) persistOpenAICodexProbeSnapshotContext(ctx context.Context, accountID int64, updates map[string]any) {
	if s == nil || s.accountRepo == nil || accountID <= 0 {
		return
	}
	if len(updates) == 0 {
		return
	}

	go func() {
		updateCtx, updateCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer updateCancel()
		if err := scopedAccountIDAccess(updateCtx, s.accountRepo, accountID, "accounts.usage.read", true); err != nil {
			return
		}
		if err := s.accountRepo.UpdateExtra(updateCtx, accountID, updates); err == nil && adminaccess.RestrictedFromContext(ctx) == nil {
			notifyOpenAIAutoReset(accountID)
		}
	}()
}

func extractOpenAICodexProbeUpdates(resp *http.Response) (map[string]any, error) {
	if resp == nil {
		return nil, nil
	}
	if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
		return buildCodexUsageExtraUpdates(snapshot, time.Now()), nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai codex probe returned status %d", resp.StatusCode)
	}
	return nil, nil
}

func mergeAccountExtra(account *Account, updates map[string]any) {
	if account == nil || len(updates) == 0 {
		return
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any, len(updates))
	}
	for k, v := range updates {
		account.Extra[k] = v
	}
}

func applyExtraToUsage(usage *UsageInfo, extra map[string]any, now time.Time) {
	if usage == nil {
		return
	}
	if progress := buildCodexUsageProgressFromExtra(extra, "5h", now); progress != nil {
		usage.FiveHour = progress
	}
	if progress := buildCodexUsageProgressFromExtra(extra, "7d", now); progress != nil {
		usage.SevenDay = progress
	}
}

func buildCodexUsageProgressFromExtra(extra map[string]any, window string, now time.Time) *UsageProgress {
	if len(extra) == 0 {
		return nil
	}

	var (
		usedPercentKey string
		resetAfterKey  string
		resetAtKey     string
	)

	switch window {
	case "5h":
		usedPercentKey = "codex_5h_used_percent"
		resetAfterKey = "codex_5h_reset_after_seconds"
		resetAtKey = "codex_5h_reset_at"
	case "7d":
		usedPercentKey = "codex_7d_used_percent"
		resetAfterKey = "codex_7d_reset_after_seconds"
		resetAtKey = "codex_7d_reset_at"
	default:
		return nil
	}

	usedRaw, ok := extra[usedPercentKey]
	if !ok {
		return nil
	}

	progress := &UsageProgress{Utilization: parseExtraFloat64(usedRaw)}
	if resetAtRaw, ok := extra[resetAtKey]; ok {
		if resetAt, err := parseTime(fmt.Sprint(resetAtRaw)); err == nil {
			progress.ResetsAt = &resetAt
			progress.RemainingSeconds = int(time.Until(resetAt).Seconds())
			if progress.RemainingSeconds < 0 {
				progress.RemainingSeconds = 0
			}
		}
	}
	if progress.ResetsAt == nil {
		if resetAfterSeconds := parseExtraInt(extra[resetAfterKey]); resetAfterSeconds > 0 {
			base := now
			if updatedAtRaw, ok := extra["codex_usage_updated_at"]; ok {
				if updatedAt, err := parseTime(fmt.Sprint(updatedAtRaw)); err == nil {
					base = updatedAt
				}
			}
			resetAt := base.Add(time.Duration(resetAfterSeconds) * time.Second)
			progress.ResetsAt = &resetAt
			progress.RemainingSeconds = int(time.Until(resetAt).Seconds())
			if progress.RemainingSeconds < 0 {
				progress.RemainingSeconds = 0
			}
		}
	}

	if progress.ResetsAt != nil && !now.Before(*progress.ResetsAt) {
		progress.Utilization = 0
	}

	return progress
}

func codexWindowStatsStart(progress *UsageProgress, fallbackWindow time.Duration, now time.Time) time.Time {
	if progress != nil && progress.ResetsAt != nil && now.Before(*progress.ResetsAt) {
		return progress.ResetsAt.Add(-fallbackWindow)
	}
	return now.Add(-fallbackWindow)
}

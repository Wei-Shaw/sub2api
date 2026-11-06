package service

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"golang.org/x/sync/errgroup"
)

// NewAccountUsageService 创建AccountUsageService实例
func NewAccountUsageService(
	accountRepo AccountRepository,
	usageLogRepo UsageLogRepository,
	usageFetcher ClaudeUsageFetcher,
	geminiQuotaService *GeminiQuotaService,
	antigravityQuotaFetcher *AntigravityQuotaFetcher,
	grokQuotaFetcher *GrokQuotaFetcher,
	grokQuotaService *GrokQuotaService,
	openAIQuotaService *OpenAIQuotaService,
	cache *UsageCache,
	identityCache IdentityCache,
	tlsFPProfileService *TLSFingerprintProfileService,
) *AccountUsageService {
	return &AccountUsageService{
		accountRepo:             accountRepo,
		usageLogRepo:            usageLogRepo,
		usageFetcher:            usageFetcher,
		geminiQuotaService:      geminiQuotaService,
		antigravityQuotaFetcher: antigravityQuotaFetcher,
		grokQuotaFetcher:        grokQuotaFetcher,
		grokQuotaService:        grokQuotaService,
		openAIQuotaService:      openAIQuotaService,
		cache:                   cache,
		identityCache:           identityCache,
		tlsFPProfileService:     tlsFPProfileService,
	}
}

func supportsAnthropicPassiveUsage(account *Account) bool {
	return account != nil && account.IsAnthropicOAuthOrSetupToken()
}

func batchUsageErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *AccountUsageService) getUsageForAccount(ctx context.Context, account *Account, forceProbe bool) (*UsageInfo, error) {
	if err := scopedAccountAccess(ctx, s.accountRepo, account, "accounts.usage.read", true); err != nil {
		return nil, err
	}
	if account == nil {
		return nil, fmt.Errorf("account is required")
	}
	accountID := account.ID

	if account.IsSyntheticUITest() && account.IsAnthropicOAuthOrSetupToken() {
		return s.getPassiveUsageForAccount(ctx, account)
	}

	if account.Platform == PlatformOpenAI && account.Type == AccountTypeOAuth {
		return s.getOpenAIUsage(ctx, account, forceProbe)
	}

	if account.Platform == PlatformGemini {
		usage, err := s.getGeminiUsage(ctx, account)
		if err == nil {
			s.tryClearRecoverableAccountError(ctx, account)
		}
		return usage, err
	}

	if account.Platform == PlatformAntigravity {
		usage, err := s.getAntigravityUsage(ctx, account, forceProbe)
		if err == nil {
			s.tryClearRecoverableAccountError(ctx, account)
		}
		return usage, err
	}

	if account.Platform == PlatformGrok {
		usage, err := s.getGrokUsage(ctx, account, forceProbe)
		if err == nil && usage != nil && usage.Error == "" {
			s.tryClearRecoverableAccountError(ctx, account)
		}
		return usage, err
	}

	if account.CanGetUsage() {
		var apiResp *ClaudeUsageResponse

		if cached, ok := s.cache.apiCache.Load(accountID); ok {
			if cache, ok := cached.(*apiUsageCache); ok {
				age := time.Since(cache.timestamp)
				if cache.err != nil && age < apiErrorCacheTTL {
					return nil, cache.err
				}
				if cache.response != nil && age < apiCacheTTL {
					apiResp = cache.response
				}
			}
		}

		if apiResp == nil {
			jitter := time.Duration(rand.Int64N(int64(apiQueryMaxJitter)))
			select {
			case <-time.After(jitter):
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			flightKey := fmt.Sprintf("usage:%d", accountID)
			result, flightErr, _ := s.cache.apiFlight.Do(flightKey, func() (any, error) {
				if cached, ok := s.cache.apiCache.Load(accountID); ok {
					if cache, ok := cached.(*apiUsageCache); ok {
						age := time.Since(cache.timestamp)
						if cache.err != nil && age < apiErrorCacheTTL {
							return nil, cache.err
						}
						if cache.response != nil && age < apiCacheTTL {
							return cache.response, nil
						}
					}
				}
				resp, fetchErr := s.fetchOAuthUsageRaw(ctx, account)
				if fetchErr != nil {
					s.cache.apiCache.Store(accountID, &apiUsageCache{
						err:       fetchErr,
						timestamp: time.Now(),
					})
					return nil, fetchErr
				}
				s.cache.apiCache.Store(accountID, &apiUsageCache{
					response:  resp,
					timestamp: time.Now(),
				})
				return resp, nil
			})
			if flightErr != nil {
				return nil, flightErr
			}
			apiResp, _ = result.(*ClaudeUsageResponse)
		}

		now := time.Now()
		usage := s.buildUsageInfo(apiResp, &now)
		s.addWindowStats(ctx, account, usage)
		s.syncActiveToPassive(ctx, account.ID, usage)

		if usage.SevenDayFable == nil {
			usage.SevenDayFable = buildPassiveUsageWindow(account.Extra, "passive_usage_7d_oi_utilization", "passive_usage_7d_oi_reset")
		}

		s.tryClearRecoverableAccountError(ctx, account)
		return usage, nil
	}

	if account.Type == AccountTypeSetupToken {
		usage := s.estimateSetupTokenUsage(account)
		s.addWindowStats(ctx, account, usage)
		return usage, nil
	}

	return nil, fmt.Errorf("account type %s does not support usage query", account.Type)
}

func (s *AccountUsageService) GetUsage(ctx context.Context, accountID int64, force ...bool) (*UsageInfo, error) {
	forceProbe := len(force) > 0 && force[0]

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get account failed: %w", err)
	}

	return s.getUsageForAccount(ctx, account, forceProbe)
}

func (s *AccountUsageService) GetUsageForAccount(ctx context.Context, account *Account, force ...bool) (*UsageInfo, error) {
	forceProbe := len(force) > 0 && force[0]
	return s.getUsageForAccount(ctx, account, forceProbe)
}

func (s *AccountUsageService) GetUsageBatch(ctx context.Context, accountIDs []int64, force bool) (map[int64]*UsageInfo, map[int64]string, error) {
	if adminaccess.RestrictedFromContext(ctx) != nil {
		if len(accountIDs) > 100 {
			return nil, nil, fmt.Errorf("账户数量超过限制")
		}
		for _, id := range accountIDs {
			if id <= 0 {
				return nil, nil, fmt.Errorf("账户 ID 无效")
			}
		}
	}
	uniqueIDs := make([]int64, 0, len(accountIDs))
	seen := make(map[int64]struct{}, len(accountIDs))
	for _, accountID := range accountIDs {
		if accountID <= 0 {
			continue
		}
		if _, ok := seen[accountID]; ok {
			continue
		}
		seen[accountID] = struct{}{}
		uniqueIDs = append(uniqueIDs, accountID)
	}

	usageByAccount := make(map[int64]*UsageInfo, len(uniqueIDs))
	errorsByAccount := make(map[int64]string)
	if len(uniqueIDs) == 0 {
		return usageByAccount, errorsByAccount, nil
	}

	accounts, err := s.accountRepo.GetByIDs(ctx, uniqueIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("get accounts failed: %w", err)
	}

	accountsByID := make(map[int64]*Account, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		accountsByID[account.ID] = account
	}

	if adminaccess.RestrictedFromContext(ctx) != nil {
		for _, id := range uniqueIDs {
			if err := scopedAccountAccess(ctx, s.accountRepo, accountsByID[id], "accounts.usage.read", true); err != nil {
				return nil, nil, err
			}
		}
	}
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(6)

	for _, accountID := range uniqueIDs {
		id := accountID
		account := accountsByID[id]
		if account == nil {
			errorsByAccount[id] = ErrAccountNotFound.Error()
			continue
		}

		g.Go(func() error {
			var usage *UsageInfo
			var usageErr error
			if supportsAnthropicPassiveUsage(account) {
				usage, usageErr = s.getPassiveUsageForAccount(gctx, account)
			} else {
				usage, usageErr = s.getUsageForAccount(gctx, account, force)
			}

			mu.Lock()
			defer mu.Unlock()
			if usageErr != nil {
				errorsByAccount[id] = batchUsageErrorMessage(usageErr)
				return nil
			}
			usageByAccount[id] = usage
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, nil, err
	}

	return usageByAccount, errorsByAccount, nil
}

func (s *AccountUsageService) GetPassiveUsage(ctx context.Context, accountID int64) (*UsageInfo, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get account failed: %w", err)
	}

	return s.getPassiveUsageForAccount(ctx, account)
}

func (s *AccountUsageService) getPassiveUsageForAccount(ctx context.Context, account *Account) (*UsageInfo, error) {
	if err := scopedAccountAccess(ctx, s.accountRepo, account, "accounts.usage.read", false); err != nil {
		return nil, err
	}
	if !supportsAnthropicPassiveUsage(account) {
		return nil, fmt.Errorf("passive usage only supported for Anthropic OAuth/SetupToken accounts")
	}

	info := s.estimateSetupTokenUsage(account)
	info.Source = "passive"

	if raw, ok := account.Extra["passive_usage_sampled_at"]; ok {
		if str, ok := raw.(string); ok {
			if t, err := time.Parse(time.RFC3339, str); err == nil {
				info.UpdatedAt = &t
			}
		}
	}

	info.SevenDay = buildPassiveUsageWindow(account.Extra, "passive_usage_7d_utilization", "passive_usage_7d_reset")
	info.SevenDayFable = buildPassiveUsageWindow(account.Extra, "passive_usage_7d_oi_utilization", "passive_usage_7d_oi_reset")

	s.addWindowStats(ctx, account, info)
	if account.IsSyntheticUITest() {
		applySyntheticWindowStats(info, account.Extra)
	}

	return info, nil
}

func applySyntheticWindowStats(info *UsageInfo, extra map[string]any) {
	if info == nil || info.FiveHour == nil || len(extra) == 0 {
		return
	}
	raw, ok := extra["synthetic_window_stats"].(map[string]any)
	if !ok {
		return
	}
	info.FiveHour.WindowStats = &WindowStats{
		Requests:     int64(parseExtraInt(raw["requests"])),
		Tokens:       int64(parseExtraInt(raw["tokens"])),
		Cost:         parseExtraFloat64(raw["cost"]),
		StandardCost: parseExtraFloat64(raw["standard_cost"]),
		UserCost:     parseExtraFloat64(raw["user_cost"]),
	}
}

func buildPassiveUsageWindow(extra map[string]any, utilKey, resetKey string) *UsageProgress {
	util := parseExtraFloat64(extra[utilKey])
	resetRaw := parseExtraFloat64(extra[resetKey])
	if util <= 0 && resetRaw <= 0 {
		return nil
	}
	var resetAt *time.Time
	var remaining int
	if resetRaw > 0 {
		t := time.Unix(int64(resetRaw), 0)
		resetAt = &t
		remaining = int(time.Until(t).Seconds())
		if remaining < 0 {
			remaining = 0
		}
	}
	return &UsageProgress{
		Utilization:      util * 100,
		ResetsAt:         resetAt,
		RemainingSeconds: remaining,
	}
}

func (s *AccountUsageService) syncActiveToPassive(ctx context.Context, accountID int64, usage *UsageInfo) {
	extraUpdates := make(map[string]any, 4)

	if usage.FiveHour != nil {
		extraUpdates["session_window_utilization"] = usage.FiveHour.Utilization / 100
	}
	if usage.SevenDay != nil {
		extraUpdates["passive_usage_7d_utilization"] = usage.SevenDay.Utilization / 100
		if usage.SevenDay.ResetsAt != nil {
			extraUpdates["passive_usage_7d_reset"] = usage.SevenDay.ResetsAt.Unix()
		}
	}
	if usage.SevenDayFable != nil {
		extraUpdates["passive_usage_7d_oi_utilization"] = usage.SevenDayFable.Utilization / 100
		if usage.SevenDayFable.ResetsAt != nil {
			extraUpdates["passive_usage_7d_oi_reset"] = usage.SevenDayFable.ResetsAt.Unix()
		}
	}

	if len(extraUpdates) > 0 {
		extraUpdates["passive_usage_sampled_at"] = time.Now().UTC().Format(time.RFC3339)
		if err := s.accountRepo.UpdateExtra(ctx, accountID, extraUpdates); err != nil {
			slog.Warn("sync_active_to_passive_failed", "account_id", accountID, "error", err)
		}
	}

	if usage.FiveHour != nil && usage.FiveHour.ResetsAt != nil {
		if err := s.accountRepo.UpdateSessionWindowEnd(ctx, accountID, *usage.FiveHour.ResetsAt); err != nil {
			slog.Warn("sync_active_to_passive_session_window_end_failed", "account_id", accountID, "error", err)
		}
	}
}

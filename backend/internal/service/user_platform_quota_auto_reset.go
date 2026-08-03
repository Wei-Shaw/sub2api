package service

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"
)

const userPlatformQuotaAnchorKey = "user_platform_quota_anchor"

// UserPlatformQuotaAutoResetter turns an anchored seven-day reset into a
// rolling seven-day reset for every active OpenAI user quota row.
type UserPlatformQuotaAutoResetter struct {
	accountRepo    AccountRepository
	weeklyResetter UserPlatformQuotaWeeklyResetter
	cache          BillingCache

	mu            sync.Mutex
	lastTriggered map[int64]time.Time
}

func NewUserPlatformQuotaAutoResetter(
	accountRepo AccountRepository,
	weeklyResetter UserPlatformQuotaWeeklyResetter,
	cache BillingCache,
) *UserPlatformQuotaAutoResetter {
	return &UserPlatformQuotaAutoResetter{
		accountRepo:    accountRepo,
		weeklyResetter: weeklyResetter,
		cache:          cache,
		lastTriggered:  make(map[int64]time.Time),
	}
}

// ObserveSevenDayReset is intentionally asynchronous so a usage page or gateway
// request never waits for the all-user quota update.
func (s *UserPlatformQuotaAutoResetter) ObserveSevenDayReset(_ context.Context, accountID int64, previousUsed, currentUsed float64, upstreamResetAt *time.Time) {
	if s == nil || accountID <= 0 || previousUsed <= 0 || currentUsed != 0 || s.accountRepo == nil || s.weeklyResetter == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if !s.isAnchoredAccount(ctx, accountID) {
			return
		}

		now := time.Now().UTC()
		s.mu.Lock()
		if last, ok := s.lastTriggered[accountID]; ok && now.Sub(last) < time.Minute {
			s.mu.Unlock()
			return
		}
		s.lastTriggered[accountID] = now
		s.mu.Unlock()

		resetAt := now.Add(7 * 24 * time.Hour)
		if upstreamResetAt != nil {
			candidate := upstreamResetAt.UTC()
			if candidate.After(now) && candidate.Sub(now) <= 8*24*time.Hour {
				resetAt = candidate
			}
		}

		userIDs, err := s.weeklyResetter.ResetAllWeeklyWindows(ctx, PlatformOpenAI, now, resetAt)
		if err != nil {
			s.mu.Lock()
			delete(s.lastTriggered, accountID)
			s.mu.Unlock()
			slog.Error("automatic_user_platform_quota_reset_failed", "account_id", accountID, "error", err)
			return
		}

		for _, userID := range userIDs {
			if s.cache == nil {
				break
			}
			if err := s.cache.DeleteUserPlatformQuotaCache(ctx, userID, PlatformOpenAI); err != nil {
				slog.Warn("automatic_user_platform_quota_cache_invalidation_failed", "user_id", userID, "platform", PlatformOpenAI, "error", err)
			}
		}
		slog.Info("automatic_user_platform_quota_reset", "account_id", accountID, "platform", PlatformOpenAI, "users", len(userIDs), "reset_at", resetAt.Format(time.RFC3339))
	}()
}

func (s *UserPlatformQuotaAutoResetter) isAnchoredAccount(ctx context.Context, accountID int64) bool {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil || !account.IsActive() || !account.IsOpenAIOAuth() || account.IsShadow() {
		return false
	}

	accounts, err := s.accountRepo.ListByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		return false
	}
	var (
		explicitSeen bool
		explicitID   int64
		candidates   []int64
	)
	for i := range accounts {
		candidate := &accounts[i]
		if !candidate.IsActive() || !candidate.IsOpenAIOAuth() || candidate.IsShadow() {
			continue
		}
		marked, enabled := quotaAnchorMarker(candidate.Extra)
		if marked {
			explicitSeen = true
			if enabled && (explicitID == 0 || candidate.ID < explicitID) {
				explicitID = candidate.ID
			}
			continue
		}
		candidates = append(candidates, candidate.ID)
	}
	if explicitSeen {
		return explicitID != 0 && explicitID == accountID
	}
	if len(candidates) == 1 {
		return candidates[0] == accountID
	}
	// With no explicit marker and multiple accounts, keep the historical
	// single-anchor behavior deterministic by choosing the lowest account ID.
	if len(candidates) == 0 {
		return false
	}
	anchorID := candidates[0]
	for _, candidateID := range candidates[1:] {
		if candidateID < anchorID {
			anchorID = candidateID
		}
	}
	return anchorID == accountID
}

func quotaAnchorMarker(extra map[string]any) (marked, enabled bool) {
	if extra == nil {
		return false, false
	}
	raw, ok := extra[userPlatformQuotaAnchorKey]
	if !ok {
		return false, false
	}
	switch value := raw.(type) {
	case bool:
		return true, value
	case string:
		parsed, err := strconv.ParseBool(value)
		return true, err == nil && parsed
	default:
		return true, false
	}
}

func sevenDaySnapshotTransition(previous, updates map[string]any) (previousUsed, currentUsed float64, resetAt *time.Time, ok bool) {
	previousRaw, previousExists := previous["codex_7d_used_percent"]
	currentRaw, currentExists := updates["codex_7d_used_percent"]
	if !previousExists || !currentExists {
		return 0, 0, nil, false
	}
	previousUsed = parseExtraFloat64(previousRaw)
	currentUsed = parseExtraFloat64(currentRaw)
	if raw, exists := updates["codex_7d_reset_at"]; exists {
		if parsed := parseExtraTime(raw); !parsed.IsZero() {
			resetAt = &parsed
		}
	}
	return previousUsed, currentUsed, resetAt, true
}

package service

import (
	"context"
	"fmt"
	"maps"
	"time"
)

type openAIUsageReader interface {
	QueryUsage(context.Context, int64) (*OpenAIQuotaUsage, error)
}

type openAIReadUsageCache struct {
	usage *OpenAIQuotaUsage
	err   error
	at    time.Time
}

// GetActiveUsage is the UI's quota-only path. It never sends a synthetic model
// request or consumes a reset credit, including for a manual forced refresh.
func (s *AccountUsageService) GetActiveUsage(ctx context.Context, id int64, force bool) (*UsageInfo, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.getActiveUsageForAccount(ctx, account, force)
}

func (s *AccountUsageService) getActiveUsageForAccount(ctx context.Context, account *Account, force bool) (*UsageInfo, error) {
	if account == nil {
		return nil, fmt.Errorf("account is required")
	}
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return s.getUsageForAccount(ctx, account, force)
	}
	if err := scopedAccountAccess(ctx, s.accountRepo, account, "accounts.usage.read", true); err != nil {
		return nil, err
	}
	if s.openAIQuotaService == nil {
		return nil, fmt.Errorf("OpenAI quota reader unavailable")
	}

	cached := func() *openAIReadUsageCache {
		if s.cache == nil {
			return nil
		}
		value, ok := s.cache.openAIReadCache.Load(account.ID)
		if !ok {
			return nil
		}
		entry := value.(*openAIReadUsageCache)
		ttl := 5 * time.Second
		if entry.err != nil {
			ttl = apiErrorCacheTTL
		}
		if time.Since(entry.at) < ttl && (entry.err != nil || !force) {
			return entry
		}
		return nil
	}
	query := func() (any, error) {
		if entry := cached(); entry != nil {
			return entry, nil
		}
		quota, err := s.openAIQuotaService.QueryUsage(WithScopedAdminAccountOperation(ctx, "accounts.usage.read"), account.ID)
		entry := &openAIReadUsageCache{usage: quota, err: err, at: time.Now()}
		// Cancellation belongs to this request, not to subsequent page polls.
		if ctx.Err() == nil && s.cache != nil {
			s.cache.openAIReadCache.Store(account.ID, entry)
		}
		return entry, nil
	}
	var value any
	if s.cache != nil {
		var err error
		value, err, _ = s.cache.apiFlight.Do(fmt.Sprintf("openai-read:%d", account.ID), query)
		if err != nil {
			return nil, err
		}
	} else {
		value, _ = query()
	}
	entry := value.(*openAIReadUsageCache)
	if entry.err != nil {
		return nil, entry.err
	}
	if entry.usage == nil {
		return nil, fmt.Errorf("empty OpenAI quota response")
	}
	updates := buildOpenAIAutoResetUsageUpdates(entry.usage, entry.at)
	if account.IsShadow() {
		updates = buildCodexSparkWindowExtraUpdates(entry.usage, entry.at)
	}
	// Each caller owns its projection, even when several tabs share one read.
	copy := *account
	copy.Extra = maps.Clone(account.Extra)
	mergeAccountExtra(&copy, updates)
	usage, err := s.getOpenAIUsage(ctx, &copy, false, true)
	if usage != nil {
		usage.Source = "active"
		usage.UpdatedAt = &entry.at
	}
	return usage, err
}

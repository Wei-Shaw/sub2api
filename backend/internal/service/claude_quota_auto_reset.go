package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const claudeAutoResetPrefix = "claude_auto_reset_credit_"

type claudeResetScope struct {
	groupID *int64
	model   string
	gateway *GatewayService
	expires time.Time
}

var claudeAutoResetRegistry struct {
	sync.RWMutex
	worker *claudeQuotaAutoReset
}

type claudeQuotaAutoReset struct {
	service  *ClaudeResetCreditService
	accounts AccountRepository
	fetcher  ClaudeUsageFetcher
	scopes   sync.Map
	cancel   context.CancelFunc
	done     chan struct{}
}

func resolveClaudeAutoResetConfig(a *Account) OpenAIAutoResetCreditConfig {
	if a == nil || a.Platform != PlatformAnthropic || a.Type != AccountTypeOAuth || a.IsShadow() {
		return OpenAIAutoResetCreditConfig{}
	}
	copied := *a
	copied.Platform = PlatformOpenAI
	copied.Extra = map[string]any{}
	for k, v := range a.Extra {
		if strings.HasPrefix(k, claudeAutoResetPrefix) {
			copied.Extra[strings.TrimPrefix(k, "claude_")] = v
		}
	}
	if _, ok := copied.Extra[OpenAIAutoResetCreditModeExtraKey]; !ok {
		copied.Extra[OpenAIAutoResetCreditModeExtraKey] = OpenAIAutoResetModeExhausted
	}
	cfg := ResolveOpenAIAutoResetCreditConfig(&copied)
	if cfg.Mode == OpenAIAutoResetModeThreshold {
		cfg.Enabled = false
	}
	return cfg
}

func (s *ClaudeResetCreditService) startAutomatic(accounts AccountRepository, fetcher ClaudeUsageFetcher) {
	ctx, cancel := context.WithCancel(context.Background())
	w := &claudeQuotaAutoReset{service: s, accounts: accounts, fetcher: fetcher, cancel: cancel, done: make(chan struct{})}
	s.automatic = w
	claudeAutoResetRegistry.Lock()
	claudeAutoResetRegistry.worker = w
	claudeAutoResetRegistry.Unlock()
	go func() {
		defer close(w.done)
		timer := time.NewTicker(time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				w.scan(ctx)
			}
		}
	}()
}
func (s *ClaudeResetCreditService) Stop() {
	if s.automatic != nil {
		s.automatic.cancel()
		<-s.automatic.done
		claudeAutoResetRegistry.Lock()
		if claudeAutoResetRegistry.worker == s.automatic {
			claudeAutoResetRegistry.worker = nil
		}
		claudeAutoResetRegistry.Unlock()
	}
}
func notifyClaudeResetScope(gateway *GatewayService, groupID *int64, model string) {
	if model == "" {
		return
	}
	claudeAutoResetRegistry.RLock()
	w := claudeAutoResetRegistry.worker
	claudeAutoResetRegistry.RUnlock()
	if w == nil {
		return
	}
	var groupCopy *int64
	if groupID != nil {
		v := *groupID
		groupCopy = &v
	}
	key := fmt.Sprintf("%v:%s", derefGroupID(groupCopy), model)
	count := 0
	w.scopes.Range(func(k, v any) bool {
		if !v.(claudeResetScope).expires.After(time.Now()) {
			w.scopes.Delete(k)
		} else {
			count++
		}
		return true
	})
	if count >= 1024 {
		return
	}
	w.scopes.Store(key, claudeResetScope{groupID: groupCopy, model: model, gateway: gateway, expires: time.Now().Add(2 * time.Minute)})
}
func (w *claudeQuotaAutoReset) scan(ctx context.Context) {
	w.scopes.Range(func(k, v any) bool {
		if !v.(claudeResetScope).expires.After(time.Now()) {
			w.scopes.Delete(k)
		}
		return true
	})
	if w.service.locks == nil {
		return
	}
	owner := uuid.NewString()
	key := "jobs:anthropic-auto-reset-credit:provider-policy"
	ok, err := w.service.locks.TryAcquireLeaderLock(ctx, key, owner, 120*time.Second)
	if err != nil || !ok {
		return
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = w.service.locks.ReleaseLeaderLock(c, key, owner)
	}()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	accounts, err := w.accounts.ListByPlatform(ctx, PlatformAnthropic)
	if err != nil {
		return
	}
	for i := range accounts {
		a := &accounts[i]
		cfg := resolveClaudeAutoResetConfig(a)
		if !cfg.Enabled || !a.IsActive() || !a.Schedulable {
			continue
		}
		if w.evaluate(ctx, a, cfg, accounts) {
			return
		}
	}
	w.scopes.Range(func(k, v any) bool {
		if !v.(claudeResetScope).expires.After(time.Now()) {
			w.scopes.Delete(k)
		}
		return true
	})
}
func (w *claudeQuotaAutoReset) usage(ctx context.Context, a *Account) (*ClaudeUsageResponse, error) {
	_, token, proxy, err := w.service.account(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	return w.fetcher.FetchUsage(ctx, token, proxy)
}
func claudeUsageExhausted(u *ClaudeUsageResponse, model string, now time.Time) bool {
	if u == nil {
		return false
	}
	windows := []ClaudeUsageWindow{{Utilization: u.FiveHour.Utilization, ResetsAt: u.FiveHour.ResetsAt}, {Utilization: u.SevenDay.Utilization, ResetsAt: u.SevenDay.ResetsAt}}
	if strings.Contains(strings.ToLower(model), "sonnet") {
		windows = append(windows, ClaudeUsageWindow{Utilization: u.SevenDaySonnet.Utilization, ResetsAt: u.SevenDaySonnet.ResetsAt})
	}
	for _, v := range windows {
		reset, err := time.Parse(time.RFC3339, v.ResetsAt)
		if err == nil && reset.After(now) && v.Utilization == 100 {
			return true
		}
	}
	return false
}
func (w *claudeQuotaAutoReset) cohort(ctx context.Context, target *Account, scope claudeResetScope, accounts []Account) bool {
	if !scope.expires.After(time.Now()) || scope.gateway == nil {
		return false
	}
	privacy := false
	if scope.groupID != nil {
		if scope.gateway.groupRepo == nil {
			return false
		}
		g, err := scope.gateway.groupRepo.GetByIDLite(ctx, *scope.groupID)
		if err != nil || g == nil || g.Status != StatusActive || g.Platform != PlatformAnthropic || g.ProfitControlEnabled {
			return false
		}
		privacy = g.RequirePrivacySet
	}
	eligible := func(a *Account) bool {
		return a.Platform == PlatformAnthropic && a.IsActive() && a.Schedulable && openAIStickyAccountMatchesGroup(a, scope.groupID) && scope.gateway.isModelSupportedByAccount(a, scope.model) && (!privacy || a.IsPrivacySet()) && !(scope.groupID != nil && scope.gateway.needsUpstreamChannelRestrictionCheck(ctx, scope.groupID) && scope.gateway.isUpstreamModelRestrictedByChannel(ctx, *scope.groupID, a, scope.model))
	}
	if !eligible(target) {
		return false
	}
	observations := []*ClaudeUsageResponse{}
	for i := range accounts {
		a := &accounts[i]
		if !eligible(a) {
			continue
		}
		if a.Type != AccountTypeOAuth || a.IsShadow() {
			return false
		}
		u, err := w.usage(ctx, a)
		if err != nil || !claudeUsageExhausted(u, scope.model, time.Now()) {
			return false
		}
		observations = append(observations, u)
	}
	if len(observations) == 0 {
		return false
	}
	for _, u := range observations {
		if !claudeUsageExhausted(u, scope.model, time.Now()) {
			return false
		}
	}
	return true
}
func (w *claudeQuotaAutoReset) evaluate(ctx context.Context, a *Account, cfg OpenAIAutoResetCreditConfig, accounts []Account) bool {
	credits, err := w.service.Query(ctx, a.ID)
	if err != nil || credits == nil || !credits.Eligible || credits.FetchedAt.IsZero() || time.Since(credits.FetchedAt) > time.Minute {
		return false
	}
	var chosenScope *claudeResetScope
	exhausted := false
	if cfg.Mode != OpenAIAutoResetModeExpiring {
		w.scopes.Range(func(_, v any) bool {
			if w.cohort(ctx, a, v.(claudeResetScope), accounts) {
				exhausted = true
				picked := v.(claudeResetScope)
				chosenScope = &picked
				return false
			}
			return true
		})
	}
	now := time.Now()
	for _, credit := range credits.Credits {
		if !credit.Redeemable || credit.ResetsLeft <= 0 || credit.SelectionToken == "" {
			continue
		}
		expiring := false
		if cfg.Mode != OpenAIAutoResetModeExhausted && credit.ExpiresAt != nil && credit.ExpiresAt.After(now) && !credit.ExpiresAt.After(now.Add(time.Duration(cfg.ExpiryHorizonSeconds)*time.Second)) {
			for _, dimension := range credit.Clears {
				used, ok := credit.PercentUsed[dimension]
				if ok && used > 0 && used <= 100 && used/100 >= cfg.ExpiryMinUtilization {
					expiring = true
				}
			}
		}
		usefulExhausted := false
		if exhausted {
			for _, dimension := range credit.Clears {
				if credit.PercentUsed[dimension] == 100 && chosenScope != nil && claudeResetDimensionMatchesModel(dimension, chosenScope.model) {
					usefulExhausted = true
				}
			}
		}
		if !usefulExhausted && !expiring {
			continue
		}
		current, err := w.accounts.GetByID(ctx, a.ID)
		if err != nil || current == nil || !current.IsActive() || !current.Schedulable {
			return false
		}
		latest := resolveClaudeAutoResetConfig(current)
		if latest != cfg {
			return false
		}
		if time.Since(credits.FetchedAt) > 30*time.Second || (credit.ExpiresAt != nil && !credit.ExpiresAt.After(time.Now())) {
			return false
		}

		// Re-read membership and native quota after account/config validation.
		// An added or newly usable peer invalidates this exhaustion decision.
		if usefulExhausted && !expiring {
			if chosenScope == nil {
				return false
			}
			freshAccounts, err := w.accounts.ListByPlatform(ctx, PlatformAnthropic)
			if err != nil || !w.cohort(ctx, current, *chosenScope, freshAccounts) {
				return false
			}
		}
		freshCredits, err := w.service.Query(ctx, a.ID)
		if err != nil || freshCredits == nil {
			return false
		}
		valid := false
		for _, freshCredit := range freshCredits.Credits {
			if freshCredit.SelectionToken != credit.SelectionToken || !freshCredit.Redeemable {
				continue
			}
			if expiring {
				valid = claudeExpiringUseful(freshCredit, cfg, time.Now())
			}
			if usefulExhausted && !expiring {
				for _, dimension := range freshCredit.Clears {
					if freshCredit.PercentUsed[dimension] == 100 && chosenScope != nil && claudeResetDimensionMatchesModel(dimension, chosenScope.model) {
						valid = true
					}
				}
			}
		}
		if !valid {
			return false
		}
		if ctx.Err() != nil {
			return false
		}
		// Native selection binds grant+remaining count+validity; Redeem separately
		// fences the native organization. Retries keep the same operation key.
		key := "claude-auto:" + shortOpenAIAutoResetHash(credit.SelectionToken)
		_, _ = w.service.Redeem(ctx, a.ID, credit.SelectionToken, key)
		return true // Any attempt ends the scan, including ambiguous outcomes.
	}
	return false
}

func claudeExpiringUseful(credit ClaudeResetCredit, cfg OpenAIAutoResetCreditConfig, now time.Time) bool {
	if !credit.Redeemable || credit.ExpiresAt == nil || !credit.ExpiresAt.After(now) || credit.ExpiresAt.After(now.Add(time.Duration(cfg.ExpiryHorizonSeconds)*time.Second)) {
		return false
	}
	for _, dimension := range credit.Clears {
		used, ok := credit.PercentUsed[dimension]
		if ok && used > 0 && used <= 100 && used/100 >= cfg.ExpiryMinUtilization {
			return true
		}
	}
	return false
}

func claudeResetDimensionMatchesModel(dimension, model string) bool {
	switch dimension {
	case "five_hour", "seven_day":
		return true
	case "seven_day_sonnet":
		return strings.Contains(strings.ToLower(model), "sonnet")
	default:
		return false
	}
}

package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	upstreamBalanceProbeLeaderLockKey = "upstream:balance:probe:leader"
	upstreamBalanceProbeMaxPerCycle   = 20
	upstreamBalanceProbeConcurrency   = 4
)

// RunDueBalances periodically probes Sub2API/NewAPI wallet balances for accounts
// that have credentials.balance_probe_source configured.
func (s *UpstreamBillingProbeService) RunDueBalances(ctx context.Context) error {
	if s == nil || s.accountRepo == nil {
		return nil
	}

	runRelease, acquired, lockErr := s.tryAcquireLeaderLock(ctx, upstreamBalanceProbeLeaderLockKey)
	if lockErr != nil {
		return fmt.Errorf("acquire upstream balance probe leader lock: %w", lockErr)
	}
	if !acquired {
		return nil
	}
	defer runRelease()

	now := s.currentTime()
	intervalMinutes := upstreamBillingProbeDefaultIntervalMinutes
	if settings, err := s.getSettings(ctx); err == nil && settings != nil && settings.IntervalMinutes > 0 {
		intervalMinutes = settings.IntervalMinutes
	}
	interval := time.Duration(intervalMinutes) * time.Minute

	accounts, err := s.accountRepo.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("list active accounts for balance probe: %w", err)
	}

	due := make([]Account, 0)
	for i := range accounts {
		account := accounts[i]
		if !accountNeedsUpstreamBalanceProbe(&account) {
			continue
		}
		if !upstreamBalanceProbeDue(&account, now, interval) {
			continue
		}
		due = append(due, account)
	}

	sort.SliceStable(due, func(i, j int) bool {
		left := upstreamBalanceLastUpdatedAt(&due[i])
		right := upstreamBalanceLastUpdatedAt(&due[j])
		leftZero := left.IsZero()
		rightZero := right.IsZero()
		if leftZero && rightZero {
			return due[i].ID < due[j].ID
		}
		if leftZero {
			return true
		}
		if rightZero {
			return false
		}
		return left.Before(right)
	})
	if len(due) > upstreamBalanceProbeMaxPerCycle {
		due = due[:upstreamBalanceProbeMaxPerCycle]
	}
	if len(due) == 0 {
		return nil
	}

	sem := make(chan struct{}, upstreamBalanceProbeConcurrency)
	var group errgroup.Group
	for i := range due {
		accountID := due[i].ID
		group.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			if _, probeErr := s.ProbeUpstreamBalance(ctx, accountID); probeErr != nil {
				slog.Warn("upstream_balance_probe_due_failed", "account_id", accountID, "error", probeErr)
			}
			return nil
		})
	}
	return group.Wait()
}

func accountNeedsUpstreamBalanceProbe(account *Account) bool {
	if account == nil || !account.IsActive() {
		return false
	}
	if account.Type != AccountTypeAPIKey {
		return false
	}
	return AccountBalanceProbeSource(account) != ""
}

func upstreamBalanceLastUpdatedAt(account *Account) time.Time {
	if account == nil || account.Extra == nil {
		return time.Time{}
	}
	raw, _ := account.Extra[upstreamBalanceExtraUpdated].(string)
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func upstreamBalanceProbeDue(account *Account, now time.Time, interval time.Duration) bool {
	last := upstreamBalanceLastUpdatedAt(account)
	if last.IsZero() {
		return true
	}
	if interval <= 0 {
		interval = time.Duration(upstreamBillingProbeDefaultIntervalMinutes) * time.Minute
	}
	return !now.Before(last.Add(interval))
}

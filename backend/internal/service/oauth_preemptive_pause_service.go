package service

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

type oauthPreemptivePauseUsageReader interface {
	GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*usagestats.AccountStats, error)
}

func evaluateOAuthPreemptivePause(ctx context.Context, account *Account, usageRepo oauthPreemptivePauseUsageReader, now time.Time, costResolver func(window string, startTime time.Time) (float64, bool)) (time.Time, bool) {
	if account == nil || !account.SupportsOAuthOfficialWindowPause() {
		return time.Time{}, false
	}

	var bestReset time.Time
	for _, window := range []string{"5h", "7d"} {
		utilization, startTime, resetAt, ok := accountOAuthOfficialWindowState(account, window, now)
		if !ok {
			continue
		}

		triggered := false
		switch window {
		case "5h":
			if limit := account.GetEffectiveOAuth5hPausePercent(); limit > 0 && utilization >= limit {
				triggered = true
			}
			if limit := account.GetEffectiveOAuth5hPauseAmount(); limit > 0 {
				cost, exists := resolveOAuthPauseCost(ctx, account, usageRepo, window, startTime, costResolver)
				if exists && cost >= limit {
					triggered = true
				}
			}
		case "7d":
			if limit := account.GetEffectiveOAuth7dPausePercent(); limit > 0 && utilization >= limit {
				triggered = true
			}
			if limit := account.GetEffectiveOAuth7dPauseAmount(); limit > 0 {
				cost, exists := resolveOAuthPauseCost(ctx, account, usageRepo, window, startTime, costResolver)
				if exists && cost >= limit {
					triggered = true
				}
			}
		}

		if triggered && resetAt.After(bestReset) {
			bestReset = resetAt
		}
	}

	if bestReset.IsZero() {
		return time.Time{}, false
	}
	return bestReset, true
}

func resolveOAuthPauseCost(ctx context.Context, account *Account, usageRepo oauthPreemptivePauseUsageReader, window string, startTime time.Time, costResolver func(window string, startTime time.Time) (float64, bool)) (float64, bool) {
	if costResolver != nil {
		if cost, ok := costResolver(window, startTime); ok {
			return cost, true
		}
	}
	if usageRepo == nil {
		return 0, false
	}
	stats, err := usageRepo.GetAccountWindowStats(ctx, account.ID, startTime)
	if err != nil || stats == nil {
		return 0, false
	}
	return stats.StandardCost, true
}

type OAuthPreemptivePauseService struct {
	accountRepo AccountRepository
	usageRepo   oauthPreemptivePauseUsageReader
	interval    time.Duration
	stopCh      chan struct{}
	stopOnce    sync.Once
	wg          sync.WaitGroup
}

func NewOAuthPreemptivePauseService(accountRepo AccountRepository, usageRepo oauthPreemptivePauseUsageReader, interval time.Duration) *OAuthPreemptivePauseService {
	return &OAuthPreemptivePauseService{
		accountRepo: accountRepo,
		usageRepo:   usageRepo,
		interval:    interval,
		stopCh:      make(chan struct{}),
	}
}

func (s *OAuthPreemptivePauseService) Start() {
	if s == nil || s.accountRepo == nil || s.usageRepo == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.runOnce()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *OAuthPreemptivePauseService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

func (s *OAuthPreemptivePauseService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	for _, platform := range []string{PlatformOpenAI, PlatformAnthropic} {
		accounts, err := s.accountRepo.ListByPlatform(ctx, platform)
		if err != nil {
			log.Printf("[OAuthPreemptivePause] list accounts for %s failed: %v", platform, err)
			continue
		}

		now := time.Now()
		for i := range accounts {
			account := &accounts[i]
			if resetAt, ok := evaluateOAuthPreemptivePause(ctx, account, s.usageRepo, now, nil); ok {
				if account.RateLimitResetAt == nil || account.RateLimitResetAt.Before(resetAt) {
					if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetAt); err != nil {
						log.Printf("[OAuthPreemptivePause] set rate limit failed for account %d: %v", account.ID, err)
					}
				}
			}
		}
	}
}

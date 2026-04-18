package service

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/safe"
)

// ClaudeSidecarProbeService periodically calls /api/oauth/usage for a randomly
// chosen active Claude OAuth account. Without this, a gateway that only ever
// forwards /v1/messages produces an endpoint mix that is trivially
// distinguishable from a real Claude Code CLI (which drives the statusline
// from /api/oauth/usage). Anthropic's subscription abuse heuristics flag that
// shape and silently throttle the weekly limit to a fraction of its nominal
// value. This service restores non-zero /api/oauth/usage traffic per account.
//
// Design notes:
//   - One probe per tick, picked at random from eligible accounts. Fanning out
//     to every account on each tick would produce a synchronous burst that is
//     itself a distinctive signal.
//   - Interval is jittered between min/max to avoid a steady periodic cadence.
//   - We deliberately piggyback on AccountUsageService.GetUsage instead of
//     calling the raw fetcher, so TLS profile, proxy, fingerprint, and cache
//     behavior all match what the admin UI path does.
//   - Probe failures are non-fatal: this is cosmetic sidecar traffic, the
//     user's real requests never depend on a successful probe.
type ClaudeSidecarProbeService struct {
	accountRepo  AccountRepository
	usageService *AccountUsageService

	minInterval time.Duration
	maxInterval time.Duration
	dryRun      bool

	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewClaudeSidecarProbeService constructs a probe service. A minInterval of
// zero or a maxInterval smaller than minInterval disables the loop entirely
// (Start becomes a no-op).
func NewClaudeSidecarProbeService(
	accountRepo AccountRepository,
	usageService *AccountUsageService,
	minInterval, maxInterval time.Duration,
	dryRun bool,
) *ClaudeSidecarProbeService {
	return &ClaudeSidecarProbeService{
		accountRepo:  accountRepo,
		usageService: usageService,
		minInterval:  minInterval,
		maxInterval:  maxInterval,
		dryRun:       dryRun,
		stopCh:       make(chan struct{}),
	}
}

// Start launches the probe loop in a goroutine. Safe to call on a nil receiver
// or with missing dependencies: both become no-ops so wire/test code doesn't
// have to guard.
func (s *ClaudeSidecarProbeService) Start() {
	if s == nil || s.accountRepo == nil || s.usageService == nil {
		return
	}
	if s.minInterval <= 0 || s.maxInterval < s.minInterval {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		safe.Run("sidecar_probe.loop", nil, s.loop)
	}()
}

// Stop signals the loop to exit and waits for it.
func (s *ClaudeSidecarProbeService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

func (s *ClaudeSidecarProbeService) loop() {
	// Initial delay so that a process restart doesn't fire a probe at t=0
	// (which would cluster with every other restart-time operation and be
	// visible to the upstream as a correlated spike).
	select {
	case <-time.After(s.jitterInterval()):
	case <-s.stopCh:
		return
	}

	for {
		s.runOnce(context.Background())
		select {
		case <-time.After(s.jitterInterval()):
		case <-s.stopCh:
			return
		}
	}
}

func (s *ClaudeSidecarProbeService) jitterInterval() time.Duration {
	if s.maxInterval <= s.minInterval {
		return s.minInterval
	}
	span := int64(s.maxInterval - s.minInterval)
	return s.minInterval + time.Duration(rand.Int64N(span))
}

func (s *ClaudeSidecarProbeService) runOnce(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, sidecarProbeRequestTimeout)
	defer cancel()

	accounts, err := s.accountRepo.ListByPlatform(ctx, PlatformAnthropic)
	if err != nil {
		slog.Warn("sidecar_probe.list_accounts_failed", "error", err)
		return
	}

	eligible := filterSidecarProbeTargets(accounts)
	if len(eligible) == 0 {
		return
	}

	target := eligible[rand.IntN(len(eligible))]

	if s.dryRun {
		slog.Info("sidecar_probe.dry_run", "account_id", target.ID, "account_name", target.Name)
		return
	}

	if _, err := s.usageService.GetUsage(ctx, target.ID); err != nil {
		slog.Warn("sidecar_probe.probe_failed", "account_id", target.ID, "error", err)
		return
	}
}

// filterSidecarProbeTargets keeps only Claude OAuth accounts that are active,
// capable of returning usage (profile scope), and have a non-empty access
// token. Setup-token accounts are inference-only and return 401 on
// /api/oauth/usage, so including them would produce nothing but error noise.
func filterSidecarProbeTargets(accounts []Account) []Account {
	var out []Account
	for i := range accounts {
		a := accounts[i]
		if a.Status != StatusActive {
			continue
		}
		if !a.CanGetUsage() {
			continue
		}
		if a.GetCredential("access_token") == "" {
			continue
		}
		out = append(out, a)
	}
	return out
}

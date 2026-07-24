package service

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/backgroundruntime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func configureServiceStandby(t *testing.T, slot string) string {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "active-slot")
	t.Setenv("DEPLOYMENT_STANDBY", "true")
	t.Setenv("DEPLOYMENT_SLOT", slot)
	t.Setenv("DEPLOYMENT_STATE_FILE", statePath)
	if err := backgroundruntime.ConfigureFromEnv(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("DEPLOYMENT_STANDBY", "false")
		_ = backgroundruntime.ConfigureFromEnv()
	})
	return statePath
}

type activationCleanupCache struct {
	ConcurrencyCache
	mu           sync.Mutex
	members      []string
	staleCalls   int
	expiredCalls atomic.Int64
}

func (c *activationCleanupCache) CleanupStaleProcessSlots(_ context.Context, activePrefix string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.staleCalls++
	kept := c.members[:0]
	for _, member := range c.members {
		if strings.HasPrefix(member, activePrefix) {
			kept = append(kept, member)
		}
	}
	c.members = kept
	return nil
}

func (c *activationCleanupCache) CleanupExpiredAccountSlotKeys(context.Context) error {
	c.expiredCalls.Add(1)
	return nil
}

func (c *activationCleanupCache) snapshot() ([]string, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.members...), c.staleCalls
}

func TestProvideConcurrencyServiceStandbyPreservesActiveProcessSlotsUntilPromotion(t *testing.T) {
	statePath := configureServiceStandby(t, "green")
	cache := &activationCleanupCache{
		members: []string{"old-process-request", RequestIDPrefix() + "-self"},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.SlotCleanupInterval = time.Hour

	svc := ProvideConcurrencyService(cache, nil, cfg)
	t.Cleanup(svc.StopSlotCleanupWorker)
	members, staleCalls := cache.snapshot()
	if staleCalls != 0 || cache.expiredCalls.Load() != 0 {
		t.Fatalf("standby ran concurrency maintenance: stale=%d expired=%d", staleCalls, cache.expiredCalls.Load())
	}
	if len(members) != 2 {
		t.Fatalf("standby removed active process slots: %v", members)
	}

	if err := os.WriteFile(statePath, []byte("green\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := backgroundruntime.Activate(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for cache.expiredCalls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	members, staleCalls = cache.snapshot()
	if staleCalls != 1 {
		t.Fatalf("promotion stale cleanup calls = %d, want 1", staleCalls)
	}
	if cache.expiredCalls.Load() == 0 {
		t.Fatal("promotion did not start periodic concurrency cleanup")
	}
	if len(members) != 1 || members[0] != RequestIDPrefix()+"-self" {
		t.Fatalf("promotion retained unexpected slots: %v", members)
	}
}

type standbyQuotaCache struct {
	BillingCache
	popCalls atomic.Int64
}

func (c *standbyQuotaCache) PopDirtyUserPlatformQuotaKeys(context.Context, int) ([]UserPlatformQuotaKey, error) {
	c.popCalls.Add(1)
	return nil, nil
}

type standbyQuotaRepository struct {
	UserPlatformQuotaRepository
}

func newStandbyQuotaFlusher(t *testing.T, slot string) (string, *UserPlatformQuotaUsageFlusher, *standbyQuotaCache) {
	t.Helper()
	statePath := configureServiceStandby(t, slot)
	timingWheel, err := NewTimingWheelService()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(timingWheel.Stop)
	cfg := &config.Config{}
	cfg.Database.UserPlatformQuotaFlusherEnabled = true
	cfg.Database.UserPlatformQuotaFlushIntervalMs = int(time.Hour / time.Millisecond)
	cache := &standbyQuotaCache{}
	return statePath, ProvideUserPlatformQuotaUsageFlusher(cfg, cache, &standbyQuotaRepository{}, timingWheel), cache
}

func TestProvideQuotaFlusherStandbyStopDoesNotFlushOrRestart(t *testing.T) {
	statePath, svc, cache := newStandbyQuotaFlusher(t, "blue")
	if svc.started.Load() {
		t.Fatal("standby quota flusher started before promotion")
	}
	svc.Stop()
	if cache.popCalls.Load() != 0 {
		t.Fatal("stopping an unpromoted standby flushed the shared dirty set")
	}
	if err := os.WriteFile(statePath, []byte("blue\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := backgroundruntime.Activate(); err != nil {
		t.Fatal(err)
	}
	if svc.started.Load() {
		t.Fatal("a stopped standby restarted after a late promotion signal")
	}
}

func TestProvideQuotaFlusherStartsOnPromotion(t *testing.T) {
	statePath, svc, cache := newStandbyQuotaFlusher(t, "blue")
	if svc.started.Load() {
		t.Fatal("standby quota flusher started before promotion")
	}
	if err := os.WriteFile(statePath, []byte("blue\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := backgroundruntime.Activate(); err != nil {
		t.Fatal(err)
	}
	if !svc.started.Load() {
		t.Fatal("quota flusher did not start on promotion")
	}
	svc.Stop()
	if cache.popCalls.Load() != 1 {
		t.Fatalf("active quota flusher final flush calls = %d, want 1", cache.popCalls.Load())
	}
}

func TestStandbyRejectsDashboardAndUsageCleanupLaunches(t *testing.T) {
	configureServiceStandby(t, "admin-jobs")
	now := time.Now().UTC()

	dashboard := NewDashboardAggregationService(
		&lifecycleDashboardRepository{},
		nil,
		&config.Config{DashboardAgg: config.DashboardAggregationConfig{Enabled: true, BackfillEnabled: true}},
	)
	for name, run := range map[string]func() error{
		"backfill":  func() error { return dashboard.TriggerBackfill(now.Add(-time.Hour), now) },
		"recompute": func() error { return dashboard.TriggerRecomputeRange(now.Add(-time.Hour), now) },
	} {
		t.Run(name, func(t *testing.T) {
			err := run()
			if !errors.Is(err, ErrDeploymentStandby) {
				t.Fatalf("error=%v, want ErrDeploymentStandby", err)
			}
			if infraerrors.Code(err) != http.StatusServiceUnavailable || infraerrors.Reason(err) != "DEPLOYMENT_STANDBY" {
				t.Fatalf("code=%d reason=%q", infraerrors.Code(err), infraerrors.Reason(err))
			}
		})
	}

	usage := NewUsageCleanupService(
		&lifecycleUsageCleanupRepository{},
		nil,
		nil,
		&config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}},
	)
	_, err := usage.CreateTask(context.Background(), UsageCleanupFilters{
		StartTime: now.Add(-time.Hour),
		EndTime:   now,
	}, 1)
	if !errors.Is(err, ErrDeploymentStandby) {
		t.Fatalf("usage cleanup error=%v, want ErrDeploymentStandby", err)
	}
}

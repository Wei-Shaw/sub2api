//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func platformWindowStart(t time.Time) *time.Time { return &t }

func TestEvaluateAPIKeyPlatformLimits(t *testing.T) {
	now := time.Now()
	fresh := platformWindowStart(now.Add(-time.Minute))
	stale5h := platformWindowStart(now.Add(-6 * time.Hour))

	tests := []struct {
		name  string
		limit APIKeyPlatformLimit
		usage *APIKeyPlatformUsageRecord
		want  error
	}{
		{
			name:  "no limit configured passes",
			limit: APIKeyPlatformLimit{},
			usage: &APIKeyPlatformUsageRecord{QuotaUsed: 999},
			want:  nil,
		},
		{
			name:  "no usage row passes",
			limit: APIKeyPlatformLimit{Quota: 1},
			usage: nil,
			want:  nil,
		},
		{
			name:  "quota exhausted",
			limit: APIKeyPlatformLimit{Quota: 10},
			usage: &APIKeyPlatformUsageRecord{QuotaUsed: 10},
			want:  ErrAPIKeyPlatformQuotaExhausted,
		},
		{
			name:  "quota below limit passes",
			limit: APIKeyPlatformLimit{Quota: 10},
			usage: &APIKeyPlatformUsageRecord{QuotaUsed: 9.99},
			want:  nil,
		},
		{
			name:  "5h window exceeded",
			limit: APIKeyPlatformLimit{RateLimit5h: 5},
			usage: &APIKeyPlatformUsageRecord{Usage5h: 5, Window5hStart: fresh},
			want:  ErrAPIKeyPlatformRate5hExceeded,
		},
		{
			name:  "expired 5h window resets usage",
			limit: APIKeyPlatformLimit{RateLimit5h: 5},
			usage: &APIKeyPlatformUsageRecord{Usage5h: 50, Window5hStart: stale5h},
			want:  nil,
		},
		{
			name:  "nil window start is treated as expired",
			limit: APIKeyPlatformLimit{RateLimit1d: 1},
			usage: &APIKeyPlatformUsageRecord{Usage1d: 100},
			want:  nil,
		},
		{
			name:  "7d window exceeded",
			limit: APIKeyPlatformLimit{RateLimit7d: 20},
			usage: &APIKeyPlatformUsageRecord{Usage7d: 25, Window7dStart: fresh},
			want:  ErrAPIKeyPlatformRate7dExceeded,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EvaluateAPIKeyPlatformLimits(tt.limit, tt.usage); !errors.Is(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeAPIKeyPlatformLimits(t *testing.T) {
	t.Run("drops empty entries", func(t *testing.T) {
		out, err := NormalizeAPIKeyPlatformLimits(APIKeyPlatformLimits{
			PlatformOpenAI:      {Quota: 5},
			PlatformAntigravity: {},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 1 {
			t.Fatalf("expected only configured platforms, got %#v", out)
		}
		if out[PlatformOpenAI].Quota != 5 {
			t.Fatalf("openai quota lost: %#v", out)
		}
	})

	t.Run("all-empty map normalizes to nil", func(t *testing.T) {
		out, err := NormalizeAPIKeyPlatformLimits(APIKeyPlatformLimits{PlatformOpenAI: {}})
		if err != nil || out != nil {
			t.Fatalf("got %#v, %v", out, err)
		}
	})

	t.Run("rejects unknown platform", func(t *testing.T) {
		if _, err := NormalizeAPIKeyPlatformLimits(APIKeyPlatformLimits{"not-a-platform": {Quota: 1}}); err == nil {
			t.Fatal("expected error for unknown platform")
		}
	})

	t.Run("rejects negative limit", func(t *testing.T) {
		if _, err := NormalizeAPIKeyPlatformLimits(APIKeyPlatformLimits{PlatformOpenAI: {RateLimit1d: -1}}); err == nil {
			t.Fatal("expected error for negative limit")
		}
	})
}

// fakePlatformUsageRepo 记录读写，用于验证「只对配置了子限额的来源读写」。
type fakePlatformUsageRepo struct {
	rows      map[string]*APIKeyPlatformUsageRecord
	getCalls  []string
	incrCalls []struct {
		platform string
		cost     float64
	}
	getErr error
}

func (f *fakePlatformUsageRepo) Get(_ context.Context, _ int64, platform string) (*APIKeyPlatformUsageRecord, error) {
	f.getCalls = append(f.getCalls, platform)
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.rows[platform], nil
}

func (f *fakePlatformUsageRepo) ListByAPIKey(_ context.Context, _ int64) ([]APIKeyPlatformUsageRecord, error) {
	out := make([]APIKeyPlatformUsageRecord, 0, len(f.rows))
	for _, row := range f.rows {
		out = append(out, *row)
	}
	return out, nil
}

func (f *fakePlatformUsageRepo) IncrementUsage(_ context.Context, _ int64, platform string, cost float64, _ time.Time) error {
	f.incrCalls = append(f.incrCalls, struct {
		platform string
		cost     float64
	}{platform, cost})
	return nil
}

func (f *fakePlatformUsageRepo) ResetUsage(context.Context, int64, []string) error { return nil }

func newPlatformLimitBillingService(repo APIKeyPlatformUsageRepository) *BillingCacheService {
	svc := &BillingCacheService{cfg: &config.Config{}}
	svc.SetAPIKeyPlatformUsageRepo(repo)
	return svc
}

func TestCheckAPIKeyPlatformLimits(t *testing.T) {
	fresh := platformWindowStart(time.Now().Add(-time.Minute))

	t.Run("only the routed platform is enforced", func(t *testing.T) {
		repo := &fakePlatformUsageRepo{rows: map[string]*APIKeyPlatformUsageRecord{
			PlatformOpenAI: {Usage1d: 10, Window1dStart: fresh},
		}}
		svc := newPlatformLimitBillingService(repo)
		key := &APIKey{ID: 7, PlatformLimits: APIKeyPlatformLimits{
			PlatformOpenAI: {RateLimit1d: 10},
		}}

		if err := svc.checkAPIKeyPlatformLimits(context.Background(), key, PlatformOpenAI); !errors.Is(err, ErrAPIKeyPlatformRate1dExceeded) {
			t.Fatalf("openai should be blocked, got %v", err)
		}
		// 同一个 key 路由到另一个来源时不受影响，且完全不查用量。
		before := len(repo.getCalls)
		if err := svc.checkAPIKeyPlatformLimits(context.Background(), key, PlatformAntigravity); err != nil {
			t.Fatalf("antigravity should pass, got %v", err)
		}
		if len(repo.getCalls) != before {
			t.Fatalf("unconfigured platform must not hit the usage store: %v", repo.getCalls)
		}
	})

	t.Run("empty platform (unresolved composite target) is a no-op", func(t *testing.T) {
		repo := &fakePlatformUsageRepo{}
		svc := newPlatformLimitBillingService(repo)
		key := &APIKey{ID: 7, PlatformLimits: APIKeyPlatformLimits{PlatformOpenAI: {Quota: 1}}}
		if err := svc.checkAPIKeyPlatformLimits(context.Background(), key, ""); err != nil {
			t.Fatalf("expected no-op, got %v", err)
		}
		if len(repo.getCalls) != 0 {
			t.Fatalf("expected no usage lookup, got %v", repo.getCalls)
		}
	})

	t.Run("usage store failure does not block the request", func(t *testing.T) {
		repo := &fakePlatformUsageRepo{getErr: errors.New("db down")}
		svc := newPlatformLimitBillingService(repo)
		key := &APIKey{ID: 7, PlatformLimits: APIKeyPlatformLimits{PlatformOpenAI: {Quota: 1}}}
		if err := svc.checkAPIKeyPlatformLimits(context.Background(), key, PlatformOpenAI); err != nil {
			t.Fatalf("read failure must fail open, got %v", err)
		}
	})

	t.Run("no repo injected disables the feature", func(t *testing.T) {
		svc := &BillingCacheService{cfg: &config.Config{}}
		key := &APIKey{ID: 7, PlatformLimits: APIKeyPlatformLimits{PlatformOpenAI: {Quota: 0.0001}}}
		if err := svc.checkAPIKeyPlatformLimits(context.Background(), key, PlatformOpenAI); err != nil {
			t.Fatalf("expected feature to be off, got %v", err)
		}
	})
}

// PlatformLimits 必须随认证快照往返，否则走缓存的请求会看不到配置
// （group.model_pricing 曾因漏投影而在网关热路径静默失效）。
func TestAPIKeyAuthSnapshotRoundTripKeepsPlatformLimits(t *testing.T) {
	svc := &APIKeyService{}
	apiKey := &APIKey{
		ID:     42,
		UserID: 1,
		Name:   "k",
		Status: StatusActive,
		User:   &User{ID: 1, Status: StatusActive},
		PlatformLimits: APIKeyPlatformLimits{
			PlatformOpenAI:      {Quota: 10, RateLimit1d: 2},
			PlatformAntigravity: {RateLimit5h: 1},
		},
	}

	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	if snapshot == nil {
		t.Fatal("snapshot is nil")
	}
	restored := svc.snapshotToAPIKey("sk-test", snapshot)
	if len(restored.PlatformLimits) != 2 {
		t.Fatalf("platform limits lost in snapshot round trip: %#v", restored.PlatformLimits)
	}
	limit, ok := restored.PlatformLimit(PlatformOpenAI)
	if !ok || limit.Quota != 10 || limit.RateLimit1d != 2 {
		t.Fatalf("openai limit mismatch: %#v (ok=%v)", limit, ok)
	}
	if _, ok := restored.PlatformLimit(PlatformGemini); ok {
		t.Fatal("unconfigured platform must report no limit")
	}
}

func TestIncrementAPIKeyPlatformUsageOnlyWritesConfiguredPlatforms(t *testing.T) {
	repo := &fakePlatformUsageRepo{}
	deps := &billingDeps{billingCacheService: newPlatformLimitBillingService(repo)}
	key := &APIKey{ID: 9, PlatformLimits: APIKeyPlatformLimits{PlatformOpenAI: {Quota: 5}}}

	incrementAPIKeyPlatformUsage(context.Background(), deps, key, PlatformOpenAI, 1.5)
	incrementAPIKeyPlatformUsage(context.Background(), deps, key, PlatformAntigravity, 2)
	incrementAPIKeyPlatformUsage(context.Background(), deps, key, PlatformOpenAI, 0)
	incrementAPIKeyPlatformUsage(context.Background(), deps, key, "", 3)

	if len(repo.incrCalls) != 1 {
		t.Fatalf("expected exactly one write, got %#v", repo.incrCalls)
	}
	if repo.incrCalls[0].platform != PlatformOpenAI || repo.incrCalls[0].cost != 1.5 {
		t.Fatalf("unexpected write: %#v", repo.incrCalls[0])
	}
}

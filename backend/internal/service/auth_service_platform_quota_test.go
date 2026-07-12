//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// fakeInsertRecorder 记录 BulkInsertInitial 调用，实现 UserPlatformQuotaRepository port。
type fakeInsertRecorder struct {
	records []UserPlatformQuotaRecord
	err     error
	lastCtx context.Context // 捕获最后一次 BulkInsertInitial 收到的 ctx（用于断言事务隔离）
}

func (f *fakeInsertRecorder) GetByUserPlatform(_ context.Context, _ int64, _ string) (*UserPlatformQuotaRecord, error) {
	return nil, nil
}

func (f *fakeInsertRecorder) BulkInsertInitial(ctx context.Context, recs []UserPlatformQuotaRecord) error {
	f.lastCtx = ctx
	if f.err != nil {
		return f.err
	}
	f.records = append(f.records, recs...)
	return nil
}

func (f *fakeInsertRecorder) IncrementUsageWithReset(_ context.Context, _ int64, _ string, _ float64, _ time.Time) error {
	return nil
}

func (f *fakeInsertRecorder) ListByUser(_ context.Context, _ int64) ([]UserPlatformQuotaRecord, error) {
	return nil, nil
}

func (f *fakeInsertRecorder) UpsertForUser(_ context.Context, _ int64, _ []UserPlatformQuotaRecord) error {
	return nil
}

func (f *fakeInsertRecorder) ResetExpiredWindow(_ context.Context, _ int64, _ string, _ string, _ time.Time) error {
	return nil
}

func (f *fakeInsertRecorder) BatchSnapshotUsage(_ context.Context, _ []UserPlatformQuotaSnapshot, _ time.Time) error {
	return nil
}

func TestSnapshotPlatformQuotaDefaults_PassesToRepoBulkInsert(t *testing.T) {
	fakeRepo := &fakeInsertRecorder{}
	s := &AuthService{userPlatformQuotaRepo: fakeRepo}

	five := 5.0
	plan := &signupGrantPlan{
		PlatformQuotas: map[string]*DefaultPlatformQuotaSetting{
			"anthropic":   {DailyLimitUSD: &five},
			"openai":      {},
			"gemini":      {},
			"antigravity": {},
		},
	}
	if err := s.snapshotPlatformQuotaDefaults(context.Background(), 999, plan); err != nil {
		t.Fatal(err)
	}
	if len(fakeRepo.records) != 4 {
		t.Errorf("expected 4 records, got %d", len(fakeRepo.records))
	}
	found := false
	for _, r := range fakeRepo.records {
		if r.UserID == 999 && r.Platform == "anthropic" && r.DailyLimitUSD != nil && *r.DailyLimitUSD == 5 {
			found = true
		}
	}
	if !found {
		t.Error("anthropic daily = 5 not snapshotted")
	}
}

// TestSnapshotPlatformQuotaDefaults_DetachesCallerTransaction 锁定 fix① 不变量：
// 平台配额快照是 best-effort，必须脱离调用方事务执行——这样它失败（例如某平台
// 违反 user_platform_quotas 的 CHECK 约束）也不会把调用方的注册主事务标记为 aborted。
// 历史 bug：snapshot 在 OAuth pending handler 的 binding tx 中执行，grok 违约毒化整个
// 事务 → consumePendingOAuthBrowserSessionTx 撞 "transaction aborted" → 500 → 清 cookie → 404。
func TestSnapshotPlatformQuotaDefaults_DetachesCallerTransaction(t *testing.T) {
	fakeRepo := &fakeInsertRecorder{}
	s := &AuthService{userPlatformQuotaRepo: fakeRepo}

	five := 5.0
	plan := &signupGrantPlan{
		PlatformQuotas: map[string]*DefaultPlatformQuotaSetting{
			"anthropic": {DailyLimitUSD: &five},
		},
	}

	// 模拟调用方（OAuth pending handler）在事务 ctx 中调用快照
	txCtx := dbent.NewTxContext(context.Background(), &dbent.Tx{})

	if err := s.snapshotPlatformQuotaDefaults(txCtx, 999, plan); err != nil {
		t.Fatalf("snapshot should not error (fail-open): %v", err)
	}

	if fakeRepo.lastCtx == nil {
		t.Fatal("expected BulkInsertInitial to be called")
	}
	if dbent.TxFromContext(fakeRepo.lastCtx) != nil {
		t.Error("快照必须脱离调用方事务执行（best-effort，失败不得毒化注册事务），但 repo 收到了仍携带事务的 ctx")
	}
}

func TestSnapshotPlatformQuotaDefaults_NilPlanIsNoop(t *testing.T) {
	fakeRepo := &fakeInsertRecorder{}
	s := &AuthService{userPlatformQuotaRepo: fakeRepo}
	if err := s.snapshotPlatformQuotaDefaults(context.Background(), 1, nil); err != nil {
		t.Errorf("nil plan should be noop, got %v", err)
	}
	if len(fakeRepo.records) != 0 {
		t.Errorf("expected no records, got %d", len(fakeRepo.records))
	}
}

func TestSnapshotPlatformQuotaDefaults_RepoErrorFailsOpen(t *testing.T) {
	fakeRepo := &fakeInsertRecorder{err: fmt.Errorf("db down")}
	s := &AuthService{userPlatformQuotaRepo: fakeRepo}
	five := 5.0
	plan := &signupGrantPlan{
		PlatformQuotas: map[string]*DefaultPlatformQuotaSetting{
			"anthropic": {DailyLimitUSD: &five},
		},
	}
	if err := s.snapshotPlatformQuotaDefaults(context.Background(), 1, plan); err != nil {
		t.Errorf("fail-open: expected nil even on repo error, got %v", err)
	}
}

func TestSnapshotPlatformQuotaDefaults_NilRepoIsNoop(t *testing.T) {
	s := &AuthService{userPlatformQuotaRepo: nil}
	five := 5.0
	plan := &signupGrantPlan{
		PlatformQuotas: map[string]*DefaultPlatformQuotaSetting{"a": {DailyLimitUSD: &five}},
	}
	if err := s.snapshotPlatformQuotaDefaults(context.Background(), 1, plan); err != nil {
		t.Errorf("nil repo should be noop, got %v", err)
	}
}

// resolveSignupGrantPlan 测试：依赖完整的 AuthService 构造，需要 SettingService（含 settingRepoStub）。
// settingRepoStub 已在 auth_service_register_test.go 中定义，同 package 可直接使用。
func TestResolveSignupGrantPlan_GlobalQuotaLoadedBeforeAuthSource(t *testing.T) {
	// 全局 quota JSON key（新格式）
	settings := map[string]string{
		SettingKeyRegistrationEnabled: "true",
		SettingKeyDefaultPlatformQuotas: `{
			"anthropic":   {"daily": 10, "weekly": 50, "monthly": 200},
			"openai":      {"daily": 5,  "weekly": 25, "monthly": 100},
			"gemini":      {"daily": 5,  "weekly": 25, "monthly": 100},
			"antigravity": {"daily": 5,  "weekly": 25, "monthly": 100}
		}`,
	}
	svc := newAuthService(nil, settings, nil, nil)
	plan := svc.resolveSignupGrantPlan(context.Background(), "email")
	if plan.PlatformQuotas == nil {
		t.Fatal("expected PlatformQuotas to be non-nil after loading global quota KVs")
	}
	q := plan.PlatformQuotas["anthropic"]
	if q == nil {
		t.Fatal("expected anthropic quota to be set")
	}
	if q.DailyLimitUSD == nil || *q.DailyLimitUSD != 10 {
		t.Errorf("expected anthropic daily=10, got %v", q.DailyLimitUSD)
	}
}

// TestResolveSignupGrantPlan_DisabledAuthSourceStillCarriesGlobalQuota 验证 P1 约束：
// !enabled 早退路径仍携带全局 quota（GetDefaultPlatformQuotas 在 ResolveAuthSourceGrantSettings 之前）。
func TestResolveSignupGrantPlan_DisabledAuthSourceStillCarriesGlobalQuota(t *testing.T) {
	settings := map[string]string{
		SettingKeyRegistrationEnabled: "true",
		// auth source 不配置（=> !enabled 路径）
		SettingKeyDefaultPlatformQuotas: `{"anthropic": {"daily": 10, "weekly": 50, "monthly": 200}}`,
	}
	svc := newAuthService(nil, settings, nil, nil)
	plan := svc.resolveSignupGrantPlan(context.Background(), "email")
	// !enabled 路径：plan.PlatformQuotas 应已含全局层（不是 nil）
	if plan.PlatformQuotas == nil {
		t.Fatal("P1 violated: PlatformQuotas is nil even with global quota KVs set")
	}
	// P1 核心断言：disabled auth source 路径不能丢失全局 quota
	if _, ok := plan.PlatformQuotas["anthropic"]; !ok {
		t.Error("P1 violated: disabled auth source path dropped global platform quota")
	}
}

//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// service_quota_repo_integration_test.go 验证"修改 limiter / path 字段不重置 id"。
//
// 计数 key 公式 svcquota:v2:<rule_id>:<path_id>:<limiter_type>:<target> 含 path_id：
// 旧实现 Update 用 DELETE+INSERT 让 path_id 自增 → 用户感知到"改限额后计数重置"。
// 新实现走 upsertLimitersTx + upsertPathsTx：
//   - limiter 按 (rule_id, limiter_type) 唯一约束 ON CONFLICT DO UPDATE，保留 id
//   - path 按 idx_service_quota_paths_unique（5 字段折叠 NULL）ON CONFLICT DO NOTHING，
//     字段未改的 path 保留旧 id，让 Redis 计数延续
//
// 这里只校验 DB 层 id 稳定性；端到端"Redis 计数延续"由 monitor 集成场景另行覆盖。

func ptrStr(v string) *string { return &v }

func newServiceQuotaTestRepo(t *testing.T) service.ServiceQuotaRuleRepository {
	t.Helper()
	t.Cleanup(func() {
		// 用 CASCADE 清理本测试创建的所有 service_quota_rules（含 limiters/paths 子表）。
		_, _ = integrationDB.ExecContext(context.Background(), `TRUNCATE service_quota_rules RESTART IDENTITY CASCADE`)
	})
	return NewServiceQuotaRuleRepository(integrationDB)
}

func enabledTrue() *bool { v := true; return &v }

// TestServiceQuotaRepo_Update_LimiterPreservesID 验证：改 limit_value 时 limiter id 不变。
func TestServiceQuotaRepo_Update_LimiterPreservesID(t *testing.T) {
	repo := newServiceQuotaTestRepo(t)
	ctx := context.Background()
	created, err := repo.Create(ctx, service.ServiceQuotaRuleInput{
		Enabled:     enabledTrue(),
		Name:        ptrStr("test-rule"),
		CounterMode: service.ServiceQuotaCounterModeShared,
		Limiters: []service.ServiceQuotaLimiterInput{
			{LimiterType: service.ServiceQuotaLimiterRPM, WindowMode: service.ServiceQuotaWindowFixed, LimitValue: 60},
		},
		Paths: []service.ServiceQuotaPathInput{
			{Platform: ptrStr("openai")},
		},
	})
	require.NoError(t, err)
	require.Len(t, created.Limiters, 1)
	originalLimiterID := created.Limiters[0].ID
	require.Len(t, created.Paths, 1)
	originalPathID := created.Paths[0].ID

	// 改 limit_value（同 type）：id 必须保留
	updated, err := repo.Update(ctx, created.ID, service.ServiceQuotaRuleInput{
		Enabled:     enabledTrue(),
		Name:        ptrStr("test-rule"),
		CounterMode: service.ServiceQuotaCounterModeShared,
		Limiters: []service.ServiceQuotaLimiterInput{
			{LimiterType: service.ServiceQuotaLimiterRPM, WindowMode: service.ServiceQuotaWindowFixed, LimitValue: 600},
		},
		Paths: []service.ServiceQuotaPathInput{
			{Platform: ptrStr("openai")},
		},
	})
	require.NoError(t, err)
	require.Len(t, updated.Limiters, 1)
	require.Equal(t, originalLimiterID, updated.Limiters[0].ID, "改 limit_value 不应 burn limiter id")
	require.Equal(t, 600.0, updated.Limiters[0].LimitValue)
	require.Len(t, updated.Paths, 1)
	require.Equal(t, originalPathID, updated.Paths[0].ID, "path 字段未变不应 burn path id")
}

// TestServiceQuotaRepo_Update_LimiterTypeReplaced 验证：把 RPM 换成 TPM 时
// 旧 RPM 行删除，新 TPM 行被插入（id 自然不同）。
func TestServiceQuotaRepo_Update_LimiterTypeReplaced(t *testing.T) {
	repo := newServiceQuotaTestRepo(t)
	ctx := context.Background()
	created, err := repo.Create(ctx, service.ServiceQuotaRuleInput{
		Enabled:     enabledTrue(),
		CounterMode: service.ServiceQuotaCounterModeShared,
		Limiters: []service.ServiceQuotaLimiterInput{
			{LimiterType: service.ServiceQuotaLimiterRPM, WindowMode: service.ServiceQuotaWindowFixed, LimitValue: 60},
		},
		Paths: []service.ServiceQuotaPathInput{{Platform: ptrStr("openai")}},
	})
	require.NoError(t, err)

	updated, err := repo.Update(ctx, created.ID, service.ServiceQuotaRuleInput{
		Enabled:     enabledTrue(),
		CounterMode: service.ServiceQuotaCounterModeShared,
		Limiters: []service.ServiceQuotaLimiterInput{
			{LimiterType: service.ServiceQuotaLimiterTPM, WindowMode: service.ServiceQuotaWindowFixed, LimitValue: 1000},
		},
		Paths: []service.ServiceQuotaPathInput{{Platform: ptrStr("openai")}},
	})
	require.NoError(t, err)
	require.Len(t, updated.Limiters, 1)
	require.Equal(t, service.ServiceQuotaLimiterTPM, updated.Limiters[0].LimiterType)
}

// TestServiceQuotaRepo_Update_PathPreservesIDWhenSameFields 验证：
// 5 个 path 字段完全一致时 path id 保留（关键：counter key 含 path_id，
// 字段未改时 redis 计数器不应"被切换"到新 key）。
func TestServiceQuotaRepo_Update_PathPreservesIDWhenSameFields(t *testing.T) {
	repo := newServiceQuotaTestRepo(t)
	ctx := context.Background()
	created, err := repo.Create(ctx, service.ServiceQuotaRuleInput{
		Enabled:     enabledTrue(),
		CounterMode: service.ServiceQuotaCounterModeShared,
		Limiters: []service.ServiceQuotaLimiterInput{
			{LimiterType: service.ServiceQuotaLimiterRPM, WindowMode: service.ServiceQuotaWindowFixed, LimitValue: 60},
		},
		Paths: []service.ServiceQuotaPathInput{
			{Platform: ptrStr("openai"), ModelPattern: ptrStr("gpt-*")},
			{Platform: ptrStr("anthropic")},
		},
	})
	require.NoError(t, err)
	require.Len(t, created.Paths, 2)
	pathByPlat := map[string]int64{}
	for _, p := range created.Paths {
		require.NotNil(t, p.Platform)
		pathByPlat[*p.Platform] = p.ID
	}

	// 仅改 limiter，不改 paths → path id 必须全部保留
	updated, err := repo.Update(ctx, created.ID, service.ServiceQuotaRuleInput{
		Enabled:     enabledTrue(),
		CounterMode: service.ServiceQuotaCounterModeShared,
		Limiters: []service.ServiceQuotaLimiterInput{
			{LimiterType: service.ServiceQuotaLimiterRPM, WindowMode: service.ServiceQuotaWindowFixed, LimitValue: 600},
		},
		Paths: []service.ServiceQuotaPathInput{
			{Platform: ptrStr("openai"), ModelPattern: ptrStr("gpt-*")},
			{Platform: ptrStr("anthropic")},
		},
	})
	require.NoError(t, err)
	require.Len(t, updated.Paths, 2)
	for _, p := range updated.Paths {
		require.NotNil(t, p.Platform)
		require.Equal(t, pathByPlat[*p.Platform], p.ID, "path 字段未变 → id 必须保留 (platform=%s)", *p.Platform)
	}
}

// TestServiceQuotaRepo_Update_PathRemoved 验证：删除其中一个 path 后该 path 行从 DB 消失，
// 剩余的 path id 仍然保留。
func TestServiceQuotaRepo_Update_PathRemoved(t *testing.T) {
	repo := newServiceQuotaTestRepo(t)
	ctx := context.Background()
	created, err := repo.Create(ctx, service.ServiceQuotaRuleInput{
		Enabled:     enabledTrue(),
		CounterMode: service.ServiceQuotaCounterModeShared,
		Limiters: []service.ServiceQuotaLimiterInput{
			{LimiterType: service.ServiceQuotaLimiterRPM, WindowMode: service.ServiceQuotaWindowFixed, LimitValue: 60},
		},
		Paths: []service.ServiceQuotaPathInput{
			{Platform: ptrStr("openai")},
			{Platform: ptrStr("anthropic")},
		},
	})
	require.NoError(t, err)
	require.Len(t, created.Paths, 2)
	keep := int64(0)
	for _, p := range created.Paths {
		if p.Platform != nil && *p.Platform == "openai" {
			keep = p.ID
		}
	}
	require.NotZero(t, keep)

	updated, err := repo.Update(ctx, created.ID, service.ServiceQuotaRuleInput{
		Enabled:     enabledTrue(),
		CounterMode: service.ServiceQuotaCounterModeShared,
		Limiters: []service.ServiceQuotaLimiterInput{
			{LimiterType: service.ServiceQuotaLimiterRPM, WindowMode: service.ServiceQuotaWindowFixed, LimitValue: 60},
		},
		Paths: []service.ServiceQuotaPathInput{
			{Platform: ptrStr("openai")},
		},
	})
	require.NoError(t, err)
	require.Len(t, updated.Paths, 1)
	require.Equal(t, keep, updated.Paths[0].ID, "保留的 path id 不应变")
}

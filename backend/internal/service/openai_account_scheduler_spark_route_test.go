package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSparkRoutingByModel(t *testing.T) {
	ctx := context.Background()
	sparkModel := "gpt-5.3-codex-spark"
	normalModel := "gpt-5.3-codex"
	sparkCreds := map[string]any{"model_mapping": defaultSparkShadowModelMapping()}

	newScheduler := func(snapshot map[int64]*Account) *defaultOpenAIAccountScheduler {
		return &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{
			schedulerSnapshot: &SchedulerSnapshotService{
				cache: &openAISnapshotCacheStub{accountsByID: snapshot},
			},
			cfg: &config.Config{},
		}}
	}
	sparkReq := OpenAIAccountScheduleRequest{RequestedModel: sparkModel, Platform: PlatformOpenAI}
	normalReq := OpenAIAccountScheduleRequest{RequestedModel: normalModel, Platform: PlatformOpenAI}

	t.Run("normal_account_with_spark_mapping_accepts_spark", func(t *testing.T) {
		acc := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Credentials: sparkCreds}
		require.True(t, newScheduler(nil).isAccountRequestCompatible(ctx, acc, sparkReq),
			"普通账号配了 spark → 可承接 spark（类型门已移除）")
	})

	t.Run("normal_account_without_spark_rejects_spark", func(t *testing.T) {
		acc := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
			Credentials: map[string]any{"model_mapping": map[string]any{normalModel: normalModel}}}
		require.False(t, newScheduler(nil).isAccountRequestCompatible(ctx, acc, sparkReq),
			"普通账号未配 spark → 拒 spark（按配置而非类型）")
	})

	t.Run("shadow_with_spark_mapping_accepts_spark_rejects_non_spark", func(t *testing.T) {
		pid := int64(100)
		parent := &Account{ID: 100, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
		shadow := &Account{ID: 200, ParentAccountID: &pid, QuotaDimension: QuotaDimensionSpark,
			Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: sparkCreds}
		s := newScheduler(map[int64]*Account{100: parent})
		require.True(t, s.isAccountRequestCompatible(ctx, shadow, sparkReq), "影子配 spark + 健康母 → 接 spark")
		require.False(t, s.isAccountRequestCompatible(ctx, shadow, normalReq), "影子（仅 spark mapping）→ 拒非 spark")
	})

	t.Run("empty_model_shadow_is_eligible_under_a2", func(t *testing.T) {
		// 有意的纯 A2 行为(用户裁决 2026-06-30)：空 model 请求不经模型门过滤
		// （isAccountRequestCompatible 的 `req.RequestedModel != ""` 短路），故影子与普通账号
		// 一样成为候选。旧类型门曾在空 model 时排除影子(opt-in)，该 opt-in 已随类型门移除——
		// routing 路径不再有任何类型判断。此测试锁定该决策，防被未来改动静默改回。
		pid := int64(100)
		parent := &Account{ID: 100, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
		shadow := &Account{ID: 200, ParentAccountID: &pid, QuotaDimension: QuotaDimensionSpark,
			Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: sparkCreds}
		emptyReq := OpenAIAccountScheduleRequest{RequestedModel: "", Platform: PlatformOpenAI}
		s := newScheduler(map[int64]*Account{100: parent})
		require.True(t, s.isAccountRequestCompatible(ctx, shadow, emptyReq),
			"空 model 时影子可被选中（有意的纯 A2 行为：类型门移除后无 opt-in 排除）")
	})
}

// TestParentHealthSchedulerIntegration 通过 isAccountRequestCompatible 验证「母账号不可调度时影子被
// 调度器拒绝」这一联动在调度器层面端到端生效。
//
// 使用的接缝：defaultOpenAIAccountScheduler.isAccountRequestCompatible，它通过
// s.service.schedulerSnapshot.GetAccount(ctx, parentID) 解析母账号；
// openAISnapshotCacheStub.accountsByID 提供对应的测试桩。
func TestParentHealthSchedulerIntegration(t *testing.T) {
	ctx := context.Background()
	pid := int64(78100)
	sparkModel := "gpt-5.3-codex-spark"

	shadow := &Account{
		ID:              78200,
		ParentAccountID: &pid,
		QuotaDimension:  QuotaDimensionSpark,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		Status:          StatusActive,
		Schedulable:     true,
		Concurrency:     1,
	}

	req := OpenAIAccountScheduleRequest{
		RequestedModel: sparkModel,
		Platform:       PlatformOpenAI,
	}

	makeScheduler := func(parent *Account) *defaultOpenAIAccountScheduler {
		snapshotCache := &openAISnapshotCacheStub{
			accountsByID: map[int64]*Account{parent.ID: parent},
		}
		snapshotSvc := &SchedulerSnapshotService{cache: snapshotCache}
		svc := &OpenAIGatewayService{
			schedulerSnapshot: snapshotSvc,
			cfg:               &config.Config{},
		}
		return &defaultOpenAIAccountScheduler{service: svc}
	}

	t.Run("unhealthy_parent_status_error_rejects_shadow", func(t *testing.T) {
		unhealthyParent := &Account{
			ID:          78100,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusError, // IsActive()==false → IsSchedulable()==false
			Schedulable: true,
		}
		require.False(t, unhealthyParent.IsSchedulable(), "前提：Status=error 的母账号不可调度")
		scheduler := makeScheduler(unhealthyParent)
		require.False(t, scheduler.isAccountRequestCompatible(ctx, shadow, req),
			"母账号不可调度时，影子账号必须被调度器拒绝")
	})

	t.Run("manual_schedulable_false_parent_does_not_reject_shadow", func(t *testing.T) {
		// F1 决策 A:母账号手动暂停(Schedulable=false)不传播到影子 —— 凭据仍可用,影子应被接受。
		manualPausedParent := &Account{
			ID:          78100,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: false, // 显式手动暂停
		}
		require.False(t, manualPausedParent.IsSchedulable(), "前提：手动暂停的母账号自身不可调度")
		scheduler := makeScheduler(manualPausedParent)
		require.True(t, scheduler.isAccountRequestCompatible(ctx, shadow, req),
			"母账号手动暂停不应连坐影子(凭据仍可用)")
	})

	t.Run("global_rate_limited_parent_does_not_reject_shadow", func(t *testing.T) {
		// F1 核心修复:母账号 global 429(RateLimitResetAt)不连坐 spark 影子。
		resetAt := time.Now().Add(1 * time.Hour)
		rateLimitedParent := &Account{
			ID:               78100,
			Platform:         PlatformOpenAI,
			Type:             AccountTypeOAuth,
			Status:           StatusActive,
			Schedulable:      true,
			RateLimitResetAt: &resetAt,
		}
		require.False(t, rateLimitedParent.IsSchedulable(), "前提：global 限流母账号自身不可调度")
		scheduler := makeScheduler(rateLimitedParent)
		require.True(t, scheduler.isAccountRequestCompatible(ctx, shadow, req),
			"母账号 global 限流不应连坐 spark 影子")
	})

	t.Run("healthy_parent_accepts_shadow_control", func(t *testing.T) {
		healthyParent := &Account{
			ID:          78100,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
		}
		require.True(t, healthyParent.IsSchedulable(), "前提：健康母账号必须可调度")
		scheduler := makeScheduler(healthyParent)
		require.True(t, scheduler.isAccountRequestCompatible(ctx, shadow, req),
			"健康母账号时，影子账号必须被调度器接受（对照组）")
	})
}

func TestParentHealthSchedulerFallsBackToRepoWhenSnapshotMissesParent(t *testing.T) {
	ctx := context.Background()
	parentID := int64(79100)
	parent := Account{
		ID:          parentID,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
	}
	shadow := &Account{
		ID:              79200,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		Status:          StatusActive,
		Schedulable:     true,
		Concurrency:     1,
	}

	repo := schedulerTestOpenAIAccountRepo{accounts: []Account{parent}}
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{
		accountRepo: repo,
		schedulerSnapshot: &SchedulerSnapshotService{
			cache:       &openAISnapshotCacheStub{},
			accountRepo: repo,
			cfg: &config.Config{
				Gateway: config.GatewayConfig{
					Scheduling: config.GatewaySchedulingConfig{
						DbFallbackEnabled: false,
					},
				},
			},
		},
		cfg: &config.Config{},
	}}

	require.True(t, scheduler.isAccountRequestCompatible(ctx, shadow, OpenAIAccountScheduleRequest{
		RequestedModel: "gpt-5.3-codex-spark",
		Platform:       PlatformOpenAI,
	}), "快照缺失母账号且调度快照 DB fallback 关闭时，应回退 repo 解析健康母账号")
}

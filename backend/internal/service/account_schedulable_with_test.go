//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// activeSchedulableAccount 构造"无任何阻塞条件"的 baseline 账号。
// 各 sub-test 在此基础上修改单个字段以验证某一类阻塞优先级。
func activeSchedulableAccount() *Account {
	return &Account{
		ID:          1,
		Status:      StatusActive,
		Schedulable: true,
		Type:        AccountTypeOAuth,
	}
}

// TestIsSchedulableWith_NilSchedCheck_EquivalentToOld：传 nil sc 必须等价于历史 IsSchedulable() 行为。
func TestIsSchedulableWith_NilSchedCheck_EquivalentToOld(t *testing.T) {
	t.Parallel()
	a := activeSchedulableAccount()
	require.True(t, a.IsSchedulableWith(nil))
	require.True(t, a.IsSchedulable())
}

// TestIsSchedulable_LegacyCallersUnchanged：原 IsSchedulable() 旧签名零修改、行为零回归。
func TestIsSchedulable_LegacyCallersUnchanged(t *testing.T) {
	t.Parallel()
	a := activeSchedulableAccount()
	a.Status = StatusDisabled
	require.False(t, a.IsSchedulable())
}

// TestIsSchedulableWith_QuotaBlock_ReturnsFalse：sc.QuotaBlock 非 nil → 不可调度。
func TestIsSchedulableWith_QuotaBlock_ReturnsFalse(t *testing.T) {
	t.Parallel()
	a := activeSchedulableAccount()
	sc := &AccountSchedCheck{
		QuotaBlock: &ServiceQuotaPredictedBlock{
			RuleID:      11,
			LimiterType: ServiceQuotaLimiterRPM,
			ScopeKind:   ServiceQuotaPredictedBlockScopeAccount,
			Used:        9,
			Limit:       9,
		},
	}
	require.False(t, a.IsSchedulableWith(sc))
}

// TestIsSchedulableWith_NativeOverridesQuotaBlock：原生条件命中（如 RateLimitResetAt）
// 时即使没传 QuotaBlock 也返回 false——原生优先级与 service_quota 隔离开。
func TestIsSchedulableWith_NativeOverridesQuotaBlock(t *testing.T) {
	t.Parallel()
	a := activeSchedulableAccount()
	now := time.Now().Add(5 * time.Minute)
	a.RateLimitResetAt = &now
	require.False(t, a.IsSchedulableWith(nil))
	require.False(t, a.IsSchedulableWith(&AccountSchedCheck{}))
}

// TestSchedulableReason_PriorityOrder：原生条件优先于 service_quota（与 IsSchedulableWith 顺序一致）。
func TestSchedulableReason_PriorityOrder(t *testing.T) {
	t.Parallel()

	t.Run("active+empty sc → no reason", func(t *testing.T) {
		require.Equal(t, "", activeSchedulableAccount().SchedulableReason(nil))
	})

	t.Run("inactive precedes everything", func(t *testing.T) {
		a := activeSchedulableAccount()
		a.Status = StatusDisabled
		require.Equal(t, "not_schedulable", a.SchedulableReason(&AccountSchedCheck{
			QuotaBlock: &ServiceQuotaPredictedBlock{LimiterType: "rpm", ScopeKind: "account"},
		}))
	})

	t.Run("rate_limited precedes service_quota", func(t *testing.T) {
		a := activeSchedulableAccount()
		future := time.Now().Add(time.Minute)
		a.RateLimitResetAt = &future
		got := a.SchedulableReason(&AccountSchedCheck{
			QuotaBlock: &ServiceQuotaPredictedBlock{LimiterType: "rpm", ScopeKind: "account"},
		})
		require.Equal(t, "rate_limited", got)
	})

	t.Run("service_quota when no native block", func(t *testing.T) {
		a := activeSchedulableAccount()
		got := a.SchedulableReason(&AccountSchedCheck{
			QuotaBlock: &ServiceQuotaPredictedBlock{
				LimiterType: ServiceQuotaLimiterTPM,
				ScopeKind:   ServiceQuotaPredictedBlockScopeChannel,
			},
		})
		require.Equal(t, "service_quota:tpm:channel", got)
	})
}

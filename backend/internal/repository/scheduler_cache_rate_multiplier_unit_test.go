//go:build unit

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// TestSchedulerCachePreservesRateMultiplier 钉死账号调度快照的两份 payload
// （full + metadata）都必须保留 RateMultiplier。
//
// 为什么值得一条专门的护栏：分组利润门对 nil RateMultiplier 是 fail-closed
// （保守拒绝），与全系统其他地方不同——Account.BillingRateMultiplier() 对同样
// 的 nil 返回 1.0，字段注释也写着「nil 表示按 1.0 处理」。DB 列有 Default(1.0)
// 且非 nillable，因此 nil 只可能来自缓存反序列化缺字段。
// buildSchedulerMetadataAccount 是显式字段清单，将来任何一次快照结构调整漏掉
// 这一项，对启用利润控制的分组就是全组 no available accounts——而且是静默的。
// 这条测试让那种漏列在 CI 就变红。
func TestSchedulerCachePreservesRateMultiplier(t *testing.T) {
	rate := 0.75
	account := service.Account{
		ID:             9001,
		Name:           "profit-gate-rate",
		Platform:       service.PlatformOpenAI,
		Type:           service.AccountTypeAPIKey,
		Status:         service.StatusActive,
		Schedulable:    true,
		Concurrency:    2,
		RateMultiplier: &rate,
	}

	t.Run("metadata payload keeps the field", func(t *testing.T) {
		meta := buildSchedulerMetadataAccount(account)
		require.NotNil(t, meta.RateMultiplier, "metadata 快照漏掉 rate_multiplier 会让利润门 fail-closed 全组拒绝")
		require.Equal(t, rate, *meta.RateMultiplier)
	})

	t.Run("both payloads survive a decode round-trip", func(t *testing.T) {
		full, meta, err := marshalSchedulerCacheAccount(account)
		require.NoError(t, err)

		for name, payload := range map[string][]byte{"full": full, "metadata": meta} {
			decoded, decodeErr := decodeCachedAccount(payload)
			require.NoError(t, decodeErr, name)
			require.NotNil(t, decoded.RateMultiplier, "%s payload 反序列化后 rate_multiplier 不得为 nil", name)
			require.Equal(t, rate, *decoded.RateMultiplier, name)
		}
	})

	t.Run("zero rate survives as zero rather than nil", func(t *testing.T) {
		// 0 是合法值（该账号上游成本为 0），绝不能被当成缺字段丢掉：
		// 丢成 nil 后利润门会把一个本该必然放行的账号判成越线。
		zero := 0.0
		zeroAccount := account
		zeroAccount.ID = 9002
		zeroAccount.RateMultiplier = &zero

		_, meta, err := marshalSchedulerCacheAccount(zeroAccount)
		require.NoError(t, err)
		decoded, err := decodeCachedAccount(meta)
		require.NoError(t, err)
		require.NotNil(t, decoded.RateMultiplier)
		require.Equal(t, 0.0, *decoded.RateMultiplier)
	})

	t.Run("SetAccount then GetAccount preserves the field", func(t *testing.T) {
		cache := newSchedulerCacheUnit(t)
		ctx := context.Background()
		require.NoError(t, cache.SetAccount(ctx, &account))

		got, err := cache.GetAccount(ctx, account.ID)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.NotNil(t, got.RateMultiplier, "端到端缓存读写后 rate_multiplier 不得丢失")
		require.Equal(t, rate, *got.RateMultiplier)
	})
}

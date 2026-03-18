//go:build unit

package service

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock: GatewayCache for affinity tests
// ---------------------------------------------------------------------------

// mockAffinityCache 为亲和调度测试提供可控的 GatewayCache mock。
// 通过 getCountBatchFunc 可以自定义 GetAccountAffinityCountBatch 的行为。
type mockAffinityCache struct {
	getCountBatchFunc  func(ctx context.Context, groupID int64, accountIDs []int64, ttl time.Duration) (map[int64]int64, error)
	getCountBatchCalls int // 记录 GetAccountAffinityCountBatch 被调用次数
}

func (m *mockAffinityCache) GetSessionAccountID(_ context.Context, _ int64, _ string) (int64, error) {
	return 0, errors.New("not found")
}
func (m *mockAffinityCache) SetSessionAccountID(_ context.Context, _ int64, _ string, _ int64, _ time.Duration) error {
	return nil
}
func (m *mockAffinityCache) RefreshSessionTTL(_ context.Context, _ int64, _ string, _ time.Duration) error {
	return nil
}
func (m *mockAffinityCache) DeleteSessionAccountID(_ context.Context, _ int64, _ string) error {
	return nil
}
func (m *mockAffinityCache) GetAffinityAccounts(_ context.Context, _ int64, _ int64, _ string, _ time.Duration) ([]int64, error) {
	return nil, nil
}
func (m *mockAffinityCache) UpdateAffinity(_ context.Context, _ int64, _ int64, _ string, _ int64, _ time.Duration) error {
	return nil
}
func (m *mockAffinityCache) GetAffinityMultiCount(_ context.Context, _ int64, _ int64, _ int64, _ time.Duration) (int64, int64, int64, error) {
	return 0, 0, 0, nil
}
func (m *mockAffinityCache) GetAccountAffinityCountBatch(ctx context.Context, groupID int64, accountIDs []int64, ttl time.Duration) (map[int64]int64, error) {
	m.getCountBatchCalls++
	if m.getCountBatchFunc != nil {
		return m.getCountBatchFunc(ctx, groupID, accountIDs, ttl)
	}
	return map[int64]int64{}, nil
}
func (m *mockAffinityCache) GetAccountAffinityClientsBatch(_ context.Context, _ map[int64][]int64, _ time.Duration) (map[int64][]string, error) {
	return map[int64][]string{}, nil
}
func (m *mockAffinityCache) GetAccountAffinityClientsWithScores(_ context.Context, _ int64, _ []int64, _ time.Duration) ([]AffinityClient, error) {
	return nil, nil
}
func (m *mockAffinityCache) ClearAccountAffinity(_ context.Context, _ int64, _ []int64) error {
	return nil
}

// ---------------------------------------------------------------------------
// Helper: 构造启用了客户端亲和的 Anthropic 账号
// ---------------------------------------------------------------------------

func newAffinityAccount(id int64, priority int, affinityEnabled bool) *Account {
	acc := &Account{
		ID:       id,
		Platform: PlatformAnthropic,
		Priority: priority,
		Status:   StatusActive,
	}
	if affinityEnabled {
		acc.Extra = map[string]any{"client_affinity_enabled": true}
	}
	return acc
}

func newAffinityAccountWithLoad(id int64, priority int, loadRate int, affinityCount int64, lastUsedAt *time.Time) accountWithLoad {
	return accountWithLoad{
		account:       newAffinityAccount(id, priority, true),
		loadInfo:      &AccountLoadInfo{AccountID: id, LoadRate: loadRate},
		affinityCount: affinityCount,
	}
}

// ===========================================================================
// 1. filterByMinAffinityCount 测试
// ===========================================================================

func TestAffinityFilterByMinAffinityCount(t *testing.T) {
	t.Run("empty slice returns empty", func(t *testing.T) {
		result := filterByMinAffinityCount(nil)
		require.Empty(t, result)
	})

	t.Run("single element returned as-is", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}, affinityCount: 5},
		}
		result := filterByMinAffinityCount(accounts)
		require.Len(t, result, 1)
		require.Equal(t, int64(1), result[0].account.ID)
	})

	t.Run("all same affinityCount returns all", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}, affinityCount: 3},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}, affinityCount: 3},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{}, affinityCount: 3},
		}
		result := filterByMinAffinityCount(accounts)
		require.Len(t, result, 3)
	})

	t.Run("filters to min affinityCount only", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}, affinityCount: 10},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}, affinityCount: 2},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{}, affinityCount: 5},
			{account: &Account{ID: 4}, loadInfo: &AccountLoadInfo{}, affinityCount: 2},
		}
		result := filterByMinAffinityCount(accounts)
		require.Len(t, result, 2)
		require.Equal(t, int64(2), result[0].account.ID)
		require.Equal(t, int64(4), result[1].account.ID)
	})

	t.Run("zero affinityCount is smallest", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}, affinityCount: 5},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}, affinityCount: 0},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{}, affinityCount: 3},
			{account: &Account{ID: 4}, loadInfo: &AccountLoadInfo{}, affinityCount: 0},
		}
		result := filterByMinAffinityCount(accounts)
		require.Len(t, result, 2)
		require.Equal(t, int64(2), result[0].account.ID)
		require.Equal(t, int64(4), result[1].account.ID)
	})

	t.Run("preserves order within same affinityCount", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 5}, loadInfo: &AccountLoadInfo{}, affinityCount: 1},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{}, affinityCount: 1},
			{account: &Account{ID: 7}, loadInfo: &AccountLoadInfo{}, affinityCount: 2},
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}, affinityCount: 1},
		}
		result := filterByMinAffinityCount(accounts)
		require.Len(t, result, 3)
		// 验证保持原始顺序
		require.Equal(t, int64(5), result[0].account.ID)
		require.Equal(t, int64(3), result[1].account.ID)
		require.Equal(t, int64(1), result[2].account.ID)
	})
}

// ===========================================================================
// 2. populateAffinityCounts 测试
// ===========================================================================

func TestAffinityPopulateAffinityCounts(t *testing.T) {
	ctx := context.Background()

	t.Run("nil cache does not panic", func(t *testing.T) {
		svc := &GatewayService{cache: nil}
		accounts := []accountWithLoad{
			{account: newAffinityAccount(1, 1, true), loadInfo: &AccountLoadInfo{}},
		}
		// 不应 panic
		svc.populateAffinityCounts(ctx, accounts, 0)
		// affinityCount 保持零值
		require.Equal(t, int64(0), accounts[0].affinityCount)
	})

	t.Run("empty accounts returns immediately", func(t *testing.T) {
		cache := &mockAffinityCache{}
		svc := &GatewayService{cache: cache}
		svc.populateAffinityCounts(ctx, nil, 0)
		require.Equal(t, 0, cache.getCountBatchCalls, "should not call Redis for empty accounts")
	})

	t.Run("no affinity-enabled accounts skips Redis call", func(t *testing.T) {
		cache := &mockAffinityCache{}
		svc := &GatewayService{cache: cache}
		accounts := []accountWithLoad{
			// Anthropic 但未启用亲和
			{account: newAffinityAccount(1, 1, false), loadInfo: &AccountLoadInfo{}},
			// 非 Anthropic 平台
			{account: &Account{ID: 2, Platform: PlatformOpenAI}, loadInfo: &AccountLoadInfo{}},
		}
		svc.populateAffinityCounts(ctx, accounts, 0)
		require.Equal(t, 0, cache.getCountBatchCalls, "should skip Redis when no affinity-enabled accounts")
	})

	t.Run("correctly populates affinityCount from Redis", func(t *testing.T) {
		cache := &mockAffinityCache{
			getCountBatchFunc: func(_ context.Context, _ int64, accountIDs []int64, _ time.Duration) (map[int64]int64, error) {
				result := map[int64]int64{}
				for _, id := range accountIDs {
					switch id {
					case 1:
						result[1] = 5
					case 2:
						result[2] = 0
					case 3:
						result[3] = 12
					}
				}
				return result, nil
			},
		}
		svc := &GatewayService{cache: cache}

		accounts := []accountWithLoad{
			{account: newAffinityAccount(1, 1, true), loadInfo: &AccountLoadInfo{}},
			{account: newAffinityAccount(2, 1, false), loadInfo: &AccountLoadInfo{}}, // 未启用，但仍在列表中
			{account: newAffinityAccount(3, 1, true), loadInfo: &AccountLoadInfo{}},
		}

		svc.populateAffinityCounts(ctx, accounts, 100)

		require.Equal(t, 1, cache.getCountBatchCalls, "should call Redis exactly once")
		require.Equal(t, int64(5), accounts[0].affinityCount, "account 1 should have count 5")
		require.Equal(t, int64(0), accounts[1].affinityCount, "account 2 should have count 0")
		require.Equal(t, int64(12), accounts[2].affinityCount, "account 3 should have count 12")
	})

	t.Run("Redis error degrades gracefully with counts at 0", func(t *testing.T) {
		cache := &mockAffinityCache{
			getCountBatchFunc: func(_ context.Context, _ int64, _ []int64, _ time.Duration) (map[int64]int64, error) {
				return nil, errors.New("redis connection refused")
			},
		}
		svc := &GatewayService{cache: cache}

		accounts := []accountWithLoad{
			{account: newAffinityAccount(1, 1, true), loadInfo: &AccountLoadInfo{}},
			{account: newAffinityAccount(2, 1, true), loadInfo: &AccountLoadInfo{}},
		}

		svc.populateAffinityCounts(ctx, accounts, 0)

		require.Equal(t, 1, cache.getCountBatchCalls)
		require.Equal(t, int64(0), accounts[0].affinityCount, "should remain 0 on error")
		require.Equal(t, int64(0), accounts[1].affinityCount, "should remain 0 on error")
	})

	t.Run("partial Redis result fills only known accounts", func(t *testing.T) {
		cache := &mockAffinityCache{
			getCountBatchFunc: func(_ context.Context, _ int64, _ []int64, _ time.Duration) (map[int64]int64, error) {
				// 只返回部分账号的计数
				return map[int64]int64{1: 7}, nil
			},
		}
		svc := &GatewayService{cache: cache}

		accounts := []accountWithLoad{
			{account: newAffinityAccount(1, 1, true), loadInfo: &AccountLoadInfo{}},
			{account: newAffinityAccount(2, 1, true), loadInfo: &AccountLoadInfo{}},
		}

		svc.populateAffinityCounts(ctx, accounts, 0)

		require.Equal(t, int64(7), accounts[0].affinityCount)
		require.Equal(t, int64(0), accounts[1].affinityCount, "missing account should default to 0")
	})

	t.Run("queries all account IDs regardless of affinity status", func(t *testing.T) {
		// 验证：只要有至少一个 affinity-enabled 账号，就查询 ALL 账号的计数
		var queriedIDs []int64
		cache := &mockAffinityCache{
			getCountBatchFunc: func(_ context.Context, _ int64, accountIDs []int64, _ time.Duration) (map[int64]int64, error) {
				queriedIDs = accountIDs
				return map[int64]int64{}, nil
			},
		}
		svc := &GatewayService{cache: cache}

		accounts := []accountWithLoad{
			{account: newAffinityAccount(1, 1, true), loadInfo: &AccountLoadInfo{}},
			{account: newAffinityAccount(2, 1, false), loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3, Platform: PlatformOpenAI}, loadInfo: &AccountLoadInfo{}},
		}

		svc.populateAffinityCounts(ctx, accounts, 0)

		require.Equal(t, 1, cache.getCountBatchCalls)
		require.Equal(t, []int64{1, 2, 3}, queriedIDs, "should query ALL account IDs, not just affinity-enabled ones")
	})
}

// ===========================================================================
// 3. Layer 1 排序链测试（sort.SliceStable 中 affinityCount 的正确性）
// ===========================================================================

func TestAffinityLayer1SortChain(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)
	muchEarlier := now.Add(-2 * time.Hour)

	// 复现 Layer 1 的排序逻辑
	sortByLayer1 := func(accounts []accountWithLoad) {
		sort.SliceStable(accounts, func(i, j int) bool {
			a, b := accounts[i], accounts[j]
			if a.account.Priority != b.account.Priority {
				return a.account.Priority < b.account.Priority
			}
			if a.loadInfo.LoadRate != b.loadInfo.LoadRate {
				return a.loadInfo.LoadRate < b.loadInfo.LoadRate
			}
			if a.affinityCount != b.affinityCount {
				return a.affinityCount < b.affinityCount
			}
			switch {
			case a.account.LastUsedAt == nil && b.account.LastUsedAt != nil:
				return true
			case a.account.LastUsedAt != nil && b.account.LastUsedAt == nil:
				return false
			case a.account.LastUsedAt == nil && b.account.LastUsedAt == nil:
				return false
			default:
				return a.account.LastUsedAt.Before(*b.account.LastUsedAt)
			}
		})
	}

	t.Run("same priority same loadRate sorts by affinityCount asc", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 50}, affinityCount: 10},
			{account: &Account{ID: 2, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 50}, affinityCount: 2},
			{account: &Account{ID: 3, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 50}, affinityCount: 5},
		}
		sortByLayer1(accounts)
		require.Equal(t, int64(2), accounts[0].account.ID, "lowest affinityCount first")
		require.Equal(t, int64(3), accounts[1].account.ID)
		require.Equal(t, int64(1), accounts[2].account.ID, "highest affinityCount last")
	})

	t.Run("priority takes precedence over affinityCount", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 2, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 50}, affinityCount: 0},
			{account: &Account{ID: 2, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 50}, affinityCount: 100},
		}
		sortByLayer1(accounts)
		require.Equal(t, int64(2), accounts[0].account.ID, "lower priority wins despite higher affinityCount")
	})

	t.Run("loadRate takes precedence over affinityCount", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 80}, affinityCount: 0},
			{account: &Account{ID: 2, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 20}, affinityCount: 100},
		}
		sortByLayer1(accounts)
		require.Equal(t, int64(2), accounts[0].account.ID, "lower loadRate wins despite higher affinityCount")
	})

	t.Run("affinityCount takes precedence over LRU", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 1, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{LoadRate: 50}, affinityCount: 5},
			{account: &Account{ID: 2, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 50}, affinityCount: 1},
		}
		sortByLayer1(accounts)
		require.Equal(t, int64(2), accounts[0].account.ID, "lower affinityCount wins despite older LRU")
	})

	t.Run("same affinityCount falls through to LRU", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 50}, affinityCount: 3},
			{account: &Account{ID: 2, Priority: 1, LastUsedAt: &earlier}, loadInfo: &AccountLoadInfo{LoadRate: 50}, affinityCount: 3},
			{account: &Account{ID: 3, Priority: 1, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{LoadRate: 50}, affinityCount: 3},
		}
		sortByLayer1(accounts)
		require.Equal(t, int64(3), accounts[0].account.ID, "LRU: oldest used first")
		require.Equal(t, int64(2), accounts[1].account.ID)
		require.Equal(t, int64(1), accounts[2].account.ID, "LRU: most recently used last")
	})

	t.Run("full chain: priority > loadRate > affinityCount > LRU", func(t *testing.T) {
		accounts := []accountWithLoad{
			// 优先级 2 - 不管其他维度如何，排在后面
			{account: &Account{ID: 10, Priority: 2, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{LoadRate: 0}, affinityCount: 0},
			// 优先级 1，负载 80% - 负载高
			{account: &Account{ID: 20, Priority: 1, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{LoadRate: 80}, affinityCount: 0},
			// 优先级 1，负载 20%，亲和 5
			{account: &Account{ID: 30, Priority: 1, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{LoadRate: 20}, affinityCount: 5},
			// 优先级 1，负载 20%，亲和 1，最近使用
			{account: &Account{ID: 40, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 20}, affinityCount: 1},
			// 优先级 1，负载 20%，亲和 1，更早使用（应排最前）
			{account: &Account{ID: 50, Priority: 1, LastUsedAt: &earlier}, loadInfo: &AccountLoadInfo{LoadRate: 20}, affinityCount: 1},
		}
		sortByLayer1(accounts)
		// 期望排序：50 → 40 → 30 → 20 → 10
		require.Equal(t, int64(50), accounts[0].account.ID, "best: p1, lr20, ac1, LRU earlier")
		require.Equal(t, int64(40), accounts[1].account.ID, "second: p1, lr20, ac1, LRU now")
		require.Equal(t, int64(30), accounts[2].account.ID, "third: p1, lr20, ac5")
		require.Equal(t, int64(20), accounts[3].account.ID, "fourth: p1, lr80")
		require.Equal(t, int64(10), accounts[4].account.ID, "last: p2")
	})
}

// ===========================================================================
// 4. Layer 2 分层过滤链完整性测试
// ===========================================================================

func TestAffinityLayer2FilterChain(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)
	muchEarlier := now.Add(-2 * time.Hour)

	// 模拟 Layer 2 的完整过滤链：Priority → LoadRate → AffinityCount → LRU
	applyLayer2 := func(accounts []accountWithLoad) *accountWithLoad {
		candidates := filterByMinPriority(accounts)
		candidates = filterByMinLoadRate(candidates)
		candidates = filterByMinAffinityCount(candidates)
		return selectByLRU(candidates, false)
	}

	t.Run("priority different - affinityCount does not matter", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 2, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{LoadRate: 0}, affinityCount: 0},
			{account: &Account{ID: 2, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 50}, affinityCount: 100},
		}
		selected := applyLayer2(accounts)
		require.NotNil(t, selected)
		require.Equal(t, int64(2), selected.account.ID, "higher priority dimension overrides affinityCount")
	})

	t.Run("same priority same loadRate different affinityCount", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 30}, affinityCount: 10},
			{account: &Account{ID: 2, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 30}, affinityCount: 2},
			{account: &Account{ID: 3, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 30}, affinityCount: 5},
		}
		selected := applyLayer2(accounts)
		require.NotNil(t, selected)
		require.Equal(t, int64(2), selected.account.ID, "lowest affinityCount wins")
	})

	t.Run("same priority same loadRate same affinityCount falls through to LRU", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 30}, affinityCount: 5},
			{account: &Account{ID: 2, Priority: 1, LastUsedAt: &earlier}, loadInfo: &AccountLoadInfo{LoadRate: 30}, affinityCount: 5},
			{account: &Account{ID: 3, Priority: 1, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{LoadRate: 30}, affinityCount: 5},
		}
		selected := applyLayer2(accounts)
		require.NotNil(t, selected)
		require.Equal(t, int64(3), selected.account.ID, "LRU selects oldest")
	})

	t.Run("loadRate different overrides affinityCount", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 80}, affinityCount: 0},
			{account: &Account{ID: 2, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 10}, affinityCount: 100},
		}
		selected := applyLayer2(accounts)
		require.NotNil(t, selected)
		require.Equal(t, int64(2), selected.account.ID, "lower loadRate wins over lower affinityCount")
	})

	t.Run("full chain integration: p → lr → ac → lru", func(t *testing.T) {
		accounts := []accountWithLoad{
			// p=2 淘汰
			{account: &Account{ID: 1, Priority: 2, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{LoadRate: 0}, affinityCount: 0},
			// p=1, lr=50 淘汰
			{account: &Account{ID: 2, Priority: 1, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{LoadRate: 50}, affinityCount: 0},
			// p=1, lr=10, ac=8 淘汰
			{account: &Account{ID: 3, Priority: 1, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{LoadRate: 10}, affinityCount: 8},
			// p=1, lr=10, ac=2, lru=now 淘汰
			{account: &Account{ID: 4, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 10}, affinityCount: 2},
			// p=1, lr=10, ac=2, lru=muchEarlier → 胜出
			{account: &Account{ID: 5, Priority: 1, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{LoadRate: 10}, affinityCount: 2},
		}
		selected := applyLayer2(accounts)
		require.NotNil(t, selected)
		require.Equal(t, int64(5), selected.account.ID, "full chain selects ID=5")
	})

	t.Run("empty input returns nil", func(t *testing.T) {
		selected := applyLayer2(nil)
		require.Nil(t, selected)
	})

	t.Run("single account always selected", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 42, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 50}, affinityCount: 100},
		}
		selected := applyLayer2(accounts)
		require.NotNil(t, selected)
		require.Equal(t, int64(42), selected.account.ID)
	})

	t.Run("affinityCount zero preferred among same p and lr", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 0}, affinityCount: 5},
			{account: &Account{ID: 2, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 0}, affinityCount: 0},
			{account: &Account{ID: 3, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 0}, affinityCount: 3},
		}
		selected := applyLayer2(accounts)
		require.NotNil(t, selected)
		require.Equal(t, int64(2), selected.account.ID, "zero affinityCount preferred")
	})
}

// ===========================================================================
// 5. populateAffinityCounts + filterByMinAffinityCount 联合测试
// ===========================================================================

func TestAffinityPopulateAndFilterIntegration(t *testing.T) {
	ctx := context.Background()

	t.Run("populate then filter selects least-loaded accounts", func(t *testing.T) {
		cache := &mockAffinityCache{
			getCountBatchFunc: func(_ context.Context, _ int64, _ []int64, _ time.Duration) (map[int64]int64, error) {
				return map[int64]int64{
					1: 10,
					2: 3,
					3: 3,
					4: 7,
				}, nil
			},
		}
		svc := &GatewayService{cache: cache}

		accounts := []accountWithLoad{
			{account: newAffinityAccount(1, 1, true), loadInfo: &AccountLoadInfo{}},
			{account: newAffinityAccount(2, 1, true), loadInfo: &AccountLoadInfo{}},
			{account: newAffinityAccount(3, 1, true), loadInfo: &AccountLoadInfo{}},
			{account: newAffinityAccount(4, 1, true), loadInfo: &AccountLoadInfo{}},
		}

		svc.populateAffinityCounts(ctx, accounts, 0)
		result := filterByMinAffinityCount(accounts)

		require.Len(t, result, 2)
		require.Equal(t, int64(2), result[0].account.ID)
		require.Equal(t, int64(3), result[1].account.ID)
	})

	t.Run("Redis failure results in all accounts having 0 affinityCount", func(t *testing.T) {
		cache := &mockAffinityCache{
			getCountBatchFunc: func(_ context.Context, _ int64, _ []int64, _ time.Duration) (map[int64]int64, error) {
				return nil, errors.New("timeout")
			},
		}
		svc := &GatewayService{cache: cache}

		accounts := []accountWithLoad{
			{account: newAffinityAccount(1, 1, true), loadInfo: &AccountLoadInfo{}},
			{account: newAffinityAccount(2, 1, true), loadInfo: &AccountLoadInfo{}},
		}

		svc.populateAffinityCounts(ctx, accounts, 0)
		result := filterByMinAffinityCount(accounts)

		// 全部为 0，全部返回
		require.Len(t, result, 2, "all accounts should pass filter when Redis fails (all have count 0)")
	})
}

// ===========================================================================
// 6. IsClientAffinityEnabled 边界测试
// ===========================================================================

func TestAffinityIsClientAffinityEnabled(t *testing.T) {
	t.Run("Anthropic with enabled flag", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Extra:    map[string]any{"client_affinity_enabled": true},
		}
		assert.True(t, acc.IsClientAffinityEnabled())
	})

	t.Run("Anthropic with disabled flag", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Extra:    map[string]any{"client_affinity_enabled": false},
		}
		assert.False(t, acc.IsClientAffinityEnabled())
	})

	t.Run("Anthropic with nil Extra", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Extra:    nil,
		}
		assert.False(t, acc.IsClientAffinityEnabled())
	})

	t.Run("Anthropic without the key", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Extra:    map[string]any{"other_key": true},
		}
		assert.False(t, acc.IsClientAffinityEnabled())
	})

	t.Run("non-Anthropic platform always false", func(t *testing.T) {
		platforms := []string{PlatformOpenAI, PlatformGemini, PlatformAntigravity}
		for _, p := range platforms {
			acc := &Account{
				Platform: p,
				Extra:    map[string]any{"client_affinity_enabled": true},
			}
			assert.False(t, acc.IsClientAffinityEnabled(), "platform=%s should not support affinity", p)
		}
	})

	t.Run("wrong type for enabled value", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Extra:    map[string]any{"client_affinity_enabled": "true"}, // string 而非 bool
		}
		assert.False(t, acc.IsClientAffinityEnabled(), "string 'true' should not enable affinity")
	})

	t.Run("new flag false overrides legacy flag true", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Extra: map[string]any{
				"affinity_enabled":        false,
				"client_affinity_enabled": true,
			},
		}
		assert.False(t, acc.IsClientAffinityEnabled())
	})

	t.Run("new flag true overrides legacy flag false", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Extra: map[string]any{
				"affinity_enabled":        true,
				"client_affinity_enabled": false,
			},
		}
		assert.True(t, acc.IsClientAffinityEnabled())
	})
}

// ===========================================================================
// GetAffinityZone 测试
// ===========================================================================

func TestGetAffinityZone(t *testing.T) {
	makeAccount := func(enabled bool, base int, buffer any) *Account {
		extra := map[string]any{"client_affinity_enabled": enabled}
		if base > 0 {
			extra["affinity_base"] = base
		}
		if buffer != nil {
			extra["affinity_buffer"] = buffer
		}
		return &Account{
			Platform: PlatformAnthropic,
			Extra:    extra,
		}
	}

	t.Run("affinity disabled always green", func(t *testing.T) {
		acc := makeAccount(false, 5, 3)
		assert.Equal(t, AffinityZoneGreen, acc.GetAffinityZone(100))
	})

	t.Run("no base configured always green", func(t *testing.T) {
		acc := makeAccount(true, 0, nil)
		assert.Equal(t, AffinityZoneGreen, acc.GetAffinityZone(100))
	})

	t.Run("within base limit is green", func(t *testing.T) {
		acc := makeAccount(true, 5, 3)
		assert.Equal(t, AffinityZoneGreen, acc.GetAffinityZone(0))
		assert.Equal(t, AffinityZoneGreen, acc.GetAffinityZone(3))
		assert.Equal(t, AffinityZoneGreen, acc.GetAffinityZone(5))
	})

	t.Run("no buffer configured infinite yellow", func(t *testing.T) {
		acc := makeAccount(true, 5, nil) // buffer not set → infinite yellow
		assert.Equal(t, AffinityZoneYellow, acc.GetAffinityZone(6))
		assert.Equal(t, AffinityZoneYellow, acc.GetAffinityZone(100))
		assert.Equal(t, AffinityZoneYellow, acc.GetAffinityZone(9999))
	})

	t.Run("buffer zero no yellow zone", func(t *testing.T) {
		acc := makeAccount(true, 5, 0) // buffer=0 → no yellow, direct red
		assert.Equal(t, AffinityZoneGreen, acc.GetAffinityZone(5))
		assert.Equal(t, AffinityZoneRed, acc.GetAffinityZone(6))
		assert.Equal(t, AffinityZoneRed, acc.GetAffinityZone(100))
	})

	t.Run("within buffer is yellow", func(t *testing.T) {
		acc := makeAccount(true, 5, 3)
		assert.Equal(t, AffinityZoneYellow, acc.GetAffinityZone(6))
		assert.Equal(t, AffinityZoneYellow, acc.GetAffinityZone(7))
		assert.Equal(t, AffinityZoneYellow, acc.GetAffinityZone(8)) // base(5)+buffer(3)=8
	})

	t.Run("beyond buffer is red", func(t *testing.T) {
		acc := makeAccount(true, 5, 3)
		assert.Equal(t, AffinityZoneRed, acc.GetAffinityZone(9))
		assert.Equal(t, AffinityZoneRed, acc.GetAffinityZone(100))
	})

	t.Run("boundary exactly at base", func(t *testing.T) {
		acc := makeAccount(true, 10, 5)
		assert.Equal(t, AffinityZoneGreen, acc.GetAffinityZone(10))
		assert.Equal(t, AffinityZoneYellow, acc.GetAffinityZone(11))
	})

	t.Run("boundary exactly at base plus buffer", func(t *testing.T) {
		acc := makeAccount(true, 10, 5)
		assert.Equal(t, AffinityZoneYellow, acc.GetAffinityZone(15))
		assert.Equal(t, AffinityZoneRed, acc.GetAffinityZone(16))
	})
}

// ===========================================================================
// classifyByAffinityZone 测试
// ===========================================================================

func TestClassifyByAffinityZone(t *testing.T) {
	makeAWL := func(id int64, base int, buffer any, count int64) accountWithLoad {
		extra := map[string]any{"client_affinity_enabled": true}
		if base > 0 {
			extra["affinity_base"] = base
		}
		if buffer != nil {
			extra["affinity_buffer"] = buffer
		}
		return accountWithLoad{
			account:       &Account{ID: id, Platform: PlatformAnthropic, Extra: extra},
			loadInfo:      &AccountLoadInfo{AccountID: id},
			affinityCount: count,
		}
	}

	t.Run("empty input returns empty", func(t *testing.T) {
		result := classifyByAffinityZone(nil)
		require.Empty(t, result)
	})

	t.Run("no zone config returns all", func(t *testing.T) {
		// 没有账号配置 affinity_base → 原样返回
		accs := []accountWithLoad{
			{account: newAffinityAccount(1, 50, true), loadInfo: &AccountLoadInfo{AccountID: 1}},
			{account: newAffinityAccount(2, 50, true), loadInfo: &AccountLoadInfo{AccountID: 2}},
		}
		result := classifyByAffinityZone(accs)
		require.Len(t, result, 2)
	})

	t.Run("greens preferred over yellows", func(t *testing.T) {
		accs := []accountWithLoad{
			makeAWL(1, 5, 3, 3), // green (3 ≤ 5)
			makeAWL(2, 5, 3, 7), // yellow (5 < 7 ≤ 8)
			makeAWL(3, 5, 3, 2), // green (2 ≤ 5)
		}
		result := classifyByAffinityZone(accs)
		require.Len(t, result, 2)

		ids := []int64{result[0].account.ID, result[1].account.ID}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		assert.Equal(t, []int64{1, 3}, ids)
	})

	t.Run("reds excluded", func(t *testing.T) {
		accs := []accountWithLoad{
			makeAWL(1, 5, 3, 10), // red (10 > 8)
			makeAWL(2, 5, 3, 6),  // yellow (5 < 6 ≤ 8)
			makeAWL(3, 5, 3, 9),  // red (9 > 8)
		}
		result := classifyByAffinityZone(accs)
		require.Len(t, result, 1)
		assert.Equal(t, int64(2), result[0].account.ID)
	})

	t.Run("all red returns empty", func(t *testing.T) {
		accs := []accountWithLoad{
			makeAWL(1, 5, 0, 6),  // buffer=0 → red (6 > 5)
			makeAWL(2, 5, 0, 10), // buffer=0 → red (10 > 5)
		}
		result := classifyByAffinityZone(accs)
		require.Empty(t, result)
	})

	t.Run("mixed with unconfigured accounts", func(t *testing.T) {
		// 账号 1: 配置了 base=5,buffer=3 → green(3≤5)
		// 账号 2: 未配置 base → 视为 green
		// 账号 3: 配置了 base=5,buffer=3 → red(10>8)
		accs := []accountWithLoad{
			makeAWL(1, 5, 3, 3),
			{account: newAffinityAccount(2, 50, true), loadInfo: &AccountLoadInfo{AccountID: 2}, affinityCount: 20},
			makeAWL(3, 5, 3, 10),
		}
		result := classifyByAffinityZone(accs)
		require.Len(t, result, 2)

		ids := []int64{result[0].account.ID, result[1].account.ID}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		assert.Equal(t, []int64{1, 2}, ids)
	})

	t.Run("infinite yellow never reds", func(t *testing.T) {
		// buffer 未配置 → 无限黄区，永不红区
		accs := []accountWithLoad{
			makeAWL(1, 5, nil, 100), // yellow (100 > 5, no buffer → infinite yellow)
			makeAWL(2, 5, nil, 3),   // green (3 ≤ 5)
		}
		result := classifyByAffinityZone(accs)
		// green 优先
		require.Len(t, result, 1)
		assert.Equal(t, int64(2), result[0].account.ID)
	})

	t.Run("only yellows when no greens", func(t *testing.T) {
		accs := []accountWithLoad{
			makeAWL(1, 5, nil, 10), // yellow
			makeAWL(2, 5, nil, 20), // yellow
		}
		result := classifyByAffinityZone(accs)
		require.Len(t, result, 2)
	})
}

// ===========================================================================
// GetMultiDimAffinityZone 测试
// ===========================================================================

func TestGetMultiDimAffinityZone(t *testing.T) {
	makeAccount := func(opts map[string]any) *Account {
		extra := map[string]any{"affinity_enabled": true}
		for k, v := range opts {
			extra[k] = v
		}
		return &Account{
			Platform: PlatformAnthropic,
			Extra:    extra,
		}
	}

	t.Run("user green + client green = green", func(t *testing.T) {
		acc := makeAccount(map[string]any{
			"affinity_base":        10,
			"affinity_buffer":      5,
			"affinity_user_base":   3,
			"affinity_user_buffer": 2,
		})
		assert.Equal(t, AffinityZoneGreen, acc.GetMultiDimAffinityZone(2, 5, 0))
	})

	t.Run("user green + client yellow = yellow", func(t *testing.T) {
		acc := makeAccount(map[string]any{
			"affinity_base":        10,
			"affinity_buffer":      5,
			"affinity_user_base":   3,
			"affinity_user_buffer": 2,
		})
		assert.Equal(t, AffinityZoneYellow, acc.GetMultiDimAffinityZone(2, 12, 0))
	})

	t.Run("user yellow + client green = yellow", func(t *testing.T) {
		acc := makeAccount(map[string]any{
			"affinity_base":        10,
			"affinity_buffer":      5,
			"affinity_user_base":   3,
			"affinity_user_buffer": 2,
		})
		assert.Equal(t, AffinityZoneYellow, acc.GetMultiDimAffinityZone(4, 5, 0))
	})

	t.Run("user red + client green = red", func(t *testing.T) {
		acc := makeAccount(map[string]any{
			"affinity_base":        10,
			"affinity_buffer":      5,
			"affinity_user_base":   3,
			"affinity_user_buffer": 0, // no yellow, direct red
		})
		assert.Equal(t, AffinityZoneRed, acc.GetMultiDimAffinityZone(4, 5, 0))
	})

	t.Run("user green + client red = red", func(t *testing.T) {
		acc := makeAccount(map[string]any{
			"affinity_base":        10,
			"affinity_buffer":      0, // no yellow, direct red
			"affinity_user_base":   3,
			"affinity_user_buffer": 2,
		})
		assert.Equal(t, AffinityZoneRed, acc.GetMultiDimAffinityZone(2, 11, 0))
	})

	t.Run("user_base=0 only checks client dimension", func(t *testing.T) {
		acc := makeAccount(map[string]any{
			"affinity_base":   10,
			"affinity_buffer": 5,
			// no affinity_user_base → 0 → user dimension always green
		})
		// client in yellow range
		assert.Equal(t, AffinityZoneYellow, acc.GetMultiDimAffinityZone(100, 12, 0))
	})

	t.Run("perUserCount exceeds limit = red", func(t *testing.T) {
		acc := makeAccount(map[string]any{
			"affinity_base":         10,
			"affinity_buffer":       5,
			"per_user_client_limit": 3,
		})
		// client green, user green, but perUser > limit
		assert.Equal(t, AffinityZoneRed, acc.GetMultiDimAffinityZone(1, 5, 4))
	})

	t.Run("perUserCount within limit = green", func(t *testing.T) {
		acc := makeAccount(map[string]any{
			"affinity_base":         10,
			"affinity_buffer":       5,
			"per_user_client_limit": 3,
		})
		assert.Equal(t, AffinityZoneGreen, acc.GetMultiDimAffinityZone(1, 5, 3))
	})

	t.Run("affinity disabled always green", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Extra: map[string]any{
				"affinity_enabled": false,
				"affinity_base":    1,
				"affinity_buffer":  0,
			},
		}
		assert.Equal(t, AffinityZoneGreen, acc.GetMultiDimAffinityZone(100, 100, 100))
	})
}

// ===========================================================================
// IsAffinityAllowSwitch 测试
// ===========================================================================

func TestIsAffinityAllowSwitch(t *testing.T) {
	t.Run("default true when no field in Extra", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Extra:    map[string]any{"affinity_enabled": true},
		}
		assert.True(t, acc.IsAffinityAllowSwitch())
	})

	t.Run("explicit false", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Extra:    map[string]any{"affinity_allow_switch": false},
		}
		assert.False(t, acc.IsAffinityAllowSwitch())
	})

	t.Run("Extra nil defaults to true", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Extra:    nil,
		}
		assert.True(t, acc.IsAffinityAllowSwitch())
	})

	t.Run("explicit true", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Extra:    map[string]any{"affinity_allow_switch": true},
		}
		assert.True(t, acc.IsAffinityAllowSwitch())
	})
}

// ===========================================================================
// GetPinnedUsers / IsPinnedUser 测试
// ===========================================================================

func TestGetPinnedUsers(t *testing.T) {
	t.Run("normal list", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Extra: map[string]any{
				"pinned_users": []any{float64(10), float64(20), float64(30)},
			},
		}
		result := acc.GetPinnedUsers()
		assert.Equal(t, []int64{10, 20, 30}, result)
	})

	t.Run("empty list", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Extra: map[string]any{
				"pinned_users": []any{},
			},
		}
		result := acc.GetPinnedUsers()
		assert.Nil(t, result)
	})

	t.Run("Extra nil", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Extra:    nil,
		}
		result := acc.GetPinnedUsers()
		assert.Nil(t, result)
	})

	t.Run("pinned_users not set", func(t *testing.T) {
		acc := &Account{
			Platform: PlatformAnthropic,
			Extra:    map[string]any{},
		}
		result := acc.GetPinnedUsers()
		assert.Nil(t, result)
	})
}

func TestIsPinnedUser(t *testing.T) {
	acc := &Account{
		Platform: PlatformAnthropic,
		Extra: map[string]any{
			"pinned_users": []any{float64(10), float64(20), float64(30)},
		},
	}

	t.Run("hit", func(t *testing.T) {
		assert.True(t, acc.IsPinnedUser(20))
	})

	t.Run("miss", func(t *testing.T) {
		assert.False(t, acc.IsPinnedUser(99))
	})

	t.Run("nil Extra", func(t *testing.T) {
		nilAcc := &Account{Platform: PlatformAnthropic, Extra: nil}
		assert.False(t, nilAcc.IsPinnedUser(10))
	})
}

// ===========================================================================
// Enhanced mock for integration tests (supports affinity lookups + tracking)
// ===========================================================================

type affinityIntegrationCache struct {
	mockAffinityCache
	getAffinityAccountsFunc   func(ctx context.Context, groupID int64, userID int64, clientID string, ttl time.Duration) ([]int64, error)
	updateAffinityCalls       []affinityUpdateCall
	getMultiCountFunc         func(ctx context.Context, groupID int64, accountID int64, targetUserID int64, ttl time.Duration) (int64, int64, int64, error)
	sessionBindings           map[string]int64
	getAffinityAccountsCalled bool // 标记 GetAffinityAccounts 是否已被调用，用于区分预处理/Layer 2 阶段
}

type affinityUpdateCall struct {
	groupID              int64
	userID               int64
	clientID             string
	accountID            int64
	beforeAffinityLookup bool // true 表示该调用发生在 GetAffinityAccounts 之前（预处理阶段）
}

func (c *affinityIntegrationCache) GetSessionAccountID(_ context.Context, _ int64, hash string) (int64, error) {
	if c.sessionBindings != nil {
		if id, ok := c.sessionBindings[hash]; ok {
			return id, nil
		}
	}
	return 0, errors.New("not found")
}

func (c *affinityIntegrationCache) GetAffinityAccounts(ctx context.Context, groupID int64, userID int64, clientID string, ttl time.Duration) ([]int64, error) {
	c.getAffinityAccountsCalled = true
	if c.getAffinityAccountsFunc != nil {
		return c.getAffinityAccountsFunc(ctx, groupID, userID, clientID, ttl)
	}
	return nil, nil
}

func (c *affinityIntegrationCache) UpdateAffinity(_ context.Context, groupID int64, userID int64, clientID string, accountID int64, _ time.Duration) error {
	c.updateAffinityCalls = append(c.updateAffinityCalls, affinityUpdateCall{
		groupID: groupID, userID: userID, clientID: clientID, accountID: accountID,
		beforeAffinityLookup: !c.getAffinityAccountsCalled,
	})
	return nil
}

func (c *affinityIntegrationCache) GetAffinityMultiCount(ctx context.Context, groupID int64, accountID int64, targetUserID int64, ttl time.Duration) (int64, int64, int64, error) {
	if c.getMultiCountFunc != nil {
		return c.getMultiCountFunc(ctx, groupID, accountID, targetUserID, ttl)
	}
	return 0, 0, 0, nil
}

// ===========================================================================
// TestAffinityPreprocessPinnedUsers — 验证 pinned_users 预处理逻辑
// ===========================================================================

func TestAffinityPreprocessPinnedUsers(t *testing.T) {
	ctx := context.Background()
	testUserID := int64(42)
	testClientID := "user_" + "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2" + "_account_test"

	t.Run("pinned user triggers UpdateAffinity", func(t *testing.T) {
		cache := &affinityIntegrationCache{}
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{
					ID: 1, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
					Extra: map[string]any{
						"affinity_enabled": true,
						"pinned_users":     []any{float64(42)},
					},
				},
				{
					ID: 2, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
					Extra: map[string]any{"affinity_enabled": true},
				},
			},
			accountsByID: map[int64]*Account{},
		}
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
		}

		concCache := &mockConcurrencyCache{acquireResults: map[int64]bool{1: true, 2: true}}
		concSvc := NewConcurrencyService(concCache)
		cfg := &config.Config{RunMode: config.RunModeStandard}
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: concSvc,
		}

		_, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, testClientID, testUserID)
		require.NoError(t, err)

		// 验证 UpdateAffinity 至少被调用了一次（pinned preprocess for account 1）
		found := false
		for _, call := range cache.updateAffinityCalls {
			if call.accountID == 1 && call.userID == testUserID {
				found = true
				break
			}
		}
		assert.True(t, found, "UpdateAffinity should be called for pinned account 1 with userID %d", testUserID)
	})

	t.Run("non-pinned user does not trigger UpdateAffinity preprocess", func(t *testing.T) {
		cache := &affinityIntegrationCache{
			getAffinityAccountsFunc: func(_ context.Context, _ int64, _ int64, _ string, _ time.Duration) ([]int64, error) {
				return nil, nil // 无亲和记录，直接进入 Layer 2
			},
		}

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{
					ID: 1, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
					Extra: map[string]any{
						"affinity_enabled": true,
						"pinned_users":     []any{float64(99)}, // different user
					},
				},
			},
			accountsByID: map[int64]*Account{},
		}
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
		}

		concCache := &mockConcurrencyCache{acquireResults: map[int64]bool{1: true}}
		concSvc := NewConcurrencyService(concCache)
		cfg := &config.Config{RunMode: config.RunModeStandard}
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: concSvc,
		}

		_, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, testClientID, testUserID)
		require.NoError(t, err)

		// 验证预处理阶段（beforeAffinityLookup=true）没有为 userID=42 在 account 1 上调用 UpdateAffinity
		for _, call := range cache.updateAffinityCalls {
			if call.beforeAffinityLookup && call.accountID == 1 && call.userID == testUserID {
				t.Errorf("UpdateAffinity should NOT be called in preprocess for non-pinned user %d on account %d", testUserID, call.accountID)
			}
		}
	})
}

// ===========================================================================
// TestAffinityNoSwitchError — 验证 ErrAffinityNoSwitch 行为
// ===========================================================================

func TestAffinityNoSwitchError(t *testing.T) {
	ctx := context.Background()
	testUserID := int64(42)
	testClientID := "user_" + "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2" + "_account_test"
	futureTime := time.Now().Add(1 * time.Hour)

	t.Run("affinity hit but unschedulable + allow_switch=false returns ErrAffinityNoSwitch", func(t *testing.T) {
		cache := &affinityIntegrationCache{
			getAffinityAccountsFunc: func(_ context.Context, _ int64, _ int64, _ string, _ time.Duration) ([]int64, error) {
				return []int64{1}, nil
			},
		}

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{
					ID: 1, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true,
					RateLimitResetAt: &futureTime, // rate limited = unschedulable for selection
					Concurrency:      5,
					Extra: map[string]any{
						"affinity_enabled":      true,
						"affinity_allow_switch": false,
					},
				},
				{
					ID: 2, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
				},
			},
			accountsByID: map[int64]*Account{},
		}
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
		}

		concCache := &mockConcurrencyCache{acquireResults: map[int64]bool{2: true}}
		concSvc := NewConcurrencyService(concCache)
		cfg := &config.Config{RunMode: config.RunModeStandard}
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: concSvc,
		}

		_, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, testClientID, testUserID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAffinityNoSwitch)
	})

	t.Run("affinity hit but unschedulable + allow_switch=true continues to Layer 2", func(t *testing.T) {
		cache := &affinityIntegrationCache{
			getAffinityAccountsFunc: func(_ context.Context, _ int64, _ int64, _ string, _ time.Duration) ([]int64, error) {
				return []int64{1}, nil
			},
		}

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{
					ID: 1, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true,
					RateLimitResetAt: &futureTime, // rate limited = unschedulable
					Concurrency:      5,
					Extra: map[string]any{
						"affinity_enabled":      true,
						"affinity_allow_switch": true,
					},
				},
				{
					ID: 2, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
				},
			},
			accountsByID: map[int64]*Account{},
		}
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
		}

		concCache := &mockConcurrencyCache{acquireResults: map[int64]bool{2: true}}
		concSvc := NewConcurrencyService(concCache)
		cfg := &config.Config{RunMode: config.RunModeStandard}
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: concSvc,
		}

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, testClientID, testUserID)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, int64(2), result.Account.ID, "should fall through to Layer 2 and select account 2")
	})

	t.Run("no affinity records - not affected by allow_switch", func(t *testing.T) {
		cache := &affinityIntegrationCache{} // returns nil from GetAffinityAccounts

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{
					ID: 1, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
					Extra: map[string]any{
						"affinity_enabled":      true,
						"affinity_allow_switch": false,
					},
				},
			},
			accountsByID: map[int64]*Account{},
		}
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
		}

		concCache := &mockConcurrencyCache{acquireResults: map[int64]bool{1: true}}
		concSvc := NewConcurrencyService(concCache)
		cfg := &config.Config{RunMode: config.RunModeStandard}
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: concSvc,
		}

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, testClientID, testUserID)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, int64(1), result.Account.ID, "should select normally via Layer 2")
	})

	t.Run("affinity disabled account ignores allow_switch=false", func(t *testing.T) {
		cache := &affinityIntegrationCache{
			getAffinityAccountsFunc: func(_ context.Context, _ int64, _ int64, _ string, _ time.Duration) ([]int64, error) {
				return []int64{1}, nil
			},
		}

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{
					ID: 1, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
					Extra: map[string]any{
						"affinity_enabled":      false,
						"affinity_allow_switch": false,
					},
				},
				{
					ID: 2, Platform: PlatformAnthropic, Priority: 2,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
				},
			},
			accountsByID: map[int64]*Account{},
		}
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
		}

		concCache := &mockConcurrencyCache{acquireResults: map[int64]bool{2: true}}
		concSvc := NewConcurrencyService(concCache)
		cfg := &config.Config{RunMode: config.RunModeStandard}
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: concSvc,
		}

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, testClientID, testUserID)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		assert.NotEqual(t, int64(0), result.Account.ID)
	})

	t.Run("allow_switch=false + model_unsupported returns ErrAffinityNoSwitch", func(t *testing.T) {
		cache := &affinityIntegrationCache{
			getAffinityAccountsFunc: func(_ context.Context, _ int64, _ int64, _ string, _ time.Duration) ([]int64, error) {
				return []int64{1}, nil
			},
		}

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{
					ID: 1, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
					Extra: map[string]any{
						"affinity_enabled":      true,
						"affinity_allow_switch": false,
					},
					// model_mapping 仅包含 claude-3-haiku → 请求 claude-3-5-sonnet 将被拒绝
					Credentials: map[string]any{
						"model_mapping": map[string]any{
							"claude-3-haiku-20240307": "claude-3-haiku-20240307",
						},
					},
				},
				{
					ID: 2, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
				},
			},
			accountsByID: map[int64]*Account{},
		}
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
		}

		concCache := &mockConcurrencyCache{acquireResults: map[int64]bool{2: true}}
		concSvc := NewConcurrencyService(concCache)
		cfg := &config.Config{RunMode: config.RunModeStandard}
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: concSvc,
		}

		_, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, testClientID, testUserID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAffinityNoSwitch)
	})

	t.Run("allow_switch=false + red_zone returns ErrAffinityNoSwitch", func(t *testing.T) {
		cache := &affinityIntegrationCache{
			getAffinityAccountsFunc: func(_ context.Context, _ int64, _ int64, _ string, _ time.Duration) ([]int64, error) {
				return []int64{1}, nil
			},
			getMultiCountFunc: func(_ context.Context, _ int64, _ int64, _ int64, _ time.Duration) (int64, int64, int64, error) {
				// userCount=5, clientCount=5, perUserCount=0
				// 账号 affinity_base=1, affinity_buffer=0 → clientCount(5) > base(1) + buffer(0) → 红区
				return 5, 5, 0, nil
			},
		}

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{
					ID: 1, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
					Extra: map[string]any{
						"affinity_enabled":      true,
						"affinity_allow_switch": false,
						"affinity_base":         1,
						"affinity_buffer":       0, // buffer=0 → 超过 base 直接红区
					},
				},
				{
					ID: 2, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
				},
			},
			accountsByID: map[int64]*Account{},
		}
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
		}

		concCache := &mockConcurrencyCache{acquireResults: map[int64]bool{2: true}}
		concSvc := NewConcurrencyService(concCache)
		cfg := &config.Config{RunMode: config.RunModeStandard}
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: concSvc,
		}

		_, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, testClientID, testUserID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAffinityNoSwitch)
	})

	t.Run("one_allow_switch=true overrides another allow_switch=false (一票放行)", func(t *testing.T) {
		// 账号 1: allow_switch=false + rate limited（不可调度）
		// 账号 3: allow_switch=true + rate limited（不可调度）
		// 一票放行：账号 3 允许切换 → 不阻断 → 降级到 Layer 2 选中账号 2
		cache := &affinityIntegrationCache{
			getAffinityAccountsFunc: func(_ context.Context, _ int64, _ int64, _ string, _ time.Duration) ([]int64, error) {
				return []int64{1, 3}, nil
			},
		}

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{
					ID: 1, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true,
					RateLimitResetAt: &futureTime, // rate limited
					Concurrency:      5,
					Extra: map[string]any{
						"affinity_enabled":      true,
						"affinity_allow_switch": false,
					},
				},
				{
					ID: 2, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
				},
				{
					ID: 3, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true,
					RateLimitResetAt: &futureTime, // rate limited
					Concurrency:      5,
					Extra: map[string]any{
						"affinity_enabled":      true,
						"affinity_allow_switch": true,
					},
				},
			},
			accountsByID: map[int64]*Account{},
		}
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
		}

		concCache := &mockConcurrencyCache{acquireResults: map[int64]bool{2: true}}
		concSvc := NewConcurrencyService(concCache)
		cfg := &config.Config{RunMode: config.RunModeStandard}
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: concSvc,
		}

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, testClientID, testUserID)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, int64(2), result.Account.ID, "一票放行: should fall through to Layer 2")
	})

	t.Run("excluded affinity account with allow_switch=false returns ErrAffinityNoSwitch", func(t *testing.T) {
		cache := &affinityIntegrationCache{
			getAffinityAccountsFunc: func(_ context.Context, _ int64, _ int64, _ string, _ time.Duration) ([]int64, error) {
				return []int64{1}, nil
			},
		}

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{
					ID: 1, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
					Extra: map[string]any{
						"affinity_enabled":      true,
						"affinity_allow_switch": false,
					},
				},
				{
					ID: 2, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
				},
			},
			accountsByID: map[int64]*Account{},
		}
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
		}

		concCache := &mockConcurrencyCache{acquireResults: map[int64]bool{2: true}}
		concSvc := NewConcurrencyService(concCache)
		cfg := &config.Config{RunMode: config.RunModeStandard}
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: concSvc,
		}

		excluded := map[int64]struct{}{1: {}}
		_, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", excluded, testClientID, testUserID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAffinityNoSwitch)
	})

	t.Run("disabled affinity record does not suppress sticky fallback", func(t *testing.T) {
		sessionHash := "sticky-disabled-affinity"
		cache := &affinityIntegrationCache{
			getAffinityAccountsFunc: func(_ context.Context, _ int64, _ int64, _ string, _ time.Duration) ([]int64, error) {
				return []int64{1}, nil
			},
			sessionBindings: map[string]int64{sessionHash: 2},
		}

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{
					ID: 1, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
					Extra: map[string]any{
						"affinity_enabled": false,
					},
				},
				{
					ID: 2, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
				},
			},
			accountsByID: map[int64]*Account{},
		}
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
		}

		concCache := &mockConcurrencyCache{acquireResults: map[int64]bool{2: true}}
		concSvc := NewConcurrencyService(concCache)
		cfg := &config.Config{RunMode: config.RunModeStandard}
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: concSvc,
		}

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, sessionHash, "claude-3-5-sonnet-20241022", nil, testClientID, testUserID)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		assert.Equal(t, int64(2), result.Account.ID)
	})

	t.Run("historical affinity record with disabled account still falls back to sticky account", func(t *testing.T) {
		sessionHash := "sticky-disabled-status-affinity"
		cache := &affinityIntegrationCache{
			getAffinityAccountsFunc: func(_ context.Context, _ int64, _ int64, _ string, _ time.Duration) ([]int64, error) {
				return []int64{1}, nil
			},
			sessionBindings: map[string]int64{sessionHash: 2},
		}

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{
					ID: 1, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusDisabled, Schedulable: true, Concurrency: 5,
					Extra: map[string]any{
						"affinity_enabled": true,
					},
				},
				{
					ID: 2, Platform: PlatformAnthropic, Priority: 1,
					Status: StatusActive, Schedulable: true, Concurrency: 5,
				},
			},
			accountsByID: map[int64]*Account{},
		}
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
		}

		concCache := &mockConcurrencyCache{acquireResults: map[int64]bool{2: true}}
		concSvc := NewConcurrencyService(concCache)
		cfg := &config.Config{RunMode: config.RunModeStandard}
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: concSvc,
		}

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, sessionHash, "claude-3-5-sonnet-20241022", nil, testClientID, testUserID)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		assert.Equal(t, int64(2), result.Account.ID)
	})
}

// ===========================================================================
// TestAffinityMultiDimClassify — 验证 classifyByAffinityZone 与多维度计数
// ===========================================================================

func TestAffinityMultiDimClassify(t *testing.T) {
	t.Run("multi-dim zone classification with user and client dimensions", func(t *testing.T) {
		makeAWL := func(id int64, extra map[string]any, count int64) accountWithLoad {
			return accountWithLoad{
				account:       &Account{ID: id, Platform: PlatformAnthropic, Extra: extra},
				loadInfo:      &AccountLoadInfo{AccountID: id},
				affinityCount: count,
			}
		}

		// Account 1: client green (3 <= 5), classified as green by old classifyByAffinityZone
		// Account 2: client red (10 > 8), classified as red
		accs := []accountWithLoad{
			makeAWL(1, map[string]any{
				"affinity_enabled": true,
				"affinity_base":    5,
				"affinity_buffer":  3,
			}, 3),
			makeAWL(2, map[string]any{
				"affinity_enabled": true,
				"affinity_base":    5,
				"affinity_buffer":  3,
			}, 10),
		}

		result := classifyByAffinityZone(accs)
		require.Len(t, result, 1)
		assert.Equal(t, int64(1), result[0].account.ID, "only green account should remain")
	})

	t.Run("all green returns all", func(t *testing.T) {
		makeAWL := func(id int64, count int64) accountWithLoad {
			return accountWithLoad{
				account: &Account{
					ID:       id,
					Platform: PlatformAnthropic,
					Extra: map[string]any{
						"affinity_enabled": true,
						"affinity_base":    10,
						"affinity_buffer":  5,
					},
				},
				loadInfo:      &AccountLoadInfo{AccountID: id},
				affinityCount: count,
			}
		}

		accs := []accountWithLoad{
			makeAWL(1, 3),
			makeAWL(2, 5),
			makeAWL(3, 10), // exactly at base
		}

		result := classifyByAffinityZone(accs)
		require.Len(t, result, 3)
	})
}

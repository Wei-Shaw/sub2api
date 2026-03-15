//go:build unit

package service

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

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
func (m *mockAffinityCache) GetClientAffinityAccounts(_ context.Context, _ int64, _ string, _ time.Duration) ([]int64, error) {
	return nil, nil
}
func (m *mockAffinityCache) UpdateClientAffinity(_ context.Context, _ int64, _ string, _ int64, _ time.Duration) error {
	return nil
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

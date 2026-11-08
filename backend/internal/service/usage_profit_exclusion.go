package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// ==================== 键名 ====================

// UsageProfitExcludeStateKey 是「不计入盈亏的用户名单」在键值状态表里的键。
//
// 与 A6 凭据那几个键同住 reconciliation_sync_state：那张表虽然名字带
// reconciliation（历史模块遗留），但迁移 247 已明确把它保留为「运行时覆盖值」
// 的通用存放处，新增经营口径的开关放这里与既有约定一致。
//
// 键名是接口契约的一部分（运维可能直接用 SQL 查这一行），不要改名。
const UsageProfitExcludeStateKey = "usage_profit_exclude_user_ids"

// usageProfitExclusionLogComponent 本文件所有日志的组件名。
const usageProfitExclusionLogComponent = "service.usage_profit_exclusion"

// usageProfitExclusionCacheTTL 是名单的内存缓存时长。
//
// 使用记录页每次刷新都会问一次名单，若不加缓存就是每个请求一条 SELECT。
// 30 秒是一个刻意偏短的值：改完名单后最多半分钟全站生效，既不会让管理员
// 反复刷新却看不到变化，也足以把高频查询打库的次数压到可忽略。
const usageProfitExclusionCacheTTL = 30 * time.Second

// ==================== 错误 ====================

var (
	// ErrUsageProfitExclusionStoreUnavailable 覆盖值存储缺席，无法确认排除名单。
	//
	// 这里刻意**不**降级成「按空名单继续算」：空名单意味着把内部人员的虚假收入
	// 也算进毛利，图上只会显示一个偏高的利润，看不出任何异常。宁可让这次查询
	// 报错，也不要发一个悄悄偏高的经营数字出去。
	ErrUsageProfitExclusionStoreUnavailable = errors.New("USAGE_PROFIT_EXCLUSION_STORE_UNAVAILABLE")

	// ErrUsageProfitExclusionUnsupported 表示当前查询路径承载不了排除名单。
	//
	// 目前只有一个场景会用到：某条趋势查询回退到了「不带 filters 的旧签名」，
	// 而那个签名没有传递排除名单的位置。真实仓库实现了带 filters 的方法，所以
	// 正常走不到；但一旦走到，继续算下去就会悄悄按「未排除」出数，也就是发出一个
	// 偏高的利润。与存储不可用同理——宁可这次失败，也不要给一个看不出错的错数字。
	ErrUsageProfitExclusionUnsupported = errors.New("USAGE_PROFIT_EXCLUSION_UNSUPPORTED_ON_QUERY_PATH")
)

// ==================== 视图 ====================

// UsageProfitExclusionView 是排除名单的对外视图。
type UsageProfitExclusionView struct {
	// UserIDs 当前的排除名单，升序去重，永不为 nil（前端直接遍历）。
	UserIDs []int64
}

// ==================== 服务 ====================

// UsageProfitExclusionService 读写「不计入盈亏的用户名单」。
//
// 语义（务必与查询侧保持一致）：名单里的用户，其**收入**不计入盈亏统计，
// 其**成本**仍然计入。这不是「把这些人的记录整行排除」——内部人员的余额由
// 管理员手工调整、没有真实付款，所以收入是假的；但他们消耗掉的上游额度是
// 真金白银，成本必须照实算。整行排除会把真实成本一起抹掉，让毛利虚高。
type UsageProfitExclusionService struct {
	stateRepo ReconciliationSyncStateRepository

	ttl time.Duration

	mu       sync.RWMutex
	cached   []int64
	cachedAt time.Time
	loaded   bool
}

// NewUsageProfitExclusionService 创建排除名单服务。
func NewUsageProfitExclusionService(stateRepo ReconciliationSyncStateRepository) *UsageProfitExclusionService {
	return &UsageProfitExclusionService{stateRepo: stateRepo, ttl: usageProfitExclusionCacheTTL}
}

// ExcludedUserIDs 返回当前生效的排除名单（只读，调用方不得修改返回值）。
//
// 读失败时返回错误而不是空名单：见 ErrUsageProfitExclusionStoreUnavailable 的说明。
// 若曾经成功读到过，则退回过期缓存并把失败记为 ERROR —— 已经生效过的经营口径
// 不应该因为一次读抖动就悄悄退回「未排除」。
func (s *UsageProfitExclusionService) ExcludedUserIDs(ctx context.Context) ([]int64, error) {
	if s == nil || s.stateRepo == nil {
		return nil, ErrUsageProfitExclusionStoreUnavailable
	}

	s.mu.RLock()
	if s.loaded && time.Since(s.cachedAt) < s.ttl {
		cached := s.cached
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	ids, err := s.load(ctx)
	if err != nil {
		s.mu.RLock()
		stale, hadLoaded := s.cached, s.loaded
		s.mu.RUnlock()
		if hadLoaded {
			logger.LegacyPrintf(usageProfitExclusionLogComponent,
				"exclude_list_read_failed_using_stale: err=%v", err)
			return stale, nil
		}
		return nil, err
	}

	s.mu.Lock()
	s.cached = ids
	s.cachedAt = time.Now()
	s.loaded = true
	s.mu.Unlock()
	return ids, nil
}

// Effective 返回对外视图。
func (s *UsageProfitExclusionService) Effective(ctx context.Context) (UsageProfitExclusionView, error) {
	ids, err := s.ExcludedUserIDs(ctx)
	if err != nil {
		return UsageProfitExclusionView{}, err
	}
	return UsageProfitExclusionView{UserIDs: cloneInt64Slice(ids)}, nil
}

// Update 覆盖排除名单并立即失效缓存。
//
// userIDs 里的 0 与负数会被丢弃（用户 ID 从 1 开始，0 通常来自前端
// 「未选择」的占位值），随后去重升序落库。
func (s *UsageProfitExclusionService) Update(ctx context.Context, userIDs []int64) (UsageProfitExclusionView, error) {
	if s == nil || s.stateRepo == nil {
		return UsageProfitExclusionView{}, ErrUsageProfitExclusionStoreUnavailable
	}

	normalized := normalizeProfitExcludedUserIDs(userIDs)
	payload, err := json.Marshal(normalized)
	if err != nil {
		return UsageProfitExclusionView{}, err
	}
	if err := s.stateRepo.Set(ctx, UsageProfitExcludeStateKey, string(payload)); err != nil {
		return UsageProfitExclusionView{}, err
	}

	s.mu.Lock()
	s.cached = normalized
	s.cachedAt = time.Now()
	s.loaded = true
	s.mu.Unlock()

	return UsageProfitExclusionView{UserIDs: cloneInt64Slice(normalized)}, nil
}

// load 从键值表读出名单。
//
// 键不存在、值为空串都表示「没有排除任何人」，是正常状态而非错误。
// 值被写坏（不是合法 JSON 数组）时返回错误：这种情况说明有人手工改过库，
// 与其猜一个数字，不如让管理员看到明确的失败。
func (s *UsageProfitExclusionService) load(ctx context.Context) ([]int64, error) {
	raw, err := s.stateRepo.Get(ctx, UsageProfitExcludeStateKey)
	if err != nil {
		return nil, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []int64{}, nil
	}

	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		logger.LegacyPrintf(usageProfitExclusionLogComponent,
			"exclude_list_invalid_json: err=%v value_length=%d", err, len(raw))
		return nil, err
	}
	return normalizeProfitExcludedUserIDs(ids), nil
}

// normalizeProfitExcludedUserIDs 丢弃非法 ID、去重并升序，返回的切片永不为 nil。
func normalizeProfitExcludedUserIDs(userIDs []int64) []int64 {
	seen := make(map[int64]struct{}, len(userIDs))
	out := make([]int64, 0, len(userIDs))
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// cloneInt64Slice 复制一份，避免调用方改到缓存里的切片。
func cloneInt64Slice(values []int64) []int64 {
	if values == nil {
		return []int64{}
	}
	out := make([]int64, len(values))
	copy(out, values)
	return out
}

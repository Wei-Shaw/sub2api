package service

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// ErrorPassthroughRepository 定义错误透传规则的数据访问接口
type ErrorPassthroughRepository interface {
	// List 获取所有规则
	List(ctx context.Context) ([]*model.ErrorPassthroughRule, error)
	// GetByID 根据 ID 获取规则
	GetByID(ctx context.Context, id int64) (*model.ErrorPassthroughRule, error)
	// Create 创建规则
	Create(ctx context.Context, rule *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error)
	// Update 更新规则
	Update(ctx context.Context, rule *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error)
	// Delete 删除规则
	Delete(ctx context.Context, id int64) error
}

// ErrorPassthroughCache 定义错误透传规则的缓存接口
type ErrorPassthroughCache interface {
	// Get 从缓存获取规则列表
	Get(ctx context.Context) ([]*model.ErrorPassthroughRule, bool)
	// Set 设置缓存
	Set(ctx context.Context, rules []*model.ErrorPassthroughRule) error
	// Invalidate 使缓存失效
	Invalidate(ctx context.Context) error
	// NotifyUpdate 通知其他实例刷新缓存
	NotifyUpdate(ctx context.Context) error
	// SubscribeUpdates 订阅缓存更新通知
	SubscribeUpdates(ctx context.Context, handler func())
}

// ErrorPassthroughService 错误透传规则服务
type ErrorPassthroughService struct {
	repo  ErrorPassthroughRepository
	cache ErrorPassthroughCache

	// 本地内存缓存，用于快速匹配
	localCache   []*cachedPassthroughRule
	localCacheMu sync.RWMutex
}

// cachedPassthroughRule 预计算的规则缓存，避免运行时重复 ToLower
type cachedPassthroughRule struct {
	*model.ErrorPassthroughRule
	lowerKeywords  []string         // 预计算的小写关键词
	lowerPlatforms []string         // 预计算的小写平台
	errorCodeSet   map[int]struct{} // 预计算的 error code set
}

const maxBodyMatchLen = 8 << 10 // 8KB，错误信息不会在 8KB 之后才出现

// NewErrorPassthroughService 创建错误透传规则服务
func NewErrorPassthroughService(
	repo ErrorPassthroughRepository,
	cache ErrorPassthroughCache,
) *ErrorPassthroughService {
	svc := &ErrorPassthroughService{
		repo:  repo,
		cache: cache,
	}

	// 启动时加载规则到本地缓存
	ctx := context.Background()
	if err := svc.reloadRulesFromDB(ctx); err != nil {
		logger.LegacyPrintf("service.error_passthrough", "[ErrorPassthroughService] Failed to load rules from DB on startup: %v", err)
		if fallbackErr := svc.refreshLocalCache(ctx); fallbackErr != nil {
			logger.LegacyPrintf("service.error_passthrough", "[ErrorPassthroughService] Failed to load rules from cache fallback on startup: %v", fallbackErr)
		}
	}

	// 订阅缓存更新通知
	if cache != nil {
		cache.SubscribeUpdates(ctx, func() {
			if err := svc.refreshLocalCache(context.Background()); err != nil {
				logger.LegacyPrintf("service.error_passthrough", "[ErrorPassthroughService] Failed to refresh cache on notification: %v", err)
			}
		})
	}

	return svc
}

// List 获取所有规则
func (s *ErrorPassthroughService) List(ctx context.Context) ([]*model.ErrorPassthroughRule, error) {
	return s.repo.List(ctx)
}

// GetByID 根据 ID 获取规则
func (s *ErrorPassthroughService) GetByID(ctx context.Context, id int64) (*model.ErrorPassthroughRule, error) {
	return s.repo.GetByID(ctx, id)
}

// Create 创建规则
func (s *ErrorPassthroughService) Create(ctx context.Context, rule *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	if err := rule.Validate(); err != nil {
		return nil, err
	}

	created, err := s.repo.Create(ctx, rule)
	if err != nil {
		return nil, err
	}

	// 刷新缓存
	refreshCtx, cancel := s.newCacheRefreshContext()
	defer cancel()
	s.invalidateAndNotify(refreshCtx)

	return created, nil
}

// Update 更新规则
func (s *ErrorPassthroughService) Update(ctx context.Context, rule *model.ErrorPassthroughRule) (*model.ErrorPassthroughRule, error) {
	if err := rule.Validate(); err != nil {
		return nil, err
	}

	updated, err := s.repo.Update(ctx, rule)
	if err != nil {
		return nil, err
	}

	// 刷新缓存
	refreshCtx, cancel := s.newCacheRefreshContext()
	defer cancel()
	s.invalidateAndNotify(refreshCtx)

	return updated, nil
}

// Delete 删除规则
func (s *ErrorPassthroughService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	// 刷新缓存
	refreshCtx, cancel := s.newCacheRefreshContext()
	defer cancel()
	s.invalidateAndNotify(refreshCtx)

	return nil
}

// MatchRule 匹配透传规则
// 返回第一个匹配的规则，如果没有匹配则返回 nil
func (s *ErrorPassthroughService) MatchRule(platform string, statusCode int, body []byte) *model.ErrorPassthroughRule {
	rules := s.getCachedRules()
	if len(rules) == 0 {
		return nil
	}

	lowerPlatform := strings.ToLower(platform)
	var bodyLower string // 延迟初始化，只在需要关键词匹配时计算
	var bodyLowerDone bool

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if !s.platformMatchesCached(rule, lowerPlatform) {
			continue
		}
		if s.ruleMatchesOptimized(rule, statusCode, body, &bodyLower, &bodyLowerDone) {
			return rule.ErrorPassthroughRule
		}
	}

	return nil
}

// getCachedRules 获取缓存的规则列表（按优先级排序）
func (s *ErrorPassthroughService) getCachedRules() []*cachedPassthroughRule {
	s.localCacheMu.RLock()
	rules := s.localCache
	s.localCacheMu.RUnlock()

	if rules != nil {
		return rules
	}

	// 如果本地缓存为空，尝试刷新
	ctx := context.Background()
	if err := s.refreshLocalCache(ctx); err != nil {
		logger.LegacyPrintf("service.error_passthrough", "[ErrorPassthroughService] Failed to refresh cache: %v", err)
		return nil
	}

	s.localCacheMu.RLock()
	defer s.localCacheMu.RUnlock()
	return s.localCache
}

// refreshLocalCache 刷新本地缓存
func (s *ErrorPassthroughService) refreshLocalCache(ctx context.Context) error {
	// 先尝试从 Redis 缓存获取
	if s.cache != nil {
		if rules, ok := s.cache.Get(ctx); ok {
			s.setLocalCache(rules)
			return nil
		}
	}

	return s.reloadRulesFromDB(ctx)
}

// 从数据库加载（repo.List 已按 priority 排序）
// 注意：该方法会绕过 cache.Get，确保拿到数据库最新值。
func (s *ErrorPassthroughService) reloadRulesFromDB(ctx context.Context) error {
	rules, err := s.repo.List(ctx)
	if err != nil {
		return err
	}

	// 更新 Redis 缓存
	if s.cache != nil {
		if err := s.cache.Set(ctx, rules); err != nil {
			logger.LegacyPrintf("service.error_passthrough", "[ErrorPassthroughService] Failed to set cache: %v", err)
		}
	}

	// 更新本地缓存（setLocalCache 内部会确保排序）
	s.setLocalCache(rules)

	return nil
}

// setLocalCache 设置本地缓存，预计算小写值和 set 以避免运行时重复计算
func (s *ErrorPassthroughService) setLocalCache(rules []*model.ErrorPassthroughRule) {
	cached := make([]*cachedPassthroughRule, len(rules))
	for i, r := range rules {
		cr := &cachedPassthroughRule{ErrorPassthroughRule: r}
		if len(r.Keywords) > 0 {
			cr.lowerKeywords = make([]string, len(r.Keywords))
			for j, kw := range r.Keywords {
				cr.lowerKeywords[j] = strings.ToLower(kw)
			}
		}
		if len(r.Platforms) > 0 {
			cr.lowerPlatforms = make([]string, len(r.Platforms))
			for j, p := range r.Platforms {
				cr.lowerPlatforms[j] = strings.ToLower(p)
			}
		}
		if len(r.ErrorCodes) > 0 {
			cr.errorCodeSet = make(map[int]struct{}, len(r.ErrorCodes))
			for _, code := range r.ErrorCodes {
				cr.errorCodeSet[code] = struct{}{}
			}
		}
		cached[i] = cr
	}

	// 按优先级排序
	sort.Slice(cached, func(i, j int) bool {
		return cached[i].Priority < cached[j].Priority
	})

	s.localCacheMu.Lock()
	s.localCache = cached
	s.localCacheMu.Unlock()
}

// clearLocalCache 清空本地缓存，避免刷新失败时继续命中陈旧规则。
func (s *ErrorPassthroughService) clearLocalCache() {
	s.localCacheMu.Lock()
	s.localCache = nil
	s.localCacheMu.Unlock()
}

// newCacheRefreshContext 为写路径缓存同步创建独立上下文，避免受请求取消影响。
func (s *ErrorPassthroughService) newCacheRefreshContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}

// invalidateAndNotify 使缓存失效并通知其他实例
func (s *ErrorPassthroughService) invalidateAndNotify(ctx context.Context) {
	// 先失效缓存，避免后续刷新读到陈旧规则。
	if s.cache != nil {
		if err := s.cache.Invalidate(ctx); err != nil {
			logger.LegacyPrintf("service.error_passthrough", "[ErrorPassthroughService] Failed to invalidate cache: %v", err)
		}
	}

	// 刷新本地缓存
	if err := s.reloadRulesFromDB(ctx); err != nil {
		logger.LegacyPrintf("service.error_passthrough", "[ErrorPassthroughService] Failed to refresh local cache: %v", err)
		// 刷新失败时清空本地缓存，避免继续使用陈旧规则。
		s.clearLocalCache()
	}

	// 通知其他实例
	if s.cache != nil {
		if err := s.cache.NotifyUpdate(ctx); err != nil {
			logger.LegacyPrintf("service.error_passthrough", "[ErrorPassthroughService] Failed to notify cache update: %v", err)
		}
	}
}

// ensureBodyLower 延迟初始化 body 的小写版本，只做一次转换，限制 8KB
func ensureBodyLower(body []byte, bodyLower *string, done *bool) string {
	if *done {
		return *bodyLower
	}
	b := body
	if len(b) > maxBodyMatchLen {
		b = b[:maxBodyMatchLen]
	}
	*bodyLower = strings.ToLower(string(b))
	*done = true
	return *bodyLower
}

// platformMatchesCached 使用预计算的小写平台检查是否匹配
func (s *ErrorPassthroughService) platformMatchesCached(rule *cachedPassthroughRule, lowerPlatform string) bool {
	if len(rule.lowerPlatforms) == 0 {
		return true
	}
	for _, p := range rule.lowerPlatforms {
		if p == lowerPlatform {
			return true
		}
	}
	return false
}

// ruleMatchesOptimized 优化的规则匹配，支持短路和延迟 body 转换
func (s *ErrorPassthroughService) ruleMatchesOptimized(rule *cachedPassthroughRule, statusCode int, body []byte, bodyLower *string, bodyLowerDone *bool) bool {
	hasErrorCodes := len(rule.errorCodeSet) > 0
	hasKeywords := len(rule.lowerKeywords) > 0

	if !hasErrorCodes && !hasKeywords {
		return false
	}

	codeMatch := !hasErrorCodes || s.containsIntSet(rule.errorCodeSet, statusCode)

	if rule.MatchMode == model.MatchModeAll {
		// "all" 模式：所有配置的条件都必须满足，短路
		if hasErrorCodes && !codeMatch {
			return false
		}
		if hasKeywords {
			return s.containsAnyKeywordCached(ensureBodyLower(body, bodyLower, bodyLowerDone), rule.lowerKeywords)
		}
		return codeMatch
	}

	// "any" 模式：任一条件满足即可，短路
	if hasErrorCodes && hasKeywords {
		if codeMatch {
			return true
		}
		return s.containsAnyKeywordCached(ensureBodyLower(body, bodyLower, bodyLowerDone), rule.lowerKeywords)
	}
	// 只配置了一种条件
	if hasKeywords {
		return s.containsAnyKeywordCached(ensureBodyLower(body, bodyLower, bodyLowerDone), rule.lowerKeywords)
	}
	return codeMatch
}

// containsIntSet 使用 map 查找替代线性扫描
func (s *ErrorPassthroughService) containsIntSet(set map[int]struct{}, val int) bool {
	_, ok := set[val]
	return ok
}

// containsAnyKeywordCached 使用预计算的小写关键词检查匹配
func (s *ErrorPassthroughService) containsAnyKeywordCached(bodyLower string, lowerKeywords []string) bool {
	for _, kw := range lowerKeywords {
		if strings.Contains(bodyLower, kw) {
			return true
		}
	}
	return false
}

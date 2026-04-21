package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// QuotaService 接口与相关 DTO 在 quota_types.go；校验 helper 在 quota_rule_validator.go。

// quotaSettingsProvider 读 quota 相关 setting 的子接口（便于测试替换）。
// 目前只需读 bool（全局开关、默认启用）；default_daily_usage_limit_usd 在
// 用户创建流程直接走 *SettingService.GetDefaultDailyUsageLimitUSD，不经此接口。
type quotaSettingsProvider interface {
	GetBool(ctx context.Context, key string) (bool, error)
}

// QuotaService 默认实现
type quotaService struct {
	ruleRepo   UserUsageLimitRuleRepository
	userRepo   UserRepository
	userWriter QuotaUserWriter
	groupRepo  GroupRepository
	settings   quotaSettingsProvider
	cache      QuotaCache // 可能为 nil，不可依赖（ISP：仅依赖配额子接口，非完整 BillingCache）
}

// NewQuotaService 构造 QuotaService
func NewQuotaService(
	ruleRepo UserUsageLimitRuleRepository,
	userRepo UserRepository,
	userWriter QuotaUserWriter,
	groupRepo GroupRepository,
	settings quotaSettingsProvider,
	cache QuotaCache,
) QuotaService {
	return &quotaService{
		ruleRepo:   ruleRepo,
		userRepo:   userRepo,
		userWriter: userWriter,
		groupRepo:  groupRepo,
		settings:   settings,
		cache:      cache,
	}
}

// ---- Resolve + MatchRule ----

// Resolve 合并全局默认 + 用户覆盖 + rules，返回"今天生效"的配额快照。
// 任何错误都返回 Enabled=false（失败安全）但仍返回非 nil 快照与 error，供上游区分失败和"未启用"。
//
// 不可拆分（~42 行 > CLAUDE.md §10 的 30 行阈值）：函数体是 5 段 early-return 流水线
// （user_id 校验 → 全局开关 → 用户存在 → 启用判定 → 限额/规则填充），每段 3-9 行。
// 规则加载和启用回退已抽至 loadDailyRules / resolveUserEnabled；剩余各段继续拆会让
// ResolvedQuota 的逐步填充变成多返回值编排，降低可读性。
func (s *quotaService) Resolve(ctx context.Context, userID int64) (*ResolvedQuota, error) {
	if userID <= 0 {
		return &ResolvedQuota{UserID: userID, Enabled: false, ResolvedAt: timezone.Now()}, nil
	}

	globalEnabled, err := s.settings.GetBool(ctx, SettingKeyUsageLimitEnabled)
	if err != nil {
		return nil, fmt.Errorf("quota settings usage_limit_enabled: %w", err)
	}
	resolved := &ResolvedQuota{UserID: userID, ResolvedAt: timezone.Now(), Rules: []QuotaRule{}}
	if !globalEnabled {
		resolved.Enabled = false
		return resolved, nil
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			resolved.Enabled = false
			return resolved, nil
		}
		return nil, fmt.Errorf("quota load user: %w", err)
	}
	enabled, err := s.resolveUserEnabled(ctx, user.UsageLimitEnabled)
	if err != nil {
		return nil, err
	}
	resolved.Enabled = enabled
	if !enabled {
		return resolved, nil
	}

	if user.DailyUsageLimitUSD != nil && *user.DailyUsageLimitUSD > 0 {
		v := *user.DailyUsageLimitUSD
		resolved.DailyLimit = &v
	}

	rules, err := s.loadDailyRules(ctx, userID)
	if err != nil {
		return nil, err
	}
	resolved.Rules = rules
	return resolved, nil
}

// loadDailyRules 拉取并过滤用户的每日规则（period == daily）。
// 其它 period 值目前不合法，过滤掉以防御 DB 侧异常数据。
func (s *quotaService) loadDailyRules(ctx context.Context, userID int64) ([]QuotaRule, error) {
	rules, err := s.ruleRepo.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("quota list rules: %w", err)
	}
	out := make([]QuotaRule, 0, len(rules))
	for _, r := range rules {
		if r == nil || r.Period != QuotaPeriodDaily {
			continue
		}
		out = append(out, *r)
	}
	return out, nil
}

func (s *quotaService) resolveUserEnabled(ctx context.Context, override *bool) (bool, error) {
	if override != nil {
		return *override, nil
	}
	v, err := s.settings.GetBool(ctx, SettingKeyDefaultUsageLimitEnabled)
	if err != nil {
		return false, fmt.Errorf("quota settings default_usage_limit_enabled: %w", err)
	}
	return v, nil
}

// MatchRule 返回命中 groupID 的规则；至多一条（规则间禁止重叠）。
func (s *quotaService) MatchRule(resolved *ResolvedQuota, groupID int64) *QuotaRule {
	if resolved == nil || !resolved.Enabled || groupID <= 0 {
		return nil
	}
	for i := range resolved.Rules {
		rule := &resolved.Rules[i]
		for _, gid := range rule.GroupIDs {
			if gid == groupID {
				return rule
			}
		}
	}
	return nil
}

// ---- Admin 面：用户配额视图 + 更新 ----

// GetUserQuota 返回 Admin 侧 UserQuotaView（override + resolved + today_usage）
func (s *quotaService) GetUserQuota(ctx context.Context, userID int64) (*UserQuotaView, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	resolved, err := s.Resolve(ctx, userID)
	if err != nil {
		return nil, err
	}
	usage, err := s.GetTodayUsage(ctx, userID, resolved)
	if err != nil {
		return nil, err
	}
	view := &UserQuotaView{
		UserOverride: UserQuotaOverride{
			UsageLimitEnabled:  user.UsageLimitEnabled,
			DailyUsageLimitUSD: user.DailyUsageLimitUSD,
		},
		Resolved:   resolved,
		TodayUsage: usage,
	}
	return view, nil
}

// UpdateUserQuota 按契约 §2.2 的双指针语义更新用户配额
func (s *quotaService) UpdateUserQuota(ctx context.Context, userID int64, req UpdateUserQuotaRequest) error {
	current, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	enabled := current.UsageLimitEnabled
	if req.UsageLimitEnabled != nil {
		enabled = *req.UsageLimitEnabled
	}
	limit := current.DailyUsageLimitUSD
	if req.DailyUsageLimitUSD != nil {
		limit = *req.DailyUsageLimitUSD
	}
	if limit != nil && *limit < 0 {
		return infraerrors.BadRequest("QUOTA_LIMIT_NEGATIVE", "daily usage limit must be >= 0")
	}
	// 归一化：0 视同"不限"，直接清空为 NULL。保持 DB 列只有 {NULL / 正值} 两态，
	// 避免下游读到 "*float64 非 nil 但 == 0" 的半中间态。
	if limit != nil && *limit == 0 {
		limit = nil
	}

	if err := s.userWriter.UpdateUsageLimit(ctx, userID, enabled, limit); err != nil {
		return err
	}
	s.invalidateQuotaConfig(ctx, userID)
	return nil
}

// ---- Admin 面：规则 CRUD ----

func (s *quotaService) ListRules(ctx context.Context, userID int64) ([]*QuotaRule, error) {
	return s.ruleRepo.ListByUser(ctx, userID)
}

func (s *quotaService) CreateRule(ctx context.Context, userID int64, req CreateRuleRequest) (*QuotaRule, error) {
	normalized, err := s.validateAndNormalizeRule(ctx, userID, 0, req.GroupIDs, req.DailyLimitUSD, req.Period)
	if err != nil {
		return nil, err
	}
	created, err := s.ruleRepo.Create(ctx, userID, CreateRuleRequest{
		GroupIDs:      normalized.groupIDs,
		DailyLimitUSD: normalized.dailyLimit,
		Period:        normalized.period,
	})
	if err != nil {
		return nil, err
	}
	s.invalidateQuotaConfig(ctx, userID)
	return created, nil
}

func (s *quotaService) UpdateRule(ctx context.Context, userID, ruleID int64, req UpdateRuleRequest) (*QuotaRule, error) {
	existing, err := s.ruleRepo.GetByIDForUser(ctx, userID, ruleID)
	if err != nil {
		return nil, err
	}

	groupIDs := existing.GroupIDs
	if req.GroupIDs != nil {
		groupIDs = *req.GroupIDs
	}
	limit := existing.DailyLimitUSD
	if req.DailyLimitUSD != nil {
		limit = *req.DailyLimitUSD
	}

	normalized, err := s.validateAndNormalizeRule(ctx, userID, ruleID, groupIDs, limit, existing.Period)
	if err != nil {
		return nil, err
	}
	normalizedIDs := normalized.groupIDs
	normalizedLimit := normalized.dailyLimit
	updated, err := s.ruleRepo.Update(ctx, userID, ruleID, UpdateRuleRequest{
		GroupIDs:      &normalizedIDs,
		DailyLimitUSD: &normalizedLimit,
	})
	if err != nil {
		return nil, err
	}
	s.invalidateQuotaConfig(ctx, userID)
	return updated, nil
}

func (s *quotaService) DeleteRule(ctx context.Context, userID, ruleID int64) error {
	if err := s.ruleRepo.Delete(ctx, userID, ruleID); err != nil {
		return err
	}
	s.invalidateQuotaConfig(ctx, userID)
	return nil
}

// ReplaceUserRules 全量替换用户规则（单事务）。
//
// 步骤：
//  1. 应用层逐条校验 + 归一化（limit>0、period==daily、分组存在且非订阅）
//  2. 校验批次内部 group_ids 不重叠（注意：同批次两条规则不能共享分组）
//  3. 调用 repo.ReplaceAll 在单事务内 DELETE 旧规则 + 批量 INSERT
//  4. 成功后失效配置缓存
//
// 注意：批次内部的重叠校验与 validateAndNormalizeRule 的跨规则重叠校验互补
// （后者看已落库的规则，这里看本次批次内部）。
//
// 不可拆分（~32 行 > 30 行阈值）：逐条校验 + 批次内重叠扫描 + 事务替换是线性编排，
// 拆出子函数会割裂"校验失败立即返回、成功再进事务"的原子流程。
func (s *quotaService) ReplaceUserRules(ctx context.Context, userID int64, rules []CreateRuleRequest) ([]*QuotaRule, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("QUOTA_USER_INVALID", "invalid user id")
	}

	normalized := make([]CreateRuleRequest, 0, len(rules))
	seen := make(map[int64]int, 0) // group_id → index 用于批次内重叠检测
	for i, input := range rules {
		n, err := s.normalizeRuleForReplace(ctx, input)
		if err != nil {
			return nil, err
		}
		for _, gid := range n.GroupIDs {
			if prev, ok := seen[gid]; ok {
				return nil, ErrRuleGroupsOverlap.WithMetadata(map[string]string{
					"group_id":       strconv.FormatInt(gid, 10),
					"batch_index_a":  strconv.Itoa(prev),
					"batch_index_b":  strconv.Itoa(i),
					"batch_conflict": "true",
				})
			}
			seen[gid] = i
		}
		normalized = append(normalized, n)
	}

	replaced, err := s.ruleRepo.ReplaceAll(ctx, userID, normalized)
	if err != nil {
		return nil, err
	}
	s.invalidateQuotaConfig(ctx, userID)
	return replaced, nil
}

// ---- 今日用量 ----

// GetTodayUsage 汇总用户今日总用量 + 各规则用量 + 下次重置时间
// fail open：Redis 读错误回退为 0，不阻塞调用方
func (s *quotaService) GetTodayUsage(ctx context.Context, userID int64, resolved *ResolvedQuota) (*QuotaUsageSnapshot, error) {
	snap := &QuotaUsageSnapshot{
		RulesUsed: map[int64]float64{},
		ResetAt:   nextQuotaResetTime(),
	}
	if s.cache == nil {
		return snap, nil
	}
	date := quotaDateKey(timezone.Now())
	total, err := s.cache.GetQuotaUsedTotal(ctx, userID, date)
	if err == nil {
		snap.TotalUsedUSD = total
	}
	if resolved != nil {
		for _, rule := range resolved.Rules {
			used, err := s.cache.GetQuotaUsedRule(ctx, userID, rule.ID, date)
			if err != nil {
				continue
			}
			snap.RulesUsed[rule.ID] = used
		}
	}
	return snap, nil
}

func (s *quotaService) invalidateQuotaConfig(ctx context.Context, userID int64) {
	if s.cache == nil {
		return
	}
	if err := s.cache.InvalidateQuotaConfig(ctx, userID); err != nil {
		// fail open：失效失败只记告警，由 TTL 兜底
		slog.Warn("invalidate quota config failed",
			"component", "service.quota",
			"user_id", userID,
			"error", err,
		)
	}
}

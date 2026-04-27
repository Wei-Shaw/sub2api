package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/metrics"
	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/lib/pq"
	"golang.org/x/sync/singleflight"
)

type serviceQuotaService struct {
	repo     ServiceQuotaRuleRepository
	settings *SettingService
	limiter  ServiceQuotaLimiter
	cache    ServiceQuotaCache
	users    ServiceQuotaUserChecker
	sf       singleflight.Group
}

// NewServiceQuotaService 构造服务限额服务。
//
// users 用于在 Create/Update 时校验 target_user_ids 引用的用户是否存在；
// 允许传 nil（生产代码不会，但旧测试桩可能不提供），nil 时跳过用户存在性检查。
func NewServiceQuotaService(
	repo ServiceQuotaRuleRepository,
	settings *SettingService,
	limiter ServiceQuotaLimiter,
	cache ServiceQuotaCache,
	users ServiceQuotaUserChecker,
) ServiceQuotaService {
	return &serviceQuotaService{
		repo:     repo,
		settings: settings,
		limiter:  limiter,
		cache:    cache,
		users:    users,
	}
}

// matchedQuotaRule 是 PreCheck/Record 用的中间态：把规则与本次请求实际命中的 path 列表绑在一起，
// 后续按 path × limiter 做笛卡尔展开。
type matchedQuotaRule struct {
	rule  *ServiceQuotaRule
	paths []ServiceQuotaPathDef
}

func (s *serviceQuotaService) ListRules(ctx context.Context, filter ServiceQuotaListFilter) ([]*ServiceQuotaRule, error) {
	return s.repo.List(ctx, filter)
}

func (s *serviceQuotaService) CreateRule(ctx context.Context, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error) {
	if err := s.normalizeAndValidate(ctx, &input); err != nil {
		return nil, err
	}
	rule, err := s.repo.Create(ctx, input)
	if err != nil {
		return nil, translateServiceQuotaPersistenceError(err)
	}
	s.reloadCache(ctx)
	return rule, nil
}

func (s *serviceQuotaService) UpdateRule(ctx context.Context, id int64, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error) {
	if err := s.normalizeAndValidate(ctx, &input); err != nil {
		return nil, err
	}
	rule, err := s.repo.Update(ctx, id, input)
	if err != nil {
		return nil, translateServiceQuotaPersistenceError(err)
	}
	s.reloadCache(ctx)
	return rule, nil
}

func (s *serviceQuotaService) DeleteRule(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return translateServiceQuotaPersistenceError(err)
	}
	s.reloadCache(ctx)
	return nil
}

// translateServiceQuotaPersistenceError 把 repo 层透传的数据库错误翻译成业务层 ApplicationError，
// 让 handler 通过 response.ErrorFrom 自动得到正确的 HTTP 状态：
//   - sql.ErrNoRows → 404 ErrServiceQuotaRuleNotFound（Update/Delete 找不到规则）
//   - PostgreSQL 23505（unique_violation，例如 service_quota_paths 的 path 重复 / rule_users 重复）→ 409 Conflict
//   - 其他原样透传，由全局错误处理走 500
func translateServiceQuotaPersistenceError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrServiceQuotaRuleNotFound.WithCause(err)
	}
	var pgErr *pq.Error
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pkgerrors.Conflict(
			"SERVICE_QUOTA_CONFLICT",
			"service quota rule conflicts with an existing record",
		).WithMetadata(map[string]string{
			"constraint": pgErr.Constraint,
		}).WithCause(err)
	}
	return fmt.Errorf("service_quota: persistence error: %w", err)
}

func (s *serviceQuotaService) InvalidateEnabledCache(ctx context.Context) {
	if s.cache != nil {
		_ = s.cache.Invalidate(ctx)
	}
}

func (s *serviceQuotaService) normalizeAndValidate(ctx context.Context, input *ServiceQuotaRuleInput) error {
	if input == nil {
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_RULE", "invalid service quota rule")
	}
	if input.CounterMode == "" {
		input.CounterMode = ServiceQuotaCounterModePerUser
	}
	switch input.CounterMode {
	case ServiceQuotaCounterModeUser, ServiceQuotaCounterModePerUser, ServiceQuotaCounterModeShared:
	default:
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_COUNTER_MODE", "invalid counter_mode")
	}
	if input.Enabled == nil {
		enabled := true
		input.Enabled = &enabled
	}
	input.TargetUserIDs = sanitizeTargetUserIDs(input.TargetUserIDs)
	if input.CounterMode == ServiceQuotaCounterModeUser && len(input.TargetUserIDs) == 0 {
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_TARGET", "target_user_ids is required when counter_mode=user")
	}
	if input.CounterMode != ServiceQuotaCounterModeUser {
		input.TargetUserIDs = nil
	}
	if err := s.validateTargetUserIDsExist(ctx, input.TargetUserIDs); err != nil {
		return err
	}
	if err := normalizeLimiters(input); err != nil {
		return err
	}
	if err := s.normalizePaths(ctx, input); err != nil {
		return err
	}
	return nil
}

// validateTargetUserIDsExist 检查 target_user_ids 中每个 ID 在 users 表中（且未软删除）实际存在。
// 缺失任何 ID 时返回 BadRequest，并在 metadata.missing_ids 给出排序后的逗号分隔列表，
// 让前端可以高亮错误的用户输入；外部 checker 不可用时（s.users == nil）跳过校验。
//
// 调用前 ids 已经过 sanitizeTargetUserIDs 去重 + 去 <=0。
func (s *serviceQuotaService) validateTargetUserIDsExist(ctx context.Context, ids []int64) error {
	if s == nil || s.users == nil || len(ids) == 0 {
		return nil
	}
	exists, err := s.users.ExistsByIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("service_quota: validate target_user_ids: %w", err)
	}
	missing := make([]int64, 0)
	for _, id := range ids {
		if !exists[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })
	parts := make([]string, len(missing))
	for i, id := range missing {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return pkgerrors.BadRequest(
		"SERVICE_QUOTA_INVALID_TARGET_USERS",
		"some target_user_ids do not refer to existing users",
	).WithMetadata(map[string]string{
		"missing_ids": strings.Join(parts, ","),
	})
}

func normalizeLimiters(input *ServiceQuotaRuleInput) error {
	if len(input.Limiters) == 0 {
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_LIMITERS", "at least one limiter is required")
	}
	seen := make(map[string]struct{}, len(input.Limiters))
	for i := range input.Limiters {
		l := &input.Limiters[i]
		switch l.LimiterType {
		case ServiceQuotaLimiterRPM, ServiceQuotaLimiterTPM, ServiceQuotaLimiterTPD,
			ServiceQuotaLimiterDailyUSD, ServiceQuotaLimiterConcurrency:
		default:
			return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_LIMITER_TYPE", "invalid limiter_type").WithMetadata(map[string]string{
				"limiter_type": l.LimiterType,
			})
		}
		if l.LimitValue <= 0 {
			return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_LIMIT_VALUE", "limit_value must be > 0").WithMetadata(map[string]string{
				"limiter_type": l.LimiterType,
			})
		}
		if l.WindowMode == "" || l.LimiterType == ServiceQuotaLimiterConcurrency {
			l.WindowMode = ServiceQuotaWindowFixed
		}
		switch l.WindowMode {
		case ServiceQuotaWindowFixed, ServiceQuotaWindowRolling:
		default:
			return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_WINDOW_MODE", "invalid window_mode").WithMetadata(map[string]string{
				"window_mode": l.WindowMode,
			})
		}
		if _, dup := seen[l.LimiterType]; dup {
			return pkgerrors.BadRequest("SERVICE_QUOTA_DUPLICATE_LIMITER", "duplicate limiter_type in rule").WithMetadata(map[string]string{
				"limiter_type": l.LimiterType,
			})
		}
		seen[l.LimiterType] = struct{}{}
		if err := normalizeLimiterTokenComponents(l); err != nil {
			return err
		}
	}
	return nil
}

// normalizeLimiterTokenComponents 校验并归一化 token_components：
//   - TPM/TPD：至少 1 项；只允许 4 个合法值；空时填充默认值
//   - 其他类型：强制置 nil（DB 仍存默认值，但 API 层对外不暴露这个字段）
func normalizeLimiterTokenComponents(l *ServiceQuotaLimiterInput) error {
	if !ServiceQuotaLimiterTypeUsesTokenComponents(l.LimiterType) {
		l.TokenComponents = nil
		return nil
	}
	if len(l.TokenComponents) == 0 {
		l.TokenComponents = append([]string(nil), ServiceQuotaTokenComponentsDefault...)
		return nil
	}
	seen := make(map[string]struct{}, len(l.TokenComponents))
	out := make([]string, 0, len(l.TokenComponents))
	for _, c := range l.TokenComponents {
		if !IsValidServiceQuotaTokenComponent(c) {
			return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_TOKEN_COMPONENT", "invalid token_component").WithMetadata(map[string]string{
				"limiter_type":    l.LimiterType,
				"token_component": c,
			})
		}
		if _, dup := seen[c]; dup {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	l.TokenComponents = out
	return nil
}

// normalizePaths 校验每条 path 的链路一致性（账号属于分组、平台一致），
// 按 platform 小写归一化，并对同一规则内 5 字段
// （platform/channel_id/group_id/account_id/model_pattern）完全相同的 path 做提前去重，
// 避免 DB 唯一约束在写入时返回 23505，让前端拿到 500 而非可读的 BadRequest。
func (s *serviceQuotaService) normalizePaths(ctx context.Context, input *ServiceQuotaRuleInput) error {
	if len(input.Paths) == 0 {
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_PATHS", "at least one path is required")
	}
	seen := make(map[string]int, len(input.Paths))
	for i := range input.Paths {
		p := &input.Paths[i]
		if p.Platform != nil {
			trimmed := strings.TrimSpace(*p.Platform)
			if trimmed == "" {
				p.Platform = nil
			} else {
				lower := strings.ToLower(trimmed)
				p.Platform = &lower
			}
		}
		if p.ModelPattern != nil {
			trimmed := strings.TrimSpace(*p.ModelPattern)
			if trimmed == "" {
				p.ModelPattern = nil
			} else {
				p.ModelPattern = &trimmed
			}
		}
		if err := s.validatePathLinkage(ctx, *p); err != nil {
			return err
		}
		key := serviceQuotaPathSignature(*p)
		if prev, dup := seen[key]; dup {
			return pkgerrors.BadRequest(
				"SERVICE_QUOTA_PATH_DUPLICATE",
				"duplicate path: same combination of platform/channel_id/group_id/account_id/model_pattern",
			).WithMetadata(map[string]string{
				"path_index":         strconv.Itoa(i),
				"duplicate_of_index": strconv.Itoa(prev),
				"signature":          key,
			})
		}
		seen[key] = i
	}
	return nil
}

// serviceQuotaPathSignature 为同一规则内的 path 生成稳定签名，
// nil 字段统一用 "_NIL_" 占位以便与显式空值/0 区分。
func serviceQuotaPathSignature(p ServiceQuotaPathInput) string {
	const nilToken = "_NIL_"
	platform := nilToken
	if p.Platform != nil {
		platform = *p.Platform
	}
	channelID := nilToken
	if p.ChannelID != nil {
		channelID = strconv.FormatInt(*p.ChannelID, 10)
	}
	groupID := nilToken
	if p.GroupID != nil {
		groupID = strconv.FormatInt(*p.GroupID, 10)
	}
	accountID := nilToken
	if p.AccountID != nil {
		accountID = strconv.FormatInt(*p.AccountID, 10)
	}
	modelPattern := nilToken
	if p.ModelPattern != nil {
		modelPattern = *p.ModelPattern
	}
	return strings.Join([]string{platform, channelID, groupID, accountID, modelPattern}, "|")
}

// validatePathLinkage 校验单条 path 内 account/group/platform 之间的隶属关系。
func (s *serviceQuotaService) validatePathLinkage(ctx context.Context, path ServiceQuotaPathInput) error {
	platform := ""
	if path.Platform != nil {
		platform = *path.Platform
	}

	var accountInfo *AccountScopeInfo
	if path.AccountID != nil && *path.AccountID > 0 {
		info, err := s.repo.FetchAccountScope(ctx, *path.AccountID)
		if err != nil {
			return err
		}
		if info == nil {
			return pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_ACCOUNT_NOT_FOUND", "account not found").WithMetadata(map[string]string{
				"account_id": strconv.FormatInt(*path.AccountID, 10),
			})
		}
		accountInfo = info
	}

	var groupInfo *GroupScopeInfo
	if path.GroupID != nil && *path.GroupID > 0 {
		info, err := s.repo.FetchGroupScope(ctx, *path.GroupID)
		if err != nil {
			return err
		}
		if info == nil {
			return pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_GROUP_NOT_FOUND", "group not found").WithMetadata(map[string]string{
				"group_id": strconv.FormatInt(*path.GroupID, 10),
			})
		}
		groupInfo = info
	}

	if accountInfo != nil && path.GroupID != nil && *path.GroupID > 0 {
		found := false
		for _, gid := range accountInfo.GroupIDs {
			if gid == *path.GroupID {
				found = true
				break
			}
		}
		if !found {
			return pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_MISMATCH", "account is not a member of the specified group").WithMetadata(map[string]string{
				"account_id": strconv.FormatInt(*path.AccountID, 10),
				"group_id":   strconv.FormatInt(*path.GroupID, 10),
			})
		}
	}
	if accountInfo != nil && platform != "" && !strings.EqualFold(accountInfo.Platform, platform) {
		return pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_MISMATCH", "account platform does not match the specified platform").WithMetadata(map[string]string{
			"account_id":        strconv.FormatInt(*path.AccountID, 10),
			"account_platform":  accountInfo.Platform,
			"expected_platform": platform,
		})
	}
	if groupInfo != nil && platform != "" && !strings.EqualFold(groupInfo.Platform, platform) {
		return pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_MISMATCH", "group platform does not match the specified platform").WithMetadata(map[string]string{
			"group_id":          strconv.FormatInt(*path.GroupID, 10),
			"group_platform":    groupInfo.Platform,
			"expected_platform": platform,
		})
	}
	return nil
}

// PreCheck 兼容旧 caller 的一阶段入口。
//
// 新代码请使用 BillingCacheService.PrepareBillingCheck（caller 路由前）+
// BillingTicket.Consume（选定 channel/account 后）的两阶段路径，参见
// service_quota_types.go 中 ServiceQuotaService.PreCheckSelect / PreCheckAcquire 的注释。
//
// 内部实现：直接复用 PreCheckSelect → PreCheckAcquire，二者共享同一份匹配/抢槽位逻辑。
func (s *serviceQuotaService) PreCheck(ctx context.Context, req ServiceQuotaCheckRequest) (*ServiceQuotaLease, error) {
	plan, err := s.PreCheckSelect(ctx, req)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, nil
	}
	return s.PreCheckAcquire(ctx, plan, req.ChannelID, req.AccountID)
}

// PreCheckSelect 仅做规则级过滤（enabled、counter_mode=user 时 target_user_ids 命中），
// 不做 path 匹配也不抢 concurrency / 增 rpm。返回 nil plan 表示"无规则需要参与本次请求"。
//
// 设计为只读、可在 caller 路由前调用——此时 ChannelID / AccountID 通常未知。
func (s *serviceQuotaService) PreCheckSelect(ctx context.Context, req ServiceQuotaCheckRequest) (*ServiceQuotaPreCheckPlan, error) {
	if s == nil || s.repo == nil || s.settings == nil || s.limiter == nil || req.UserID <= 0 {
		return nil, nil
	}
	if !s.isEnabled(ctx) {
		return nil, nil
	}
	rules := s.loadRulesWithCache(ctx)
	if len(rules) == 0 {
		return nil, nil
	}
	candidates := make([]*ServiceQuotaRule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil || !rule.Enabled {
			continue
		}
		if !serviceQuotaTargetMatches(rule, req.UserID) {
			continue
		}
		candidates = append(candidates, rule)
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	return &ServiceQuotaPreCheckPlan{
		Rules:      candidates,
		Req:        req,
		PreparedAt: time.Now(),
	}, nil
}

// PreCheckAcquire 在 caller 选定 channel + account 后调用，基于 plan 重新做 path 匹配
// （含 channel/account 维度），按 concurrency → rpm → deferred 三轮判定，命中超限返回错误，
// 否则返回已 acquire 的 *ServiceQuotaLease。
//
// plan == nil（PreCheckSelect 返回空）或匹配后无任何 path 命中时返回 (nil, nil)。
// 任何超限错误会先释放本轮已抢的 concurrency 槽位再返回，调用方无需补偿。
func (s *serviceQuotaService) PreCheckAcquire(ctx context.Context, plan *ServiceQuotaPreCheckPlan, channelID, accountID int64) (*ServiceQuotaLease, error) {
	if plan == nil || len(plan.Rules) == 0 {
		return nil, nil
	}
	req := plan.Req
	req.ChannelID = channelID
	req.AccountID = accountID

	matched := make([]matchedQuotaRule, 0, len(plan.Rules))
	for _, rule := range plan.Rules {
		paths := findMatchingPaths(rule, req)
		if len(paths) == 0 {
			continue
		}
		matched = append(matched, matchedQuotaRule{rule: rule, paths: paths})
	}
	if len(matched) == 0 {
		return nil, nil
	}
	matched = applyServiceQuotaFallbackOverride(matched)

	lease := &ServiceQuotaLease{}
	// 三轮：concurrency → RPM → deferred(tpm/tpd/daily_usd)。先抢并发额度避免后续判断时占用未释放。
	for _, m := range matched {
		for _, p := range m.paths {
			for _, lim := range m.rule.Limiters {
				if lim.LimiterType != ServiceQuotaLimiterConcurrency {
					continue
				}
				if err := s.acquireConcurrency(ctx, req, m.rule, p, lim, lease); err != nil {
					lease.release()
					return nil, err
				}
			}
		}
	}
	for _, m := range matched {
		for _, p := range m.paths {
			for _, lim := range m.rule.Limiters {
				if lim.LimiterType != ServiceQuotaLimiterRPM {
					continue
				}
				if err := s.checkRPM(ctx, req, m.rule, p, lim); err != nil {
					lease.release()
					return nil, err
				}
			}
		}
	}
	for _, m := range matched {
		for _, p := range m.paths {
			for _, lim := range m.rule.Limiters {
				if !isDeferredServiceQuotaLimiter(lim.LimiterType) {
					continue
				}
				if err := s.checkDeferred(ctx, req, m.rule, p, lim); err != nil {
					lease.release()
					return nil, err
				}
			}
		}
	}
	return lease, nil
}

// IsPreCheckTwoPhase 返回 service_quota.precheck_two_phase setting 的当前值。
// 读取失败或 setting 缺失时一律返回 false（保守降级，沿用旧行为）。
func (s *serviceQuotaService) IsPreCheckTwoPhase(ctx context.Context) bool {
	if s == nil || s.settings == nil {
		return false
	}
	enabled, err := s.settings.GetBool(ctx, SettingKeyServiceQuotaPreCheckTwoPhase)
	if err != nil {
		return false
	}
	return enabled
}

func (s *serviceQuotaService) Record(ctx context.Context, req ServiceQuotaRecordRequest) {
	matched, ok := s.matchedRules(ctx, req.ServiceQuotaCheckRequest)
	if !ok || len(matched) == 0 {
		return
	}
	for _, m := range applyServiceQuotaFallbackOverride(matched) {
		for _, p := range m.paths {
			for _, lim := range m.rule.Limiters {
				delta := serviceQuotaRecordDelta(lim, req)
				if delta <= 0 {
					continue
				}
				key := s.counterKey(req.ServiceQuotaCheckRequest, m.rule, p, lim)
				if _, err := s.limiter.Increment(ctx, key, delta, serviceQuotaWindow(lim), lim.WindowMode); err != nil {
					s.logLimiterFailure("record", m.rule, lim, key, err)
				}
			}
		}
	}
}

func (s *serviceQuotaService) matchedRules(ctx context.Context, req ServiceQuotaCheckRequest) ([]matchedQuotaRule, bool) {
	if s == nil || s.repo == nil || s.settings == nil || s.limiter == nil || req.UserID <= 0 {
		return nil, false
	}
	if !s.isEnabled(ctx) {
		return nil, false
	}
	rules := s.loadRulesWithCache(ctx)
	if rules == nil {
		return nil, false
	}
	out := make([]matchedQuotaRule, 0, len(rules))
	for _, rule := range rules {
		if !rule.Enabled || !serviceQuotaTargetMatches(rule, req.UserID) {
			continue
		}
		paths := findMatchingPaths(rule, req)
		if len(paths) == 0 {
			continue
		}
		out = append(out, matchedQuotaRule{rule: rule, paths: paths})
	}
	return out, true
}

func (s *serviceQuotaService) isEnabled(ctx context.Context) bool {
	if s.cache != nil {
		val, err := s.cache.GetEnabled(ctx)
		if err == nil && val != nil {
			return *val
		}
	}
	enabled, err := s.settings.GetBool(ctx, SettingKeyServiceQuotaEnabled)
	if err != nil {
		return false
	}
	if s.cache != nil {
		_ = s.cache.SetEnabled(ctx, enabled)
	}
	return enabled
}

func (s *serviceQuotaService) loadRulesWithCache(ctx context.Context) []*ServiceQuotaRule {
	if s.cache != nil {
		rules, hit, err := s.cache.GetRules(ctx)
		if err == nil && hit {
			return rules
		}
	}
	val, err, _ := s.sf.Do("load_rules", func() (interface{}, error) {
		rules, err := s.repo.List(ctx, ServiceQuotaListFilter{})
		if err != nil {
			return nil, err
		}
		if s.cache != nil {
			_ = s.cache.SetRules(ctx, rules)
		}
		return rules, nil
	})
	if err != nil {
		return nil
	}
	return val.([]*ServiceQuotaRule)
}

// reloadCache 是写路径在写完 DB 后的缓存失效入口。多实例部署下只能 Del 缓存
// （而不是主动 SetRules），让其他实例下次读时通过 singleflight 重新拉取，
// 否则其他实例仍会持有最长达 serviceQuotaCacheTTL 的旧规则。
func (s *serviceQuotaService) reloadCache(ctx context.Context) {
	if s.cache == nil {
		return
	}
	if err := s.cache.InvalidateRules(ctx); err != nil {
		slog.Warn("invalidate service quota rules cache failed", "error", err)
	}
}

// ReloadCache 暴露给 handler 层在删除 channel/group/account/user 等
// 上游实体后主动失效服务限额规则缓存，下次读时会重新从 DB 拉取。
//
// 实现上仅 Del key，不重新 SELECT 数据库再 SetRules——后者会回到
// "实例 A 立即覆盖、实例 B 仍持旧值" 的旧语义，违背任务 #3 的修复初衷。
func (s *serviceQuotaService) ReloadCache(ctx context.Context) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.Invalidate(ctx)
}

func (s *serviceQuotaService) acquireConcurrency(ctx context.Context, req ServiceQuotaCheckRequest, rule *ServiceQuotaRule, path ServiceQuotaPathDef, lim ServiceQuotaLimiterDef, lease *ServiceQuotaLease) error {
	member := fmt.Sprintf("%d:%d", req.UserID, time.Now().UnixNano())
	key := s.counterKey(req, rule, path, lim)
	ok, err := s.limiter.Acquire(ctx, key, member, int64(lim.LimitValue))
	if err != nil {
		s.recordFailOpen(rule, lim, "acquire_concurrency", key, err)
		return nil
	}
	if !ok {
		recordPreCheckResult(rule, lim, metrics.ServiceQuotaPreCheckResultDenied)
		return serviceQuotaExceeded(rule, lim, 0)
	}
	recordPreCheckResult(rule, lim, metrics.ServiceQuotaPreCheckResultAllowed)
	prev := lease.Release
	lease.Release = func() {
		if prev != nil {
			prev()
		}
		if rerr := s.limiter.Release(context.Background(), key, member); rerr != nil {
			s.logLimiterFailure("release_concurrency", rule, lim, key, rerr)
		}
	}
	return nil
}

func (s *serviceQuotaService) checkRPM(ctx context.Context, req ServiceQuotaCheckRequest, rule *ServiceQuotaRule, path ServiceQuotaPathDef, lim ServiceQuotaLimiterDef) error {
	key := s.counterKey(req, rule, path, lim)
	used, err := s.limiter.Increment(ctx, key, 1, time.Minute, lim.WindowMode)
	if err != nil {
		// fail-open: Increment 报错时，已经 +1 的计数会成为下次请求的 “幽灵 +1”。
		// best-effort 反向 Increment(-1) 抵消：fixed window 走 IncrByFloat(-1) 干净回退；
		// rolling window 通过追加一条 delta=-1 的 member 与原 +1 在 sumRollingMembers 求和时
		// 相互抵消，等价于回滚。任何回退失败都忽略，不阻塞主流程。
		_, _ = s.limiter.Increment(ctx, key, -1, time.Minute, lim.WindowMode)
		s.recordFailOpen(rule, lim, "check_rpm", key, err)
		return nil
	}
	if used > lim.LimitValue {
		recordPreCheckResult(rule, lim, metrics.ServiceQuotaPreCheckResultDenied)
		return serviceQuotaExceeded(rule, lim, used)
	}
	recordPreCheckResult(rule, lim, metrics.ServiceQuotaPreCheckResultAllowed)
	return nil
}

func (s *serviceQuotaService) checkDeferred(ctx context.Context, req ServiceQuotaCheckRequest, rule *ServiceQuotaRule, path ServiceQuotaPathDef, lim ServiceQuotaLimiterDef) error {
	key := s.counterKey(req, rule, path, lim)
	used, err := s.limiter.Current(ctx, key, serviceQuotaWindow(lim), lim.WindowMode)
	if err != nil {
		s.recordFailOpen(rule, lim, "check_deferred", key, err)
		return nil
	}
	if used >= lim.LimitValue {
		recordPreCheckResult(rule, lim, metrics.ServiceQuotaPreCheckResultDenied)
		return serviceQuotaExceeded(rule, lim, used)
	}
	recordPreCheckResult(rule, lim, metrics.ServiceQuotaPreCheckResultAllowed)
	return nil
}

// recordFailOpen 在限流器报错降级时同时落日志 + 累加 metrics。
//
// reason 区分 timeout（context.DeadlineExceeded / context.Canceled）和 redis_error（其他），
// 让运维侧能区分网络超时与 redis 连接 / Lua 脚本异常。
func (s *serviceQuotaService) recordFailOpen(rule *ServiceQuotaRule, lim ServiceQuotaLimiterDef, op, key string, err error) {
	s.logLimiterFailure(op, rule, lim, key, err)
	reason := metrics.ServiceQuotaFailOpenReasonRedisError
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		reason = metrics.ServiceQuotaFailOpenReasonTimeout
	}
	metrics.ServiceQuotaFailOpenTotal.WithLabelValues(lim.LimiterType, reason).Inc()
	recordPreCheckResult(rule, lim, metrics.ServiceQuotaPreCheckResultFailOpen)
}

// recordPreCheckResult 把 (rule, limiter, result) 三元组的一次判定累加到 PreCheck 指标桶。
func recordPreCheckResult(rule *ServiceQuotaRule, lim ServiceQuotaLimiterDef, result string) {
	ruleID := "0"
	if rule != nil {
		ruleID = strconv.FormatInt(rule.ID, 10)
	}
	metrics.ServiceQuotaPreCheckTotal.WithLabelValues(ruleID, lim.LimiterType, result).Inc()
}

// Fail-open is intentional: when Redis is unreachable we allow the request
// through rather than return 5xx, but we must surface the degradation so the
// operator knows service quota rules are silently inert.
func (s *serviceQuotaService) logLimiterFailure(op string, rule *ServiceQuotaRule, lim ServiceQuotaLimiterDef, key string, err error) {
	slog.Warn("service quota limiter unavailable, rule not enforced",
		"op", op,
		"rule_id", rule.ID,
		"limiter_id", lim.ID,
		"limiter_type", lim.LimiterType,
		"window_mode", lim.WindowMode,
		"key", key,
		"error", err,
	)
}

// serviceQuotaCounterKeyTargetShared 是 counter key 中"共享计数器"的目标段。
// counter_mode=shared 与未指定 user 的旁路读取都用这个值，确保 counterKey 与
// BuildServiceQuotaCounterKey 对同一逻辑路径输出完全相同的 Redis key。
const serviceQuotaCounterKeyTargetShared = "shared"

// BuildServiceQuotaCounterKey 拼接 path × limiter 的 Redis 计数 key。
//
// 入参 scopeUserID == nil → target 段写 "shared"；非 nil → 写 *scopeUserID。
// 写路径（counterKey）内部根据 rule.CounterMode 判定要不要传 userID；只读路径
// （Snapshot 类）可以直接显式构造 key，无需感知 rule 结构。
//
// v2 前缀与历史 counterKey 保持一致，让监控/快照与限流读写命中同一计数器。
func BuildServiceQuotaCounterKey(ruleID, pathID int64, limiterType string, scopeUserID *int64) string {
	target := serviceQuotaCounterKeyTargetShared
	if scopeUserID != nil {
		target = strconv.FormatInt(*scopeUserID, 10)
	}
	return fmt.Sprintf("svcquota:v2:%d:%d:%s:%s", ruleID, pathID, limiterType, target)
}

// counterKey 返回 path × limiter 的独立计数 key。v2 前缀使旧 key 自然失效。
//
// 内部委托给 BuildServiceQuotaCounterKey：counter_mode=user/per_user 时把
// req.UserID 传进去，shared 时传 nil；保持与历史 key 完全一致。
func (s *serviceQuotaService) counterKey(req ServiceQuotaCheckRequest, rule *ServiceQuotaRule, path ServiceQuotaPathDef, lim ServiceQuotaLimiterDef) string {
	var scopeUserID *int64
	if rule.CounterMode == ServiceQuotaCounterModeUser || rule.CounterMode == ServiceQuotaCounterModePerUser {
		uid := req.UserID
		scopeUserID = &uid
	}
	return BuildServiceQuotaCounterKey(rule.ID, path.ID, lim.LimiterType, scopeUserID)
}

func (l *ServiceQuotaLease) release() {
	if l != nil && l.Release != nil {
		l.Release()
	}
}

func sanitizeTargetUserIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func serviceQuotaTargetMatches(rule *ServiceQuotaRule, userID int64) bool {
	if rule.CounterMode != ServiceQuotaCounterModeUser {
		return true
	}
	for _, id := range rule.TargetUserIDs {
		if id == userID {
			return true
		}
	}
	return false
}

// pathMatches 判断单条 path 是否命中请求。每个非 nil 字段都要相等，nil 视作不限制该维度。
func pathMatches(path ServiceQuotaPathDef, req ServiceQuotaCheckRequest) bool {
	if path.Platform != nil && !strings.EqualFold(*path.Platform, req.Platform) {
		return false
	}
	if path.ChannelID != nil && *path.ChannelID != req.ChannelID {
		return false
	}
	if path.GroupID != nil && *path.GroupID != req.GroupID {
		return false
	}
	if path.AccountID != nil && *path.AccountID != req.AccountID {
		return false
	}
	if path.ModelPattern != nil && !serviceQuotaModelMatches(*path.ModelPattern, req.Model) {
		return false
	}
	return true
}

// findMatchingPaths 返回规则中命中本次请求的路径列表。规则必须至少有一条 path（由 normalizePaths 保证）。
func findMatchingPaths(rule *ServiceQuotaRule, req ServiceQuotaCheckRequest) []ServiceQuotaPathDef {
	out := make([]ServiceQuotaPathDef, 0, len(rule.Paths))
	for _, p := range rule.Paths {
		if pathMatches(p, req) {
			out = append(out, p)
		}
	}
	return out
}

func serviceQuotaModelMatches(pattern, model string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return true
	}
	ok, err := filepath.Match(pattern, model)
	return err == nil && ok
}

// applyServiceQuotaFallbackOverride 在限流器层级处理 is_fallback 语义：
// 同一 limiter_type 若有任意非 fallback 规则命中并配置了该类型限流器，则丢弃 fallback 规则中该类型的限流器
// （而不是整条 fallback 规则）。如果丢空了 fallback 规则的所有限流器，则该 fallback 规则整条被忽略。
func applyServiceQuotaFallbackOverride(matched []matchedQuotaRule) []matchedQuotaRule {
	covered := map[string]bool{}
	for _, m := range matched {
		if m.rule.IsFallback {
			continue
		}
		for _, lim := range m.rule.Limiters {
			covered[lim.LimiterType] = true
		}
	}
	out := make([]matchedQuotaRule, 0, len(matched))
	for _, m := range matched {
		if !m.rule.IsFallback {
			out = append(out, m)
			continue
		}
		kept := make([]ServiceQuotaLimiterDef, 0, len(m.rule.Limiters))
		for _, lim := range m.rule.Limiters {
			if covered[lim.LimiterType] {
				continue
			}
			kept = append(kept, lim)
		}
		if len(kept) == 0 {
			continue
		}
		clone := *m.rule
		clone.Limiters = kept
		out = append(out, matchedQuotaRule{rule: &clone, paths: m.paths})
	}
	return out
}

func isDeferredServiceQuotaLimiter(kind string) bool {
	return kind == ServiceQuotaLimiterTPM || kind == ServiceQuotaLimiterTPD || kind == ServiceQuotaLimiterDailyUSD
}

func serviceQuotaWindow(lim ServiceQuotaLimiterDef) time.Duration {
	switch lim.LimiterType {
	case ServiceQuotaLimiterRPM, ServiceQuotaLimiterTPM:
		return time.Minute
	case ServiceQuotaLimiterTPD, ServiceQuotaLimiterDailyUSD:
		return 24 * time.Hour
	default:
		return time.Minute
	}
}

// serviceQuotaRecordDelta 计算单个 limiter 在一次请求中要增加的计数。
//
// TPM/TPD：按 limiter.TokenComponents 求和——每个限流器可独立选择 input/output/cache_creation/cache_read 子集。
//   - 默认 {input, output, cache_creation}（DB DEFAULT + ServiceQuotaTokenComponentsDefault）
//   - TokenComponents 为空时退化为默认值，向前兼容老规则
//
// daily_usd：按金额；其他 limiter type（rpm/concurrency）走单独路径，不入 Record。
func serviceQuotaRecordDelta(lim ServiceQuotaLimiterDef, req ServiceQuotaRecordRequest) float64 {
	switch lim.LimiterType {
	case ServiceQuotaLimiterTPM, ServiceQuotaLimiterTPD:
		components := lim.TokenComponents
		if len(components) == 0 {
			components = ServiceQuotaTokenComponentsDefault
		}
		return float64(req.SumTokens(components))
	case ServiceQuotaLimiterDailyUSD:
		return req.Cost
	default:
		return 0
	}
}

// serviceQuotaExceeded 构造 429 拒绝错误。
//
// metadata 必须携带足够字段让前端在不发二次请求的前提下定位是哪条规则的哪个 limiter 超了：
//   - rule_id：规则 ID
//   - rule_name：规则可读名（缺省 fallback 为 "unnamed-{id}"）
//   - limiter_type：rpm/tpm/tpd/daily_usd/concurrency
//   - limiter_window：fixed/rolling/none（concurrency 没有窗口语义，统一返回 "none"）
//   - limit_value：阈值
//   - limit：阈值（保留旧 key 以兼容老前端）
//   - used：当前已用量
func serviceQuotaExceeded(rule *ServiceQuotaRule, lim ServiceQuotaLimiterDef, used float64) error {
	return ErrServiceQuotaExceeded.WithMetadata(map[string]string{
		"rule_id":        strconv.FormatInt(rule.ID, 10),
		"rule_name":      ruleDisplayName(rule),
		"limiter_type":   lim.LimiterType,
		"limiter_window": limiterWindowLabel(lim),
		"limit_value":    strconv.FormatFloat(lim.LimitValue, 'f', -1, 64),
		"limit":          strconv.FormatFloat(lim.LimitValue, 'f', -1, 64),
		"used":           strconv.FormatFloat(used, 'f', -1, 64),
	})
}

// ruleDisplayName 返回规则的展示名，rule.Name 为 nil 或空字符串时回退为 "unnamed-{id}"。
func ruleDisplayName(rule *ServiceQuotaRule) string {
	if rule == nil {
		return "unnamed-0"
	}
	if rule.Name != nil {
		if name := strings.TrimSpace(*rule.Name); name != "" {
			return name
		}
	}
	return "unnamed-" + strconv.FormatInt(rule.ID, 10)
}

// limiterWindowLabel 返回前端可消费的 window 标签：concurrency 没有窗口语义统一为 "none"。
func limiterWindowLabel(lim ServiceQuotaLimiterDef) string {
	if lim.LimiterType == ServiceQuotaLimiterConcurrency {
		return "none"
	}
	if lim.WindowMode == "" {
		return ServiceQuotaWindowFixed
	}
	return lim.WindowMode
}

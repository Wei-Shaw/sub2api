package service

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

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
	if input.CounterMode != ServiceQuotaCounterModeUser {
		input.TargetUserIDs = nil
	}
	// 先做字段级硬校验，一次性收集所有字段错误（前端按 metadata.fields JSON 渲染高亮）。
	// 字段错误 reason = SERVICE_QUOTA_VALIDATION_ERROR，与旧 SERVICE_QUOTA_INVALID_TARGET 等单字段错误码区分；
	// 旧错误码保留给"业务级校验"（链路 mismatch / 重复 path 等）。
	if err := validateRuleFields(input); err != nil {
		return err
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
		if err := validateLimiterType(l, seen); err != nil {
			return err
		}
		seen[l.LimiterType] = struct{}{}
		if err := normalizeLimiterTokenComponents(l); err != nil {
			return err
		}
		normalizeLimiterCountOnArrival(l)
		normalizeLimiterValue(l)
	}
	return nil
}

// normalizeLimiterValue 对整数型 limiter（rpm/tpm/tpd/concurrency）做 LimitValue 取整，
// 截断浮点漂移让"1000000 来回往返不再变成 999999.999997"。
//
// daily_usd 是金额类型本身就允许小数，保持原值不动（项目 CLAUDE.md 强制金额走 decimal，
// 但当前 storage / API 还是 float64，等彻底改 string-based 时再统一处理）。
//
// 这是数据底线：前端 NumericInput integer prop 是 UX 屏障，但任何来源（API 直调 / 旧客户端 /
// 数据导入）传非整数都会被这里拉回整数，不让浮点污染落库。
//
// 用 math.Round（四舍五入）而非 math.Floor / math.Trunc：用户传 999.9 显然意图是 1000，
// 而不是 999；项目里 IEEE 754 浮点漂移的实际场景都是 ±1e-9 量级，round 的结果与意图一致。
func normalizeLimiterValue(l *ServiceQuotaLimiterInput) {
	if !ServiceQuotaLimiterTypeIsInteger(l.LimiterType) {
		return
	}
	l.LimitValue = math.Round(l.LimitValue)
}

// validateLimiterType 校验单个 limiter 的"硬性"字段：
//   - LimiterType 在合法枚举内
//   - LimitValue > 0（rpm/tpm/tpd/concurrency/daily_usd 都要求严格正数）
//   - WindowMode 合法（concurrency 强制为 fixed；空值默认 fixed）
//   - 同 rule 内 LimiterType 不重复（seen 集合由调用方维护与回写，避免 helper 改副作用）
//
// 拆出来让 normalizeLimiters 主循环只剩 "校验 → 标记 seen → normalize" 三段，
// 不再被 4 段 switch + if 撑成 ~30 行。WindowMode 的强制 fixed 写回 *l.WindowMode 是必要副作用，
// 保留在 helper 内（与原 inline 逻辑等价）。
func validateLimiterType(l *ServiceQuotaLimiterInput, seen map[string]struct{}) error {
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
	return nil
}

// normalizeLimiterCountOnArrival 归一化 count_on_arrival：仅 RPM 保留传入值，
// 其他 limiter type 一律置 nil，避免管理员误填污染存储。
func normalizeLimiterCountOnArrival(l *ServiceQuotaLimiterInput) {
	if !ServiceQuotaLimiterTypeUsesCountOnArrival(l.LimiterType) {
		l.CountOnArrival = nil
	}
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

// validatePathLinkage 校验单条 path 内 channel/account/group/platform 之间的隶属关系。
//
// 校验维度（按出现先后短路）：
//  1. account ↔ group：account 必须属于 path 指定 group
//  2. account ↔ platform：account 平台必须等于 path 指定 platform
//  3. group ↔ platform：group 平台必须等于 path 指定 platform
//  4. channel ↔ platform：channel.model_pricing 必须服务 path 指定 platform
//  5. channel ↔ group：channel.channel_groups 必须包含 path 指定 group
//
// 错误码 SERVICE_QUOTA_SCOPE_MISMATCH 统一 metadata 标 entity 字段（account/group/channel）
// 让前端据此决定高亮哪个下拉框。channel 不存在返回 SERVICE_QUOTA_SCOPE_CHANNEL_NOT_FOUND，
// 与 account/group not found 保持一致。
func (s *serviceQuotaService) validatePathLinkage(ctx context.Context, path ServiceQuotaPathInput) error {
	platform := ""
	if path.Platform != nil {
		platform = *path.Platform
	}

	accountInfo, err := s.fetchAccountScopeForPath(ctx, path)
	if err != nil {
		return err
	}
	groupInfo, err := s.fetchGroupScopeForPath(ctx, path)
	if err != nil {
		return err
	}
	channelInfo, err := s.fetchChannelScopeForPath(ctx, path)
	if err != nil {
		return err
	}

	if err := validateAccountLinkage(path, platform, accountInfo); err != nil {
		return err
	}
	if err := validateGroupLinkage(path, platform, groupInfo); err != nil {
		return err
	}
	return validateChannelLinkage(path, platform, channelInfo)
}

func (s *serviceQuotaService) fetchAccountScopeForPath(ctx context.Context, path ServiceQuotaPathInput) (*AccountScopeInfo, error) {
	if path.AccountID == nil || *path.AccountID <= 0 {
		return nil, nil
	}
	info, err := s.repo.FetchAccountScope(ctx, *path.AccountID)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_ACCOUNT_NOT_FOUND", "account not found").WithMetadata(map[string]string{
			"account_id": strconv.FormatInt(*path.AccountID, 10),
		})
	}
	return info, nil
}

func (s *serviceQuotaService) fetchGroupScopeForPath(ctx context.Context, path ServiceQuotaPathInput) (*GroupScopeInfo, error) {
	if path.GroupID == nil || *path.GroupID <= 0 {
		return nil, nil
	}
	info, err := s.repo.FetchGroupScope(ctx, *path.GroupID)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_GROUP_NOT_FOUND", "group not found").WithMetadata(map[string]string{
			"group_id": strconv.FormatInt(*path.GroupID, 10),
		})
	}
	return info, nil
}

func (s *serviceQuotaService) fetchChannelScopeForPath(ctx context.Context, path ServiceQuotaPathInput) (*ChannelScopeInfo, error) {
	if path.ChannelID == nil || *path.ChannelID <= 0 {
		return nil, nil
	}
	info, err := s.repo.FetchChannelScope(ctx, *path.ChannelID)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_CHANNEL_NOT_FOUND", "channel not found").WithMetadata(map[string]string{
			"channel_id": strconv.FormatInt(*path.ChannelID, 10),
		})
	}
	return info, nil
}

func validateAccountLinkage(path ServiceQuotaPathInput, platform string, accountInfo *AccountScopeInfo) error {
	if accountInfo == nil {
		return nil
	}
	if path.GroupID != nil && *path.GroupID > 0 {
		if !containsInt64(accountInfo.GroupIDs, *path.GroupID) {
			return pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_MISMATCH", "account is not a member of the specified group").WithMetadata(map[string]string{
				"entity":     "account_group",
				"account_id": strconv.FormatInt(*path.AccountID, 10),
				"group_id":   strconv.FormatInt(*path.GroupID, 10),
			})
		}
	}
	if platform != "" && !strings.EqualFold(accountInfo.Platform, platform) {
		return pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_MISMATCH", "account platform does not match the specified platform").WithMetadata(map[string]string{
			"entity":            "account_platform",
			"account_id":        strconv.FormatInt(*path.AccountID, 10),
			"account_platform":  accountInfo.Platform,
			"expected_platform": platform,
		})
	}
	return nil
}

func validateGroupLinkage(path ServiceQuotaPathInput, platform string, groupInfo *GroupScopeInfo) error {
	if groupInfo == nil || platform == "" {
		return nil
	}
	if !strings.EqualFold(groupInfo.Platform, platform) {
		return pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_MISMATCH", "group platform does not match the specified platform").WithMetadata(map[string]string{
			"entity":            "group_platform",
			"group_id":          strconv.FormatInt(*path.GroupID, 10),
			"group_platform":    groupInfo.Platform,
			"expected_platform": platform,
		})
	}
	return nil
}

// validateChannelLinkage 校验 channel 与 path.platform / path.group_id 的隶属关系：
//   - channel ↔ platform：path.platform 必须出现在 channel.model_pricing 的 platform 集合中
//   - channel ↔ group：path.group_id 必须出现在 channel_groups 关联表的集合中
//
// 注意 channel 服务多个 platform 是常见场景（同一渠道横跨 anthropic/openai 等），所以这里
// 用 contains-check 而非 EqualFold 单值比较。
func validateChannelLinkage(path ServiceQuotaPathInput, platform string, channelInfo *ChannelScopeInfo) error {
	if channelInfo == nil {
		return nil
	}
	if platform != "" && !containsStringFold(channelInfo.Platforms, platform) {
		return pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_MISMATCH", "channel does not serve the specified platform").WithMetadata(map[string]string{
			"entity":            "channel_platform",
			"channel_id":        strconv.FormatInt(*path.ChannelID, 10),
			"channel_platforms": strings.Join(channelInfo.Platforms, ","),
			"expected_platform": platform,
		})
	}
	if path.GroupID != nil && *path.GroupID > 0 && !containsInt64(channelInfo.GroupIDs, *path.GroupID) {
		return pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_MISMATCH", "channel does not include the specified group").WithMetadata(map[string]string{
			"entity":     "channel_group",
			"channel_id": strconv.FormatInt(*path.ChannelID, 10),
			"group_id":   strconv.FormatInt(*path.GroupID, 10),
		})
	}
	return nil
}

// containsInt64 在 ops_retry.go 已存在，复用。

func containsStringFold(haystack []string, needle string) bool {
	for _, v := range haystack {
		if strings.EqualFold(v, needle) {
			return true
		}
	}
	return false
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

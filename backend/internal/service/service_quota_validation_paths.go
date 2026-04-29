package service

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

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

package service

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// MaxBatchDisableUsers 是单次批量禁用的用户数上限。
const MaxBatchDisableUsers = MaxRegistrationEmailBlacklistSize

// UserEmailBlacklistRepository 是精确邮箱黑名单关联已注册用户所需的仓储能力。
type UserEmailBlacklistRepository interface {
	// ListActiveUsersByEmailAliasProbeKeys 返回未删除、状态为 active、非管理员且邮箱折叠后
	// 命中任一 EmailAliasProbeKey 的用户。结果会过度匹配，调用方需按收件箱身份复核。
	ListActiveUsersByEmailAliasProbeKeys(ctx context.Context, keys []string) ([]User, error)
	// BatchDisableUsers 把未删除、非管理员且尚未禁用的用户置为 disabled，并在同一事务内吊销这些用户的
	// 全部 MCP 授权，返回实际变更的用户 ID。
	BatchDisableUsers(ctx context.Context, userIDs []int64) ([]int64, error)
}

// EmailAliasProbeKey 把探针拼成仓储层比对用的 "本地部分@域名" 键（两侧均已去点）。
func EmailAliasProbeKey(probe EmailAliasProbe) string {
	return probe.Local + "@" + probe.Domain
}

// FindActiveUsersByBlacklistedEmails 返回收件箱身份命中给定邮箱的未禁用普通用户；
// 格式不合法的邮箱忽略。
func (s *adminServiceImpl) FindActiveUsersByBlacklistedEmails(ctx context.Context, emails []string) ([]User, error) {
	repo, ok := s.userRepo.(UserEmailBlacklistRepository)
	if !ok {
		return nil, ErrServiceUnavailable
	}

	normalized := make([]string, 0, len(emails))
	for _, raw := range emails {
		if email, valid := NormalizeRegistrationEmailBlacklistEntry(raw); valid {
			normalized = append(normalized, email)
		}
	}
	identities := RegistrationEmailBlacklistIdentities(normalized)
	if len(identities) == 0 {
		return []User{}, nil
	}

	keySet := make(map[string]struct{}, 2*len(normalized))
	keys := make([]string, 0, 2*len(normalized))
	for _, email := range normalized {
		for _, probe := range EmailAliasDedupProbes(email) {
			key := EmailAliasProbeKey(probe)
			if _, dup := keySet[key]; dup {
				continue
			}
			keySet[key] = struct{}{}
			keys = append(keys, key)
		}
	}

	candidates, err := repo.ListActiveUsersByEmailAliasProbeKeys(ctx, keys)
	if err != nil {
		return nil, err
	}
	matched := make([]User, 0, len(candidates))
	for _, candidate := range candidates {
		if _, hit := identities[NormalizeEmailForAliasDedup(candidate.Email)]; hit {
			matched = append(matched, candidate)
		}
	}
	return matched, nil
}

// BatchDisableUsers 批量禁用普通用户（管理员与已禁用用户跳过），返回实际禁用的人数。
func (s *adminServiceImpl) BatchDisableUsers(ctx context.Context, userIDs []int64) (int, error) {
	cleaned := make([]int64, 0, len(userIDs))
	seen := make(map[int64]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		if _, dup := seen[userID]; dup {
			continue
		}
		seen[userID] = struct{}{}
		cleaned = append(cleaned, userID)
	}
	if len(cleaned) == 0 {
		return 0, nil
	}
	if len(cleaned) > MaxBatchDisableUsers {
		return 0, fmt.Errorf("user_ids cannot exceed %d", MaxBatchDisableUsers)
	}

	repo, ok := s.userRepo.(UserEmailBlacklistRepository)
	if !ok {
		return 0, ErrServiceUnavailable
	}
	disabled, err := repo.BatchDisableUsers(ctx, cleaned)
	if err != nil {
		return 0, err
	}
	if s.authCacheInvalidator != nil {
		// 状态已落库，缓存失效不随请求取消中断，否则未清到的用户在缓存 TTL 内仍能用 API Key。
		invalidateCtx := context.WithoutCancel(ctx)
		for _, userID := range disabled {
			s.authCacheInvalidator.InvalidateAuthCacheByUserID(invalidateCtx, userID)
		}
	}
	logger.LegacyPrintf("service.admin", "audit: batch disable users requested=%d disabled=%d", len(cleaned), len(disabled))
	return len(disabled), nil
}

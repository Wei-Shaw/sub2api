package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *serviceQuotaRuleRepository) FetchAccountScope(ctx context.Context, accountID int64) (*service.AccountScopeInfo, error) {
	info := &service.AccountScopeInfo{}
	err := r.db.QueryRowContext(ctx, `SELECT platform FROM accounts WHERE id = $1`, accountID).Scan(&info.Platform)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT group_id FROM account_groups WHERE account_id = $1`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var gid int64
		if err := rows.Scan(&gid); err != nil {
			return nil, err
		}
		info.GroupIDs = append(info.GroupIDs, gid)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return info, nil
}

func (r *serviceQuotaRuleRepository) FetchGroupScope(ctx context.Context, groupID int64) (*service.GroupScopeInfo, error) {
	info := &service.GroupScopeInfo{}
	err := r.db.QueryRowContext(ctx, `SELECT platform FROM groups WHERE id = $1`, groupID).Scan(&info.Platform)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return info, nil
}

// FetchChannelScope 收集 channel 服务的 platform 集合（来自 channel_model_pricing.platform，
// 同 channel 多条 pricing 行可能横跨多个平台）与 group 集合（来自 channel_groups 关联表）。
//
// 与 FetchAccountScope/FetchGroupScope 风格一致：channel 不存在返回 (nil, nil) 让 caller
// 把它翻成 BadRequest。channel 存在但没有 model_pricing / group 关联时返回空切片（非 nil），
// 让 validatePathLinkage 的 contains-check 保持单一路径。
func (r *serviceQuotaRuleRepository) FetchChannelScope(ctx context.Context, channelID int64) (*service.ChannelScopeInfo, error) {
	var exists bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM channels WHERE id = $1)`, channelID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	platforms, err := queryStringList(ctx, r.db,
		// 同一 channel 的多条 pricing 行可能重复出现同一 platform，DISTINCT + LOWER 归一化。
		`SELECT DISTINCT LOWER(platform) FROM channel_model_pricing WHERE channel_id = $1 AND platform IS NOT NULL AND platform <> ''`,
		channelID)
	if err != nil {
		return nil, err
	}
	groupIDs, err := queryInt64List(ctx, r.db,
		`SELECT group_id FROM channel_groups WHERE channel_id = $1`,
		channelID)
	if err != nil {
		return nil, err
	}
	return &service.ChannelScopeInfo{Platforms: platforms, GroupIDs: groupIDs}, nil
}

// FetchPathIDsByOwner 查 service_quota_paths 中引用某 owner（channel/group/account）的所有 path_id。
//
// 用于 admin 删除 channel/group/account 前快照受影响 path_ids → 删 DB → 异步清 Redis 残留 counter key。
// CASCADE 删完就查不到了，所以必须严格按"先快照 → 再 DELETE"顺序。
//
// owner 必须是 ServiceQuotaPathOwner* 常量之一，否则返回错误（防 SQL 注入：列名不能从外部直传）。
func (r *serviceQuotaRuleRepository) FetchPathIDsByOwner(ctx context.Context, owner string, ownerID int64) ([]int64, error) {
	switch owner {
	case service.ServiceQuotaPathOwnerChannel,
		service.ServiceQuotaPathOwnerGroup,
		service.ServiceQuotaPathOwnerAccount:
	default:
		return nil, fmt.Errorf("FetchPathIDsByOwner: unsupported owner %q", owner)
	}
	if ownerID <= 0 {
		return nil, nil
	}
	// owner 已经被白名单校验过，不会触发 SQL 注入；这里用 fmt.Sprintf 拼列名。
	query := fmt.Sprintf(`SELECT id FROM service_quota_paths WHERE %s = $1`, owner)
	return queryInt64List(ctx, r.db, query, ownerID)
}

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
)

// privateSubUserGroup 批量同步后需要失效缓存的 (user, group) 对。
type privateSubUserGroup struct {
	UserID  int64
	GroupID int64
}

const privateSubSyncBatchSize = 1000

// BulkSyncPrivateSubscriptionExpires 将全部私有专属订阅的 expires_at/status 统一改写。
// 匹配条件：group.name 匹配 private 白名单命名 + is_exclusive + subscription_type=subscription。
// 含 active/expired/suspended（S1）。返回更新行数与用于缓存失效的 (user,group) 对。
func (s *SubscriptionService) BulkSyncPrivateSubscriptionExpires(
	ctx context.Context,
	expiresAt time.Time,
	status string,
) (updated int64, pairs []privateSubUserGroup, err error) {
	if s == nil || s.entClient == nil {
		return 0, nil, fmt.Errorf("subscription service not configured for private sync")
	}
	if expiresAt.After(MaxExpiresAt) {
		expiresAt = MaxExpiresAt
	}
	if status != SubscriptionStatusActive && status != SubscriptionStatusExpired {
		return 0, nil, fmt.Errorf("invalid private sync status: %s", status)
	}

	client := s.entClient
	var afterID int64
	pairs = make([]privateSubUserGroup, 0, privateSubSyncBatchSize)

	for {
		rows, qerr := client.UserSubscription.Query().
			Where(
				usersubscription.IDGT(afterID),
				usersubscription.HasGroupWith(
					group.NameHasPrefix("private-"),
					group.IsExclusiveEQ(true),
					group.SubscriptionTypeEQ(SubscriptionTypeSubscription),
				),
			).
			WithGroup().
			Order(usersubscription.ByID()).
			Limit(privateSubSyncBatchSize).
			All(ctx)
		if qerr != nil {
			return updated, pairs, fmt.Errorf("list private subscriptions: %w", qerr)
		}
		if len(rows) == 0 {
			break
		}

		ids := make([]int64, 0, len(rows))
		batchPairs := make([]privateSubUserGroup, 0, len(rows))
		for _, row := range rows {
			afterID = row.ID
			// 白名单正则精过滤：拒绝 private-1-composite 等非平台名
			if g := row.Edges.Group; g != nil && !IsPrivateGroupName(g.Name) {
				continue
			}
			ids = append(ids, row.ID)
			batchPairs = append(batchPairs, privateSubUserGroup{UserID: row.UserID, GroupID: row.GroupID})
		}
		if len(ids) == 0 {
			if len(rows) < privateSubSyncBatchSize {
				break
			}
			continue
		}

		n, uerr := client.UserSubscription.Update().
			Where(usersubscription.IDIn(ids...)).
			SetExpiresAt(expiresAt).
			SetStatus(status).
			Save(ctx)
		if uerr != nil {
			return updated, pairs, fmt.Errorf("bulk update private subscriptions: %w", uerr)
		}
		updated += int64(n)
		pairs = append(pairs, batchPairs...)

		if len(rows) < privateSubSyncBatchSize {
			break
		}
	}
	return updated, pairs, nil
}

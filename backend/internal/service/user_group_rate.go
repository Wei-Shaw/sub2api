package service

import "context"

// UserGroupRateRepository 用户专属分组倍率仓储接口
// 允许管理员为特定用户设置分组的专属计费倍率，覆盖分组默认倍率
type UserGroupRateRepository interface {
	// GetByUserID 获取用户的所有专属分组倍率
	// 返回 map[groupID]rateMultiplier
	GetByUserID(ctx context.Context, userID int64) (map[int64]float64, error)

	// GetByUserAndGroup 获取用户在特定分组的专属倍率
	// 如果未设置专属倍率，返回 nil
	GetByUserAndGroup(ctx context.Context, userID, groupID int64) (*float64, error)

	// SyncUserGroupRates 同步用户的分组专属倍率
	// rates: map[groupID]*rateMultiplier，nil 表示删除该分组的专属倍率
	SyncUserGroupRates(ctx context.Context, userID int64, rates map[int64]*float64) error

	// DeleteByGroupID 删除指定分组的所有用户专属倍率（分组删除时调用）
	DeleteByGroupID(ctx context.Context, groupID int64) error

	// DeleteByUserID 删除指定用户的所有专属倍率（用户删除时调用）
	DeleteByUserID(ctx context.Context, userID int64) error
REDACTED

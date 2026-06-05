package service

import "context"

// UserRPMCache 用户/分组级 RPM 计数器接口。
//
// 与账号级 RPMCache 的区别：
//   - RPMCache    —— 按外部 AI provider 账号聚合（key: rpm:{accountID}:{min}）。
//   - UserRPMCache —— 按用户或 (用户, 分组) 聚合，杜绝"同一用户创建多个 API Key 绕过 RPM"的路径。
//     key 形如 rpm:ug:{userID}:{groupID}:{min} 或 rpm:u:{userID}:{min}。
type UserRPMCache interface {
	// IncrementUserGroupRPM 原子递增 (user, group) 级分钟计数并返回最新值。
	// 用于分组 rpm_limit 与 user-group rpm_override 两种命中分支。
	IncrementUserGroupRPM(ctx context.Context, userID, groupID int64) (count int, err error)

	// IncrementUserRPM 原子递增用户级分钟计数并返回最新值。
	// 用于用户全局 rpm_limit 兜底分支（分组未设且无 override 时）。
	IncrementUserRPM(ctx context.Context, userID int64) (count int, err error)

	// GetUserGroupRPM 获取 (user, group) 当前分钟已用 RPM（只读，不递增）。
	GetUserGroupRPM(ctx context.Context, userID, groupID int64) (count int, err error)

	// GetUserRPM 获取用户当前分钟已用 RPM（只读，不递增）。
	GetUserRPM(ctx context.Context, userID int64) (count int, err error)
}

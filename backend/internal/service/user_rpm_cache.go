package service

import "github.com/Wei-Shaw/sub2api/internal/port/cache"

// UserRPMCache 用户/分组级 RPM 计数器接口。
// 按用户或 (用户, 分组) 聚合，杜绝"同一用户创建多个 API Key 绕过 RPM"的路径。
type UserRPMCache = cache.UserRPMCache

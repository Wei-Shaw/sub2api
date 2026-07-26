package service

import "github.com/Wei-Shaw/sub2api/internal/port/cache"

// SessionLimitCache 管理账号级别的活跃会话跟踪
// 用于 Anthropic OAuth/SetupToken 账号的会话数量限制
//
// Key 格式: session_limit:account:{accountID}
// 数据结构: Sorted Set (member=sessionUUID, score=timestamp)
//
// 会话在空闲超时后自动过期，无需手动清理
type SessionLimitCache = cache.SessionLimitCache

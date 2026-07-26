package service

import "github.com/Wei-Shaw/sub2api/internal/port/cache"

// RPMCache RPM 计数器缓存接口
// 用于 Anthropic OAuth/SetupToken 账号的每分钟请求数限制
type RPMCache = cache.RPMCache

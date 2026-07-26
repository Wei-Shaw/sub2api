package service

import (
	"github.com/Wei-Shaw/sub2api/internal/port/cache"
)

// ErrRefreshTokenNotFound is returned when a refresh token is not found in cache.
// This is used to abstract away the underlying cache implementation (e.g., redis.Nil).
var ErrRefreshTokenNotFound = cache.ErrRefreshTokenNotFound

// RefreshTokenData 存储在Redis中的Refresh Token数据
type RefreshTokenData = cache.RefreshTokenData

// RefreshTokenCache 管理Refresh Token的Redis缓存
// 用于JWT Token刷新机制，支持Token轮转和防重放攻击
//
// Key 格式:
//   - refresh_token:{token_hash}     -> RefreshTokenData (JSON)
//   - user_refresh_tokens:{user_id}  -> Set<token_hash>
//   - token_family:{family_id}       -> Set<token_hash>
type RefreshTokenCache = cache.RefreshTokenCache

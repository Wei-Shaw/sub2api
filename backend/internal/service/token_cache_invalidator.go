package service

import (
	"context"
	"log/slog"
	"strconv"
)

type TokenCacheInvalidator interface {
	InvalidateToken(ctx context.Context, account *Account) error
}

type CompositeTokenCacheInvalidator struct {
	cache GeminiTokenCache // 统一使用一个缓存接口，通过缓存键前缀区分平台
}

func NewCompositeTokenCacheInvalidator(cache GeminiTokenCache) *CompositeTokenCacheInvalidator {
	return &CompositeTokenCacheInvalidator{
		cache: cache,
	}
}

func (c *CompositeTokenCacheInvalidator) InvalidateToken(ctx context.Context, account *Account) error {
	if c == nil || c.cache == nil || account == nil {
		return nil
	}
	if account.Type != AccountTypeOAuth {
		return nil
	}

	var keysToDelete []string
	accountIDKey := "account:" + strconv.FormatInt(account.ID, 10)

	switch account.Platform {
	case PlatformGemini:
		// Gemini 可能有两种缓存键：project_id 或 account_id
		// 首次获取 token 时可能没有 project_id，之后自动检测到 project_id 后会使用新 key
		// 刷新时需要同时删除两种可能的 key，确保不会遗留旧缓存
		keysToDelete = append(keysToDelete, GeminiTokenCacheKey(account))
		keysToDelete = append(keysToDelete, "gemini:"+accountIDKey)
	case PlatformAntigravity:
		// Antigravity 同样可能有两种缓存键
		keysToDelete = append(keysToDelete, AntigravityTokenCacheKey(account))
		keysToDelete = append(keysToDelete, "ag:"+accountIDKey)
	case PlatformOpenAI:
		keysToDelete = append(keysToDelete, OpenAITokenCacheKey(account))
	case PlatformAnthropic:
		keysToDelete = append(keysToDelete, ClaudeTokenCacheKey(account))
	default:
		return nil
	}

	// 删除所有可能的缓存键（去重后）
	seen := make(map[string]bool)
	for _, key := range keysToDelete {
		if seen[key] {
			continue
		}
		seen[key] = true
		if err := c.cache.DeleteAccessToken(ctx, key); err != nil {
			slog.Warn("token_cache_delete_failed", "key", key, "account_id", account.ID, "error", err)
		}
	}

	return nil
}

// CheckTokenVersion 检查 account 的 token 版本是否已过时，并返回最新的 account
// 用于解决异步刷新任务与请求线程的竞态条件：
// 如果刷新任务已更新 token 并删除缓存，此时请求线程的旧 account 对象不应写入缓存
//
// 返回值:
//   - latestAccount: 从 DB 获取的最新 account（如果查询失败则返回 nil）
//   - isStale: true 表示 token 已过时（应使用 latestAccount），false 表示可以使用当前 account
func CheckTokenVersion(ctx context.Context, account *Account, repo AccountRepository) (latestAccount *Account, isStale bool) {
	if account == nil || repo == nil {
		return nil, false
	}

	currentVersion := account.GetCredentialAsInt64("_token_version")

	latestAccount, err := repo.GetByID(ctx, account.ID)
	if err != nil || latestAccount == nil {
		// 查询失败，默认允许缓存，不返回 latestAccount
		return nil, false
	}

	latestVersion := latestAccount.GetCredentialAsInt64("_token_version")

	// 情况1: 当前 account 没有版本号，但 DB 中已有版本号
	// 说明异步刷新任务已更新 token，当前 account 已过时
	if currentVersion == 0 && latestVersion > 0 {
		slog.Debug("token_version_stale_no_current_version",
			"account_id", account.ID,
			"latest_version", latestVersion)
		return latestAccount, true
	}

	// 情况2: 两边都没有版本号，说明从未被异步刷新过，允许缓存
	if currentVersion == 0 && latestVersion == 0 {
		return latestAccount, false
	}

	// 情况3: 比较版本号，如果 DB 中的版本更新，当前 account 已过时
	if latestVersion > currentVersion {
		slog.Debug("token_version_stale",
			"account_id", account.ID,
			"current_version", currentVersion,
			"latest_version", latestVersion)
		return latestAccount, true
	}

	return latestAccount, false
}

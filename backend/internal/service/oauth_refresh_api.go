package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OAuthRefreshExecutor 各平台实现的 OAuth 刷新执行器
// TokenRefresher 接口的超集：增加了 CacheKey 方法用于分布式锁
type OAuthRefreshExecutor interface {
	TokenRefresher

	// CacheKey 返回用于分布式锁的缓存键（与 TokenProvider 使用的一致）
	CacheKey(account *Account) string
}

const defaultRefreshLockTTL = 60 * time.Second

// OAuthRefreshResult 统一刷新结果
type OAuthRefreshResult struct {
	Refreshed      bool           // 实际执行了刷新
	NewCredentials map[string]any // 刷新后的 credentials（nil 表示未刷新）
	Account        *Account       // 从 DB 重新读取的最新 account
	LockHeld       bool           // 锁被其他 worker 持有（未执行刷新）
}

// OAuthRefreshAPI 统一的 OAuth Token 刷新入口
// 封装分布式锁、进程内互斥锁、DB 重读、已刷新检查、竞争恢复等通用逻辑
type OAuthRefreshAPI struct {
	accountRepo AccountRepository
	tokenCache  GeminiTokenCache // 可选，nil = 无分布式锁
	lockTTL     time.Duration
	localLocks  sync.Map // key: cacheKey string -> value: *sync.Mutex
}

// NewOAuthRefreshAPI 创建统一刷新 API
// 可选传入 lockTTL 覆盖默认的 60s 分布式锁 TTL
func NewOAuthRefreshAPI(accountRepo AccountRepository, tokenCache GeminiTokenCache, lockTTL ...time.Duration) *OAuthRefreshAPI {
	ttl := defaultRefreshLockTTL
	if len(lockTTL) > 0 && lockTTL[0] > 0 {
		ttl = lockTTL[0]
	}
	return &OAuthRefreshAPI{
		accountRepo: accountRepo,
		tokenCache:  tokenCache,
		lockTTL:     ttl,
	}
}

// getLocalLock 返回指定 cacheKey 的进程内互斥锁
func (api *OAuthRefreshAPI) getLocalLock(cacheKey string) *sync.Mutex {
	actual, _ := api.localLocks.LoadOrStore(cacheKey, &sync.Mutex{})
	mu, ok := actual.(*sync.Mutex)
	if !ok {
		mu = &sync.Mutex{}
		api.localLocks.Store(cacheKey, mu)
	}
	return mu
}

// RefreshIfNeeded 在分布式锁保护下按需刷新 OAuth token
//
// 流程:
//  1. 获取分布式锁
//  2. 从 DB 重读最新 account（防止使用过时的 refresh_token）
//  3. 二次检查是否仍需刷新
//  4. 调用 executor.Refresh() 执行平台特定刷新逻辑
//  5. 设置 _token_version + 更新 DB
//  6. 释放锁
func (api *OAuthRefreshAPI) RefreshIfNeeded(
	ctx context.Context,
	account *Account,
	executor OAuthRefreshExecutor,
	refreshWindow time.Duration,
) (*OAuthRefreshResult, error) {
	cacheKey := executor.CacheKey(account)

	// 0. 获取进程内互斥锁（防止同一进程内的并发刷新竞争）
	localMu := api.getLocalLock(cacheKey)
	localMu.Lock()
	defer localMu.Unlock()

	// 1. 获取分布式锁
	lockAcquired := false
	if api.tokenCache != nil {
		acquired, lockErr := api.tokenCache.AcquireRefreshLock(ctx, cacheKey, api.lockTTL)
		if lockErr != nil {
			// Redis 错误，降级为无锁刷新（进程内互斥锁仍生效）
			slog.Warn("oauth_refresh_lock_failed_degraded",
				"account_id", account.ID,
				"cache_key", cacheKey,
				"error", lockErr,
			)
		} else if !acquired {
			// 锁被其他 worker 持有
			return &OAuthRefreshResult{LockHeld: true}, nil
		} else {
			lockAcquired = true
			defer func() { _ = api.tokenCache.ReleaseRefreshLock(ctx, cacheKey) }()
		}
	}

	// 2. 从 DB 重读最新 account（锁保护下，确保使用最新的 refresh_token）
	freshAccount, err := api.accountRepo.GetByID(ctx, account.ID)
	if err != nil {
		slog.Warn("oauth_refresh_db_reread_failed",
			"account_id", account.ID,
			"error", err,
		)
		// 降级使用传入的 account
		freshAccount = account
	} else if freshAccount == nil {
		freshAccount = account
	}

	// 3. 二次检查是否仍需刷新（另一条路径可能已刷新）
	if !executor.NeedsRefresh(freshAccount, refreshWindow) {
		return &OAuthRefreshResult{
			Account: freshAccount,
		}, nil
	}

	// 4. 执行平台特定刷新逻辑
	newCredentials, refreshErr := executor.Refresh(ctx, freshAccount)
	if refreshErr != nil {
		// 竞争恢复：invalid_grant 可能是另一个 worker 已消费了旧 refresh_token
		// 重新读取 DB，如果 refresh_token 已更新则说明是竞争，返回成功
		if isInvalidGrantError(refreshErr) {
			if recoveredAccount, recovered := api.tryRecoverFromRefreshRace(ctx, freshAccount); recovered {
				slog.Info("oauth_refresh_race_recovered",
					"account_id", freshAccount.ID,
					"platform", freshAccount.Platform,
				)
				return &OAuthRefreshResult{
					Account: recoveredAccount,
				}, nil
			}
		}
		return nil, refreshErr
	}

	// 5. 设置版本号 + 更新 DB
	if newCredentials != nil {
		newCredentials["_token_version"] = time.Now().UnixMilli()
		if updateErr := persistAccountCredentials(ctx, api.accountRepo, freshAccount, newCredentials); updateErr != nil {
			slog.Error("oauth_refresh_update_failed",
				"account_id", freshAccount.ID,
				"error", updateErr,
			)
			return nil, fmt.Errorf("oauth refresh succeeded but DB update failed: %w", updateErr)
		}
	}

	_ = lockAcquired // suppress unused warning when tokenCache is nil

	return &OAuthRefreshResult{
		Refreshed:      true,
		NewCredentials: newCredentials,
		Account:        freshAccount,
	}, nil
}

// isInvalidGrantError 检查错误是否为 invalid_grant
func isInvalidGrantError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "invalid_grant")
}

// tryRecoverFromRefreshRace 在 invalid_grant 错误后尝试竞争恢复
// 重新读取 DB，如果 refresh_token 已改变（说明另一个 worker 成功刷新），则返回更新后的 account
func (api *OAuthRefreshAPI) tryRecoverFromRefreshRace(ctx context.Context, usedAccount *Account) (*Account, bool) {
	if api.accountRepo == nil {
		return nil, false
	}
	reReadAccount, err := api.accountRepo.GetByID(ctx, usedAccount.ID)
	if err != nil || reReadAccount == nil {
		return nil, false
	}
	usedRT := usedAccount.GetCredential("refresh_token")
	currentRT := reReadAccount.GetCredential("refresh_token")
	if usedRT == "" || currentRT == "" {
		return nil, false
	}
	// refresh_token 不同 → 另一个 worker 已成功刷新
	if usedRT != currentRT {
		return reReadAccount, true
	}
	return nil, false
}

// MergeCredentials 将旧 credentials 中不存在于新 map 的字段保留到新 map 中
func MergeCredentials(oldCreds, newCreds map[string]any) map[string]any {
	if newCreds == nil {
		newCreds = make(map[string]any)
	}
	for k, v := range oldCreds {
		if _, exists := newCreds[k]; !exists {
			newCreds[k] = v
		}
	}
	return newCreds
}

// BuildClaudeAccountCredentials 为 Claude 平台构建 OAuth credentials map
// 消除 Claude 平台没有 BuildAccountCredentials 方法的问题
func BuildClaudeAccountCredentials(tokenInfo *TokenInfo) map[string]any {
	creds := map[string]any{
		"access_token": tokenInfo.AccessToken,
		"token_type":   tokenInfo.TokenType,
		"expires_in":   strconv.FormatInt(tokenInfo.ExpiresIn, 10),
		"expires_at":   strconv.FormatInt(tokenInfo.ExpiresAt, 10),
	}
	if tokenInfo.RefreshToken != "" {
		creds["refresh_token"] = tokenInfo.RefreshToken
	}
	if tokenInfo.Scope != "" {
		creds["scope"] = tokenInfo.Scope
	}
	return creds
}

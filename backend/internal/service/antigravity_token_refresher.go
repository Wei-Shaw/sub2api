package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

const (
	// antigravityRefreshWindow Antigravity token 提前刷新窗口：15分钟
	// Google OAuth token 有效期55分钟，提前15分钟刷新
	antigravityRefreshWindow = 15 * time.Minute
)

// AntigravityTokenRefresher 实现 TokenRefresher 接口
type AntigravityTokenRefresher struct {
	antigravityOAuthService *AntigravityOAuthService
}

func NewAntigravityTokenRefresher(antigravityOAuthService *AntigravityOAuthService) *AntigravityTokenRefresher {
	return &AntigravityTokenRefresher{
		antigravityOAuthService: antigravityOAuthService,
	}
}

// CacheKey 返回用于分布式锁的缓存键
func (r *AntigravityTokenRefresher) CacheKey(account *Account) string {
	return AntigravityTokenCacheKey(account)
}

// CanRefresh 检查是否可以刷新此账户
func (r *AntigravityTokenRefresher) CanRefresh(account *Account) bool {
	return account.Platform == PlatformAntigravity && account.Type == AccountTypeOAuth
}

// NeedsRefresh 检查账户是否需要刷新
// Antigravity 使用固定的15分钟刷新窗口，忽略全局配置
func (r *AntigravityTokenRefresher) NeedsRefresh(account *Account, _ time.Duration) bool {
	if !r.CanRefresh(account) {
		return false
	}
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return false
	}
	timeUntilExpiry := time.Until(*expiresAt)
	needsRefresh := timeUntilExpiry < antigravityRefreshWindow
	if needsRefresh {
		fmt.Printf("[AntigravityTokenRefresher] Account %d needs refresh: expires_at=%s, time_until_expiry=%v, window=%v\n",
			account.ID, expiresAt.Format("2006-01-02 15:04:05"), timeUntilExpiry, antigravityRefreshWindow)
	}
	return needsRefresh
}

// Refresh 执行 token 刷新
func (r *AntigravityTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	tokenInfo, err := r.antigravityOAuthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}

	newCredentials := r.antigravityOAuthService.BuildAccountCredentials(tokenInfo)
	// 合并旧的 credentials，保留新 credentials 中不存在的字段
	newCredentials = MergeCredentials(account.Credentials, newCredentials)

	// 特殊处理 project_id：如果新值为空但旧值非空，保留旧值
	// 这确保了即使 LoadCodeAssist 失败，project_id 也不会丢失
	if newProjectID, _ := newCredentials["project_id"].(string); newProjectID == "" {
		if oldProjectID := strings.TrimSpace(account.GetCredential("project_id")); oldProjectID != "" {
			newCredentials["project_id"] = oldProjectID
		}
	}

	// 如果 project_id 获取失败，只记录警告，不返回错误
	// LoadCodeAssist 失败可能是临时网络问题，应该允许重试而不是立即标记为不可重试错误
	// Token 刷新本身是成功的（access_token 和 refresh_token 已更新）
	if tokenInfo.ProjectIDMissing {
		if tokenInfo.ProjectID != "" {
			// 有旧的 project_id，本次获取失败，保留旧值
			log.Printf("[AntigravityTokenRefresher] Account %d: LoadCodeAssist 临时失败，保留旧 project_id", account.ID)
		} else {
			// 从未获取过 project_id，本次也失败，但不返回错误以允许下次重试
			log.Printf("[AntigravityTokenRefresher] Account %d: LoadCodeAssist 失败，project_id 缺失，但 token 已更新，将在下次刷新时重试", account.ID)
		}
	}

	return newCredentials, nil
}

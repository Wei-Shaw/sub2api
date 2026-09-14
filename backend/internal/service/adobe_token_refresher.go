package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
)

// adobeRefreshWindow 是 token 剩余寿命低于该值时触发刷新的阈值。
// IMS token 通常只有约 24 小时，留 30 分钟足够覆盖一轮刷新周期。
const adobeRefreshWindow = 30 * time.Minute

// AdobeTokenRefresher 用账号里的长期 cookie 换取短期 IMS access_token。
//
// 与其它平台的区别：Adobe 没有 refresh_token，长期凭据就是浏览器 cookie。
// cookie 失效只能由用户重新导出，故此类失败必须让账号进 error 态而不是无限重试。
type AdobeTokenRefresher struct {
	clients *adobeClientCache
}

func NewAdobeTokenRefresher() *AdobeTokenRefresher {
	return &AdobeTokenRefresher{clients: &adobeClientCache{}}
}

func (r *AdobeTokenRefresher) CacheKey(account *Account) string {
	if account == nil {
		return ""
	}
	return "adobe:token:" + strconv.FormatInt(account.ID, 10)
}

func (r *AdobeTokenRefresher) CanRefresh(account *Account) bool {
	return account != nil &&
		account.Platform == PlatformAdobe &&
		account.Type == AccountTypeOAuth &&
		strings.TrimSpace(account.GetCredential("cookie")) != ""
}

// NeedsRefresh 在 token 缺失或即将过期时返回 true。
//
// refreshWindow 由调度方传入，但 Adobe token 的寿命远短于多数平台，
// 故取传入值与 adobeRefreshWindow 中较大的那个，避免窗口过小导致刷不及。
func (r *AdobeTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	if !r.CanRefresh(account) {
		return false
	}
	token := strings.TrimSpace(account.GetCredential("access_token"))
	if token == "" {
		return true
	}
	window := refreshWindow
	if window < adobeRefreshWindow {
		window = adobeRefreshWindow
	}
	return adobe.IsTokenExpired(token, window)
}

// Refresh 用 cookie 换新 token，返回保留了原有全部字段的 credentials。
func (r *AdobeTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if !r.CanRefresh(account) {
		return nil, errors.New("account is not a refreshable adobe account")
	}

	client := r.clients.clientForAccount(account)
	// 不查账号信息：刷新是热路径，省掉一次往返；账号信息在账号测试时另行获取。
	result, err := client.RefreshAccessTokenFromCookie(
		ctx, account.GetCredential("cookie"), adobe.RefreshOptions{SkipAccountFetch: true})
	if err != nil {
		var authErr *adobe.AuthError
		if errors.As(err, &authErr) {
			// cookie 已失效，重试没有意义——包装成明确信息让上层把账号置为 error。
			return nil, fmt.Errorf("adobe cookie is no longer valid, re-export it from the browser: %w", err)
		}
		return nil, fmt.Errorf("refresh adobe token: %w", err)
	}

	// 只更新 token 字段：cookie 与 model_mapping 等都在原 credentials 里，
	// 直接返回新 map 会把它们抹掉。
	return MergeCredentials(account.Credentials, map[string]any{
		"access_token": result.AccessToken,
		"expires_at":   adobeTokenExpiryString(result),
	}), nil
}

// adobeTokenExpiryString 取 token 的过期时刻（RFC3339）。
// 优先用 token 自身的 exp；上游只给了 expires_in 时按当前时间推算。
func adobeTokenExpiryString(result *adobe.RefreshResult) string {
	if exp, ok := adobe.DecodeJWTExp(result.AccessToken); ok {
		return time.Unix(exp, 0).UTC().Format(time.RFC3339)
	}
	if result.ExpiresIn > 0 {
		return time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}
	return ""
}

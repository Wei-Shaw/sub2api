package service

import (
	"context"
	"errors"
	"hash/fnv"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

// kiroTokenRefreshSkew 是基础预热窗口：剩余寿命低于此值就刷新。
// Kiro 的 access token 通常 1 小时，提前刷新可让请求路径少走缓存未命中。
const kiroTokenRefreshSkew = 30 * time.Minute

// kiroTokenRefreshJitterMax 把同批导入账号的刷新时刻散开，
// 避免它们在同一个 TokenRefreshService 周期一起打上游。
const kiroTokenRefreshJitterMax = 3 * time.Minute

// kiroTokenRefreshSkewMin 是 jitter 之后的窗口下限。
const kiroTokenRefreshSkewMin = 10 * time.Minute

// KiroTokenRefresher 实现 OAuthRefreshExecutor，接入后台刷新循环。
type KiroTokenRefresher struct {
	oauthService *KiroOAuthService
}

// NewKiroTokenRefresher 创建刷新器。
func NewKiroTokenRefresher(oauthService *KiroOAuthService) *KiroTokenRefresher {
	return &KiroTokenRefresher{oauthService: oauthService}
}

// CacheKey 返回分布式刷新锁使用的键。
func (r *KiroTokenRefresher) CacheKey(account *Account) string {
	return KiroTokenCacheKey(account)
}

// CanRefresh 判断该账号是否由本刷新器负责。
//
// API Key 账号必须排除 —— 它们没有 refresh token，纳入后台循环只会持续报错。
func (r *KiroTokenRefresher) CanRefresh(account *Account) bool {
	if account == nil || account.Platform != PlatformKiro {
		return false
	}
	if account.IsKiroAPIKeyAccount() {
		return false
	}
	return strings.TrimSpace(account.KiroRefreshToken()) != ""
}

// NeedsRefresh 判断是否到了预热刷新的时刻。
func (r *KiroTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	if account == nil || strings.TrimSpace(account.KiroRefreshToken()) == "" {
		return false
	}
	if strings.TrimSpace(account.KiroAccessToken()) == "" {
		return true
	}

	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return true
	}

	if refreshWindow < kiroTokenRefreshSkew {
		refreshWindow = kiroTokenRefreshSkew
	}
	refreshWindow = kiroTokenRefreshWindowWithJitter(account.ID, refreshWindow)

	return time.Until(*expiresAt) < refreshWindow
}

// kiroTokenRefreshWindowWithJitter 按账号 ID 做确定性抖动，
// 用哈希而非随机数，保证测试可复现。
func kiroTokenRefreshWindowWithJitter(accountID int64, refreshWindow time.Duration) time.Duration {
	if accountID <= 0 || refreshWindow <= kiroTokenRefreshSkewMin {
		return refreshWindow
	}

	h := fnv.New32a()
	var b [8]byte
	id := uint64(accountID)
	for i := 0; i < 8; i++ {
		b[i] = byte(id >> (8 * i))
	}
	_, _ = h.Write(b[:])

	jitter := time.Duration(h.Sum32()%uint32(kiroTokenRefreshJitterMax/time.Second)) * time.Second
	out := refreshWindow - jitter
	if out < kiroTokenRefreshSkewMin {
		return kiroTokenRefreshSkewMin
	}
	return out
}

// Refresh 刷新令牌并返回完整的新 credentials。
//
// 必须用 MergeCredentials 保留原有字段 —— machine_id / fake_thinking /
// issuer_url 等都不在刷新响应里，丢失 machine_id 等于每次刷新都换一台设备。
func (r *KiroTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if r == nil || r.oauthService == nil {
		return nil, errors.New("kiro oauth service is not configured")
	}

	ts, err := r.oauthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}

	clientID, clientSecret := account.KiroClientCredentials()
	newCreds := r.oauthService.BuildAccountCredentials(KiroCredentialInput{
		TokenSet:     ts,
		Method:       account.KiroAuthMethod(),
		Region:       account.KiroRegion(),
		IssuerURL:    account.KiroIssuerURL(),
		ClientID:     clientID,
		ClientSecret: clientSecret,
	})

	merged := MergeCredentials(account.Credentials, newCreds)

	// BuildAccountCredentials 内部用 EnsureKiroMachineID 往新 creds 里塞一个
	// machine_id（服务首次授权场景，此时确实没有旧值）。这会让 newCreds 里的
	// key 在 MergeCredentials 的字段优先级里“赢过”账号原有的设备指纹 ——
	// 而设备指纹一旦生成就必须稳定，刷新场景要反过来：原值优先。这里显式把
	// 账号原有的 machine_id 覆盖回去，而不是依赖 MergeCredentials 的空位填充。
	if old := strings.TrimSpace(account.KiroMachineID()); old != "" {
		merged["machine_id"] = old
	}

	// 可选的 profileArn 自动发现（锦上添花，见 DiscoverProfileArn 的文档）：
	// 只在这次刷新之后仍然没有 profile_arn、且是 idc/builder_id 账号时才
	// 尝试一次——social 已经从 token 响应里自动带回，api_key 账号不使用
	// profileArn（BuildAccountCredentials/KiroProfileArn 的既有约定）。
	// 发现失败/账号在已知区域没有可用 profile 都不应该让 token 刷新本身
	// 失败，只记一条 debug 日志，管理端手填入口仍然保留作为兜底。
	if method := account.KiroAuthMethod(); method == kiro.AuthIdC || method == kiro.AuthBuilderID {
		if existing, _ := merged["profile_arn"].(string); strings.TrimSpace(existing) == "" {
			accessToken, _ := merged["access_token"].(string)
			discovered, discErr := r.oauthService.DiscoverProfileArn(ctx, accessToken, account.KiroMachineID(), account.ProxyID)
			if discErr != nil {
				slog.Debug("kiro_profile_arn_discovery_failed", "account_id", account.ID, "auth_method", string(method), "error", discErr)
			} else if discovered != "" {
				merged["profile_arn"] = discovered
			}
		}
	}

	return merged, nil
}

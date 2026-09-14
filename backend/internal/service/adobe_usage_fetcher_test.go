//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/stretchr/testify/require"
)

// stubAdobeCreditsClient 是一个极小的假 adobe.Client：只回一份预置的 balance。
// 用它跳开真实的 HTTP + Transport 层，专测 fetcher 的解析、cooldown 与降级逻辑。
type stubAdobeCreditsClient struct {
	balance   *adobe.CreditsBalance
	err       error
	calls     atomic.Int32
	lastToken atomic.Value // string
}

func (c *stubAdobeCreditsClient) FetchCreditsBalance(_ context.Context, token string) (*adobe.CreditsBalance, error) {
	c.calls.Add(1)
	c.lastToken.Store(token)
	if c.err != nil {
		return nil, c.err
	}
	return c.balance, nil
}

func (c *stubAdobeCreditsClient) lastAccessToken() string {
	token, _ := c.lastToken.Load().(string)
	return token
}

// adobeUsageTestRepo 记录 SetTempUnschedulable 调用，用于断言调度联动。
type adobeUsageTestRepo struct {
	AccountRepository
	mu      sync.Mutex
	calls   []adobeCooldownCall
	updated map[string]any
}

type adobeCooldownCall struct {
	accountID int64
	until     time.Time
	reason    string
}

func (r *adobeUsageTestRepo) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, adobeCooldownCall{accountID: id, until: until, reason: reason})
	return nil
}

func (r *adobeUsageTestRepo) UpdateCredentials(_ context.Context, _ int64, credentials map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updated = credentials
	return nil
}

// int64Ptr 已在 ops_openai_token_stats_test.go 定义

func newAdobeUsageTestService(t *testing.T, repo AccountRepository, client adobeCreditsClient) *AccountUsageService {
	t.Helper()
	svc := &AccountUsageService{
		accountRepo: repo,
		cache:       NewUsageCache(),
	}
	prev := adobeUsageClientFactory
	adobeUsageClientFactory = func(*Account) adobeCreditsClient { return client }
	t.Cleanup(func() { adobeUsageClientFactory = prev })
	return svc
}

func adobeAccountWithToken() *Account {
	return &Account{
		ID:       9,
		Platform: domain.PlatformAdobe,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"cookie":       "aux_sid=...",
			"access_token": "test-access-token",
		},
	}
}

func TestGetAdobeUsageSkipsRelayAccounts(t *testing.T) {
	client := &stubAdobeCreditsClient{
		err: errors.New("should not be called"),
	}
	repo := &adobeUsageTestRepo{}
	svc := newAdobeUsageTestService(t, repo, client)

	info, err := svc.getAdobeUsage(context.Background(), &Account{
		ID:       11,
		Platform: domain.PlatformAdobe,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://relay.example",
		},
	}, "active", true)
	require.NoError(t, err)
	require.Zero(t, client.calls.Load())
	require.Empty(t, info.Error)
	require.Empty(t, repo.calls)
}

// available=0 时精准摆号：until 应等于上游给的 availableUntil，不是兜底 30min。
func TestGetAdobeUsageExhaustsScheduling(t *testing.T) {
	reset := time.Now().Add(6 * time.Hour).UTC().Round(time.Millisecond)
	client := &stubAdobeCreditsClient{
		balance: &adobe.CreditsBalance{
			Total: int64Ptr(10), Used: int64Ptr(10), Available: int64Ptr(0),
			AvailableUntil: reset.Format(time.RFC3339Nano),
			PlanCap:        "FREE",
			CreditPools: []adobe.CreditPool{{
				Name:  "firefly_free_credit",
				Total: int64Ptr(10), Used: int64Ptr(10), Available: int64Ptr(0),
			}},
		},
	}
	repo := &adobeUsageTestRepo{}
	svc := newAdobeUsageTestService(t, repo, client)

	info, err := svc.getAdobeUsage(context.Background(), adobeAccountWithToken(), "active", true)
	require.NoError(t, err)
	require.Equal(t, "FREE", info.AdobePlanCap)
	require.NotNil(t, info.AdobeCredit)
	require.Equal(t, float64(10), info.AdobeCredit.UsageLimit)
	require.Equal(t, float64(100), info.AdobeCredit.PercentageUsed)
	require.Len(t, info.AdobeCreditPools, 1)
	require.NotNil(t, info.AdobeCreditResetAt)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.calls, 1, "available=0 必须调用一次 SetTempUnschedulable")
	require.Equal(t, int64(9), repo.calls[0].accountID)
	require.Equal(t, "adobe_credits_exhausted", repo.calls[0].reason)
	require.WithinDuration(t, reset, repo.calls[0].until, time.Second,
		"until 应精准落到 availableUntil，不是走 30min 兜底")
}

// 还有额度时不能碰账号状态——否则免费账号头 10 次都会被误摆。
func TestGetAdobeUsageDoesNotTouchSchedulingWhenAvailable(t *testing.T) {
	client := &stubAdobeCreditsClient{
		balance: &adobe.CreditsBalance{
			Total: int64Ptr(10), Used: int64Ptr(2), Available: int64Ptr(8),
			PlanCap: "FREE",
		},
	}
	repo := &adobeUsageTestRepo{}
	svc := newAdobeUsageTestService(t, repo, client)

	info, err := svc.getAdobeUsage(context.Background(), adobeAccountWithToken(), "active", true)
	require.NoError(t, err)
	require.Equal(t, float64(20), info.AdobeCredit.PercentageUsed)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Empty(t, repo.calls, "有余额时不该摆号")
}

// availableUntil 缺失时走 30min 兜底——避免上游临时不给这个字段就永久摆号。
func TestGetAdobeUsageExhaustedFallsBackWhenResetMissing(t *testing.T) {
	client := &stubAdobeCreditsClient{
		balance: &adobe.CreditsBalance{
			Total: int64Ptr(10), Used: int64Ptr(10), Available: int64Ptr(0),
			// AvailableUntil 缺
		},
	}
	repo := &adobeUsageTestRepo{}
	svc := newAdobeUsageTestService(t, repo, client)

	_, err := svc.getAdobeUsage(context.Background(), adobeAccountWithToken(), "active", true)
	require.NoError(t, err)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.calls, 1)
	require.WithinDuration(t, time.Now().Add(adobeCreditsExhaustedFallback),
		repo.calls[0].until, 2*time.Second)
}

// 上游 401：降级信息里应带 unauthenticated + NeedsReauth，但**不能**摆号——
// token 过期与配额耗尽是两回事，误摆会掩盖凭据问题。
func TestGetAdobeUsageAuthErrorDoesNotCooldown(t *testing.T) {
	client := &stubAdobeCreditsClient{err: adobe.NewAuthError("expired", http.StatusUnauthorized)}
	repo := &adobeUsageTestRepo{}
	svc := newAdobeUsageTestService(t, repo, client)

	info, err := svc.getAdobeUsage(context.Background(), adobeAccountWithToken(), "active", true)
	require.NoError(t, err, "降级返回不该抛错")
	require.Equal(t, errorCodeUnauthenticated, info.ErrorCode)
	require.True(t, info.NeedsReauth)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Empty(t, repo.calls)
}

// 权益不足不是 cookie 坏了：不能标 NeedsReauth，也不能冷却账号。
func TestGetAdobeUsageNotEntitledDoesNotNeedReauth(t *testing.T) {
	client := &stubAdobeCreditsClient{err: adobe.NewNotEntitledError(
		`credits request failed: 403 {"error_code":"model_not_entitled"}`, http.StatusForbidden, "")}
	repo := &adobeUsageTestRepo{}
	svc := newAdobeUsageTestService(t, repo, client)

	info, err := svc.getAdobeUsage(context.Background(), adobeAccountWithToken(), "active", true)
	require.NoError(t, err)
	require.Equal(t, errorCodeNetworkError, info.ErrorCode)
	require.False(t, info.NeedsReauth)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Empty(t, repo.calls)
}

// 无 cookie 且无 token：无法换票，降级但不摆号。
func TestGetAdobeUsageMissingTokenDegradesWithoutCooldown(t *testing.T) {
	client := &stubAdobeCreditsClient{}
	repo := &adobeUsageTestRepo{}
	svc := newAdobeUsageTestService(t, repo, client)

	account := &Account{
		ID:          9,
		Platform:    domain.PlatformAdobe,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{},
	}

	info, err := svc.getAdobeUsage(context.Background(), account, "active", true)
	require.NoError(t, err)
	require.Equal(t, errorCodeNetworkError, info.ErrorCode)
	require.Zero(t, client.calls.Load(), "无凭据应短路，不发上游请求")

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Empty(t, repo.calls)
}

// 只填 cookie 的新账号：用量路径当场换 IMS token，再拉 credits。
func TestGetAdobeUsageRefreshesEmptyTokenFromCookie(t *testing.T) {
	newToken := adobeTestToken(t, map[string]any{
		"user_id": "u1", "exp": time.Now().Add(24 * time.Hour).Unix(),
	})
	refresher := newAdobeTestRefresher(t, func(*adobe.Request, int) (*adobe.Response, error) {
		body, _ := json.Marshal(map[string]any{"access_token": newToken, "expires_in": 86400})
		return &adobe.Response{StatusCode: 200, Headers: map[string]string{}, Body: body}, nil
	})
	client := &stubAdobeCreditsClient{
		balance: &adobe.CreditsBalance{
			Total: int64Ptr(15), Used: int64Ptr(3), Available: int64Ptr(12),
			PlanCap: "HARD",
		},
	}
	repo := &adobeUsageTestRepo{}
	svc := newAdobeUsageTestService(t, repo, client)
	svc.adobeTokenRefresher = refresher

	account := &Account{
		ID:       9,
		Platform: domain.PlatformAdobe,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"cookie": "aux_sid=abc",
		},
	}

	info, err := svc.getAdobeUsage(context.Background(), account, "active", true)
	require.NoError(t, err)
	require.Equal(t, "HARD", info.AdobePlanCap)
	require.Equal(t, newToken, client.lastAccessToken())
	require.Equal(t, int32(1), client.calls.Load())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Equal(t, newToken, repo.updated["access_token"])
	require.Equal(t, newToken, account.GetCredential("access_token"))
}

// singleflight：并发同一账号只打上游一次。
//
// 用 gate channel 卡住上游调用直到所有 goroutine 都进入过 singleflight——如果不加
// 这道同步，goroutine 可能先后串行执行完，singleflight 就没机会合并（每次都是新一轮 flight）。
func TestGetAdobeUsageSingleflight(t *testing.T) {
	gate := make(chan struct{})
	client := &blockingAdobeClient{
		balance: &adobe.CreditsBalance{
			Total: int64Ptr(10), Used: int64Ptr(0), Available: int64Ptr(10),
		},
		gate: gate,
	}
	repo := &adobeUsageTestRepo{}
	svc := newAdobeUsageTestService(t, repo, client)
	account := adobeAccountWithToken()

	var wg sync.WaitGroup
	started := make(chan struct{}, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			started <- struct{}{}
			_, _ = svc.getAdobeUsage(context.Background(), account, "active", true)
		}()
	}
	// 等 8 个 goroutine 都进入 svc.getAdobeUsage（至少已 signal started）
	for i := 0; i < 8; i++ {
		<-started
	}
	// 再等一会儿让第一个 goroutine 真的抢到 singleflight 令牌并开始 upstream 调用。
	time.Sleep(50 * time.Millisecond)
	close(gate) // 放行 upstream 请求
	wg.Wait()
	require.Equal(t, int32(1), client.calls.Load(),
		"singleflight 应把 8 次并发合并成上游 1 次")
}

// 第一次 force 拉上游；随后 force=false 必须吃 3 分钟缓存，不再出站。
func TestGetAdobeUsageCachesWithoutForce(t *testing.T) {
	client := &stubAdobeCreditsClient{
		balance: &adobe.CreditsBalance{
			Total: int64Ptr(10), Used: int64Ptr(1), Available: int64Ptr(9),
		},
	}
	svc := newAdobeUsageTestService(t, &adobeUsageTestRepo{}, client)
	account := adobeAccountWithToken()

	first, err := svc.getAdobeUsage(context.Background(), account, "active", true)
	require.NoError(t, err)
	require.Equal(t, float64(10), first.AdobeCredit.UsageLimit)
	require.Equal(t, int32(1), client.calls.Load())

	second, err := svc.getAdobeUsage(context.Background(), account, "passive", false)
	require.NoError(t, err)
	require.Equal(t, first.AdobeCredit.CurrentUsage, second.AdobeCredit.CurrentUsage)
	require.Equal(t, int32(1), client.calls.Load(), "force=false 应命中缓存")
}

// 表头手动刷新会带 force=true；必须绕过 TTL，否则用户看到的还是旧额度。
func TestGetAdobeUsageForceBypassesCache(t *testing.T) {
	client := &stubAdobeCreditsClient{
		balance: &adobe.CreditsBalance{
			Total: int64Ptr(10), Used: int64Ptr(1), Available: int64Ptr(9),
		},
	}
	svc := newAdobeUsageTestService(t, &adobeUsageTestRepo{}, client)
	account := adobeAccountWithToken()

	_, err := svc.getAdobeUsage(context.Background(), account, "active", true)
	require.NoError(t, err)
	require.Equal(t, int32(1), client.calls.Load())

	client.balance = &adobe.CreditsBalance{
		Total: int64Ptr(10), Used: int64Ptr(4), Available: int64Ptr(6),
	}
	forced, err := svc.getAdobeUsage(context.Background(), account, "active", true)
	require.NoError(t, err)
	require.Equal(t, int32(2), client.calls.Load(), "force=true 必须绕过缓存")
	require.Equal(t, float64(40), forced.AdobeCredit.PercentageUsed)
}

type blockingAdobeClient struct {
	balance *adobe.CreditsBalance
	gate    chan struct{}
	calls   atomic.Int32
}

func (c *blockingAdobeClient) FetchCreditsBalance(_ context.Context, _ string) (*adobe.CreditsBalance, error) {
	<-c.gate
	c.calls.Add(1)
	return c.balance, nil
}

// pkg 层返回的错误可能是包着 fmt.Errorf 的——降级映射必须走 errors.As 解包。
func TestBuildAdobeDegradedUsageUnwrapsWrappedErrors(t *testing.T) {
	info := buildAdobeDegradedUsage(errors.New("plain"))
	require.Equal(t, errorCodeNetworkError, info.ErrorCode)

	quota := buildAdobeDegradedUsage(
		&wrappedAdobeErr{inner: adobe.NewQuotaExhaustedError("q", http.StatusForbidden)})
	require.Equal(t, errorCodeRateLimited, quota.ErrorCode)
}

type wrappedAdobeErr struct{ inner error }

func (w *wrappedAdobeErr) Error() string { return w.inner.Error() }
func (w *wrappedAdobeErr) Unwrap() error { return w.inner }

//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/stretchr/testify/require"
)

// adobeGatewayCredsRepo 是内存版账号仓库：GetByID 返回副本，
// UpdateAdobeTokenIfCookieUnchanged 按 cookie 条件合并 token 字段，与 SQL 实现语义一致。
type adobeGatewayCredsRepo struct {
	AccountRepository
	mu               sync.Mutex
	account          *Account
	getByIDErr       error
	beforeCAS        func(account *Account)
	casCalls         int
	setErrorCalls    int
	lastErrorMessage string
	setErrorErr      error
}

func newAdobeGatewayCredsRepo(account *Account) *adobeGatewayCredsRepo {
	return &adobeGatewayCredsRepo{account: snapshotOAuthRefreshAccount(account)}
}

func (r *adobeGatewayCredsRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
	}
	if r.account == nil || r.account.ID != id {
		return nil, ErrAccountNotFound
	}
	return snapshotOAuthRefreshAccount(r.account), nil
}

func (r *adobeGatewayCredsRepo) UpdateAdobeTokenIfCookieUnchanged(
	_ context.Context, id int64, expectedCookie string, tokenFields map[string]any,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.casCalls++
	if r.beforeCAS != nil {
		r.beforeCAS(r.account)
	}
	if r.account == nil || r.account.ID != id || r.account.GetCredential("cookie") != expectedCookie {
		return false, nil
	}
	merged := shallowCopyMap(r.account.Credentials)
	for key, value := range tokenFields {
		merged[key] = value
	}
	r.account.Credentials = merged
	return true, nil
}

func (r *adobeGatewayCredsRepo) SetError(_ context.Context, _ int64, errorMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setErrorCalls++
	r.lastErrorMessage = errorMsg
	return r.setErrorErr
}

func (r *adobeGatewayCredsRepo) storedCredential(key string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.account.GetCredential(key)
}

// adobeLockHeldTokenCache 模拟另一个实例持有 Redis 刷新锁。
type adobeLockHeldTokenCache struct {
	GeminiTokenCache
}

func (adobeLockHeldTokenCache) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	return false, nil
}

func adobeGatewayTestAccount(id int64, credentials map[string]any) *Account {
	return &Account{
		ID: id, Platform: PlatformAdobe, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: credentials,
	}
}

func adobeIMSTokenHandler(token string, calls *atomic.Int32) func(*adobe.Request, int) (*adobe.Response, error) {
	return func(*adobe.Request, int) (*adobe.Response, error) {
		if calls != nil {
			calls.Add(1)
		}
		body, _ := json.Marshal(map[string]any{"access_token": token, "expires_in": 86400})
		return &adobe.Response{StatusCode: 200, Headers: map[string]string{}, Body: body}, nil
	}
}

func adobeIMSStatusHandler(status int) func(*adobe.Request, int) (*adobe.Response, error) {
	return adobeIMSStatusBodyHandler(status, "{}")
}

func adobeIMSStatusBodyHandler(status int, body string) func(*adobe.Request, int) (*adobe.Response, error) {
	return func(*adobe.Request, int) (*adobe.Response, error) {
		return &adobe.Response{StatusCode: status, Headers: map[string]string{}, Body: []byte(body)}, nil
	}
}

func adobeIMSMustNotBeCalled(t *testing.T) func(*adobe.Request, int) (*adobe.Response, error) {
	return func(*adobe.Request, int) (*adobe.Response, error) {
		t.Error("IMS must not be called")
		return nil, errors.New("unexpected IMS call")
	}
}

func TestGetOAuthTokenRefreshesExpiredAdobeToken(t *testing.T) {
	newToken := adobeTestToken(t, map[string]any{
		"user_id": "u1", "exp": time.Now().Add(24 * time.Hour).Unix(),
	})
	stale := adobeTestToken(t, map[string]any{"exp": time.Now().Add(-time.Hour).Unix()})
	account := adobeGatewayTestAccount(11, map[string]any{
		"cookie": "aux_sid=abc", "access_token": stale, "model_mapping": map[string]any{"a": "b"},
	})
	repo := newAdobeGatewayCredsRepo(account)
	refresher := newAdobeTestRefresher(t, adobeIMSTokenHandler(newToken, nil))
	svc := &GatewayService{accountRepo: repo, adobeTokenProvider: newAdobeTokenProvider(repo, nil, refresher)}

	token, kind, err := svc.getOAuthToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "oauth", kind)
	require.Equal(t, newToken, token)
	require.Equal(t, newToken, repo.storedCredential("access_token"))
	require.Equal(t, "aux_sid=abc", repo.storedCredential("cookie"))
	require.Equal(t, stale, account.GetCredential("access_token"), "caller's scheduler snapshot must not be mutated")
}

func TestGetOAuthTokenSkipsFreshAdobeToken(t *testing.T) {
	fresh := adobeTestToken(t, map[string]any{"exp": time.Now().Add(24 * time.Hour).Unix()})
	refresher := newAdobeTestRefresher(t, adobeIMSMustNotBeCalled(t))
	svc := &GatewayService{adobeTokenProvider: newAdobeTokenProvider(nil, nil, refresher)}

	token, kind, err := svc.getOAuthToken(context.Background(), adobeGatewayTestAccount(12, map[string]any{
		"cookie": "aux_sid=abc", "access_token": fresh,
	}))
	require.NoError(t, err)
	require.Equal(t, "oauth", kind)
	require.Equal(t, fresh, token)
}

func TestGetOAuthTokenReturnsExpiredAdobeTokenWithoutCookie(t *testing.T) {
	stale := adobeTestToken(t, map[string]any{"exp": time.Now().Add(-time.Hour).Unix()})
	refresher := newAdobeTestRefresher(t, adobeIMSMustNotBeCalled(t))
	svc := &GatewayService{adobeTokenProvider: newAdobeTokenProvider(nil, nil, refresher)}

	token, kind, err := svc.getOAuthToken(context.Background(), adobeGatewayTestAccount(13, map[string]any{
		"access_token": stale,
	}))
	require.NoError(t, err)
	require.Equal(t, "oauth", kind)
	require.Equal(t, stale, token)
}

func TestAdobeTokenProviderMarksDeadCookieAsError(t *testing.T) {
	account := adobeGatewayTestAccount(14, map[string]any{"cookie": "dead"})
	repo := newAdobeGatewayCredsRepo(account)
	provider := newAdobeTokenProvider(repo, nil, newAdobeTestRefresher(t, adobeIMSStatusHandler(401)))

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Empty(t, token)
	var authErr *adobe.AuthError
	require.True(t, errors.As(err, &authErr))
	require.Equal(t, 1, repo.setErrorCalls)
	require.Contains(t, repo.lastErrorMessage, "non-retryable")
	require.Zero(t, repo.casCalls, "dead cookie must not persist a new access_token")
}

// cookie 明确失效时即便旧 token 还没过期也返回错误：账号需要管理员重新导出 cookie。
func TestAdobeTokenProviderDeadCookieFailsEvenWithUsableToken(t *testing.T) {
	usable := adobeTestToken(t, map[string]any{"exp": time.Now().Add(10 * time.Minute).Unix()})
	account := adobeGatewayTestAccount(21, map[string]any{"cookie": "dead", "access_token": usable})
	repo := newAdobeGatewayCredsRepo(account)
	provider := newAdobeTokenProvider(repo, nil, newAdobeTestRefresher(t, adobeIMSStatusHandler(401)))

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Empty(t, token)
	var authErr *adobe.AuthError
	require.True(t, errors.As(err, &authErr))
	require.Equal(t, 1, repo.setErrorCalls)
}

// adobeConditionalErrorCredsRepo 让内存仓库支持按 cookie 条件置错。
type adobeConditionalErrorCredsRepo struct {
	*adobeGatewayCredsRepo
	conditionalCalls int
}

func (r *adobeConditionalErrorCredsRepo) SetAdobeErrorIfCookieUnchanged(
	_ context.Context, id int64, expectedCookie string, errorMsg string,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conditionalCalls++
	if r.account == nil || r.account.ID != id || r.account.GetCredential("cookie") != expectedCookie {
		return false, nil
	}
	r.account.Status = StatusError
	r.lastErrorMessage = errorMsg
	return true, nil
}

// IMS 往返期间管理员换了新 cookie：旧 cookie 的失效结论不能把账号置 error。
func TestAdobeTokenProviderSkipsErrorWhenCookieReplaced(t *testing.T) {
	account := adobeGatewayTestAccount(23, map[string]any{"cookie": "dead"})
	base := newAdobeGatewayCredsRepo(account)
	repo := &adobeConditionalErrorCredsRepo{adobeGatewayCredsRepo: base}
	refresher := newAdobeTestRefresher(t, func(*adobe.Request, int) (*adobe.Response, error) {
		base.mu.Lock()
		base.account.Credentials = map[string]any{"cookie": "fresh"}
		base.mu.Unlock()
		return &adobe.Response{StatusCode: 401, Headers: map[string]string{}, Body: []byte("{}")}, nil
	})
	provider := newAdobeTokenProvider(repo, nil, refresher)

	_, err := provider.GetAccessToken(context.Background(), account)
	var authErr *adobe.AuthError
	require.True(t, errors.As(err, &authErr))
	require.Equal(t, 1, repo.conditionalCalls)
	require.Zero(t, base.setErrorCalls, "conditional repository must replace the unconditional SetError")
	require.Equal(t, StatusActive, base.account.Status)
}

func TestAdobeTokenProviderStillFailsWhenSetErrorFails(t *testing.T) {
	account := adobeGatewayTestAccount(15, map[string]any{"cookie": "dead"})
	repo := newAdobeGatewayCredsRepo(account)
	repo.setErrorErr = errors.New("db down")
	provider := newAdobeTokenProvider(repo, nil, newAdobeTestRefresher(t, adobeIMSStatusHandler(401)))

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Empty(t, token)
	var authErr *adobe.AuthError
	require.True(t, errors.As(err, &authErr), "SetError failure must not hide the IMS AuthError")
	require.Equal(t, 1, repo.setErrorCalls)
}

func TestAdobeTokenProviderDoesNotMarkTemporaryIMSFailure(t *testing.T) {
	account := adobeGatewayTestAccount(16, map[string]any{"cookie": "aux_sid=abc"})
	repo := newAdobeGatewayCredsRepo(account)
	provider := newAdobeTokenProvider(repo, nil, newAdobeTestRefresher(t, adobeIMSStatusHandler(503)))

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Empty(t, token)
	var tempErr *adobe.UpstreamTemporaryError
	require.True(t, errors.As(err, &tempErr))
	require.Zero(t, repo.setErrorCalls, "IMS 5xx must stay retryable")
}

// 非强信号的 IMS 拒绝（全局 client_id 错误、其它 JSON 错误码）只让当次请求换号：
// 不置 error，旧 token 仍可用就继续用。
func TestAdobeTokenProviderWeakIMSRejectionKeepsAccountState(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"403 access_denied":  {status: 403, body: `{"error":"access_denied"}`},
		"403 invalid_client": {status: 403, body: `{"error":"invalid_client"}`},
		"401 invalid_client": {status: 401, body: `{"error_code":"invalid_client"}`},
	} {
		t.Run(name, func(t *testing.T) {
			usable := adobeTestToken(t, map[string]any{"exp": time.Now().Add(10 * time.Minute).Unix()})
			account := adobeGatewayTestAccount(30, map[string]any{"cookie": "aux_sid=abc", "access_token": usable})
			repo := newAdobeGatewayCredsRepo(account)
			provider := newAdobeTokenProvider(repo, nil, newAdobeTestRefresher(t, adobeIMSStatusBodyHandler(tc.status, tc.body)))

			token, err := provider.GetAccessToken(context.Background(), account)
			require.NoError(t, err)
			require.Equal(t, usable, token)
			require.Zero(t, repo.setErrorCalls)
		})
	}

	t.Run("无可用旧 token 时返回临时错误且不置 error", func(t *testing.T) {
		account := adobeGatewayTestAccount(31, map[string]any{"cookie": "aux_sid=abc"})
		repo := newAdobeGatewayCredsRepo(account)
		provider := newAdobeTokenProvider(repo, nil, newAdobeTestRefresher(t, adobeIMSStatusBodyHandler(403, `{"error":"access_denied"}`)))

		_, err := provider.GetAccessToken(context.Background(), account)
		var tempErr *adobe.UpstreamTemporaryError
		require.True(t, errors.As(err, &tempErr))
		require.Zero(t, repo.setErrorCalls)
	})
}

// IMS 抖动时，还有十几分钟寿命的旧 token 继续服务，账号不被提前跳过。
func TestAdobeTokenProviderTemporaryFailureKeepsUsableToken(t *testing.T) {
	usable := adobeTestToken(t, map[string]any{"exp": time.Now().Add(10 * time.Minute).Unix()})
	account := adobeGatewayTestAccount(22, map[string]any{"cookie": "aux_sid=abc", "access_token": usable})
	repo := newAdobeGatewayCredsRepo(account)
	provider := newAdobeTokenProvider(repo, nil, newAdobeTestRefresher(t, adobeIMSStatusHandler(503)))

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, usable, token)
	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.casCalls)
}

func TestAdobeTokenProviderConcurrentRequestsRefreshOnce(t *testing.T) {
	newToken := adobeTestToken(t, map[string]any{"exp": time.Now().Add(24 * time.Hour).Unix()})
	stale := adobeTestToken(t, map[string]any{"exp": time.Now().Add(-time.Hour).Unix()})
	account := adobeGatewayTestAccount(23, map[string]any{"cookie": "aux_sid=abc", "access_token": stale})
	repo := newAdobeGatewayCredsRepo(account)
	var imsCalls atomic.Int32
	provider := newAdobeTokenProvider(repo, nil, newAdobeTestRefresher(t, adobeIMSTokenHandler(newToken, &imsCalls)))

	const workers = 8
	start := make(chan struct{})
	tokens := make([]string, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			// 每个请求持有各自的调度快照（都还是旧 token）。
			tokens[i], errs[i] = provider.GetAccessToken(context.Background(), snapshotOAuthRefreshAccount(account))
		}(i)
	}
	close(start)
	wg.Wait()

	for i := 0; i < workers; i++ {
		require.NoError(t, errs[i])
		require.Equal(t, newToken, tokens[i])
	}
	require.Equal(t, int32(1), imsCalls.Load(), "concurrent requests must share one IMS refresh")
	require.Equal(t, 1, repo.casCalls)
}

// IMS 往返期间管理员替换了 cookie：用旧 cookie 换来的 token 不得落库，新 cookie 保持不动。
func TestAdobeTokenProviderDiscardsTokenWhenCookieChangedDuringRefresh(t *testing.T) {
	newToken := adobeTestToken(t, map[string]any{"exp": time.Now().Add(24 * time.Hour).Unix()})
	stale := adobeTestToken(t, map[string]any{"exp": time.Now().Add(-time.Hour).Unix()})
	account := adobeGatewayTestAccount(24, map[string]any{"cookie": "old-cookie", "access_token": stale})
	repo := newAdobeGatewayCredsRepo(account)
	repo.beforeCAS = func(current *Account) {
		current.Credentials = map[string]any{"cookie": "new-cookie", "access_token": stale}
	}
	provider := newAdobeTokenProvider(repo, nil, newAdobeTestRefresher(t, adobeIMSTokenHandler(newToken, nil)))

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Empty(t, token)
	require.Equal(t, "new-cookie", repo.storedCredential("cookie"))
	require.Equal(t, stale, repo.storedCredential("access_token"))
}

func TestAdobeTokenProviderLockHeldKeepsUsableToken(t *testing.T) {
	usable := adobeTestToken(t, map[string]any{"exp": time.Now().Add(10 * time.Minute).Unix()})
	account := adobeGatewayTestAccount(25, map[string]any{"cookie": "aux_sid=abc", "access_token": usable})
	repo := newAdobeGatewayCredsRepo(account)
	api := NewOAuthRefreshAPI(repo, adobeLockHeldTokenCache{})
	provider := newAdobeTokenProvider(repo, api, newAdobeTestRefresher(t, adobeIMSMustNotBeCalled(t)))

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, usable, token)
}

func TestAdobeTokenProviderLockHeldReadsTokenRefreshedElsewhere(t *testing.T) {
	newToken := adobeTestToken(t, map[string]any{"exp": time.Now().Add(24 * time.Hour).Unix()})
	account := adobeGatewayTestAccount(26, map[string]any{"cookie": "aux_sid=abc"})
	repo := newAdobeGatewayCredsRepo(account)
	// 另一个实例已经写好新 token，但本请求的快照里还没有。
	repo.account.Credentials["access_token"] = newToken
	api := NewOAuthRefreshAPI(repo, adobeLockHeldTokenCache{})
	provider := newAdobeTokenProvider(repo, api, newAdobeTestRefresher(t, adobeIMSMustNotBeCalled(t)))

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, newToken, token)
}

func TestAdobeTokenProviderRereadFailureKeepsUsableToken(t *testing.T) {
	usable := adobeTestToken(t, map[string]any{"exp": time.Now().Add(10 * time.Minute).Unix()})
	account := adobeGatewayTestAccount(27, map[string]any{"cookie": "aux_sid=abc", "access_token": usable})
	repo := newAdobeGatewayCredsRepo(account)
	repo.getByIDErr = errors.New("db down")
	provider := newAdobeTokenProvider(repo, nil, newAdobeTestRefresher(t, adobeIMSMustNotBeCalled(t)))

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, usable, token)
}

// 锁内重读发现账号已被停用：尊重管理员操作，不回退到旧 token。
func TestAdobeTokenProviderDisabledAccountDoesNotFallback(t *testing.T) {
	usable := adobeTestToken(t, map[string]any{"exp": time.Now().Add(10 * time.Minute).Unix()})
	account := adobeGatewayTestAccount(28, map[string]any{"cookie": "aux_sid=abc", "access_token": usable})
	repo := newAdobeGatewayCredsRepo(account)
	repo.account.Status = StatusDisabled
	provider := newAdobeTokenProvider(repo, nil, newAdobeTestRefresher(t, adobeIMSMustNotBeCalled(t)))

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Empty(t, token)
	require.True(t, errors.Is(err, errOAuthRefreshAccountStateChanged))
}

func TestAdobeTokenProviderNilProviderReturnsStoredToken(t *testing.T) {
	var provider *AdobeTokenProvider
	token, err := provider.GetAccessToken(context.Background(), adobeGatewayTestAccount(29, map[string]any{
		"access_token": "stored",
	}))
	require.NoError(t, err)
	require.Equal(t, "stored", token)
}

// adobeNoCASRepo 只能重读，不实现条件写入接口。
type adobeNoCASRepo struct {
	AccountRepository
	account *Account
}

func (r *adobeNoCASRepo) GetByID(context.Context, int64) (*Account, error) {
	return snapshotOAuthRefreshAccount(r.account), nil
}

func TestOAuthRefreshAPIAdobeRequiresConditionalRepository(t *testing.T) {
	newToken := adobeTestToken(t, map[string]any{"exp": time.Now().Add(24 * time.Hour).Unix()})
	account := adobeGatewayTestAccount(30, map[string]any{"cookie": "aux_sid=abc"})
	api := NewOAuthRefreshAPI(&adobeNoCASRepo{account: account}, nil)
	refresher := newAdobeTestRefresher(t, adobeIMSTokenHandler(newToken, nil))

	result, err := api.RefreshIfNeeded(context.Background(), account, refresher, adobeRefreshWindow)
	require.Nil(t, result)
	var configErr *providerConfigurationRefreshError
	require.True(t, errors.As(err, &configErr))
}

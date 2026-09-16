//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

// shadowSkipTestRepo 是满足 AccountRepository 接口的最小 stub（只实现 GetByID）。
// 其他方法通过嵌入 nil 接口值满足编译，若被误调则 panic，便于发现意外调用路径。
type shadowSkipTestRepo struct {
	AccountRepository
	account *Account
}

func (r *shadowSkipTestRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	if r.account == nil || r.account.ID != id {
		return nil, ErrAccountNotFound
	}
	return r.account, nil
}

func newShadowTestGinCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/200/test", nil)
	return c
}

// --- 1. CanRefresh 守卫 ---

// TestOpenAITokenRefresherSkipsShadow 验证影子账号不被后台 token 刷新器处理。
func TestOpenAITokenRefresherSkipsShadow(t *testing.T) {
	pid := int64(100)
	r := NewOpenAITokenRefresher(nil, nil)
	// 影子账号：ParentAccountID 非 nil → CanRefresh 应返回 false
	require.False(t, r.CanRefresh(&Account{ID: 200, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &pid}))
	// 普通账号：有 refresh_token → CanRefresh 应返回 true
	require.True(t, r.CanRefresh(&Account{ID: 100, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"refresh_token": "RT"}}))
}

// --- 2. TestAccountConnection 影子凭据解析 ---

// TestAccountTestServiceSkipsShadow 验证影子账号连接测试不再早拒,而是尝试解析母账号凭据。
func TestAccountTestServiceSkipsShadow(t *testing.T) {
	pid := int64(100)
	shadow := &Account{
		ID:              200,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		ParentAccountID: &pid,
	}
	repo := &shadowSkipTestRepo{account: shadow}
	svc := &AccountTestService{accountRepo: repo}
	c := newShadowTestGinCtx()

	err := svc.TestAccountConnection(c, 200, "", "", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolve spark shadow parent")
}

// --- 3. EnsureOpenAIPrivacy 守卫 ---

// TestEnsureOpenAIPrivacySkipsShadow 验证影子账号跳过隐私设置（不调用 privacyClientFactory）。
// 影子账号透传母账号凭据，但 Extra 通常为空，需给它一个 access_token 才能让
// 现有的 token=="" 提前返回路径失效，从而真实验证 IsCredentialShadow 守卫。
func TestEnsureOpenAIPrivacySkipsShadow(t *testing.T) {
	pid := int64(100)
	shadow := &Account{
		ID:              200,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		ParentAccountID: &pid,
		// 提供 access_token：没有影子守卫时会进入 factory 调用
		Credentials: map[string]any{"access_token": "shadow-passthrough-token"},
	}
	privacyCalled := false
	svc := &adminServiceImpl{
		privacyClientFactory: func(proxyURL string) (*req.Client, error) {
			privacyCalled = true
			return nil, errors.New("should not reach factory for shadow account")
		},
	}
	got := svc.EnsureOpenAIPrivacy(context.Background(), shadow)
	require.Equal(t, "", got)
	require.False(t, privacyCalled, "privacyClientFactory 不应被影子账号触发")
}

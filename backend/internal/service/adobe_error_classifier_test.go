//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/stretchr/testify/require"
)

// 配额耗尽与 token 失效共用 401/403，但处置完全相反：
// 前者要把账号摘掉一段时间，后者要留着等刷新器修好。混淆会让好账号被白白冷却。
func TestClassifyAdobeErrorQuotaVsAuth(t *testing.T) {
	quota := classifyAdobeError(adobe.NewQuotaExhaustedError("exhausted", http.StatusForbidden))
	require.Equal(t, NextAccountRetry, quota.Failover.NextAccountAction)
	require.Equal(t, adobeFailureQuotaExhausted, quota.Failover.Reason)
	require.Equal(t, GatewayFailureScopeAccount, quota.Failover.Scope)
	require.Positive(t, quota.Cooldown)

	auth := classifyAdobeError(adobe.NewAuthError("expired", http.StatusUnauthorized))
	require.Equal(t, NextAccountRetry, auth.Failover.NextAccountAction)
	require.Equal(t, adobeFailureAuth, auth.Failover.Reason)
	require.Equal(t, GatewayFailureStageAccountAuth, auth.Failover.Stage)
	require.Zero(t, auth.Cooldown, "token 过期由刷新器修复，冷却只会拖慢恢复")
	require.True(t, auth.InvalidateToken, "401 是上游对 token 的明确拒绝")

	forbidden := classifyAdobeError(adobe.NewAuthError("forbidden", http.StatusForbidden))
	require.Equal(t, adobeFailureAuth, forbidden.Failover.Reason)
	require.False(t, forbidden.InvalidateToken, "403 可能是 WAF，不能清掉 token")
}

func TestClassifyAdobeErrorNotEntitled(t *testing.T) {
	failure := classifyAdobeError(adobe.NewNotEntitledError(
		`submit failed: 403 {"error_code":"model_not_entitled"}`, http.StatusForbidden, ""))
	require.Equal(t, NextAccountRetry, failure.Failover.NextAccountAction)
	require.Equal(t, adobeFailureNotEntitled, failure.Failover.Reason)
	require.NotEqual(t, adobeFailureAuth, failure.Failover.Reason)
	require.Equal(t, GatewayFailureStageInference, failure.Failover.Stage)
	require.Equal(t, GatewayFailureScopeAccount, failure.Failover.Scope)
	require.Equal(t, http.StatusForbidden, failure.Failover.ClientStatusCode)
	require.Contains(t, failure.Failover.ClientMessage, "not entitled")
	require.Zero(t, failure.Cooldown, "权益不足不是账号故障，不能冷却")
}

func TestClassifyAdobeErrorUpstreamTemporary(t *testing.T) {
	failure := classifyAdobeError(
		adobe.NewUpstreamTemporaryError("boom", http.StatusServiceUnavailable, adobe.ErrorTypeStatus))
	require.Equal(t, NextAccountRetry, failure.Failover.NextAccountAction)
	require.Equal(t, http.StatusServiceUnavailable, failure.Failover.StatusCode)
	require.Equal(t, GatewayFailureScopeProvider, failure.Failover.Scope)
	require.Zero(t, failure.Cooldown)
}

func TestClassifyAdobeErrorContentRejected(t *testing.T) {
	failure := classifyAdobeError(adobe.NewContentRejectedError(
		`image poll failed: 451 {"error_code":"image_unsafe"}`, http.StatusUnavailableForLegalReasons, ""))
	require.Equal(t, NextAccountRetry, failure.Failover.NextAccountAction)
	require.Equal(t, adobeFailureContentRejected, failure.Failover.Reason)
	require.Equal(t, http.StatusBadRequest, failure.Failover.ClientStatusCode)
	require.Equal(t, GatewayFailureScopeRequest, failure.Failover.Scope)
	require.Zero(t, failure.Cooldown, "内容拒绝不是账号故障，不能冷却 Cookie 号")
	require.True(t, IsAdobeContentRejected(failure.Failover))
}

// 请求本身的问题换号也救不了，必须停止 failover 并把原因带给客户端。
func TestClassifyAdobeErrorTerminalRequest(t *testing.T) {
	failure := classifyAdobeError(adobe.NewRequestError("unsupported aspect ratio"))
	require.Equal(t, NextAccountStop, failure.Failover.NextAccountAction)
	require.Equal(t, http.StatusBadRequest, failure.Failover.ClientStatusCode)
	require.Equal(t, "unsupported aspect ratio", failure.Failover.ClientMessage)
	require.Zero(t, failure.Cooldown)
}

func TestClassifyAdobeErrorPassesThroughForeignErrors(t *testing.T) {
	require.Nil(t, classifyAdobeError(nil).Failover)
	require.Nil(t, classifyAdobeError(errors.New("boom")).Failover)
	require.Nil(t, classifyAdobeError(context.Canceled).Failover)
}

// service 层常用 %w 加上下文再上抛，包装后仍须能正确分类。
func TestClassifyAdobeErrorThroughWrapping(t *testing.T) {
	wrapped := fmt.Errorf("generate: %w", adobe.NewQuotaExhaustedError("q", 403))
	require.Equal(t, adobeFailureQuotaExhausted, classifyAdobeError(wrapped).Failover.Reason)
}

// 上游没给状态码（网络层错误）时也要有个合理的对外状态码。
func TestClassifyAdobeErrorFillsMissingStatus(t *testing.T) {
	require.Equal(t, http.StatusBadGateway,
		classifyAdobeError(adobe.NewUpstreamTemporaryError("net down", 0, adobe.ErrorTypeConnection)).
			Failover.StatusCode)
	require.Equal(t, http.StatusTooManyRequests,
		classifyAdobeError(adobe.NewQuotaExhaustedError("q", 0)).Failover.StatusCode)
}

type adobeCooldownRecorder struct {
	AccountRepository
	accountID int64
	until     time.Time
	reason    string
	err       error
	calls     int
}

func (r *adobeCooldownRecorder) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	r.calls++
	r.accountID, r.until, r.reason = id, until, reason
	return r.err
}

func TestApplyAdobeCooldown(t *testing.T) {
	repo := &adobeCooldownRecorder{}
	applyAdobeCooldown(context.Background(), repo,
		9, classifyAdobeError(adobe.NewQuotaExhaustedError("q", 403)))
	require.Equal(t, 1, repo.calls)
	require.Equal(t, int64(9), repo.accountID)
	require.Contains(t, repo.reason, "credits")
	require.True(t, repo.until.After(time.Now()))

	// 无冷却诉求的错误不应触碰账号状态。
	quiet := &adobeCooldownRecorder{}
	applyAdobeCooldown(context.Background(), quiet, 9, classifyAdobeError(adobe.NewAuthError("a", 401)))
	require.Zero(t, quiet.calls)

	entitled := &adobeCooldownRecorder{}
	applyAdobeCooldown(context.Background(), entitled, 9,
		classifyAdobeError(adobe.NewNotEntitledError("e", 403, "")))
	require.Zero(t, entitled.calls)
}

// 冷却是优化项：写不进去最多下次再撞一次，不该让本次请求失败。
func TestApplyAdobeCooldownSwallowsRepoError(t *testing.T) {
	repo := &adobeCooldownRecorder{err: errors.New("db down")}
	require.NotPanics(t, func() {
		applyAdobeCooldown(context.Background(), repo,
			9, classifyAdobeError(adobe.NewQuotaExhaustedError("q", 403)))
	})
	require.Equal(t, 1, repo.calls)
}

type adobeTokenInvalidationRecorder struct {
	AccountRepository
	calls     int
	accountID int64
	token     string
	err       error
}

func (r *adobeTokenInvalidationRecorder) InvalidateAdobeAccessTokenIfUnchanged(_ context.Context, id int64, token string) (bool, error) {
	r.calls++
	r.accountID, r.token = id, token
	return r.err == nil, r.err
}

// 上游 401 是对这个 token 的明确拒绝：清掉它，下次取 token 走 cookie 刷新而不是按 exp 复用。
// 403 可能是 WAF/风控，只让当次请求换号，不动账号凭据。
func TestAdobeFailoverInvalidatesRejectedToken(t *testing.T) {
	t.Run("401 清掉本次 token", func(t *testing.T) {
		repo := &adobeTokenInvalidationRecorder{}
		svc := &GatewayService{accountRepo: repo}
		failover := svc.AdobeFailover(context.Background(), 9, "tok-rejected",
			adobe.NewAuthError("Token invalid or expired", http.StatusUnauthorized))
		require.NotNil(t, failover)
		require.Equal(t, adobeFailureAuth, failover.Reason)
		require.Equal(t, 1, repo.calls)
		require.Equal(t, int64(9), repo.accountID)
		require.Equal(t, "tok-rejected", repo.token)
	})

	t.Run("403 不动凭据", func(t *testing.T) {
		repo := &adobeTokenInvalidationRecorder{}
		svc := &GatewayService{accountRepo: repo}
		require.NotNil(t, svc.AdobeFailover(context.Background(), 9, "tok",
			adobe.NewAuthError("Token invalid or expired", http.StatusForbidden)))
		require.Zero(t, repo.calls)
	})

	t.Run("非鉴权错误不动凭据", func(t *testing.T) {
		repo := &adobeTokenInvalidationRecorder{}
		svc := &GatewayService{accountRepo: repo}
		require.NotNil(t, svc.AdobeFailover(context.Background(), 9, "tok",
			adobe.NewUpstreamTemporaryError("boom", http.StatusBadGateway, adobe.ErrorTypeStatus)))
		require.Zero(t, repo.calls)
	})

	t.Run("清除失败不影响本次换号", func(t *testing.T) {
		repo := &adobeTokenInvalidationRecorder{err: errors.New("db down")}
		svc := &GatewayService{accountRepo: repo}
		failover := svc.AdobeFailover(context.Background(), 9, "tok",
			adobe.NewAuthError("Token invalid or expired", http.StatusUnauthorized))
		require.NotNil(t, failover)
		require.Equal(t, NextAccountRetry, failover.NextAccountAction)
		require.Equal(t, 1, repo.calls)
	})
}

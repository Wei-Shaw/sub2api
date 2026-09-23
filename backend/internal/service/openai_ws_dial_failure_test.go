//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newOpenAIWSDialTransportError(cause error) *openAIWSDialError {
	return &openAIWSDialError{Err: &openAIWSHandshakeError{Err: fmt.Errorf("failed to WebSocket dial: %w", cause)}}
}

func newOpenAIWSDialFailureTestService() (*OpenAIGatewayService, *openaiTransportAccountRepoStub) {
	repo := &openaiTransportAccountRepoStub{}
	return &OpenAIGatewayService{accountRepo: repo}, repo
}

func requireOpenAIWSDialFailover(t *testing.T, failoverErr *UpstreamFailoverError) {
	t.Helper()
	require.NotNil(t, failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.True(t, failoverErr.ShouldRetryNextAccount())
	require.False(t, failoverErr.RetryableOnSameAccount)
	require.JSONEq(t, string(openAITransportFailoverBody), string(failoverErr.ResponseBody))
}

func TestOpenAITransportLegacyPolicy_WSDial(t *testing.T) {
	runOpenAITransportLegacyPolicyTests(t, func(
		svc *OpenAIGatewayService, ctx context.Context, c *gin.Context,
		account *Account, cause error, passthrough bool,
	) error {
		failover := svc.handleOpenAIWSDialTransportFailure(
			ctx, c, account, 2, newOpenAIWSDialTransportError(cause), passthrough,
		)
		if failover == nil {
			return errors.New("expected WS transport failover")
		}
		return failover
	})
}

func TestHandleOpenAIWSDialTransportFailure_WrappedCanceledNotHandled(t *testing.T) {
	svc, repo := newOpenAIWSDialFailureTestService()
	account := &Account{ID: 717, Platform: PlatformOpenAI}
	c, rec := newOpenAITransportErrTestContext()
	dialErr := newOpenAIWSDialTransportError(fmt.Errorf("dial canceled: %w", context.Canceled))
	require.Nil(t, svc.handleOpenAIWSDialTransportFailure(context.Background(), c, account, 1, dialErr, false))
	require.Empty(t, repo.tempUnschedCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Zero(t, rec.Body.Len())
}

func TestHandleOpenAIWSDialTransportFailure_ParentDeadlineNotHandled(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	svc, repo := newOpenAIWSDialFailureTestService()
	account := &Account{ID: 718, Platform: PlatformOpenAI}
	c, _ := newOpenAITransportErrTestContext()
	dialErr := newOpenAIWSDialTransportError(context.DeadlineExceeded)
	require.Nil(t, svc.handleOpenAIWSDialTransportFailure(ctx, c, account, 1, dialErr, false))
	require.Empty(t, repo.tempUnschedCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

// 代理凭证被拒不会自愈：换号的同时一次即封 10 分钟。
func TestHandleOpenAIWSDialTransportFailure_CredentialBlocksImmediately(t *testing.T) {
	c, _ := newOpenAITransportErrTestContext()
	svc, repo := newOpenAIWSDialFailureTestService()
	account := &Account{ID: 157, Name: "proxy-expired", Platform: PlatformOpenAI}
	dialErr := newOpenAIWSDialTransportError(errors.New("socks connect tcp 1.2.3.4:1080->chatgpt.com:443: username/password authentication failed"))

	before := time.Now()
	requireOpenAIWSDialFailover(t, svc.handleOpenAIWSDialTransportFailure(context.Background(), c, account, 1, dialErr, false))
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Len(t, repo.tempUnschedCalls, 1)
	require.True(t, repo.tempUnschedCalls[0].until.After(before.Add(openAITransportErrorTempUnschedDuration-time.Second)))
}

// 非持久类传输错误（如握手中途 EOF）只换号，不增加冷却。
func TestHandleOpenAIWSDialTransportFailure_TransientErrorNeverBlocks(t *testing.T) {
	c, _ := newOpenAITransportErrTestContext()
	svc, repo := newOpenAIWSDialFailureTestService()
	account := &Account{ID: 153, Name: "flaky", Platform: PlatformOpenAI}
	dialErr := newOpenAIWSDialTransportError(errors.New("EOF"))

	for i := 1; i <= 4; i++ {
		requireOpenAIWSDialFailover(t, svc.handleOpenAIWSDialTransportFailure(context.Background(), c, account, 1, dialErr, false))
	}
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Empty(t, repo.tempUnschedCalls)
}

// 客户端已取消：不换号也不封禁，上游没有机会证明自己有故障。
func TestHandleOpenAIWSDialTransportFailure_ClientCanceledNotHandled(t *testing.T) {
	c, _ := newOpenAITransportErrTestContext()
	svc, repo := newOpenAIWSDialFailureTestService()
	account := &Account{ID: 155, Name: "client-gone", Platform: PlatformOpenAI}
	dialErr := newOpenAIWSDialTransportError(context.DeadlineExceeded)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for i := 1; i <= 4; i++ {
		require.Nil(t, svc.handleOpenAIWSDialTransportFailure(ctx, c, account, 1, dialErr, false))
	}
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Empty(t, repo.tempUnschedCalls)
}

func TestHandleOpenAIWSDialTransportFailure_StatusCodeNotHandled(t *testing.T) {
	for _, status := range []int{401, 403, 407, 429, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			svc, repo := newOpenAIWSDialFailureTestService()
			account := &Account{ID: 719, Platform: PlatformOpenAI}
			c, _ := newOpenAITransportErrTestContext()
			dialErr := &openAIWSDialError{StatusCode: status, Err: errors.New("connection refused")}
			require.Nil(t, svc.handleOpenAIWSDialTransportFailure(context.Background(), c, account, 1, dialErr, false))
			require.Empty(t, repo.tempUnschedCalls)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
			_, exists := c.Get(OpsUpstreamErrorsKey)
			require.False(t, exists)
		})
	}
}

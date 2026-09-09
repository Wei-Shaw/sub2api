package handler

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestContinuationMaxWaitingSharedHTTPAndWS(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModeCtxPool, service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeHTTPBridge} {
		for _, limit := range []int{7, 100} {
			t.Run(fmt.Sprintf("%s/limit=%d", mode, limit), func(t *testing.T) {
				f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: mode, timeout: 3 * time.Second, burstLimit: 2, waitingLimit: limit, skipDial: true})
				ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
				defer cancel()
				account := &service.Account{ID: 801, Platform: service.PlatformOpenAI, Concurrency: 1}
				plan := f.handler.gatewayService.OpenAIWSAccountWaitPlan(account)
				ok, err := f.cache.AcquireAccountSlot(ctx, account.ID, 1, "busy")
				require.NoError(t, err)
				require.True(t, ok)
				for range limit - 1 {
					ok, err = f.cache.IncrementAccountContinuationWaitCount(ctx, account.ID, limit)
					require.NoError(t, err)
					require.True(t, ok)
				}
				waitForCount := func(want int) {
					t.Helper()
					require.Eventually(t, func() bool {
						n, err := f.cache.GetAccountContinuationWaitingCount(ctx, account.ID)
						return err == nil && n == want
					}, time.Second, time.Millisecond)
				}
				httpCtx, cancelHTTP := context.WithCancel(ctx)
				defer cancelHTTP()
				c := newYieldTestGinContext(t)
				c.Request = c.Request.WithContext(httpCtx)
				httpDone := make(chan openAISlotAcquireResult, 1)
				go func() {
					started := false
					release, result := f.handler.acquireOpenAIAccountSlot(c, nil, "", &service.AccountSelectionResult{Account: account, WaitPlan: plan}, false, &started, zap.NewNop(), func(int, string, string, string) {})
					if release != nil {
						release()
					}
					httpDone <- result
				}()
				waitForCount(limit)
				release, err := f.handler.acquireWSAccountSlot(ctx, account, 1, plan, &openAIWSAccountWaitBudget{turn: 2, continuation: true}, mode, openAIWSAccountWaitPhaseSubsequent, time.Time{}, zap.NewNop())
				require.Nil(t, release)
				var closeErr *service.OpenAIWSClientCloseError
				require.ErrorAs(t, err, &closeErr)
				require.Equal(t, coderws.StatusTryAgainLater, closeErr.StatusCode())
				waitForCount(limit)
				cancelHTTP()
				select {
				case result := <-httpDone:
					require.Equal(t, openAISlotAcquireFailed, result)
				case <-ctx.Done():
					t.Fatal("HTTP cancellation did not finish")
				}
				waitForCount(limit - 1)
				wsDone := make(chan error, 1)
				go func() {
					release, err := f.handler.acquireWSAccountSlot(ctx, account, 1, plan, &openAIWSAccountWaitBudget{turn: 2, continuation: true}, mode, openAIWSAccountWaitPhaseSubsequent, time.Time{}, zap.NewNop())
					if release != nil {
						release()
					}
					wsDone <- err
				}()
				waitForCount(limit)
				started, status := false, 0
				release, result := f.handler.acquireOpenAIAccountSlot(newYieldTestGinContext(t), nil, "", &service.AccountSelectionResult{Account: account, WaitPlan: plan}, false, &started, zap.NewNop(), func(code int, _, _, _ string) { status = code })
				require.Nil(t, release)
				require.Equal(t, openAISlotAcquireFailed, result)
				require.Equal(t, http.StatusTooManyRequests, status)
				waitForCount(limit)
				require.NoError(t, f.cache.ReleaseAccountSlot(ctx, account.ID, "busy"))
				select {
				case err := <-wsDone:
					require.NoError(t, err)
				case <-ctx.Done():
					t.Fatal("WS did not acquire the released slot")
				}
				waitForCount(limit - 1)
				for range limit - 1 {
					require.NoError(t, f.cache.DecrementAccountContinuationWaitCount(ctx, account.ID))
				}
				waitForCount(0)
				require.Zero(t, accountConcurrency(t, ctx, f.cache))
			})
		}
	}
}

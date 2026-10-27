package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestContinuationBurstWSTurnsYieldAndResume(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModeCtxPool, service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeHTTPBridge} {
		for _, hold := range []int{0, 5} {
			t.Run(fmt.Sprintf("%s/hold=%d", mode, hold), func(t *testing.T) {
				f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: mode, timeout: 3 * time.Second, holdSeconds: hold, burstLimit: 2, waitingLimit: 100})
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				completeOpenAIWSTurnKeepingSlot(t, ctx, f, "first")
				ok, err := f.cache.IncrementAccountWaitCount(ctx, 801, 10)
				require.NoError(t, err)
				require.True(t, ok)
				admission, supported := f.cache.(service.AccountAdmissionCache)
				require.True(t, supported)
				for i := 1; i <= 2; i++ {
					completeOpenAIWSTurnKeepingSlot(t, ctx, f, fmt.Sprint(i))
					state, err := admission.GetAccountAdmissionState(ctx, 801)
					require.NoError(t, err)
					require.Equal(t, i, state.ContinuationBurst)
				}
				require.Eventually(t, func() bool { return accountConcurrency(t, ctx, f.cache) == 0 }, time.Second, time.Millisecond, "达到两次后轮末让出")
				for range 3 {
					ok, err := f.cache.IncrementAccountContinuationWaitCount(ctx, 801, 100)
					require.NoError(t, err)
					require.True(t, ok)
				}
				require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"after-yield"}`)))
				require.Eventually(t, func() bool {
					n, err := f.cache.GetAccountContinuationWaitingCount(ctx, 801)
					return err == nil && n == 4
				}, time.Second, time.Millisecond, "让槽后的续聊可超过粘性阈值入队，仍须等待新会话获槽")
				select {
				case <-f.requests:
					t.Fatal("续聊在新会话之前到达上游")
				default:
				}
				helper := NewConcurrencyHelper(service.NewConcurrencyService(f.cache), "", 0)
				streamStarted := false
				release, err := helper.AcquireAccountSlotWithWaitTimeoutForClass(newYieldTestGinContext(t), 801, 1, time.Second, false, &streamStarted, service.AccountWaitClassNewSession, 2)
				require.NoError(t, err, "HTTP 新会话能取得让出的槽")
				require.NoError(t, f.cache.DecrementAccountWaitCount(ctx, 801))
				release()
				_, body, err := f.conn.Read(ctx)
				require.NoError(t, err, "同一 WS 连接继续工作")
				require.Contains(t, string(body), "response.completed")
				<-f.requests
				for range 3 {
					require.NoError(t, f.cache.DecrementAccountContinuationWaitCount(ctx, 801))
				}
				require.Eventually(t, func() bool {
					n, err := f.cache.GetAccountContinuationWaitingCount(ctx, 801)
					return err == nil && n == 0
				}, time.Second, time.Millisecond)
			})
		}
	}
}

func TestContinuationBurstHeldSlotCannotBypassOtherAdmissions(t *testing.T) {
	f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeCtxPool, timeout: 3 * time.Second, holdSeconds: 5, burstLimit: 2})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "first")
	ok, err := f.cache.IncrementAccountWaitCount(ctx, 801, 10)
	require.NoError(t, err)
	require.True(t, ok)
	other := service.NewConcurrencyService(f.cache)
	for range 2 {
		result, err := other.AcquireAccountSlotForClass(ctx, 801, 2, service.AccountWaitClassContinuation, 2)
		require.NoError(t, err)
		require.True(t, result.Acquired)
		result.ReleaseFunc()
	}
	require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"second"}`)))
	require.Eventually(t, func() bool {
		n, _ := f.cache.GetAccountContinuationWaitingCount(ctx, 801)
		return n == 1 && accountConcurrency(t, ctx, f.cache) == 0
	}, time.Second, time.Millisecond, "其他连接达到门槛后，已挂起的槽也必须让出")
	result, err := other.AcquireAccountSlotForClass(ctx, 801, 1, service.AccountWaitClassNewSession, 2)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.NoError(t, f.cache.DecrementAccountWaitCount(ctx, 801))
	result.ReleaseFunc()
	_, body, err := f.conn.Read(ctx)
	require.NoError(t, err)
	require.Contains(t, string(body), "response.completed")
	<-f.requests
}

func TestContinuationBurstHTTPWaitingKeepsContinuationClass(t *testing.T) {
	cache := testutil.NewTestConcurrencyCache(t)
	ctx := context.Background()
	first := NewConcurrencyHelper(service.NewConcurrencyService(cache), "", 0)
	second := NewConcurrencyHelper(service.NewConcurrencyService(cache), "", 0)
	ok, err := cache.IncrementAccountWaitCount(ctx, 41, 10)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = cache.AcquireAccountSlot(ctx, 41, 1, "busy")
	require.NoError(t, err)
	require.True(t, ok)
	for i := range 2 {
		if i == 0 {
			go func() {
				time.Sleep(50 * time.Millisecond)
				_ = cache.ReleaseAccountSlot(ctx, 41, "busy")
			}()
		}
		streamStarted := false
		release, err := first.AcquireAccountSlotWithWaitTimeoutForClass(newYieldTestGinContext(t), 41, 1, time.Second, false, &streamStarted, service.AccountWaitClassContinuation, 2)
		require.NoError(t, err)
		release()
	}
	result, err := second.TryAcquireAccountSlotForPlan(ctx, 41, 1, service.AccountWaitClassContinuation, 2)
	require.NoError(t, err)
	require.False(t, result.Acquired, "HTTP 轮询两次成功均记作续聊，共享给另一实例")
	result, err = second.TryAcquireAccountSlotForPlan(ctx, 41, 1, service.AccountWaitClassNewSession, 2)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	result.ReleaseFunc()
}

func TestContinuationBurstDisabledWSKeepsHolding(t *testing.T) {
	f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeCtxPool, timeout: time.Second, holdSeconds: 5, burstLimit: 0})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "first")
	ok, err := f.cache.IncrementAccountWaitCount(ctx, 801, 10)
	require.NoError(t, err)
	require.True(t, ok)
	for i := range 4 {
		completeOpenAIWSTurnKeepingSlot(t, ctx, f, fmt.Sprint(i))
		require.Equal(t, 1, accountConcurrency(t, ctx, f.cache))
	}
}

func TestContinuationBurstHTTPEndpoints(t *testing.T) {
	for _, endpoint := range []struct{ path, body string }{
		{"/v1/responses", `{"model":"gpt-5.1","input":"hello","stream":true}`},
		{"/v1/chat/completions", `{"model":"gpt-5.1","messages":[{"role":"user","content":"hello"}],"stream":true}`},
		{"/v1/messages", `{"model":"gpt-5.1","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":true}`},
	} {
		t.Run(endpoint.path, func(t *testing.T) {
			f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeCtxPool, timeout: 200 * time.Millisecond, burstLimit: 2, bindings: &openAIWSStickyBindings{}, skipDial: true})
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			post := func(session string, reachesUpstream bool) {
				t.Helper()
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.server.URL+endpoint.path, strings.NewReader(endpoint.body))
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("session_id", session)
				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				_, err = io.Copy(io.Discard, resp.Body)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				if reachesUpstream {
					select {
					case <-f.requests:
					case <-ctx.Done():
						t.Fatal("准入请求未到达上游")
					}
				} else {
					select {
					case <-f.requests:
						t.Fatal("续聊绕过限额到达上游")
					default:
					}
				}
				require.Eventually(t, func() bool { return accountConcurrency(t, ctx, f.cache) == 0 }, time.Second, time.Millisecond)
			}
			post("bound", true)
			ok, err := f.cache.IncrementAccountWaitCount(ctx, 801, 10)
			require.NoError(t, err)
			require.True(t, ok)
			post("bound", true)
			post("bound", true)
			post("bound", false)
			post("new-session", true)
			post("bound", true)
		})
	}
}

func TestContinuationBurstWSWaitsAfterYieldOnlyForOpenAI(t *testing.T) {
	for _, platform := range []string{service.PlatformGrok, service.PlatformKimi, service.PlatformZhipu, service.PlatformDeepseek, service.PlatformMiniMax, service.PlatformOpenCodeGo} {
		t.Run(platform, func(t *testing.T) {
			f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeHTTPBridge, timeout: 3 * time.Second, burstLimit: 2, skipDial: true})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ok, err := f.cache.AcquireAccountSlot(ctx, 801, 1, "busy")
			require.NoError(t, err)
			require.True(t, ok)
			started := time.Now()
			release, err := f.handler.acquireWSAccountSlot(ctx, &service.Account{ID: 801, Platform: platform}, 1,
				&service.AccountWaitPlan{Timeout: 3 * time.Second, MaxWaiting: 3},
				&openAIWSAccountWaitBudget{turn: 2, continuation: true}, service.OpenAIWSIngressModeHTTPBridge, openAIWSAccountWaitPhaseSubsequent, time.Time{}, zap.NewNop())
			require.Nil(t, release)
			require.Equal(t, coderws.StatusTryAgainLater, closeStatusOf(t, err), "非 openai 平台满槽照上游立即断开")
			require.Less(t, time.Since(started), time.Second)
			n, err := f.cache.GetAccountWaitingCount(ctx, 801)
			require.NoError(t, err)
			require.Zero(t, n, "不进入账号等待队列")
		})
	}
	for _, platform := range []string{service.PlatformOpenAI} {
		t.Run(platform, func(t *testing.T) {
			f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeHTTPBridge, timeout: 3 * time.Second, burstLimit: 2, skipDial: true})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			concurrency := service.NewConcurrencyService(f.cache)
			ok, err := f.cache.IncrementAccountWaitCount(ctx, 801, 10)
			require.NoError(t, err)
			require.True(t, ok)
			for range 2 {
				result, err := concurrency.AcquireAccountSlotForClass(ctx, 801, 1, service.AccountWaitClassContinuation, 2)
				require.NoError(t, err)
				require.True(t, result.Acquired)
				result.ReleaseFunc()
			}
			resultCh := make(chan error, 1)
			go func() {
				release, err := f.handler.acquireWSAccountSlot(ctx, &service.Account{ID: 801, Platform: platform}, 1,
					&service.AccountWaitPlan{Class: service.AccountWaitClassContinuation, Timeout: 3 * time.Second, MaxWaiting: 3},
					&openAIWSAccountWaitBudget{turn: 2, continuation: true}, service.OpenAIWSIngressModeHTTPBridge, openAIWSAccountWaitPhaseSubsequent, time.Time{}, zap.NewNop())
				if release != nil {
					release()
				}
				resultCh <- err
			}()
			require.Eventually(t, func() bool {
				n, _ := f.cache.GetAccountContinuationWaitingCount(ctx, 801)
				return n == 1
			}, time.Second, time.Millisecond, "兼容平台续聊应等待而不是直接断开")
			result, err := concurrency.AcquireAccountSlotForClass(ctx, 801, 1, service.AccountWaitClassNewSession, 2)
			require.NoError(t, err)
			require.True(t, result.Acquired)
			require.NoError(t, f.cache.DecrementAccountWaitCount(ctx, 801))
			result.ReleaseFunc()
			select {
			case err := <-resultCh:
				require.NoError(t, err)
			case <-ctx.Done():
				t.Fatal("续聊未恢复")
			}
		})
	}
}

func TestContinuationBurstCanceledWSWaitReleasesSlots(t *testing.T) {
	f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeCtxPool, timeout: 3 * time.Second, holdSeconds: 5, burstLimit: 2})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "first")
	ok, err := f.cache.IncrementAccountWaitCount(ctx, 801, 10)
	require.NoError(t, err)
	require.True(t, ok)
	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "second")
	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "third")
	require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"waiting"}`)))
	require.Eventually(t, func() bool {
		n, _ := f.cache.GetAccountContinuationWaitingCount(ctx, 801)
		return n == 1
	}, time.Second, time.Millisecond)
	require.NoError(t, f.conn.CloseNow())
	scenarioWaitFinished(t, ctx, f)
	n, err := f.cache.GetAccountContinuationWaitingCount(ctx, 801)
	require.NoError(t, err)
	require.Zero(t, n)
	require.Zero(t, accountConcurrency(t, ctx, f.cache))
	n, err = f.cache.GetUserConcurrency(ctx, 1702)
	require.NoError(t, err)
	require.Zero(t, n)
	require.NoError(t, f.cache.DecrementAccountWaitCount(ctx, 801))
	admission, supported := f.cache.(service.AccountAdmissionCache)
	require.True(t, supported)
	state, err := admission.GetAccountAdmissionState(ctx, 801)
	require.NoError(t, err)
	require.Zero(t, state.ContinuationBurst)
}

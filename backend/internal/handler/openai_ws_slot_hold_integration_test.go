package handler

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func accountConcurrency(t *testing.T, ctx context.Context, cache service.ConcurrencyCache) int {
	t.Helper()
	n, err := cache.GetAccountConcurrency(ctx, 801)
	require.NoError(t, err)
	return n
}

func TestOpenAIWSSlotHold_ReusesSlotWithinHoldAndReleasesOnClose(t *testing.T) {
	f := newOpenAIWSAccountWaitSessionWithHold(t, service.OpenAIWSIngressModeCtxPool, 2*time.Second, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "first")

	time.Sleep(300 * time.Millisecond)
	require.Equal(t, 1, accountConcurrency(t, ctx, f.cache), "轮结束后账号槽在保留期内不释放")

	other, err := f.cache.AcquireAccountSlot(ctx, 801, 1, "other")
	require.NoError(t, err)
	require.False(t, other, "保留期内别的会话拿不到这个槽")

	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "second")
	require.Equal(t, 1, accountConcurrency(t, ctx, f.cache), "取回复用，不重抢")

	require.NoError(t, f.conn.Close(coderws.StatusNormalClosure, ""))
	require.Eventually(t, func() bool { return accountConcurrency(t, ctx, f.cache) == 0 }, time.Second, 10*time.Millisecond, "连接关闭立即释放挂起槽，不等保留期")
}

func TestOpenAIWSSlotHold_ExpiresAfterHold(t *testing.T) {
	f := newOpenAIWSAccountWaitSessionWithHold(t, service.OpenAIWSIngressModeCtxPool, 2*time.Second, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "first")
	require.Equal(t, 1, accountConcurrency(t, ctx, f.cache))
	require.Eventually(t, func() bool { return accountConcurrency(t, ctx, f.cache) == 0 }, 2*time.Second, 20*time.Millisecond, "保留期到期自动释放")

	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "second")
	require.Equal(t, 1, accountConcurrency(t, ctx, f.cache), "到期后下一轮走正常抢槽")
}

func TestOpenAIWSSlotHold_YieldsWhenContinuationWaiting(t *testing.T) {
	f := newOpenAIWSAccountWaitSessionWithHold(t, service.OpenAIWSIngressModeCtxPool, 2*time.Second, 5)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "first")
	require.Equal(t, 1, accountConcurrency(t, ctx, f.cache))

	ok, err := f.cache.IncrementAccountContinuationWaitCount(ctx, 801, 3)
	require.NoError(t, err)
	require.True(t, ok)
	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "second")
	require.Eventually(t, func() bool { return accountConcurrency(t, ctx, f.cache) == 0 }, 500*time.Millisecond, 10*time.Millisecond, "本账号有续聊等待者时轮末不挂起、直接让出")
}

func TestOpenAIWSSlotHold_UserWaitReleasesHeldSlotFirst(t *testing.T) {
	f := newOpenAIWSAccountWaitSessionWithHold(t, service.OpenAIWSIngressModeCtxPool, 2*time.Second, 5)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "first")
	require.Equal(t, 1, accountConcurrency(t, ctx, f.cache))
	require.Eventually(t, func() bool { n, _ := f.cache.GetUserConcurrency(ctx, 1702); return n == 0 }, time.Second, time.Millisecond)

	ok, err := f.cache.AcquireUserSlot(ctx, 1702, 1, "other-user-slot")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"second"}`)))
	require.Eventually(t, func() bool { return accountConcurrency(t, ctx, f.cache) == 0 }, 500*time.Millisecond, 10*time.Millisecond, "用户槽拿不到时先释放挂起的账号槽再等，不持账号槽等用户槽")

	require.NoError(t, f.cache.ReleaseUserSlot(ctx, 1702, "other-user-slot"))
	_, body, err := f.conn.Read(ctx)
	require.NoError(t, err)
	require.Contains(t, string(body), "response.completed")
	<-f.requests
	require.Equal(t, 1, accountConcurrency(t, ctx, f.cache), "拿到用户槽后按续聊类重新申请账号槽")
}

func TestOpenAIWSSlotHold_SameAccountRetryDoesNotHoldTwoSlots(t *testing.T) {
	f := newOpenAIWSSessionWithOptions(t, openAIWSSessionOptions{mode: service.OpenAIWSIngressModeHTTPBridge, timeout: 2 * time.Second, holdSeconds: 5, oauth: true})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "first")
	require.Equal(t, 1, accountConcurrency(t, ctx, f.cache), "第 1 轮结束后账号槽挂起")

	f.rateLimitPOSTs.Store(1)
	start := time.Now()
	require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"second"}`)))
	_, body, err := f.conn.Read(ctx)
	require.NoError(t, err)
	require.Contains(t, string(body), "response.completed", "429 后在同一账号重试成功")
	require.Less(t, time.Since(start), 3*time.Second, "并发 1 的账号上重试不等挂起槽保留期到期")
	<-f.requests
	<-f.requests
	require.Equal(t, 1, accountConcurrency(t, ctx, f.cache), "重试成功后只挂起一个槽，不双占")
}

func TestOpenAIWSSlotHold_DisabledKeepsPerTurnRelease(t *testing.T) {
	f := newOpenAIWSAccountWaitSessionWithHold(t, service.OpenAIWSIngressModeCtxPool, 2*time.Second, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	completeOpenAIWSTurnKeepingSlot(t, ctx, f, "first")
	require.Eventually(t, func() bool { return accountConcurrency(t, ctx, f.cache) == 0 }, time.Second, time.Millisecond, "保留期为 0 时与现状一致")
}

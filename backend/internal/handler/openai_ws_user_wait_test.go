package handler

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func userWaitingCount(t *testing.T, ctx context.Context, cache service.ConcurrencyCache, userID int64) int {
	t.Helper()
	loads, err := cache.GetUsersLoadBatch(ctx, []service.UserWithConcurrency{{ID: userID, MaxConcurrency: 1}})
	require.NoError(t, err)
	if loads[userID] == nil {
		return 0
	}
	return loads[userID].WaitingCount
}

func TestOpenAIWSUserWait_FirstTurnWaitsForUserSlot(t *testing.T) {
	f := newOpenAIWSAccountWaitSession(t, service.OpenAIWSIngressModePassthrough, 2*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ok, err := f.cache.AcquireUserSlot(ctx, 1702, 1, "other-user-slot")
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"first","previous_response_id":"resp_prev"}`)))
	require.Eventually(t, func() bool { return userWaitingCount(t, ctx, f.cache, 1702) == 1 }, time.Second, time.Millisecond, "透传首轮带 previous_response_id 也能等用户槽，首帧未发出")
	select {
	case <-f.requests:
		t.Fatal("等待期间不得有上游请求")
	case <-time.After(150 * time.Millisecond):
	}

	require.NoError(t, f.cache.ReleaseUserSlot(ctx, 1702, "other-user-slot"))
	_, body, err := f.conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(body, "type").String())
	<-f.requests
	require.Eventually(t, func() bool { return userWaitingCount(t, ctx, f.cache, 1702) == 0 }, time.Second, time.Millisecond)
}

func TestOpenAIWSUserWait_SubsequentTurnWaitsForUserSlot(t *testing.T) {
	f := newOpenAIWSAccountWaitSession(t, service.OpenAIWSIngressModeCtxPool, 2*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	completeOpenAIWSAccountWaitTurn(t, ctx, f)
	require.Eventually(t, func() bool { n, _ := f.cache.GetUserConcurrency(ctx, 1702); return n == 0 }, time.Second, time.Millisecond)

	ok, err := f.cache.AcquireUserSlot(ctx, 1702, 1, "other-user-slot")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"second"}`)))
	require.Eventually(t, func() bool { return userWaitingCount(t, ctx, f.cache, 1702) == 1 }, time.Second, time.Millisecond)

	require.NoError(t, f.cache.ReleaseUserSlot(ctx, 1702, "other-user-slot"))
	_, body, err := f.conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(body, "type").String())
	<-f.requests
}

func TestOpenAIWSUserWait_TimeoutAndQueueFullClose1013(t *testing.T) {
	previous := openAIWSUserWaitTimeout
	openAIWSUserWaitTimeout = 300 * time.Millisecond
	t.Cleanup(func() { openAIWSUserWaitTimeout = previous })

	t.Run("timeout", func(t *testing.T) {
		f := newOpenAIWSAccountWaitSession(t, service.OpenAIWSIngressModeCtxPool, 2*time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ok, err := f.cache.AcquireUserSlot(ctx, 1702, 1, "other-user-slot")
		require.NoError(t, err)
		require.True(t, ok)
		require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"first"}`)))
		_, _, err = f.conn.Read(ctx)
		require.Error(t, err)
		require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(err))
		require.Contains(t, err.Error(), "too many concurrent requests")
		require.Eventually(t, func() bool { return userWaitingCount(t, ctx, f.cache, 1702) == 0 }, time.Second, time.Millisecond)
	})

	t.Run("queue_full", func(t *testing.T) {
		f := newOpenAIWSAccountWaitSession(t, service.OpenAIWSIngressModeCtxPool, 2*time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ok, err := f.cache.AcquireUserSlot(ctx, 1702, 1, "other-user-slot")
		require.NoError(t, err)
		require.True(t, ok)
		queueLimit := service.CalculateMaxWait(1) - 1
		for range queueLimit {
			ok, err = f.cache.IncrementWaitCount(ctx, 1702, queueLimit)
			require.NoError(t, err)
			require.True(t, ok)
		}
		started := time.Now()
		require.NoError(t, f.conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"first"}`)))
		_, _, err = f.conn.Read(ctx)
		require.Error(t, err)
		require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(err))
		require.Less(t, time.Since(started), 250*time.Millisecond, "名额满时立即关闭，不等待")
	})
}

package handler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIWSHeldAccountSlot_TakeBeforeExpiryStopsTimer(t *testing.T) {
	var released atomic.Int32
	held := &openAIWSHeldAccountSlot{}
	refresh := func(context.Context) (bool, error) { return true, nil }
	held.park(func() { released.Add(1) }, refresh, 50*time.Millisecond, nil)
	require.True(t, held.parked())

	release, gotRefresh, heldFor := held.take()
	require.NotNil(t, release)
	require.NotNil(t, gotRefresh)
	require.GreaterOrEqual(t, heldFor, time.Duration(0))
	require.False(t, held.parked())

	time.Sleep(120 * time.Millisecond)
	require.Equal(t, int32(0), released.Load(), "取回后定时器不得再释放")
	release()
	require.Equal(t, int32(1), released.Load())
}

func TestOpenAIWSHeldAccountSlot_ExpireReleasesOnceAndTakeReturnsNil(t *testing.T) {
	var released atomic.Int32
	expired := make(chan time.Duration, 1)
	held := &openAIWSHeldAccountSlot{}
	held.park(func() { released.Add(1) }, nil, 20*time.Millisecond, func(heldFor time.Duration) { expired <- heldFor })
	select {
	case <-expired:
	case <-time.After(time.Second):
		t.Fatal("到期回调未触发")
	}
	require.Equal(t, int32(1), released.Load())
	release, _, _ := held.take()
	require.Nil(t, release)
	require.False(t, held.releaseNow())
	require.Equal(t, int32(1), released.Load(), "到期后 releaseNow 不得再次释放")
}

func TestOpenAIWSHeldAccountSlot_ReleaseNowReleasesOnce(t *testing.T) {
	var released atomic.Int32
	held := &openAIWSHeldAccountSlot{}
	held.park(func() { released.Add(1) }, nil, time.Hour, nil)
	require.True(t, held.releaseNow())
	require.False(t, held.releaseNow())
	require.Equal(t, int32(1), released.Load())
	require.False(t, held.parked())
}

func TestOpenAIWSHeldAccountSlot_ParkAgainReleasesPrevious(t *testing.T) {
	var first, second atomic.Int32
	held := &openAIWSHeldAccountSlot{}
	held.park(func() { first.Add(1) }, nil, time.Hour, nil)
	held.park(func() { second.Add(1) }, nil, time.Hour, nil)
	require.Equal(t, int32(1), first.Load(), "重复挂起先释放前一个")
	require.Equal(t, int32(0), second.Load())
	require.True(t, held.releaseNow())
	require.Equal(t, int32(1), second.Load())
}

func TestOpenAIWSHeldAccountSlot_StaleTimerCannotReleaseNewPark(t *testing.T) {
	for range 300 {
		var first, second atomic.Int32
		held := &openAIWSHeldAccountSlot{}
		held.park(func() { first.Add(1) }, nil, 0, nil)
		release, _, _ := held.take()
		held.park(func() { second.Add(1) }, nil, time.Hour, nil)
		if release != nil {
			release()
		}
		time.Sleep(2 * time.Millisecond)
		require.Equal(t, int32(0), second.Load(), "上一代定时器的回调不得释放新挂起的槽")
		require.True(t, held.parked())
		require.True(t, held.releaseNow())
		require.Equal(t, int32(1), second.Load())
		require.Equal(t, int32(1), first.Load(), "第一代要么被取回后释放，要么被自己那一代的定时器释放，总之恰好一次")
	}
}

func TestOpenAIWSHeldAccountSlot_TakeRacesExpireReleaseExactlyOnce(t *testing.T) {
	for range 300 {
		var released atomic.Int32
		held := &openAIWSHeldAccountSlot{}
		held.park(func() { released.Add(1) }, nil, 0, nil)
		var wg sync.WaitGroup
		wg.Add(1)
		var release func()
		go func() {
			defer wg.Done()
			release, _, _ = held.take()
		}()
		wg.Wait()
		if release != nil {
			release()
		}
		require.Eventually(t, func() bool { return released.Load() == 1 }, time.Second, time.Millisecond, "取回与到期竞争时恰好释放一次")
		time.Sleep(time.Millisecond)
		require.Equal(t, int32(1), released.Load())
	}
}

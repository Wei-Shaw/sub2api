package handler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// sleepAntigravitySingleAccountBackoff 测试
// ---------------------------------------------------------------------------

func TestSleepAntigravitySingleAccountBackoff_ReturnsTrue(t *testing.T) {
	ctx := context.Background()
	start := time.Now()
	ok := sleepAntigravitySingleAccountBackoff(ctx, 1)
	elapsed := time.Since(start)

	require.True(t, ok, "should return true when context is not canceled")
	// 固定延迟 2s
	require.GreaterOrEqual(t, elapsed, 1500*time.Millisecond, "should wait approximately 2s")
	require.Less(t, elapsed, 5*time.Second, "should not wait too long")
REDACTED

func TestSleepAntigravitySingleAccountBackoff_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	start := time.Now()
	ok := sleepAntigravitySingleAccountBackoff(ctx, 1)
	elapsed := time.Since(start)

	require.False(t, ok, "should return false when context is canceled")
	require.Less(t, elapsed, 500*time.Millisecond, "should return immediately on cancel")
REDACTED

func TestSleepAntigravitySingleAccountBackoff_FixedDelay(t *testing.T) {
	// 验证不同 retryCount 都使用固定 2s 延迟
	ctx := context.Background()

	start := time.Now()
	ok := sleepAntigravitySingleAccountBackoff(ctx, 5)
	elapsed := time.Since(start)

	require.True(t, ok)
	// 即使 retryCount=5，延迟仍然是固定的 2s
	require.GreaterOrEqual(t, elapsed, 1500*time.Millisecond)
	require.Less(t, elapsed, 5*time.Second)
REDACTED

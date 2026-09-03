package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEmailQueueServiceStopIsIdempotent 锁定 M16 修复：Stop 可能被多次调用
// （优雅退出与 provideCleanup 各一次），裸 close 已关闭的 channel 会 panic。
func TestEmailQueueServiceStopIsIdempotent(t *testing.T) {
	svc := NewEmailQueueService(nil, 1)
	svc.Stop()
	require.NotPanics(t, svc.Stop)
	require.NotPanics(t, svc.Stop)
}

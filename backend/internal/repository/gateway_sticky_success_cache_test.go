package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// 通用路径成功偏好：分组/会话/模型三维隔离、CAS 围栏与 TTL。
func TestGatewayStickySuccessCacheIsolationAndCAS(t *testing.T) {
	r := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: r.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewGatewayCache(client)
	ctx := context.Background()

	empty := service.GatewayStickySuccessBinding{}
	one := service.GatewayStickySuccessBinding{AccountID: 1, Revision: "first"}
	renewed := service.GatewayStickySuccessBinding{AccountID: 1, Revision: "new-success-same-account"}

	_, err := cache.GetGatewayStickySuccess(ctx, 7, "session", "model")
	require.ErrorIs(t, err, service.ErrStickySessionNotFound)

	changed, err := cache.CompareAndSwapGatewayStickySuccess(ctx, 7, "session", "model", empty, one, time.Minute)
	require.NoError(t, err)
	require.True(t, changed, "键不存在时零值 expected 必须允许建立首个偏好")

	changed, err = cache.CompareAndSwapGatewayStickySuccess(ctx, 7, "session", "model", empty, service.GatewayStickySuccessBinding{AccountID: 3, Revision: "racing-first"}, time.Minute)
	require.NoError(t, err)
	require.False(t, changed, "偏好已存在时零值 expected 不得覆盖")

	changed, err = cache.CompareAndSwapGatewayStickySuccess(ctx, 7, "session", "model", one, renewed, time.Minute)
	require.NoError(t, err)
	require.True(t, changed, "同账号再次成功按成功规则换 revision")

	changed, err = cache.CompareAndSwapGatewayStickySuccess(ctx, 7, "session", "model", one, service.GatewayStickySuccessBinding{AccountID: 2, Revision: "stale"}, time.Minute)
	require.NoError(t, err)
	require.False(t, changed, "晚完成的旧请求不得覆盖较新的成功偏好")

	got, err := cache.GetGatewayStickySuccess(ctx, 7, "session", "model")
	require.NoError(t, err)
	require.Equal(t, renewed, got)

	for _, scope := range []struct {
		group          int64
		session, model string
	}{{8, "session", "model"}, {7, "other", "model"}, {7, "session", "other"}} {
		_, err := cache.GetGatewayStickySuccess(ctx, scope.group, scope.session, scope.model)
		require.ErrorIs(t, err, service.ErrStickySessionNotFound)
	}

	// 成功偏好与旧绑定是两个独立键：写偏好不影响 sticky_session:{g}:{hash}。
	_, err = cache.GetSessionAccountID(ctx, 7, "session")
	require.ErrorIs(t, err, service.ErrStickySessionNotFound)

	r.FastForward(time.Minute)
	_, err = cache.GetGatewayStickySuccess(ctx, 7, "session", "model")
	require.ErrorIs(t, err, service.ErrStickySessionNotFound)
}

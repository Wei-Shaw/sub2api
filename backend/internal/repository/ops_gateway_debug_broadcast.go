package repository

import (
	"context"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// opsGatewayDebugBroadcastChannel 集群内广播网关调试日志开关变更的 Redis Pub/Sub 频道。
const opsGatewayDebugBroadcastChannel = "ops_gateway_debug_switch_updated"

// opsGatewayDebugBroadcaster 通过 Redis Pub/Sub 把网关调试日志开关（GatewayDebugLogEnabled /
// GatewayDebugRespEnabled）的变更实时通知到集群内其他实例。这些开关是进程内运行时状态
// （打开的日志文件句柄 + atomic），多副本部署时点击保存只会热切处理该请求的那台节点，
// 其它节点仍是旧状态。订阅方收到通知后从 DB 重新读取并应用一次热切，保证全节点一致。
type opsGatewayDebugBroadcaster struct {
	rdb *redis.Client
}

// NewOpsGatewayDebugBroadcaster 构造一个基于 Redis Pub/Sub 的广播器。
// 传入的 rdb 为 nil 时返回 nil，让 wire 层自然降级为「不广播」。
func NewOpsGatewayDebugBroadcaster(rdb *redis.Client) service.GatewayDebugBroadcaster {
	if rdb == nil {
		return nil
	}
	return &opsGatewayDebugBroadcaster{rdb: rdb}
}

// Publish 通知其他实例：DB 中的网关调试日志开关已更新。
func (b *opsGatewayDebugBroadcaster) Publish(ctx context.Context) error {
	if b == nil || b.rdb == nil {
		return errors.New("nil ops gateway debug broadcaster")
	}
	// payload 目前只用作触发信号，订阅方会重新从 DB 读取最新开关。
	return b.rdb.Publish(ctx, opsGatewayDebugBroadcastChannel, "refresh").Err()
}

// Subscribe 启动后台 goroutine 监听广播，收到通知时调用 handler。
// 生命周期由传入的 ctx 控制；ctx 结束会自动关闭订阅并退出。
func (b *opsGatewayDebugBroadcaster) Subscribe(ctx context.Context, handler func()) {
	if b == nil || b.rdb == nil || handler == nil {
		return
	}
	go func() {
		sub := b.rdb.Subscribe(ctx, opsGatewayDebugBroadcastChannel)
		defer func() { _ = sub.Close() }()

		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok || msg == nil {
					// go-redis 会在断线后自动重连，通道被关闭意味着订阅已终止，直接退出。
					return
				}
				func() {
					defer func() {
						if r := recover(); r != nil {
							logger.With(
								zap.String("component", "ops.gateway_debug_broadcast"),
								zap.Any("panic", r),
							).Warn("gateway debug broadcast handler panicked")
						}
					}()
					handler()
				}()
			}
		}
	}()
}

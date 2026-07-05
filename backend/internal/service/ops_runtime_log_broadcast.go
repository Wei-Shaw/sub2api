package service

import (
	"context"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// opsRuntimeLogBroadcastChannel 集群内广播运行时日志配置变更的 Redis Pub/Sub 频道。
const opsRuntimeLogBroadcastChannel = "ops_runtime_log_config_updated"

// opsRuntimeLogBroadcaster 通过 Redis Pub/Sub 把 UpdateRuntimeLogConfig / ResetRuntimeLogConfig
// 的变更实时通知到集群内其他实例，解决多副本部署时点击保存只对处理请求的那台节点生效的问题。
type opsRuntimeLogBroadcaster struct {
	rdb *redis.Client
}

// NewOpsRuntimeLogBroadcaster 构造一个基于 Redis Pub/Sub 的广播器。
// 传入的 rdb 为 nil 时返回 nil，让 wire 层自然降级为「不广播」。
func NewOpsRuntimeLogBroadcaster(rdb *redis.Client) RuntimeLogBroadcaster {
	if rdb == nil {
		return nil
	}
	return &opsRuntimeLogBroadcaster{rdb: rdb}
}

// Publish 通知其他实例：DB 中的运行时日志配置已更新。
func (b *opsRuntimeLogBroadcaster) Publish(ctx context.Context) error {
	if b == nil || b.rdb == nil {
		return errors.New("nil ops runtime log broadcaster")
	}
	// payload 目前只用作触发信号，订阅方会重新从 DB 读取最新配置。
	return b.rdb.Publish(ctx, opsRuntimeLogBroadcastChannel, "refresh").Err()
}

// Subscribe 启动后台 goroutine 监听广播，收到通知时调用 handler。
// 生命周期由传入的 ctx 控制；ctx 结束会自动关闭订阅并退出。
func (b *opsRuntimeLogBroadcaster) Subscribe(ctx context.Context, handler func()) {
	if b == nil || b.rdb == nil || handler == nil {
		return
	}
	go func() {
		sub := b.rdb.Subscribe(ctx, opsRuntimeLogBroadcastChannel)
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
								zap.String("component", "ops.runtime_log_broadcast"),
								zap.Any("panic", r),
							).Warn("runtime log broadcast handler panicked")
						}
					}()
					handler()
				}()
			}
		}
	}()
}

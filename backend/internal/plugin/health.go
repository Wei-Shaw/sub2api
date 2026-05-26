package plugin

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// HealthMonitor 周期性调用插件的 HealthCheck RPC,连续失败超过阈值后触发回调。
type HealthMonitor struct {
	interval      time.Duration
	failThreshold int
	callTimeout   time.Duration
}

// NewHealthMonitor 创建监控器。
// interval 表示两次检查的间隔,failThreshold 表示触发 onFail 所需的连续失败次数。
// 单次 RPC 默认在 interval 的 80% 时长内必须返回,避免重叠调用造成堆积。
func NewHealthMonitor(interval time.Duration, failThreshold int) *HealthMonitor {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if failThreshold <= 0 {
		failThreshold = 3
	}
	return &HealthMonitor{
		interval:      interval,
		failThreshold: failThreshold,
		callTimeout:   interval * 8 / 10,
	}
}

// Monitor 启动监控循环,直到 ctx 取消。
// onFail 在连续 failThreshold 次失败后被调用一次,然后循环退出。
// 如果 inst 或其 LifecycleStub 为 nil,函数立即返回。
func (m *HealthMonitor) Monitor(ctx context.Context, inst *PluginInstance, onFail func()) {
	if inst == nil {
		return
	}
	stub := inst.LifecycleStub
	if stub == nil {
		slog.Warn("health monitor: nil lifecycle stub", "plugin", inst.Name)
		return
	}

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	consecutiveFails := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ok := m.singleCheck(ctx, stub)
			if ok {
				consecutiveFails = 0
				continue
			}
			consecutiveFails++
			slog.Warn("plugin health check failed",
				"plugin", inst.Name,
				"consecutive_failures", consecutiveFails,
				"threshold", m.failThreshold,
			)
			if consecutiveFails >= m.failThreshold {
				if onFail != nil {
					onFail()
				}
				return
			}
		}
	}
}

// singleCheck 调用一次 HealthCheck,带超时控制。
func (m *HealthMonitor) singleCheck(ctx context.Context, stub pluginsdk.PluginLifecycleClient) bool {
	checkCtx, cancel := context.WithTimeout(ctx, m.callTimeout)
	defer cancel()

	resp, err := stub.HealthCheck(checkCtx, &emptypb.Empty{})
	if err != nil {
		return false
	}
	return resp != nil && resp.Healthy
}

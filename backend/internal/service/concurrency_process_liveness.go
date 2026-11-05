package service

import (
	"context"
	"os"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// processHeartbeatInterval 必须远小于缓存侧的存活窗口（Redis 实现为 60 秒），
// 偶发的一两次心跳失败不能让仍在服务的进程被同伴当成已死进程清掉槽位。
const (
	processHeartbeatInterval = 10 * time.Second
	processHeartbeatTimeout  = 3 * time.Second
)

// ProcessLivenessCache 是可选能力：实现它的缓存让启动清理能区分已死进程的残留
// 和同一 Redis 上其他仍在服务的进程（蓝绿两槽、worker）。未实现时启动清理照旧
// 把所有非本进程前缀视为残留，与单进程部署的语义一致。
type ProcessLivenessCache interface {
	// HeartbeatProcess 记录 requestPrefix 所属进程仍存活。instanceID 跨进程重启保持不变、
	// 但同时运行的进程之间互不相同；同一 instanceID 下更早的进程视为已死。
	HeartbeatProcess(ctx context.Context, requestPrefix, instanceID string) error
}

// processInstanceID 用主机名识别「同一实例的上一代进程」：容器重启保留主机名，
// 同时运行的不同容器主机名不同。取不到时返回空，只是不做同实例识别。
func processInstanceID() string {
	host, err := os.Hostname()
	if err != nil {
		return ""
	}
	return host
}

func (s *ConcurrencyService) heartbeatProcess(ctx context.Context) error {
	if s == nil {
		return nil
	}
	liveness, ok := s.cache.(ProcessLivenessCache)
	if !ok {
		return nil
	}
	return liveness.HeartbeatProcess(ctx, RequestIDPrefix(), processInstanceID())
}

// StartProcessHeartbeat 周期性刷新本进程心跳，让之后启动的进程保留本进程的在途槽位。
func (s *ConcurrencyService) StartProcessHeartbeat() {
	if s == nil {
		return
	}
	if _, ok := s.cache.(ProcessLivenessCache); !ok {
		return
	}
	go func() {
		ticker := time.NewTicker(processHeartbeatInterval)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), processHeartbeatTimeout)
			err := s.heartbeatProcess(ctx)
			cancel()
			if err != nil {
				logger.LegacyPrintf("service.concurrency", "Warning: process heartbeat failed: %v", err)
			}
		}
	}()
}

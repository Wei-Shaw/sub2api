package node

// xiugai 添加节点功能

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	gopsnet "github.com/shirou/gopsutil/v4/net"
)

// systemMetrics 保存某一时刻的主机资源使用率快照。
type systemMetrics struct {
	// CPUUsage CPU 总使用率百分比（0~100）。
	CPUUsage float64
	// MemUsage 虚拟内存使用率百分比（0~100）。
	MemUsage float64
}

// collectSystemMetrics 采集当前主机的 CPU 和内存使用率。
// CPU 采样会阻塞约 1 秒以计算区间平均值；采集失败时对应字段保持为 0。
//
// 参数：
//   - ctx：上下文，用于提前取消采集（如心跳超时时）。
//
// 返回值：
//   - systemMetrics：包含 CPU 和内存使用率的快照结构体。
func collectSystemMetrics(ctx context.Context) systemMetrics {
	var m systemMetrics

	// cpu.PercentWithContext 会阻塞 1 s 以测量区间 CPU 使用率，false 表示返回全核平均值。
	if percents, err := cpu.PercentWithContext(ctx, time.Second, false); err == nil && len(percents) > 0 {
		m.CPUUsage = percents[0]
	}

	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		m.MemUsage = vm.UsedPercent
	}

	return m
}

// detectInternalIP 遍历本机所有已启动的非回环网络接口，返回第一个找到的
// 非回环单播 IPv4 地址字符串。若未找到任何合适地址，返回空字符串。
//
// 返回值：
//   - string：内网 IPv4 地址字符串，例如 "10.0.0.12"；未检测到时为空字符串。
func detectInternalIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		// 跳过已关闭或回环接口。
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			// 仅返回 IPv4 地址。
			if v4 := ip.To4(); v4 != nil {
				return v4.String()
			}
		}
	}
	return ""
}

// xiugai 添加节点功能 - 带宽采样器

// bandwidthReading 保存一次带宽采样的结果，单位 MB/s。
type bandwidthReading struct {
	// UploadMBps 上行带宽，单位 MB/s。
	UploadMBps float64
	// DownloadMBps 下行带宽，单位 MB/s。
	DownloadMBps float64
}

// bandwidthSampler 在独立 goroutine 中每 5 秒测量一次当前 1 秒内的
// 网络上行/下行带宽，并将结果缓存供心跳上报读取。
type bandwidthSampler struct {
	mu      sync.RWMutex
	latest  bandwidthReading
}

// newBandwidthSampler 创建带宽采样器实例。
// 需调用 Start(ctx) 启动后台采样 goroutine。
//
// 返回值：
//   - *bandwidthSampler：新建的采样器，初始带宽读数均为 0。
func newBandwidthSampler() *bandwidthSampler {
	return &bandwidthSampler{}
}

// Start 启动带宽采样后台 goroutine：立即执行一次采样，之后每 5 秒执行一次。
// 与心跳上报的间隔对齐，保证心跳发出前带宽读数已更新。
// ctx 取消时 goroutine 退出。
//
// 参数：
//   - ctx：上下文，取消后采样 goroutine 停止。
func (s *bandwidthSampler) Start(ctx context.Context) {
	go s.run(ctx)
}

// run 是 bandwidthSampler 的后台循环，由 Start 启动。
//
// 参数：
//   - ctx：上下文，取消时退出循环。
func (s *bandwidthSampler) run(ctx context.Context) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	// 立即执行一次，使首次心跳携带带宽数据。
	s.sample(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sample(ctx)
		}
	}
}

// sample 在 1 秒窗口内采集两次网络 I/O 计数器，计算期间的收发速率（MB/s）
// 并更新缓存。采集失败时保持上一次的读数不变。
//
// 参数：
//   - ctx：上下文；若在等待 1 秒期间 ctx 被取消则立即返回。
func (s *bandwidthSampler) sample(ctx context.Context) {
	// 第一次读取网络 I/O 计数器（pernic=false 表示返回所有接口的汇总值）。
	counters1, err := gopsnet.IOCountersWithContext(ctx, false)
	if err != nil || len(counters1) == 0 {
		return
	}

	// 等待 1 秒作为采样窗口。
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Second):
	}

	// 第二次读取，与第一次作差得到 1 秒内的字节增量。
	counters2, err := gopsnet.IOCountersWithContext(ctx, false)
	if err != nil || len(counters2) == 0 {
		return
	}

	// 字节差值除以 1048576 转换为 MB/s。
	upload := float64(counters2[0].BytesSent-counters1[0].BytesSent) / 1048576.0
	download := float64(counters2[0].BytesRecv-counters1[0].BytesRecv) / 1048576.0

	// 防止计数器回绕导致负值（系统重启或溢出时）。
	if upload < 0 {
		upload = 0
	}
	if download < 0 {
		download = 0
	}

	s.mu.Lock()
	s.latest = bandwidthReading{UploadMBps: upload, DownloadMBps: download}
	s.mu.Unlock()
}

// Latest 返回最近一次采样得到的带宽读数（单位 MB/s）。
// 若采样器尚未完成首次采样，返回零值。
//
// 返回值：
//   - bandwidthReading：包含上行和下行带宽（MB/s）的结构体。
func (s *bandwidthSampler) Latest() bandwidthReading {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latest
}

// end

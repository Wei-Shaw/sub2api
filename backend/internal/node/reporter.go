package node

// xiugai 添加节点功能

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"time"
)

// heartbeatInterval 心跳上报的时间间隔，每 5 秒向控制服务器发送一次心跳。
const heartbeatInterval = 5 * time.Second

// heartbeatRequest 对应控制服务器 POST /api/v1/heartbeat 的请求体结构。
type heartbeatRequest struct {
	// InternalIP 节点内网 IP。
	InternalIP string `json:"internal_ip"`
	// NodeName 节点名称，与控制服务器中的记录对应。
	NodeName string `json:"node_name"`
	// ListenPort 本节点的服务监听端口。
	ListenPort int `json:"listen_port"`
	// CPUUsage CPU 使用率百分比，可选。
	CPUUsage float64 `json:"cpu_usage,omitempty"`
	// MemUsage 内存使用率百分比，可选。
	MemUsage float64 `json:"mem_usage,omitempty"`
	// xiugai 添加节点功能
	// UploadBandwidth 当前系统上行带宽，单位 MB/s，可选。
	UploadBandwidth float64 `json:"upload_bandwidth,omitempty"`
	// DownloadBandwidth 当前系统下行带宽，单位 MB/s，可选。
	DownloadBandwidth float64 `json:"download_bandwidth,omitempty"`
	// end
}

// accountStatus 对应控制服务器账号列表中单个账号的状态信息。
type accountStatus struct {
	// ID 账号唯一标识（本地数据库主键转字符串）。
	ID string `json:"id"`
	// Name 账号显示名称。
	Name string `json:"name"`
	// Status 账号当前状态，如 active、disabled、error 等。
	Status string `json:"status"`
}

// accountsStatusRequest 对应控制服务器 POST /api/v1/accounts/status 的请求体结构。
type accountsStatusRequest struct {
	// NodeName 节点名称，用于在控制服务器端定位节点。
	NodeName string `json:"node_name"`
	// Accounts 本节点当前所有账号状态的完整快照，会覆盖服务器端旧数据。
	Accounts []accountStatus `json:"accounts"`
}

// NodeAccount 是上报服务所需的账号信息结构，由 AccountLister 返回。
type NodeAccount struct {
	// ID 账号数据库主键。
	ID int64
	// Name 账号显示名称。
	Name string
	// Status 账号原始数据库状态（active/error/disabled）。
	Status string
	// Schedulable 账号是否被管理员标记为可调度。
	Schedulable bool
	// RateLimitResetAt 速率限制（429）解除时间，nil 表示未限流。
	RateLimitResetAt *time.Time
	// OverloadUntil 过载（529）解除时间，nil 表示未过载。
	OverloadUntil *time.Time
	// TempUnschedulableUntil 临时不可调度解除时间，nil 表示正常。
	TempUnschedulableUntil *time.Time
	// QuotaExceeded 是否已超出配额限制。
	QuotaExceeded bool
}

// effectiveStatus 综合多个字段计算账号的有效展示状态，与前端 AccountStatusIndicator 逻辑一致。
// 优先级（从高到低）：rate_limited > overloaded > error > temp_unschedulable > disabled > quota_exceeded > paused > active
func effectiveStatus(a NodeAccount) string {
	now := time.Now()
	if a.RateLimitResetAt != nil && now.Before(*a.RateLimitResetAt) {
		return "速率限制"
	}
	if a.OverloadUntil != nil && now.Before(*a.OverloadUntil) {
		return "过载"
	}
	if a.Status == "error" {
		return "错误"
	}
	if a.TempUnschedulableUntil != nil && now.Before(*a.TempUnschedulableUntil) {
		return "临时不可调度"
	}
	if a.Status != "active" {
		return a.Status
	}
	if a.QuotaExceeded {
		return "配额超限"
	}
	if !a.Schedulable {
		return "关闭调度"
	}
	return "正常"
}

// AccountLister 定义获取本地全量账号的接口，由 accountRepoLister 适配器实现。
type AccountLister interface {
	// ListAll 返回本节点所有账号，无论状态如何。
	// 参数：
	//   - ctx：上下文，用于超时和取消控制。
	// 返回值：
	//   - []NodeAccount：所有账号的快照切片。
	//   - error：查询失败时返回错误。
	ListAll(ctx context.Context) ([]NodeAccount, error)
}

// Reporter 负责定期向控制服务器发送心跳，并在账号状态发生变化时上传完整快照。
// 创建后调用 Start(ctx) 启动，ctx 取消时自动停止。
type Reporter struct {
	// cfg 节点配置，包含节点名称、控制服务器地址等信息。
	cfg *Config
	// client 向控制服务器发送 HTTP 请求的客户端。
	client *controlClient
	// lister 提供本地账号列表的接口实现。
	lister AccountLister
	// internalIP 上报心跳时携带的节点内网 IP。
	internalIP string

	mu sync.Mutex
	// lastSnapshot 上次成功上传时的账号状态哈希字符串，用于变更检测。
	lastSnapshot string
	// pendingUpload 标记是否需要在下次心跳后立即上传账号状态（首次或上次上传失败时为 true）。
	pendingUpload bool

	// xiugai 添加节点功能
	// bandwidth 独立后台带宽采样器，每 5 秒测量一次 1 秒窗口内的网络收发速率。
	bandwidth *bandwidthSampler
	// end
}

// NewReporter 创建并初始化 Reporter。
// 节点内网 IP 始终通过 detectInternalIP() 从系统网络接口自动获取。
//
// 参数：
//   - cfg：节点配置对象，包含控制服务器地址和 TLS 证书路径等。
//   - lister：提供本地账号列表的 AccountLister 实现。
//
// 返回值：
//   - *Reporter：初始化完成的上报器实例。
//   - error：HTTP 客户端构建失败时返回错误。
func NewReporter(cfg *Config, lister AccountLister) (*Reporter, error) {
	client, err := newControlClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("node reporter: build client: %w", err)
	}

	return &Reporter{
		cfg:           cfg,
		client:        client,
		lister:        lister,
		internalIP:    detectInternalIP(),
		pendingUpload: true, // 首次启动时必须上传一次账号状态。
		// xiugai 添加节点功能
		bandwidth: newBandwidthSampler(),
		// end
	}, nil
}

// Start 启动心跳上报循环：立即执行一次，之后每 5 秒执行一次。
// 该方法会阻塞，直到 ctx 被取消后退出。
//
// 参数：
//   - ctx：上下文，取消后上报循环停止。
func (r *Reporter) Start(ctx context.Context) {
	slog.Info("node reporter started",
		"node", r.cfg.NodeName,
		"control", r.cfg.ControlAddr,
		"interval", heartbeatInterval,
	)

	// xiugai 添加节点功能
	// 启动带宽采样后台 goroutine，与主心跳循环共享同一 ctx。
	r.bandwidth.Start(ctx)
	// end

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	// 立即执行第一次上报，无需等待第一个 tick。
	r.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("node reporter stopped", "node", r.cfg.NodeName)
			return
		case <-ticker.C:
			r.tick(ctx)
		}
	}
}

// tick 执行单次心跳上报，并在必要时上传账号状态。
// 若心跳请求失败，则跳过账号上传（控制服务器要求先注册心跳才能接受账号状态）。
//
// 参数：
//   - ctx：父上下文，tick 内部会派生带超时的子上下文。
func (r *Reporter) tick(ctx context.Context) {
	// 为单次 tick 设置超时，预留 500ms 给网络 I/O，避免阻塞下次心跳。
	tickCtx, cancel := context.WithTimeout(ctx, heartbeatInterval-500*time.Millisecond)
	defer cancel()

	metrics := collectSystemMetrics(tickCtx)

	// xiugai 添加节点功能
	// 读取最近一次带宽采样结果，随心跳一起上报。
	bw := r.bandwidth.Latest()
	// end

	if err := r.client.post(tickCtx, "/heartbeat", heartbeatRequest{
		InternalIP:        r.internalIP,
		NodeName:          r.cfg.NodeName,
		ListenPort:        r.cfg.NodePort,
		CPUUsage:          metrics.CPUUsage,
		MemUsage:          metrics.MemUsage,
		UploadBandwidth:   bw.UploadMBps,
		DownloadBandwidth: bw.DownloadMBps,
	}); err != nil {
		slog.Warn("node heartbeat failed", "node", r.cfg.NodeName, "error", err)
		// 心跳失败时不尝试上传账号状态，因为控制服务器要求先有心跳记录。
		return
	}

	r.maybeUploadAccounts(tickCtx)
}

// maybeUploadAccounts 从本地数据库读取全量账号，与上次快照对比。
// 若内容发生变化（或标记了 pendingUpload），则向控制服务器上传完整账号状态列表。
// 上传失败时设置 pendingUpload=true，下次 tick 时重试。
//
// 参数：
//   - ctx：请求上下文，用于超时控制。
func (r *Reporter) maybeUploadAccounts(ctx context.Context) {
	accounts, err := r.lister.ListAll(ctx)
	if err != nil {
		slog.Warn("node: list accounts failed", "error", err)
		return
	}

	// 将 NodeAccount 转换为 accountStatus 上报结构，使用综合状态。
	statuses := make([]accountStatus, 0, len(accounts))
	for _, a := range accounts {
		statuses = append(statuses, accountStatus{
			ID:     strconv.FormatInt(a.ID, 10),
			Name:   a.Name,
			Status: effectiveStatus(a),
		})
	}

	snapshot := buildSnapshot(statuses)

	r.mu.Lock()
	shouldUpload := r.pendingUpload || snapshot != r.lastSnapshot
	r.mu.Unlock()

	if !shouldUpload {
		return
	}

	if err := r.client.post(ctx, "/accounts/status", accountsStatusRequest{
		NodeName: r.cfg.NodeName,
		Accounts: statuses,
	}); err != nil {
		slog.Warn("node: account status upload failed",
			"node", r.cfg.NodeName, "error", err)
		// 标记上传待重试，下次 tick 会再次尝试。
		r.mu.Lock()
		r.pendingUpload = true
		r.mu.Unlock()
		return
	}

	slog.Info("node: account status uploaded",
		"node", r.cfg.NodeName, "count", len(statuses))

	// 上传成功后保存本次快照，清除重试标记。
	r.mu.Lock()
	r.lastSnapshot = snapshot
	r.pendingUpload = false
	r.mu.Unlock()
}

// buildSnapshot 将账号状态列表转换为确定性字符串，用于变更检测。
// 按 id 排序后拼接 "id:status\n"，相同账号集合必然产生相同字符串。
//
// 参数：
//   - statuses：待生成快照的账号状态切片。
//
// 返回值：
//   - string：排序后拼接的快照字符串。
func buildSnapshot(statuses []accountStatus) string {
	pairs := make([]string, len(statuses))
	for i, s := range statuses {
		pairs[i] = s.ID + ":" + s.Status
	}
	sort.Strings(pairs)
	out := ""
	for _, p := range pairs {
		out += p + "\n"
	}
	return out
}

// end

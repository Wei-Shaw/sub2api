// Package service — trusted_proxy_resolver.go 实现"可信代理动态拉取"
// (switch-trusted-proxies-dynamic) 的运行时核心。
//
// 职责：
//
//  1. 组合 3 类 CIDR 源，去重合并后写入 Gin 的 SetTrustedProxies：
//     - 静态列表（config.yaml -> server.trusted_proxies）
//     - admin 面板固定条目 (setting: trusted_proxies_dynamic_extra_cidrs)
//     - 动态拉取（每个 enabled source 的 URL 内容缓存）
//
//  2. 按 sources 配置管理一组后台 goroutine：
//     - Start 时按每个 enabled source 起 goroutine（首次立即拉一次，之后按 interval ticker）
//     - Reconfigure 时停旧的 goroutine，按新配置重开
//     - Stop（Context 取消）时所有 goroutine 优雅退出
//
//  3. 拉取失败时保留上一次成功缓存 + 记录 last_error（对外通过 GetStatus 展示）。
//
// 线程安全：
//   - Rebuild 内部 mu 串行化对 gin.Engine.SetTrustedProxies 的调用（Gin 官方
//     没有明确并发保证，但看源码只是替换内部 []*net.IPNet 切片指针，串行足够）；
//   - sourceCache 用 sync.Map；sources / extraCIDRs 用 atomic.Pointer；
//   - Reconfigure 与 fetchAndUpdate 之间的竞争由 configMu 保护。
package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// TrustedProxySourceStatus 是单个拉取源的运行时状态快照，用于 admin GET 响应。
type TrustedProxySourceStatus struct {
	ID            string    `json:"id"`
	LastRunAt     time.Time `json:"last_run_at,omitempty"`
	LastSuccessAt time.Time `json:"last_success_at,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
	CIDRCount     int       `json:"cidr_count"`
	NextRunAt     time.Time `json:"next_run_at,omitempty"`
}

// TrustedProxyResolver 负责合并/去重/rebuild Gin trusted proxies。
type TrustedProxyResolver struct {
	engine      *gin.Engine // 指向全局 gin engine，用于调 SetTrustedProxies
	staticCIDRs []string    // 来自 config.yaml，进程生命周期不变
	httpClient  *http.Client

	// runtime 配置（可被 Reconfigure 原子替换）
	enabled    atomic.Bool
	sources    atomic.Pointer[[]TrustedProxyDynamicSource]
	extraCIDRs atomic.Pointer[[]string]

	// sourceCache: sourceID -> 上次成功拉取到的 []string（CIDR 列表）
	// 键始终小写化避免歧义。
	sourceCache sync.Map
	// sourceStatus: sourceID -> *TrustedProxySourceStatus（原子指针，读方 copy 值）
	sourceStatus sync.Map

	// rebuildMu 串行化 Rebuild → engine.SetTrustedProxies 调用
	rebuildMu sync.Mutex

	// configMu 保护 Reconfigure 停旧 goroutine / 起新 goroutine 的原子性
	configMu     sync.Mutex
	workerCancel context.CancelFunc
	workerWG     sync.WaitGroup
	rootCtx      context.Context // 用于派生 worker ctx；由 Start 设定
	started      atomic.Bool
}

// NewTrustedProxyResolver 构造 resolver。engine 通常来自 ProvideRouter；staticCIDRs
// 来自 config.yaml。此时未启动 goroutine，需 admin/main 稍后调 Start(ctx)。
func NewTrustedProxyResolver(engine *gin.Engine, staticCIDRs []string) *TrustedProxyResolver {
	staticCopy := append([]string(nil), staticCIDRs...)
	r := &TrustedProxyResolver{
		engine:      engine,
		staticCIDRs: staticCopy,
		httpClient:  &http.Client{Timeout: time.Duration(TrustedProxyTimeoutSecondsDefault) * time.Second},
	}
	// 初始 empty 值，避免 nil 解引用
	empty := []TrustedProxyDynamicSource{}
	r.sources.Store(&empty)
	emptyCIDRs := []string{}
	r.extraCIDRs.Store(&emptyCIDRs)
	return r
}

// Configure 在启动前（或 admin 保存 setting 后）注入最新配置；不启动 goroutine。
//
// 参数：
//   - enabled: 总开关
//   - sources: 拉取源列表（已通过 Normalize 校验）
//   - extraCIDRs: admin 固定 CIDR（已通过 Normalize 校验）
//
// 语义：只更新 in-memory 配置。Start 之前调用 → 记录状态；Start 之后调用相当于
// Reconfigure（Start 后请优先用 Reconfigure）。
func (r *TrustedProxyResolver) Configure(enabled bool, sources []TrustedProxyDynamicSource, extraCIDRs []string) {
	r.enabled.Store(enabled)
	srcCopy := append([]TrustedProxyDynamicSource(nil), sources...)
	r.sources.Store(&srcCopy)
	extraCopy := append([]string(nil), extraCIDRs...)
	r.extraCIDRs.Store(&extraCopy)
}

// Start 启动后台 goroutine 组。传入的 ctx 生命周期由调用方管理
// （通常是进程 root context）。首次调用有效，重复调用直接返回。
func (r *TrustedProxyResolver) Start(ctx context.Context) {
	if !r.started.CompareAndSwap(false, true) {
		return
	}
	r.configMu.Lock()
	r.rootCtx = ctx
	r.configMu.Unlock()

	// 无论 enabled 与否，都先做一次 Rebuild（把 static + extra 生效）；
	// 若 enabled=true，再启动 workers。
	if err := r.Rebuild(); err != nil {
		slog.Warn("trusted_proxy_resolver: initial rebuild failed", slog.String("error", err.Error()))
	}
	if r.enabled.Load() {
		r.spawnWorkers()
	}
}

// Reconfigure 在 admin 保存 setting 后调用。安全地停旧 goroutine → 更新配置 → 起新 goroutine。
// 无论 enabled 变化与否，都会触发一次 Rebuild（extra_cidrs 变化也需要立即生效）。
func (r *TrustedProxyResolver) Reconfigure(enabled bool, sources []TrustedProxyDynamicSource, extraCIDRs []string) {
	r.configMu.Lock()
	defer r.configMu.Unlock()

	// 1. 停旧 workers
	r.stopWorkersLocked()

	// 2. 更新配置
	r.Configure(enabled, sources, extraCIDRs)

	// 3. 立即 Rebuild（把 static + extra + 现有 sourceCache 生效）
	if err := r.Rebuild(); err != nil {
		slog.Warn("trusted_proxy_resolver: rebuild after reconfigure failed", slog.String("error", err.Error()))
	}

	// 4. 若 enabled 且 resolver 已 Start，则起新 workers
	if enabled && r.started.Load() && r.rootCtx != nil {
		r.spawnWorkers()
	}
}

// Rebuild 合并 static + extra + all source caches 去重后写入 gin engine。
// 该方法可被并发调用，内部靠 rebuildMu 串行化。
func (r *TrustedProxyResolver) Rebuild() error {
	r.rebuildMu.Lock()
	defer r.rebuildMu.Unlock()

	merged := r.mergedCIDRs()
	if len(merged) > TrustedProxyMergedMaxCIDRs {
		// 极端情况：拉取源突然返回海量数据。截断后继续。
		slog.Warn("trusted_proxy_resolver: merged CIDR count exceeds cap, truncating",
			slog.Int("count", len(merged)), slog.Int("cap", TrustedProxyMergedMaxCIDRs))
		merged = merged[:TrustedProxyMergedMaxCIDRs]
	}

	if r.engine == nil {
		return nil
	}
	// SetTrustedProxies 允许 nil，等价"不信任任何代理，直接用 RemoteAddr"。
	// 我们传空切片时依然让 Gin 保留 nil 语义。
	var toSet []string
	if len(merged) > 0 {
		toSet = merged
	}
	if err := r.engine.SetTrustedProxies(toSet); err != nil {
		return fmt.Errorf("gin SetTrustedProxies: %w", err)
	}
	slog.Info("trusted_proxy_resolver: rebuild ok",
		slog.Int("total", len(merged)),
		slog.Int("static", len(r.staticCIDRs)),
		slog.Int("extra", len(*r.extraCIDRs.Load())),
	)
	return nil
}

// StaticCIDRs 返回静态 config.yaml CIDR 列表快照（TrustedProxySnapshotProvider 接口）。
func (r *TrustedProxyResolver) StaticCIDRs() []string {
	return append([]string(nil), r.staticCIDRs...)
}

// SourceStatuses 是 GetSourceStatuses 的接口别名（TrustedProxySnapshotProvider 接口）。
func (r *TrustedProxyResolver) SourceStatuses() []TrustedProxySourceStatus {
	return r.GetSourceStatuses()
}

// GetSourceStatuses 返回所有 source 的最新状态快照（浅拷贝，可安全返回给 handler 序列化）。
func (r *TrustedProxyResolver) GetSourceStatuses() []TrustedProxySourceStatus {
	sources := *r.sources.Load()
	out := make([]TrustedProxySourceStatus, 0, len(sources))
	for _, s := range sources {
		if v, ok := r.sourceStatus.Load(s.ID); ok {
			if st, ok := v.(*TrustedProxySourceStatus); ok && st != nil {
				out = append(out, *st)
				continue
			}
		}
		out = append(out, TrustedProxySourceStatus{ID: s.ID})
	}
	return out
}

// ─── internals ──────────────────────────────────────────────────────────────

// mergedCIDRs 收集三类源，去重后返回排序切片。
func (r *TrustedProxyResolver) mergedCIDRs() []string {
	seen := make(map[string]struct{}, 64)
	push := func(items []string) {
		for _, item := range items {
			normalized := normalizeCIDROrIP(item)
			if normalized == "" {
				continue
			}
			seen[normalized] = struct{}{}
		}
	}
	push(r.staticCIDRs)
	push(*r.extraCIDRs.Load())
	if r.enabled.Load() {
		sources := *r.sources.Load()
		for _, src := range sources {
			if !src.Enabled {
				continue
			}
			if v, ok := r.sourceCache.Load(src.ID); ok {
				if list, ok := v.([]string); ok {
					push(list)
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out) // 排序保证 Rebuild 结果稳定，便于日志/审计对比
	return out
}

// spawnWorkers 按当前 enabled sources 起后台 goroutine。调用方需持有 configMu。
func (r *TrustedProxyResolver) spawnWorkers() {
	if r.rootCtx == nil {
		return
	}
	ctx, cancel := context.WithCancel(r.rootCtx)
	r.workerCancel = cancel
	sources := *r.sources.Load()
	for _, src := range sources {
		if !src.Enabled {
			continue
		}
		r.workerWG.Add(1)
		go r.runSource(ctx, src)
	}
}

// stopWorkersLocked 停掉当前 workers 并等待退出。调用方需持有 configMu。
func (r *TrustedProxyResolver) stopWorkersLocked() {
	if r.workerCancel != nil {
		r.workerCancel()
		r.workerCancel = nil
	}
	r.workerWG.Wait()
}

// Shutdown 停止所有 goroutine（一般用于测试；生产 rootCtx cancel 自然会 stop）。
func (r *TrustedProxyResolver) Shutdown() {
	r.configMu.Lock()
	defer r.configMu.Unlock()
	r.stopWorkersLocked()
}

// runSource 单个 source 的循环：启动立即拉一次 → 按 interval ticker 定期拉。
func (r *TrustedProxyResolver) runSource(ctx context.Context, src TrustedProxyDynamicSource) {
	defer r.workerWG.Done()

	// 首次立即拉，让新添加/启用的 source 快速生效
	r.fetchAndUpdate(ctx, src)

	interval := time.Duration(src.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = time.Duration(TrustedProxyIntervalSecondsDefault) * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.fetchAndUpdate(ctx, src)
		}
	}
}

// fetchAndUpdate 执行一次拉取 + 缓存更新 + rebuild。失败时保留上一次成功缓存。
func (r *TrustedProxyResolver) fetchAndUpdate(ctx context.Context, src TrustedProxyDynamicSource) {
	now := time.Now()
	status := &TrustedProxySourceStatus{
		ID:        src.ID,
		LastRunAt: now,
		NextRunAt: now.Add(time.Duration(src.IntervalSeconds) * time.Second),
	}
	// 保留上次已知 count（失败时前端仍展示旧值，便于对比）
	if prev, ok := r.sourceStatus.Load(src.ID); ok {
		if pst, ok := prev.(*TrustedProxySourceStatus); ok && pst != nil {
			status.CIDRCount = pst.CIDRCount
			status.LastSuccessAt = pst.LastSuccessAt
		}
	}

	cidrs, err := r.fetchSource(ctx, src)
	if err != nil {
		status.LastError = err.Error()
		r.sourceStatus.Store(src.ID, status)
		slog.Warn("trusted_proxy_resolver: fetch failed, keeping previous cache",
			slog.String("source_id", src.ID),
			slog.String("url", src.URL),
			slog.String("error", err.Error()),
		)
		return
	}

	// 成功：写缓存 + 状态 + 触发 Rebuild
	r.sourceCache.Store(src.ID, cidrs)
	status.LastError = ""
	status.LastSuccessAt = now
	status.CIDRCount = len(cidrs)
	r.sourceStatus.Store(src.ID, status)

	if err := r.Rebuild(); err != nil {
		slog.Warn("trusted_proxy_resolver: rebuild after fetch failed",
			slog.String("source_id", src.ID),
			slog.String("error", err.Error()),
		)
	} else {
		slog.Info("trusted_proxy_resolver: fetch ok",
			slog.String("source_id", src.ID),
			slog.Int("cidrs", len(cidrs)),
		)
	}
}

// fetchSource 抓取单个 URL 的 CIDR 列表。返回规范化的字符串数组（去重）。
// 响应格式：一行一个 CIDR/IP，空行和 # 开头的注释行被忽略。
func (r *TrustedProxyResolver) fetchSource(ctx context.Context, src TrustedProxyDynamicSource) ([]string, error) {
	timeout := time.Duration(src.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(TrustedProxyTimeoutSecondsDefault) * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, src.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "sub2api-trusted-proxy-resolver/1")
	req.Header.Set("Accept", "text/plain")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream status %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, TrustedProxySourceMaxResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if len(body) > TrustedProxySourceMaxResponseBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes", TrustedProxySourceMaxResponseBytes)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	// 允许较长 CIDR 行（IPv6 也就 40+ 字符，1KiB 足够）
	scanner.Buffer(make([]byte, 4096), 4096)

	seen := make(map[string]struct{}, 64)
	out := make([]string, 0, 32)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// 有的源可能一行多个（分号/逗号/空白分隔）——保守起见按空白分割
		for _, tok := range splitLine(line) {
			normalized := normalizeCIDROrIP(tok)
			if normalized == "" {
				continue
			}
			// 只接受合法 CIDR（normalize 已保证）
			if _, _, err := net.ParseCIDR(normalized); err != nil {
				continue
			}
			if _, dup := seen[normalized]; dup {
				continue
			}
			seen[normalized] = struct{}{}
			out = append(out, normalized)
			if len(out) > TrustedProxySourceMaxCIDRs {
				return nil, fmt.Errorf("too many CIDRs (> %d)", TrustedProxySourceMaxCIDRs)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan body: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid CIDR entries found in response body")
	}
	return out, nil
}

// splitLine 拆分一行文本为可能的 CIDR token。以任意空白 / 逗号 / 分号分隔。
func splitLine(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == ',' || r == ';'
	})
}

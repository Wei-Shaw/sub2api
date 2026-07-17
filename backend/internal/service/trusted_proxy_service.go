// Package service — trusted_proxy_service.go 提供 SettingService 上关于
// "可信代理动态拉取"三条 setting 的读取快捷方法，以及在 admin 保存 settings 后
// 触发 TrustedProxyResolver.Reconfigure 的回调注册点。
package service

import (
	"context"
	"sync/atomic"
)

// TrustedProxyReconfigureFn 是 admin 保存 settings 后通知 resolver 的回调签名。
// 由 infra 层（http.go 组装 resolver 时）通过 SetTrustedProxyReconfigure 注入。
type TrustedProxyReconfigureFn func(enabled bool, sources []TrustedProxyDynamicSource, extraCIDRs []string)

// trustedProxyReconfigureCB 全局回调（进程内单例）。
// 使用 atomic.Pointer 保证读写并发安全（Configure 由 infra 层调一次，SettingService
// 保存后触发时读取）。
var trustedProxyReconfigureCB atomic.Pointer[TrustedProxyReconfigureFn]

// SetTrustedProxyReconfigure 由 infra 层在启动时注入 resolver 的 Reconfigure 方法。
// admin 保存 settings 后 setting_update.go 会读取该回调并触发。
// 设为 nil 可禁用回调（主要用于测试）。
func SetTrustedProxyReconfigure(fn TrustedProxyReconfigureFn) {
	if fn == nil {
		trustedProxyReconfigureCB.Store(nil)
		return
	}
	trustedProxyReconfigureCB.Store(&fn)
}

// notifyTrustedProxyReconfigure 内部使用：settings 保存流程调用。
func notifyTrustedProxyReconfigure(enabled bool, sources []TrustedProxyDynamicSource, extraCIDRs []string) {
	p := trustedProxyReconfigureCB.Load()
	if p == nil || *p == nil {
		return
	}
	(*p)(enabled, sources, extraCIDRs)
}

// TrustedProxySnapshotProvider 是 admin GET/PUT 响应展示"只读快照"的接口——
// resolver 实现这个接口，infra 层用 SetTrustedProxySnapshotProvider 注册。
type TrustedProxySnapshotProvider interface {
	// StaticCIDRs 返回 config.yaml 里的静态 CIDR 列表（进程生命周期不变）。
	StaticCIDRs() []string
	// SourceStatuses 返回各 source 的运行时状态快照。
	SourceStatuses() []TrustedProxySourceStatus
}

var trustedProxySnapshotCB atomic.Pointer[TrustedProxySnapshotProvider]

// SetTrustedProxySnapshotProvider 由 infra 层在启动时注入 resolver。
func SetTrustedProxySnapshotProvider(p TrustedProxySnapshotProvider) {
	if p == nil {
		trustedProxySnapshotCB.Store(nil)
		return
	}
	trustedProxySnapshotCB.Store(&p)
}

// GetStaticTrustedProxies 返回静态 config.yaml CIDR 列表。未注入 → 空切片。
func GetStaticTrustedProxies() []string {
	p := trustedProxySnapshotCB.Load()
	if p == nil || *p == nil {
		return []string{}
	}
	return (*p).StaticCIDRs()
}

// GetTrustedProxySourceStatuses 返回所有 source 的状态快照。未注入 → 空切片。
func GetTrustedProxySourceStatuses() []TrustedProxySourceStatus {
	p := trustedProxySnapshotCB.Load()
	if p == nil || *p == nil {
		return []TrustedProxySourceStatus{}
	}
	return (*p).SourceStatuses()
}

// ─── SettingService getter ────────────────────────────────────────────────

// IsTrustedProxiesDynamicEnabled 读取总开关。库不可达 / 未设置 → 默认 false。
func (s *SettingService) IsTrustedProxiesDynamicEnabled(ctx context.Context) bool {
	v, err := s.settingRepo.Get(ctx, SettingKeyTrustedProxiesDynamicEnabled)
	if err != nil || v == nil {
		return false
	}
	return v.Value == "true"
}

// GetTrustedProxiesDynamicSources 读取 sources 列表。库不可达 → 内置默认候选。
func (s *SettingService) GetTrustedProxiesDynamicSources(ctx context.Context) []TrustedProxyDynamicSource {
	v, err := s.settingRepo.Get(ctx, SettingKeyTrustedProxiesDynamicSources)
	if err != nil || v == nil {
		return DefaultTrustedProxyDynamicSources()
	}
	return ParseTrustedProxyDynamicSources(v.Value)
}

// GetTrustedProxiesDynamicExtraCIDRs 读取 admin 固定 CIDR。库不可达 → 空切片。
func (s *SettingService) GetTrustedProxiesDynamicExtraCIDRs(ctx context.Context) []string {
	v, err := s.settingRepo.Get(ctx, SettingKeyTrustedProxiesDynamicExtraCIDRs)
	if err != nil || v == nil {
		return []string{}
	}
	return ParseTrustedProxyDynamicExtraCIDRs(v.Value)
}

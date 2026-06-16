package plugin

import (
	"context"
	"encoding/json"
	"fmt"
)

// =============================================================
// Admin handler 适配方法 + PluginInfo / PluginRecord 互转 helpers
//
// admin.PluginManager 接口期望短动词命名 (List / Get / Enable / ...),
// 而 PluginManager 实际方法叫 ListPlugins / GetPlugin / EnablePlugin 等。
// 这里的薄适配层避免上层因命名差异而依赖具体类型, 同时把 PluginInfo
// 与 PluginRecord 之间的字段互转 / 配置序列化等"无业务的样板代码"
// 与生命周期主流程隔离。
// =============================================================

// List 是 ListPlugins 的别名,实现 admin.PluginManager 接口。
func (m *PluginManager) List(ctx context.Context) ([]PluginInfo, error) {
	return m.ListPlugins(ctx)
}

// ListExt 是 ListPluginsExt 的别名, 实现 admin.PluginManager 接口的扩展列表
// 方法 — 用于 admin "已卸载" 视图按需把软卸载条目带回来。
func (m *PluginManager) ListExt(ctx context.Context, includeUninstalled bool) ([]PluginInfo, error) {
	return m.ListPluginsExt(ctx, includeUninstalled)
}

// Get 是 GetPlugin 的别名,实现 admin.PluginManager 接口。
func (m *PluginManager) Get(ctx context.Context, name string) (*PluginInfo, error) {
	return m.GetPlugin(ctx, name)
}

// Enable 是 EnablePlugin 的别名,实现 admin.PluginManager 接口。
func (m *PluginManager) Enable(ctx context.Context, name string) error {
	return m.EnablePlugin(ctx, name)
}

// Disable 是 DisablePlugin 的别名,实现 admin.PluginManager 接口。
func (m *PluginManager) Disable(ctx context.Context, name string) error {
	return m.DisablePlugin(ctx, name)
}

// Restart 是 RestartPlugin 的别名,实现 admin.PluginManager 接口。
func (m *PluginManager) Restart(ctx context.Context, name string) error {
	return m.RestartPlugin(ctx, name)
}

// UpdateConfig 持久化插件的配置 JSON。
//
// 当前实现只更新数据库,**不会**自动重启插件子进程,
// 是否需要重启由调用方根据具体配置项决定;插件可以在 Init 时拉取最新配置,
// 或通过自定义 RPC 在运行时热加载。
//
// 因为存储层使用 map[string]string,这里把 any 值统一序列化为字符串形式:
// 简单类型走 fmt.Sprintf("%v"),复合类型(map/slice)走 json.Marshal,
// 保证 round-trip 时配置语义不丢失。
func (m *PluginManager) UpdateConfig(ctx context.Context, name string, cfg map[string]any) error {
	if !IsValidPluginName(name) {
		return ErrInvalidPluginName
	}
	strCfg, err := configToString(cfg)
	if err != nil {
		return err
	}
	return m.repo.UpdateConfig(ctx, name, strCfg)
}

// =============================================================
// PluginInfo / PluginRecord 互转 + 配置序列化 helpers
// =============================================================

// mergeRecordIntoInfo 把数据库字段(Enabled/SortOrder/Description/Config 等)填入 info,
// 同时在运行时 manifest 缺失 DisplayName/Version 时回退到数据库记录。
func mergeRecordIntoInfo(info *PluginInfo, rec PluginRecord) {
	info.Enabled = rec.Enabled
	info.SortOrder = rec.SortOrder
	info.Description = rec.Description
	if info.DisplayName == "" {
		info.DisplayName = rec.DisplayName
	}
	if info.Version == "" {
		info.Version = rec.Version
	}
	info.Config = configToAny(rec.Config)
	if rec.UninstalledAt != nil {
		t := *rec.UninstalledAt
		info.UninstalledAt = &t
	}
}

// configToAny 把存储层的 map[string]string 转成 API 暴露的 map[string]any。
// 配置为空时返回 nil,避免 API 输出空对象。
func configToAny(in map[string]string) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// configToString 把 admin API 收到的 map[string]any 转成存储层的 map[string]string。
// 复合值序列化为 JSON 以便后续读取时还原。
func configToString(in map[string]any) (map[string]string, error) {
	out := make(map[string]string, len(in))
	for k, v := range in {
		switch val := v.(type) {
		case nil:
			out[k] = ""
		case string:
			out[k] = val
		case bool, int, int32, int64, float32, float64:
			out[k] = fmt.Sprintf("%v", val)
		default:
			b, err := json.Marshal(val)
			if err != nil {
				return nil, fmt.Errorf("marshal config key %q: %w", k, err)
			}
			out[k] = string(b)
		}
	}
	return out, nil
}

// copyConfig 深拷贝插件配置 map, 避免调用方修改 cfgCopy 影响 repo 缓存。
func copyConfig(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

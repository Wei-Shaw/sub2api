package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// =============================================================
// Frontend manifest aggregation
//
// 把所有正在运行插件的 manifest 中"前端可见的部分"
// (menu_items + routes + entry assets) 序列化成一个 JSON 数组,
// FrontendServer 注入到 window.__PLUGIN_MANIFESTS__。
// =============================================================

// GetPluginManifestsJSON 返回所有正在运行插件的前端清单(menu_items + routes),
// 用于 FrontendServer 注入 window.__PLUGIN_MANIFESTS__。
//
// 仅 StateRunning 且声明了 frontend manifest 的插件会被包含;
// 失败/未启动/无前端的插件直接跳过,避免前端拿到半成品配置。
//
// 输出始终是合法 JSON: 无可用插件时返回 "[]" 而非 nil,
// 让 FrontendServer 的注入逻辑无需特判长度。
func (m *PluginManager) GetPluginManifestsJSON() []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	manifests := make([]map[string]any, 0, len(m.plugins))
	for _, inst := range m.plugins {
		entry := buildManifestEntry(inst)
		if entry == nil {
			continue
		}
		manifests = append(manifests, entry)
	}
	if len(manifests) == 0 {
		return []byte("[]")
	}
	data, err := json.Marshal(manifests)
	if err != nil {
		// 这里 manifests 全部是基本类型,理论上不会序列化失败;
		// 退回空数组而非 panic,避免影响前端首屏。
		m.logger.Error("marshal plugin manifests failed", "error", err)
		return []byte("[]")
	}
	return data
}

// buildManifestEntry 在持有 m.mu 读锁的前提下构造单个插件的 manifest 项。
// 实例未运行或没有 manifest 时返回 nil。
func buildManifestEntry(inst *PluginInstance) map[string]any {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.State != StateRunning || inst.Manifest == nil {
		return nil
	}
	frontend := inst.Manifest.GetFrontend()
	entry := map[string]any{
		"name":         inst.Manifest.GetName(),
		"display_name": inst.Manifest.GetDisplayName(),
		"version":      inst.Manifest.GetVersion(),
		"description":  inst.Manifest.GetDescription(),
		"menu_items":   convertMenuItems(frontend.GetMenuItems()),
		"routes":       convertRoutes(frontend.GetRoutes()),
	}
	name := inst.Manifest.GetName()
	if entryJS := frontend.GetEntryJs(); entryJS != "" {
		entry["entry_js"] = entryJS
		// entry_js_url 是前端实际访问的 HTTP 路径; 走核心代理 -> gRPC GetFrontendBundle.
		// `?v=<hash>` 让浏览器在 plugin 重建后自动拉新 bundle, 不再受
		// pluginAssetCacheControl 的 max-age 阻塞 (见 prefetchEntryHashes).
		entry["entry_js_url"] = withVersionQuery(pluginAssetsURLPrefix+name+"/"+entryJS, inst.EntryJSHash)
	}
	if entryCSS := frontend.GetEntryCss(); entryCSS != "" {
		entry["entry_css"] = entryCSS
		entry["entry_css_url"] = withVersionQuery(pluginAssetsURLPrefix+name+"/"+entryCSS, inst.EntryCSSHash)
	}
	// settings_component_path: plugins that ship a custom settings UI surface
	// the component name here so PluginSettingsForm.vue can mount it instead
	// of the generic JSON-schema form. Empty string means fall back to the
	// schema renderer.
	if comp := inst.Manifest.GetSettingsComponentPath(); comp != "" {
		entry["settings_component_path"] = comp
	}
	return entry
}

// convertMenuItems 把 proto 菜单项递归转成可被前端直接消费的 map 列表。
// 保持空列表语义: 输入为空时返回空切片,前端可统一按数组处理。
//
// `icon_svg` 和 `labels` 是 SDK 新引入的字段(参见 plugin.proto),让插件
// 直接交付完整 SVG 与按 locale 分类的翻译文本,核心不再需要维护"图标名 ->
// SVG"或"label_key -> 翻译"的映射表。旧的 `icon` / `label_key` 仍透传作为
// 前端的 fallback,保留对早期插件的兼容性。
func convertMenuItems(items []*pluginsdk.MenuItem) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, mi := range items {
		if mi == nil {
			continue
		}
		entry := map[string]any{
			"path":                mi.GetPath(),
			"label_key":           mi.GetLabelKey(),
			"icon":                mi.GetIcon(),
			"icon_svg":            mi.GetIconSvg(),
			"section":             mi.GetSection(),
			"sort_order":          mi.GetSortOrder(),
			"requires_admin":      mi.GetRequiresAdmin(),
			"hide_in_simple_mode": mi.GetHideInSimpleMode(),
			"feature_flag":        mi.GetFeatureFlag(),
			// labels: copy into a fresh map so JSON serialization always emits
			// {} rather than null when the plugin supplied an empty map. nil
			// is fine here because encoding/json renders it as null and the
			// frontend treats null + empty as equivalent.
			"labels": mi.GetLabels(),
			// descriptions: per-locale description text the AppHeader renders
			// under the page title. Same null/empty semantics as labels —
			// frontend treats both as "no description".
			"descriptions": mi.GetDescriptions(),
			// V5/W7 Placement DSL — pass through verbatim so the frontend can
			// route the item into the right sidebar bucket. Empty group + zero
			// order is the legacy fallback (frontend lands the item at the
			// section's "<section>/end" bucket).
			"placement_group": mi.GetPlacementGroup(),
			"placement_order": mi.GetPlacementOrder(),
		}
		if children := convertMenuItems(mi.GetChildren()); len(children) > 0 {
			entry["children"] = children
		}
		out = append(out, entry)
	}
	return out
}

// convertRoutes 把 proto 路由定义转成前端可消费的 map 列表。
func convertRoutes(routes []*pluginsdk.RouteDefinition) []map[string]any {
	out := make([]map[string]any, 0, len(routes))
	for _, r := range routes {
		if r == nil {
			continue
		}
		out = append(out, map[string]any{
			"path":           r.GetPath(),
			"name":           r.GetName(),
			"component_path": r.GetComponentPath(),
			"meta":           r.GetMeta(),
		})
	}
	return out
}

// withVersionQuery returns rawURL with `?v=<hash>` appended when hash is
// non-empty, otherwise rawURL unchanged. Plugin entry URLs that lack a
// hash (prefetch failed, plugin has no frontend yet) fall back to the
// short Cache-Control window applied by the asset handler when no `v`
// query is present.
func withVersionQuery(rawURL, hash string) string {
	if hash == "" {
		return rawURL
	}
	return rawURL + "?v=" + hash
}

// entryAssetPrefetchTimeout caps a single entry.js / entry.css prefetch.
// Hashing 1 MB twice fits comfortably within 30s on the host's loopback
// gRPC; the timeout exists to bound a stuck plugin, not to be reached
// in the steady state.
const entryAssetPrefetchTimeout = 30 * time.Second

// prefetchEntryHashes pulls the plugin's entry.js and entry.css bytes
// once after StateRunning, computes a short content hash, and stamps
// EntryJSHash / EntryCSSHash on the instance so the next
// GetPluginManifestsJSON snapshot publishes URLs with `?v=<hash>`.
//
// Best-effort: a missing bundle, a timeout, or a plugin that has gone
// down by the time we get here all leave the corresponding hash empty.
// In that case the URL omits the version query and falls through to
// the shorter `pluginAssetCacheControl` lifetime — the page still
// works, browsers just hold the asset for fewer minutes than they
// would on the hashed path.
//
// Runs in its own goroutine off the spawn pipeline so the prefetch
// gRPC roundtrip never blocks lifecycle progression.
func (m *PluginManager) prefetchEntryHashes(inst *PluginInstance) {
	if inst == nil {
		return
	}
	inst.mu.Lock()
	manifest := inst.Manifest
	inst.mu.Unlock()
	if manifest == nil {
		return
	}
	frontend := manifest.GetFrontend()
	if frontend == nil {
		return
	}

	jsHash := m.fetchAndHash(inst.Name, frontend.GetEntryJs())
	cssHash := m.fetchAndHash(inst.Name, frontend.GetEntryCss())

	inst.mu.Lock()
	inst.EntryJSHash = jsHash
	inst.EntryCSSHash = cssHash
	inst.mu.Unlock()
}

// fetchAndHash resolves one entry asset and returns its short content
// hash, or empty when the plugin declares no asset / the fetch fails.
// The plugin-not-running case is treated as a soft miss (warn-level
// log) — the manager will recompute on the next spawnAndConnect.
func (m *PluginManager) fetchAndHash(pluginName, assetPath string) string {
	if assetPath == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), entryAssetPrefetchTimeout)
	defer cancel()
	asset, err := m.FetchFrontendAsset(ctx, pluginName, assetPath)
	if err != nil {
		level := m.logger.Warn
		if errors.Is(err, ErrPluginNotRunning) {
			level = m.logger.Debug
		}
		level("plugin entry asset prefetch failed",
			"plugin", pluginName, "asset", assetPath, "error", err)
		return ""
	}
	return computeShortHash(asset.Data)
}

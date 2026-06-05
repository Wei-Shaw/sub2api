package plugin

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// =============================================================
// Spawn pipeline — capability-side stages
//
// 这一组 stage 负责"已建立 gRPC 连接 + 拿到 manifest"之后,把插件
// 声明的 capability 注册到 host 侧的多个授权 / 路由表上:
//
//   - approveCapabilities → SDKServer.RegisterPlugin (authn map)
//   - tables              → SDKServer.capabilities.SetTables (SQL gate)
//   - events              → eventsAllowList.Set (events allow-list)
//   - settings            → settingsService.RegisterSchema + Subscribe
//
// 与 manager_spawn.go 的早期 stage(pipe / handshake / dial / manifest)
// 解耦,新增一种 capability 注册不需要碰主流程文件。每个 stage 仍然走
// spawnStages slice 里的 do/rollback 协议,失败时由 spawnAndConnect 主
// 驱动倒序回滚。
// =============================================================

// -------------------------------------------------------------
// Stage: capabilities — 计算授权 capability 集 + 注册到 SDK
// -------------------------------------------------------------

func (m *PluginManager) spawnApproveCapabilities(sc *spawnCtx) error {
	sc.approved = approveCapabilities(sc.inst.Name, sc.manifest.GetCapabilities(), m.logger)
	// 在 SDKServer 上注册插件 → 授权 capabilities 的映射,后续 Redis Do 会查询。
	m.sdkServer.RegisterPlugin(sc.inst.Name, sc.approved)
	return nil
}

func (m *PluginManager) spawnRollbackCapabilities(sc *spawnCtx) {
	m.sdkServer.UnregisterPlugin(sc.inst.Name)
}

// -------------------------------------------------------------
// Stage: tables — 注册 owned_tables 给 SQL gate
// -------------------------------------------------------------

func (m *PluginManager) spawnSetTables(sc *spawnCtx) error {
	// P12·B-1 SQL gate: persist the manifest's owned_tables so SDKServer's
	// SQLProxy can authorise plugin SQL against the per-plugin allow-list.
	m.sdkServer.capabilities.SetTables(sc.inst.Name, sc.manifest.GetOwnedTables())
	return nil
}

// -------------------------------------------------------------
// Stage: events — 注册 SubscribedEvents 到 events allow-list
// -------------------------------------------------------------

func (m *PluginManager) spawnEventsCapabilities(sc *spawnCtx) error {
	// Phase B: register the plugin manifest.SubscribedEvents so the
	// EventsExtension server can validate Subscribe requests against
	// what the plugin actually declared interest in.
	if m.eventsAllowList != nil {
		m.eventsAllowList.Set(sc.inst.Name, sc.manifest.GetSubscribedEvents())
	}
	return nil
}

func (m *PluginManager) spawnRollbackEvents(sc *spawnCtx) {
	if m.eventsAllowList != nil {
		m.eventsAllowList.Forget(sc.inst.Name)
	}
}

// -------------------------------------------------------------
// Stage: settings — 注册 schema + 订阅变更事件
// -------------------------------------------------------------

func (m *PluginManager) spawnSettingsCapabilities(sc *spawnCtx) error {
	if m.settingsService == nil || len(sc.manifest.GetSettingsSchemaJson()) == 0 {
		return nil
	}
	regCtx, cancelReg := context.WithTimeout(sc.parentCtx, m.cfg.ManifestTimeout)
	// V5/W6 SETTINGS-V2 (DESIGN §4.1): pass the full manifest envelope
	// — schema_version + properties_meta_json — so the service can
	// (1) detect schema_version bumps and drop existing watchers,
	// (2) prefer the SDK-authoritative properties_meta over re-deriving
	// it from x-* vendor extensions inside the schema bytes.
	err := m.settingsService.RegisterSchemaWithInput(regCtx, service.RegisterSchemaInput{
		PluginName:         sc.inst.Name,
		SchemaJSON:         sc.manifest.GetSettingsSchemaJson(),
		DefaultsJSON:       sc.manifest.GetSettingsDefaultsJson(),
		SchemaVersion:      sc.manifest.GetSettingsSchemaVersion(),
		PropertiesMetaJSON: sc.manifest.GetSettingsPropertiesMetaJson(),
	})
	cancelReg()
	if err != nil {
		return fmt.Errorf("plugin settings register schema: %w", err)
	}

	// V5/W6 SETTINGS-V2 (DESIGN §4.4): subscribe to value-change events
	// so RequiresReload-flagged updates can trigger a coalesced reload.
	// Empty key = whole namespace. The unsubscribe is captured on the
	// instance and invoked from stopInstance — see DESIGN §4.4 cleanup.
	ch, unsubscribe := m.settingsService.Subscribe(sc.inst.Name, "")
	sc.inst.mu.Lock()
	sc.inst.settingsUnsubscribe = unsubscribe
	sc.inst.mu.Unlock()
	go m.handlePluginSettingsEvents(sc.inst.Name, ch)

	sc.settingsRegistered = true
	return nil
}

func (m *PluginManager) spawnRollbackSettings(sc *spawnCtx) {
	if !sc.settingsRegistered {
		return
	}
	sc.inst.mu.Lock()
	unsub := sc.inst.settingsUnsubscribe
	sc.inst.settingsUnsubscribe = nil
	sc.inst.mu.Unlock()
	if unsub != nil {
		unsub()
	}
	if m.settingsService != nil {
		m.settingsService.UnregisterSchema(sc.inst.Name)
	}
}

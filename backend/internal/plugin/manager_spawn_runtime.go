package plugin

import (
	"context"
	"fmt"

	sdkroot "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// =============================================================
// Spawn pipeline — runtime / installation stages
//
// 这一组 stage 在 capability 层注册完成 (manager_spawn_capabilities.go)
// 之后跑,负责把插件真正"装"到 host:
//
//   - init        → 通知插件 SDK 地址 + 已批准的 capability 列表
//   - migrations  → 跑插件声明的 SQL 迁移 (V5/W1)
//   - route       → 校验路由声明 + 写入路由表
//   - health      → 写回 instance state + 启动 HealthMonitor
//   - pricing     → 接线 PricingExtension (best-effort)
//
// 这些 stage 与 manager_spawn_capabilities.go 是同一 spawn pipeline 的
// 后半段,共享 spawnCtx,通过 manager_spawn.go::spawnStages() 串成完整
// 顺序。新增"运行期挂载"步骤时只动这个文件。
// =============================================================

// -------------------------------------------------------------
// Stage: init — 调用 PluginLifecycle.Init 把 SDK 地址 + 授权下发
// -------------------------------------------------------------

func (m *PluginManager) spawnInit(sc *spawnCtx) error {
	// 调用 Init 把 SDK 地址、plugin_name 与已批准的 capabilities 传给子进程。
	// V5/W6 SETTINGS-V2: PluginInitRequest.config (proto field 2) is
	// reserved — pluginConfig stays host-side only (e.g. skip_migration);
	// it is no longer wired into the plugin process. See
	// SETTINGS-V2-DESIGN §3 (decision 1).
	initCtx, cancelInit := context.WithTimeout(sc.parentCtx, m.cfg.ManifestTimeout)
	initResp, initErr := sc.lifecycle.Init(initCtx, &pluginsdk.PluginInitRequest{
		SdkAddress:       m.sdkAddr,
		PluginName:       sc.inst.Name,
		Capabilities:     expandWithLegacyAliases(sc.approved),
		OutboundDefaults: m.buildOutboundDefaults(),
	})
	cancelInit()
	if initErr != nil {
		return fmt.Errorf("plugin init rpc: %w", initErr)
	}
	if initResp != nil && !initResp.Success {
		return fmt.Errorf("plugin init reported failure: %s", initResp.Error)
	}
	return nil
}

// -------------------------------------------------------------
// Stage: migrations — 跑插件迁移 (V5/W1)
// -------------------------------------------------------------

func (m *PluginManager) spawnMigrations(sc *spawnCtx) error {
	// 跑插件迁移 (V5/W1)。manifest.migrations 是新版插件声明的 SQL 文件列表,
	// 每条带有 sha256 pin。host 通过 PluginLifecycle.GetMigration 拉取 SQL body,
	// 重新校验 checksum 后调用 RunPluginMigrations(advisory lock + 记录 plugin_migrations)。
	//
	// 失败语义 (V5-CURATE Q3): 默认阻塞插件启动 — 返回 error 让 EnablePlugin 失败。
	// escape hatch: PluginRecord.Config["skip_migration"]="true" 时跳过执行。
	if shouldSkipPluginMigrations(sc.pluginConfig) {
		m.logger.Warn("plugin migrations skipped via skip_migration config",
			"plugin", sc.inst.Name,
			"declared_count", len(sc.manifest.GetMigrations()),
		)
		return nil
	}
	if len(sc.manifest.GetMigrations()) > 0 {
		if err := fetchAndRunPluginMigrations(sc.parentCtx, m.db, sc.lifecycle, sc.manifest, sc.inst.Name, m.logger); err != nil {
			return fmt.Errorf("plugin migrations: %w", err)
		}
		return nil
	}
	if len(sc.manifest.GetMigrationFiles()) > 0 {
		// 兼容旧插件二进制:没有 sha256 pin,host 无法拉取 SQL,只记录日志。
		m.logger.Info("plugin declared legacy migration_files (no sha256 pin, skipped)",
			"plugin", sc.inst.Name,
			"count", len(sc.manifest.GetMigrationFiles()),
		)
	}
	return nil
}

// -------------------------------------------------------------
// Stage: route_validate / route_install — manifest 路由校验 + 注入路由表
// -------------------------------------------------------------

func (m *PluginManager) spawnValidateRoutes(sc *spawnCtx) error {
	// P12·B-1 HTTP route namespace gate: reject manifests that try to claim
	// host-reserved /api/v1/admin/* or gateway /v1/* without the
	// http.register.gateway capability.
	if err := validatePluginRoutePaths(sc.inst.Name, sc.manifest, sc.approved); err != nil {
		return fmt.Errorf("plugin manifest route validation: %w", err)
	}
	return nil
}

func (m *PluginManager) spawnInstallRoutes(sc *spawnCtx) error {
	entries := buildRouteEntries(sc.inst.Name, sc.manifest, sc.hs.HTTPAddr)
	newTable := m.router.CurrentTable().AddPlugin(sc.inst.Name, entries)
	m.router.SwapRouteTable(newTable)
	return nil
}

func (m *PluginManager) spawnRollbackRoutes(sc *spawnCtx) {
	newTable := m.router.CurrentTable().RemovePlugin(sc.inst.Name)
	m.router.SwapRouteTable(newTable)
}

// -------------------------------------------------------------
// Stage: health — 把所有就绪状态写回 inst, 启动 health monitor + waitProcessExit
// -------------------------------------------------------------

func (m *PluginManager) spawnStartHealth(sc *spawnCtx) error {
	healthCtx, healthCancel := context.WithCancel(context.Background())
	monitor := NewHealthMonitor(m.cfg.HealthInterval, m.cfg.HealthFailThreshold)

	if err := m.attachInstanceState(sc, healthCancel); err != nil {
		healthCancel()
		return err
	}

	go monitor.Monitor(healthCtx, sc.inst, func() {
		m.handleUnhealthy(sc.inst)
	})
	go m.waitProcessExit(sc.inst)
	// Prefetch entry.js / entry.css content hashes off the spawn pipeline.
	// The result populates EntryJSHash / EntryCSSHash so the next manifest
	// snapshot publishes URLs with `?v=<hash>`, letting browsers refetch
	// after a redeploy without sitting on a stale 5-minute disk cache.
	go m.prefetchEntryHashes(sc.inst)
	sc.healthCancel = healthCancel
	return nil
}

// -------------------------------------------------------------
// Stage: pricing — 接线 PricingExtension (best-effort)
// -------------------------------------------------------------

func (m *PluginManager) spawnTryPricing(sc *spawnCtx) error {
	// PricingExtension wiring: probe the plugin lazily — if it does not
	// register the service the bootstrap List returns codes.Unimplemented
	// and the client logs a debug-level message. The wire is best-effort:
	// any failure here leaves the plugin running normally, just without
	// pricing overrides feeding into BillingService.
	m.tryStartPricingExtension(sc.parentCtx, sc.inst)
	return nil
}


// -------------------------------------------------------------
// Stage: platforms — register platform declarations from manifest
// -------------------------------------------------------------

func (m *PluginManager) spawnRegisterPlatforms(sc *spawnCtx) error {
	pbPlatforms := sc.manifest.GetPlatforms()
	if len(pbPlatforms) == 0 {
		return nil
	}
	for _, pbDecl := range pbPlatforms {
		decl := sdkroot.PlatformDeclFromProto(pbDecl)
		if !m.platformRegistry.Register(sc.inst.Name, decl, sc.conn) {
			m.logger.Warn("platform already registered, skipping",
				"plugin", sc.inst.Name,
				"platform", decl.Platform,
			)
		} else {
			m.logger.Info("platform registered",
				"plugin", sc.inst.Name,
				"platform", decl.Platform,
				"account_types", len(decl.AccountTypes),
			)
			// Register a gateway provider so the pipeline can forward
			// requests to this plugin's platform.
			m.registerGatewayProvider(sc, decl.Platform, decl.CompatibleGateways)
		}
	}
	m.invalidateFrontendCache()
	return nil
}

// registerGatewayProvider creates and registers a PluginGatewayProvider for
// the given platform. The protocols default to [platform] (same convention
// as the host-internal adapters). No-op if SetGatewayProviderRegistry was
// not called.
func (m *PluginManager) registerGatewayProvider(sc *spawnCtx, platform string, compatibleGateways []string) {
	m.mu.RLock()
	registry := m.gatewayProviderRegistry
	m.mu.RUnlock()
	if registry == nil {
		return
	}
	// Use the plugin's declared compatible gateways if provided;
	// otherwise fall back to the platform name itself.
	protocols := compatibleGateways
	if len(protocols) == 0 {
		protocols = []string{platform}
	}
	registry.RegisterPlugin(platform, protocols, sc.inst.Name, sc.conn)
	m.logger.Info("gateway provider registered",
		"plugin", sc.inst.Name,
		"platform", platform,
	)
}

func (m *PluginManager) spawnRollbackPlatforms(sc *spawnCtx) {
	m.unregisterGatewayProviders(sc.inst.Name, sc.manifest)
	m.platformRegistry.Unregister(sc.inst.Name)
	m.invalidateFrontendCache()
}

// unregisterGatewayProviders removes all gateway providers registered by
// the named plugin. Called during rollback and stop.
func (m *PluginManager) unregisterGatewayProviders(pluginName string, manifest *pluginsdk.ManifestResponse) {
	m.mu.RLock()
	registry := m.gatewayProviderRegistry
	m.mu.RUnlock()
	if registry == nil {
		return
	}
	if manifest == nil {
		return
	}
	for _, pbDecl := range manifest.GetPlatforms() {
		registry.UnregisterPlugin(pbDecl.GetPlatform())
		m.logger.Info("gateway provider unregistered",
			"plugin", pluginName,
			"platform", pbDecl.GetPlatform(),
		)
	}
}
// -------------------------------------------------------------
// Stage: maintenance — attach MaintenanceExtension client (best-effort)
// -------------------------------------------------------------

func (m *PluginManager) spawnTryMaintenance(sc *spawnCtx) error {
	// MaintenanceExtension wiring: unconditionally create the client for
	// every plugin. No probe RPC is issued — codes.Unimplemented is
	// handled at call-time in RunPluginMaintenance, keeping spawn cheap.
	m.tryStartMaintenanceExtension(sc.parentCtx, sc.inst)
	return nil
}

func (m *PluginManager) spawnTryContentIntercept(sc *spawnCtx) error {
	m.tryStartContentInterceptExtension(sc.inst)
	return nil
}

package plugin

// =============================================================
// In-memory plugin instance map create
//
// PluginManager 的 m.plugins map 由 manager.go 的三件套 (deleteInstance /
// lookupInstance / snapshotPluginNames) 负责"删 / 查 / 枚举", 而本文件
// 只放"惰性创建"逻辑 — getOrCreateInstance。这样写主要是因为创建一个
// PluginInstance 需要 RestartPolicy 配置 (依赖 m.cfg.Restart), 比纯粹
// 的 map 操作多出"构造默认值"的职责; 把它独立成一个文件能保留 manager.go
// 对 map 操作的极简语义, 同时让 spawn / install pipeline 在调用时只看到
// "若不存在就创建" 这一稳定动词。
//
// 注意: 该 helper 仍然在 m.mu 这把全局写锁下工作, 与 manager.go 三件套
// 共享同一把锁, 因此无需额外的并发原语。
// =============================================================

// getOrCreateInstance 在持有 m.mu 的前提下查找或创建 PluginInstance。
func (m *PluginManager) getOrCreateInstance(name, binPath string) *PluginInstance {
	m.mu.Lock()
	defer m.mu.Unlock()
	if inst, ok := m.plugins[name]; ok {
		return inst
	}
	inst := NewPluginInstance(name, binPath, NewRestartPolicy(m.cfg.Restart))
	m.plugins[name] = inst
	return inst
}

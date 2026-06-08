package plugin

import "context"

// =============================================================
// 查询 API (T27 拆分)
//
// 仅包含只读查询入口 (ListPlugins / ListPluginsExt / GetPlugin), 不涉及
// 任何状态变更。与 manager_lifecycle_api.go (变更类) 分离, 让 admin handler
// 在调用栈层面就能区分 "看" 和 "改"。
//
// 内部依赖 m.lookupInstance / m.isBuiltinPlugin / mergeRecordIntoInfo —
// 均定义在 manager.go / manager_install.go / manager_admin.go。
// =============================================================

// ListPlugins 返回所有活动插件的运行时信息 (软卸载的隐藏)。
// 等价于 ListPluginsExt(ctx, false)。
func (m *PluginManager) ListPlugins(ctx context.Context) ([]PluginInfo, error) {
	return m.ListPluginsExt(ctx, false)
}

// ListPluginsExt 返回插件列表; includeUninstalled=true 时把软卸载的插件也带上,
// 用于 admin "已卸载" 视图。每个软卸载条目的 PluginInfo.UninstalledAt 不为 nil,
// 调用方据此决定是否高亮 / 提供 "恢复" 按钮。
func (m *PluginManager) ListPluginsExt(ctx context.Context, includeUninstalled bool) ([]PluginInfo, error) {
	var (
		records []PluginRecord
		err     error
	)
	if includeUninstalled {
		records, err = m.repo.ListAll(ctx)
	} else {
		records, err = m.repo.List(ctx)
	}
	if err != nil {
		return nil, err
	}

	out := make([]PluginInfo, 0, len(records))
	for _, rec := range records {
		inst, ok := m.lookupInstance(rec.Name)

		var info PluginInfo
		// 软卸载行: 进程必然已停, 不应该把可能残留的 instance 状态当成"运行中".
		if ok && rec.UninstalledAt == nil {
			info = inst.SnapshotInfo()
		} else {
			info = PluginInfo{
				Name:  rec.Name,
				State: StateRegistered.String(),
			}
		}
		mergeRecordIntoInfo(&info, rec)
		info.Builtin = m.isBuiltinPlugin(rec.Name)
		out = append(out, info)
	}
	return out, nil
}

// GetPlugin 返回单个插件的信息。
func (m *PluginManager) GetPlugin(ctx context.Context, name string) (*PluginInfo, error) {
	rec, err := m.repo.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	inst, ok := m.lookupInstance(name)

	var info PluginInfo
	if ok {
		info = inst.SnapshotInfo()
	} else {
		info = PluginInfo{
			Name:  rec.Name,
			State: StateRegistered.String(),
		}
	}
	mergeRecordIntoInfo(&info, *rec)
	info.Builtin = m.isBuiltinPlugin(rec.Name)
	return &info, nil
}

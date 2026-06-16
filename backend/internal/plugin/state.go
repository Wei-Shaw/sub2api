package plugin

import "fmt"

// PluginState 表示插件实例当前所处的生命周期状态。
//
// P13/C-1 高层状态机 (从 admin / DB 视角):
//
//	absent       — DB 中无记录, manager 中无 instance
//	disabled     — DB 行存在, enabled=false, uninstalled_at IS NULL
//	enabled      — DB 行存在, enabled=true,  uninstalled_at IS NULL
//	                进程通常 Running, 也可能在 Starting / Restarting / Errored
//	uninstalled  — DB 行存在, uninstalled_at IS NOT NULL
//	                进程已停, 数据保留, 可通过 Install 撤回; 不出现在 List / sidebar
//
// 这里的 PluginState 仅描述运行时进程状态 (Registered/Starting/Running/Errored/
// Restarting); enabled / uninstalled_at 由 PluginRepository 持久化, 通过 PluginInfo
// 一起暴露给上层。运行时状态与 DB 状态正交: 例如 enabled=true 但进程刚崩溃时
// State=Errored, 或 uninstalled_at 非空但 instance 还没来得及清理时 State 仍可能
// 是 Running 的瞬间窗口 (Uninstall 内部串行化保证窗口对外不可见)。
type PluginState int

const (
	// StateRegistered 插件已注册到数据库,但尚未运行。
	StateRegistered PluginState = iota
	// StateStarting 插件子进程正在启动中。
	StateStarting
	// StateRunning 插件运行正常,健康检查通过,路由已激活。
	StateRunning
	// StateErrored 插件出现错误(进程崩溃或健康检查失败),等待重启决策。
	StateErrored
	// StateRestarting 插件正在按重启策略自动重启。
	StateRestarting
)

// String 返回插件状态的可读名称,主要用于日志输出。
func (s PluginState) String() string {
	switch s {
	case StateRegistered:
		return "registered"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateErrored:
		return "errored"
	case StateRestarting:
		return "restarting"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// validTransitions 定义了插件状态机允许的转换路径。
// key 是源状态,value 是允许转换到的目标状态列表。
var validTransitions = map[PluginState][]PluginState{
	StateRegistered: {StateStarting},
	StateStarting:   {StateRunning, StateErrored, StateRegistered},
	StateRunning:    {StateErrored, StateRegistered, StateRestarting},
	StateErrored:    {StateRestarting, StateRegistered, StateStarting},
	StateRestarting: {StateStarting, StateErrored, StateRegistered},
}

// CanTransitionTo 判断当前状态是否允许转换到目标状态。
func (s PluginState) CanTransitionTo(target PluginState) bool {
	allowed, ok := validTransitions[s]
	if !ok {
		return false
	}
	for _, st := range allowed {
		if st == target {
			return true
		}
	}
	return false
}

package plugin

import "fmt"

// PluginState 表示插件实例当前所处的生命周期状态。
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

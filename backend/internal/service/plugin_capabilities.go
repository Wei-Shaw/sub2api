package service

import (
	"sync/atomic"
	"time"
)

var pluginRoutingCapabilities = [...]string{
	PluginCapabilityOpenAIOAuthOutbound,
	PluginCapabilityAnthropicOAuthPatch,
}

func pluginCapabilityPlatform(capability string) (string, bool) {
	switch capability {
	case PluginCapabilityOpenAIOAuthOutbound:
		return PlatformOpenAI, true
	case PluginCapabilityAnthropicOAuthPatch:
		return PlatformAnthropic, true
	default:
		return "", false
	}
}

func hasEnabledCapabilityBinding(bindings []PluginBinding, capability string) bool {
	platform, supported := pluginCapabilityPlatform(capability)
	if !supported {
		return false
	}
	for _, binding := range bindings {
		if binding.Enabled && binding.Capability == capability &&
			binding.Platform == platform && binding.AccountType == AccountTypeOAuth {
			return true
		}
	}
	return false
}

func hasEnabledPluginBinding(bindings []PluginBinding) bool {
	for _, capability := range pluginRoutingCapabilities {
		if hasEnabledCapabilityBinding(bindings, capability) {
			return true
		}
	}
	return false
}

func capabilityBindingRollout(bindings []PluginBinding, capability string) int {
	for _, binding := range bindings {
		if binding.Capability == capability {
			return binding.RolloutPercent
		}
	}
	return 100
}

func (m *PluginManager) capabilityRoute(capability string) *atomic.Pointer[pluginRoute] {
	switch capability {
	case PluginCapabilityOpenAIOAuthOutbound:
		return &m.route
	case PluginCapabilityAnthropicOAuthPatch:
		return &m.anthropicRoute
	default:
		panic("unsupported plugin routing capability")
	}
}

func (m *PluginManager) routeForPlugin(id int64) *pluginRoute {
	for _, capability := range pluginRoutingCapabilities {
		if route := m.capabilityRoute(capability).Load(); route != nil && route.pluginID == id {
			return route
		}
	}
	return nil
}

// 两种能力可以共享同一个进程，只有最后一个路由解除引用后才回收它。
// 调用方持有 m.mu，并在解锁后等待已有请求结束。
func (m *PluginManager) replaceCapabilityRouteLocked(capability string, next *pluginRoute) *pluginRuntime {
	previous := m.capabilityRoute(capability).Swap(next)
	if previous == nil || previous.runtime == nil || (next != nil && previous.runtime == next.runtime) {
		return nil
	}
	for _, candidate := range pluginRoutingCapabilities {
		if route := m.capabilityRoute(candidate).Load(); route != nil && route.runtime == previous.runtime {
			return nil
		}
	}
	if m.runtimes[previous.pluginID] == previous.runtime {
		delete(m.runtimes, previous.pluginID)
	}
	previous.runtime.draining.Store(true)
	return previous.runtime
}

func (m *PluginManager) detachCapabilityRoute(capability string) {
	m.mu.Lock()
	stale := m.replaceCapabilityRouteLocked(capability, nil)
	m.mu.Unlock()
	if stale != nil {
		stale.drain(10 * time.Second)
	}
}

func (m *PluginManager) publishUnavailableCapability(capability string, pluginID int64, rollout int, message string) {
	m.mu.Lock()
	stale := m.replaceCapabilityRouteLocked(capability, &pluginRoute{
		pluginID: pluginID, rolloutPercent: rollout, unavailable: message,
	})
	m.mu.Unlock()
	if stale != nil {
		stale.drain(10 * time.Second)
	}
}

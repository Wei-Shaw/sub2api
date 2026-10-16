package domain

import "time"

// CodexModelCapabilities 记录单个上游模型 slug 在 Codex 模型目录中广告的能力：
// service_tiers 的 id 列表与 available_access_programs.cyber 列表（含 standard）。
type CodexModelCapabilities struct {
	ServiceTiers  []string `json:"service_tiers,omitempty"`
	CyberPrograms []string `json:"cyber_programs,omitempty"`
}

// CodexAccountCapabilities 是按账号持久化的 Codex 能力记录（accounts.extra["codex_capabilities"]）。
// Models 按上游 slug 保存，供 manifest 投影使用；ServiceTiers / CyberPrograms 是派生的账号级
// 并集，供调度过滤使用。账号级 CyberPrograms 不含 standard：standard 表示「没有项目」。
type CodexAccountCapabilities struct {
	Models        map[string]CodexModelCapabilities `json:"models"`
	ServiceTiers  []string                          `json:"service_tiers"`
	CyberPrograms []string                          `json:"cyber_programs"`
	SyncedAt      time.Time                         `json:"synced_at"`
}

// SupportsServiceTier 报告账号任一模型是否广告了该 service tier。
func (c *CodexAccountCapabilities) SupportsServiceTier(tier string) bool {
	return c != nil && containsString(c.ServiceTiers, tier)
}

// SupportsCyberProgram 报告账号任一模型是否广告了该 Daybreak 等级。
func (c *CodexAccountCapabilities) SupportsCyberProgram(program string) bool {
	return c != nil && containsString(c.CyberPrograms, program)
}

// EquivalentTo 在模型级与账号级能力上比较两条记录，忽略同步时间。
func (c *CodexAccountCapabilities) EquivalentTo(other *CodexAccountCapabilities) bool {
	if c == nil || other == nil {
		return c == nil && other == nil
	}
	if !equalStrings(c.ServiceTiers, other.ServiceTiers) || !equalStrings(c.CyberPrograms, other.CyberPrograms) {
		return false
	}
	if len(c.Models) != len(other.Models) {
		return false
	}
	for slug, caps := range c.Models {
		otherCaps, ok := other.Models[slug]
		if !ok {
			return false
		}
		if !equalStrings(caps.ServiceTiers, otherCaps.ServiceTiers) || !equalStrings(caps.CyberPrograms, otherCaps.CyberPrograms) {
			return false
		}
	}
	return true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

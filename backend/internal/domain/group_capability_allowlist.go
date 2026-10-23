package domain

// GroupCapabilityAllowlist 是分组级 Codex 能力白名单：开关开启时只允许集合内的
// Daybreak 等级（daybreak_blue / daybreak_red）与 service tier（priority / ultrafast）。
// standard 作为基线始终允许，不出现在配置中。关闭时允许全部能力。
type GroupCapabilityAllowlist struct {
	Enabled       bool     `json:"enabled"`
	CyberPrograms []string `json:"cyber_programs,omitempty"`
	ServiceTiers  []string `json:"service_tiers,omitempty"`
}

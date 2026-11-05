package domain

// GroupClientExportConfig stores administrator-selected native client defaults.
// Empty strings preserve native behavior; enum strings distinguish an explicit
// disabled choice from an unset preference without changing existing groups.
type GroupClientExportConfig struct {
	CodexModel           string `json:"codex_model,omitempty"`
	GeminiModel          string `json:"gemini_model,omitempty"`
	GrokModel            string `json:"grok_model,omitempty"`
	CodexReasoningEffort string `json:"codex_reasoning_effort,omitempty"`
	CodexAuthMode        string `json:"codex_auth_mode,omitempty"`
	CodexCatalogMode     string `json:"codex_catalog_mode,omitempty"`
	ClaudeTraffic        string `json:"claude_traffic,omitempty"`
	CCUsageMode          string `json:"cc_usage_mode,omitempty"`
	CCUsageInterval      string `json:"cc_usage_interval,omitempty"`
}

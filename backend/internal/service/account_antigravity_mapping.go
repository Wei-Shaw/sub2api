package service

import (
	"slices"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// antigravityDefaultPassthroughModels records legacy injected entries for the
// capability-sync compatibility filter. Explicit mappings no longer add them.
var antigravityDefaultPassthroughModels = []string{
	"gemini-3-flash",
	"gemini-3.1-pro-high",
	"gemini-3.1-pro-low",
	"gemini-3.6-flash",
	"gemini-3.6-flash-high",
	"gemini-3.6-flash-low",
	"gemini-3.6-flash-medium",
	"gemini-3.6-flash-tiered",
	"gemini-3.7-flash",
	"gemini-3.7-flash-high",
	"gemini-3.7-flash-low",
	"gemini-3.7-flash-medium",
	"gemini-3.7-flash-tiered",
	"gemini-3.8-flash",
	"gemini-3.8-flash-high",
	"gemini-3.8-flash-low",
	"gemini-3.8-flash-medium",
	"gemini-3.8-flash-tiered",
}

// isAntigravityInjectedDefaultPassthrough identifies legacy synthetic entries
// without treating an administrator's explicit identity mapping as synthetic.
func (a *Account) isAntigravityInjectedDefaultPassthrough(key, target string) bool {
	if a == nil || a.Platform != domain.PlatformAntigravity || key != target || !slices.Contains(antigravityDefaultPassthroughModels, key) {
		return false
	}
	rawMapping, _ := a.Credentials["model_mapping"].(map[string]any)
	hasExplicitEntry := false
	for _, v := range rawMapping {
		if _, ok := v.(string); ok {
			hasExplicitEntry = true
			break
		}
	}
	if !hasExplicitEntry {
		// 空映射走默认目录，不做运行时补齐。
		return false
	}
	_, configured := rawMapping[key].(string)
	return !configured
}

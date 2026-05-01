package service

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

type AccountModelFilterEntry struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type AccountModelFilterGroup struct {
	Platform string                    `json:"platform"`
	Label    string                    `json:"label"`
	Models   []AccountModelFilterEntry `json:"models"`
}

const (
	AccountModelFilterLimited                      = "__limited__"
	AccountModelFilterUnlimited                    = "__unlimited__"
	AccountStatusFilterActiveExcludingQuotaStopped = "active_excluding_quota_stopped"
	AccountStatusFilterOpenAI5HUsedZero            = "openai_5h_used_zero"
	AccountStatusFilterOpenAI7DUsedZero            = "openai_7d_used_zero"
)

func ListAccountModelFilterGroups() []AccountModelFilterGroup {
	return []AccountModelFilterGroup{
		{
			Platform: PlatformOpenAI,
			Label:    "OpenAI",
			Models:   buildOpenAIModelFilterEntries(),
		},
		{
			Platform: PlatformAnthropic,
			Label:    "Anthropic",
			Models:   buildClaudeModelFilterEntries(),
		},
		{
			Platform: PlatformGemini,
			Label:    "Gemini",
			Models:   buildGeminiModelFilterEntries(),
		},
		{
			Platform: PlatformAntigravity,
			Label:    "Antigravity",
			Models:   buildAntigravityModelFilterEntries(),
		},
	}
}

func FilterAccountModelGroupsByPlatform(groups []AccountModelFilterGroup, platform string) []AccountModelFilterGroup {
	normalizedPlatform := strings.TrimSpace(platform)
	if normalizedPlatform == "" {
		return groups
	}

	filtered := make([]AccountModelFilterGroup, 0, 1)
	for _, group := range groups {
		if group.Platform == normalizedPlatform {
			filtered = append(filtered, group)
			break
		}
	}
	return filtered
}

func IsAccountSupportedForModelFilter(account *Account, requestedModel string) bool {
	if account == nil {
		return false
	}

	trimmed := strings.TrimSpace(requestedModel)
	if trimmed == "" {
		return false
	}

	switch trimmed {
	case AccountModelFilterLimited:
		return hasExplicitModelRestriction(account)
	case AccountModelFilterUnlimited:
		return !hasExplicitModelRestriction(account)
	}

	mapping := account.GetModelMapping()
	if len(mapping) == 0 {
		return true
	}

	if account.IsAnthropicOAuthOrSetupToken() {
		for _, alias := range buildAnthropicModelAliases(trimmed) {
			if mappingSupportsRequestedModel(mapping, alias) {
				return true
			}
		}
		if mappingSupportsAnthropicModelAlias(mapping, trimmed) {
			return true
		}
	}

	return account.IsModelSupported(trimmed)
}

func hasExplicitModelRestriction(account *Account) bool {
	if account == nil || account.Credentials == nil {
		return false
	}
	rawMapping, ok := account.Credentials["model_mapping"]
	if !ok || rawMapping == nil {
		return false
	}
	switch mapping := rawMapping.(type) {
	case map[string]any:
		return len(mapping) > 0
	case map[string]string:
		return len(mapping) > 0
	default:
		return false
	}
}

func MatchesOpenAIQuotaStrategyFilter(account *Account, requestedStrategy string) bool {
	if account == nil {
		return false
	}

	switch strings.TrimSpace(requestedStrategy) {
	case "":
		return true
	case "prefer_5h":
		return account.GetOpenAIQuotaStrategy() == "prefer_5h"
	case "prefer_7d":
		return account.GetOpenAIQuotaStrategy() == "prefer_7d"
	case "enabled":
		strategy := account.GetOpenAIQuotaStrategy()
		return strategy == "prefer_5h" || strategy == "prefer_7d"
	case "disabled":
		return account.GetOpenAIQuotaStrategy() == ""
	default:
		return false
	}
}

func MatchesAccountListStatusFilter(account *Account, requestedStatus string, now time.Time) bool {
	if account == nil {
		return false
	}

	switch strings.TrimSpace(requestedStatus) {
	case "":
		return true
	case StatusActive:
		return matchesActiveAccountListStatusFilter(account, now, false)
	case AccountStatusFilterActiveExcludingQuotaStopped:
		return matchesActiveAccountListStatusFilter(account, now, true)
	case "rate_limited":
		return account.Status == StatusActive &&
			isAccountRateLimitedAt(account, now) &&
			!isAccountTempUnschedulableAt(account, now)
	case "temp_unschedulable":
		return account.Status == StatusActive && isAccountTempUnschedulableAt(account, now)
	case "unschedulable":
		return account.Status == StatusActive &&
			!account.Schedulable &&
			!isAccountRateLimitedAt(account, now) &&
			!isAccountTempUnschedulableAt(account, now)
	case AccountStatusFilterOpenAI5HUsedZero:
		return isOpenAIUsagePercentExactlyZero(account, "codex_5h_used_percent")
	case AccountStatusFilterOpenAI7DUsedZero:
		return isOpenAIUsagePercentExactlyZero(account, "codex_7d_used_percent")
	default:
		return account.Status == requestedStatus
	}
}

func matchesActiveAccountListStatusFilter(account *Account, now time.Time, excludeQuotaStopped bool) bool {
	if account == nil {
		return false
	}
	if account.Status != StatusActive || !account.Schedulable {
		return false
	}
	if isAccountRateLimitedAt(account, now) || isAccountTempUnschedulableAt(account, now) {
		return false
	}
	if excludeQuotaStopped && !account.IsOpenAIQuotaStrategySchedulable() {
		return false
	}
	return true
}

func isAccountRateLimitedAt(account *Account, now time.Time) bool {
	return account != nil && account.RateLimitResetAt != nil && account.RateLimitResetAt.After(now)
}

func isAccountTempUnschedulableAt(account *Account, now time.Time) bool {
	return account != nil && account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(now)
}

func isOpenAIUsagePercentExactlyZero(account *Account, key string) bool {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth || account.Extra == nil {
		return false
	}
	window := ""
	switch key {
	case "codex_5h_used_percent":
		window = "5h"
	case "codex_7d_used_percent":
		window = "7d"
	default:
		return false
	}
	progress := buildCodexUsageProgressFromExtra(account.Extra, window, time.Now())
	return progress != nil && progress.Utilization == 0
}

func buildAnthropicModelAliases(requestedModel string) []string {
	trimmed := strings.TrimSpace(requestedModel)
	if trimmed == "" {
		return nil
	}

	aliases := make([]string, 0, 8)
	seen := make(map[string]struct{}, 8)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		aliases = append(aliases, value)
	}

	add(trimmed)
	add(claude.NormalizeModelID(trimmed))
	add(claude.DenormalizeModelID(trimmed))

	if short := stripAnthropicDateSuffix(trimmed); short != trimmed {
		add(short)
		add(claude.NormalizeModelID(short))
	}

	for _, model := range claude.DefaultModels {
		modelID := strings.TrimSpace(model.ID)
		shortID := stripAnthropicDateSuffix(modelID)
		if trimmed == modelID || trimmed == shortID {
			add(modelID)
			add(shortID)
		}
	}

	return aliases
}

func mappingSupportsAnthropicModelAlias(mapping map[string]string, requestedModel string) bool {
	normalizedRequested := stripAnthropicDateSuffix(requestedModel)
	if normalizedRequested == "" {
		return false
	}
	for key, value := range mapping {
		if stripAnthropicDateSuffix(key) == normalizedRequested {
			return true
		}
		if stripAnthropicDateSuffix(value) == normalizedRequested {
			return true
		}
	}
	return false
}

func stripAnthropicDateSuffix(model string) string {
	parts := strings.Split(strings.TrimSpace(model), "-")
	if len(parts) < 2 {
		return strings.TrimSpace(model)
	}
	last := parts[len(parts)-1]
	if len(last) != 8 {
		return strings.TrimSpace(model)
	}
	for _, ch := range last {
		if ch < '0' || ch > '9' {
			return strings.TrimSpace(model)
		}
	}
	return strings.Join(parts[:len(parts)-1], "-")
}

func buildOpenAIModelFilterEntries() []AccountModelFilterEntry {
	entries := make([]AccountModelFilterEntry, 0, len(openai.DefaultModels))
	for _, model := range openai.DefaultModels {
		entries = append(entries, AccountModelFilterEntry{Value: model.ID, Label: model.DisplayName})
	}
	return entries
}

func buildClaudeModelFilterEntries() []AccountModelFilterEntry {
	entries := make([]AccountModelFilterEntry, 0, len(claude.DefaultModels))
	for _, model := range claude.DefaultModels {
		entries = append(entries, AccountModelFilterEntry{Value: model.ID, Label: model.DisplayName})
	}
	return entries
}

func buildGeminiModelFilterEntries() []AccountModelFilterEntry {
	entries := make([]AccountModelFilterEntry, 0, len(geminicli.DefaultModels))
	for _, model := range geminicli.DefaultModels {
		entries = append(entries, AccountModelFilterEntry{Value: model.ID, Label: model.DisplayName})
	}
	return entries
}

func buildAntigravityModelFilterEntries() []AccountModelFilterEntry {
	models := antigravity.DefaultModels()
	entries := make([]AccountModelFilterEntry, 0, len(models))
	for _, model := range models {
		entries = append(entries, AccountModelFilterEntry{Value: model.ID, Label: model.DisplayName})
	}
	return entries
}

package antigravity

import (
	"encoding/json"
	"strconv"
	"strings"
)

// GeminiThinkingMode describes the thinkingConfig contract supported by a
// Gemini model family.
type GeminiThinkingMode uint8

const (
	GeminiThinkingUnsupported GeminiThinkingMode = iota
	GeminiThinkingLevel
	GeminiThinkingBudget
)

// Gemini25FlashThinkingBudgetLimit is the provider limit for Gemini 2.5 Flash
// thinking budgets.
const Gemini25FlashThinkingBudgetLimit = 24576

// GeminiThinkingSettings is the provider-native thinking configuration derived
// from an API-agnostic reasoning effort.
type GeminiThinkingSettings struct {
	IncludeThoughts bool
	ThinkingLevel   string
	ThinkingBudget  int
}

// ResolveGeminiThinkingMode resolves only model families with a known
// thinkingConfig contract. Unknown generations are deliberately left alone so
// a newly introduced provider model is not sent incompatible parameters.
func ResolveGeminiThinkingMode(model string) GeminiThinkingMode {
	model = normalizeGeminiModelID(model)
	if model == "" || !strings.HasPrefix(model, "gemini-") {
		return GeminiThinkingUnsupported
	}

	parts := strings.Split(strings.TrimPrefix(model, "gemini-"), "-")
	if len(parts) < 2 || isGeminiNonTextModel(parts[2:]) {
		return GeminiThinkingUnsupported
	}

	family := parts[1]
	if family != "pro" && family != "flash" {
		return GeminiThinkingUnsupported
	}
	if family == "flash" && len(parts) > 2 && parts[2] == "lite" {
		return GeminiThinkingUnsupported
	}

	major, minor, ok := parseGeminiGeneration(parts[0])
	if !ok {
		return GeminiThinkingUnsupported
	}
	switch {
	case major == 3:
		return GeminiThinkingLevel
	case major == 2 && minor == 5:
		return GeminiThinkingBudget
	default:
		return GeminiThinkingUnsupported
	}
}

// GeminiThinkingSettingsForEffort maps a normalized reasoning effort to the
// model family's native thinkingConfig representation. An omitted effort uses
// the provider's lowest thinking level for supported families.
func GeminiThinkingSettingsForEffort(model, effort string) (GeminiThinkingSettings, bool) {
	mode := ResolveGeminiThinkingMode(model)
	if mode == GeminiThinkingUnsupported {
		return GeminiThinkingSettings{}, false
	}

	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" {
		effort = "low"
	}

	var settings GeminiThinkingSettings
	switch effort {
	case "low":
		settings.ThinkingLevel = "LOW"
		settings.ThinkingBudget = 1024
	case "medium":
		settings.ThinkingLevel = "MEDIUM"
		settings.ThinkingBudget = 4096
	case "high":
		settings.ThinkingLevel = "HIGH"
		settings.ThinkingBudget = 10240
	case "xhigh", "max":
		settings.ThinkingLevel = "HIGH"
		settings.ThinkingBudget = 32768
	default:
		return GeminiThinkingSettings{}, false
	}

	settings.IncludeThoughts = true
	if mode == GeminiThinkingLevel {
		settings.ThinkingBudget = 0
		return settings, true
	}
	settings.ThinkingLevel = ""
	if limit := GeminiThinkingBudgetLimitForModel(model); limit > 0 && settings.ThinkingBudget > limit {
		settings.ThinkingBudget = limit
	}
	return settings, true
}

// GeminiThinkingBudgetLimitForModel returns a non-zero limit only for a
// supported budget-based Gemini family.
func GeminiThinkingBudgetLimitForModel(model string) int {
	if ResolveGeminiThinkingMode(model) != GeminiThinkingBudget {
		return 0
	}
	normalizedModel := normalizeGeminiModelID(model)
	parts := strings.Split(strings.TrimPrefix(normalizedModel, "gemini-"), "-")
	if len(parts) > 1 && parts[1] == "flash" {
		return Gemini25FlashThinkingBudgetLimit
	}
	return 0
}

// GeminiThinkingLevelForBudget converts the budget shape used by the Claude
// compatibility request into Gemini 3's level shape.
func GeminiThinkingLevelForBudget(budget int) string {
	switch {
	case budget > 0 && budget <= 1024:
		return "LOW"
	case budget > 0 && budget <= 4096:
		return "MEDIUM"
	default:
		return "HIGH"
	}
}

// ApplyGeminiThinkingConfig updates only generationConfig.thinkingConfig in a
// native Gemini request. Supported Gemini families default to their lowest
// thinking level when effort is omitted. Unsupported models and unknown effort
// values are passed through byte-for-byte so other provider paths keep their
// existing behavior.
func ApplyGeminiThinkingConfig(body []byte, model string, effort *string) ([]byte, error) {
	requestedEffort := ""
	if effort != nil {
		requestedEffort = *effort
	}
	settings, ok := GeminiThinkingSettingsForEffort(model, requestedEffort)
	if !ok {
		return body, nil
	}

	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, err
	}
	generationConfig, ok := request["generationConfig"].(map[string]any)
	if !ok || generationConfig == nil {
		generationConfig = make(map[string]any)
	}
	thinkingConfig, ok := generationConfig["thinkingConfig"].(map[string]any)
	if !ok || thinkingConfig == nil {
		thinkingConfig = make(map[string]any)
	}

	thinkingConfig["includeThoughts"] = settings.IncludeThoughts
	if settings.ThinkingLevel != "" {
		delete(thinkingConfig, "thinkingBudget")
		thinkingConfig["thinkingLevel"] = settings.ThinkingLevel
	} else {
		delete(thinkingConfig, "thinkingLevel")
		thinkingConfig["thinkingBudget"] = settings.ThinkingBudget
	}
	generationConfig["thinkingConfig"] = thinkingConfig
	request["generationConfig"] = generationConfig

	return json.Marshal(request)
}

func normalizeGeminiModelID(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	model = strings.TrimPrefix(model, "models/")
	return model
}

func isGeminiNonTextModel(suffixes []string) bool {
	for _, suffix := range suffixes {
		switch suffix {
		case "image", "audio", "tts", "live", "embedding":
			return true
		}
	}
	return false
}

func parseGeminiGeneration(version string) (major, minor int, ok bool) {
	parts := strings.Split(version, ".")
	if len(parts) > 2 || len(parts) == 0 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	if len(parts) == 2 {
		minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, false
		}
	}
	return major, minor, true
}

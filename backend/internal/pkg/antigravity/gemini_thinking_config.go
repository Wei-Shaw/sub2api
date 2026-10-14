package antigravity

import (
	"encoding/json"
	"strconv"
	"strings"
)

const (
	geminiThinkingLevelLow    = "LOW"
	geminiThinkingLevelMedium = "MEDIUM"
	geminiThinkingLevelHigh   = "HIGH"

	gemini25ThinkingBudgetLow    = 1024
	gemini25ThinkingBudgetMedium = 4096
	gemini25ThinkingBudgetHigh   = 10240
)

type geminiThinkingMode uint8

const (
	geminiThinkingUnsupported geminiThinkingMode = iota
	geminiThinkingLevelMode
	geminiThinkingBudgetMode
)

// ApplyGeminiThinkingConfig writes an explicit reasoning level into a Gemini
// request. Gemini 3 uses thinkingLevel; Gemini 2.5 uses thinkingBudget.
// Unsupported models and empty levels are returned unchanged.
func ApplyGeminiThinkingConfig(body []byte, model, level string) ([]byte, error) {
	level = normalizeGeminiThinkingLevel(level)
	if level == "" {
		return body, nil
	}

	mode := geminiThinkingModeForModel(model)
	if mode == geminiThinkingUnsupported {
		return body, nil
	}

	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, err
	}

	generationConfig, _ := request["generationConfig"].(map[string]any)
	if generationConfig == nil {
		generationConfig = make(map[string]any)
	}
	thinkingConfig, _ := generationConfig["thinkingConfig"].(map[string]any)
	if thinkingConfig == nil {
		thinkingConfig = make(map[string]any)
	}
	thinkingConfig["includeThoughts"] = true

	if mode == geminiThinkingLevelMode {
		delete(thinkingConfig, "thinkingBudget")
		thinkingConfig["thinkingLevel"] = level
	} else {
		delete(thinkingConfig, "thinkingLevel")
		thinkingConfig["thinkingBudget"] = gemini25ThinkingBudgetForLevel(model, level)
	}

	generationConfig["thinkingConfig"] = thinkingConfig
	request["generationConfig"] = generationConfig
	return json.Marshal(request)
}

func normalizeGeminiThinkingLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "minimal", "low":
		return geminiThinkingLevelLow
	case "medium":
		return geminiThinkingLevelMedium
	case "high", "xhigh", "max":
		return geminiThinkingLevelHigh
	default:
		return ""
	}
}

func geminiThinkingModeForModel(model string) geminiThinkingMode {
	model = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(model), "models/"))
	if !strings.HasPrefix(model, "gemini-") {
		return geminiThinkingUnsupported
	}

	parts := strings.Split(strings.TrimPrefix(model, "gemini-"), "-")
	if len(parts) < 2 || containsGeminiNonTextModality(parts[2:]) {
		return geminiThinkingUnsupported
	}
	if family := parts[1]; family != "flash" && family != "pro" {
		return geminiThinkingUnsupported
	}
	if len(parts) > 2 && parts[1] == "flash" && parts[2] == "lite" {
		return geminiThinkingUnsupported
	}

	major, minor, ok := parseGeminiVersion(parts[0])
	if !ok {
		return geminiThinkingUnsupported
	}
	switch {
	case major == 3:
		return geminiThinkingLevelMode
	case major == 2 && minor == 5:
		return geminiThinkingBudgetMode
	default:
		return geminiThinkingUnsupported
	}
}

func gemini25ThinkingBudgetForLevel(model, level string) int {
	var budget int
	switch level {
	case geminiThinkingLevelLow:
		budget = gemini25ThinkingBudgetLow
	case geminiThinkingLevelMedium:
		budget = gemini25ThinkingBudgetMedium
	default:
		budget = gemini25ThinkingBudgetHigh
	}

	normalized := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(model), "models/"))
	if strings.HasPrefix(normalized, "gemini-2.5-flash") &&
		budget > Gemini25FlashThinkingBudgetLimit {
		return Gemini25FlashThinkingBudgetLimit
	}
	return budget
}

func containsGeminiNonTextModality(suffixes []string) bool {
	for _, suffix := range suffixes {
		switch suffix {
		case "image", "audio", "tts", "live", "embedding":
			return true
		}
	}
	return false
}

func parseGeminiVersion(version string) (major, minor int, ok bool) {
	parts := strings.Split(version, ".")
	if len(parts) == 0 || len(parts) > 2 {
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

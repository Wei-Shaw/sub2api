package service

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

var openAIPromptCacheLegacyModels = map[string]struct{}{
	"gpt-5.5":             {},
	"gpt-5.5-pro":         {},
	"gpt-5.4":             {},
	"gpt-5.2":             {},
	"gpt-5.1-codex-max":   {},
	"gpt-5.1":             {},
	"gpt-5.1-codex":       {},
	"gpt-5.1-codex-mini":  {},
	"gpt-5.1-chat-latest": {},
	"gpt-5":               {},
	"gpt-5-codex":         {},
	"gpt-4.1":             {},
}

// isOpenAIPromptCacheGPT56OrLater is deliberately numeric rather than an
// alias lookup: dated/preview GPT-5.6 models and all later major/minor
// versions must take the new prompt_cache_options branch.
func isOpenAIPromptCacheGPT56OrLater(model string) bool {
	model = strings.ToLower(strings.TrimSpace(lastOpenAIModelSegment(model)))
	if !strings.HasPrefix(model, "gpt-") {
		return false
	}
	version := strings.TrimPrefix(model, "gpt-")
	dot := strings.IndexByte(version, '.')
	if dot < 1 {
		return false
	}
	major, errMajor := strconv.Atoi(version[:dot])
	minorAndSuffix := version[dot+1:]
	minorEnd := strings.IndexByte(minorAndSuffix, '-')
	minorText := minorAndSuffix
	suffix := ""
	if minorEnd >= 0 {
		minorText = minorAndSuffix[:minorEnd]
		suffix = minorAndSuffix[minorEnd:]
	}
	minor, errMinor := strconv.Atoi(minorText)
	if errMajor != nil || errMinor != nil {
		return false
	}
	if suffix != "" && !isOpenAIPromptCacheStructuredSuffix(suffix) {
		return false
	}
	return major > 5 || (major == 5 && minor >= 6)
}

func isOpenAIPromptCacheStructuredSuffix(suffix string) bool {
	if suffix == "-sol" || suffix == "-terra" || suffix == "-luna" {
		return true
	}
	if len(suffix) != len("-2026-08-01") || suffix[0] != '-' || !isDigits(suffix[1:5]) || suffix[5] != '-' ||
		!isDigits(suffix[6:8]) || suffix[8] != '-' || !isDigits(suffix[9:11]) {
		return false
	}
	return true
}

func isOpenAIPromptCacheLegacyModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(lastOpenAIModelSegment(model)))
	if _, ok := openAIPromptCacheLegacyModels[model]; ok {
		return true
	}
	for legacy := range openAIPromptCacheLegacyModels {
		suffix := strings.TrimPrefix(model, legacy)
		if strings.HasPrefix(model, legacy) && len(suffix) == len("-2000-01-01") && suffix[0] == '-' &&
			isDigits(suffix[1:5]) && suffix[5] == '-' && isDigits(suffix[6:8]) && suffix[8] == '-' && isDigits(suffix[9:11]) {
			return true
		}
	}
	return false
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// supportsOpenAIPromptCacheRetention is the pure account/model predicate.
// Unlike endpoint scheduling capabilities, this is an egress field policy:
// credentials are an explicit default-deny opt-in and no base URL inference is
// allowed.
func supportsOpenAIPromptCacheRetention(account *Account, upstreamModel string) bool {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		return false
	}
	if isOpenAIPromptCacheGPT56OrLater(upstreamModel) || !isOpenAIPromptCacheLegacyModel(upstreamModel) {
		return false
	}
	capabilities, found := account.openAIEndpointCapabilitySet()
	return found && capabilities[string(OpenAIEndpointCapabilityPromptCacheRetention)]
}

// normalizeOpenAIPromptCacheFields applies the same final-model policy to a
// decoded request/frame. prompt_cache_key is intentionally never touched.
func normalizeOpenAIPromptCacheFields(body map[string]any, account *Account, upstreamModel string) bool {
	if body == nil {
		return false
	}
	if upstreamModel == "" {
		upstreamModel, _ = body["model"].(string)
	}
	// GPT-5.6+ uses the new cache-options shape. The model-generation rule
	// takes precedence over an account's legacy-retention capability, including
	// for OpenAI-compatible upstreams that map to a GPT-5.6+ model.
	keepOptions := isOpenAIPromptCacheGPT56OrLater(upstreamModel) && account != nil && account.Platform == PlatformOpenAI && account.Type == AccountTypeAPIKey
	keepRetention := supportsOpenAIPromptCacheRetention(account, upstreamModel)
	changed := false
	if !keepRetention {
		if _, ok := body["prompt_cache_retention"]; ok {
			delete(body, "prompt_cache_retention")
			changed = true
		}
	}
	if !keepOptions {
		if _, ok := body["prompt_cache_options"]; ok {
			delete(body, "prompt_cache_options")
			changed = true
		}
	}
	return changed
}

func normalizeOpenAIPromptCacheFieldsRaw(body []byte, account *Account, upstreamModel string) ([]byte, bool, error) {
	return normalizeOpenAIPromptCacheFieldsRawWithSessionModel(body, account, upstreamModel, "")
}

func openAIPromptCacheFieldsPresent(body []byte) bool {
	values := gjson.GetManyBytes(body, "prompt_cache_retention", "prompt_cache_options")
	return values[0].Exists() || values[1].Exists()
}

func normalizeOpenAIPromptCacheFieldsRawWithSessionModel(body []byte, account *Account, upstreamModel, sessionUpstreamModel string) ([]byte, bool, error) {
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return body, false, err
	}
	changed := normalizeOpenAIPromptCacheFields(decoded, account, upstreamModel)
	if session, ok := decoded["session"].(map[string]any); ok {
		sessionModel := sessionUpstreamModel
		if sessionModel == "" {
			sessionModel, _ = session["model"].(string)
		}
		if sessionModel == "" {
			sessionModel = upstreamModel
		}
		if normalizeOpenAIPromptCacheFields(session, account, sessionModel) {
			changed = true
		}
	}
	if !changed {
		return body, false, nil
	}
	normalized, err := json.Marshal(decoded)
	return normalized, true, err
}

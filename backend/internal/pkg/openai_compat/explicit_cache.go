package openai_compat

import (
	"strings"

	"github.com/tidwall/gjson"
)

const ExplicitCacheParam = "explicit_cache"

// ShouldRejectGPT56ResponsesExplicitCache reports whether a Responses request
// uses the legacy top-level explicit_cache field with GPT-5.6 series models.
func ShouldRejectGPT56ResponsesExplicitCache(model string, body []byte) bool {
	return IsGPT56SeriesModel(model) && gjson.GetBytes(body, ExplicitCacheParam).Exists()
}

// IsGPT56SeriesModel matches GPT-5.6 model names, including provider-prefixed
// aliases such as "openai/gpt-5.6-sol".
func IsGPT56SeriesModel(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if slash := strings.LastIndex(normalized, "/"); slash >= 0 {
		normalized = normalized[slash+1:]
	}

	return normalized == "gpt-5.6" || strings.HasPrefix(normalized, "gpt-5.6-")
}

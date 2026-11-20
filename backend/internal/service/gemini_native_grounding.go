package service

import (
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

// Gemini 3 and newer meter Google Search grounding per query.
// See https://ai.google.dev/gemini-api/docs/google-search#pricing.
func geminiPerQueryGrounding(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndexByte(model, '/'); i >= 0 {
		model = model[i+1:]
	}
	version, ok := strings.CutPrefix(model, "gemini-")
	end := strings.IndexAny(version, ".-")
	if !ok || end < 0 {
		return false
	}
	major, err := strconv.Atoi(version[:end])
	return err == nil && major >= 3
}

func configuredGeminiGroundingPrice(apiKey *APIKey) *float64 {
	group := apiKeyGroup(apiKey)
	if group == nil || group.Platform != PlatformGemini {
		return nil
	}
	price := group.GetSearchPricePer1k()
	if price == nil || *price <= 0 {
		return nil
	}
	return price
}

// The set is scoped to one response, across all candidates and stream frames.
type geminiGroundingQueries map[string]struct{}

func (queries *geminiGroundingQueries) observe(data []byte) {
	candidates := gjson.GetBytes(data, "candidates")
	if !candidates.IsArray() {
		return
	}
	candidates.ForEach(func(_, candidate gjson.Result) bool {
		returned := candidate.Get("groundingMetadata.webSearchQueries")
		if !returned.IsArray() {
			return true
		}
		returned.ForEach(func(_, query gjson.Result) bool {
			if text := strings.TrimSpace(query.Str); query.Type == gjson.String && text != "" {
				if *queries == nil {
					*queries = make(geminiGroundingQueries)
				}
				(*queries)[text] = struct{}{}
			}
			return true
		})
		return true
	})
}

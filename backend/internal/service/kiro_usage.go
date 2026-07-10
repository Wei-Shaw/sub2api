package service

import "github.com/tidwall/gjson"

func kiroCreditsFromUsageGJSON(value gjson.Result) float64 {
	if !value.Exists() || !value.IsObject() {
		return 0
	}
	for _, field := range []string{
		"_sub2api_kiro_credits",
		"kiro_credits",
		"kiroCredits",
		"credits",
		"creditsUsed",
		"creditUsage",
		"consumedCredits",
	} {
		if credits := value.Get(field); credits.Exists() && credits.Float() > 0 {
			return credits.Float()
		}
	}
	return 0
}

func mergeOpenAIUsageKiroCreditsFromJSON(usage *OpenAIUsage, body []byte) {
	if usage == nil || len(body) == 0 || !gjson.ValidBytes(body) {
		return
	}
	if credits := kiroCreditsFromUsageGJSON(gjson.GetBytes(body, "usage")); credits > 0 {
		usage.KiroCredits = credits
		return
	}
	if credits := kiroCreditsFromUsageGJSON(gjson.GetBytes(body, "response.usage")); credits > 0 {
		usage.KiroCredits = credits
	}
}

package service

import "github.com/tidwall/gjson"

func kiroCreditsFromUsageGJSON(usage gjson.Result) float64 {
	if !usage.Exists() {
		return 0
	}
	for _, key := range []string{"_sub2api_kiro_credits", "kiro_credits", "kiroCredits", "credits", "creditsUsed", "creditUsage"} {
		if v := usage.Get(key); v.Exists() && v.Float() > 0 {
			return v.Float()
		}
	}
	return 0
}

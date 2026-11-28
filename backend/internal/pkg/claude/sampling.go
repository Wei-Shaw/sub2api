package claude

import "strings"

var temperatureRejectingFamilies = []string{
	"claude-opus-4-7",
	"claude-opus-4-8",
}

func RejectsTemperature(model string) bool {
	if IsOpus55(model) || IsFable52(model) || IsMythos52(model) {
		return true
	}
	id := normalizeEffortModelID(model)
	for _, family := range temperatureRejectingFamilies {
		if id == family || strings.HasPrefix(id, family+"-") {
			return true
		}
	}
	return false
}

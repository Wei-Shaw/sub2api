package claude

import (
	"strings"
	"unicode"
)

var (
	effortLowMediumHigh         = []string{"low", "medium", "high"}
	effortLowMediumHighMax      = []string{"low", "medium", "high", "max"}
	effortLowMediumHighXHighMax = []string{"low", "medium", "high", "xhigh", "max"}
)

var effortFamilies = []struct {
	family string
	levels []string
}{
	{family: "claude-mythos-preview", levels: effortLowMediumHighMax},
	{family: "claude-mythos-5", levels: effortLowMediumHighXHighMax},
	{family: "claude-fable-5", levels: effortLowMediumHighXHighMax},
	{family: "claude-sonnet-4-6", levels: effortLowMediumHighMax},
	{family: "claude-sonnet-5-5", levels: effortLowMediumHighXHighMax},
	{family: "claude-sonnet-5", levels: effortLowMediumHighXHighMax},
	{family: "claude-opus-4-8", levels: effortLowMediumHighXHighMax},
	{family: "claude-opus-4-7", levels: effortLowMediumHighXHighMax},
	{family: "claude-opus-4-6", levels: effortLowMediumHighMax},
	{family: "claude-opus-4-5", levels: effortLowMediumHigh},
	{family: "claude-opus-5-5", levels: effortLowMediumHighXHighMax},
	{family: "claude-opus-5", levels: effortLowMediumHighXHighMax},
}

// EffortLevelsForModel returns the output_config.effort values accepted by a
// Claude model, ordered from the lightest to the deepest reasoning level.
func EffortLevelsForModel(model string) []string {
	id := normalizeEffortModelID(model)
	for _, entry := range effortFamilies {
		if id == entry.family || strings.HasPrefix(id, entry.family+"-") {
			return append([]string(nil), entry.levels...)
		}
	}
	return nil
}

// IsOpus55 identifies the fixed Opus 5.5 ID after provider/local suffix normalization.
func IsOpus55(model string) bool {
	return normalizeEffortModelID(model) == "claude-opus-5-5"
}

// IsSonnet55 identifies the fixed Sonnet 5.5 ID after provider/local suffix normalization.
func IsSonnet55(model string) bool {
	return normalizeEffortModelID(model) == "claude-sonnet-5-5"
}

func normalizeEffortModelID(model string) string {
	id := strings.ToLower(strings.TrimSpace(model))
	id = strings.TrimPrefix(id, "models/")
	if slash := strings.IndexByte(id, '/'); slash >= 0 {
		id = strings.TrimPrefix(strings.TrimSpace(id[slash+1:]), "models/")
	}
	for _, prefix := range []string{"us.", "eu.", "apac.", "jp.", "au.", "us-gov.", "global."} {
		id = strings.TrimPrefix(id, prefix)
	}
	id = strings.TrimPrefix(id, "anthropic.")
	id = strings.TrimSuffix(id, "-thinking")
	// OpenRouter uses dotted minor versions for some models. Normalize them
	// before effort, thinking, and billing family lookups.
	if id == "claude-opus-5.5" {
		id = "claude-opus-5-5"
	}
	if id == "claude-sonnet-5.5" {
		id = "claude-sonnet-5-5"
	}
	if mapped, ok := ModelIDReverseOverrides[id]; ok {
		id = mapped
	}
	if len(id) >= 9 {
		suffix := id[len(id)-9:]
		if suffix[0] == '-' {
			digits := true
			for _, r := range suffix[1:] {
				if !unicode.IsDigit(r) {
					digits = false
					break
				}
			}
			if digits {
				id = id[:len(id)-9]
			}
		}
	}
	return id
}

// SupportsMidConversationSystem returns true for Claude models that support
// inline system messages (Claude 5.x and later). Earlier models (3.x, 4.x)
// require system content to be hoisted to the top-level system parameter.
// See: https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#mid-conversation-system-messages
func SupportsMidConversationSystem(model string) bool {
	normalized := normalizeEffortModelID(model)
	if !strings.HasPrefix(normalized, "claude-") {
		return false
	}
	// Extract major version from claude-{family}-{major}-{minor} or claude-{family}-{major}
	// e.g., "claude-opus-5-5" -> parts[2] = "5"
	//       "claude-sonnet-5" -> parts[2] = "5"
	parts := strings.Split(normalized, "-")
	if len(parts) < 3 {
		return false
	}
	// parts[0]="claude", parts[1]="opus/sonnet/haiku/etc", parts[2]=major version
	majorStr := parts[2]
	// Handle potential decimal point (though normalizeEffortModelID already handles it)
	if dotIdx := strings.IndexByte(majorStr, '.'); dotIdx >= 0 {
		majorStr = majorStr[:dotIdx]
	}
	// Parse major version number
	major := 0
	for _, r := range majorStr {
		if !unicode.IsDigit(r) {
			return false
		}
		major = major*10 + int(r-'0')
	}
	// Claude 5.x and later support inline system messages
	return major >= 5
}

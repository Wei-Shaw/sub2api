// Package minimax defines model-specific MiniMax reasoning semantics shared by
// the gateway and protocol converters.
package minimax

import (
	"fmt"
	"strings"
)

const (
	ModelM31FlashPreview   = "MiniMax-M3.1-Flash-Preview"
	DefaultReasoningEffort = "max"
)

func IsM31(model string) bool {
	model = strings.TrimSpace(model)
	if i := strings.LastIndex(model, "/"); i >= 0 {
		model = model[i+1:]
	}
	return strings.EqualFold(model, ModelM31FlashPreview)
}

func ReasoningEfforts() []string {
	return []string{"low", "medium", "high", "xhigh", "max"}
}

func ValidateReasoningEffort(model, effort string) error {
	if !IsM31(model) || effort == "" {
		return nil
	}
	switch effort {
	case "low", "medium", "high", "xhigh", "max":
		return nil
	default:
		return fmt.Errorf("%s does not support reasoning effort %q; use low, medium, high, xhigh or max", model, effort)
	}
}

func ValidateThinkingType(model, thinkingType string) error {
	if !IsM31(model) {
		return nil
	}
	switch thinkingType {
	case "", "enabled", "adaptive":
		// enabled is the client-side Anthropic spelling, normalized on forwarding.
		return nil
	default:
		return fmt.Errorf("%s always thinks; omit thinking or use thinking.type adaptive", model)
	}
}

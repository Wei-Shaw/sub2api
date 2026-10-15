package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/minimax"
	"github.com/tidwall/gjson"
)

// Validate before protocol conversion or none-placeholder filtering can discard
// an explicit disable. model must be the final mapped upstream model.
func validateMiniMaxM31Request(body []byte, model string) error {
	if !minimax.IsM31(model) {
		return nil
	}
	if err := minimax.ValidateThinkingType(model, gjson.GetBytes(body, "thinking.type").String()); err != nil {
		return err
	}
	for _, path := range []string{"output_config.effort", "reasoning_effort", "reasoning.effort"} {
		if err := minimax.ValidateReasoningEffort(model, gjson.GetBytes(body, path).String()); err != nil {
			return err
		}
	}
	return nil
}

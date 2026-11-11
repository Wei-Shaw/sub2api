package service

import "github.com/Wei-Shaw/sub2api/internal/pkg/openai"

// A Free web-image account exposes every supported image compatibility entry,
// independently of its ordinary chat-model mapping. Preserve the nil/default
// catalog fallback and never add unrelated text models to a restricted catalog.
func supplementChatGPTWebImageModels(accounts []Account, models []string) []string {
	if len(models) == 0 {
		return models
	}
	for i := range accounts {
		if validateChatGPTWebImageAccount(&accounts[i]) != nil {
			continue
		}
		out := append([]string(nil), models...)
		for _, model := range openai.DefaultModels {
			if IsGPTImageGenerationModel(model.ID) {
				out = append(out, model.ID)
			}
		}
		return dedupeAndSortModelIDs(out)
	}
	return models
}

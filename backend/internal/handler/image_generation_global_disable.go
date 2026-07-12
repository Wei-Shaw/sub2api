package handler

import "github.com/Wei-Shaw/sub2api/internal/service"

func (h *OpenAIGatewayHandler) imageGenerationGloballyDisabled() bool {
	return h != nil && h.cfg != nil && h.cfg.DisableImageGeneration
}

func (h *OpenAIGatewayHandler) normalizeGloballyDisabledImageGeneration(payload []byte) ([]byte, bool, error) {
	if !h.imageGenerationGloballyDisabled() {
		return payload, false, nil
	}
	return service.StripOpenAIImageGenerationToolsFromRawPayload(payload)
}

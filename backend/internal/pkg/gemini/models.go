// Package gemini provides minimal fallback model metadata for Gemini native endpoints.
// It is used when upstream model listing is unavailable (e.g. OAuth token missing AI Studio scopes).
package gemini

import "strings"

type Model struct {
	Name                       string   `json:"name"`
	DisplayName                string   `json:"displayName,omitempty"`
	Description                string   `json:"description,omitempty"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods,omitempty"`
}

type ModelsListResponse struct {
	Models []Model `json:"models"`
}

func DefaultModels() []Model {
	methods := []string{"generateContent", "streamGenerateContent"}
	return []Model{
		{Name: "models/gemini-2.0-flash", SupportedGenerationMethods: methods},
		{Name: "models/gemini-2.5-flash", SupportedGenerationMethods: methods},
		{Name: "models/gemini-2.5-flash-image", SupportedGenerationMethods: methods},
		{Name: "models/gemini-2.5-pro", SupportedGenerationMethods: methods},
		{Name: "models/gemini-3-flash-preview", SupportedGenerationMethods: methods},
		{Name: "models/gemini-3-pro-preview", SupportedGenerationMethods: methods},
		{Name: "models/gemini-3.1-pro-preview", SupportedGenerationMethods: methods},
		{Name: "models/gemini-3.1-pro-preview-customtools", SupportedGenerationMethods: methods},
		{Name: "models/gemini-3.1-flash-image", SupportedGenerationMethods: methods},
	}
}

func HasFallbackModel(model string) bool {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return false
	}
	if !strings.HasPrefix(trimmed, "models/") {
		trimmed = "models/" + trimmed
	}
	for _, model := range DefaultModels() {
		if model.Name == trimmed {
			return true
		}
	}
	return false
}

func FallbackModelsList() ModelsListResponse {
	return ModelsListResponse{Models: DefaultModels()}
}

func FallbackModel(model string) Model {
	methods := []string{"generateContent", "streamGenerateContent"}
	if model == "" {
		return Model{Name: "models/unknown", SupportedGenerationMethods: methods}
	}
	if len(model) >= 7 && model[:7] == "models/" {
		return Model{Name: model, SupportedGenerationMethods: methods}
	}
	return Model{Name: "models/" + model, SupportedGenerationMethods: methods}
}

// Package bailian provides constants and helpers for the Alibaba Cloud
// Bailian (DashScope / Model Studio) platform.
package bailian

// Model describes a Bailian model in OpenAI-compatible /models shape.
type Model struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created,omitempty"`
	OwnedBy     string `json:"owned_by"`
	DisplayName string `json:"display_name,omitempty"`
}

// DefaultTextModelID is the fallback text model used when Claude-family model
// names are dispatched onto a Bailian group.
const DefaultTextModelID = "qwen3-max"

// Model IDs mirror the DashScope catalog; verify against the Bailian console
// before relying on them (the upstream catalog moves fast).
var defaultModels = []Model{
	// Text models served through the OpenAI-compatible endpoint.
	{ID: "qwen3-max", Object: "model", OwnedBy: "alibaba", DisplayName: "Qwen3 Max"},
	{ID: "qwen-plus", Object: "model", OwnedBy: "alibaba", DisplayName: "Qwen Plus"},
	{ID: "qwen-flash", Object: "model", OwnedBy: "alibaba", DisplayName: "Qwen Flash"},
	{ID: "qwen-turbo", Object: "model", OwnedBy: "alibaba", DisplayName: "Qwen Turbo"},
	// Video generation models served through the async DashScope task API.
	{ID: "happyhorse-1.1-t2v", Object: "model", OwnedBy: "alibaba", DisplayName: "HappyHorse 1.1 Text-to-Video"},
	{ID: "happyhorse-1.1-i2v", Object: "model", OwnedBy: "alibaba", DisplayName: "HappyHorse 1.1 Image-to-Video"},
	{ID: "wan2.7-t2v", Object: "model", OwnedBy: "alibaba", DisplayName: "Wan 2.7 Text-to-Video"},
	{ID: "wan2.7-i2v", Object: "model", OwnedBy: "alibaba", DisplayName: "Wan 2.7 Image-to-Video"},
	{ID: "wan2.6-t2v", Object: "model", OwnedBy: "alibaba", DisplayName: "Wan 2.6 Text-to-Video"},
	{ID: "wan2.6-i2v", Object: "model", OwnedBy: "alibaba", DisplayName: "Wan 2.6 Image-to-Video"},
}

func DefaultModels() []Model {
	out := make([]Model, len(defaultModels))
	copy(out, defaultModels)
	return out
}

func DefaultModelIDs() []string {
	models := DefaultModels()
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}

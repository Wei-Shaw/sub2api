package xai

import (
	"encoding/json"
	"sort"
	"strings"
)

// Model describes an xAI model in OpenAI-compatible /models shape.
type Model struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created,omitempty"`
	OwnedBy     string `json:"owned_by"`
	Type        string `json:"type,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Name        string `json:"name,omitempty"`
}

var defaultModels = []Model{
	{ID: "grok-4.3-fast", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.3 Fast", Name: "Grok 4.3 Fast"},
	{ID: "grok-4.3", Object: "model", OwnedBy: "xai", DisplayName: "Grok 4.3"},
	{ID: "grok-build", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok Build", Name: "Grok Build"},
	{ID: "grok-build-0.1", Object: "model", OwnedBy: "xai", DisplayName: "Grok Build 0.1"},
	{ID: "grok-composer-2.5-fast", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok Composer 2.5 Fast", Name: "Grok Composer 2.5 Fast"},
	{ID: "grok-4.20-0309-reasoning", Object: "model", OwnedBy: "xai", DisplayName: "Grok 4.20 Reasoning"},
	{ID: "grok-4.20-0309-non-reasoning", Object: "model", OwnedBy: "xai", DisplayName: "Grok 4.20 Non Reasoning"},
	{ID: "grok-4.20-multi-agent-0309", Object: "model", OwnedBy: "xai", DisplayName: "Grok 4.20 Multi Agent"},
	{ID: "grok-imagine", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine"},
	{ID: "grok-imagine-image", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Image"},
	{ID: "grok-imagine-image-quality", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Image Quality"},
	{ID: "grok-imagine-edit", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Edit"},
	{ID: "grok-imagine-video", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Video"},
	{ID: "grok-imagine-video-1.5", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Video 1.5"},
	{ID: "grok-imagine-video-1.5-preview", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Video 1.5 Preview"},
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

func DefaultModelMapping() map[string]string {
	mapping := make(map[string]string, len(defaultModels)+3)
	for _, model := range defaultModels {
		mapping[model.ID] = model.ID
	}
	mapping["grok"] = "grok-4.3"
	mapping["grok-latest"] = "grok-4.3"
	mapping["grok-build"] = "grok-build-0.1"
	mapping["grok-4.20-reasoning"] = "grok-4.20-0309-reasoning"
	mapping["grok-4.20-non-reasoning"] = "grok-4.20-0309-non-reasoning"
	return mapping
}

func ParseModelList(data []byte) ([]Model, error) {
	var resp struct {
		Data []Model `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return ModelsFromRaw(resp.Data), nil
}

func ModelsFromRaw(raw []Model) []Model {
	seen := make(map[string]struct{}, len(raw))
	models := make([]Model, 0, len(raw))
	for _, model := range raw {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" {
			continue
		}
		if _, ok := seen[model.ID]; ok {
			continue
		}
		seen[model.ID] = struct{}{}
		model.Object = strings.TrimSpace(model.Object)
		if model.Object == "" {
			model.Object = "model"
		}
		model.OwnedBy = strings.TrimSpace(model.OwnedBy)
		if model.OwnedBy == "" {
			model.OwnedBy = "xai"
		}
		model.Type = strings.TrimSpace(model.Type)
		if model.Type == "" {
			model.Type = "model"
		}
		model.DisplayName = strings.TrimSpace(model.DisplayName)
		model.Name = strings.TrimSpace(model.Name)
		if model.DisplayName == "" {
			model.DisplayName = model.Name
		}
		if model.Name == "" {
			model.Name = model.DisplayName
		}
		if model.DisplayName == "" {
			model.DisplayName = model.ID
			model.Name = model.ID
		}
		models = append(models, model)
	}
	return models
}

func ModelsFromIDs(ids []string) []Model {
	defaultByID := make(map[string]Model, len(defaultModels))
	for _, model := range DefaultModels() {
		defaultByID[model.ID] = model
	}

	seen := make(map[string]struct{}, len(ids))
	models := make([]Model, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if model, ok := defaultByID[id]; ok {
			models = append(models, model)
			continue
		}
		models = append(models, Model{
			ID:          id,
			Object:      "model",
			OwnedBy:     "xai",
			Type:        "model",
			DisplayName: id,
			Name:        id,
		})
	}
	sort.SliceStable(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models
}

type ModelsV2Response struct {
	Object string          `json:"object"`
	Data   []ModelsV2Model `json:"data"`
}

type ModelsV2Model struct {
	ID            string `json:"id"`
	Object        string `json:"object"`
	Name          string `json:"name"`
	ContextWindow int    `json:"context_window"`
	APIBackend    string `json:"api_backend"`
}

func DefaultModelsV2Response() ModelsV2Response {
	return ModelsV2Response{
		Object: "list",
		Data: []ModelsV2Model{
			{
				ID:            "grok-composer-2.5-fast",
				Object:        "model",
				Name:          "Grok Composer 2.5 Fast",
				ContextWindow: 512000,
				APIBackend:    "responses",
			},
			{
				ID:            "grok-build",
				Object:        "model",
				Name:          "Grok Build",
				ContextWindow: 512000,
				APIBackend:    "responses",
			},
		},
	}
}

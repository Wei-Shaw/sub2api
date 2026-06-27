package xai

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	DefaultAPIBaseURL = "https://api.x.ai/v1"
	DefaultTestModel  = "grok-4.3-fast"
)

type Model struct {
	ID          string `json:"id"`
	Object      string `json:"object,omitempty"`
	Created     int64  `json:"created,omitempty"`
	OwnedBy     string `json:"owned_by,omitempty"`
	Type        string `json:"type,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type ModelListResponse struct {
	Object string          `json:"object"`
	Data   []ModelListItem `json:"data"`
}

type ModelListItem struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created"`
	OwnedBy     string `json:"owned_by"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	Name        string `json:"name"`
}

type ModelV2Item struct {
	ID                          string   `json:"id"`
	Object                      string   `json:"object"`
	OwnedBy                     string   `json:"owned_by"`
	Model                       string   `json:"model"`
	Name                        string   `json:"name"`
	Description                 string   `json:"description,omitempty"`
	ContextWindow               int      `json:"context_window,omitempty"`
	AutoCompactThresholdPercent *int     `json:"auto_compact_threshold_percent,omitempty"`
	Temperature                 *float64 `json:"temperature,omitempty"`
	TopP                        *float64 `json:"top_p,omitempty"`
	APIBackend                  string   `json:"api_backend,omitempty"`
	SupportsBackendSearch       bool     `json:"supports_backend_search,omitempty"`
	AgentType                   string   `json:"agent_type,omitempty"`
}

type ModelV2ListResponse struct {
	Object string        `json:"object"`
	Data   []ModelV2Item `json:"data"`
}

var DefaultModels = []Model{
	{ID: "grok-build", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok Build", Name: "Grok Build"},
	{ID: "grok-build-0.1", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok Build 0.1", Name: "Grok Build 0.1"},
	{ID: "grok-composer-2.5-fast", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Composer 2.5", Name: "Composer 2.5"},
	{ID: "grok-4.3", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.3", Name: "Grok 4.3"},
	{ID: "grok-4.3-fast", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.3 Fast", Name: "Grok 4.3 Fast"},
	{ID: "grok-4.3-mini", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.3 Mini", Name: "Grok 4.3 Mini"},
	{ID: "grok-4.3-mini-fast", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.3 Mini Fast", Name: "Grok 4.3 Mini Fast"},
	{ID: "grok-4.20", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.20", Name: "Grok 4.20"},
	{ID: "grok-4.20-fast", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.20 Fast", Name: "Grok 4.20 Fast"},
	{ID: "grok-4.20-mini", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.20 Mini", Name: "Grok 4.20 Mini"},
	{ID: "grok-4.20-mini-fast", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.20 Mini Fast", Name: "Grok 4.20 Mini Fast"},
	{ID: "grok-4.20-0309-reasoning", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.20 Reasoning", Name: "Grok 4.20 Reasoning"},
	{ID: "grok-4.20-0309-non-reasoning", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.20 Non Reasoning", Name: "Grok 4.20 Non Reasoning"},
	{ID: "grok-4.20-multi-agent-0309", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.20 Multi Agent", Name: "Grok 4.20 Multi Agent"},
	{ID: "grok-code-fast-1", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok Code Fast 1", Name: "Grok Code Fast 1"},
	{ID: "grok-3-mini", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 3 Mini", Name: "Grok 3 Mini"},
	{ID: "grok-imagine-image", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok Imagine Image", Name: "Grok Imagine Image"},
	{ID: "grok-imagine-image-quality", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok Imagine Image Quality", Name: "Grok Imagine Image Quality"},
	{ID: "grok-imagine-video", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok Imagine Video", Name: "Grok Imagine Video"},
	{ID: "grok-imagine-video-1.5-preview", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok Imagine Video 1.5 Preview", Name: "Grok Imagine Video 1.5 Preview"},
}

func DefaultModelsV2Response() ModelV2ListResponse {
	autoCompact := 80
	tempBuild := 0.7
	topPBuild := 0.95
	return ModelV2ListResponse{
		Object: "list",
		Data: []ModelV2Item{
			{
				ID:            "grok-composer-2.5-fast",
				Object:        "model",
				OwnedBy:       "xAI",
				Model:         "grok-composer-2.5-fast",
				Name:          "Composer 2.5",
				Description:   "Cursor's latest coding model",
				ContextWindow: 200000,
				APIBackend:    "responses",
				AgentType:     "cursor",
			},
			{
				ID:                          "grok-build",
				Object:                      "model",
				OwnedBy:                     "xAI",
				Model:                       "grok-build",
				Name:                        "Grok Build",
				Description:                 "Best for advanced coding tasks",
				ContextWindow:               512000,
				AutoCompactThresholdPercent: &autoCompact,
				Temperature:                 &tempBuild,
				TopP:                        &topPBuild,
				APIBackend:                  "responses",
				SupportsBackendSearch:       true,
				AgentType:                   "grok-build-plan",
			},
		},
	}
}

func DefaultModelIDs() []string {
	out := make([]string, 0, len(DefaultModels))
	for _, model := range DefaultModels {
		out = append(out, model.ID)
	}
	return out
}

func DefaultModelMapping() map[string]string {
	mapping := make(map[string]string, len(DefaultModels)+5)
	for _, model := range DefaultModels {
		mapping[model.ID] = model.ID
	}
	mapping["grok"] = "grok-4.3"
	mapping["grok-latest"] = "grok-4.3"
	mapping["grok-build"] = "grok-build-0.1"
	mapping["grok-4.20-reasoning"] = "grok-4.20-0309-reasoning"
	mapping["grok-4.20-non-reasoning"] = "grok-4.20-0309-non-reasoning"
	return mapping
}

func ParseModelList(body []byte) ([]Model, error) {
	var resp ModelListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse xAI models response: %w", err)
	}
	models := make([]Model, 0, len(resp.Data))
	seen := make(map[string]struct{}, len(resp.Data))
	for _, item := range resp.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		displayName := strings.TrimSpace(item.DisplayName)
		if displayName == "" {
			displayName = strings.TrimSpace(item.Name)
		}
		if displayName == "" {
			displayName = id
		}
		model := Model{
			ID:          id,
			Object:      strings.TrimSpace(item.Object),
			Created:     item.Created,
			OwnedBy:     strings.TrimSpace(item.OwnedBy),
			Type:        strings.TrimSpace(item.Type),
			DisplayName: displayName,
			Name:        displayName,
		}
		if model.Object == "" {
			model.Object = "model"
		}
		if model.OwnedBy == "" {
			model.OwnedBy = "xai"
		}
		if model.Type == "" {
			model.Type = "model"
		}
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})
	return models, nil
}

func ModelsFromIDs(ids []string) []Model {
	normalized := normalizeModelIDs(ids)
	if len(normalized) == 0 {
		return nil
	}
	defaultByID := make(map[string]Model, len(DefaultModels))
	for _, model := range DefaultModels {
		defaultByID[model.ID] = model
	}
	models := make([]Model, 0, len(normalized))
	for _, id := range normalized {
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
	return models
}

func normalizeModelIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

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

var DefaultModels = []Model{
	{ID: "grok-4.3", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.3", Name: "Grok 4.3"},
	{ID: "grok-4.3-fast", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.3 Fast", Name: "Grok 4.3 Fast"},
	{ID: "grok-4.3-mini", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.3 Mini", Name: "Grok 4.3 Mini"},
	{ID: "grok-4.3-mini-fast", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.3 Mini Fast", Name: "Grok 4.3 Mini Fast"},
	{ID: "grok-4.20", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.20", Name: "Grok 4.20"},
	{ID: "grok-4.20-fast", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.20 Fast", Name: "Grok 4.20 Fast"},
	{ID: "grok-4.20-mini", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.20 Mini", Name: "Grok 4.20 Mini"},
	{ID: "grok-4.20-mini-fast", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 4.20 Mini Fast", Name: "Grok 4.20 Mini Fast"},
	{ID: "grok-code-fast-1", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok Code Fast 1", Name: "Grok Code Fast 1"},
	{ID: "grok-3-mini", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok 3 Mini", Name: "Grok 3 Mini"},
	{ID: "grok-imagine-image", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok Imagine Image", Name: "Grok Imagine Image"},
	{ID: "grok-imagine-image-quality", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok Imagine Image Quality", Name: "Grok Imagine Image Quality"},
	{ID: "grok-imagine-video", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok Imagine Video", Name: "Grok Imagine Video"},
	{ID: "grok-imagine-video-1.5-preview", Object: "model", OwnedBy: "xai", Type: "model", DisplayName: "Grok Imagine Video 1.5 Preview", Name: "Grok Imagine Video 1.5 Preview"},
}

func DefaultModelIDs() []string {
	out := make([]string, 0, len(DefaultModels))
	for _, model := range DefaultModels {
		out = append(out, model.ID)
	}
	return out
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

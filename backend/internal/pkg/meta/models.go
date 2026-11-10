package meta

import (
	"encoding/json"
	"errors"
	"strings"
)

type Model struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Type        string `json:"type,omitempty"`
	Created     int64  `json:"created,omitempty"`
	OwnedBy     string `json:"owned_by"`
	DisplayName string `json:"display_name,omitempty"`
}

var defaultModelIDs = []string{
	"muse-spark-1.3",
	"muse-spark-1.3-contributor",
	"muse-spark-1.2",
	"muse-spark-1.2-contributor",
	"muse-spark-1.1",
	"muse-wonder-1.0",
	"muse-wonder-1.0-contributor",
	"muse-wonder-full-1.0",
	"muse-wonder-full-1.0-contributor",
}

func DefaultModelIDs() []string {
	return append([]string(nil), defaultModelIDs...)
}

func DefaultModels() []Model {
	models := make([]Model, 0, len(defaultModelIDs))
	for _, id := range defaultModelIDs {
		models = append(models, NewModel(id))
	}
	return models
}

func NewModel(id string) Model {
	return Model{
		ID:          id,
		Object:      "model",
		Type:        "model",
		OwnedBy:     "meta",
		DisplayName: id,
	}
}

func ParseModelList(body []byte) ([]Model, error) {
	if len(body) == 0 {
		return nil, errors.New("meta: empty model list body")
	}
	var payload struct {
		Data []struct {
			ID          string `json:"id"`
			Object      string `json:"object"`
			Type        string `json:"type"`
			Created     int64  `json:"created"`
			OwnedBy     string `json:"owned_by"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(payload.Data))
	models := make([]Model, 0, len(payload.Data))
	for _, entry := range payload.Data {
		id := strings.TrimSpace(entry.ID)
		if !IsMuseModelID(id) {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		model := NewModel(id)
		if object := strings.TrimSpace(entry.Object); object != "" {
			model.Object = object
		}
		if entryType := strings.TrimSpace(entry.Type); entryType != "" {
			model.Type = entryType
		}
		model.Created = entry.Created
		if ownedBy := strings.TrimSpace(entry.OwnedBy); ownedBy != "" {
			model.OwnedBy = ownedBy
		}
		if displayName := strings.TrimSpace(entry.DisplayName); displayName != "" {
			model.DisplayName = displayName
		}
		models = append(models, model)
	}
	if len(models) == 0 {
		return nil, errors.New("meta: upstream model list has no muse models")
	}
	return models, nil
}

func IsMuseModelID(id string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(id)), "muse-")
}

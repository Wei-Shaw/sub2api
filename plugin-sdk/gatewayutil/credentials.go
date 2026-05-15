package gatewayutil

import (
	"encoding/json"
	"strings"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// CredStr extracts a string credential value from a map, trimming whitespace.
// Returns "" if the map is nil or the key is absent/non-string.
func CredStr(creds map[string]any, key string) string {
	if creds == nil {
		return ""
	}
	v, _ := creds[key].(string)
	return strings.TrimSpace(v)
}

// ExtractModelMapping parses credentials JSON and returns the
// model_mapping as map[string]string. Returns nil if absent or empty.
func ExtractModelMapping(credentialsJSON []byte) map[string]string {
	if len(credentialsJSON) == 0 {
		return nil
	}
	var creds map[string]any
	if err := json.Unmarshal(credentialsJSON, &creds); err != nil {
		return nil
	}
	raw, ok := creds["model_mapping"].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	result := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// ModelsFromMapping builds the model list from mapping keys. If a key
// matches a known model in defaultModelIndex, the default display name
// is used; otherwise the model ID itself becomes the display name.
//
// The defaultModelIndex parameter is the per-plugin index of known
// models (built from the plugin's defaultModels list).
func ModelsFromMapping(
	mapping map[string]string,
	defaultModelIndex map[string]*pb.AvailableModelInfo,
) []*pb.AvailableModelInfo {
	models := make([]*pb.AvailableModelInfo, 0, len(mapping))
	for requestedModel := range mapping {
		if dm, ok := defaultModelIndex[requestedModel]; ok {
			models = append(models, dm)
			continue
		}
		models = append(models, &pb.AvailableModelInfo{
			ModelId:     requestedModel,
			DisplayName: requestedModel,
			Available:   true,
		})
	}
	return models
}

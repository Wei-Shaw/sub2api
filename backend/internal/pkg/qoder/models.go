package qoder

// ModelInfo describes a Qoder model entry for OpenAI-compatible /v1/models
// listings.
type ModelInfo struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created"`
	OwnedBy     string `json:"owned_by"`
	DisplayName string `json:"display_name,omitempty"`
}

// DirectSupportedModelIDs enumerates the model aliases accepted by the
// stateless direct model server. The Cloud Agents API advertises additional
// aliases (e.g. gm51model, cmodel, qmodel_latest) that the direct server
// rejects with "Unsupported model".
var DirectSupportedModelIDs = map[string]bool{
	"auto":        true,
	"ultimate":    true,
	"performance": true,
	"efficient":   true,
	"lite":        true,
	"qmodel":      true,
	"kmodel":      true,
	"dmodel":      true,
	"mmodel":      true,
}

// DefaultModels is the fallback model catalog advertised on /v1/models when an
// account has no explicit model_mapping. The identifiers mirror the Qoder Cloud
// Agents model aliases.
var DefaultModels = []ModelInfo{
	{ID: "auto", Object: "model", Created: 1735689600, OwnedBy: "qoder"},
	{ID: "ultimate", Object: "model", Created: 1735689600, OwnedBy: "qoder"},
	{ID: "performance", Object: "model", Created: 1735689600, OwnedBy: "qoder"},
	{ID: "efficient", Object: "model", Created: 1735689600, OwnedBy: "qoder"},
	{ID: "lite", Object: "model", Created: 1735689600, OwnedBy: "qoder"},
	{ID: "qmodel", Object: "model", Created: 1735689600, OwnedBy: "qoder"},
	{ID: "qmodel_latest", Object: "model", Created: 1735689600, OwnedBy: "qoder"},
	{ID: "qmodel_preview", Object: "model", Created: 1735689600, OwnedBy: "qoder"},
	{ID: "kmodel", Object: "model", Created: 1735689600, OwnedBy: "qoder"},
	{ID: "kmodel_latest", Object: "model", Created: 1735689600, OwnedBy: "qoder"},
	{ID: "cmodel", Object: "model", Created: 1735689600, OwnedBy: "qoder"},
	{ID: "gm51model", Object: "model", Created: 1735689600, OwnedBy: "qoder"},
	{ID: "dmodel", Object: "model", Created: 1735689600, OwnedBy: "qoder"},
	{ID: "dfmodel", Object: "model", Created: 1735689600, OwnedBy: "qoder"},
	{ID: "mmodel", Object: "model", Created: 1735689600, OwnedBy: "qoder"},
}

// DefaultModelIDs returns the default model ID list.
func DefaultModelIDs() []string {
	ids := make([]string, len(DefaultModels))
	for i, m := range DefaultModels {
		ids[i] = m.ID
	}
	return ids
}

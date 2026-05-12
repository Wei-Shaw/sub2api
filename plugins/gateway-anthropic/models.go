package main

import (
	"context"
	"encoding/json"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// defaultTestModel is the default model for testing Anthropic accounts.
const defaultTestModel = "claude-sonnet-4-5-20250929"

// defaultModels is the canonical set of Claude models returned when no
// model_mapping is configured. Keep in sync with upstream releases.
var defaultModels = []*pb.AvailableModelInfo{
	{ModelId: "claude-opus-4-7", DisplayName: "Claude Opus 4.7", Available: true},
	{ModelId: "claude-opus-4-6", DisplayName: "Claude Opus 4.6", Available: true},
	{ModelId: "claude-opus-4-5-20251101", DisplayName: "Claude Opus 4.5", Available: true},
	{ModelId: "claude-sonnet-4-6", DisplayName: "Claude Sonnet 4.6", Available: true},
	{ModelId: "claude-sonnet-4-5-20250929", DisplayName: "Claude Sonnet 4.5", Available: true},
	{ModelId: "claude-haiku-4-5-20251001", DisplayName: "Claude Haiku 4.5", Available: true},
}

// defaultModelIndex maps model ID to its entry in defaultModels for O(1)
// lookup when resolving model_mapping keys against known models.
var defaultModelIndex map[string]*pb.AvailableModelInfo

func init() {
	defaultModelIndex = make(map[string]*pb.AvailableModelInfo, len(defaultModels))
	for _, m := range defaultModels {
		defaultModelIndex[m.ModelId] = m
	}
}

// GetAvailableModels returns the models available for an Anthropic account.
//
// Logic mirrors the host's account_handler GetAvailableModels for Anthropic:
//  1. For OAuth / setup-token accounts: return the full default model set.
//  2. If a model_mapping exists in credentials, return only the mapped
//     models (enriched with display names from defaults when possible).
//  3. Otherwise return the full default model set.
func (s *accountPlatformServer) GetAvailableModels(
	ctx context.Context,
	req *pb.GetAvailableModelsRequest,
) (*pb.GetAvailableModelsResponse, error) {
	accountType := req.GetAccountType()

	// OAuth, setup-token, and Vertex accounts always get the full model set.
	if accountType == accountTypeOAuth || accountType == accountTypeSetupToken || accountType == accountTypeServiceAccount {
		return &pb.GetAvailableModelsResponse{Models: defaultModels}, nil
	}

	mapping := extractModelMapping(req.GetCredentialsJson())
	if len(mapping) == 0 {
		return &pb.GetAvailableModelsResponse{Models: defaultModels}, nil
	}

	return &pb.GetAvailableModelsResponse{
		Models: modelsFromMapping(mapping),
	}, nil
}

// extractModelMapping parses credentials JSON and returns the
// model_mapping as map[string]string. Returns nil if absent or empty.
func extractModelMapping(credentialsJSON []byte) map[string]string {
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

// modelsFromMapping builds the model list from mapping keys. If a key
// matches a known default model, the default display name is used;
// otherwise the model ID itself becomes the display name.
func modelsFromMapping(mapping map[string]string) []*pb.AvailableModelInfo {
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

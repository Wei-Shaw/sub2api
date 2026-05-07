package main

import (
	"context"
	"encoding/json"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// defaultModels is the canonical set of Gemini models returned when no
// model_mapping is configured. Keep in sync with geminicli.DefaultModels.
var defaultModels = []*pb.AvailableModelInfo{
	{ModelId: "gemini-2.0-flash", DisplayName: "Gemini 2.0 Flash", Available: true},
	{ModelId: "gemini-2.5-flash", DisplayName: "Gemini 2.5 Flash", Available: true},
	{ModelId: "gemini-2.5-flash-image", DisplayName: "Gemini 2.5 Flash Image", Available: true},
	{ModelId: "gemini-2.5-pro", DisplayName: "Gemini 2.5 Pro", Available: true},
	{ModelId: "gemini-3-flash-preview", DisplayName: "Gemini 3 Flash Preview", Available: true},
	{ModelId: "gemini-3-pro-preview", DisplayName: "Gemini 3 Pro Preview", Available: true},
	{ModelId: "gemini-3.1-pro-preview", DisplayName: "Gemini 3.1 Pro Preview", Available: true},
	{ModelId: "gemini-3.1-flash-image", DisplayName: "Gemini 3.1 Flash Image", Available: true},
}

// defaultModelIndex maps model ID to its entry in defaultModels for O(1) lookup.
var defaultModelIndex map[string]*pb.AvailableModelInfo

func init() {
	defaultModelIndex = make(map[string]*pb.AvailableModelInfo, len(defaultModels))
	for _, m := range defaultModels {
		defaultModelIndex[m.ModelId] = m
	}
}

// GetAvailableModels returns the models available for a Gemini account.
//
// Logic mirrors the host's account_handler GetAvailableModels for Gemini:
//  1. OAuth accounts always return the full default model set.
//  2. If a model_mapping exists in credentials (API key / service account),
//     return only the mapped models with display names from defaults.
//  3. Otherwise return the full default model set.
func (s *accountPlatformServer) GetAvailableModels(
	_ context.Context,
	req *pb.GetAvailableModelsRequest,
) (*pb.GetAvailableModelsResponse, error) {
	// OAuth accounts always get the full default set.
	if req.GetAccountType() == accountTypeOAuth {
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

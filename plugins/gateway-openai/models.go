package main

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// defaultModels is the canonical set of OpenAI models returned when no
// model_mapping is configured. Keep in sync with upstream releases.
var defaultModels = []*pb.AvailableModelInfo{
	{ModelId: "gpt-5.5", DisplayName: "GPT-5.5", Available: true},
	{ModelId: "gpt-5.4", DisplayName: "GPT-5.4", Available: true},
	{ModelId: "gpt-5.4-mini", DisplayName: "GPT-5.4 Mini", Available: true},
	{ModelId: "gpt-5.3-codex", DisplayName: "GPT-5.3 Codex", Available: true},
	{ModelId: "gpt-5.3-codex-spark", DisplayName: "GPT-5.3 Codex Spark", Available: true},
	{ModelId: "gpt-5.2", DisplayName: "GPT-5.2", Available: true},
	{ModelId: "gpt-image-1", DisplayName: "GPT Image 1", Available: true},
	{ModelId: "gpt-image-1.5", DisplayName: "GPT Image 1.5", Available: true},
	{ModelId: "gpt-image-2", DisplayName: "GPT Image 2", Available: true},
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

// GetAvailableModels returns the models available for an OpenAI account.
//
// Logic mirrors the host's account_handler GetAvailableModels for OpenAI:
//  1. If passthrough is enabled (extra.openai_passthrough or legacy
//     extra.openai_oauth_passthrough), return the full default model set.
//  2. If a model_mapping exists in credentials, return only the mapped
//     models (enriched with display names from defaults when possible).
//  3. Otherwise return the full default model set.
func (s *accountPlatformServer) GetAvailableModels(
	ctx context.Context,
	req *pb.GetAvailableModelsRequest,
) (*pb.GetAvailableModelsResponse, error) {
	if isPassthroughEnabled(req.GetExtraJson()) {
		return &pb.GetAvailableModelsResponse{Models: defaultModels}, nil
	}

	mapping := gatewayutil.ExtractModelMapping(req.GetCredentialsJson())
	if len(mapping) == 0 {
		return &pb.GetAvailableModelsResponse{Models: defaultModels}, nil
	}

	return &pb.GetAvailableModelsResponse{
		Models: gatewayutil.ModelsFromMapping(mapping, defaultModelIndex),
	}, nil
}

// isPassthroughEnabled checks whether the account has passthrough mode
// enabled via extra.openai_passthrough or the legacy
// extra.openai_oauth_passthrough flag.
func isPassthroughEnabled(extraJSON []byte) bool {
	if len(extraJSON) == 0 {
		return false
	}
	var extra map[string]any
	if err := json.Unmarshal(extraJSON, &extra); err != nil {
		return false
	}
	if enabled, ok := extra["openai_passthrough"].(bool); ok {
		return enabled
	}
	if enabled, ok := extra["openai_oauth_passthrough"].(bool); ok {
		return enabled
	}
	return false
}

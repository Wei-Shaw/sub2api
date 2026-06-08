package main

import (
	"context"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/gatewayutil"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// defaultModels is the canonical set of Antigravity models (Claude + Gemini)
// returned when no model_mapping is configured. Keep in sync with
// backend/internal/pkg/antigravity/claude_types.go DefaultModels().
var defaultModels = []*pb.AvailableModelInfo{
	// Claude models
	{ModelId: "claude-opus-4-7", DisplayName: "Claude Opus 4.7", Available: true},
	{ModelId: "claude-opus-4-6-thinking", DisplayName: "Claude Opus 4.6 Thinking", Available: true},
	{ModelId: "claude-opus-4-6", DisplayName: "Claude Opus 4.6", Available: true},
	{ModelId: "claude-sonnet-4-6", DisplayName: "Claude Sonnet 4.6", Available: true},
	{ModelId: "claude-sonnet-4-5", DisplayName: "Claude Sonnet 4.5", Available: true},
	{ModelId: "claude-sonnet-4-5-thinking", DisplayName: "Claude Sonnet 4.5 Thinking", Available: true},
	// Gemini 2.5 models
	{ModelId: "gemini-2.5-flash", DisplayName: "Gemini 2.5 Flash", Available: true},
	{ModelId: "gemini-2.5-flash-image", DisplayName: "Gemini 2.5 Flash Image", Available: true},
	{ModelId: "gemini-2.5-flash-lite", DisplayName: "Gemini 2.5 Flash Lite", Available: true},
	{ModelId: "gemini-2.5-flash-thinking", DisplayName: "Gemini 2.5 Flash Thinking", Available: true},
	{ModelId: "gemini-2.5-pro", DisplayName: "Gemini 2.5 Pro", Available: true},
	// Gemini 3 models
	{ModelId: "gemini-3-flash", DisplayName: "Gemini 3 Flash", Available: true},
	{ModelId: "gemini-3-pro-low", DisplayName: "Gemini 3 Pro Low", Available: true},
	{ModelId: "gemini-3-pro-high", DisplayName: "Gemini 3 Pro High", Available: true},
	// Gemini 3.1 models
	{ModelId: "gemini-3.1-pro-low", DisplayName: "Gemini 3.1 Pro Low", Available: true},
	{ModelId: "gemini-3.1-pro-high", DisplayName: "Gemini 3.1 Pro High", Available: true},
	{ModelId: "gemini-3.1-flash-image", DisplayName: "Gemini 3.1 Flash Image", Available: true},
}

// defaultModelIndex maps model ID to its entry for O(1) lookup.
var defaultModelIndex map[string]*pb.AvailableModelInfo

func init() {
	defaultModelIndex = make(map[string]*pb.AvailableModelInfo, len(defaultModels))
	for _, m := range defaultModels {
		defaultModelIndex[m.ModelId] = m
	}
}

// GetAvailableModels returns the models available for an Antigravity account.
//
// Logic mirrors the host's account_handler GetAvailableModels for Antigravity:
//  1. If a model_mapping exists in credentials, return only the mapped models
//     (enriched with display names from defaults when possible).
//  2. Otherwise return the full default model set (Claude + Gemini).
func (s *accountPlatformServer) GetAvailableModels(
	ctx context.Context,
	req *pb.GetAvailableModelsRequest,
) (*pb.GetAvailableModelsResponse, error) {
	mapping := gatewayutil.ExtractModelMapping(req.GetCredentialsJson())
	if len(mapping) == 0 {
		return &pb.GetAvailableModelsResponse{Models: defaultModels}, nil
	}

	return &pb.GetAvailableModelsResponse{
		Models: gatewayutil.ModelsFromMapping(mapping, defaultModelIndex),
	}, nil
}

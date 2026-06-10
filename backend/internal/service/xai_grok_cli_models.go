package service

import "github.com/Wei-Shaw/sub2api/internal/pkg/xai"

func isXAIGrokCLIModel(platform, model string) bool {
	return platform == PlatformXAI && xai.IsGrokCLIModel(model)
}
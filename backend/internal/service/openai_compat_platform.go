package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

var openAICompatiblePlatformSet = map[string]struct{}{
	PlatformOpenAI:   {},
	PlatformGrok:     {},
	PlatformDeepSeek: {},
	PlatformGLM:      {},
	PlatformZAI:      {},
	PlatformMiniMax:  {},
	PlatformOpenCode: {},
}

func IsOpenAICompatiblePlatform(platform string) bool {
	_, ok := openAICompatiblePlatformSet[strings.ToLower(strings.TrimSpace(platform))]
	return ok
}

func IsOpenAIChatCompletionsOnlyPlatform(platform string) bool {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case PlatformDeepSeek, PlatformGLM, PlatformZAI, PlatformMiniMax, PlatformOpenCode:
		return true
	default:
		return false
	}
}

func ShouldUseOpenAICompatibleChatCompletions(account *Account) bool {
	if account == nil || account.Type != AccountTypeAPIKey {
		return false
	}
	if IsOpenAIChatCompletionsOnlyPlatform(account.Platform) {
		return true
	}
	return !openai_compat.ShouldUseResponsesAPI(account.Extra)
}

func DefaultOpenAICompatibleBaseURL(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case PlatformGrok:
		return xai.DefaultBaseURL
	case PlatformDeepSeek:
		return "https://api.deepseek.com"
	case PlatformGLM:
		return "https://api.z.ai/api/paas/v4"
	case PlatformZAI:
		return "https://chat.z.ai/api"
	case PlatformMiniMax:
		return "https://api.minimax.io/v1"
	case PlatformOpenCode:
		return "https://opencode.ai/zen/go/v1"
	case PlatformOpenAI:
		fallthrough
	default:
		return "https://api.openai.com"
	}
}

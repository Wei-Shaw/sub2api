package service

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/bailian"
)

// bailianAPIRoot resolves and validates the DashScope API root for an account.
// The same root serves both the OpenAI-compatible text endpoint and the native
// async video task endpoints.
func (s *OpenAIGatewayService) bailianAPIRoot(account *Account) (string, error) {
	baseURL := account.GetBailianBaseURL()
	if baseURL == "" {
		baseURL = bailian.DefaultBaseURL
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base_url: %w", err)
	}
	return bailian.NormalizeAPIRoot(validatedURL), nil
}

func (s *OpenAIGatewayService) bailianChatCompletionsURL(account *Account) (string, error) {
	root, err := s.bailianAPIRoot(account)
	if err != nil {
		return "", err
	}
	return bailian.ChatCompletionsURL(root), nil
}

func (s *OpenAIGatewayService) bailianVideoSynthesisURL(account *Account) (string, error) {
	root, err := s.bailianAPIRoot(account)
	if err != nil {
		return "", err
	}
	return bailian.VideoSynthesisURL(root), nil
}

func (s *OpenAIGatewayService) bailianTaskURL(account *Account, taskID string) (string, error) {
	root, err := s.bailianAPIRoot(account)
	if err != nil {
		return "", err
	}
	return bailian.TaskURL(root, taskID), nil
}

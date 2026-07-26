package service

import "github.com/Wei-Shaw/sub2api/internal/pkg/openai"

func buildOpenAIEndpointURL(base string, endpoint string) string {
	return openai.BuildEndpointURL(base, endpoint)
}

func buildOpenAIResponsesInputTokensURL(base string) string {
	return openai.BuildResponsesInputTokensURL(base)
}

func openAIBaseURLHasVersionSuffix(raw string) bool {
	return openai.BaseURLHasVersionSuffix(raw)
}

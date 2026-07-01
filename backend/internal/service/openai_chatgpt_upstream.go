package service

import (
	"net/url"
	"strings"
)

func (s *OpenAIGatewayService) openAIChatGPTCodexResponsesURL() string {
	if s == nil || s.cfg == nil {
		return chatgptCodexURL
	}
	return buildOpenAIChatGPTCodexResponsesURL(s.cfg.Gateway.OpenAIChatGPTBaseURL)
}

func buildOpenAIChatGPTCodexResponsesURL(base string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if trimmed == "" {
		return chatgptCodexURL
	}
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasSuffix(lower, "/backend-api/codex/responses"):
		return trimmed
	case strings.HasSuffix(lower, "/backend-api/codex"):
		return trimmed + "/responses"
	case strings.HasSuffix(lower, "/responses"):
		return trimmed
	default:
		return trimmed + "/backend-api/codex/responses"
	}
}

func shouldSetOpenAIChatGPTHostHeader(u *url.URL) bool {
	if u == nil {
		return true
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	return host == "chatgpt.com" || strings.HasSuffix(host, ".chatgpt.com")
}

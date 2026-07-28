//go:build integration

package service

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAIAgentLiveProviderStreamingCompatibility(t *testing.T) {
	tests := []struct {
		name      string
		protocol  string
		envPrefix string
		thinking  string
	}{
		{name: "chat_completions", protocol: agentProtocolChatCompletions, envPrefix: "AI_AGENT_COMPAT_CHAT"},
		{name: "responses", protocol: agentProtocolResponses, envPrefix: "AI_AGENT_COMPAT_RESPONSES"},
		{name: "messages", protocol: agentProtocolMessages, envPrefix: "AI_AGENT_COMPAT_MESSAGES"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			baseURL := strings.TrimSpace(os.Getenv(test.envPrefix + "_BASE_URL"))
			apiKey := strings.TrimSpace(os.Getenv(test.envPrefix + "_API_KEY"))
			model := strings.TrimSpace(os.Getenv(test.envPrefix + "_MODEL"))
			if baseURL == "" || apiKey == "" || model == "" {
				t.Skipf("set %s_BASE_URL, %s_API_KEY, and %s_MODEL to run the live compatibility check", test.envPrefix, test.envPrefix, test.envPrefix)
			}
			service := &AIAgentService{client: newAgentHTTPClient()}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			var streamed strings.Builder
			message, err := service.complete(ctx, AIAgentConfig{
				BaseURL: baseURL, Model: model, Protocol: test.protocol, ThinkingMode: test.thinking,
			}, apiKey, []agentModelMessage{{Role: "user", Content: "Reply with exactly: compatibility-ok"}}, func(delta string) {
				_, _ = streamed.WriteString(delta)
			})
			if err != nil {
				t.Fatalf("live %s stream failed: %v", test.protocol, err)
			}
			if strings.TrimSpace(streamed.String()) == "" || strings.TrimSpace(modelMessageText(message.Content)) == "" {
				t.Fatalf("live %s stream returned no visible text", test.protocol)
			}
		})
	}
}

func newAgentHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 90 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

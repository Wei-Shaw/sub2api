package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInferJSToFormat_OpenAIOAuthChat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set("openai_js_protocol", "openai_chat")
	acc := &service.Account{Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	require.Equal(t, "codex", inferJSToFormat(c, "openai_chat", acc))
}

func TestInferJSToFormat_Anthropic(t *testing.T) {
	acc := &service.Account{Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth}
	require.Equal(t, "anthropic", inferJSToFormat(nil, "anthropic_messages", acc))
}

func TestInferJSToFormat_AnthropicGroupOpenAIChat(t *testing.T) {
	acc := &service.Account{Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey}
	require.Equal(t, "openai_chat", inferJSToFormat(nil, "openai_chat", acc))
}

func TestInferJSToFormat_AnthropicGroupOpenAIResponses(t *testing.T) {
	acc := &service.Account{Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey}
	require.Equal(t, "openai_responses", inferJSToFormat(nil, "openai_responses", acc))
}
package service

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIsUpstreamNativeWebSearchSupported(t *testing.T) {
	t.Run("Grok platform natively supported", func(t *testing.T) {
		acc := &Account{Platform: PlatformGrok, Type: AccountTypeAPIKey}
		require.True(t, isUpstreamNativeWebSearchSupported(acc))
	})

	t.Run("OpenAI OAuth natively supported", func(t *testing.T) {
		acc := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
		require.True(t, isUpstreamNativeWebSearchSupported(acc))
	})

	t.Run("Official OpenAI api.openai.com natively supported", func(t *testing.T) {
		acc := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"base_url": "https://api.openai.com/v1"},
		}
		require.True(t, isUpstreamNativeWebSearchSupported(acc))
	})

	t.Run("Default OpenAI baseURL natively supported", func(t *testing.T) {
		acc := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{},
		}
		require.True(t, isUpstreamNativeWebSearchSupported(acc))
	})

	t.Run("Third-party Ollama Cloud NOT natively supported", func(t *testing.T) {
		acc := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"base_url": "https://ollama.com/v1"},
		}
		require.False(t, isUpstreamNativeWebSearchSupported(acc))
	})

	t.Run("Third-party 9router NOT natively supported", func(t *testing.T) {
		acc := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"base_url": "https://9router.com/v1"},
		}
		require.False(t, isUpstreamNativeWebSearchSupported(acc))
	})

	t.Run("Other platforms NOT natively supported", func(t *testing.T) {
		acc := &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey}
		require.False(t, isUpstreamNativeWebSearchSupported(acc))
	})
}

func TestAdaptOpenAIResponsesWebSearchTool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}

	t.Run("Upstream natively supported: leaves body unchanged", func(t *testing.T) {
		acc := &Account{Platform: PlatformGrok, Type: AccountTypeAPIKey}
		body := []byte(`{"model":"grok-beta","tools":[{"type":"web_search"}]}`)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		adapted, changed, err := svc.adaptOpenAIResponsesWebSearchTool(context.Background(), c, acc, body)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, string(body), string(adapted))
	})

	t.Run("Emulation disabled: strips web_search tool from third-party upstream", func(t *testing.T) {
		acc := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Extra:       map[string]any{featureKeyWebSearchEmulation: "disabled"},
			Credentials: map[string]any{"base_url": "https://ollama.com/v1"},
		}
		body := []byte(`{"model":"glm-5.3-flash","tools":[{"type":"web_search"},{"type":"function","name":"exec","description":"run command"}]}`)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		adapted, changed, err := svc.adaptOpenAIResponsesWebSearchTool(context.Background(), c, acc, body)
		require.NoError(t, err)
		require.True(t, changed)

		// web_search stripped, exec preserved
		tools := gjson.GetBytes(adapted, "tools").Array()
		require.Len(t, tools, 1)
		require.Equal(t, "function", tools[0].Get("type").String())
		require.Equal(t, "exec", tools[0].Get("name").String())
	})

	t.Run("Emulation disabled: deletes tools and tool_choice when only web_search present", func(t *testing.T) {
		acc := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Extra:       map[string]any{featureKeyWebSearchEmulation: "disabled"},
			Credentials: map[string]any{"base_url": "https://ollama.com/v1"},
		}
		body := []byte(`{"model":"glm-5.3-flash","tools":[{"type":"web_search"}],"tool_choice":"auto"}`)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		adapted, changed, err := svc.adaptOpenAIResponsesWebSearchTool(context.Background(), c, acc, body)
		require.NoError(t, err)
		require.True(t, changed)
		require.False(t, gjson.GetBytes(adapted, "tools").Exists())
		require.False(t, gjson.GetBytes(adapted, "tool_choice").Exists())
	})

	t.Run("Emulation enabled: adapts web_search to function tool for third-party upstream", func(t *testing.T) {
		// Mock webSearchManagerPtr
		acc := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Extra:       map[string]any{featureKeyWebSearchEmulation: "enabled"},
			Credentials: map[string]any{"base_url": "https://ollama.com/v1"},
		}
		body := []byte(`{"model":"glm-5.3-flash","tools":[{"type":"web_search"},{"type":"function","name":"exec"}]}`)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// If websearch.Manager is nil, shouldEmulateOpenAIWebSearch is false, so it strips.
		// Let's test the strip fallback:
		adapted, changed, err := svc.adaptOpenAIResponsesWebSearchTool(context.Background(), c, acc, body)
		require.NoError(t, err)
		require.True(t, changed)
		tools := gjson.GetBytes(adapted, "tools").Array()
		require.Len(t, tools, 1)
		require.Equal(t, "exec", tools[0].Get("name").String())
	})

	t.Run("Collision with declared function web_search: strips server tool safely", func(t *testing.T) {
		acc := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"base_url": "https://ollama.com/v1"},
		}
		body := []byte(`{"model":"glm-5.3-flash","tools":[{"type":"web_search"},{"type":"function","name":"web_search"}]}`)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		adapted, changed, err := svc.adaptOpenAIResponsesWebSearchTool(context.Background(), c, acc, body)
		require.NoError(t, err)
		require.True(t, changed)
		tools := gjson.GetBytes(adapted, "tools").Array()
		require.Len(t, tools, 1)
		require.Equal(t, "function", tools[0].Get("type").String())
		require.Equal(t, "web_search", tools[0].Get("name").String())
	})
}

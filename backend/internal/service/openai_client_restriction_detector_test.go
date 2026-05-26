package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newCodexDetectorTestContext(ua string, originator string) *gin.Context {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if ua != "" {
		c.Request.Header.Set("User-Agent", ua)
	}
	if originator != "" {
		c.Request.Header.Set("originator", originator)
	}
	return c
}

func TestOpenAICodexClientRestrictionDetector_Detect(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("未开启开关时绕过", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}}

		result := detector.Detect(newCodexDetectorTestContext("curl/8.0", ""), account)
		require.False(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonDisabled, result.Reason)
	})

	t.Run("开启后 codex_cli_rs 命中", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}

		result := detector.Detect(newCodexDetectorTestContext("codex_cli_rs/0.99.0", ""), account)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedUA, result.Reason)
	})

	t.Run("开启后 codex_vscode 命中", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}

		result := detector.Detect(newCodexDetectorTestContext("codex_vscode/1.0.0", ""), account)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedUA, result.Reason)
	})

	t.Run("开启后 codex_app 命中", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}

		result := detector.Detect(newCodexDetectorTestContext("codex_app/2.1.0", ""), account)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedUA, result.Reason)
	})

	t.Run("开启后 originator 命中", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}

		result := detector.Detect(newCodexDetectorTestContext("curl/8.0", "codex_chatgpt_desktop"), account)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedOriginator, result.Reason)
	})

	t.Run("开启后非官方客户端拒绝", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}

		result := detector.Detect(newCodexDetectorTestContext("curl/8.0", "my_client"), account)
		require.True(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
	})

	t.Run("开启 ForceCodexCLI 时允许通过", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(&config.Config{
			Gateway: config.GatewayConfig{ForceCodexCLI: true},
		})
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}

		result := detector.Detect(newCodexDetectorTestContext("curl/8.0", "my_client"), account)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonForceCodexCLI, result.Reason)
	})
}

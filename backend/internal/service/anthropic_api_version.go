package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
)

// resolveAnthropicAPIVersion shares the version policy between messages,
// count_tokens, OAuth body normalization, and account probes. Account overrides
// take precedence over an explicit client version, including in mimicry mode.
// Other providers' Anthropic-compatible endpoints retain their existing default.
func resolveAnthropicAPIVersion(c *gin.Context, account *Account) string {
	if version, ok := account.HeaderOverrideValue("anthropic-version"); ok {
		return version
	}
	if c != nil && c.Request != nil {
		if version := getHeaderRaw(c.Request.Header, "anthropic-version"); version != "" {
			return version
		}
	}
	if account != nil && account.Platform != PlatformAnthropic {
		return claude.LegacyAPIVersion
	}
	return claude.DefaultAPIVersion
}

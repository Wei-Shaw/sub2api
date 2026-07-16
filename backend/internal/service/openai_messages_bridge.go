package service

import (
	"bytes"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const openAICompatMessagesBridgeContextKey = "openai_compat_messages_bridge"

func isOpenAICompatMessagesBridgeBody(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	if bytes.Contains(body, []byte(openAICompatClaudeCodeTodoGuardMarker)) {
		return true
	}
	return isOpenAICompatMessagesBridgePromptCacheKey(gjson.GetBytes(body, "prompt_cache_key").String())
}

func isOpenAICompatMessagesBridgeRequestBody(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
	}
	if input, ok := reqBody["input"].([]any); ok && inputContainsText(input, openAICompatClaudeCodeTodoGuardMarker) {
		return true
	}
	return isOpenAICompatMessagesBridgePromptCacheKey(firstNonEmptyString(reqBody["prompt_cache_key"]))
}

func isOpenAICompatMessagesBridgePromptCacheKey(key string) bool {
	key = strings.TrimSpace(key)
	return strings.HasPrefix(key, "anthropic-metadata-") ||
		strings.HasPrefix(key, "anthropic-cache-") ||
		strings.HasPrefix(key, "anthropic-digest-")
}

func setOpenAICompatMessagesBridgeContext(c *gin.Context, enabled bool) {
	if c == nil || !enabled {
		return
	}
	c.Set(openAICompatMessagesBridgeContextKey, true)
}

func isOpenAICompatMessagesBridgeContext(c *gin.Context) bool {
	if c == nil {
		return false
	}
	value, ok := c.Get(openAICompatMessagesBridgeContextKey)
	if !ok {
		return false
	}
	enabled, ok := value.(bool)
	return ok && enabled
}

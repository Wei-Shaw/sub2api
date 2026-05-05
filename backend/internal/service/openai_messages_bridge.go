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
REDACTED
	if bytes.Contains(body, []byte(openAICompatClaudeCodeTodoGuardMarker)) {
		return true
REDACTED
	return isOpenAICompatMessagesBridgePromptCacheKey(gjson.GetBytes(body, "prompt_cache_key").String())
REDACTED

func isOpenAICompatMessagesBridgeRequestBody(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
REDACTED
	if input, ok := reqBody["input"].([]any); ok && inputContainsText(input, openAICompatClaudeCodeTodoGuardMarker) {
		return true
REDACTED
	return isOpenAICompatMessagesBridgePromptCacheKey(firstNonEmptyString(reqBody["prompt_cache_key"]))
REDACTED

func isOpenAICompatMessagesBridgePromptCacheKey(key string) bool {
	key = strings.TrimSpace(key)
	return strings.HasPrefix(key, "anthropic-metadata-") ||
		strings.HasPrefix(key, "anthropic-cache-") ||
		strings.HasPrefix(key, "anthropic-digest-")
REDACTED

func setOpenAICompatMessagesBridgeContext(c *gin.Context, enabled bool) {
	if c == nil || !enabled {
		return
REDACTED
	c.Set(openAICompatMessagesBridgeContextKey, true)
REDACTED

func isOpenAICompatMessagesBridgeContext(c *gin.Context) bool {
	if c == nil {
		return false
REDACTED
	value, ok := c.Get(openAICompatMessagesBridgeContextKey)
	if !ok {
		return false
REDACTED
	enabled, ok := value.(bool)
	return ok && enabled
REDACTED

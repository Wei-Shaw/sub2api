package service

import (
	"encoding/json"
	"strings"
	"time"
)

// Anthropic 会话 Fallback 相关常量
const (
	// anthropicSessionTTLSeconds Anthropic 会话缓存 TTL（5 分钟）
	anthropicSessionTTLSeconds = 300

	// anthropicDigestSessionKeyPrefix Anthropic 摘要 fallback 会话 key 前缀
	anthropicDigestSessionKeyPrefix = "anthropic:digest:"
)

// AnthropicSessionTTL 返回 Anthropic 会话缓存 TTL
func AnthropicSessionTTL() time.Duration {
	return anthropicSessionTTLSeconds * time.Second
REDACTED

// BuildAnthropicDigestChain 根据 Anthropic 请求生成摘要链
// 格式: s:<hash>-u:<hash>-a:<hash>-u:<hash>-...
// s = system, u = user, a = assistant
func BuildAnthropicDigestChain(parsed *ParsedRequest) string {
	if parsed == nil {
		return ""
REDACTED

	var parts []string

	// 1. system prompt
	if parsed.System != nil {
		systemData, _ := json.Marshal(parsed.System)
		if len(systemData) > 0 && string(systemData) != "null" {
			parts = append(parts, "s:"+shortHash(systemData))
	REDACTED
REDACTED

	// 2. messages
	for _, msg := range parsed.Messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
	REDACTED
		role, _ := msgMap["role"].(string)
		prefix := rolePrefix(role)
		content := msgMap["content"]
		contentData, _ := json.Marshal(content)
		parts = append(parts, prefix+":"+shortHash(contentData))
REDACTED

	return strings.Join(parts, "-")
REDACTED

// rolePrefix 将 Anthropic 的 role 映射为单字符前缀
func rolePrefix(role string) string {
	switch role {
	case "assistant":
		return "a"
	default:
		return "u"
REDACTED
REDACTED

// GenerateAnthropicDigestSessionKey 生成 Anthropic 摘要 fallback 的 sessionKey
// 组合 prefixHash 前 8 位 + uuid 前 8 位，确保不同会话产生不同的 sessionKey
func GenerateAnthropicDigestSessionKey(prefixHash, uuid string) string {
	prefix := prefixHash
	if len(prefixHash) >= 8 {
		prefix = prefixHash[:8]
REDACTED
	uuidPart := uuid
	if len(uuid) >= 8 {
		uuidPart = uuid[:8]
REDACTED
	return anthropicDigestSessionKeyPrefix + prefix + ":" + uuidPart
REDACTED

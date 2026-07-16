package service

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/tidwall/gjson"
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
}

// BuildAnthropicDigestChain 根据 Anthropic 请求生成摘要链
// 格式: s:<hash>-u:<hash>-a:<hash>-u:<hash>-...
// s = system, u = user, a = assistant
func BuildAnthropicDigestChain(parsed *ParsedRequest) string {
	if parsed == nil {
		return ""
	}

	var parts []string

	if systemRaw := parsed.SystemRaw(); len(systemRaw) > 0 && string(systemRaw) != "null" {
		parts = append(parts, "s:"+shortHash(canonicalAnthropicDigestJSON(systemRaw)))
	}

	messages := parsed.MessagesRaw()
	if len(messages) > 0 {
		gjson.ParseBytes(messages).ForEach(func(_, msg gjson.Result) bool {
			prefix := rolePrefix(msg.Get("role").String())
			content := msg.Get("content")
			parts = append(parts, prefix+":"+shortHash(canonicalAnthropicDigestJSON([]byte(content.Raw))))
			return true
		})
	}

	return strings.Join(parts, "-")
}

// canonicalAnthropicDigestJSON 保持 digest 对 JSON key 顺序和空白不敏感。
func canonicalAnthropicDigestJSON(raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return canonical
}

// rolePrefix 将 Anthropic 的 role 映射为单字符前缀
func rolePrefix(role string) string {
	switch role {
	case "assistant":
		return "a"
	default:
		return "u"
	}
}

// GenerateAnthropicDigestSessionKey 生成 Anthropic 摘要 fallback 的 sessionKey
// 组合 prefixHash 前 8 位 + uuid 前 8 位，确保不同会话产生不同的 sessionKey
func GenerateAnthropicDigestSessionKey(prefixHash, uuid string) string {
	prefix := prefixHash
	if len(prefixHash) >= 8 {
		prefix = prefixHash[:8]
	}
	uuidPart := uuid
	if len(uuid) >= 8 {
		uuidPart = uuid[:8]
	}
	return anthropicDigestSessionKeyPrefix + prefix + ":" + uuidPart
}

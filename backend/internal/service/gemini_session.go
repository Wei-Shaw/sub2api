package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/cespare/xxhash/v2"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// shortHash 使用 XXHash64 + Base36 生成短 hash（16 字符）
// XXHash64 比 SHA256 快约 10 倍，Base36 比 Hex 短约 20%
func shortHash(data []byte) string {
	h := xxhash.Sum64(data)
	return strconv.FormatUint(h, 36)
}

// BuildGeminiDigestChain 根据 Gemini 请求生成摘要链
// 格式: s:<hash>-u:<hash>-m:<hash>-u:<hash>-...
// s = systemInstruction, u = user, m = model
func BuildGeminiDigestChain(req *antigravity.GeminiRequest) string {
	if req == nil {
		return ""
	}

	var parts []string

	// 1. system instruction
	if req.SystemInstruction != nil && len(req.SystemInstruction.Parts) > 0 {
		partsData, _ := json.Marshal(req.SystemInstruction.Parts)
		parts = append(parts, "s:"+shortHash(partsData))
	}

	// 2. contents
	for _, c := range req.Contents {
		prefix := "u" // user
		if c.Role == "model" {
			prefix = "m"
		}
		partsData, _ := json.Marshal(c.Parts)
		parts = append(parts, prefix+":"+shortHash(partsData))
	}

	return strings.Join(parts, "-")
}

// GenerateGeminiPrefixHash 生成前缀 hash（用于分区隔离）
// 组合: userID + apiKeyID + ip + userAgent + platform + model
// 返回 16 字符的 Base64 编码的 SHA256 前缀
func GenerateGeminiPrefixHash(userID, apiKeyID int64, ip, userAgent, platform, model string) string {
	// 组合所有标识符
	normalizedUserAgent := NormalizeSessionUserAgent(userAgent)
	combined := strconv.FormatInt(userID, 10) + ":" +
		strconv.FormatInt(apiKeyID, 10) + ":" +
		ip + ":" +
		normalizedUserAgent + ":" +
		platform + ":" +
		model

	hash := sha256.Sum256([]byte(combined))
	// 取前 12 字节，Base64 编码后正好 16 字符
	return base64.RawURLEncoding.EncodeToString(hash[:12])
}

// ParseGeminiSessionValue 解析 Gemini 会话缓存值
// 格式: {uuid}:{accountID}
func ParseGeminiSessionValue(value string) (uuid string, accountID int64, ok bool) {
	if value == "" {
		return "", 0, false
	}

	// 找到最后一个 ":" 的位置（因为 uuid 可能包含 ":"）
	i := strings.LastIndex(value, ":")
	if i <= 0 || i >= len(value)-1 {
		return "", 0, false
	}

	uuid = value[:i]
	accountID, err := strconv.ParseInt(value[i+1:], 10, 64)
	if err != nil {
		return "", 0, false
	}

	return uuid, accountID, true
}

// FormatGeminiSessionValue 格式化 Gemini 会话缓存值
// 格式: {uuid}:{accountID}
func FormatGeminiSessionValue(uuid string, accountID int64) string {
	return uuid + ":" + strconv.FormatInt(accountID, 10)
}

// geminiDigestSessionKeyPrefix Gemini 摘要 fallback 会话 key 前缀
const geminiDigestSessionKeyPrefix = "gemini:digest:"

// geminiAnchorSessionKeyPrefix Gemini 对话首轮锚点粘性会话 key 前缀（持久化于 Redis）
const geminiAnchorSessionKeyPrefix = "gemini:anchor:"

// GenerateGeminiDigestSessionKey 生成 Gemini 摘要 fallback 的 sessionKey
// 保留完整分区与会话标识，避免截短后不同 API Key / 会话碰撞。
// 用于在 SelectAccountWithLoadAwareness 中保持粘性会话
func GenerateGeminiDigestSessionKey(prefixHash, uuid string) string {
	return geminiDigestSessionKeyPrefix + prefixHash + ":" + uuid
}

// BuildGeminiStickyAnchorKey 基于 prefixHash + systemInstruction + 首条 user 消息生成稳定锚点 sessionKey。
// 成立条件：当客户端未改写/压缩首轮历史（systemInstruction 与首条 user 消息保持不变）且 prefixHash 输入维度一致时，
// 同一多轮对话生成的锚点 key 保持稳定，可在进程内 DigestSessionStore 过期（>5m）或重启后从 Redis 恢复粘性账号。
func BuildGeminiStickyAnchorKey(prefixHash string, req *antigravity.GeminiRequest) string {
	if req == nil || len(req.Contents) == 0 {
		return ""
	}

	var anchorParts []string
	if req.SystemInstruction != nil && len(req.SystemInstruction.Parts) > 0 {
		if sysData, err := json.Marshal(req.SystemInstruction.Parts); err == nil {
			anchorParts = append(anchorParts, "s:"+shortHash(sysData))
		}
	}

	firstUserFound := false
	for _, c := range req.Contents {
		role := strings.ToLower(strings.TrimSpace(c.Role))
		if (role == "" || role == "user") && len(c.Parts) > 0 {
			if userData, err := json.Marshal(c.Parts); err == nil {
				anchorParts = append(anchorParts, "u:"+shortHash(userData))
				firstUserFound = true
				break
			}
		}
	}
	if !firstUserFound {
		return ""
	}

	anchorDigest := shortHash([]byte(strings.Join(anchorParts, "|")))
	return geminiAnchorSessionKeyPrefix + prefixHash + ":" + anchorDigest
}

// DeriveGeminiAnchorSessionUUID 从锚点 key 派生稳定的短 UUID，供恢复 DigestSessionKey 使用。
func DeriveGeminiAnchorSessionUUID(anchorKey string) string {
	if anchorKey == "" {
		return ""
	}
	h := sha256.Sum256([]byte(anchorKey))
	return base64.RawURLEncoding.EncodeToString(h[:12])
}

// EnsureGeminiRequestSessionIDBytes 确保发往 Antigravity v1internal 的 Gemini 请求携带稳定的 sessionId。
// 有网关隔离标识时，使用经过 API Key、账号和分组隔离的会话值。
// 没有隔离上下文的兼容调用保留 sessionId，或把 session_id 补入 sessionId。
// 通过 sjson.SetBytes 修改字段，不重排其余 JSON 字段。
func EnsureGeminiRequestSessionIDBytes(body []byte, identity ...string) []byte {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body
	}
	var seedText string
	if len(identity) > 0 {
		seedText = identity[0]
	}
	if seedText != "" {
		body = removeAmbiguousGeminiSessionFields(body)
	}
	if seedText == "" {
		if strings.TrimSpace(gjson.GetBytes(body, "sessionId").String()) != "" {
			return body
		}
		if alias := gjson.GetBytes(body, "session_id"); alias.Type == gjson.String && strings.TrimSpace(alias.String()) != "" {
			updated, err := sjson.SetBytes(body, "sessionId", alias.String())
			if err == nil {
				return updated
			}
			return body
		}
		seedText = extractGeminiSessionSeedTextBytes(body)
	}
	if seedText == "" {
		return body
	}

	h := sha256.Sum256([]byte(seedText))
	n := int64(binary.BigEndian.Uint64(h[:8])) & 0x7FFFFFFFFFFFFFFF
	updated, err := sjson.SetBytes(body, "sessionId", "-"+strconv.FormatInt(n, 10))
	if err != nil {
		return body
	}
	if len(identity) > 0 && identity[0] != "" {
		// Protobuf accepts both spellings. Do not leave a caller-controlled
		// alias alongside the isolated canonical field.
		if canonical, err := sjson.DeleteBytes(updated, "session_id"); err == nil {
			return canonical
		}
	}
	return updated
}

func extractGeminiSessionSeedTextBytes(body []byte) string {
	contents := gjson.GetBytes(body, "contents")
	if contents.IsArray() {
		var seed string
		contents.ForEach(func(_, content gjson.Result) bool {
			role := strings.ToLower(strings.TrimSpace(content.Get("role").String()))
			if role != "" && role != "user" {
				return true
			}
			parts := content.Get("parts")
			if !parts.IsArray() {
				return true
			}
			parts.ForEach(func(_, part gjson.Result) bool {
				if text := strings.TrimSpace(part.Get("text").String()); text != "" {
					seed = text
					return false
				}
				return true
			})
			if seed != "" {
				return false
			}
			if raw := strings.TrimSpace(parts.Raw); raw != "" && raw != "[]" {
				seed = raw
				return false
			}
			return true
		})
		if seed != "" {
			return seed
		}
	}
	if sysInst := gjson.GetBytes(body, "systemInstruction"); sysInst.Exists() {
		if raw := strings.TrimSpace(sysInst.Raw); raw != "" && raw != "null" && raw != "{}" {
			return raw
		}
	}
	return ""
}

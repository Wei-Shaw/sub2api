package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// codexSignatureIncludeValue 是 Codex 官方客户端在 include 里声明的加密推理内容。
// 校验签名的新-api 系中转把它当作"请求来自 Codex 官方客户端"的证据之一。
const codexSignatureIncludeValue = "reasoning.encrypted_content"

// prompt_cache_key 缺失时派生值的固定前缀与兜底值。
const (
	codexSignatureCacheKeyPrefix   = "sub2api-codex-"
	codexSignatureCacheKeyFallback = "sub2api-codex-signature"
)

// ensureOpenAIResponsesCodexSignature 为出站 Responses 请求补齐 Codex 客户端签名。
//
// 部分 OpenAI 兼容中转（new-api 系）要求 include 含 reasoning.encrypted_content
// 且 prompt_cache_key 为非空字符串，缺任一项即回 400 invalid codex request。Codex
// 客户端并非每次请求都带齐这两项，因此同一模型的请求会"有的正常、有的报错"。
//
// 本函数只做补齐，不删除客户端已有取值：
//   - include 已含签名取值时原样保留（其余取值与顺序不变）；
//   - prompt_cache_key 已是非空字符串时原样保留，否则按会话派生一个稳定值。
//
// 空 body、非法 JSON 与非对象 body 原样返回：补丁本身不得成为新的失败点。
// 由账号开关 openai_responses_ensure_codex_signature 控制调用，默认不生效。
func ensureOpenAIResponsesCodexSignature(body []byte) ([]byte, bool, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, false, nil
	}
	root := parseRawJSONView(body)
	if !root.IsObject() {
		return body, false, nil
	}

	out := body
	changed := false

	include := root.Get("include")
	if !openAIResponsesIncludeHasCodexSignature(include) {
		next, err := appendOpenAIResponsesCodexInclude(out, include)
		if err != nil {
			return body, false, err
		}
		out = next
		changed = true
	}

	// prompt_cache_key 必须是字符串：数字/布尔/null 与缺失、空串一并视为未带签名。
	key := root.Get("prompt_cache_key")
	if key.Type != gjson.String || strings.TrimSpace(key.Str) == "" {
		next, err := sjson.SetBytes(out, "prompt_cache_key", deriveCodexSignaturePromptCacheKey(root))
		if err != nil {
			return body, false, err
		}
		out = next
		changed = true
	}

	if !changed {
		return body, false, nil
	}
	return out, true, nil
}

// openAIResponsesIncludeHasCodexSignature 报告 include 数组里是否已含签名取值。
func openAIResponsesIncludeHasCodexSignature(include gjson.Result) bool {
	if !include.IsArray() {
		return false
	}
	found := false
	include.ForEach(func(_, value gjson.Result) bool {
		if value.Type == gjson.String && value.Str == codexSignatureIncludeValue {
			found = true
			return false
		}
		return true
	})
	return found
}

// appendOpenAIResponsesCodexInclude 在保留客户端既有取值与顺序的前提下补上签名取值。
// include 缺失或不是数组时整体写成只含签名的数组：校验方要求它是数组。
func appendOpenAIResponsesCodexInclude(body []byte, include gjson.Result) ([]byte, error) {
	if !include.IsArray() {
		return sjson.SetBytes(body, "include", []string{codexSignatureIncludeValue})
	}
	items := include.Array()
	raw := make([]string, 0, len(items)+1)
	for _, item := range items {
		raw = append(raw, item.Raw)
	}
	raw = append(raw, strconv.Quote(codexSignatureIncludeValue))
	return sjson.SetRawBytes(body, "include", []byte("["+strings.Join(raw, ",")+"]"))
}

// deriveCodexSignaturePromptCacheKey 为缺失 prompt_cache_key 的请求派生稳定值。
//
// 优先取会话标识，让同一会话的后续轮次落在同一个缓存键上（也避免同一账号下不同
// 会话互相污染提示缓存）。会话标识缺失时退化为按 instructions + 首条 input 派生：
// 同一会话每轮都重复这段前缀，因此仍会得到同一个键。
func deriveCodexSignaturePromptCacheKey(root gjson.Result) string {
	for _, path := range []string{"client_metadata.session_id", "session_id", "conversation_id"} {
		if v := strings.TrimSpace(root.Get(path).String()); v != "" {
			return v
		}
	}
	instructions := root.Get("instructions")
	input := root.Get("input")
	if !instructions.Exists() && !input.Exists() {
		return codexSignatureCacheKeyFallback
	}
	digest := sha256.New()
	digest.Write([]byte(instructions.String()))
	switch {
	case input.IsArray():
		if items := input.Array(); len(items) > 0 {
			digest.Write([]byte(items[0].Raw))
		}
	case input.Type == gjson.String:
		digest.Write([]byte(input.Str))
	}
	sum := hex.EncodeToString(digest.Sum(nil))
	return codexSignatureCacheKeyPrefix + sum[:32]
}

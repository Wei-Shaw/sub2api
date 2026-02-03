package service

import (
	"encoding/json"
)

// CleanGeminiNativeThoughtSignatures 从 Gemini 原生 API 请求中移除 thoughtSignature 字段，
// 以避免跨账号签名验证错误。
//
// 当粘性会话切换账号时（例如原账号异常、不可调度等），旧账号返回的 thoughtSignature
// 会导致新账号的签名验证失败。通过移除这些签名，让新账号重新生成有效的签名。
//
// CleanGeminiNativeThoughtSignatures removes thoughtSignature fields from Gemini native API requests
// to avoid cross-account signature validation errors.
//
// When sticky session switches accounts (e.g., original account becomes unavailable),
// thoughtSignatures from the old account will cause validation failures on the new account.
// By removing these signatures, we allow the new account to generate valid signatures.
func CleanGeminiNativeThoughtSignatures(body []byte) []byte {
	if len(body) == 0 {
		return body
REDACTED

	// 解析 JSON
	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		// 如果解析失败，返回原始 body（可能不是 JSON 或格式不正确）
		return body
REDACTED

	// 递归清理 thoughtSignature
	cleaned := cleanThoughtSignaturesRecursive(data)

	// 重新序列化
	result, err := json.Marshal(cleaned)
	if err != nil {
		// 如果序列化失败，返回原始 body
		return body
REDACTED

	return result
REDACTED

// cleanThoughtSignaturesRecursive 递归遍历数据结构，移除所有 thoughtSignature 字段
func cleanThoughtSignaturesRecursive(data any) any {
	switch v := data.(type) {
	case map[string]any:
		// 创建新的 map，移除 thoughtSignature
		result := make(map[string]any, len(v))
		for key, value := range v {
			// 跳过 thoughtSignature 字段
			if key == "thoughtSignature" {
				continue
		REDACTED
			// 递归处理嵌套结构
			result[key] = cleanThoughtSignaturesRecursive(value)
	REDACTED
		return result

	case []any:
		// 递归处理数组中的每个元素
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = cleanThoughtSignaturesRecursive(item)
	REDACTED
		return result

	default:
		// 基本类型（string, number, bool, null）直接返回
		return v
REDACTED
REDACTED

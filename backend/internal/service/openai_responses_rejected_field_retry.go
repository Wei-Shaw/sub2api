package service

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const maxOpenAIResponsesRejectedFieldRetries = 6

var (
	openAIResponsesRejectedNamespaceParamPattern = regexp.MustCompile(`(?i)^input\[(\d+)\]\.namespace$`)
	openAIResponsesRejectedMessageParamPattern   = regexp.MustCompile(`(?i)(?:unknown|unsupported)[ _-]+parameter\s*(?::|=|is)?\s*["']?(max_output_tokens|input\[\d+\]\.namespace)(?:["']|\b)`)
	// ChatGPT internal Codex 端点点名拒绝顶层字段时用的是另一种文案，例如
	// "prompt_cache_retention is not supported on this model"（code 为
	// invalid_parameter），与上面的 "unknown/unsupported parameter: X" 形态不同。
	openAIResponsesRejectedUnsupportedOnModelPattern = regexp.MustCompile(`(?i)^([a-z0-9_]+)\s+is\s+not\s+supported\s+(?:on|with|for|by)\s+this\s+model`)
)

// openAIResponsesStrippableRejectedParams 是被上游以 400 点名拒绝后，可以安全剥离
// 并重试一次的顶层可选字段。
//
// 判据是「删掉它等价于客户端从没发过这个字段」：这些字段只影响提示缓存策略、
// 调用方标识与用量统计口径，不参与生成语义，剥掉不会改变模型输出。
//
// 刻意不含 model / input / instructions / tools / reasoning 等语义关键字段——
// 那些被拒时必须把错误如实返回客户端，而不是悄悄改写请求再重试。
var openAIResponsesStrippableRejectedParams = map[string]struct{}{
	"prompt_cache_retention": {},
	"prompt_cache_options":   {},
	"safety_identifier":      {},
	"stream_options":         {},
	"metadata":               {},
	"user":                   {},
}

type openAIResponsesRejectedFieldRetryState struct {
	attempts       int
	seenBodyHashes map[[sha256.Size]byte]struct{}
}

func newOpenAIResponsesRejectedFieldRetryState(initialBody []byte) *openAIResponsesRejectedFieldRetryState {
	state := &openAIResponsesRejectedFieldRetryState{
		seenBodyHashes: make(map[[sha256.Size]byte]struct{}, maxOpenAIResponsesRejectedFieldRetries+1),
	}
	state.remember(initialBody)
	return state
}

func (s *openAIResponsesRejectedFieldRetryState) Allow(nextBody []byte) bool {
	if s == nil || len(nextBody) == 0 || s.attempts >= maxOpenAIResponsesRejectedFieldRetries {
		return false
	}
	bodyHash := sha256.Sum256(nextBody)
	if _, seen := s.seenBodyHashes[bodyHash]; seen {
		return false
	}
	s.seenBodyHashes[bodyHash] = struct{}{}
	s.attempts++
	return true
}

func (s *openAIResponsesRejectedFieldRetryState) remember(body []byte) {
	if s == nil || len(body) == 0 {
		return
	}
	if s.seenBodyHashes == nil {
		s.seenBodyHashes = make(map[[sha256.Size]byte]struct{}, maxOpenAIResponsesRejectedFieldRetries+1)
	}
	s.seenBodyHashes[sha256.Sum256(body)] = struct{}{}
}

func normalizeOpenAIResponsesRejectedFieldRetryBody(statusCode int, body, responseBody []byte) ([]byte, string, bool, error) {
	if statusCode != http.StatusBadRequest || len(body) == 0 || len(responseBody) == 0 {
		return nil, "", false, nil
	}

	code := strings.ToLower(strings.TrimSpace(extractUpstreamErrorCode(responseBody)))
	message := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(responseBody)))

	param := strings.ToLower(strings.TrimSpace(gjson.GetBytes(responseBody, "error.param").String()))
	if param == "" {
		param = openAIResponsesRejectedParamFromMessage(message)
	}

	// 已知可安全剥离的顶层可选字段：只要上游点名拒绝，就剥掉重试一次。
	//
	// 这一支刻意不走 isExplicitOpenAIResponsesFieldRejection 的 code 白名单：
	// 这类拒绝的 code 并不统一（ChatGPT internal 实测用 invalid_parameter，
	// 文案为 "X is not supported on this model"），靠 code 判定会漏。安全性由
	// openAIResponsesStrippableRejectedParams 白名单保证——只有明确无生成语义的
	// 字段才会被剥离，其余 param（含 model、input[N].name 等）一律不动。
	if retryBody, reason, changed, err := stripRejectedOptionalParam(body, param); err != nil || changed {
		return retryBody, reason, changed, err
	}

	if !isExplicitOpenAIResponsesFieldRejection(code, message) {
		return nil, "", false, nil
	}

	if index, ok := openAIResponsesRejectedNamespaceIndex(param); ok {
		return removeOpenAIResponsesRejectedNamespaceAtIndex(body, index)
	}
	if param == "max_output_tokens" && gjson.GetBytes(body, "max_output_tokens").Exists() {
		retryBody, err := sjson.DeleteBytes(body, "max_output_tokens")
		if err != nil {
			return nil, "", false, fmt.Errorf("delete rejected max_output_tokens: %w", err)
		}
		return retryBody, "max_output_tokens parameter rejection", true, nil
	}
	return nil, "", false, nil
}

func isExplicitOpenAIResponsesFieldRejection(code, message string) bool {
	switch strings.TrimSpace(code) {
	case "unknown_parameter", "unsupported_parameter":
		return true
	}
	return strings.Contains(message, "unknown parameter") ||
		strings.Contains(message, "unsupported parameter")
}

func openAIResponsesRejectedParamFromMessage(message string) string {
	trimmed := strings.TrimSpace(message)
	if match := openAIResponsesRejectedMessageParamPattern.FindStringSubmatch(trimmed); len(match) == 2 {
		return strings.ToLower(strings.TrimSpace(match[1]))
	}
	// "X is not supported on this model" 形态：上游通常同时给了 error.param，
	// 这里只是缺失时的兜底。
	if match := openAIResponsesRejectedUnsupportedOnModelPattern.FindStringSubmatch(trimmed); len(match) == 2 {
		return strings.ToLower(strings.TrimSpace(match[1]))
	}
	return ""
}

// stripRejectedOptionalParam 在 param 命中可剥离白名单且请求体确实带了该字段时，
// 删除它并返回可用于重试的 body。
//
// param 形如 "prompt_cache_options.ttl" 时按其顶层字段处理：上游拒的是该对象的
// 内容，整体删掉即等价于没发这个对象。非白名单 param（含 "input[74].name" 这类
// 带下标的路径）一律返回 changed=false，交给后续分支或如实回传客户端。
func stripRejectedOptionalParam(body []byte, param string) ([]byte, string, bool, error) {
	if param == "" {
		return nil, "", false, nil
	}
	topLevel := param
	if idx := strings.IndexByte(topLevel, '.'); idx > 0 {
		topLevel = topLevel[:idx]
	}
	if _, strippable := openAIResponsesStrippableRejectedParams[topLevel]; !strippable {
		return nil, "", false, nil
	}
	if !gjson.GetBytes(body, topLevel).Exists() {
		return nil, "", false, nil
	}
	retryBody, err := sjson.DeleteBytes(body, topLevel)
	if err != nil {
		return nil, "", false, fmt.Errorf("delete rejected %s: %w", topLevel, err)
	}
	return retryBody, topLevel + " parameter rejection", true, nil
}

func openAIResponsesRejectedNamespaceIndex(param string) (int, bool) {
	match := openAIResponsesRejectedNamespaceParamPattern.FindStringSubmatch(strings.TrimSpace(param))
	if len(match) != 2 {
		return 0, false
	}
	index, err := strconv.Atoi(match[1])
	if err == nil && index >= 0 {
		return index, true
	}
	return 0, false
}

func removeOpenAIResponsesRejectedNamespaceAtIndex(body []byte, index int) ([]byte, string, bool, error) {
	itemPath := fmt.Sprintf("input.%d", index)
	itemType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, itemPath+".type").String()))
	switch itemType {
	case "function_call", "tool_call", "custom_tool_call", "mcp_tool_call":
	default:
		return nil, "", false, nil
	}

	namespacePath := itemPath + ".namespace"
	if !gjson.GetBytes(body, namespacePath).Exists() {
		return nil, "", false, nil
	}
	retryBody, err := sjson.DeleteBytes(body, namespacePath)
	if err != nil {
		return nil, "", false, fmt.Errorf("delete rejected namespace at input[%d]: %w", index, err)
	}
	return retryBody, "indexed namespace parameter rejection", true, nil
}

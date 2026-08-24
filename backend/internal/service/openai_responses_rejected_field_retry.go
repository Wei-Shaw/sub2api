package service

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const maxOpenAIResponsesRejectedFieldRetries = 6

var (
	openAIResponsesRejectedNamespaceParamPattern  = regexp.MustCompile(`(?i)^input\[(\d+)\]\.namespace$`)
	openAIResponsesRejectedStatusParamPattern     = regexp.MustCompile(`(?i)^input\[(\d+)\]\.status$`)
	openAIResponsesRejectedContentParamPattern    = regexp.MustCompile(`(?i)^input\[(\d+)\]\.content$`)
	openAIResponsesRejectedCacheParamPattern      = regexp.MustCompile(`(?i)^input\[(\d+)\]\.prompt_cache_breakpoint$`)
	openAIResponsesRejectedMessageParamPattern    = regexp.MustCompile(`(?i)(?:unknown|unsupported)[ _-]+parameter\s*(?::|=|is)?\s*["']?(max_output_tokens|truncation|input\[\d+\]\.(?:namespace|status))(?:["']|\b)`)
	openAIResponsesInvalidTypeMessageParamPattern = regexp.MustCompile(`(?i)invalid[ _-]+type\s+for\s+["']?(input\[\d+\]\.content)(?:["']|\b)[^\n]*\b(?:got|received)\s+null\b`)
	openAIResponsesMaxZeroContentMessagePattern   = regexp.MustCompile(`(?i)invalid\s+["']?(input\[\d+\]\.content)["']?\s*:\s*array too long\.[^\n]*maximum length 0\b`)
	openAIResponsesCacheModelRejectionPattern     = regexp.MustCompile(`(?i)["']?(prompt_cache_breakpoint|input\[\d+\]\.prompt_cache_breakpoint)["']?\s+is\s+not\s+supported\s+on\s+this\s+model\b`)
	openAIResponsesToolParametersParamPattern     = regexp.MustCompile(`(?i)^(?:tools|input)\[\d+\](?:\.tools\[\d+\])*(?:\.function)?\.parameters$`)
	openAIResponsesMissingSchemaTypePattern       = regexp.MustCompile(`(?i)\bgot\s+["']?type\s*:\s*["']?none["']?`)
)

type openAIResponsesRejectedFieldRetryState struct {
	mu             sync.Mutex
	budget         *openAIResponsesRejectedFieldRetryBudget
	seenBodyHashes map[[sha256.Size]byte]struct{}
}

type openAIResponsesRejectedFieldRetryBudget struct {
	mu       sync.Mutex
	attempts int
}

const openAIResponsesRejectedFieldRetryBudgetContextKey = "openai_responses_rejected_field_retry_budget"

// openAIResponsesRejectedFieldRetryStateForRequest returns a fresh loop guard
// for one account attempt backed by the inbound request's shared retry budget.
// A later account may apply the same compatibility transform, while all account
// attempts together remain bounded.
func openAIResponsesRejectedFieldRetryStateForRequest(c *gin.Context, initialBody []byte) *openAIResponsesRejectedFieldRetryState {
	var budget *openAIResponsesRejectedFieldRetryBudget
	if c != nil {
		if existing, ok := c.Get(openAIResponsesRejectedFieldRetryBudgetContextKey); ok {
			budget, _ = existing.(*openAIResponsesRejectedFieldRetryBudget)
		}
	}
	if budget == nil {
		budget = &openAIResponsesRejectedFieldRetryBudget{}
		if c != nil {
			c.Set(openAIResponsesRejectedFieldRetryBudgetContextKey, budget)
		}
	}
	return newOpenAIResponsesRejectedFieldRetryStateWithBudget(initialBody, budget)
}

func newOpenAIResponsesRejectedFieldRetryState(initialBody []byte) *openAIResponsesRejectedFieldRetryState {
	return newOpenAIResponsesRejectedFieldRetryStateWithBudget(initialBody, &openAIResponsesRejectedFieldRetryBudget{})
}

func newOpenAIResponsesRejectedFieldRetryStateWithBudget(initialBody []byte, budget *openAIResponsesRejectedFieldRetryBudget) *openAIResponsesRejectedFieldRetryState {
	state := &openAIResponsesRejectedFieldRetryState{
		budget:         budget,
		seenBodyHashes: make(map[[sha256.Size]byte]struct{}, maxOpenAIResponsesRejectedFieldRetries+1),
	}
	state.remember(initialBody)
	return state
}

func (s *openAIResponsesRejectedFieldRetryState) Allow(nextBody []byte) bool {
	if s == nil || s.budget == nil || len(nextBody) == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bodyHash := sha256.Sum256(nextBody)
	if _, seen := s.seenBodyHashes[bodyHash]; seen {
		return false
	}
	s.budget.mu.Lock()
	defer s.budget.mu.Unlock()
	if s.budget.attempts >= maxOpenAIResponsesRejectedFieldRetries {
		return false
	}
	s.seenBodyHashes[bodyHash] = struct{}{}
	s.budget.attempts++
	return true
}

func (s *openAIResponsesRejectedFieldRetryState) remember(body []byte) {
	if s == nil || len(body) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rememberLocked(body)
}

func (s *openAIResponsesRejectedFieldRetryState) rememberLocked(body []byte) {
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
	if code == "invalid_function_parameters" &&
		openAIResponsesToolParametersParamPattern.MatchString(param) &&
		openAIResponsesMissingSchemaTypePattern.MatchString(message) {
		retryBody, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)
		if err != nil {
			return nil, "", false, fmt.Errorf("repair rejected tool parameter root type: %w", err)
		}
		if changed {
			return retryBody, "tool parameter root type rejection", true, nil
		}
	}
	cacheMessageParam := openAIResponsesCacheModelRejectionParamFromMessage(message)
	cacheParam := param
	if cacheParam == "" {
		cacheParam = cacheMessageParam
	}
	cacheParamMatchesMessage := cacheMessageParam == "" || cacheParam == cacheMessageParam
	cacheModelRejection := code == "invalid_parameter" || cacheMessageParam != ""
	if cacheParam != "" && cacheParamMatchesMessage && cacheModelRejection {
		if cacheParam == "prompt_cache_breakpoint" && gjson.GetBytes(body, cacheParam).Exists() {
			retryBody, err := sjson.DeleteBytes(body, cacheParam)
			if err != nil {
				return nil, "", false, fmt.Errorf("delete rejected prompt_cache_breakpoint: %w", err)
			}
			return retryBody, "prompt_cache_breakpoint parameter rejection", true, nil
		}
		if index, ok := openAIResponsesRejectedCacheIndex(cacheParam); ok {
			return removeOpenAIResponsesRejectedCacheAtIndex(body, index)
		}
	}
	if isExplicitOpenAIResponsesFieldRejection(code, message) {
		messageParam := openAIResponsesRejectedParamFromMessage(message)
		if param != "" && messageParam != "" && param != messageParam {
			return nil, "", false, nil
		}
		if param == "" {
			param = messageParam
		}
		if index, ok := openAIResponsesRejectedNamespaceIndex(param); ok {
			return removeOpenAIResponsesRejectedNamespaceAtIndex(body, index)
		}
		if index, ok := openAIResponsesRejectedStatusIndex(param); ok {
			return removeOpenAIResponsesRejectedStatusAtIndex(body, index)
		}
		if param == "max_output_tokens" && gjson.GetBytes(body, "max_output_tokens").Exists() {
			retryBody, err := sjson.DeleteBytes(body, "max_output_tokens")
			if err != nil {
				return nil, "", false, fmt.Errorf("delete rejected max_output_tokens: %w", err)
			}
			return retryBody, "max_output_tokens parameter rejection", true, nil
		}
		if param == "truncation" && gjson.GetBytes(body, "truncation").Exists() {
			retryBody, err := sjson.DeleteBytes(body, "truncation")
			if err != nil {
				return nil, "", false, fmt.Errorf("delete rejected truncation: %w", err)
			}
			return retryBody, "truncation parameter rejection", true, nil
		}
	}

	messageContentParam := openAIResponsesInvalidTypeParamFromMessage(message)
	contentParam := param
	if contentParam == "" {
		contentParam = messageContentParam
	}
	if index, ok := openAIResponsesRejectedContentIndex(contentParam); ok &&
		contentParam == messageContentParam && isExplicitOpenAIResponsesNullContentRejection(code, message) {
		return normalizeOpenAIResponsesRejectedNullContentAtIndex(body, index)
	}
	maxZeroContentParam := openAIResponsesMaxZeroContentParamFromMessage(message)
	if index, ok := openAIResponsesRejectedContentIndex(param); ok &&
		param == maxZeroContentParam && code == "array_above_max_length" {
		return removeOpenAIResponsesRejectedReasoningContentAtIndex(body, index)
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
	match := openAIResponsesRejectedMessageParamPattern.FindStringSubmatch(strings.TrimSpace(message))
	if len(match) != 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(match[1]))
}

func openAIResponsesMaxZeroContentParamFromMessage(message string) string {
	match := openAIResponsesMaxZeroContentMessagePattern.FindStringSubmatch(strings.TrimSpace(message))
	if len(match) != 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(match[1]))
}

func openAIResponsesInvalidTypeParamFromMessage(message string) string {
	match := openAIResponsesInvalidTypeMessageParamPattern.FindStringSubmatch(strings.TrimSpace(message))
	if len(match) != 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(match[1]))
}

func openAIResponsesCacheModelRejectionParamFromMessage(message string) string {
	match := openAIResponsesCacheModelRejectionPattern.FindStringSubmatch(strings.TrimSpace(message))
	if len(match) != 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(match[1]))
}

func isExplicitOpenAIResponsesNullContentRejection(code, message string) bool {
	code = strings.TrimSpace(code)
	return (code == "invalid_type" || code == "invalid_request_error" || code == "") &&
		openAIResponsesInvalidTypeMessageParamPattern.MatchString(strings.TrimSpace(message))
}

func openAIResponsesRejectedNamespaceIndex(param string) (int, bool) {
	return openAIResponsesRejectedInputIndex(openAIResponsesRejectedNamespaceParamPattern, param)
}

func openAIResponsesRejectedStatusIndex(param string) (int, bool) {
	return openAIResponsesRejectedInputIndex(openAIResponsesRejectedStatusParamPattern, param)
}

func openAIResponsesRejectedContentIndex(param string) (int, bool) {
	return openAIResponsesRejectedInputIndex(openAIResponsesRejectedContentParamPattern, param)
}

func openAIResponsesRejectedCacheIndex(param string) (int, bool) {
	return openAIResponsesRejectedInputIndex(openAIResponsesRejectedCacheParamPattern, param)
}

func openAIResponsesRejectedInputIndex(pattern *regexp.Regexp, param string) (int, bool) {
	match := pattern.FindStringSubmatch(strings.TrimSpace(param))
	if len(match) != 2 {
		return 0, false
	}
	index, err := strconv.Atoi(match[1])
	if err == nil && index >= 0 {
		return index, true
	}
	return 0, false
}

func removeOpenAIResponsesRejectedStatusAtIndex(body []byte, index int) ([]byte, string, bool, error) {
	itemPath := fmt.Sprintf("input.%d", index)
	if !gjson.GetBytes(body, itemPath).IsObject() {
		return nil, "", false, nil
	}
	statusPath := itemPath + ".status"
	if !gjson.GetBytes(body, statusPath).Exists() {
		return nil, "", false, nil
	}
	retryBody, err := sjson.DeleteBytes(body, statusPath)
	if err != nil {
		return nil, "", false, fmt.Errorf("delete rejected status at input[%d]: %w", index, err)
	}
	return retryBody, "indexed status parameter rejection", true, nil
}

func removeOpenAIResponsesRejectedCacheAtIndex(body []byte, index int) ([]byte, string, bool, error) {
	itemPath := fmt.Sprintf("input.%d", index)
	if !gjson.GetBytes(body, itemPath).IsObject() {
		return nil, "", false, nil
	}
	cachePath := itemPath + ".prompt_cache_breakpoint"
	if !gjson.GetBytes(body, cachePath).Exists() {
		return nil, "", false, nil
	}
	retryBody, err := sjson.DeleteBytes(body, cachePath)
	if err != nil {
		return nil, "", false, fmt.Errorf("delete rejected prompt_cache_breakpoint at input[%d]: %w", index, err)
	}
	return retryBody, "indexed prompt_cache_breakpoint parameter rejection", true, nil
}

func normalizeOpenAIResponsesRejectedNullContentAtIndex(body []byte, index int) ([]byte, string, bool, error) {
	itemPath := fmt.Sprintf("input.%d", index)
	item := gjson.GetBytes(body, itemPath)
	content := gjson.GetBytes(body, itemPath+".content")
	if !item.IsObject() || !content.Exists() || content.Type != gjson.Null {
		return nil, "", false, nil
	}

	itemType := strings.ToLower(strings.TrimSpace(item.Get("type").String()))
	role := strings.TrimSpace(item.Get("role").String())
	contentPath := itemPath + ".content"
	switch {
	case itemType == "reasoning":
		retryBody, err := sjson.DeleteBytes(body, contentPath)
		if err != nil {
			return nil, "", false, fmt.Errorf("delete rejected null content at input[%d]: %w", index, err)
		}
		return retryBody, "indexed reasoning null content rejection", true, nil
	case itemType == "message" || role != "":
		retryBody, err := sjson.SetBytes(body, contentPath, "")
		if err != nil {
			return nil, "", false, fmt.Errorf("normalize rejected null content at input[%d]: %w", index, err)
		}
		return retryBody, "indexed message null content rejection", true, nil
	default:
		return nil, "", false, nil
	}
}

func removeOpenAIResponsesRejectedReasoningContentAtIndex(body []byte, index int) ([]byte, string, bool, error) {
	itemPath := fmt.Sprintf("input.%d", index)
	item := gjson.GetBytes(body, itemPath)
	content := item.Get("content")
	if !item.IsObject() || strings.TrimSpace(item.Get("type").String()) != "reasoning" || !content.IsArray() || len(content.Array()) == 0 {
		return nil, "", false, nil
	}
	retryBody, err := sjson.DeleteBytes(body, itemPath+".content")
	if err != nil {
		return nil, "", false, fmt.Errorf("delete rejected reasoning content at input[%d]: %w", index, err)
	}
	return retryBody, "indexed reasoning content maximum-length rejection", true, nil
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

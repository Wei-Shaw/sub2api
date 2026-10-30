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
	openAIResponsesRejectedCacheParamPattern      = regexp.MustCompile(`(?i)^(?:input|messages)\[(\d+)\]\.prompt_cache_breakpoint(?:\.(?:mode|ttl))?$`)
	openAIResponsesRejectedCachePartPattern       = regexp.MustCompile(`(?i)^(?:input|messages)\[(\d+)\]\.content\[(\d+)\]\.prompt_cache_breakpoint(?:\.(?:mode|ttl))?$`)
	openAIResponsesRejectedOptionsParamPattern    = regexp.MustCompile(`(?i)^prompt_cache_options(?:\.(?:mode|ttl))?$`)
	openAIResponsesRejectedMessageParamPattern    = regexp.MustCompile(`(?i)(?:unknown|unsupported)[ _-]+parameter\s*(?::|=|is)?\s*["']?(max_output_tokens|truncation|prompt_cache_options|prompt_cache_breakpoint|(?:input|messages)\[\d+\](?:\.content\[\d+\])?\.(?:prompt_cache_breakpoint|namespace|status))(?:["']|\b)`)
	openAIResponsesInvalidTypeMessageParamPattern = regexp.MustCompile(`(?i)invalid[ _-]+type\s+for\s+["']?(input\[\d+\]\.content)(?:["']|\b)[^\n]*\b(?:got|received)\s+null\b`)
	openAIResponsesMaxZeroContentMessagePattern   = regexp.MustCompile(`(?i)invalid\s+["']?(input\[\d+\]\.content)["']?\s*:\s*array too long\.[^\n]*maximum length 0\b`)
	openAIResponsesCacheModelRejectionPattern     = regexp.MustCompile(`(?i)["']?(prompt_cache_options|prompt_cache_breakpoint|(?:input|messages)\[\d+\](?:\.content\[\d+\])?\.prompt_cache_breakpoint)["']?\s+is\s+not\s+supported\s+on\s+(?:this|the\s+selected)\s+model\b`)
	openAIResponsesMissingCacheParamPattern       = regexp.MustCompile(`(?i)missing\s+required\s+parameter\s*:?\s*["']?(prompt_cache_options|prompt_cache_breakpoint|(?:input|messages)\[\d+\](?:\.content\[\d+\])?\.prompt_cache_breakpoint)(?:\.(?:mode|ttl))?["']?`)
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
	cacheMessageParam := openAIResponsesRejectedPromptCacheParamFromMessage(message)
	cacheParam := param
	if isPromptCacheHintParam(cacheParam) {
		cacheParam = normalizePromptCacheHintParam(cacheParam)
	} else {
		cacheParam = ""
	}
	if cacheParam != "" && cacheMessageParam != "" && cacheParam != cacheMessageParam {
		// The structured param and the message disagree about which hint the
		// upstream rejected (for example param=prompt_cache_breakpoint while the
		// message names input[0].prompt_cache_breakpoint). Deleting the wrong one
		// would only burn the retry budget.
		return nil, "", false, nil
	}
	if cacheParam == "" {
		cacheParam = cacheMessageParam
	}
	cacheModelRejection := code == "invalid_parameter" || cacheMessageParam != ""
	if cacheParam != "" && cacheModelRejection {
		return removeOpenAIResponsesRejectedPromptCacheHint(body, cacheParam)
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

// openAIResponsesRejectedPromptCacheParamFromMessage resolves which explicit
// prompt-cache control an upstream 400 is pointing at. Three wordings reach us
// in practice:
//
//	prompt_cache_options is not supported on this model
//	Unsupported parameter: prompt_cache_breakpoint
//	Missing required parameter: 'input[0].content[0].prompt_cache_breakpoint.mode'
func openAIResponsesRejectedPromptCacheParamFromMessage(message string) string {
	message = strings.TrimSpace(message)
	for _, pattern := range []*regexp.Regexp{
		openAIResponsesCacheModelRejectionPattern,
		openAIResponsesMissingCacheParamPattern,
		openAIResponsesRejectedMessageParamPattern,
	} {
		match := pattern.FindStringSubmatch(message)
		if len(match) < 2 {
			continue
		}
		if param := normalizePromptCacheHintParam(match[1]); isPromptCacheHintParam(param) {
			return param
		}
	}
	return ""
}

// normalizePromptCacheHintParam lowercases a prompt-cache param path and drops
// the nested sub-field, so `input[0].content[0].prompt_cache_breakpoint.mode`
// and `input[0].content[0].prompt_cache_breakpoint` name the same hint.
func normalizePromptCacheHintParam(param string) string {
	param = strings.ToLower(strings.TrimSpace(param))
	param = strings.Trim(param, `"'`)
	param = strings.TrimSuffix(param, ".mode")
	param = strings.TrimSuffix(param, ".ttl")
	return strings.Trim(param, `"'`)
}

func isPromptCacheHintParam(param string) bool {
	param = normalizePromptCacheHintParam(param)
	switch {
	case param == "":
		return false
	case param == "prompt_cache_options", param == "prompt_cache_breakpoint":
		return true
	case openAIResponsesRejectedOptionsParamPattern.MatchString(param):
		return true
	case openAIResponsesRejectedCacheParamPattern.MatchString(param):
		return true
	case openAIResponsesRejectedCachePartPattern.MatchString(param):
		return true
	}
	return false
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

// openAIResponsesRejectedCacheIndex returns the request root ("input" or
// "messages") and item index a prompt_cache_breakpoint rejection names.
func openAIResponsesRejectedCacheIndex(param string) (string, int, bool) {
	param = normalizePromptCacheHintParam(param)
	match := openAIResponsesRejectedCacheParamPattern.FindStringSubmatch(param)
	if len(match) != 2 {
		return "", 0, false
	}
	index, err := strconv.Atoi(match[1])
	if err != nil || index < 0 {
		return "", 0, false
	}
	return promptCacheHintRoot(param), index, true
}

// openAIResponsesRejectedCachePartIndex returns the request root, the item
// index and the content-part index a nested prompt_cache_breakpoint rejection
// names (`input[0].content[1].prompt_cache_breakpoint`).
func openAIResponsesRejectedCachePartIndex(param string) (string, int, int, bool) {
	param = normalizePromptCacheHintParam(param)
	match := openAIResponsesRejectedCachePartPattern.FindStringSubmatch(param)
	if len(match) != 3 {
		return "", 0, 0, false
	}
	itemIndex, itemErr := strconv.Atoi(match[1])
	partIndex, partErr := strconv.Atoi(match[2])
	if itemErr != nil || partErr != nil || itemIndex < 0 || partIndex < 0 {
		return "", 0, 0, false
	}
	return promptCacheHintRoot(param), itemIndex, partIndex, true
}

func promptCacheHintRoot(param string) string {
	if strings.HasPrefix(normalizePromptCacheHintParam(param), "messages[") {
		return "messages"
	}
	return "input"
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

// removeOpenAIResponsesRejectedStatusAtIndex drops the status field the
// upstream rejected, and the status of every other input item sharing the
// rejected item's type.
//
// The upstream names one offending index per response, but a replayed
// conversation routinely carries dozens of items of the same type, each with a
// status its schema does not accept. Clearing one index per round trip would
// need one retry per item and exhaust the bounded retry budget long before the
// request could succeed. Items of other types keep their status: the rejection
// only proves that this type has no status field.
func removeOpenAIResponsesRejectedStatusAtIndex(body []byte, index int) ([]byte, string, bool, error) {
	itemPath := fmt.Sprintf("input.%d", index)
	rejected := gjson.GetBytes(body, itemPath)
	if !rejected.IsObject() {
		return nil, "", false, nil
	}
	if !gjson.GetBytes(body, itemPath+".status").Exists() {
		return nil, "", false, nil
	}

	retryBody := body
	cleared := 0
	rejectedType := strings.TrimSpace(rejected.Get("type").String())
	if input := gjson.GetBytes(body, "input"); rejectedType != "" && input.IsArray() {
		// Deleting a field never shifts array indexes, so positions read from
		// the original body stay valid against the rewritten one.
		for itemIndex, item := range input.Array() {
			if !item.IsObject() || strings.TrimSpace(item.Get("type").String()) != rejectedType {
				continue
			}
			statusPath := fmt.Sprintf("input.%d.status", itemIndex)
			if !gjson.GetBytes(retryBody, statusPath).Exists() {
				continue
			}
			next, err := sjson.DeleteBytes(retryBody, statusPath)
			if err != nil {
				return nil, "", false, fmt.Errorf("delete rejected status at input[%d]: %w", itemIndex, err)
			}
			retryBody = next
			cleared++
		}
	}
	if cleared == 0 {
		// The rejected item carries no type to match on; fall back to clearing
		// just the index the upstream named.
		next, err := sjson.DeleteBytes(retryBody, itemPath+".status")
		if err != nil {
			return nil, "", false, fmt.Errorf("delete rejected status at input[%d]: %w", index, err)
		}
		retryBody = next
	}
	return retryBody, "indexed status parameter rejection", true, nil
}

// removeOpenAIResponsesRejectedPromptCacheHint deletes the explicit prompt-cache
// control the upstream named, at whichever structural position it occupies.
func removeOpenAIResponsesRejectedPromptCacheHint(body []byte, param string) ([]byte, string, bool, error) {
	param = normalizePromptCacheHintParam(param)
	switch {
	case param == "prompt_cache_options" || openAIResponsesRejectedOptionsParamPattern.MatchString(param):
		return removeOpenAIResponsesPromptCacheOptions(body)
	case param == "prompt_cache_breakpoint":
		return removeOpenAIResponsesTopLevelCacheBreakpoint(body)
	}
	if root, itemIndex, partIndex, ok := openAIResponsesRejectedCachePartIndex(param); ok {
		return removeOpenAIResponsesRejectedCachePartAt(body, root, itemIndex, partIndex)
	}
	if root, index, ok := openAIResponsesRejectedCacheIndex(param); ok {
		return removeOpenAIResponsesRejectedCacheAt(body, root, index)
	}
	return nil, "", false, nil
}

func removeOpenAIResponsesPromptCacheOptions(body []byte) ([]byte, string, bool, error) {
	if !gjson.GetBytes(body, "prompt_cache_options").Exists() {
		return nil, "", false, nil
	}
	retryBody, err := sjson.DeleteBytes(body, "prompt_cache_options")
	if err != nil {
		return nil, "", false, fmt.Errorf("delete rejected prompt_cache_options: %w", err)
	}
	return retryBody, "prompt_cache_options parameter rejection", true, nil
}

func removeOpenAIResponsesTopLevelCacheBreakpoint(body []byte) ([]byte, string, bool, error) {
	if !gjson.GetBytes(body, "prompt_cache_breakpoint").Exists() {
		return nil, "", false, nil
	}
	retryBody, err := sjson.DeleteBytes(body, "prompt_cache_breakpoint")
	if err != nil {
		return nil, "", false, fmt.Errorf("delete rejected prompt_cache_breakpoint: %w", err)
	}
	// An explicit-only cache mode without breakpoints silently disables caching,
	// so drop the mode together with the hint it depended on.
	retryBody, _ = dropExplicitPromptCacheOptions(retryBody)
	return retryBody, "prompt_cache_breakpoint parameter rejection", true, nil
}

func removeOpenAIResponsesRejectedCacheAt(body []byte, root string, index int) ([]byte, string, bool, error) {
	itemPath := fmt.Sprintf("%s.%d", root, index)
	if !gjson.GetBytes(body, itemPath).IsObject() {
		return nil, "", false, nil
	}
	cachePath := itemPath + ".prompt_cache_breakpoint"
	if !gjson.GetBytes(body, cachePath).Exists() {
		return nil, "", false, nil
	}
	retryBody, err := sjson.DeleteBytes(body, cachePath)
	if err != nil {
		return nil, "", false, fmt.Errorf("delete rejected prompt_cache_breakpoint at %s[%d]: %w", root, index, err)
	}
	retryBody, _ = dropExplicitPromptCacheOptions(retryBody)
	return retryBody, "indexed prompt_cache_breakpoint parameter rejection", true, nil
}

// removeOpenAIResponsesRejectedCachePartAt drops the breakpoint the upstream
// named inside a content part (`input[0].content[1].prompt_cache_breakpoint`).
// Chat Completions requests converted to Responses carry their breakpoints
// exactly there, which is the position the top-level and item-level retries
// used to miss.
func removeOpenAIResponsesRejectedCachePartAt(body []byte, root string, itemIndex, partIndex int) ([]byte, string, bool, error) {
	partPath := fmt.Sprintf("%s.%d.content.%d", root, itemIndex, partIndex)
	if !gjson.GetBytes(body, partPath).IsObject() {
		return nil, "", false, nil
	}
	cachePath := partPath + ".prompt_cache_breakpoint"
	if !gjson.GetBytes(body, cachePath).Exists() {
		return nil, "", false, nil
	}
	retryBody, err := sjson.DeleteBytes(body, cachePath)
	if err != nil {
		return nil, "", false, fmt.Errorf("delete rejected prompt_cache_breakpoint at %s[%d].content[%d]: %w", root, itemIndex, partIndex, err)
	}
	retryBody, _ = dropExplicitPromptCacheOptions(retryBody)
	return retryBody, "content prompt_cache_breakpoint parameter rejection", true, nil
}

// dropExplicitPromptCacheOptions removes prompt_cache_options when its mode is
// explicit: with no explicit breakpoint left, that mode requests that the
// upstream write no cache at all.
func dropExplicitPromptCacheOptions(body []byte) ([]byte, bool) {
	if strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_options.mode").String())) != "explicit" {
		return body, false
	}
	next, err := sjson.DeleteBytes(body, "prompt_cache_options")
	if err != nil {
		return body, false
	}
	return next, true
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

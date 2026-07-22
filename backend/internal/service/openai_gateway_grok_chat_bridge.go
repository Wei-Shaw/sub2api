package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	grokChatResponsesEndpoint = "/v1/responses"
	grokChatRawEndpoint       = "/v1/chat/completions"
)

var grokChatResponsesBridgeTopLevelFields = map[string]struct{REDACTED{
	"model":                 {REDACTED,
	"messages":              {REDACTED,
	"instructions":          {REDACTED,
	"stream":                {REDACTED,
	"stream_options":        {REDACTED,
	"max_tokens":            {REDACTED,
	"max_completion_tokens": {REDACTED,
	"temperature":           {REDACTED,
	"top_p":                 {REDACTED,
	"stop":                  {REDACTED,
	"reasoning_effort":      {REDACTED,
	"prompt_cache_key":      {REDACTED,
	"tools":                 {REDACTED,
	"tool_choice":           {REDACTED,
	"functions":             {REDACTED,
	"function_call":         {REDACTED,
	"parallel_tool_calls":   {REDACTED,
	"response_format":       {REDACTED,
	"service_tier":          {REDACTED,
REDACTED

// grokChatResponsesBridgeEligibility deliberately accepts only request shapes
// whose Chat Completions semantics are preserved by the Responses bridge.
// Everything else stays on raw Chat Completions rather than being silently
// dropped or rewritten.
func grokChatResponsesBridgeEligibility(body []byte) (bool, string) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return false, "invalid_json"
REDACTED

	// These fields have no effect when explicitly set to JSON null. Accepting
	// that common SDK representation keeps the request on the bridge path,
	// while non-null values remain unsupported because the Responses converter
	// cannot preserve their Chat Completions semantics.
	for _, field := range []string{"stop", "reasoning_effort"REDACTED {
		if raw, exists := root[field]; exists && !grokChatJSONNull(raw) {
			return false, "unsupported_" + field
	REDACTED
REDACTED
	if raw, exists := root["instructions"]; exists {
		var instructions string
		if !grokChatJSONNull(raw) && json.Unmarshal(raw, &instructions) != nil {
			return false, "invalid_instructions"
	REDACTED
REDACTED
	if raw, exists := root["response_format"]; exists {
		var responseFormat map[string]json.RawMessage
		if !grokChatJSONNull(raw) && (json.Unmarshal(raw, &responseFormat) != nil || responseFormat == nil) {
			return false, "invalid_response_format"
	REDACTED
REDACTED
	if raw, exists := root["service_tier"]; exists {
		var serviceTier string
		if !grokChatJSONNull(raw) && json.Unmarshal(raw, &serviceTier) != nil {
			return false, "invalid_service_tier"
	REDACTED
REDACTED
	if raw, exists := root["tools"]; exists {
		if ok, reason := grokChatFunctionDeclarationsBridgeable(raw); !ok {
			return false, reason
	REDACTED
REDACTED
	if raw, exists := root["functions"]; exists && !grokChatNullOrEmptyArray(raw) {
		return false, "unsupported_functions"
REDACTED
	if raw, exists := root["tool_choice"]; exists {
		if ok, reason := grokChatToolChoiceBridgeable(raw); !ok {
			return false, reason
	REDACTED
		var choice string
		if json.Unmarshal(raw, &choice) == nil && choice == "required" && !grokChatHasFunctionDeclarations(root) {
			return false, "required_tool_choice_without_tools"
	REDACTED
REDACTED
	if raw, exists := root["function_call"]; exists && !grokChatNullOrNone(raw) {
		return false, "unsupported_function_call"
REDACTED
	for field := range root {
		if _, supported := grokChatResponsesBridgeTopLevelFields[field]; !supported {
			return false, "unknown_field_" + field
	REDACTED
REDACTED

	var model string
	if raw, ok := root["model"]; !ok || json.Unmarshal(raw, &model) != nil || strings.TrimSpace(model) == "" {
		return false, "invalid_model"
REDACTED

	if raw, ok := root["stream"]; ok {
		var stream *bool
		if json.Unmarshal(raw, &stream) != nil || stream == nil {
			return false, "invalid_stream"
	REDACTED
REDACTED
	if raw, ok := root["parallel_tool_calls"]; ok {
		var parallelToolCalls *bool
		if json.Unmarshal(raw, &parallelToolCalls) != nil || parallelToolCalls == nil {
			return false, "invalid_parallel_tool_calls"
	REDACTED
REDACTED
	if raw, ok := root["stream_options"]; ok {
		var options map[string]json.RawMessage
		if json.Unmarshal(raw, &options) != nil || options == nil {
			return false, "invalid_stream_options"
	REDACTED
		for field, value := range options {
			if field != "include_usage" {
				return false, "unknown_stream_option_" + field
		REDACTED
			var includeUsage *bool
			if json.Unmarshal(value, &includeUsage) != nil || includeUsage == nil {
				return false, "invalid_stream_include_usage"
		REDACTED
	REDACTED
REDACTED

	for _, field := range []string{"max_tokens", "max_completion_tokens"REDACTED {
		if raw, ok := root[field]; ok {
			var value *int
			if json.Unmarshal(raw, &value) != nil || value == nil || *value < 128 {
				return false, "unsafe_" + field
		REDACTED
	REDACTED
REDACTED
	if _, hasMaxTokens := root["max_tokens"]; hasMaxTokens {
		if _, hasMaxCompletionTokens := root["max_completion_tokens"]; hasMaxCompletionTokens {
			return false, "conflicting_max_tokens"
	REDACTED
REDACTED
	for _, field := range []string{"temperature", "top_p"REDACTED {
		if raw, ok := root[field]; ok {
			var value *float64
			if json.Unmarshal(raw, &value) != nil || value == nil {
				return false, "invalid_" + field
		REDACTED
	REDACTED
REDACTED
	if raw, ok := root["prompt_cache_key"]; ok {
		var key string
		if json.Unmarshal(raw, &key) != nil {
			return false, "invalid_prompt_cache_key"
	REDACTED
REDACTED

	var messages []map[string]json.RawMessage
	rawMessages, ok := root["messages"]
	if !ok || json.Unmarshal(rawMessages, &messages) != nil || len(messages) == 0 {
		return false, "invalid_messages"
REDACTED
	for _, message := range messages {
		var role string
		if raw, exists := message["role"]; !exists || json.Unmarshal(raw, &role) != nil {
			return false, "invalid_message_role"
	REDACTED
		switch role {
		case "system", "user":
			if ok, reason := grokChatMessageFieldsBridgeable(message, "role", "content"); !ok {
				return false, reason
		REDACTED
			raw, exists := message["content"]
			if !exists {
				return false, "non_text_message_content"
		REDACTED
			if ok, reason := grokChatRequiredMessageContentBridgeable(raw); !ok {
				return false, reason
		REDACTED
		case "assistant":
			if ok, reason := grokChatMessageFieldsBridgeable(message, "role", "content", "reasoning_content", "tool_calls"); !ok {
				return false, reason
		REDACTED
			reasoningContent := ""
			if raw, exists := message["reasoning_content"]; exists {
				if !grokChatJSONNull(raw) && json.Unmarshal(raw, &reasoningContent) != nil {
					return false, "invalid_reasoning_content"
			REDACTED
		REDACTED
			hasReasoningContent := strings.TrimSpace(reasoningContent) != ""
			toolCallCount := 0
			if raw, exists := message["tool_calls"]; exists {
				var reason string
				toolCallCount, reason = grokChatAssistantToolCallsBridgeable(raw)
				if reason != "" {
					return false, reason
			REDACTED
		REDACTED
			raw, hasContent := message["content"]
			if !hasContent || strings.TrimSpace(string(raw)) == "null" {
				if toolCallCount == 0 && !hasReasoningContent {
					return false, "non_text_message_content"
			REDACTED
				continue
		REDACTED
			var content string
			if json.Unmarshal(raw, &content) == nil {
				if strings.TrimSpace(content) == "" && toolCallCount == 0 && !hasReasoningContent {
					return false, "empty_message_content"
			REDACTED
				continue
		REDACTED
			if ok, reason := grokChatStructuredContentBridgeable(raw); !ok {
				// The converter can still emit a standalone reasoning part when
				// an otherwise empty content array accompanies reasoning_content.
				// Do not broaden this exception to unsupported/malformed parts.
				if !hasReasoningContent || reason != "empty_message_content" {
					return false, reason
			REDACTED
		REDACTED
		case "tool":
			if ok, reason := grokChatMessageFieldsBridgeable(message, "role", "content", "tool_call_id"); !ok {
				return false, reason
		REDACTED
			var callID string
			if raw, exists := message["tool_call_id"]; !exists || json.Unmarshal(raw, &callID) != nil || strings.TrimSpace(callID) == "" {
				return false, "invalid_tool_call_id"
		REDACTED
			var output string
			if raw, exists := message["content"]; !exists || json.Unmarshal(raw, &output) != nil || output == "" {
				return false, "invalid_tool_message_content"
		REDACTED
		default:
			return false, "unsupported_message_role_" + role
	REDACTED
REDACTED

	return true, ""
REDACTED

func grokChatFunctionDeclarationsBridgeable(raw json.RawMessage) (bool, string) {
	if strings.TrimSpace(string(raw)) == "null" {
		return true, ""
REDACTED
	var declarations []json.RawMessage
	if json.Unmarshal(raw, &declarations) != nil {
		return false, "invalid_tools"
REDACTED
	for _, declaration := range declarations {
		var tool map[string]json.RawMessage
		if json.Unmarshal(declaration, &tool) != nil || tool == nil {
			return false, "invalid_tool"
	REDACTED
		for field := range tool {
			if field != "type" && field != "function" {
				return false, "unsafe_tool_field_" + field
		REDACTED
	REDACTED
		var toolType string
		if rawType, exists := tool["type"]; !exists || json.Unmarshal(rawType, &toolType) != nil || toolType != "function" {
			return false, "unsupported_tool_type"
	REDACTED
		functionRaw, exists := tool["function"]
		if !exists {
			return false, "invalid_tool_function"
	REDACTED

		var function map[string]json.RawMessage
		if json.Unmarshal(functionRaw, &function) != nil || function == nil {
			return false, "invalid_tool_function"
	REDACTED
		for field := range function {
			switch field {
			case "name", "description", "parameters", "strict":
			default:
				return false, "unsafe_tool_function_field_" + field
		REDACTED
	REDACTED
		var name string
		if rawName, exists := function["name"]; !exists || json.Unmarshal(rawName, &name) != nil || strings.TrimSpace(name) == "" {
			return false, "invalid_tool_function_name"
	REDACTED
		if rawDescription, exists := function["description"]; exists {
			var description string
			if json.Unmarshal(rawDescription, &description) != nil {
				return false, "invalid_tool_function_description"
		REDACTED
	REDACTED
		var parameters map[string]json.RawMessage
		if rawParameters, exists := function["parameters"]; !exists || json.Unmarshal(rawParameters, &parameters) != nil || parameters == nil {
			return false, "invalid_tool_function_parameters"
	REDACTED
		if rawStrict, exists := function["strict"]; exists {
			var strict bool
			if json.Unmarshal(rawStrict, &strict) != nil {
				return false, "invalid_tool_function_strict"
		REDACTED
	REDACTED
REDACTED
	return true, ""
REDACTED

func grokChatToolChoiceBridgeable(raw json.RawMessage) (bool, string) {
	if strings.TrimSpace(string(raw)) == "null" {
		return true, ""
REDACTED
	var choice string
	if json.Unmarshal(raw, &choice) != nil {
		return false, "unsupported_tool_choice"
REDACTED
	switch choice {
	case "auto", "none", "required":
		return true, ""
	default:
		return false, "unsupported_tool_choice"
REDACTED
REDACTED

func grokChatHasFunctionDeclarations(root map[string]json.RawMessage) bool {
	for _, field := range []string{"tools", "functions"REDACTED {
		raw, exists := root[field]
		if !exists {
			continue
	REDACTED
		var declarations []json.RawMessage
		if json.Unmarshal(raw, &declarations) == nil && len(declarations) > 0 {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func grokChatMessageFieldsBridgeable(message map[string]json.RawMessage, allowedFields ...string) (bool, string) {
	allowed := make(map[string]struct{REDACTED, len(allowedFields))
	for _, field := range allowedFields {
		allowed[field] = struct{REDACTED{REDACTED
REDACTED
	for field := range message {
		if _, ok := allowed[field]; !ok {
			return false, "unsafe_message_field_" + field
	REDACTED
REDACTED
	return true, ""
REDACTED

func grokChatRequiredMessageContentBridgeable(raw json.RawMessage) (bool, string) {
	var content string
	if json.Unmarshal(raw, &content) == nil {
		if strings.TrimSpace(content) == "" {
			return false, "empty_message_content"
	REDACTED
		return true, ""
REDACTED
	// Structured content: only allow arrays whose parts are text or
	// image_url. These are losslessly convertible to Responses input_text/
	// input_image parts, so the bridge preserves Chat Completions semantics.
	return grokChatStructuredContentBridgeable(raw)
REDACTED

func grokChatAssistantToolCallsBridgeable(raw json.RawMessage) (int, string) {
	if strings.TrimSpace(string(raw)) == "null" {
		return 0, ""
REDACTED
	var calls []map[string]json.RawMessage
	if json.Unmarshal(raw, &calls) != nil {
		return 0, "invalid_tool_calls"
REDACTED
	for _, call := range calls {
		if call == nil {
			return 0, "invalid_tool_call"
	REDACTED
		for field := range call {
			switch field {
			case "id", "type", "function", "index":
			default:
				return 0, "unsafe_tool_call_field_" + field
		REDACTED
	REDACTED
		if rawIndex, exists := call["index"]; exists {
			var index *int
			if json.Unmarshal(rawIndex, &index) != nil || (index != nil && *index < 0) {
				return 0, "invalid_tool_call_index"
		REDACTED
	REDACTED
		var callID string
		if rawID, exists := call["id"]; !exists || json.Unmarshal(rawID, &callID) != nil || strings.TrimSpace(callID) == "" {
			return 0, "invalid_tool_call_id"
	REDACTED
		var callType string
		if rawType, exists := call["type"]; !exists || json.Unmarshal(rawType, &callType) != nil || callType != "function" {
			return 0, "unsupported_tool_call_type"
	REDACTED
		var function map[string]json.RawMessage
		if rawFunction, exists := call["function"]; !exists || json.Unmarshal(rawFunction, &function) != nil || function == nil {
			return 0, "invalid_tool_call_function"
	REDACTED
		for field := range function {
			if field != "name" && field != "arguments" {
				return 0, "unsafe_tool_call_function_field_" + field
		REDACTED
	REDACTED
		var name string
		if rawName, exists := function["name"]; !exists || json.Unmarshal(rawName, &name) != nil || strings.TrimSpace(name) == "" {
			return 0, "invalid_tool_call_function_name"
	REDACTED
		var arguments string
		if rawArguments, exists := function["arguments"]; !exists || json.Unmarshal(rawArguments, &arguments) != nil || !json.Valid([]byte(arguments)) {
			return 0, "invalid_tool_call_arguments"
	REDACTED
REDACTED
	return len(calls), ""
REDACTED

func grokChatStructuredContentBridgeable(raw json.RawMessage) (bool, string) {
	var parts []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return false, "non_text_message_content"
REDACTED
	if len(parts) == 0 {
		return false, "empty_message_content"
REDACTED
	hasContent := false
	for _, part := range parts {
		var partType string
		rawType, ok := part["type"]
		if !ok || json.Unmarshal(rawType, &partType) != nil {
			return false, "non_text_message_content"
	REDACTED
		switch strings.TrimSpace(partType) {
		case "text":
			var text string
			if raw, ok := part["text"]; ok && json.Unmarshal(raw, &text) == nil {
				if strings.TrimSpace(text) != "" {
					hasContent = true
			REDACTED
		REDACTED
		case "image_url", "input_image":
			hasContent = true
		default:
			return false, "unsupported_content_part_" + strings.TrimSpace(partType)
	REDACTED
REDACTED
	if !hasContent {
		return false, "empty_message_content"
REDACTED
	return true, ""
REDACTED

func grokChatNullOrNone(raw json.RawMessage) bool {
	if strings.TrimSpace(string(raw)) == "null" {
		return true
REDACTED
	var value string
	return json.Unmarshal(raw, &value) == nil && strings.EqualFold(strings.TrimSpace(value), "none")
REDACTED

func grokChatJSONNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
REDACTED

func grokChatNullOrEmptyArray(raw json.RawMessage) bool {
	if strings.TrimSpace(string(raw)) == "null" {
		return true
REDACTED
	var values []json.RawMessage
	return json.Unmarshal(raw, &values) == nil && len(values) == 0
REDACTED

func grokChatResponsesCacheIntentBody(body []byte) ([]byte, error) {
	// An empty Chat tools array is omitted by the Responses converter. In that
	// case auto/none is also a semantic no-op and must not suppress the normal
	// tool-free cache route. Non-empty converted tools are always kept intact.
	if gjson.GetBytes(body, "tools").Exists() {
		return append([]byte(nil), body...), nil
REDACTED
	choice := gjson.GetBytes(body, "tool_choice")
	if !choice.Exists() || choice.Type != gjson.String || (choice.String() != "auto" && choice.String() != "none") {
		return append([]byte(nil), body...), nil
REDACTED
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
REDACTED
	delete(root, "tool_choice")
	return json.Marshal(root)
REDACTED

func grokChatResponsesRuntimeEligible(upstreamModel, cacheIdentity string) bool {
	return strings.TrimSpace(upstreamModel) == "grok-4.5" && strings.TrimSpace(cacheIdentity) != ""
REDACTED

// forwardGrokChatCompletionsViaResponses converts a strictly compatible Chat
// request into xAI Responses format and reuses the established Responses-to-
// Chat response translators. It intentionally does not run the Codex OAuth
// transform because Grok CLI is a separate upstream protocol.
func (s *OpenAIGatewayService) forwardGrokChatCompletionsViaResponses(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	promptCacheKey string,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	var chatReq apicompat.ChatCompletionsRequest
	if err := json.Unmarshal(body, &chatReq); err != nil {
		return nil, fmt.Errorf("parse grok chat completions request: %w", err)
REDACTED
	originalModel := chatReq.Model
	clientStream := chatReq.Stream
	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	cacheIdentity := resolveGrokCacheIdentity(c, body, promptCacheKey, upstreamModel)
	// Image inputs must go through the Responses bridge: the raw Chat
	// Completions path cannot forward image_url parts to Grok's native vision
	// for non-composer models, so they would be silently dropped. Route them to
	// Responses even when no prompt-cache identity is available.
	hasImageInput := openAIJSONValueMayContainImageInput(gjson.GetBytes(body, "messages"))
	if !grokChatResponsesRuntimeEligible(upstreamModel, cacheIdentity) && (!hasImageInput || strings.TrimSpace(upstreamModel) != "grok-4.5") {
		return s.forwardAsRawChatCompletions(ctx, c, account, body, defaultMappedModel)
REDACTED

	responsesReq, err := apicompat.ChatCompletionsToResponses(&chatReq)
	if err != nil {
		return nil, fmt.Errorf("convert grok chat completions to responses: %w", err)
REDACTED
	responsesReq.Model = upstreamModel
	responsesReq.Stream = true
	// Keep Chat and native Responses paths aligned for OpenAI-compatible
	// service_tier aliases (for example, "fast" -> "priority"). Unknown
	// values are omitted by the shared normalizer instead of reaching xAI.
	normalizeResponsesRequestServiceTier(responsesReq)
	// These fields are useful to Codex but are not needed by the Grok CLI
	// protocol. Keep the bridge request as close as possible to native Grok.
	responsesReq.Include = nil
	responsesReq.Store = nil

	responsesBody, err := json.Marshal(responsesReq)
	if err != nil {
		return nil, fmt.Errorf("marshal grok responses bridge request: %w", err)
REDACTED
	// Preserve the converted Responses intent before Grok capability
	// sanitization. Cache routing must see the actual client function tools,
	// not the nested Chat Completions declarations and not a tool-free copy.
	intentBody, err := grokChatResponsesCacheIntentBody(responsesBody)
	if err != nil {
		return nil, fmt.Errorf("normalize grok responses bridge cache intent: %w", err)
REDACTED
	responsesBody, err = patchGrokResponsesBody(responsesBody, upstreamModel)
	if err != nil {
		return nil, fmt.Errorf("patch grok responses bridge request: %w", err)
REDACTED
	responsesBody, err = applyGrokResponsesCacheIdentity(responsesBody, intentBody, cacheIdentity, true)
	if err != nil {
		return nil, fmt.Errorf("apply grok responses bridge cache identity: %w", err)
REDACTED
	responsesBody, err = applyGrokFreeRequestToolCacheRoute(c, responsesBody, intentBody, account, cacheIdentity)
	if err != nil {
		return nil, fmt.Errorf("apply grok responses bridge function-tool cache route: %w", err)
REDACTED

	updatedBody, policyErr := s.applyOpenAIFastPolicyToBody(ctx, account, upstreamModel, responsesBody)
	if policyErr != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(policyErr, &blocked) {
			MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
			writeChatCompletionsError(c, http.StatusForbidden, "permission_error", blocked.Message)
	REDACTED
		return nil, policyErr
REDACTED
	responsesBody = updatedBody

	token, _, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, fmt.Errorf("get grok access token: %w", err)
REDACTED
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := buildGrokResponsesRequest(upstreamCtx, c, account, responsesBody, token, cacheIdentity, s.cfg)
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build grok responses bridge request: %w", err)
REDACTED
	SetActualOpenAIUpstreamEndpoint(c, grokChatResponsesEndpoint)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode >= http.StatusBadRequest {
		respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
		if upstreamMsg == "" {
			upstreamMsg = fmt.Sprintf("xAI upstream returned status %d", resp.StatusCode)
	REDACTED
		kind := "http_error"
		if s.shouldFailoverGrokUpstreamError(resp.StatusCode, respBody) {
			kind = "failover"
	REDACTED
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
			Kind:               kind,
			Message:            upstreamMsg,
	REDACTED)
		s.handleGrokAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		if s.shouldFailoverGrokUpstreamError(resp.StatusCode, respBody) {
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				ResponseHeaders:        resp.Header.Clone(),
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
		REDACTED
	REDACTED
		return s.handleChatCompletionsErrorResponse(resp, c, account, billingModel)
REDACTED

	s.updateGrokUsageFromResponse(ctx, account, resp.Header, resp.StatusCode)

	var result *OpenAIForwardResult
	if clientStream {
		result, err = s.handleChatStreamingResponse(resp, c, account, originalModel, billingModel, upstreamModel, startTime, len(body))
REDACTED else {
		result, err = s.handleChatBufferedStreamingResponse(resp, c, account, originalModel, billingModel, upstreamModel, startTime)
REDACTED
	if result != nil {
		result.UpstreamEndpoint = grokChatResponsesEndpoint
		result.ResponseHeaders = resp.Header.Clone()
		if result.RequestID == "" {
			result.RequestID = firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id"))
	REDACTED
		result.ReasoningEffort = extractOpenAIReasoningEffortFromBody(body, upstreamModel, billingModel, originalModel)
REDACTED
	return result, err
REDACTED

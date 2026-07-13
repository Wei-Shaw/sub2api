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
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
)

const (
	grokChatResponsesEndpoint = "/v1/responses"
	grokChatRawEndpoint       = "/v1/chat/completions"
)

var grokChatResponsesBridgeTopLevelFields = map[string]struct{REDACTED{
	"model":                 {REDACTED,
	"messages":              {REDACTED,
	"stream":                {REDACTED,
	"stream_options":        {REDACTED,
	"max_tokens":            {REDACTED,
	"max_completion_tokens": {REDACTED,
	"temperature":           {REDACTED,
	"top_p":                 {REDACTED,
	"prompt_cache_key":      {REDACTED,
	"tools":                 {REDACTED,
	"tool_choice":           {REDACTED,
	"functions":             {REDACTED,
	"function_call":         {REDACTED,
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

	for _, field := range []string{"stop", "reasoning_effort"REDACTED {
		if _, exists := root[field]; exists {
			return false, "unsupported_" + field
	REDACTED
REDACTED
	for _, field := range []string{"tools", "functions"REDACTED {
		if raw, exists := root[field]; exists && !grokChatNullOrEmptyArray(raw) {
			return false, "unsupported_" + field
	REDACTED
REDACTED
	if raw, exists := root["tool_choice"]; exists && !grokChatNullOrNone(raw) {
		return false, "unsupported_tool_choice"
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
		for field := range message {
			if field != "role" && field != "content" {
				return false, "unsafe_message_field_" + field
		REDACTED
	REDACTED
		var role string
		if raw, exists := message["role"]; !exists || json.Unmarshal(raw, &role) != nil {
			return false, "invalid_message_role"
	REDACTED
		switch role {
		case "system", "user", "assistant":
		default:
			return false, "unsupported_message_role_" + role
	REDACTED
		var content string
		if raw, exists := message["content"]; !exists || json.Unmarshal(raw, &content) != nil {
			// Structured content includes image_url and other parts whose exact
			// behavior is not guaranteed by this bridge.
			return false, "non_text_message_content"
	REDACTED
		if strings.TrimSpace(content) == "" {
			return false, "empty_message_content"
	REDACTED
REDACTED

	return true, ""
REDACTED

func grokChatNullOrEmptyArray(raw json.RawMessage) bool {
	if strings.TrimSpace(string(raw)) == "null" {
		return true
REDACTED
	var values []json.RawMessage
	return json.Unmarshal(raw, &values) == nil && len(values) == 0
REDACTED

func grokChatNullOrNone(raw json.RawMessage) bool {
	if strings.TrimSpace(string(raw)) == "null" {
		return true
REDACTED
	var value string
	return json.Unmarshal(raw, &value) == nil && strings.EqualFold(strings.TrimSpace(value), "none")
REDACTED

func grokChatCacheIntentBody(body []byte) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
REDACTED
	for _, field := range []string{"tools", "tool_choice", "functions", "function_call"REDACTED {
		delete(root, field)
REDACTED
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
	if !grokChatResponsesRuntimeEligible(upstreamModel, cacheIdentity) {
		return s.forwardAsRawChatCompletions(ctx, c, account, body, defaultMappedModel)
REDACTED

	responsesReq, err := apicompat.ChatCompletionsToResponses(&chatReq)
	if err != nil {
		return nil, fmt.Errorf("convert grok chat completions to responses: %w", err)
REDACTED
	responsesReq.Model = upstreamModel
	responsesReq.Stream = true
	// These fields are useful to Codex but are not needed by the Grok CLI
	// protocol. Keep the bridge request as close as possible to native Grok.
	responsesReq.Include = nil
	responsesReq.Store = nil

	responsesBody, err := json.Marshal(responsesReq)
	if err != nil {
		return nil, fmt.Errorf("marshal grok responses bridge request: %w", err)
REDACTED
	responsesBody, err = patchGrokResponsesBody(responsesBody, upstreamModel)
	if err != nil {
		return nil, fmt.Errorf("patch grok responses bridge request: %w", err)
REDACTED
	intentBody, err := grokChatCacheIntentBody(body)
	if err != nil {
		return nil, fmt.Errorf("normalize grok responses bridge tool intent: %w", err)
REDACTED
	responsesBody, err = applyGrokResponsesCacheIdentity(responsesBody, intentBody, cacheIdentity, true)
	if err != nil {
		return nil, fmt.Errorf("apply grok responses bridge cache identity: %w", err)
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

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("get grok access token: %w", err)
REDACTED
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := buildGrokResponsesRequest(upstreamCtx, c, account, responsesBody, token, cacheIdentity)
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
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
			Kind:               "failover",
			Message:            upstreamMsg,
	REDACTED)
		s.handleGrokAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
		REDACTED
	REDACTED
		return s.handleChatCompletionsErrorResponse(resp, c, account, billingModel)
REDACTED

	s.updateGrokUsageSnapshot(ctx, account, xai.ParseQuotaHeaders(resp.Header, resp.StatusCode))

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

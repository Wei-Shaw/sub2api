package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	grokComposerImageBridgeVisionModel     = "grok-build-0.1"
	grokComposerImageBridgeMaxOutputTokens = 512
	grokUpstreamUserAgent                  = "sub2api-grok/1.0"
	grokCLIVersion                         = "0.2.93"
	grokDefaultResponsesModel              = "grok-4.5"
	grokRateLimitFallbackCooldown          = 2 * time.Minute
	grokRateLimitRepeatCooldown            = 10 * time.Minute
	grokRateLimitSustainedCooldown         = 30 * time.Minute
	grokRateLimitMaxAdaptiveCooldown       = time.Hour
	grokRateLimitBackoffQuietPeriod        = time.Hour
)

func (s *OpenAIGatewayService) forwardGrokResponses(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	originalModel string,
	reqStream bool,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	if account.Type != AccountTypeOAuth && account.Type != AccountTypeAPIKey {
		return nil, fmt.Errorf("grok account type %s is not supported by Responses forwarding", account.Type)
REDACTED

	upstreamModel := account.GetMappedModel(originalModel)
	if strings.TrimSpace(upstreamModel) == "" {
		upstreamModel = grokDefaultResponsesModel
REDACTED
	if isGrokImageGenerationModel(upstreamModel) {
		return nil, fmt.Errorf("model %s is an image model and is not available on the Responses endpoint; use /v1/images/generations instead", upstreamModel)
REDACTED
	patchedBody, clientToolMapping, err := patchGrokResponsesBodyWithClientTools(body, upstreamModel)
	if err != nil {
		setOpsUpstreamError(c, http.StatusBadRequest, err.Error(), "")
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type": "invalid_request_error", "message": err.Error(), "param": "tools",
	REDACTEDREDACTED)
		return nil, err
REDACTED
	setGrokResponsesClientToolMapping(c, clientToolMapping)
	// OpenAI /responses/compact is not a native xAI endpoint. Convert it into a
	// normal Grok Responses turn that asks for a structured summary, then map the
	// reply back to an OpenAI compaction item on the way out.
	if isOpenAIResponsesCompactPath(c) {
		patchedBody, err = buildGrokCompactRequestBody(patchedBody)
		if err != nil {
			return nil, err
	REDACTED
REDACTED
	// Derive the identity from the request xAI will actually see. This makes
	// Codex Responses Lite additional_tools part of the stable tool prefix.
	cacheIdentity := resolveGrokCacheIdentity(c, patchedBody, "", upstreamModel)
	mixedCacheIntentBody := append([]byte(nil), patchedBody...)
	patchedBody, err = applyGrokResponsesCacheIdentity(patchedBody, body, cacheIdentity, account.IsGrokOAuth())
	if err != nil {
		return nil, fmt.Errorf("apply grok prompt cache identity: %w", err)
REDACTED
	// Free OAuth + client function tools: reuse Messages mixed-tools cache route
	// (append web_search/x_search so xAI does not force non-cacheable build-free).
	patchedBody, err = applyGrokFreeRequestToolCacheRoute(c, patchedBody, mixedCacheIntentBody, account, cacheIdentity)
	if err != nil {
		return nil, fmt.Errorf("apply grok Free function-tool cache route: %w", err)
REDACTED

	token, _, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, err
REDACTED

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	upstreamStart := time.Now()
	var resp *http.Response
	for attempt := 0; ; attempt++ {
		upstreamReq, buildErr := buildGrokResponsesRequest(upstreamCtx, c, account, patchedBody, token, cacheIdentity, s.cfg)
		if buildErr != nil {
			return nil, buildErr
	REDACTED

		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		if err != nil {
			return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	REDACTED

		// xAI can reject encrypted reasoning copied from a response produced under
		// another account or cache identity. Retry once with the same routing and
		// credential after removing only the rejected encrypted reasoning payload.
		if attempt > 0 || resp.StatusCode != http.StatusBadRequest {
			break
	REDACTED
		respBody := s.readUpstreamErrorBody(resp)
		if resp.Body != nil {
			_ = resp.Body.Close()
	REDACTED
		if !isGrokInvalidEncryptedContentResponse(resp.StatusCode, respBody) {
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			break
	REDACTED

		retryBody, changed, trimErr := trimGrokInvalidEncryptedContentRetryBody(patchedBody)
		if trimErr != nil {
			return nil, fmt.Errorf("prepare Grok invalid encrypted_content retry: %w", trimErr)
	REDACTED
		if !changed {
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			break
	REDACTED

		patchedBody = retryBody
		slog.Info("grok_invalid_encrypted_content_retry", "account_id", account.ID, "cache_identity_present", cacheIdentity != "")
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMsg := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(respBody))
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
				ResponseHeaders:        resp.Header.Clone(),
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
		REDACTED
	REDACTED
		return s.handleErrorResponse(ctx, resp, c, account, patchedBody, upstreamModel)
REDACTED

	s.updateGrokUsageFromResponse(ctx, account, resp.Header, resp.StatusCode)

	var usage *OpenAIUsage
	var firstTokenMs *int
	responseID := ""
	if reqStream {
		if hasGrokResponsesClientToolMapping(clientToolMapping) {
			maxLineSize := defaultMaxLineSize
			if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
				maxLineSize = s.cfg.Gateway.MaxLineSize
		REDACTED
			resp.Body = newGrokResponsesClientToolStreamBody(resp.Body, clientToolMapping, maxLineSize)
	REDACTED
		streamResult, err := s.handleStreamingResponse(ctx, resp, c, account, startTime, originalModel, upstreamModel)
		if err != nil {
			return nil, err
	REDACTED
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
		responseID = strings.TrimSpace(streamResult.responseID)
REDACTED else {
		nonStreamResult, err := s.handleNonStreamingResponse(ctx, resp, c, account, originalModel, upstreamModel)
		if err != nil {
			return nil, err
	REDACTED
		usage = nonStreamResult.usage
		responseID = strings.TrimSpace(nonStreamResult.responseID)
REDACTED

	if usage == nil {
		usage = &OpenAIUsage{REDACTED
REDACTED
	reasoningEffort := extractOpenAIReasoningEffortFromBody(patchedBody, originalModel)
	return &OpenAIForwardResult{
		RequestID:       firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
		ResponseID:      responseID,
		Usage:           *usage,
		Model:           originalModel,
		UpstreamModel:   upstreamModel,
		ReasoningEffort: reasoningEffort,
		Stream:          reqStream,
		OpenAIWSMode:    false,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
REDACTED, nil
REDACTED

func isGrokInvalidEncryptedContentResponse(statusCode int, body []byte) bool {
	if statusCode != http.StatusBadRequest {
		return false
REDACTED

	// xAI has used both flat and nested error envelopes:
	//   {"code":"invalid-argument","error":"Could not decrypt the provided encrypted_content."REDACTED
	//   {"error":{"message":"Could not decrypt the provided encrypted_content."REDACTEDREDACTED
	code := strings.TrimSpace(gjson.GetBytes(body, "code").String())
	message := ""
	errNode := gjson.GetBytes(body, "error")
	switch {
	case errNode.Type == gjson.String:
		message = errNode.String()
	case errNode.IsObject():
		message = firstNonEmpty(errNode.Get("message").String(), errNode.Get("error").String())
		if code == "" {
			code = strings.TrimSpace(errNode.Get("code").String())
	REDACTED
	default:
		message = gjson.GetBytes(body, "message").String()
REDACTED
	normalizedMessage := strings.ToLower(strings.TrimSpace(message))
	if normalizedMessage == "" {
		return false
REDACTED

	if strings.EqualFold(code, "invalid_encrypted_content") {
		return true
REDACTED
	// Keep the official xAI flat-code gate so unrelated 400s are not retried.
	if !strings.EqualFold(code, "invalid-argument") && code != "" {
		return false
REDACTED
	// Nested OpenAI-style envelopes may omit top-level code; require decrypt text.
	if code == "" && !strings.Contains(normalizedMessage, "decrypt") {
		return false
REDACTED
	return strings.Contains(normalizedMessage, "encrypted_content") &&
		(strings.Contains(normalizedMessage, "decrypt") ||
			strings.Contains(normalizedMessage, "unmodified"))
REDACTED

// requestHasGrokEncryptedReasoning reports whether the outbound Responses body
// still carries reasoning.encrypted_content that can be stripped for retry.
func requestHasGrokEncryptedReasoning(body []byte) bool {
	input := gjson.GetBytes(body, "input")
	if !input.Exists() {
		return false
REDACTED
	items := input.Array()
	if input.IsObject() {
		items = []gjson.Result{inputREDACTED
REDACTED
	for _, item := range items {
		if strings.TrimSpace(item.Get("type").String()) != "reasoning" {
			continue
	REDACTED
		enc := item.Get("encrypted_content")
		if enc.Exists() && enc.Type != gjson.Null && strings.TrimSpace(enc.String()) != "" {
			return true
	REDACTED
REDACTED
	return false
REDACTED

type grokEncryptedContentStripRetriedKey struct{REDACTED

func markGrokEncryptedContentStripRetried(ctx context.Context) context.Context {
	return context.WithValue(ctx, grokEncryptedContentStripRetriedKey{REDACTED, true)
REDACTED

func grokEncryptedContentStripRetried(ctx context.Context) bool {
	v, _ := ctx.Value(grokEncryptedContentStripRetriedKey{REDACTED).(bool)
	return v
REDACTED

// stripAnthropicThinkingSignatures removes thinking.signature from Claude
// history so a different Grok OAuth account can accept multi-turn tool
// continuations after decrypt failures. Returns ok=false when nothing changed.
func stripAnthropicThinkingSignatures(body []byte) ([]byte, bool) {
	if len(body) == 0 || !bytes.Contains(body, []byte(`"signature"`)) {
		return body, false
REDACTED
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body, false
REDACTED
	messages, ok := req["messages"].([]any)
	if !ok || len(messages) == 0 {
		return body, false
REDACTED
	changed := false
	for _, rawMsg := range messages {
		msg, ok := rawMsg.(map[string]any)
		if !ok {
			continue
	REDACTED
		content, ok := msg["content"].([]any)
		if !ok {
			continue
	REDACTED
		for _, rawBlock := range content {
			block, ok := rawBlock.(map[string]any)
			if !ok {
				continue
		REDACTED
			if typ, _ := block["type"].(string); typ != "thinking" {
				continue
		REDACTED
			if _, has := block["signature"]; has {
				delete(block, "signature")
				changed = true
		REDACTED
	REDACTED
REDACTED
	if !changed {
		return body, false
REDACTED
	out, err := json.Marshal(req)
	if err != nil {
		return body, false
REDACTED
	return out, true
REDACTED

func trimGrokInvalidEncryptedContentRetryBody(body []byte) ([]byte, bool, error) {
	input := gjson.GetBytes(body, "input")
	items := input.Array()
	if input.IsObject() {
		items = []gjson.Result{inputREDACTED
REDACTED

	hasEncryptedReasoning := false
	for _, item := range items {
		if strings.TrimSpace(item.Get("type").String()) == "reasoning" && item.Get("encrypted_content").Exists() {
			hasEncryptedReasoning = true
			break
	REDACTED
REDACTED
	if !hasEncryptedReasoning {
		return body, false, nil
REDACTED

	var requestBody map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&requestBody); err != nil {
		return nil, false, err
REDACTED
	if !trimOpenAIEncryptedReasoningItems(requestBody) {
		return body, false, nil
REDACTED

	retryBody, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return nil, false, err
REDACTED
	return retryBody, true, nil
REDACTED

func patchGrokResponsesBody(body []byte, upstreamModel string) ([]byte, error) {
	return patchGrokResponsesBodyBase(body, upstreamModel)
REDACTED

func patchGrokResponsesBodyWithClientTools(body []byte, upstreamModel string) ([]byte, apicompat.ResponsesClientToolMapping, error) {
	if !json.Valid(body) {
		return nil, apicompat.ResponsesClientToolMapping{REDACTED, fmt.Errorf("invalid json request body")
REDACTED
	promoted, err := sanitizeGrokResponsesInput(body)
	if err != nil {
		return nil, apicompat.ResponsesClientToolMapping{REDACTED, err
REDACTED
	adapted, mapping, err := adaptGrokResponsesClientTools(promoted)
	if err != nil {
		return nil, apicompat.ResponsesClientToolMapping{REDACTED, err
REDACTED
	patched, err := patchGrokResponsesBodyBase(adapted, upstreamModel)
	if err != nil {
		return nil, apicompat.ResponsesClientToolMapping{REDACTED, err
REDACTED
	return patched, mapping, nil
REDACTED

func patchGrokResponsesBodyBase(body []byte, upstreamModel string) ([]byte, error) {
	if !json.Valid(body) {
		return nil, fmt.Errorf("invalid json request body")
REDACTED
	out, err := sjson.SetBytes(body, "model", upstreamModel)
	if err != nil {
		return nil, err
REDACTED
	out, err = sanitizeGrokResponsesModelCapabilities(out, upstreamModel)
	if err != nil {
		return nil, err
REDACTED
	for _, unsupportedField := range []string{"prompt_cache_retention", "safety_identifier"REDACTED {
		if gjson.GetBytes(out, unsupportedField).Exists() {
			out, err = sjson.DeleteBytes(out, unsupportedField)
			if err != nil {
				return nil, err
		REDACTED
	REDACTED
REDACTED
	if strings.EqualFold(upstreamModel, "grok-4.5") {
		for _, unsupportedField := range []string{"presence_penalty", "presencePenalty", "frequency_penalty", "frequencyPenalty", "stop"REDACTED {
			if gjson.GetBytes(out, unsupportedField).Exists() {
				out, err = sjson.DeleteBytes(out, unsupportedField)
				if err != nil {
					return nil, err
			REDACTED
		REDACTED
	REDACTED
REDACTED
	out, err = sanitizeGrokResponsesUnsupportedFields(out)
	if err != nil {
		return nil, err
REDACTED
	out, err = convertOpenAICompactInputsForGrok(out)
	if err != nil {
		return nil, err
REDACTED
	out, err = sanitizeGrokResponsesInput(out)
	if err != nil {
		return nil, err
REDACTED
	out, err = sanitizeGrokReasoningNullContent(out)
	if err != nil {
		return nil, err
REDACTED
	out, err = sanitizeGrokResponsesTools(out)
	if err != nil {
		return nil, err
REDACTED
	return out, nil
REDACTED

func sanitizeGrokResponsesModelCapabilities(body []byte, upstreamModel string) ([]byte, error) {
	if !grokModelRejectsReasoningEffort(upstreamModel) {
		return body, nil
REDACTED

	out := body
	for _, field := range []string{"reasoning", "reasoning_effort", "reasoningEffort"REDACTED {
		if !gjson.GetBytes(out, field).Exists() {
			continue
	REDACTED
		var err error
		out, err = sjson.DeleteBytes(out, field)
		if err != nil {
			return nil, fmt.Errorf("remove unsupported Grok Composer %s: %w", field, err)
	REDACTED
REDACTED
	return out, nil
REDACTED

func grokModelRejectsReasoningEffort(model string) bool {
	model = strings.TrimSpace(strings.ToLower(model))
	if slash := strings.LastIndex(model, "/"); slash >= 0 {
		model = strings.TrimSpace(model[slash+1:])
REDACTED
	switch model {
	case "grok-composer", "grok-composer-2.5-fast", "composer-2.5":
		return true
	default:
		return false
REDACTED
REDACTED

var grokResponsesUnsupportedRecursiveFields = map[string]struct{REDACTED{
	"external_web_access": {REDACTED,
REDACTED

func sanitizeGrokResponsesUnsupportedFields(body []byte) ([]byte, error) {
	if !bytes.Contains(body, []byte(`"external_web_access"`)) {
		return body, nil
REDACTED

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
REDACTED
	if !deleteJSONFields(payload, grokResponsesUnsupportedRecursiveFields) {
		return body, nil
REDACTED
	return json.Marshal(payload)
REDACTED

func deleteJSONFields(value any, fields map[string]struct{REDACTED) bool {
	switch typed := value.(type) {
	case map[string]any:
		changed := false
		for field := range fields {
			if _, ok := typed[field]; ok {
				delete(typed, field)
				changed = true
		REDACTED
	REDACTED
		for _, child := range typed {
			if deleteJSONFields(child, fields) {
				changed = true
		REDACTED
	REDACTED
		return changed
	case []any:
		changed := false
		for _, child := range typed {
			if deleteJSONFields(child, fields) {
				changed = true
		REDACTED
	REDACTED
		return changed
	default:
		return false
REDACTED
REDACTED

// additional_tools is a Codex/Responses Lite private input carrier. xAI's
// Responses schema rejects the carrier itself, but accepts supported tools at
// the top level. Preserve top-level order, append newly discovered tools in
// carrier order, then let sanitizeGrokResponsesTools filter unsupported types.
func sanitizeGrokResponsesInput(body []byte) ([]byte, error) {
	if !bytes.Contains(body, []byte(`"additional_tools"`)) {
		return body, nil
REDACTED
	input := gjson.GetBytes(body, "input")
	if !input.Exists() || !input.IsArray() {
		return body, nil
REDACTED

	rawItems := input.Array()
	filtered := make([]json.RawMessage, 0, len(rawItems))
	topLevelTools := gjson.GetBytes(body, "tools")
	mergedTools := make([]json.RawMessage, 0)
	seenTools := make(map[string]struct{REDACTED)
	appendTool := func(tool gjson.Result) bool {
		key := grokResponsesToolDedupKey(tool)
		if _, exists := seenTools[key]; exists {
			return false
	REDACTED
		seenTools[key] = struct{REDACTED{REDACTED
		mergedTools = append(mergedTools, json.RawMessage(tool.Raw))
		return true
REDACTED
	if topLevelTools.IsArray() {
		for _, tool := range topLevelTools.Array() {
			seenTools[grokResponsesToolDedupKey(tool)] = struct{REDACTED{REDACTED
			mergedTools = append(mergedTools, json.RawMessage(tool.Raw))
	REDACTED
REDACTED

	promoted := false
	for _, item := range rawItems {
		if strings.TrimSpace(item.Get("type").String()) == "additional_tools" {
			tools := item.Get("tools")
			if tools.IsArray() {
				for _, tool := range tools.Array() {
					if appendTool(tool) {
						promoted = true
				REDACTED
			REDACTED
		REDACTED
			continue
	REDACTED
		filtered = append(filtered, json.RawMessage(item.Raw))
REDACTED
	if len(filtered) == len(rawItems) {
		return body, nil
REDACTED
	encoded, err := json.Marshal(filtered)
	if err != nil {
		return nil, err
REDACTED
	body, err = sjson.SetRawBytes(body, "input", encoded)
	if err != nil || !promoted {
		return body, err
REDACTED
	encodedTools, err := json.Marshal(mergedTools)
	if err != nil {
		return nil, err
REDACTED
	return sjson.SetRawBytes(body, "tools", encodedTools)
REDACTED

func grokResponsesToolDedupKey(tool gjson.Result) string {
	toolType := strings.TrimSpace(tool.Get("type").String())
	if toolType != "" {
		if name := strings.TrimSpace(tool.Get("name").String()); name != "" {
			return "type:" + toolType + "\x00name:" + name
	REDACTED
		if toolType == "mcp" {
			if label := strings.TrimSpace(tool.Get("server_label").String()); label != "" {
				return "type:mcp\x00server_label:" + label
		REDACTED
	REDACTED
REDACTED
	return "json:" + normalizeCompatSeedJSON(json.RawMessage(tool.Raw))
REDACTED

// sanitizeGrokReasoningNullContent 删除 reasoning 项中的 "content": null。
// xAI 的 untagged enum 反序列化器拒收该字段，返回 422。
func sanitizeGrokReasoningNullContent(body []byte) ([]byte, error) {
	input := gjson.GetBytes(body, "input")
	if !input.Exists() || !input.IsArray() {
		return body, nil
REDACTED

	items := input.Array()
	changed := false
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		if strings.TrimSpace(item.Get("type").String()) != "reasoning" {
			continue
	REDACTED
		contentResult := item.Get("content")
		if contentResult.Exists() && contentResult.Type == gjson.Null {
			var err error
			body, err = sjson.DeleteBytes(body, fmt.Sprintf("input.%d.content", i))
			if err != nil {
				return nil, err
		REDACTED
			changed = true
	REDACTED
REDACTED
	_ = changed
	return body, nil
REDACTED

var grokResponsesSupportedToolTypes = map[string]struct{REDACTED{
	"code_execution":     {REDACTED,
	"code_interpreter":   {REDACTED,
	"collections_search": {REDACTED,
	"file_search":        {REDACTED,
	"function":           {REDACTED,
	"mcp":                {REDACTED,
	"shell":              {REDACTED,
	"web_search":         {REDACTED,
	"x_search":           {REDACTED,
REDACTED

func sanitizeGrokResponsesTools(body []byte) ([]byte, error) {
	tools := gjson.GetBytes(body, "tools")
	if !tools.Exists() || !tools.IsArray() {
		return body, nil
REDACTED

	rawTools := tools.Array()
	filteredTools := make([]json.RawMessage, 0, len(rawTools))
	for _, tool := range rawTools {
		toolType := strings.TrimSpace(tool.Get("type").String())
		if _, ok := grokResponsesSupportedToolTypes[toolType]; ok {
			filteredTools = append(filteredTools, json.RawMessage(tool.Raw))
	REDACTED
REDACTED

	var err error
	if len(filteredTools) != len(rawTools) {
		if len(filteredTools) == 0 {
			body, err = sjson.DeleteBytes(body, "tools")
	REDACTED else {
			var encoded []byte
			encoded, err = json.Marshal(filteredTools)
			if err != nil {
				return nil, err
		REDACTED
			body, err = sjson.SetRawBytes(body, "tools", encoded)
	REDACTED
		if err != nil {
			return nil, err
	REDACTED
REDACTED

	toolChoice := gjson.GetBytes(body, "tool_choice")
	if !toolChoice.Exists() {
		return body, nil
REDACTED
	if shouldDropGrokToolChoice(toolChoice, filteredTools) {
		body, err = sjson.DeleteBytes(body, "tool_choice")
		if err != nil {
			return nil, err
	REDACTED
REDACTED
	return body, nil
REDACTED

func shouldDropGrokToolChoice(toolChoice gjson.Result, tools []json.RawMessage) bool {
	if len(tools) == 0 {
		return true
REDACTED
	if !toolChoice.IsObject() {
		return false
REDACTED
	choiceType := strings.TrimSpace(toolChoice.Get("type").String())
	if choiceType == "" {
		return false
REDACTED
	if _, ok := grokResponsesSupportedToolTypes[choiceType]; !ok {
		return true
REDACTED
	if choiceType == "function" {
		choiceName := strings.TrimSpace(toolChoice.Get("name").String())
		if choiceName == "" {
			choiceName = strings.TrimSpace(toolChoice.Get("function.name").String())
	REDACTED
		if choiceName == "" {
			return false
	REDACTED
		for _, tool := range tools {
			var item struct {
				Type     string `json:"type"`
				Name     string `json:"name"`
				Function struct {
					Name string `json:"name"`
			REDACTED `json:"function"`
		REDACTED
			if err := json.Unmarshal(tool, &item); err != nil {
				continue
		REDACTED
			name := strings.TrimSpace(item.Name)
			if name == "" {
				name = strings.TrimSpace(item.Function.Name)
		REDACTED
			if strings.TrimSpace(item.Type) == "function" && name == choiceName {
				return false
		REDACTED
	REDACTED
		return true
REDACTED
	return false
REDACTED

func (s *OpenAIGatewayService) bridgeGrokComposerImageInputs(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
) ([]byte, OpenAIUsage, bool, error) {
	if !shouldBridgeGrokComposerImageInputs(body) {
		return body, OpenAIUsage{REDACTED, false, nil
REDACTED

	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return body, OpenAIUsage{REDACTED, false, fmt.Errorf("parse grok composer image bridge request: %w", err)
REDACTED

	imageURLs := collectGrokComposerImageURLs(reqBody)
	if len(imageURLs) == 0 {
		return body, OpenAIUsage{REDACTED, false, nil
REDACTED

	descriptions := make([]string, 0, len(imageURLs))
	var bridgeUsage OpenAIUsage
	for index, imageURL := range imageURLs {
		description, usage, err := s.describeGrokComposerImage(ctx, c, account, token, imageURL, index+1)
		if err != nil {
			return body, bridgeUsage, false, err
	REDACTED
		descriptions = append(descriptions, description)
		addOpenAIUsage(&bridgeUsage, usage)
REDACTED

	if !rewriteGrokComposerImagesAsText(reqBody, descriptions) {
		return body, bridgeUsage, false, nil
REDACTED
	bridgedBody, err := marshalOpenAIUpstreamJSON(reqBody)
	if err != nil {
		return body, bridgeUsage, false, fmt.Errorf("serialize grok composer image bridge request: %w", err)
REDACTED
	return bridgedBody, bridgeUsage, true, nil
REDACTED

func shouldBridgeGrokComposerImageInputs(body []byte) bool {
	if len(body) == 0 || !isGrokComposerModel(gjson.GetBytes(body, "model").String()) {
		return false
REDACTED
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() {
		return false
REDACTED
	return openAIJSONValueMayContainImageInput(messages)
REDACTED

func isGrokComposerModel(model string) bool {
	model = strings.TrimSpace(strings.ToLower(model))
	if model == "" {
		return false
REDACTED
	if strings.Contains(model, "/") {
		parts := strings.Split(model, "/")
		model = strings.TrimSpace(parts[len(parts)-1])
REDACTED
	return strings.Contains(model, "composer")
REDACTED

func collectGrokComposerImageURLs(reqBody map[string]any) []string {
	messages, ok := reqBody["messages"].([]any)
	if !ok {
		return nil
REDACTED

	var imageURLs []string
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
	REDACTED
		parts, ok := msgMap["content"].([]any)
		if !ok {
			continue
	REDACTED
		for _, part := range parts {
			if imageURL := grokComposerImageURLFromPart(part); imageURL != "" {
				imageURLs = append(imageURLs, imageURL)
		REDACTED
	REDACTED
REDACTED
	return imageURLs
REDACTED

func grokComposerImageURLFromPart(part any) string {
	partMap, ok := part.(map[string]any)
	if !ok {
		return ""
REDACTED
	if strings.TrimSpace(strings.ToLower(fmt.Sprint(partMap["type"]))) != "image_url" {
		return ""
REDACTED
	switch imageURL := partMap["image_url"].(type) {
	case string:
		return normalizeGrokComposerImageURL(imageURL)
	case map[string]any:
		raw, _ := imageURL["url"].(string)
		return normalizeGrokComposerImageURL(raw)
	default:
		return ""
REDACTED
REDACTED

func normalizeGrokComposerImageURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || isEmptyBase64DataURI(trimmed) {
		return ""
REDACTED
	return trimmed
REDACTED

func (s *OpenAIGatewayService) describeGrokComposerImage(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token string,
	imageURL string,
	index int,
) (string, OpenAIUsage, error) {
	body, err := buildGrokComposerImageDescriptionBody(imageURL, index)
	if err != nil {
		return "", OpenAIUsage{REDACTED, err
REDACTED

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	// Image-description probes are auxiliary requests, not conversation turns.
	// Do not bind them to the caller's Grok prompt-cache identity.
	upstreamReq, err := buildGrokResponsesRequest(upstreamCtx, c, account, body, token, "", s.cfg)
	releaseUpstreamCtx()
	if err != nil {
		return "", OpenAIUsage{REDACTED, fmt.Errorf("build grok composer image bridge request: %w", err)
REDACTED

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return "", OpenAIUsage{REDACTED, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		upstreamMsg := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(respBody))
		if upstreamMsg == "" {
			upstreamMsg = fmt.Sprintf("xAI image bridge upstream returned status %d", resp.StatusCode)
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
			return "", OpenAIUsage{REDACTED, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				ResponseHeaders:        resp.Header.Clone(),
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
		REDACTED
	REDACTED
		return "", OpenAIUsage{REDACTED, fmt.Errorf("grok composer image bridge upstream error: %s", upstreamMsg)
REDACTED

	s.updateGrokUsageFromResponse(ctx, account, resp.Header, resp.StatusCode)
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, nil)
	if err != nil {
		return "", OpenAIUsage{REDACTED, fmt.Errorf("read grok composer image bridge response: %w", err)
REDACTED

	var parsed apicompat.ResponsesResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", OpenAIUsage{REDACTED, fmt.Errorf("parse grok composer image bridge response: %w", err)
REDACTED
	description := strings.TrimSpace(grokResponsesOutputText(&parsed))
	if description == "" {
		return "", copyOpenAIUsageFromResponsesUsage(parsed.Usage), fmt.Errorf("grok composer image bridge returned empty description")
REDACTED
	return description, copyOpenAIUsageFromResponsesUsage(parsed.Usage), nil
REDACTED

func buildGrokComposerImageDescriptionBody(imageURL string, index int) ([]byte, error) {
	prompt := fmt.Sprintf("Describe image %d in concise, factual text for a downstream coding/composer model. Include visible text, UI elements, diagrams, errors, and spatial relationships. Do not mention that you are an image analysis bridge.", index)
	req := map[string]any{
		"model":             grokComposerImageBridgeVisionModel,
		"stream":            false,
		"store":             false,
		"max_output_tokens": grokComposerImageBridgeMaxOutputTokens,
		"input": []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": promptREDACTED,
					map[string]any{"type": "input_image", "image_url": imageURLREDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	return marshalOpenAIUpstreamJSON(req)
REDACTED

func grokResponsesOutputText(resp *apicompat.ResponsesResponse) string {
	if resp == nil {
		return ""
REDACTED
	var parts []string
	for _, output := range resp.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" || content.Type == "text" || content.Type == "input_text" {
				if text := strings.TrimSpace(content.Text); text != "" {
					parts = append(parts, text)
			REDACTED
		REDACTED
	REDACTED
REDACTED
	return strings.Join(parts, "\n\n")
REDACTED

func rewriteGrokComposerImagesAsText(reqBody map[string]any, descriptions []string) bool {
	messages, ok := reqBody["messages"].([]any)
	if !ok {
		return false
REDACTED

	imageIndex := 0
	changed := false
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
	REDACTED
		parts, ok := msgMap["content"].([]any)
		if !ok {
			continue
	REDACTED
		var textParts []string
		messageChanged := false
		for _, part := range parts {
			if imageURL := grokComposerImageURLFromPart(part); imageURL != "" {
				if imageIndex < len(descriptions) {
					textParts = append(textParts, fmt.Sprintf("Image %d description: %s", imageIndex+1, strings.TrimSpace(descriptions[imageIndex])))
			REDACTED
				imageIndex++
				messageChanged = true
				continue
		REDACTED
			if text := grokComposerTextFromPart(part); text != "" {
				textParts = append(textParts, text)
		REDACTED
	REDACTED
		if messageChanged {
			msgMap["content"] = strings.Join(textParts, "\n\n")
			changed = true
	REDACTED
REDACTED
	return changed
REDACTED

func grokComposerTextFromPart(part any) string {
	partMap, ok := part.(map[string]any)
	if !ok {
		return ""
REDACTED
	partType := strings.TrimSpace(strings.ToLower(fmt.Sprint(partMap["type"])))
	switch partType {
	case "text", "input_text":
		text, _ := partMap["text"].(string)
		return strings.TrimSpace(text)
	default:
		return ""
REDACTED
REDACTED

func addOpenAIUsage(dst *OpenAIUsage, usage OpenAIUsage) {
	if dst == nil {
		return
REDACTED
	dst.InputTokens += usage.InputTokens
	dst.ImageInputTokens += usage.ImageInputTokens
	dst.OutputTokens += usage.OutputTokens
	dst.CacheCreationInputTokens += usage.CacheCreationInputTokens
	dst.CacheReadInputTokens += usage.CacheReadInputTokens
	dst.ImageOutputTokens += usage.ImageOutputTokens
REDACTED

func buildGrokResponsesRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token, cacheIdentity string, cfg *config.Config) (*http.Request, error) {
	targetURL, err := buildGrokResponsesURL(account, cfg)
	if err != nil {
		return nil, err
REDACTED
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if account.IsGrokOAuth() {
		applyGrokCLIHeaders(req.Header)
REDACTED
	applyGrokCacheHeaders(req.Header, cacheIdentity)
	if c != nil {
		if v := c.GetHeader("OpenAI-Beta"); strings.TrimSpace(v) != "" {
			req.Header.Set("OpenAI-Beta", v)
	REDACTED
REDACTED
	// 账号级请求头覆写最后应用，使配置值优先于上面的内置默认头；
	// 打到官方 CLI 网关时身份头仍由共享传输层最终强制。
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
REDACTED

// applyGrokCLIHeaders identifies subscription traffic as a supported Grok CLI
// version. The CLI gateway rejects otherwise valid OAuth requests without it.
func applyGrokCLIHeaders(headers http.Header) {
	if headers == nil {
		return
REDACTED
	headers.Set("User-Agent", grokUpstreamUserAgent)
	headers.Set("X-Grok-Client-Version", grokCLIVersion)
REDACTED

func (s *OpenAIGatewayService) updateGrokUsageSnapshot(ctx context.Context, account *Account, snapshot *xai.QuotaSnapshot) {
	if s == nil || account == nil || account.ID <= 0 || snapshot == nil {
		return
REDACTED
	accountID := account.ID
	now := time.Now()
	resetAt, hasActiveLimit := grokRateLimitResetAtForAccount(account, snapshot, now)
	if hasActiveLimit {
		normalizeGrokExhaustedWindowResets(snapshot, resetAt, now)
REDACTED
	recovery := isSuccessfulGrokRateLimitRecovery(account, snapshot)
	critical := snapshot.StatusCode == http.StatusTooManyRequests || hasActiveLimit || recovery
	if s.codexSnapshotThrottle != nil {
		allowed := s.codexSnapshotThrottle.Allow(accountID, now)
		if !critical && !allowed {
			return
	REDACTED
REDACTED

	stateCtx := ctx
	if hasActiveLimit {
		var cancel context.CancelFunc
		stateCtx, cancel = openAIAccountStateContext(ctx)
		defer cancel()
REDACTED
	if s.accountRepo != nil {
		_ = s.accountRepo.UpdateExtra(stateCtx, accountID, map[string]any{
			grokQuotaSnapshotExtraKey: snapshot,
	REDACTED)
REDACTED
	// Error responses are reconciled by handleGrokAccountUpstreamError, which
	// also installs the immediate in-memory scheduling block. Successful
	// responses can still consume the last available request/token, so persist
	// that exhausted window here as a real rate limit rather than relying only
	// on the passive snapshot scheduler check.
	if hasActiveLimit {
		s.rateLimitGrok(stateCtx, account, resetAt)
REDACTED else if recovery {
		clearGrokRateLimitAfterRecovery(stateCtx, s.accountRepo, account)
REDACTED
REDACTED

func (s *OpenAIGatewayService) updateGrokUsageFromResponse(ctx context.Context, account *Account, headers http.Header, statusCode int) {
	snapshot := parseGrokQuotaSnapshot(headers, statusCode, time.Now())
	if snapshot != nil {
		s.updateGrokUsageSnapshot(ctx, account, snapshot)
		return
REDACTED
	// Successful responses are recovery evidence even when the upstream omits
	// optional quota headers. Do not replace an informative stored snapshot with
	// an empty one; only clear the exact observed cooldown generation.
	recoverySnapshot := &xai.QuotaSnapshot{StatusCode: statusCodeREDACTED
	if isSuccessfulGrokRateLimitRecovery(account, recoverySnapshot) {
		clearGrokRateLimitAfterRecovery(ctx, s.accountRepo, account)
REDACTED
REDACTED

func parseGrokQuotaSnapshot(headers http.Header, statusCode int, now time.Time) *xai.QuotaSnapshot {
	snapshot := xai.ParseQuotaHeaders(headers, statusCode)
	if snapshot == nil && statusCode == http.StatusTooManyRequests {
		return &xai.QuotaSnapshot{
			StatusCode: statusCode,
			UpdatedAt:  now.UTC().Format(time.RFC3339),
	REDACTED
REDACTED
	return snapshot
REDACTED

func normalizeGrokExhaustedWindowResets(snapshot *xai.QuotaSnapshot, resetAt, now time.Time) {
	if snapshot == nil || !resetAt.After(now) {
		return
REDACTED
	for _, window := range []*xai.QuotaWindow{snapshot.Requests, snapshot.TokensREDACTED {
		if window == nil || window.Remaining == nil || *window.Remaining > 0 {
			continue
	REDACTED
		candidate := time.Time{REDACTED
		if window.ResetUnix != nil && *window.ResetUnix > 0 {
			candidate = time.Unix(*window.ResetUnix, 0)
	REDACTED else if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(window.ResetAt)); err == nil {
			candidate = parsed
	REDACTED
		if !candidate.After(now) {
			candidate = resetAt
	REDACTED
		resetUnix := candidate.Unix()
		window.ResetUnix = &resetUnix
		window.ResetAt = candidate.UTC().Format(time.RFC3339)
REDACTED
REDACTED

func grokRateLimitResetAt(snapshot *xai.QuotaSnapshot, now time.Time) (time.Time, bool) {
	if snapshot == nil {
		return time.Time{REDACTED, false
REDACTED

	// Retry-After is xAI's explicit retry boundary. Use the observation time so
	// a persisted snapshot does not start a fresh cooldown every time it is read.
	retryAfterExpired := false
	var resetAt time.Time
	if snapshot.RetryAfterSeconds != nil && *snapshot.RetryAfterSeconds > 0 {
		observedAt := now
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(snapshot.UpdatedAt)); err == nil {
			observedAt = parsed
	REDACTED
		retryAfterResetAt := observedAt.Add(time.Duration(*snapshot.RetryAfterSeconds) * time.Second)
		if retryAfterResetAt.After(now) {
			resetAt = retryAfterResetAt
	REDACTED else {
			retryAfterExpired = true
	REDACTED
REDACTED

	exhausted := false
	for _, window := range []*xai.QuotaWindow{snapshot.Requests, snapshot.TokensREDACTED {
		if window == nil || window.Remaining == nil || *window.Remaining > 0 {
			continue
	REDACTED
		exhausted = true
		candidate := time.Time{REDACTED
		if window.ResetUnix != nil && *window.ResetUnix > 0 {
			candidate = time.Unix(*window.ResetUnix, 0)
	REDACTED else if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(window.ResetAt)); err == nil {
			candidate = parsed
	REDACTED
		if candidate.After(now) && candidate.After(resetAt) {
			resetAt = candidate
	REDACTED
REDACTED
	if !resetAt.IsZero() {
		return resetAt, true
REDACTED
	// An observed Retry-After is an absolute boundary once combined with the
	// snapshot timestamp. Do not turn an expired persisted snapshot into a new
	// rolling fallback cooldown, but still allow a later explicit window reset.
	if retryAfterExpired {
		return time.Time{REDACTED, false
REDACTED
	if exhausted || snapshot.StatusCode == http.StatusTooManyRequests {
		return now.Add(grokRateLimitFallbackCooldown), true
REDACTED
	return time.Time{REDACTED, false
REDACTED

func grokRateLimitResetAtForAccount(account *Account, snapshot *xai.QuotaSnapshot, now time.Time) (time.Time, bool) {
	resetAt, limited := grokRateLimitResetAt(snapshot, now)
	if !limited || !isGrokOAuthAccount(account) || snapshot == nil || snapshot.StatusCode != http.StatusTooManyRequests {
		return resetAt, limited
REDACTED
	if account.RateLimitedAt == nil || account.RateLimitResetAt == nil {
		return resetAt, true
REDACTED
	previousResetAt := *account.RateLimitResetAt
	if previousResetAt.After(now) || now.Sub(previousResetAt) > grokRateLimitBackoffQuietPeriod {
		return resetAt, true
REDACTED
	previousCooldown := previousResetAt.Sub(*account.RateLimitedAt)
	if previousCooldown <= 0 {
		return resetAt, true
REDACTED

	adaptiveCooldown := grokRateLimitRepeatCooldown
	switch {
	case previousCooldown >= grokRateLimitSustainedCooldown:
		adaptiveCooldown = grokRateLimitMaxAdaptiveCooldown
	case previousCooldown >= grokRateLimitRepeatCooldown:
		adaptiveCooldown = grokRateLimitSustainedCooldown
REDACTED
	adaptiveResetAt := now.Add(adaptiveCooldown)
	if adaptiveResetAt.After(resetAt) {
		resetAt = adaptiveResetAt
REDACTED
	return resetAt, true
REDACTED

func normalizeGrokRateLimitResetAt(account *Account, resetAt, now time.Time) time.Time {
	if !resetAt.After(now) {
		resetAt = now.Add(grokRateLimitFallbackCooldown)
REDACTED
	if account != nil && account.RateLimitResetAt != nil && account.RateLimitResetAt.After(resetAt) {
		resetAt = *account.RateLimitResetAt
REDACTED
	return resetAt
REDACTED

type grokRateLimitExtendingRepository interface {
	SetRateLimitedIfLater(ctx context.Context, id int64, resetAt time.Time) error
REDACTED

type grokRateLimitRecoveryRepository interface {
	ClearRateLimitIfObserved(ctx context.Context, id int64, observedLimitedAt, observedResetAt time.Time) (bool, error)
REDACTED

func isSuccessfulGrokRateLimitRecovery(account *Account, snapshot *xai.QuotaSnapshot) bool {
	return isGrokOAuthAccount(account) &&
		account.RateLimitedAt != nil &&
		account.RateLimitResetAt != nil &&
		snapshot != nil &&
		snapshot.StatusCode >= http.StatusOK &&
		snapshot.StatusCode < http.StatusMultipleChoices
REDACTED

func clearGrokRateLimitAfterRecovery(ctx context.Context, repo AccountRepository, account *Account) {
	if repo == nil || account == nil || account.RateLimitedAt == nil || account.RateLimitResetAt == nil || ctx.Err() != nil {
		return
REDACTED
	recoveryRepo, ok := repo.(grokRateLimitRecoveryRepository)
	if !ok {
		return
REDACTED
	_, err := recoveryRepo.ClearRateLimitIfObserved(ctx, account.ID, *account.RateLimitedAt, *account.RateLimitResetAt)
	if err != nil {
		slog.Warn("grok_rate_limit_recovery_clear_failed", "account_id", account.ID, "error", err)
REDACTED
REDACTED

func persistGrokRateLimit(ctx context.Context, repo AccountRepository, account *Account, resetAt time.Time) {
	if repo == nil || account == nil || account.ID <= 0 {
		return
REDACTED
	resetAt = normalizeGrokRateLimitResetAt(account, resetAt, time.Now())
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	var err error
	if extendingRepo, ok := repo.(grokRateLimitExtendingRepository); ok {
		err = extendingRepo.SetRateLimitedIfLater(stateCtx, account.ID, resetAt)
REDACTED else {
		err = repo.SetRateLimited(stateCtx, account.ID, resetAt)
REDACTED
	if err != nil {
		slog.Warn("persist_grok_rate_limit_failed", "account_id", account.ID, "reset_at", resetAt.UTC(), "error", err)
REDACTED
REDACTED

func (s *OpenAIGatewayService) rateLimitGrok(ctx context.Context, account *Account, resetAt time.Time) {
	if s == nil || account == nil {
		return
REDACTED
	resetAt = normalizeGrokRateLimitResetAt(account, resetAt, time.Now())

	runtimeUntil := resetAt
	if account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(runtimeUntil) {
		runtimeUntil = *account.TempUnschedulableUntil
REDACTED
	s.BlockAccountScheduling(account, runtimeUntil, "429")
	persistGrokRateLimit(ctx, s.accountRepo, account, resetAt)
REDACTED

func (s *OpenAIGatewayService) handleGrokAccountUpstreamError(ctx context.Context, account *Account, statusCode int, headers http.Header, responseBody []byte) {
	if s == nil || account == nil {
		return
REDACTED
	now := time.Now()
	s.updateGrokUsageSnapshot(ctx, account, parseGrokQuotaSnapshot(headers, statusCode, now))
	switch statusCode {
	case http.StatusUnauthorized:
		s.tempUnscheduleGrok(ctx, account, 10*time.Minute, "grok credentials unauthorized")
	case http.StatusForbidden:
		s.tempUnscheduleGrok(ctx, account, 30*time.Minute, "grok access or entitlement denied")
	case http.StatusTooManyRequests:
		// updateGrokUsageSnapshot installs both runtime and durable rate-limit state.
	default:
		if statusCode >= 500 {
			s.tempUnscheduleGrok(ctx, account, 2*time.Minute, "grok upstream temporary error")
	REDACTED
REDACTED
	_ = responseBody
REDACTED

func (s *OpenAIGatewayService) tempUnscheduleGrok(ctx context.Context, account *Account, cooldown time.Duration, reason string) {
	if s == nil || account == nil {
		return
REDACTED
	until := time.Now().Add(cooldown)
	if account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(until) {
		until = *account.TempUnschedulableUntil
REDACTED
	s.BlockAccountScheduling(account, until, reason)
	if s.accountRepo != nil {
		stateCtx, cancel := openAIAccountStateContext(ctx)
		defer cancel()
		_ = s.accountRepo.SetTempUnschedulable(stateCtx, account.ID, until, reason)
REDACTED
REDACTED

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
	// grokUpstreamUserAgent lives in grok_upstream_headers.go (shared with TLS header helpers).
	grokCLIVersion                   = xai.CLIClientVersion
	grokDefaultResponsesModel        = "grok-4.5"
	grokRateLimitFallbackCooldown    = 2 * time.Minute
	grokRateLimitRepeatCooldown      = 10 * time.Minute
	grokRateLimitSustainedCooldown   = 30 * time.Minute
	grokRateLimitMaxAdaptiveCooldown = time.Hour
	grokRateLimitBackoffQuietPeriod  = time.Hour
	grokFreeUsageExhaustedCode       = "subscription:free-usage-exhausted"
	grokFreeUsageExhaustedLegacyCode = "subscription-free-usage-exhausted"
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
	}

	upstreamModel := account.GetMappedModel(originalModel)
	if strings.TrimSpace(upstreamModel) == "" {
		upstreamModel = grokDefaultResponsesModel
	}
	// Account mappings are optional. Canonicalize client aliases even when the
	// account has no model_mapping, matching the Chat Completions path and xAI's
	// actual Responses model IDs.
	upstreamModel = xai.ResolveGrokTextResponsesModelID(upstreamModel, grokDefaultResponsesModel)
	if isGrokImageGenerationModel(upstreamModel) {
		return nil, fmt.Errorf("model %s is an image model and is not available on the Responses endpoint; use /v1/images/generations instead", upstreamModel)
	}
	patchedBody, clientToolMapping, err := patchGrokResponsesBodyWithClientTools(body, upstreamModel)
	if err != nil {
		setOpsUpstreamError(c, http.StatusBadRequest, err.Error(), "")
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type": "invalid_request_error", "message": err.Error(), "param": "tools",
		}})
		return nil, err
	}
	setGrokResponsesClientToolMapping(c, clientToolMapping)
	// OpenAI /responses/compact is not a native xAI endpoint. Convert it into a
	// normal Grok Responses turn that asks for a structured summary, then map the
	// reply back to an OpenAI compaction item on the way out.
	if isOpenAIResponsesCompactPath(c) {
		patchedBody, err = buildGrokCompactRequestBody(patchedBody)
		if err != nil {
			return nil, err
		}
	}
	// Derive the identity from the request xAI will actually see. This makes
	// Codex Responses Lite additional_tools part of the stable tool prefix.
	cacheIdentity := resolveGrokCacheIdentity(c, patchedBody, "", upstreamModel)
	mixedCacheIntentBody := append([]byte(nil), patchedBody...)
	patchedBody, err = applyGrokResponsesCacheIdentity(patchedBody, body, cacheIdentity, account.IsGrokOAuth())
	if err != nil {
		return nil, fmt.Errorf("apply grok prompt cache identity: %w", err)
	}
	// Free OAuth + client function tools: reuse Messages mixed-tools cache route
	// (append web_search/x_search so xAI does not force non-cacheable build-free).
	patchedBody, err = applyGrokFreeRequestToolCacheRoute(c, patchedBody, mixedCacheIntentBody, account, cacheIdentity)
	if err != nil {
		return nil, fmt.Errorf("apply grok Free function-tool cache route: %w", err)
	}

	token, _, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, err
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()

	proxyURL := account.ProxyURL()

	upstreamStart := time.Now()
	var resp *http.Response
	for attempt := 0; ; attempt++ {
		upstreamReq, buildErr := buildGrokResponsesRequest(upstreamCtx, c, account, patchedBody, token, cacheIdentity, s.cfg, s.settingService)
		if buildErr != nil {
			return nil, buildErr
		}

		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		if err != nil {
			return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
		}

		// xAI can reject encrypted reasoning copied from a response produced under
		// another account or cache identity. Retry once with the same routing and
		// credential after removing only the rejected encrypted reasoning payload.
		if attempt > 0 || resp.StatusCode != http.StatusBadRequest {
			break
		}
		respBody := s.readUpstreamErrorBody(resp)
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		if !isGrokInvalidEncryptedContentResponse(resp.StatusCode, respBody) {
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			break
		}

		retryBody, changed, trimErr := trimGrokInvalidEncryptedContentRetryBody(patchedBody)
		if trimErr != nil {
			return nil, fmt.Errorf("prepare Grok invalid encrypted_content retry: %w", trimErr)
		}
		if !changed {
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			break
		}

		patchedBody = retryBody
		slog.Info("grok_invalid_encrypted_content_retry", "account_id", account.ID, "cache_identity_present", cacheIdentity != "")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMsg := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(respBody))
		if upstreamMsg == "" {
			upstreamMsg = fmt.Sprintf("xAI upstream returned status %d", resp.StatusCode)
		}
		kind := "http_error"
		if s.shouldFailoverGrokUpstreamError(resp.StatusCode, respBody) {
			kind = "failover"
		}
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
			Kind:               kind,
			Message:            upstreamMsg,
		})
		errCtx := withGrokTeamRateLimitModel(ctx, upstreamModel)
		permanentlyDisabled := s.handleGrokAccountUpstreamError(errCtx, account, resp.StatusCode, resp.Header, respBody, upstreamModel)
		// 429 / free-usage: stamp team+model cool so sibling accounts skip this model.
		// Prefer the account reset written above so Retry-After is not stretched
		// to the 10-minute team default.
		if resp.StatusCode == http.StatusTooManyRequests ||
			classifyGrokUpstreamFailure(resp.StatusCode, respBody, upstreamModel).Class == GrokFailureFreeUsage {
			now := time.Now()
			until := now.Add(grokTeamRateLimitDefaultTTL)
			if account.RateLimitResetAt != nil {
				until = resolveGrokTeamRateLimitUntil(*account.RateLimitResetAt, now)
			}
			markGrokTeamModelRateLimit(account, upstreamModel, until)
		}
		if s.shouldFailoverGrokUpstreamError(resp.StatusCode, respBody) {
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				ResponseHeaders:        resp.Header.Clone(),
				RetryableOnSameAccount: grokRetryableOnSameAccount(account, resp.StatusCode, permanentlyDisabled),
			}
		}
		return s.handleErrorResponse(ctx, resp, c, account, patchedBody, upstreamModel)
	}

	// Attach model so rate-limit snapshots can fan out a team+model cool.
	stateCtx := withGrokTeamRateLimitModel(ctx, upstreamModel)
	s.updateGrokUsageFromResponse(stateCtx, account, resp.Header, resp.StatusCode)

	var usage *OpenAIUsage
	var firstTokenMs *int
	responseID := ""
	searchCount := 0
	imageCount := 0
	var imageOutputSizes []string
	if reqStream {
		maxLineSize := defaultMaxLineSize
		if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
			maxLineSize = s.cfg.Gateway.MaxLineSize
		}
		resp.Body = newGrokResponsesBillingPingFilterBody(resp.Body, account, maxLineSize)
		if hasGrokResponsesClientToolMapping(clientToolMapping) {
			resp.Body = newGrokResponsesClientToolStreamBody(resp.Body, clientToolMapping, maxLineSize)
		}
		streamResult, err := s.handleStreamingResponse(ctx, resp, c, account, startTime, originalModel, upstreamModel)
		if err != nil {
			return nil, err
		}
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
		responseID = strings.TrimSpace(streamResult.responseID)
		searchCount = streamResult.searchCount
		imageCount = streamResult.imageCount
		imageOutputSizes = streamResult.imageOutputSizes
	} else {
		nonStreamResult, err := s.handleNonStreamingResponse(ctx, resp, c, account, originalModel, upstreamModel)
		if err != nil {
			return nil, err
		}
		usage = nonStreamResult.usage
		responseID = strings.TrimSpace(nonStreamResult.responseID)
		searchCount = nonStreamResult.searchCount
		imageCount = nonStreamResult.imageCount
		imageOutputSizes = nonStreamResult.imageOutputSizes
	}

	if usage == nil {
		usage = &OpenAIUsage{}
	}
	reasoningEffort := extractOpenAIReasoningEffortFromBody(patchedBody, originalModel)
	result := &OpenAIForwardResult{
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
	}
	// Propagate search/image counters from the shared Responses handler — without
	// this, stream/JSON counting runs but search_price_per_1k / image bills never apply.
	if searchCount > 0 {
		result.SearchCount = searchCount
	}
	if imageCount > 0 {
		result.ImageCount = imageCount
		result.ImageOutputSizes = imageOutputSizes
	}
	return result, nil
}

func isGrokInvalidEncryptedContentResponse(statusCode int, body []byte) bool {
	if statusCode != http.StatusBadRequest {
		return false
	}

	// xAI has used both flat and nested error envelopes:
	//   {"code":"invalid-argument","error":"Could not decrypt the provided encrypted_content."}
	//   {"error":{"message":"Could not decrypt the provided encrypted_content."}}
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
		}
	default:
		message = gjson.GetBytes(body, "message").String()
	}
	normalizedMessage := strings.ToLower(strings.TrimSpace(message))
	if normalizedMessage == "" {
		return false
	}

	if strings.EqualFold(code, "invalid_encrypted_content") {
		return true
	}
	// Keep the official xAI flat-code gate so unrelated 400s are not retried.
	if !strings.EqualFold(code, "invalid-argument") && code != "" {
		return false
	}
	// Nested OpenAI-style envelopes may omit top-level code; require decrypt text.
	if code == "" && !strings.Contains(normalizedMessage, "decrypt") {
		return false
	}
	return strings.Contains(normalizedMessage, "encrypted_content") &&
		(strings.Contains(normalizedMessage, "decrypt") ||
			strings.Contains(normalizedMessage, "unmodified"))
}

// requestHasGrokEncryptedReasoning reports whether the outbound Responses body
// still carries reasoning.encrypted_content that can be stripped for retry.
func requestHasGrokEncryptedReasoning(body []byte) bool {
	input := gjson.GetBytes(body, "input")
	if !input.Exists() {
		return false
	}
	items := input.Array()
	if input.IsObject() {
		items = []gjson.Result{input}
	}
	for _, item := range items {
		if strings.TrimSpace(item.Get("type").String()) != "reasoning" {
			continue
		}
		enc := item.Get("encrypted_content")
		if enc.Exists() && enc.Type != gjson.Null && strings.TrimSpace(enc.String()) != "" {
			return true
		}
	}
	return false
}

type grokEncryptedContentStripRetriedKey struct{}

func markGrokEncryptedContentStripRetried(ctx context.Context) context.Context {
	return context.WithValue(ctx, grokEncryptedContentStripRetriedKey{}, true)
}

func grokEncryptedContentStripRetried(ctx context.Context) bool {
	v, _ := ctx.Value(grokEncryptedContentStripRetriedKey{}).(bool)
	return v
}

// stripAnthropicThinkingSignatures removes thinking.signature from Claude
// history so a different Grok OAuth account can accept multi-turn tool
// continuations after decrypt failures. Returns ok=false when nothing changed.
func stripAnthropicThinkingSignatures(body []byte) ([]byte, bool) {
	if len(body) == 0 || !bytes.Contains(body, []byte(`"signature"`)) {
		return body, false
	}
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body, false
	}
	messages, ok := req["messages"].([]any)
	if !ok || len(messages) == 0 {
		return body, false
	}
	changed := false
	for _, rawMsg := range messages {
		msg, ok := rawMsg.(map[string]any)
		if !ok {
			continue
		}
		content, ok := msg["content"].([]any)
		if !ok {
			continue
		}
		for _, rawBlock := range content {
			block, ok := rawBlock.(map[string]any)
			if !ok {
				continue
			}
			if typ, _ := block["type"].(string); typ != "thinking" {
				continue
			}
			if _, has := block["signature"]; has {
				delete(block, "signature")
				changed = true
			}
		}
	}
	if !changed {
		return body, false
	}
	out, err := json.Marshal(req)
	if err != nil {
		return body, false
	}
	return out, true
}

func trimGrokInvalidEncryptedContentRetryBody(body []byte) ([]byte, bool, error) {
	input := gjson.GetBytes(body, "input")
	items := input.Array()
	if input.IsObject() {
		items = []gjson.Result{input}
	}

	hasEncryptedReasoning := false
	for _, item := range items {
		if strings.TrimSpace(item.Get("type").String()) == "reasoning" && item.Get("encrypted_content").Exists() {
			hasEncryptedReasoning = true
			break
		}
	}
	if !hasEncryptedReasoning {
		return body, false, nil
	}

	var requestBody map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&requestBody); err != nil {
		return nil, false, err
	}
	if !trimOpenAIEncryptedReasoningItems(requestBody) {
		return body, false, nil
	}

	retryBody, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return nil, false, err
	}
	return retryBody, true, nil
}

func patchGrokResponsesBody(body []byte, upstreamModel string) ([]byte, error) {
	return patchGrokResponsesBodyBase(body, upstreamModel)
}

func patchGrokResponsesBodyWithClientTools(body []byte, upstreamModel string) ([]byte, apicompat.ResponsesClientToolMapping, error) {
	if !json.Valid(body) {
		return nil, apicompat.ResponsesClientToolMapping{}, fmt.Errorf("invalid json request body")
	}
	promoted, err := sanitizeGrokResponsesInput(body)
	if err != nil {
		return nil, apicompat.ResponsesClientToolMapping{}, err
	}
	adapted, mapping, err := adaptGrokResponsesClientTools(promoted)
	if err != nil {
		return nil, apicompat.ResponsesClientToolMapping{}, err
	}
	patched, err := patchGrokResponsesBodyBase(adapted, upstreamModel)
	if err != nil {
		return nil, apicompat.ResponsesClientToolMapping{}, err
	}
	return patched, mapping, nil
}

func patchGrokResponsesBodyBase(body []byte, upstreamModel string) ([]byte, error) {
	if !json.Valid(body) {
		return nil, fmt.Errorf("invalid json request body")
	}
	// sjson may reuse the input backing array; keep the caller's request bytes
	// unchanged because the same body can be inspected for billing/retry paths.
	out, err := sjson.SetBytes(append([]byte(nil), body...), "model", upstreamModel)
	if err != nil {
		return nil, err
	}
	out, err = normalizeGrokResponsesReasoningEffort(out, upstreamModel)
	if err != nil {
		return nil, err
	}
	out, err = sanitizeGrokResponsesModelCapabilities(out, upstreamModel)
	if err != nil {
		return nil, err
	}
	for _, unsupportedField := range []string{"prompt_cache_retention", "safety_identifier"} {
		if gjson.GetBytes(out, unsupportedField).Exists() {
			out, err = sjson.DeleteBytes(out, unsupportedField)
			if err != nil {
				return nil, err
			}
		}
	}
	if strings.EqualFold(upstreamModel, "grok-4.5") {
		for _, unsupportedField := range []string{"presence_penalty", "presencePenalty", "frequency_penalty", "frequencyPenalty", "stop"} {
			if gjson.GetBytes(out, unsupportedField).Exists() {
				out, err = sjson.DeleteBytes(out, unsupportedField)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	if grokModelRejectsLogprobs(upstreamModel) {
		for _, unsupportedField := range []string{"logprobs", "top_logprobs"} {
			if gjson.GetBytes(out, unsupportedField).Exists() {
				out, err = sjson.DeleteBytes(out, unsupportedField)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	out, err = sanitizeGrokResponsesUnsupportedFields(out)
	if err != nil {
		return nil, err
	}
	out, err = convertOpenAICompactInputsForGrok(out)
	if err != nil {
		return nil, err
	}
	out, err = sanitizeGrokResponsesInput(out)
	if err != nil {
		return nil, err
	}
	out, err = sanitizeGrokReasoningNullContent(out)
	if err != nil {
		return nil, err
	}
	out, err = sanitizeGrokResponsesTools(out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// xAI's Grok 4.20 family and newer models do not support OpenAI's logprobs
// fields. Remove them before egress instead of forwarding a request the
// upstream rejects. Older Grok models retain the fields for compatibility.
func grokModelRejectsLogprobs(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if slash := strings.LastIndex(model, "/"); slash >= 0 {
		model = strings.TrimSpace(model[slash+1:])
	}
	return strings.HasPrefix(model, "grok-4.20")
}

func sanitizeGrokResponsesModelCapabilities(body []byte, upstreamModel string) ([]byte, error) {
	if !grokModelRejectsReasoningEffort(upstreamModel) {
		return body, nil
	}

	out := body
	for _, field := range []string{"reasoning", "reasoning_effort", "reasoningEffort"} {
		if !gjson.GetBytes(out, field).Exists() {
			continue
		}
		var err error
		out, err = sjson.DeleteBytes(out, field)
		if err != nil {
			return nil, fmt.Errorf("remove unsupported Grok Composer %s: %w", field, err)
		}
	}
	return out, nil
}

func grokModelRejectsReasoningEffort(model string) bool {
	model = strings.TrimSpace(strings.ToLower(model))
	if slash := strings.LastIndex(model, "/"); slash >= 0 {
		model = strings.TrimSpace(model[slash+1:])
	}
	switch model {
	case "grok-composer", "grok-composer-2.5-fast", "composer-2.5":
		return true
	default:
		return false
	}
}

func normalizeGrokResponsesReasoningEffort(body []byte, upstreamModel string) ([]byte, error) {
	supportsEffort := grokSupportsReasoningEffort(upstreamModel)
	out := body
	var err error
	for _, field := range []string{"reasoning.effort", "reasoning_effort"} {
		value := gjson.GetBytes(out, field)
		if !value.Exists() {
			continue
		}
		normalized, keep := normalizeGrokReasoningEffortValue(value.String(), upstreamModel)
		if !supportsEffort || !keep {
			out, err = sjson.DeleteBytes(out, field)
		} else {
			out, err = sjson.SetBytes(out, field, normalized)
		}
		if err != nil {
			return nil, fmt.Errorf("normalize Grok reasoning field %s: %w", field, err)
		}
	}
	if camel := gjson.GetBytes(out, "reasoningEffort"); camel.Exists() {
		normalized, keep := normalizeGrokReasoningEffortValue(camel.String(), upstreamModel)
		out, err = sjson.DeleteBytes(out, "reasoningEffort")
		if err != nil {
			return nil, fmt.Errorf("remove Grok reasoningEffort: %w", err)
		}
		if supportsEffort && keep && !gjson.GetBytes(out, "reasoning_effort").Exists() {
			out, err = sjson.SetBytes(out, "reasoning_effort", normalized)
			if err != nil {
				return nil, fmt.Errorf("set Grok reasoning_effort: %w", err)
			}
		}
	}
	if reasoning := gjson.GetBytes(out, "reasoning"); reasoning.Exists() && reasoning.IsObject() && len(reasoning.Map()) == 0 {
		out, err = sjson.DeleteBytes(out, "reasoning")
		if err != nil {
			return nil, fmt.Errorf("remove empty Grok reasoning: %w", err)
		}
	}
	return out, nil
}

func normalizeGrokChatReasoningEffort(body []byte, upstreamModel string) ([]byte, error) {
	raw := strings.TrimSpace(gjson.GetBytes(body, "reasoning_effort").String())
	if raw == "" {
		raw = strings.TrimSpace(gjson.GetBytes(body, "reasoningEffort").String())
	}
	normalized, keep := normalizeGrokReasoningEffortValue(raw, upstreamModel)
	keep = keep && grokSupportsReasoningEffort(upstreamModel)
	out := body
	var err error
	if gjson.GetBytes(out, "reasoningEffort").Exists() {
		out, err = sjson.DeleteBytes(out, "reasoningEffort")
		if err != nil {
			return nil, err
		}
	}
	if !keep {
		if gjson.GetBytes(out, "reasoning_effort").Exists() {
			out, err = sjson.DeleteBytes(out, "reasoning_effort")
		}
		return out, err
	}
	out, err = sjson.SetBytes(out, "reasoning_effort", normalized)
	return out, err
}

func normalizeGrokReasoningEffortValue(raw, upstreamModel string) (string, bool) {
	value := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(raw)))
	switch value {
	case "none", "low", "medium", "high":
		return value, true
	case "minimal":
		return "low", true
	case "xhigh", "extrahigh", "max", "ultra":
		// Codex / Anthropic "max"/"ultra" are the top client tier. On models
		// that document xhigh, keep that official level; otherwise collapse
		// to high so grok-4.5 and earlier do not see an unknown value.
		if xai.SupportsXHighReasoningEffort(upstreamModel) {
			return "xhigh", true
		}
		return "high", true
	default:
		return "", false
	}
}

func grokSupportsReasoningEffort(model string) bool {
	model = strings.ToLower(xai.StripGrokProviderPrefix(strings.TrimSpace(model)))
	switch model {
	case xai.DefaultTextModel, "grok-4.5-latest", "grok-4.6", "grok-4.6-latest",
		"grok-4.3", "grok-4.3-latest",
		"grok-3-mini", "grok-3-mini-fast", "grok-4.20-0309-reasoning",
		"grok-4.20-reasoning", "grok-4.20-multi-agent-0309",
		"grok-4.20-multi-agent", "grok-4.20-multi-agent-latest":
		return true
	default:
		return false
	}
}

var grokResponsesUnsupportedRecursiveFields = map[string]struct{}{
	"external_web_access": {},
}

func sanitizeGrokResponsesUnsupportedFields(body []byte) ([]byte, error) {
	if !bytes.Contains(body, []byte(`"external_web_access"`)) {
		return body, nil
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if !deleteJSONFields(payload, grokResponsesUnsupportedRecursiveFields) {
		return body, nil
	}
	return json.Marshal(payload)
}

func deleteJSONFields(value any, fields map[string]struct{}) bool {
	switch typed := value.(type) {
	case map[string]any:
		changed := false
		for field := range fields {
			if _, ok := typed[field]; ok {
				delete(typed, field)
				changed = true
			}
		}
		for _, child := range typed {
			if deleteJSONFields(child, fields) {
				changed = true
			}
		}
		return changed
	case []any:
		changed := false
		for _, child := range typed {
			if deleteJSONFields(child, fields) {
				changed = true
			}
		}
		return changed
	default:
		return false
	}
}

// additional_tools is a Codex/Responses Lite private input carrier. xAI's
// Responses schema rejects the carrier itself, but accepts supported tools at
// the top level. Preserve top-level order, append newly discovered tools in
// carrier order, then let sanitizeGrokResponsesTools filter unsupported types.
func sanitizeGrokResponsesInput(body []byte) ([]byte, error) {
	if !bytes.Contains(body, []byte(`"additional_tools"`)) {
		return body, nil
	}
	input := gjson.GetBytes(body, "input")
	if !input.Exists() || !input.IsArray() {
		return body, nil
	}

	rawItems := input.Array()
	filtered := make([]json.RawMessage, 0, len(rawItems))
	topLevelTools := gjson.GetBytes(body, "tools")
	mergedTools := make([]json.RawMessage, 0)
	seenTools := make(map[string]struct{})
	appendTool := func(tool gjson.Result) bool {
		key := grokResponsesToolDedupKey(tool)
		if _, exists := seenTools[key]; exists {
			return false
		}
		seenTools[key] = struct{}{}
		mergedTools = append(mergedTools, json.RawMessage(tool.Raw))
		return true
	}
	if topLevelTools.IsArray() {
		for _, tool := range topLevelTools.Array() {
			seenTools[grokResponsesToolDedupKey(tool)] = struct{}{}
			mergedTools = append(mergedTools, json.RawMessage(tool.Raw))
		}
	}

	promoted := false
	for _, item := range rawItems {
		if strings.TrimSpace(item.Get("type").String()) == "additional_tools" {
			tools := item.Get("tools")
			if tools.IsArray() {
				for _, tool := range tools.Array() {
					if appendTool(tool) {
						promoted = true
					}
				}
			}
			continue
		}
		filtered = append(filtered, json.RawMessage(item.Raw))
	}
	if len(filtered) == len(rawItems) {
		return body, nil
	}
	encoded, err := json.Marshal(filtered)
	if err != nil {
		return nil, err
	}
	body, err = sjson.SetRawBytes(body, "input", encoded)
	if err != nil || !promoted {
		return body, err
	}
	encodedTools, err := json.Marshal(mergedTools)
	if err != nil {
		return nil, err
	}
	return sjson.SetRawBytes(body, "tools", encodedTools)
}

func grokResponsesToolDedupKey(tool gjson.Result) string {
	toolType := strings.TrimSpace(tool.Get("type").String())
	if toolType != "" {
		if name := strings.TrimSpace(tool.Get("name").String()); name != "" {
			return "type:" + toolType + "\x00name:" + name
		}
		if toolType == "mcp" {
			if label := strings.TrimSpace(tool.Get("server_label").String()); label != "" {
				return "type:mcp\x00server_label:" + label
			}
		}
	}
	return "json:" + normalizeCompatSeedJSON(json.RawMessage(tool.Raw))
}

// sanitizeGrokReasoningNullContent 删除 reasoning 项中的 "content": null。
// xAI 的 untagged enum 反序列化器拒收该字段，返回 422。
func sanitizeGrokReasoningNullContent(body []byte) ([]byte, error) {
	input := gjson.GetBytes(body, "input")
	if !input.Exists() || !input.IsArray() {
		return body, nil
	}

	items := input.Array()
	changed := false
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		if strings.TrimSpace(item.Get("type").String()) != "reasoning" {
			continue
		}
		contentResult := item.Get("content")
		if contentResult.Exists() && contentResult.Type == gjson.Null {
			var err error
			body, err = sjson.DeleteBytes(body, fmt.Sprintf("input.%d.content", i))
			if err != nil {
				return nil, err
			}
			changed = true
		}
	}
	_ = changed
	return body, nil
}

var grokResponsesSupportedToolTypes = map[string]struct{}{
	"code_execution":     {},
	"code_interpreter":   {},
	"collections_search": {},
	"file_search":        {},
	"function":           {},
	"mcp":                {},
	"shell":              {},
	"web_search":         {},
	"x_search":           {},
}

func sanitizeGrokResponsesTools(body []byte) ([]byte, error) {
	tools := gjson.GetBytes(body, "tools")
	if !tools.Exists() {
		if gjson.GetBytes(body, "tool_choice").Exists() {
			return sjson.DeleteBytes(body, "tool_choice")
		}
		return body, nil
	}
	if !tools.IsArray() {
		return body, nil
	}

	rawTools := tools.Array()
	filteredTools := make([]json.RawMessage, 0, len(rawTools))
	toolsChanged := false
	for _, tool := range rawTools {
		toolType := strings.TrimSpace(tool.Get("type").String())
		if _, ok := grokResponsesSupportedToolTypes[toolType]; ok {
			raw := json.RawMessage(tool.Raw)
			if toolType == "function" && (!tool.Get("parameters").Exists() || tool.Get("parameters").Type == gjson.Null) {
				var payload map[string]any
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
				payload["parameters"] = map[string]any{"type": "object", "properties": map[string]any{}}
				encoded, err := json.Marshal(payload)
				if err != nil {
					return nil, err
				}
				raw = encoded
				toolsChanged = true
			}
			filteredTools = append(filteredTools, raw)
		}
	}

	var err error
	if len(filteredTools) != len(rawTools) || toolsChanged {
		if len(filteredTools) == 0 {
			body, err = sjson.DeleteBytes(body, "tools")
		} else {
			var encoded []byte
			encoded, err = json.Marshal(filteredTools)
			if err != nil {
				return nil, err
			}
			body, err = sjson.SetRawBytes(body, "tools", encoded)
		}
		if err != nil {
			return nil, err
		}
	}

	toolChoice := gjson.GetBytes(body, "tool_choice")
	if !toolChoice.Exists() {
		return body, nil
	}
	if shouldDropGrokToolChoice(toolChoice, filteredTools) {
		body, err = sjson.DeleteBytes(body, "tool_choice")
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

func shouldDropGrokToolChoice(toolChoice gjson.Result, tools []json.RawMessage) bool {
	if len(tools) == 0 {
		return true
	}
	if !toolChoice.IsObject() {
		return false
	}
	choiceType := strings.TrimSpace(toolChoice.Get("type").String())
	if choiceType == "" {
		return false
	}
	if _, ok := grokResponsesSupportedToolTypes[choiceType]; !ok {
		return true
	}
	if choiceType == "function" {
		choiceName := strings.TrimSpace(toolChoice.Get("name").String())
		if choiceName == "" {
			choiceName = strings.TrimSpace(toolChoice.Get("function.name").String())
		}
		if choiceName == "" {
			return false
		}
		for _, tool := range tools {
			var item struct {
				Type     string `json:"type"`
				Name     string `json:"name"`
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			}
			if err := json.Unmarshal(tool, &item); err != nil {
				continue
			}
			name := strings.TrimSpace(item.Name)
			if name == "" {
				name = strings.TrimSpace(item.Function.Name)
			}
			if strings.TrimSpace(item.Type) == "function" && name == choiceName {
				return false
			}
		}
		return true
	}
	return false
}

func (s *OpenAIGatewayService) bridgeGrokComposerImageInputs(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
) ([]byte, OpenAIUsage, bool, error) {
	if !shouldBridgeGrokComposerImageInputs(body) {
		return body, OpenAIUsage{}, false, nil
	}

	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return body, OpenAIUsage{}, false, fmt.Errorf("parse grok composer image bridge request: %w", err)
	}

	imageURLs := collectGrokComposerImageURLs(reqBody)
	if len(imageURLs) == 0 {
		return body, OpenAIUsage{}, false, nil
	}

	descriptions := make([]string, 0, len(imageURLs))
	var bridgeUsage OpenAIUsage
	for index, imageURL := range imageURLs {
		description, usage, err := s.describeGrokComposerImage(ctx, c, account, token, imageURL, index+1)
		if err != nil {
			return body, bridgeUsage, false, err
		}
		descriptions = append(descriptions, description)
		addOpenAIUsage(&bridgeUsage, usage)
	}

	if !rewriteGrokComposerImagesAsText(reqBody, descriptions) {
		return body, bridgeUsage, false, nil
	}
	bridgedBody, err := marshalOpenAIUpstreamJSON(reqBody)
	if err != nil {
		return body, bridgeUsage, false, fmt.Errorf("serialize grok composer image bridge request: %w", err)
	}
	return bridgedBody, bridgeUsage, true, nil
}

func shouldBridgeGrokComposerImageInputs(body []byte) bool {
	if len(body) == 0 || !isGrokComposerModel(gjson.GetBytes(body, "model").String()) {
		return false
	}
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() {
		return false
	}
	return openAIJSONValueMayContainImageInput(messages)
}

func isGrokComposerModel(model string) bool {
	model = strings.TrimSpace(strings.ToLower(model))
	if model == "" {
		return false
	}
	if strings.Contains(model, "/") {
		parts := strings.Split(model, "/")
		model = strings.TrimSpace(parts[len(parts)-1])
	}
	return strings.Contains(model, "composer")
}

func collectGrokComposerImageURLs(reqBody map[string]any) []string {
	messages, ok := reqBody["messages"].([]any)
	if !ok {
		return nil
	}

	var imageURLs []string
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := msgMap["content"].([]any)
		if !ok {
			continue
		}
		for _, part := range parts {
			if imageURL := grokComposerImageURLFromPart(part); imageURL != "" {
				imageURLs = append(imageURLs, imageURL)
			}
		}
	}
	return imageURLs
}

func grokComposerImageURLFromPart(part any) string {
	partMap, ok := part.(map[string]any)
	if !ok {
		return ""
	}
	if strings.TrimSpace(strings.ToLower(fmt.Sprint(partMap["type"]))) != "image_url" {
		return ""
	}
	switch imageURL := partMap["image_url"].(type) {
	case string:
		return normalizeGrokComposerImageURL(imageURL)
	case map[string]any:
		raw, _ := imageURL["url"].(string)
		return normalizeGrokComposerImageURL(raw)
	default:
		return ""
	}
}

func normalizeGrokComposerImageURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || isEmptyBase64DataURI(trimmed) {
		return ""
	}
	return trimmed
}

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
		return "", OpenAIUsage{}, err
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	// Image-description probes are auxiliary requests, not conversation turns.
	// Do not bind them to the caller's Grok prompt-cache identity.
	upstreamReq, err := buildGrokResponsesRequest(upstreamCtx, c, account, body, token, "", s.cfg, s.settingService)
	releaseUpstreamCtx()
	if err != nil {
		return "", OpenAIUsage{}, fmt.Errorf("build grok composer image bridge request: %w", err)
	}

	proxyURL := account.ProxyURL()

	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return "", OpenAIUsage{}, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		upstreamMsg := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(respBody))
		if upstreamMsg == "" {
			upstreamMsg = fmt.Sprintf("xAI image bridge upstream returned status %d", resp.StatusCode)
		}
		kind := "http_error"
		if s.shouldFailoverGrokUpstreamError(resp.StatusCode, respBody) {
			kind = "failover"
		}
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
			Kind:               kind,
			Message:            upstreamMsg,
		})
		permanentlyDisabled := s.handleGrokAccountUpstreamError(
			withGrokTeamRateLimitModel(ctx, grokComposerImageBridgeVisionModel),
			account,
			resp.StatusCode,
			resp.Header,
			respBody,
			grokComposerImageBridgeVisionModel,
		)
		if s.shouldFailoverGrokUpstreamError(resp.StatusCode, respBody) {
			return "", OpenAIUsage{}, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				ResponseHeaders:        resp.Header.Clone(),
				RetryableOnSameAccount: grokRetryableOnSameAccount(account, resp.StatusCode, permanentlyDisabled),
			}
		}
		return "", OpenAIUsage{}, fmt.Errorf("grok composer image bridge upstream error: %s", upstreamMsg)
	}

	s.updateGrokUsageFromResponse(withGrokTeamRateLimitModel(ctx, grokComposerImageBridgeVisionModel), account, resp.Header, resp.StatusCode)
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, nil)
	if err != nil {
		return "", OpenAIUsage{}, fmt.Errorf("read grok composer image bridge response: %w", err)
	}

	var parsed apicompat.ResponsesResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", OpenAIUsage{}, fmt.Errorf("parse grok composer image bridge response: %w", err)
	}
	description := strings.TrimSpace(grokResponsesOutputText(&parsed))
	if description == "" {
		return "", copyOpenAIUsageFromResponsesUsage(parsed.Usage), fmt.Errorf("grok composer image bridge returned empty description")
	}
	return description, copyOpenAIUsageFromResponsesUsage(parsed.Usage), nil
}

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
					map[string]any{"type": "input_text", "text": prompt},
					map[string]any{"type": "input_image", "image_url": imageURL},
				},
			},
		},
	}
	return marshalOpenAIUpstreamJSON(req)
}

func grokResponsesOutputText(resp *apicompat.ResponsesResponse) string {
	if resp == nil {
		return ""
	}
	var parts []string
	for _, output := range resp.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" || content.Type == "text" || content.Type == "input_text" {
				if text := strings.TrimSpace(content.Text); text != "" {
					parts = append(parts, text)
				}
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func rewriteGrokComposerImagesAsText(reqBody map[string]any, descriptions []string) bool {
	messages, ok := reqBody["messages"].([]any)
	if !ok {
		return false
	}

	imageIndex := 0
	changed := false
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := msgMap["content"].([]any)
		if !ok {
			continue
		}
		var textParts []string
		messageChanged := false
		for _, part := range parts {
			if imageURL := grokComposerImageURLFromPart(part); imageURL != "" {
				if imageIndex < len(descriptions) {
					textParts = append(textParts, fmt.Sprintf("Image %d description: %s", imageIndex+1, strings.TrimSpace(descriptions[imageIndex])))
				}
				imageIndex++
				messageChanged = true
				continue
			}
			if text := grokComposerTextFromPart(part); text != "" {
				textParts = append(textParts, text)
			}
		}
		if messageChanged {
			msgMap["content"] = strings.Join(textParts, "\n\n")
			changed = true
		}
	}
	return changed
}

func grokComposerTextFromPart(part any) string {
	partMap, ok := part.(map[string]any)
	if !ok {
		return ""
	}
	partType := strings.TrimSpace(strings.ToLower(fmt.Sprint(partMap["type"])))
	switch partType {
	case "text", "input_text":
		text, _ := partMap["text"].(string)
		return strings.TrimSpace(text)
	default:
		return ""
	}
}

func addOpenAIUsage(dst *OpenAIUsage, usage OpenAIUsage) {
	if dst == nil {
		return
	}
	dst.InputTokens += usage.InputTokens
	dst.ImageInputTokens += usage.ImageInputTokens
	dst.OutputTokens += usage.OutputTokens
	dst.CacheCreationInputTokens += usage.CacheCreationInputTokens
	dst.CacheReadInputTokens += usage.CacheReadInputTokens
	dst.ImageOutputTokens += usage.ImageOutputTokens
}

func buildGrokResponsesRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token, cacheIdentity string, cfg *config.Config, settings ...*SettingService) (*http.Request, error) {
	targetURL, err := buildGrokResponsesURL(account, cfg, settings...)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if account.IsGrokOAuth() {
		applyGrokCLIHeaders(req.Header)
	}
	applyGrokCacheHeaders(req.Header, cacheIdentity)
	if c != nil {
		if v := c.GetHeader("OpenAI-Beta"); strings.TrimSpace(v) != "" {
			req.Header.Set("OpenAI-Beta", v)
		}
	}
	// 账号级请求头覆写最后应用，使配置值优先于上面的内置默认头；
	// 打到官方 CLI 网关时身份头仍由共享传输层最终强制。
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
}

// applyGrokCLIHeaders identifies subscription traffic as a supported Grok CLI
// version. The CLI gateway rejects otherwise valid OAuth requests without it.
// Identity pins come from package xai so service-layer headers match the final
// transport rewrite on cli-chat-proxy.grok.com.
func applyGrokCLIHeaders(headers http.Header) {
	if headers == nil {
		return
	}
	version := xai.ResolveCLIVersion()
	headers.Set("User-Agent", xai.CLIUserAgent(version))
	headers.Set("X-Grok-Client-Version", version)
	headers.Set("x-grok-client-version", version)
	headers.Set("x-grok-client-identifier", xai.CLIClientIdentifier)
	// Historical mode value expected by some unit tests / older CLI probes.
	headers.Set("X-Grok-Client-Mode", "interactive")
}

func (s *OpenAIGatewayService) updateGrokUsageSnapshot(ctx context.Context, account *Account, snapshot *xai.QuotaSnapshot) {
	s.updateGrokUsageSnapshotForResponse(ctx, account, snapshot, nil)
}

func (s *OpenAIGatewayService) updateGrokUsageSnapshotForResponse(
	ctx context.Context,
	account *Account,
	snapshot *xai.QuotaSnapshot,
	responseBody []byte,
) {
	if s == nil || account == nil || account.ID <= 0 || snapshot == nil {
		return
	}
	accountID := account.ID
	now := time.Now()
	resetAt, hasActiveLimit := grokRateLimitResetAtForAccount(account, snapshot, now)
	if hasActiveLimit {
		normalizeGrokExhaustedWindowResets(snapshot, resetAt, now)
	}
	extraUpdates, freeRecoveryPending := buildGrokQuotaSnapshotUpdatesForResponse(account, snapshot, now, responseBody)
	recovery := isSuccessfulGrokRateLimitRecovery(account, snapshot)
	critical := snapshot.StatusCode == http.StatusTooManyRequests || hasActiveLimit || freeRecoveryPending || recovery
	if s.codexSnapshotThrottle != nil {
		allowed := s.codexSnapshotThrottle.Allow(accountID, now)
		if !critical && !allowed {
			return
		}
	}

	updates := map[string]any{
		grokQuotaSnapshotExtraKey: snapshot,
	}
	// Also derive the scheduling-threshold extras (grok_sched_*) the evaluator
	// reads in grokThresholdCandidates. Without this writer the admin-configured
	// Grok auto-pause threshold could never fire (the read side was dead config).
	for k, v := range buildGrokSchedulerExtraUpdates(snapshot) {
		updates[k] = v
	}
	stateCtx := ctx
	if hasActiveLimit || freeRecoveryPending {
		var cancel context.CancelFunc
		stateCtx, cancel = openAIAccountStateContext(ctx)
		defer cancel()
	}
	// Account pointers on the request path are per-request copies (Redis/DB decode),
	// not a shared in-process cache. Mutating Extra here matches token refresh /
	// rate-limit writers; do not reuse the same *Account across goroutines.
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	account.Extra[grokQuotaSnapshotExtraKey] = snapshot
	// Pool-mode API keys keep the snapshot for observability but leave account
	// health to the upstream pool — never install runtime/durable rate-limit blocks.
	poolMode := account.IsPoolMode()
	if (hasActiveLimit || freeRecoveryPending) && !poolMode {
		resetAt = extendGrokFreeRecoveryLease(resetAt, now, freeRecoveryPending)
		// Block the in-memory scheduler before any persistence call. This closes
		// the window where another request could select the same account.
		s.blockGrokScheduling(account, resetAt)
	}
	// Merge scheduling-threshold extras into the free-recovery / snapshot map so
	// a single UpdateExtra write carries both observations.
	for k, v := range updates {
		if _, exists := extraUpdates[k]; !exists {
			extraUpdates[k] = v
		} else if k == grokQuotaSnapshotExtraKey {
			extraUpdates[k] = v
		}
	}
	if freeRecoveryPending && !poolMode {
		if err := persistGrokFreeRecoveryPendingState(stateCtx, s.accountRepo, account, extraUpdates, resetAt); err != nil {
			slog.Warn("persist_grok_free_recovery_pending_failed", "account_id", accountID, "error", err)
		}
	} else if s.accountRepo != nil {
		if err := s.accountRepo.UpdateExtra(stateCtx, accountID, extraUpdates); err != nil {
			slog.Warn("persist_grok_usage_snapshot_failed", "account_id", accountID, "error", err)
		}
	}
	// Successful non-pool responses can still consume the last available
	// request/token, so persist that exhausted window as a real rate limit.
	if hasActiveLimit && !freeRecoveryPending && !poolMode {
		s.blockGrokScheduling(account, resetAt)
		persistGrokRateLimit(stateCtx, s.accountRepo, account, resetAt)
	} else if recovery && !poolMode {
		clearGrokRateLimitAfterRecovery(stateCtx, s.accountRepo, account)
	}
}

func buildGrokQuotaSnapshotUpdatesForResponse(
	account *Account,
	snapshot *xai.QuotaSnapshot,
	now time.Time,
	responseBody []byte,
) (map[string]any, bool) {
	updates := map[string]any{
		grokQuotaSnapshotExtraKey: snapshot,
	}
	pending := grokShouldEnterFreeRecovery(account, snapshot, responseBody)
	if pending {
		updates[GrokFreeRecoveryPendingExtraKey] = true
		// Only the recovery worker owns next_probe_at once the latch is set.
		// Rewriting it here on every free-exhausted 429 invalidates the worker's
		// claim generation (CAS on next_probe_at) so probe results never stick
		// and operators see "next probe" forever with empty last_probe_result.
		if account == nil || !account.IsGrokFreeRecoveryPending() {
			updates[GrokFreeRecoveryNextProbeAtExtraKey] = now.Add(grokFreeRecoveryProbeInterval).UTC().Format(time.RFC3339Nano)
		}
	}
	return updates, pending
}

func grokShouldEnterFreeRecovery(account *Account, snapshot *xai.QuotaSnapshot, responseBody []byte) bool {
	if account == nil || !account.IsGrokOAuth() || snapshot == nil || snapshot.StatusCode != http.StatusTooManyRequests {
		return false
	}
	if grokResponseIndicatesFreeUsageExhausted(responseBody) {
		return true
	}
	if grokQuotaSnapshotHasExhaustedGlobalWindow(snapshot) && grokSnapshotHasKnownFreeEvidence(account, snapshot) {
		return true
	}
	// Production xAI 429s often use Chinese/English quota copy without the
	// structured free-usage code or remaining=0 headers. Those used to take
	// the model-scoped 429 path and stay globally schedulable.
	if !account.IsGrokFreeOrUnknownOAuth() && !isKnownGrokFreeAccount(account) {
		return false
	}
	return grokResponseIndicatesAccountWideFreeUsage(responseBody)
}

func grokQuotaSnapshotHasExhaustedGlobalWindow(snapshot *xai.QuotaSnapshot) bool {
	if snapshot == nil {
		return false
	}
	for _, window := range []*xai.QuotaWindow{snapshot.Requests, snapshot.Tokens} {
		if window != nil && window.Remaining != nil && *window.Remaining <= 0 {
			return true
		}
	}
	return false
}

// grokQuotaSnapshotBlocksScheduling reports that a persisted Grok quota
// observation still says the account is exhausted. Used by IsSchedulable and
// the pre-select soft-gate so remaining=0 accounts cannot re-enter rotation
// after a 2-minute fallback cooldown expires.
func grokQuotaSnapshotBlocksScheduling(account *Account) bool {
	if account == nil || !account.IsGrokOAuth() {
		return false
	}
	snapshot, err := grokQuotaSnapshotFromExtra(account.Extra)
	if err != nil || snapshot == nil {
		return false
	}
	return grokQuotaSnapshotBlocksSchedulingFromSnapshot(account, snapshot)
}

func grokQuotaSnapshotBlocksSchedulingFromSnapshot(account *Account, snapshot *xai.QuotaSnapshot) bool {
	if snapshot == nil {
		return false
	}
	if grokStoredFreeUsageExhaustionStillActive(snapshot) {
		return true
	}
	if !grokQuotaSnapshotHasExhaustedGlobalWindow(snapshot) {
		return false
	}
	now := time.Now()
	if resetAt := grokSnapshotAbsoluteResetAt(snapshot, now); !resetAt.IsZero() {
		return resetAt.After(now)
	}
	// remaining=0 with no absolute reset: keep free/unknown accounts out until
	// a probe rewrites the snapshot. Paid accounts fall back to the ordinary
	// rate_limit_reset_at / temp-unsched timers.
	return account != nil && (account.IsGrokFreeOrUnknownOAuth() || isKnownGrokFreeAccount(account))
}

func grokStoredFreeUsageExhaustionStillActive(snapshot *xai.QuotaSnapshot) bool {
	if snapshot == nil {
		return false
	}
	code := strings.ToLower(strings.TrimSpace(snapshot.ProviderErrorCode))
	return code == grokFreeUsageExhaustedErrorCode || code == grokFreeUsageExhaustedLegacyCode
}

// grokSnapshotAbsoluteResetAt returns a future retry boundary recorded on the
// snapshot. It never invents a rolling fallback from "now", so a stale
// remaining=0 observation cannot permanently self-renew every time it is read.
func grokSnapshotAbsoluteResetAt(snapshot *xai.QuotaSnapshot, now time.Time) time.Time {
	if snapshot == nil {
		return time.Time{}
	}
	var resetAt time.Time
	if snapshot.RetryAfterSeconds != nil && *snapshot.RetryAfterSeconds > 0 {
		observedAt := now
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(snapshot.UpdatedAt)); err == nil {
			observedAt = parsed
		}
		candidate := observedAt.Add(time.Duration(*snapshot.RetryAfterSeconds) * time.Second)
		if candidate.After(now) {
			resetAt = candidate
		}
	}
	for _, window := range []*xai.QuotaWindow{snapshot.Requests, snapshot.Tokens} {
		if window == nil {
			continue
		}
		candidate := time.Time{}
		if window.ResetUnix != nil && *window.ResetUnix > 0 {
			candidate = time.Unix(*window.ResetUnix, 0)
		} else if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(window.ResetAt)); err == nil {
			candidate = parsed
		}
		if candidate.After(now) && candidate.After(resetAt) {
			resetAt = candidate
		}
	}
	return resetAt
}

func grokSnapshotHasKnownFreeEvidence(account *Account, snapshot *xai.QuotaSnapshot) bool {
	// Positive paid evidence on the selected account wins over inferred Free
	// headers. The exact structured Free-exhaustion code is handled before this
	// helper and may still override stale billing metadata.
	if account == nil || !account.IsGrokFreeOrUnknownOAuth() {
		return false
	}
	if snapshot != nil {
		if isGrokFreeSubscriptionTier(snapshot.SubscriptionTier) {
			return true
		}
		if snapshot.Tokens != nil && snapshot.Tokens.Limit != nil && xai.IsGrokFreeRolling24hTokenLimit(*snapshot.Tokens.Limit) {
			return true
		}
	}
	return isKnownGrokFreeAccount(account)
}

func grok429HasAuthoritativeGlobalQuotaEvidence(account *Account, snapshot *xai.QuotaSnapshot, responseBody []byte) bool {
	if grokResponseIndicatesFreeUsageExhausted(responseBody) {
		return true
	}
	if grokQuotaSnapshotHasExhaustedGlobalWindow(snapshot) {
		return true
	}
	// Any account-wide or model-scoped free-usage body must skip the short
	// persistGrokModelCooldown path so handleGrok can latch or model-block.
	return grokResponseIndicatesAccountWideFreeUsage(responseBody) ||
		classifyGrokUpstreamFailure(http.StatusTooManyRequests, responseBody, "").Class == GrokFailureFreeUsage
}

// grokResponseIndicatesAccountWideFreeUsage is true when the body describes
// rolling Free quota exhaustion that is not scoped to a single model.
func grokResponseIndicatesAccountWideFreeUsage(responseBody []byte) bool {
	decision := classifyGrokUpstreamFailure(http.StatusTooManyRequests, responseBody, "")
	if decision.Class != GrokFailureFreeUsage {
		return false
	}
	low := strings.ToLower(decision.Reason)
	return decision.Model == "" || !isGrokModelSpecificFreeUsage(low, decision.Model)
}

// The upstream error code is authoritative even when stale billing metadata
// makes a Free account look paid. Only exact structured codes trigger the
// probe-gated Free recovery path.
func grokResponseIndicatesFreeUsageExhausted(responseBody []byte) bool {
	if len(responseBody) == 0 || !gjson.ValidBytes(responseBody) {
		return false
	}
	for _, path := range []string{"code", "error.code", "error.type"} {
		code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(responseBody, path).String()))
		if code == grokFreeUsageExhaustedCode || code == grokFreeUsageExhaustedLegacyCode {
			return true
		}
	}
	return false
}

func extendGrokFreeRecoveryLease(resetAt, now time.Time, pending bool) time.Time {
	if pending {
		leaseUntil := now.Add(grokFreeRecoveryLeaseDuration)
		if leaseUntil.After(resetAt) {
			return leaseUntil
		}
	}
	return resetAt
}

func (s *OpenAIGatewayService) updateGrokUsageFromResponse(ctx context.Context, account *Account, headers http.Header, statusCode int) {
	snapshot := parseGrokQuotaSnapshot(headers, statusCode, time.Now())
	if snapshot != nil {
		stampGrokQuotaSnapshotForPlan(account, snapshot, grokRequestedModelFromCtx(ctx))
		s.updateGrokUsageSnapshot(ctx, account, snapshot)
		return
	}
	// Successful responses are recovery evidence even when the upstream omits
	// optional quota headers. Do not replace an informative stored snapshot with
	// an empty one; only clear the exact observed cooldown generation.
	recoverySnapshot := &xai.QuotaSnapshot{StatusCode: statusCode}
	if isSuccessfulGrokRateLimitRecovery(account, recoverySnapshot) {
		clearGrokRateLimitAfterRecovery(ctx, s.accountRepo, account)
	}
}

func parseGrokQuotaSnapshot(headers http.Header, statusCode int, now time.Time) *xai.QuotaSnapshot {
	snapshot := xai.ParseQuotaHeaders(headers, statusCode)
	if snapshot == nil && statusCode == http.StatusTooManyRequests {
		return &xai.QuotaSnapshot{
			StatusCode: statusCode,
			UpdatedAt:  now.UTC().Format(time.RFC3339),
		}
	}
	return snapshot
}

func normalizeGrokExhaustedWindowResets(snapshot *xai.QuotaSnapshot, resetAt, now time.Time) {
	if snapshot == nil || !resetAt.After(now) {
		return
	}
	for _, window := range []*xai.QuotaWindow{snapshot.Requests, snapshot.Tokens} {
		if window == nil || window.Remaining == nil || *window.Remaining > 0 {
			continue
		}
		candidate := time.Time{}
		if window.ResetUnix != nil && *window.ResetUnix > 0 {
			candidate = time.Unix(*window.ResetUnix, 0)
		} else if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(window.ResetAt)); err == nil {
			candidate = parsed
		}
		if !candidate.After(now) {
			candidate = resetAt
		}
		resetUnix := candidate.Unix()
		window.ResetUnix = &resetUnix
		window.ResetAt = candidate.UTC().Format(time.RFC3339)
	}
}

func grokRateLimitResetAt(snapshot *xai.QuotaSnapshot, now time.Time) (time.Time, bool) {
	if snapshot == nil {
		return time.Time{}, false
	}

	// Retry-After is xAI's explicit retry boundary. Use the observation time so
	// a persisted snapshot does not start a fresh cooldown every time it is read.
	retryAfterExpired := false
	var resetAt time.Time
	if snapshot.RetryAfterSeconds != nil && *snapshot.RetryAfterSeconds > 0 {
		observedAt := now
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(snapshot.UpdatedAt)); err == nil {
			observedAt = parsed
		}
		retryAfterResetAt := observedAt.Add(time.Duration(*snapshot.RetryAfterSeconds) * time.Second)
		if retryAfterResetAt.After(now) {
			resetAt = retryAfterResetAt
		} else {
			retryAfterExpired = true
		}
	}

	exhausted := false
	for _, window := range []*xai.QuotaWindow{snapshot.Requests, snapshot.Tokens} {
		if window == nil || window.Remaining == nil || *window.Remaining > 0 {
			continue
		}
		exhausted = true
		candidate := time.Time{}
		if window.ResetUnix != nil && *window.ResetUnix > 0 {
			candidate = time.Unix(*window.ResetUnix, 0)
		} else if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(window.ResetAt)); err == nil {
			candidate = parsed
		}
		if candidate.After(now) && candidate.After(resetAt) {
			resetAt = candidate
		}
	}
	if !resetAt.IsZero() {
		return resetAt, true
	}
	// An observed Retry-After is an absolute boundary once combined with the
	// snapshot timestamp. Do not turn an expired persisted snapshot into a new
	// rolling fallback cooldown, but still allow a later explicit window reset.
	if retryAfterExpired {
		return time.Time{}, false
	}
	if exhausted || snapshot.StatusCode == http.StatusTooManyRequests {
		cooldown := grokRateLimitFallbackCooldown
		if snapshot.ProviderErrorCode == grokFreeUsageExhaustedErrorCode {
			cooldown = grokFreeUsageExhaustedCooldown
		}
		return now.Add(cooldown), true
	}
	return time.Time{}, false
}

func grokRateLimitResetAtForAccount(account *Account, snapshot *xai.QuotaSnapshot, now time.Time) (time.Time, bool) {
	resetAt, limited := grokRateLimitResetAt(snapshot, now)
	if !limited || !isGrokOAuthAccount(account) || snapshot == nil || snapshot.StatusCode != http.StatusTooManyRequests {
		return resetAt, limited
	}
	if account.RateLimitedAt == nil || account.RateLimitResetAt == nil {
		return resetAt, true
	}
	previousResetAt := *account.RateLimitResetAt
	if previousResetAt.After(now) || now.Sub(previousResetAt) > grokRateLimitBackoffQuietPeriod {
		return resetAt, true
	}
	previousCooldown := previousResetAt.Sub(*account.RateLimitedAt)
	if previousCooldown <= 0 {
		return resetAt, true
	}

	adaptiveCooldown := grokRateLimitRepeatCooldown
	switch {
	case previousCooldown >= grokRateLimitSustainedCooldown:
		adaptiveCooldown = grokRateLimitMaxAdaptiveCooldown
	case previousCooldown >= grokRateLimitRepeatCooldown:
		adaptiveCooldown = grokRateLimitSustainedCooldown
	}
	adaptiveResetAt := now.Add(adaptiveCooldown)
	if adaptiveResetAt.After(resetAt) {
		resetAt = adaptiveResetAt
	}
	return resetAt, true
}

func normalizeGrokRateLimitResetAt(account *Account, resetAt, now time.Time) time.Time {
	if !resetAt.After(now) {
		resetAt = now.Add(grokRateLimitFallbackCooldown)
	}
	if account != nil && account.RateLimitResetAt != nil && account.RateLimitResetAt.After(resetAt) {
		resetAt = *account.RateLimitResetAt
	}
	// Cap durable/runtime RateLimitResetAt the same way grok_sched_reset_at is
	// capped. Without this, a malformed or multi-day upstream reset is written
	// once and then permanently ratcheted by SetRateLimitedIfLater (which never
	// shortens), pinning free accounts for weeks.
	if horizon := now.Add(grokRateLimitResetHorizon); resetAt.After(horizon) {
		resetAt = horizon
	}
	return resetAt
}

// grokRateLimitResetRequiresForce reports that the in-memory stored reset is
// still later than the normalized (capped) value, so SetRateLimitedIfLater
// cannot heal it and an absolute SetRateLimited is required.
func grokRateLimitResetRequiresForce(account *Account, normalizedResetAt time.Time) bool {
	if account == nil || account.RateLimitResetAt == nil {
		return false
	}
	return account.RateLimitResetAt.After(normalizedResetAt)
}

// writeGrokRateLimitReset persists a normalized Grok rate-limit reset, forcing
// an absolute write when the stored value sits beyond the horizon cap.
func writeGrokRateLimitReset(ctx context.Context, repo AccountRepository, account *Account, resetAt time.Time) error {
	if repo == nil || account == nil || account.ID <= 0 {
		return nil
	}
	now := time.Now()
	resetAt = normalizeGrokRateLimitResetAt(account, resetAt, now)
	if grokRateLimitResetRequiresForce(account, resetAt) {
		if err := repo.SetRateLimited(ctx, account.ID, resetAt); err != nil {
			return err
		}
		// Absolute clamp is authoritative — refresh the in-memory view so the
		// same request does not re-read the pre-horizon dirty value.
		account.RateLimitResetAt = cloneTimePtr(&resetAt)
		if account.RateLimitedAt == nil {
			limitedAt := now
			account.RateLimitedAt = &limitedAt
		}
		return nil
	}
	if extendingRepo, ok := repo.(grokRateLimitExtendingRepository); ok {
		return extendingRepo.SetRateLimitedIfLater(ctx, account.ID, resetAt)
	}
	return repo.SetRateLimited(ctx, account.ID, resetAt)
}

type grokRateLimitExtendingRepository interface {
	SetRateLimitedIfLater(ctx context.Context, id int64, resetAt time.Time) error
}

type grokRateLimitRecoveryRepository interface {
	ClearRateLimitIfObserved(ctx context.Context, id int64, observedLimitedAt, observedResetAt time.Time) (bool, error)
}

func isSuccessfulGrokRateLimitRecovery(account *Account, snapshot *xai.QuotaSnapshot) bool {
	if !isGrokOAuthAccount(account) ||
		account.IsGrokFreeRecoveryPending() ||
		account.RateLimitedAt == nil ||
		account.RateLimitResetAt == nil ||
		snapshot == nil ||
		snapshot.StatusCode < http.StatusOK ||
		snapshot.StatusCode >= http.StatusMultipleChoices {
		return false
	}
	if grokQuotaSnapshotHasExhaustedGlobalWindow(snapshot) || grokStoredFreeUsageExhaustionStillActive(snapshot) {
		return false
	}
	// A headerless 2xx must not clear a stored remaining=0 / free-usage snapshot.
	if snapshot.Requests == nil && snapshot.Tokens == nil && strings.TrimSpace(snapshot.ProviderErrorCode) == "" {
		if stored, err := grokQuotaSnapshotFromExtra(account.Extra); err == nil && grokQuotaSnapshotBlocksSchedulingFromSnapshot(account, stored) {
			return false
		}
	}
	return true
}

func clearGrokRateLimitAfterRecovery(ctx context.Context, repo AccountRepository, account *Account) {
	if repo == nil || account == nil || account.RateLimitedAt == nil || account.RateLimitResetAt == nil || ctx.Err() != nil {
		return
	}
	recoveryRepo, ok := repo.(grokRateLimitRecoveryRepository)
	if !ok {
		return
	}
	_, err := recoveryRepo.ClearRateLimitIfObserved(ctx, account.ID, *account.RateLimitedAt, *account.RateLimitResetAt)
	if err != nil {
		slog.Warn("grok_rate_limit_recovery_clear_failed", "account_id", account.ID, "error", err)
	}
}

func persistGrokRateLimit(ctx context.Context, repo AccountRepository, account *Account, resetAt time.Time) {
	if repo == nil || account == nil || account.ID <= 0 {
		return
	}
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	if err := writeGrokRateLimitReset(stateCtx, repo, account, resetAt); err != nil {
		slog.Warn("persist_grok_rate_limit_failed", "account_id", account.ID, "reset_at", resetAt.UTC(), "error", err)
	}
}

func (s *OpenAIGatewayService) blockGrokScheduling(account *Account, resetAt time.Time) {
	if s == nil {
		return
	}
	blockGrokAccountScheduling(s, account, resetAt)
}

func blockGrokAccountScheduling(blocker AccountRuntimeBlocker, account *Account, resetAt time.Time) {
	if blocker == nil || account == nil {
		return
	}
	now := time.Now()
	resetAt = normalizeGrokRateLimitResetAt(account, resetAt, now)

	runtimeUntil := resetAt
	if account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(runtimeUntil) {
		runtimeUntil = *account.TempUnschedulableUntil
	}
	blocker.BlockAccountScheduling(account, runtimeUntil, "429")
}

// rateLimitGrok installs both runtime and durable rate-limit state, then
// optionally fans out a short team+model cool for sibling OAuth accounts.
func (s *OpenAIGatewayService) rateLimitGrok(ctx context.Context, account *Account, resetAt time.Time) {
	if s == nil || account == nil {
		return
	}
	now := time.Now()
	resetAt = normalizeGrokRateLimitResetAt(account, resetAt, now)

	runtimeUntil := resetAt
	if account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(runtimeUntil) {
		runtimeUntil = *account.TempUnschedulableUntil
	}
	s.BlockAccountScheduling(account, runtimeUntil, "429")
	persistGrokRateLimit(ctx, s.accountRepo, account, resetAt)

	// Propagate a short team+model cool so sibling OAuth accounts on the same
	// xAI team skip the hot model without waiting for each to hit 429 alone.
	// Model is taken from the latest request context when available; empty is a
	// no-op inside markGrokTeamModelRateLimit.
	if model, _ := ctx.Value(grokTeamRateLimitModelContextKey{}).(string); model != "" {
		markGrokTeamModelRateLimit(account, model, resolveGrokTeamRateLimitUntil(resetAt, now))
	}
}

// buildGrokSchedulerExtraUpdates derives the grok_sched_* scheduling snapshot
// (utilization percent + reset time) consumed by EvaluateAccountSchedulingThreshold.
// Utilization is the most-constrained of the requests/tokens windows.
func buildGrokSchedulerExtraUpdates(snapshot *xai.QuotaSnapshot) map[string]any {
	if snapshot == nil {
		return nil
	}
	util, reset, ok := grokSnapshotUtilization(snapshot)
	if !ok {
		return nil
	}
	updates := map[string]any{
		"grok_sched_utilization":      util,
		"grok_sched_usage_updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	if reset != nil {
		// 防御：调度阈值暂停时长由 grok_sched_reset_at 决定。若上游返回脏的
		// reset 头（例如把相对毫秒 "6000" 误当相对秒解析出 ~33h 的未来时刻），
		// 不设上限会把耗尽账号长时间锁死。xAI 配额窗口不会超过一天，因此对
		// 未来时刻做 grokMaxSchedulingResetHorizon 钳制；过去/无效值直接不写。
		now := time.Now()
		if reset.After(now) {
			capped := *reset
			if horizon := now.Add(grokMaxSchedulingResetHorizon); capped.After(horizon) {
				capped = horizon
			}
			updates["grok_sched_reset_at"] = capped.UTC().Format(time.RFC3339)
		}
	}
	return updates
}

// grokSnapshotUtilization returns the highest window utilization (0-100) across
// the requests/tokens quota windows and the reset time of that window.
func grokSnapshotUtilization(snapshot *xai.QuotaSnapshot) (float64, *time.Time, bool) {
	if snapshot == nil {
		return 0, nil, false
	}
	best := -1.0
	var bestReset *time.Time
	consider := func(window *xai.QuotaWindow) {
		if window == nil || window.Limit == nil || *window.Limit <= 0 || window.Remaining == nil {
			return
		}
		remaining := *window.Remaining
		if remaining < 0 {
			remaining = 0
		}
		util := (1 - float64(remaining)/float64(*window.Limit)) * 100
		if util < 0 {
			util = 0
		}
		if util > 100 {
			util = 100
		}
		if util > best {
			best = util
			if window.ResetUnix != nil {
				t := time.Unix(*window.ResetUnix, 0).UTC()
				bestReset = &t
			} else {
				bestReset = nil
			}
		}
	}
	consider(snapshot.Requests)
	consider(snapshot.Tokens)
	if best < 0 {
		return 0, nil, false
	}
	return best, bestReset, true
}

// grokMaxSchedulingResetHorizon bounds how far into the future a Grok
// scheduling-threshold pause (grok_sched_reset_at) may be set, so a malformed
// upstream reset header can't park an over-threshold account for days. xAI quota
// windows do not exceed ~a day.
const grokMaxSchedulingResetHorizon = 25 * time.Hour

// grokRateLimitResetHorizon bounds accounts.rate_limit_reset_at for Grok the
// same way. Free rolling windows are ~24h; multi-week values are treated as
// dirty upstream/header parses and clamped on every write path.
const grokRateLimitResetHorizon = grokMaxSchedulingResetHorizon

// grokTeamRateLimitModelContextKey carries the upstream model for team cools.
type grokTeamRateLimitModelContextKey struct{}

// withGrokTeamRateLimitModel attaches the upstream model name for rate-limit
// side effects (team+model cool). Safe when model is empty.
func withGrokTeamRateLimitModel(ctx context.Context, model string) context.Context {
	model = strings.TrimSpace(model)
	if model == "" || ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, grokTeamRateLimitModelContextKey{}, model)
}

func grokRequestedModelFromCtx(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	model, _ := ctx.Value(grokTeamRateLimitModelContextKey{}).(string)
	return strings.TrimSpace(model)
}

func isGrokHeavyTransientModel(requestedModel string) bool {
	model := strings.ToLower(strings.TrimSpace(xai.ResolveGrokTextResponsesModelID(requestedModel)))
	return strings.Contains(model, "multi-agent")
}

func persistGrokTransientModelCooldown(account *Account, decision GrokUpstreamFailureDecision) bool {
	if account == nil {
		return false
	}
	model := strings.TrimSpace(decision.Model)
	if model == "" || !isGrokHeavyTransientModel(model) {
		return false
	}
	cooldown := decision.Cooldown
	if cooldown <= 0 {
		cooldown = 3 * time.Minute
	}
	markGrokModelTransientBlock(account.ID, model, time.Now().Add(cooldown))
	return true
}

func (s *OpenAIGatewayService) handleGrokAccountUpstreamError(
	ctx context.Context,
	account *Account,
	statusCode int,
	headers http.Header,
	responseBody []byte,
	canonicalModel ...string,
) (permanentlyDisabled bool) {
	if s == nil || account == nil {
		return false
	}
	// A canceled client request must not mutate persistent or scheduler state.
	// The handler will stop failover as soon as Forward returns.
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if isGrokContentPolicyRejection(statusCode, responseBody) {
		return false
	}
	now := time.Now()
	snapshot := parseGrokQuotaSnapshot(headers, statusCode, now)
	snapshot = enrichGrokQuotaSnapshotFromError(snapshot, responseBody, statusCode, now)
	model := firstNonEmpty(canonicalModel...)
	if model == "" {
		model = grokRequestedModelFromCtx(ctx)
	}
	stampGrokQuotaSnapshotForPlan(account, snapshot, model)
	if statusCode == http.StatusTooManyRequests && account.IsGrokOAuth() && strings.TrimSpace(model) != "" &&
		!grok429HasAuthoritativeGlobalQuotaEvidence(account, snapshot, responseBody) {
		s.persistGrokModelCooldown(ctx, account, model, statusCode, snapshot, responseBody)
		return false
	}

	// Body-first free-usage / empty / billing / capacity must run before the
	// status switch so non-429 free-usage bodies still cool the account.
	// Pool-mode still skips durable mutation unless an explicit temp rule matches.
	decision := classifyGrokUpstreamFailure(statusCode, responseBody, model)
	if decision.ShouldCooldown && decision.Class != GrokFailureNone && decision.Class != GrokFailureRateLimit {
		if !account.IsPoolMode() {
			// A free-tier exhaustion message describes a rolling usage window. Use
			// an upstream absolute reset (or Retry-After) when available; otherwise
			// apply only a short probe cooldown. Never start a fabricated 24h window
			// at the instant this error was received.
			if decision.Class == GrokFailureFreeUsage {
				if resetAt, limited := grokRateLimitResetAtForAccount(account, snapshot, now); limited && resetAt.After(now) {
					if decision.Model != "" && isGrokModelSpecificFreeUsage(strings.ToLower(decision.Reason), decision.Model) {
						markGrokModelQuotaBlock(account.ID, decision.Model, resetAt)
						return false
					}
					// Snapshot writer installs the single durable rate-limit (or
					// free-recovery latch+lease). rateLimitGrok would call
					// SetRateLimited a second time for the same event.
					s.updateGrokUsageSnapshotForResponse(ctx, account, snapshot, responseBody)
					if model, _ := ctx.Value(grokTeamRateLimitModelContextKey{}).(string); model != "" {
						markGrokTeamModelRateLimit(account, model, resolveGrokTeamRateLimitUntil(resetAt, now))
					}
					return false
				}
			}
			if s.applyGrokUpstreamFailureDecision(ctx, account, decision) {
				return false
			}
		}
	}

	s.updateGrokUsageSnapshotForResponse(ctx, account, snapshot, responseBody)

	// Admin-configured temp rules always apply (pool and non-pool).
	if s.applyGrokConfiguredUnschedulablePolicy(ctx, account, statusCode, responseBody) {
		return false
	}

	// Layered account-scoped policy for 402/403/404 (pool and non-pool):
	// 1) model/endpoint scope  2) hard permanent death
	// 3) soft entitlement/billing temp cooldown  4) ambiguous → no state change
	switch statusCode {
	case http.StatusPaymentRequired, http.StatusForbidden, http.StatusNotFound:
		if s.handleGrokModelOrEndpointError(ctx, account, model, statusCode, responseBody) {
			return false
		}
		if s.markGrokAccountError(ctx, account, statusCode, responseBody) {
			// Permanent path only. CAS miss must not fall through to a temp cooldown.
			return true
		}
		// Spending-limit often arrives as 403 with a billing body.
		if isGrokSpendingLimitError(responseBody) {
			s.rateLimitGrok(ctx, account, grokSpendingLimitResetAt(account, time.Now()))
			return false
		}
		if grokResponseIndicatesSoftEntitlementOrBilling(statusCode, responseBody) {
			// Pool mode must not treat bare 402 as a default cooldown (origin
			// pool bypass). Soft entitlement still applies when the body has
			// explicit billing/entitlement markers (or non-402 statuses).
			if account.IsPoolMode() && statusCode == http.StatusPaymentRequired &&
				!grokSoftEntitlementHasExplicitBodySignal(responseBody) {
				return false
			}
			s.tempUnscheduleGrok(ctx, account, grokSoftEntitlementCooldown, grokSoftEntitlementReason)
			return false
		}
		// Ambiguous 403/404: leave durable and runtime scheduling state alone.
		return false
	}

	// Pool-mode: skip default cooldowns (401 / 405 generic / 5xx). Snapshot already
	// retained above; permanent/soft/admin handled in the layered path.
	if account.IsPoolMode() {
		slog.Info("grok_pool_mode_default_cooldown_skipped", "account_id", account.ID, "status_code", statusCode)
		return false
	}

	switch statusCode {
	case http.StatusUnauthorized:
		s.tempUnscheduleGrok(ctx, account, 10*time.Minute, "grok credentials unauthorized")
	case http.StatusMethodNotAllowed:
		// Account-scoped: a custom base_url that does not implement /v1/responses
		// fails identically on every retry and sticky reconnect (upstream #4668).
		applyGrokMethodNotAllowedCooldown(ctx, s, s.accountRepo, account)
	case http.StatusTooManyRequests:
		// updateGrokUsageSnapshot installs both runtime and durable rate-limit state.
		// free-usage-exhausted is parsed from responseBody into the snapshot first.
		// Free-usage 429 was already cooled above via body classification when present.
	default:
		if statusCode >= 500 {
			s.tempUnscheduleGrok(ctx, account, 2*time.Minute, "grok upstream temporary error")
		}
	}
	return false
}

func (s *OpenAIGatewayService) handleGrokModelOrEndpointError(
	ctx context.Context,
	account *Account,
	canonicalModel string,
	statusCode int,
	responseBody []byte,
) bool {
	if s == nil || account == nil || strings.TrimSpace(canonicalModel) == "" {
		return false
	}
	if s.rateLimitService != nil && s.rateLimitService.HandleUpstreamModelNotFound(ctx, account, canonicalModel, statusCode, responseBody) {
		return true
	}
	if !grokResponseIndicatesModelOrEndpointFailure(responseBody) {
		return false
	}
	return s.persistGrokModelCooldown(ctx, account, canonicalModel, statusCode, nil, responseBody)
}

func (s *OpenAIGatewayService) persistGrokModelCooldown(
	ctx context.Context,
	account *Account,
	canonicalModel string,
	statusCode int,
	snapshot *xai.QuotaSnapshot,
	responseBody []byte,
) bool {
	if s == nil || s.accountRepo == nil || account == nil || strings.TrimSpace(canonicalModel) == "" {
		return false
	}
	now := time.Now()
	resetAt, ok := grokRateLimitResetAt(snapshot, now)
	if !ok {
		resetAt = now.Add(grokRateLimitFallbackCooldown)
	}
	reason := fmt.Sprintf("grok upstream status %d scoped to model", statusCode)
	if message := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(responseBody))); message != "" {
		reason += ": " + message
	}
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	if err := s.accountRepo.SetModelRateLimit(stateCtx, account.ID, strings.TrimSpace(canonicalModel), resetAt, reason); err != nil {
		slog.Warn("persist_grok_model_rate_limit_failed", "account_id", account.ID, "model", canonicalModel, "error", err)
		return false
	}
	slog.Info("grok_model_rate_limited", "account_id", account.ID, "model", canonicalModel, "reset_at", resetAt)
	return true
}

func (s *OpenAIGatewayService) markGrokAccountError(ctx context.Context, account *Account, statusCode int, responseBody []byte) bool {
	if s == nil {
		return false
	}
	return markGrokAccountErrorState(ctx, s, s.accountRepo, account, statusCode, responseBody)
}

const (
	grokSoftEntitlementCooldown = 30 * time.Minute
	grokSoftEntitlementReason   = "grok access or entitlement denied"

	grokMethodNotAllowedCooldown = 30 * time.Minute
	grokMethodNotAllowedReason   = "grok upstream endpoint method not allowed"
)

// applyGrokAccountScopedHTTPFailure applies permanent disable for hard account
// death, or a temporary soft-entitlement cooldown. Callers that know model or
// admin-rule context should handle those layers before invoking this helper.
func applyGrokAccountScopedHTTPFailure(
	ctx context.Context,
	blocker AccountRuntimeBlocker,
	accountRepo AccountRepository,
	account *Account,
	statusCode int,
	responseBody []byte,
) (permanentlyDisabled bool) {
	if account == nil {
		return false
	}
	if isGrokContentPolicyRejection(statusCode, responseBody) {
		return false
	}
	if markGrokAccountErrorState(ctx, blocker, accountRepo, account, statusCode, responseBody) {
		return true
	}
	if grokResponseIndicatesSoftEntitlementOrBilling(statusCode, responseBody) {
		applyGrokSoftEntitlementCooldown(ctx, blocker, accountRepo, account)
	}
	return false
}

func applyGrokSoftEntitlementCooldown(
	ctx context.Context,
	blocker AccountRuntimeBlocker,
	accountRepo AccountRepository,
	account *Account,
) {
	applyGrokAccountTempCooldown(ctx, blocker, accountRepo, account, grokSoftEntitlementCooldown, grokSoftEntitlementReason)
}

// applyGrokMethodNotAllowedCooldown benches an account whose upstream rejects
// the Responses endpoint with 405. Shared by the gateway error handler and the
// manual account-test reconciliation so both paths converge on the same state.
func applyGrokMethodNotAllowedCooldown(
	ctx context.Context,
	blocker AccountRuntimeBlocker,
	accountRepo AccountRepository,
	account *Account,
) {
	applyGrokAccountTempCooldown(ctx, blocker, accountRepo, account, grokMethodNotAllowedCooldown, grokMethodNotAllowedReason)
}

func applyGrokAccountTempCooldown(
	ctx context.Context,
	blocker AccountRuntimeBlocker,
	accountRepo AccountRepository,
	account *Account,
	cooldown time.Duration,
	reason string,
) {
	if account == nil {
		return
	}
	until := time.Now().Add(cooldown)
	if account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(until) {
		until = *account.TempUnschedulableUntil
	}
	if blocker != nil {
		blocker.BlockAccountScheduling(account, until, reason)
	}
	if accountRepo == nil {
		return
	}
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	_ = accountRepo.SetTempUnschedulable(stateCtx, account.ID, until, reason)
}

func markGrokAccountErrorState(
	ctx context.Context,
	blocker AccountRuntimeBlocker,
	accountRepo AccountRepository,
	account *Account,
	statusCode int,
	responseBody []byte,
) bool {
	if account == nil || !grokResponseIndicatesPermanentAccountFailure(statusCode, responseBody) {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	message := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(responseBody)))
	reason := fmt.Sprintf("grok upstream returned status %d", statusCode)
	if message != "" {
		reason += ": " + message
	}

	if accountRepo == nil {
		slog.Warn("persist_grok_account_error_skipped", "account_id", account.ID, "status_code", statusCode, "reason", "account repository unavailable")
		return false
	}
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	if account.IsGrokOAuth() {
		conditionalRepo, ok := accountRepo.(grokCredentialConditionalStateRepository)
		if !ok {
			slog.Warn("persist_grok_account_error_skipped", "account_id", account.ID, "status_code", statusCode, "reason", "conditional repository unavailable")
			return false
		}
		updated, err := conditionalRepo.SetGrokCredentialErrorIfMatch(
			stateCtx,
			account.ID,
			grokCredentialMutationSnapshot(account),
			reason,
		)
		if err != nil {
			slog.Warn("persist_grok_account_error_failed", "account_id", account.ID, "status_code", statusCode, "error", err)
			return false
		}
		if !updated {
			// Concurrent reauth/proxy repair won; do not layer a temp cooldown.
			slog.Info("persist_grok_account_error_cas_miss", "account_id", account.ID, "status_code", statusCode)
			return false
		}
	} else if err := accountRepo.SetError(stateCtx, account.ID, reason); err != nil {
		slog.Warn("persist_grok_account_error_failed", "account_id", account.ID, "status_code", statusCode, "error", err)
		return false
	}

	// Publish the bridge block only after the durable conditional mutation wins.
	// A concurrent reauthorization or proxy repair must leave the fresh account
	// schedulable on this node as well as in the database.
	if blocker != nil {
		blocker.BlockAccountScheduling(account, time.Time{}, fmt.Sprintf("grok_upstream_%d", statusCode))
	}
	return true
}

// grokResponseIndicatesPermanentAccountFailure matches high-confidence hard
// account death only (suspended/banned/deleted). Soft billing/entitlement
// signals are handled separately via temporary cooldown.
func grokResponseIndicatesPermanentAccountFailure(statusCode int, responseBody []byte) bool {
	if statusCode != http.StatusForbidden && statusCode != http.StatusNotFound {
		return false
	}
	if grokResponseIndicatesModelOrEndpointFailure(responseBody) {
		return false
	}

	codes := grokStructuredErrorValues(responseBody, "code", "error.code", "error.type", "response.error.code", "response.error.type")
	combined := strings.ToLower(strings.TrimSpace(string(responseBody)))
	switch statusCode {
	case http.StatusForbidden:
		for _, code := range codes {
			switch normalizeGrokErrorMarker(code) {
			case "account_suspended", "account_disabled", "account_deactivated", "account_banned":
				return true
			}
		}
		return containsAnyGrokErrorMarker(combined,
			"account suspended", "account disabled", "account deactivated", "account banned")
	case http.StatusNotFound:
		for _, code := range codes {
			switch normalizeGrokErrorMarker(code) {
			case "account_not_found", "user_not_found", "workspace_not_found", "organization_not_found":
				return true
			}
		}
		if strings.Contains(combined, "endpoint") || strings.Contains(combined, "model") {
			return false
		}
		return containsAnyGrokErrorMarker(combined,
			"account not found", "user not found", "workspace not found", "organization not found",
			"account was deleted", "workspace was deleted")
	default:
		return false
	}
}

// grokResponseIndicatesSoftEntitlementOrBilling matches recoverable billing or
// entitlement rejections. These should temporarily leave the pool, not be
// permanently disabled (subscription churn / payment lag / plan glitches).
func grokSoftEntitlementHasExplicitBodySignal(responseBody []byte) bool {
	codes := grokStructuredErrorValues(responseBody, "code", "error.code", "error.type", "response.error.code", "response.error.type")
	for _, code := range codes {
		switch normalizeGrokErrorMarker(code) {
		case "subscription_required", "billing_required", "entitlement_denied", "entitlement_required",
			"no_active_subscription", "not_entitled", "plan_required":
			return true
		}
	}
	combined := strings.ToLower(strings.TrimSpace(string(responseBody)))
	return containsAnyGrokErrorMarker(combined,
		"subscription required", "no active grok subscription", "billing account disabled",
		"billing required", "entitlement required", "entitlement denied", "not entitled")
}

func grokResponseIndicatesSoftEntitlementOrBilling(statusCode int, responseBody []byte) bool {
	if statusCode != http.StatusPaymentRequired && statusCode != http.StatusForbidden {
		return false
	}
	if grokResponseIndicatesModelOrEndpointFailure(responseBody) {
		return false
	}
	// Hard death signals take precedence when both classifiers could match.
	if grokResponseIndicatesPermanentAccountFailure(statusCode, responseBody) {
		return false
	}
	if statusCode == http.StatusPaymentRequired {
		// Bare 402 is a billing signal unless the payload proved model/endpoint.
		return true
	}
	return grokSoftEntitlementHasExplicitBodySignal(responseBody)
}

func grokResponseIndicatesModelOrEndpointFailure(responseBody []byte) bool {
	if len(responseBody) == 0 {
		return false
	}
	combined := strings.ToLower(strings.TrimSpace(string(responseBody)))
	if strings.Contains(combined, "endpoint") && containsAnyGrokErrorMarker(combined, "not found", "unsupported", "unknown") {
		return true
	}
	if !strings.Contains(combined, "model") {
		return false
	}
	return containsAnyGrokErrorMarker(combined,
		"not found", "unknown", "unsupported", "not supported", "permission", "not allowed", "requires")
}

func grokStructuredErrorValues(responseBody []byte, paths ...string) []string {
	if len(responseBody) == 0 || !gjson.ValidBytes(responseBody) {
		return nil
	}
	values := make([]string, 0, len(paths))
	for _, path := range paths {
		if value := strings.TrimSpace(gjson.GetBytes(responseBody, path).String()); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func normalizeGrokErrorMarker(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("-", "_", ":", "_", " ", "_").Replace(value)
	return strings.Trim(value, "_")
}

func containsAnyGrokErrorMarker(value string, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func grokRetryableOnSameAccount(account *Account, statusCode int, permanentlyDisabled bool) bool {
	return !permanentlyDisabled && account != nil && account.IsPoolMode() && account.IsPoolModeRetryableStatus(statusCode)
}

// isGrokSpendingLimitError detects xAI billing exhaustion bodies (often 403, sometimes 402).
func isGrokSpendingLimitError(responseBody []byte) bool {
	if len(responseBody) == 0 {
		return false
	}
	code := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		gjson.GetBytes(responseBody, "code").String(),
		gjson.GetBytes(responseBody, "error.code").String(),
	)))
	if code == "personal-team-blocked:spending-limit" {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		gjson.GetBytes(responseBody, "error").String(),
		gjson.GetBytes(responseBody, "error.message").String(),
		gjson.GetBytes(responseBody, "message").String(),
	)))
	return strings.Contains(message, "spending limit") ||
		strings.Contains(message, "run out of credits")
}

func (s *OpenAIGatewayService) tempUnscheduleGrok(ctx context.Context, account *Account, cooldown time.Duration, reason string) {
	if s == nil || account == nil {
		return
	}
	until := time.Now().Add(cooldown)
	if account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(until) {
		until = *account.TempUnschedulableUntil
	}
	s.BlockAccountScheduling(account, until, reason)
	if s.accountRepo != nil {
		stateCtx, cancel := openAIAccountStateContext(ctx)
		defer cancel()
		_ = s.accountRepo.SetTempUnschedulable(stateCtx, account.ID, until, reason)
	}
}

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type openAICompactFallbackSignal struct {
	payload []byte
	message string
}

const (
	// The initial model counts as one attempt. Every retry must select a new
	// candidate, and the chain remains bounded even when upstream keeps
	// rejecting models.
	openAICompactFallbackMaxAttempts = 4
	// This was the previous default and is retained as a bounded probe only; it
	// is not treated as a universal Lite capability assertion.
	legacyOpenAICompactFallbackModel = "gpt-5.4"
)

type openAICompactFallbackState struct {
	tried    map[string]struct{}
	attempts int
}

func newOpenAICompactFallbackState() *openAICompactFallbackState {
	return &openAICompactFallbackState{tried: make(map[string]struct{})}
}

func (s *openAICompactFallbackState) mark(model string) {
	if s == nil {
		return
	}
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return
	}
	if s.tried == nil {
		s.tried = make(map[string]struct{})
	}
	if _, exists := s.tried[model]; exists {
		return
	}
	s.tried[model] = struct{}{}
	s.attempts++
}

func (s *openAICompactFallbackState) hasTried(model string) bool {
	if s == nil {
		return false
	}
	_, exists := s.tried[strings.ToLower(strings.TrimSpace(model))]
	return exists
}

func (s *openAICompactFallbackState) canTry(ctx context.Context) bool {
	return s != nil && (ctx == nil || ctx.Err() == nil) && s.attempts < openAICompactFallbackMaxAttempts
}

func (e *openAICompactFallbackSignal) Error() string {
	if e == nil || strings.TrimSpace(e.message) == "" {
		return "upstream compact request failed"
	}
	return e.message
}

func asOpenAICompactFallbackSignal(err error) (*openAICompactFallbackSignal, bool) {
	var signal *openAICompactFallbackSignal
	return signal, errors.As(err, &signal) && signal != nil
}

func isExplicitOpenAICompactContext(c *gin.Context) bool {
	return isOpenAIResponsesCompactPath(c) || isOpenAINativeCompactionV2(c)
}

func newOpenAICompactFallbackSignal(c *gin.Context, payload []byte, message string) error {
	if !isExplicitOpenAICompactContext(c) ||
		!isOpenAICompactModelFailure(http.StatusBadRequest, message, payload) {
		return nil
	}
	return &openAICompactFallbackSignal{
		payload: append([]byte(nil), payload...),
		message: sanitizeUpstreamErrorMessage(strings.TrimSpace(message)),
	}
}

func isExplicitOpenAICompactRequest(c *gin.Context, body []byte) bool {
	return isOpenAIResponsesCompactPath(c) || HasCompactionTriggerInInput(body)
}

// resolveOpenAICompactFallbackModel prefers the account's compact-only rule
// for the client-visible model. The process-wide fallback is used only when
// that account has no matching compact rule.
func (s *OpenAIGatewayService) resolveOpenAICompactFallbackModel(account *Account, requestedModel string) string {
	requestedModel = strings.TrimSpace(requestedModel)
	if account != nil {
		if mapped, matched := account.ResolveCompactMappedModel(requestedModel); matched {
			if mapped = strings.TrimSpace(mapped); mapped != "" {
				return mapped
			}
		}
	}
	if s == nil || s.cfg == nil {
		return ""
	}
	fallback := strings.TrimSpace(s.cfg.Gateway.OpenAICompactModel)
	if fallback == "" {
		return ""
	}
	return strings.TrimSpace(resolveOpenAIAccountUpstreamModelForRequest(account, fallback, false))
}

func isOpenAICompactModelFailure(statusCode int, upstreamMsg string, upstreamBody []byte) bool {
	if isOpenAIResponsesLiteModelCompatibilityFailure(statusCode, upstreamMsg, upstreamBody) {
		return true
	}
	if isOpenAIContextWindowError(upstreamMsg, upstreamBody) {
		return true
	}
	if statusCode != http.StatusBadRequest && statusCode != http.StatusNotFound {
		return false
	}

	values := []string{
		extractUpstreamErrorCode(upstreamBody),
		upstreamMsg,
		gjson.GetBytes(upstreamBody, "error.type").String(),
		gjson.GetBytes(upstreamBody, "response.error.code").String(),
		gjson.GetBytes(upstreamBody, "response.error.type").String(),
	}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		switch value {
		case "model_not_found", "model_not_available", "unsupported_model", "invalid_model":
			return true
		}
		if isExplicitOpenAIModelAvailabilityMessage(value) {
			return true
		}
	}
	// Some compact providers return only a failed response shell. It is safe to
	// retry that shape for an explicit compact request, but a populated error is
	// left untouched so business and policy failures keep their original wire.
	if strings.EqualFold(strings.TrimSpace(gjson.GetBytes(upstreamBody, "response.status").String()), "failed") ||
		strings.EqualFold(strings.TrimSpace(gjson.GetBytes(upstreamBody, "status").String()), "failed") {
		for _, path := range []string{
			"error.message", "error.code", "error.type",
			"response.error.message", "response.error.code", "response.error.type",
		} {
			if strings.TrimSpace(gjson.GetBytes(upstreamBody, path).String()) != "" {
				return false
			}
		}
		return strings.TrimSpace(upstreamMsg) == ""
	}
	return false
}

func isOpenAIResponsesLiteModelCompatibilityFailure(statusCode int, upstreamMsg string, upstreamBody []byte) bool {
	if statusCode != http.StatusBadRequest && statusCode != http.StatusNotFound {
		return false
	}
	values := []string{
		upstreamMsg,
		extractUpstreamErrorCode(upstreamBody),
		gjson.GetBytes(upstreamBody, "error.message").String(),
		gjson.GetBytes(upstreamBody, "error.param").String(),
		gjson.GetBytes(upstreamBody, "response.error.message").String(),
	}
	joined := strings.ToLower(strings.Join(values, " "))
	return strings.Contains(joined, "x-openai-internal-codex-responses-lite") &&
		strings.Contains(joined, "model") &&
		(strings.Contains(joined, "not supported") || strings.Contains(joined, "unsupported"))
}

func isExplicitOpenAIModelAvailabilityMessage(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	for _, phrase := range []string{
		"model not found",
		"model does not exist",
		"model is unavailable",
		"model is not available",
		"model is unsupported",
		"model is not supported",
		"unsupported model",
	} {
		if strings.Contains(value, phrase) {
			return true
		}
	}
	// OpenAI commonly identifies the missing model between the word "model"
	// and the terminal availability phrase, for example: "The model `x` does
	// not exist". Requiring the message to start with the model subject avoids
	// treating unrelated feature errors such as "model output is not supported"
	// as a signal to change models.
	if strings.HasPrefix(value, "the model ") || strings.HasPrefix(value, "model ") {
		return strings.Contains(value, " does not exist") ||
			strings.Contains(value, " was not found") ||
			strings.Contains(value, " is unavailable") ||
			strings.Contains(value, " is not available")
	}
	return false
}

func openAICompactFallbackErrorResponse(resp *http.Response, signal *openAICompactFallbackSignal) (*http.Response, []byte) {
	headers := make(http.Header)
	if resp != nil {
		headers = resp.Header.Clone()
	}
	if headers.Get("Content-Type") == "" {
		headers.Set("Content-Type", "application/json")
	}
	payload := normalizeOpenAICompactFallbackHTTPErrorPayload(signal)
	return &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     headers,
		Body:       io.NopCloser(bytes.NewReader(payload)),
	}, payload
}

func normalizeOpenAICompactFallbackHTTPErrorPayload(signal *openAICompactFallbackSignal) []byte {
	if signal == nil {
		return nil
	}
	payload := append([]byte(nil), signal.payload...)
	var terminal struct {
		Error    json.RawMessage `json:"error"`
		Response struct {
			Error json.RawMessage `json:"error"`
		} `json:"response"`
	}
	if json.Unmarshal(payload, &terminal) != nil || len(bytes.TrimSpace(terminal.Response.Error)) == 0 ||
		bytes.Equal(bytes.TrimSpace(terminal.Response.Error), []byte("null")) {
		return payload
	}
	// Standard HTTP error handlers consume error.message/type/code. A streamed
	// response.failed terminal nests the same object under response.error, so
	// normalize only that envelope at the stream-to-HTTP boundary.
	normalized, err := json.Marshal(struct {
		Error json.RawMessage `json:"error"`
	}{Error: terminal.Response.Error})
	if err != nil {
		return payload
	}
	return normalized
}

func (s *OpenAIGatewayService) appendOpenAICompactFallbackRetryOps(
	c *gin.Context,
	account *Account,
	resp *http.Response,
	payload []byte,
	message string,
	passthrough bool,
) {
	if account == nil {
		return
	}
	statusCode := http.StatusBadRequest
	requestID := ""
	if resp != nil {
		statusCode = resp.StatusCode
		requestID = resp.Header.Get("x-request-id")
	}
	detail := ""
	if s != nil && s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		detail = truncateString(string(payload), maxBytes)
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		ProxyID:              opsUpstreamProxyID(account),
		ProxyName:            opsUpstreamProxyName(account),
		Platform:             account.Platform,
		AccountID:            account.ID,
		AccountName:          account.Name,
		UpstreamStatusCode:   statusCode,
		UpstreamRequestID:    requestID,
		Passthrough:          passthrough,
		Kind:                 "retry",
		Reason:               "compact_model_fallback",
		Message:              sanitizeUpstreamErrorMessage(strings.TrimSpace(message)),
		Detail:               detail,
		UpstreamResponseBody: detail,
	})
}

func (s *OpenAIGatewayService) openAICompactFallbackCandidates(account *Account, requestedModel, currentModel string, allowLegacyProbe bool) []string {
	candidates := make([]string, 0, openAICompactFallbackMaxAttempts)
	seen := make(map[string]struct{})
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" || strings.EqualFold(model, currentModel) {
			return
		}
		key := strings.ToLower(model)
		if _, exists := seen[key]; exists || !openAICompactCandidateMayUseResponsesLite(account, model) {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, model)
	}

	if account != nil {
		if mapped, matched := account.ResolveCompactMappedModel(requestedModel); matched {
			add(resolveOpenAIAccountUpstreamModelForRequest(account, mapped, false))
		}
	}
	if s != nil && s.cfg != nil {
		add(resolveOpenAIAccountUpstreamModelForRequest(account, s.cfg.Gateway.OpenAICompactModel, false))
	}

	// A synced Codex manifest is the strongest local capability evidence. Keep
	// its order deterministic because Models is a map and cap the resulting
	// chain in the caller.
	if account != nil {
		if snapshot := account.GetUpstreamModelMetadataSnapshot(); snapshot != nil {
			modelIDs := make([]string, 0, len(snapshot.Models))
			for modelID := range snapshot.Models {
				if openAIModelUsesResponsesLite(snapshot.Models[modelID]) {
					modelIDs = append(modelIDs, modelID)
				}
			}
			sort.Strings(modelIDs)
			for _, modelID := range modelIDs {
				add(resolveOpenAIAccountUpstreamModelForRequest(account, modelID, false))
			}
		}
	}

	// Keep the pre-gpt-5.5 default as a bounded live probe. This is not an
	// allowlist: an upstream that rejects it simply consumes one attempt.
	if allowLegacyProbe && strings.EqualFold(strings.TrimSpace(currentModel), "gpt-5.5") {
		add(legacyOpenAICompactFallbackModel)
	}
	return candidates
}

func openAIModelUsesResponsesLite(metadata UpstreamModelMetadata) bool {
	value, ok := metadata.CodexToolCapabilities["use_responses_lite"]
	if !ok {
		return false
	}
	var enabled bool
	return json.Unmarshal(value, &enabled) == nil && enabled
}

func openAICompactCandidateMayUseResponsesLite(account *Account, model string) bool {
	if account == nil {
		return true
	}
	model = strings.TrimSpace(model)
	target := account.GetMappedModel(model)
	if account.Platform == PlatformOpenAI && account.Type == AccountTypeAPIKey {
		if _, excluded := apiKeyCodexModelsWithoutResponsesLite[target]; excluded {
			return false
		}
	}
	for _, modelID := range []string{model, target} {
		if metadata, ok := account.GetUpstreamModelMetadata(modelID); ok {
			value, exists := metadata.CodexToolCapabilities["use_responses_lite"]
			if !exists {
				continue
			}
			var enabled bool
			if json.Unmarshal(value, &enabled) == nil {
				return enabled
			}
		}
	}
	return true
}

// prepareOpenAICompactFallbackRetry returns a body for one safe, same-account
// retry. Callers invoke it only before any downstream response has been
// written; it changes the model and deliberately leaves path, trigger, and
// native-v2 context state untouched.
func (s *OpenAIGatewayService) prepareOpenAICompactFallbackRetry(
	c *gin.Context,
	account *Account,
	requestedModel string,
	currentBody []byte,
	statusCode int,
	upstreamMsg string,
	upstreamBody []byte,
	alreadyRetried bool,
) ([]byte, string, bool) {
	if alreadyRetried {
		return currentBody, "", false
	}
	state := newOpenAICompactFallbackState()
	return s.prepareOpenAICompactFallbackRetryWithState(
		c, context.Background(), account, requestedModel, currentBody, statusCode, upstreamMsg, upstreamBody, state,
	)
}

func (s *OpenAIGatewayService) prepareOpenAICompactFallbackRetryWithState(
	c *gin.Context,
	ctx context.Context,
	account *Account,
	requestedModel string,
	currentBody []byte,
	statusCode int,
	upstreamMsg string,
	upstreamBody []byte,
	state *openAICompactFallbackState,
) ([]byte, string, bool) {
	if !isExplicitOpenAICompactRequest(c, currentBody) ||
		!isOpenAIResponsesLiteModelCompatibilityFailure(statusCode, upstreamMsg, upstreamBody) {
		return currentBody, "", false
	}
	currentModel := strings.TrimSpace(gjson.GetBytes(currentBody, "model").String())
	if state == nil {
		state = newOpenAICompactFallbackState()
	}
	state.mark(currentModel)
	if !state.canTry(ctx) {
		return currentBody, "", false
	}
	for _, fallbackModel := range s.openAICompactFallbackCandidates(
		account,
		requestedModel,
		currentModel,
		isOpenAIResponsesLiteModelCompatibilityFailure(statusCode, upstreamMsg, upstreamBody),
	) {
		if state.hasTried(fallbackModel) {
			continue
		}
		retryBody := ReplaceModelInBody(currentBody, fallbackModel)
		if strings.EqualFold(strings.TrimSpace(gjson.GetBytes(retryBody, "model").String()), currentModel) {
			continue
		}
		state.mark(fallbackModel)
		return retryBody, fallbackModel, true
	}
	return currentBody, "", false
}

func (s *OpenAIGatewayService) applyOpenAIPassthroughCompactFallbackFromSignal(
	c *gin.Context,
	account *Account,
	requestedModel string,
	body []byte,
	err error,
	alreadyRetried bool,
	resp *http.Response,
) ([]byte, string, bool) {
	return s.applyOpenAIPassthroughCompactFallbackFromSignalWithState(
		c, context.Background(), account, requestedModel, body, err, alreadyRetried, resp, nil,
	)
}

func (s *OpenAIGatewayService) applyOpenAIPassthroughCompactFallbackFromSignalWithState(
	c *gin.Context,
	ctx context.Context,
	account *Account,
	requestedModel string,
	body []byte,
	err error,
	alreadyRetried bool,
	resp *http.Response,
	state *openAICompactFallbackState,
) ([]byte, string, bool) {
	signal, ok := asOpenAICompactFallbackSignal(err)
	if !ok {
		return body, "", false
	}
	if state == nil {
		state = newOpenAICompactFallbackState()
		if alreadyRetried {
			state.mark(gjson.GetBytes(body, "model").String())
			state.attempts = openAICompactFallbackMaxAttempts
		}
	}
	retryBody, fallbackModel, retry := s.prepareOpenAICompactFallbackRetryWithState(
		c, ctx, account, requestedModel, body, http.StatusBadRequest, signal.message, signal.payload, state,
	)
	if !retry {
		return body, "", false
	}
	s.appendOpenAICompactFallbackRetryOps(c, account, resp, signal.payload, signal.message, true)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	fromModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	accountName := ""
	if account != nil {
		accountName = account.Name
	}
	SetOpsUpstreamModel(c, fallbackModel)
	logger.LegacyPrintf(
		"service.openai_gateway",
		"[OpenAI passthrough] Retrying explicit compact request with fallback model (account: %s, from: %s, to: %s, upstream_code: %s)",
		accountName, fromModel, fallbackModel, extractUpstreamErrorCode(signal.payload),
	)
	return retryBody, fallbackModel, true
}

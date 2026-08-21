package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type openAICompactFallbackSignal struct {
	payload []byte
	message string
REDACTED

func (e *openAICompactFallbackSignal) Error() string {
	if e == nil || strings.TrimSpace(e.message) == "" {
		return "upstream compact request failed"
REDACTED
	return e.message
REDACTED

func asOpenAICompactFallbackSignal(err error) (*openAICompactFallbackSignal, bool) {
	var signal *openAICompactFallbackSignal
	return signal, errors.As(err, &signal) && signal != nil
REDACTED

func isExplicitOpenAICompactContext(c *gin.Context) bool {
	return isOpenAIResponsesCompactPath(c) || isOpenAINativeCompactionV2(c)
REDACTED

func newOpenAICompactFallbackSignal(c *gin.Context, payload []byte, message string) error {
	if !isExplicitOpenAICompactContext(c) ||
		!isOpenAICompactModelFailure(http.StatusBadRequest, message, payload) {
		return nil
REDACTED
	return &openAICompactFallbackSignal{
		payload: append([]byte(nil), payload...),
		message: sanitizeUpstreamErrorMessage(strings.TrimSpace(message)),
REDACTED
REDACTED

func isExplicitOpenAICompactRequest(c *gin.Context, body []byte) bool {
	return isOpenAIResponsesCompactPath(c) || HasCompactionTriggerInInput(body)
REDACTED

// resolveOpenAICompactFallbackModel prefers the account's compact-only rule
// for the client-visible model. The process-wide fallback is used only when
// that account has no matching compact rule.
func (s *OpenAIGatewayService) resolveOpenAICompactFallbackModel(account *Account, requestedModel string) string {
	requestedModel = strings.TrimSpace(requestedModel)
	if account != nil {
		if mapped, matched := account.ResolveCompactMappedModel(requestedModel); matched {
			if mapped = strings.TrimSpace(mapped); mapped != "" {
				return mapped
		REDACTED
	REDACTED
REDACTED
	if s == nil || s.cfg == nil {
		return ""
REDACTED
	fallback := strings.TrimSpace(s.cfg.Gateway.OpenAICompactModel)
	if fallback == "" {
		return ""
REDACTED
	return strings.TrimSpace(resolveOpenAIAccountUpstreamModelForRequest(account, fallback, false))
REDACTED

func isOpenAICompactModelFailure(statusCode int, upstreamMsg string, upstreamBody []byte) bool {
	if isOpenAIContextWindowError(upstreamMsg, upstreamBody) {
		return true
REDACTED
	if statusCode != http.StatusBadRequest && statusCode != http.StatusNotFound {
		return false
REDACTED

	values := []string{
		extractUpstreamErrorCode(upstreamBody),
		upstreamMsg,
		gjson.GetBytes(upstreamBody, "error.type").String(),
		gjson.GetBytes(upstreamBody, "response.error.code").String(),
		gjson.GetBytes(upstreamBody, "response.error.type").String(),
REDACTED
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		switch value {
		case "model_not_found", "model_not_available", "unsupported_model", "invalid_model":
			return true
	REDACTED
		if isExplicitOpenAIModelAvailabilityMessage(value) {
			return true
	REDACTED
REDACTED
	// Some compact providers return only a failed response shell. It is safe to
	// retry that shape for an explicit compact request, but a populated error is
	// left untouched so business and policy failures keep their original wire.
	if strings.EqualFold(strings.TrimSpace(gjson.GetBytes(upstreamBody, "response.status").String()), "failed") ||
		strings.EqualFold(strings.TrimSpace(gjson.GetBytes(upstreamBody, "status").String()), "failed") {
		for _, path := range []string{
			"error.message", "error.code", "error.type",
			"response.error.message", "response.error.code", "response.error.type",
	REDACTED {
			if strings.TrimSpace(gjson.GetBytes(upstreamBody, path).String()) != "" {
				return false
		REDACTED
	REDACTED
		return strings.TrimSpace(upstreamMsg) == ""
REDACTED
	return false
REDACTED

func isExplicitOpenAIModelAvailabilityMessage(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
REDACTED
	for _, phrase := range []string{
		"model not found",
		"model does not exist",
		"model is unavailable",
		"model is not available",
		"model is unsupported",
		"model is not supported",
		"unsupported model",
REDACTED {
		if strings.Contains(value, phrase) {
			return true
	REDACTED
REDACTED
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
REDACTED
	return false
REDACTED

func openAICompactFallbackErrorResponse(resp *http.Response, signal *openAICompactFallbackSignal) (*http.Response, []byte) {
	headers := make(http.Header)
	if resp != nil {
		headers = resp.Header.Clone()
REDACTED
	if headers.Get("Content-Type") == "" {
		headers.Set("Content-Type", "application/json")
REDACTED
	payload := normalizeOpenAICompactFallbackHTTPErrorPayload(signal)
	return &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     headers,
		Body:       io.NopCloser(bytes.NewReader(payload)),
REDACTED, payload
REDACTED

func normalizeOpenAICompactFallbackHTTPErrorPayload(signal *openAICompactFallbackSignal) []byte {
	if signal == nil {
		return nil
REDACTED
	payload := append([]byte(nil), signal.payload...)
	var terminal struct {
		Error    json.RawMessage `json:"error"`
		Response struct {
			Error json.RawMessage `json:"error"`
	REDACTED `json:"response"`
REDACTED
	if json.Unmarshal(payload, &terminal) != nil || len(bytes.TrimSpace(terminal.Response.Error)) == 0 ||
		bytes.Equal(bytes.TrimSpace(terminal.Response.Error), []byte("null")) {
		return payload
REDACTED
	// Standard HTTP error handlers consume error.message/type/code. A streamed
	// response.failed terminal nests the same object under response.error, so
	// normalize only that envelope at the stream-to-HTTP boundary.
	normalized, err := json.Marshal(struct {
		Error json.RawMessage `json:"error"`
REDACTED{Error: terminal.Response.ErrorREDACTED)
	if err != nil {
		return payload
REDACTED
	return normalized
REDACTED

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
REDACTED
	statusCode := http.StatusBadRequest
	requestID := ""
	if resp != nil {
		statusCode = resp.StatusCode
		requestID = resp.Header.Get("x-request-id")
REDACTED
	detail := ""
	if s != nil && s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
	REDACTED
		detail = truncateString(string(payload), maxBytes)
REDACTED
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
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
REDACTED)
REDACTED

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
	if alreadyRetried || !isExplicitOpenAICompactRequest(c, currentBody) ||
		!isOpenAICompactModelFailure(statusCode, upstreamMsg, upstreamBody) {
		return currentBody, "", false
REDACTED
	fallbackModel := s.resolveOpenAICompactFallbackModel(account, requestedModel)
	currentModel := strings.TrimSpace(gjson.GetBytes(currentBody, "model").String())
	if fallbackModel == "" || strings.EqualFold(fallbackModel, currentModel) {
		return currentBody, "", false
REDACTED
	retryBody := ReplaceModelInBody(currentBody, fallbackModel)
	if strings.EqualFold(strings.TrimSpace(gjson.GetBytes(retryBody, "model").String()), currentModel) {
		return currentBody, "", false
REDACTED
	return retryBody, fallbackModel, true
REDACTED

func (s *OpenAIGatewayService) applyOpenAIPassthroughCompactFallbackFromSignal(
	c *gin.Context,
	account *Account,
	requestedModel string,
	body []byte,
	err error,
	alreadyRetried bool,
	resp *http.Response,
) ([]byte, string, bool) {
	signal, ok := asOpenAICompactFallbackSignal(err)
	if !ok {
		return body, "", false
REDACTED
	retryBody, fallbackModel, retry := s.prepareOpenAICompactFallbackRetry(
		c, account, requestedModel, body, http.StatusBadRequest, signal.message, signal.payload, alreadyRetried,
	)
	if !retry {
		return body, "", false
REDACTED
	s.appendOpenAICompactFallbackRetryOps(c, account, resp, signal.payload, signal.message, true)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
REDACTED
	fromModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	accountName := ""
	if account != nil {
		accountName = account.Name
REDACTED
	SetOpsUpstreamModel(c, fallbackModel)
	logger.LegacyPrintf(
		"service.openai_gateway",
		"[OpenAI passthrough] Retrying explicit compact request once with fallback model (account: %s, from: %s, to: %s, upstream_code: %s)",
		accountName, fromModel, fallbackModel, extractUpstreamErrorCode(signal.payload),
	)
	return retryBody, fallbackModel, true
REDACTED

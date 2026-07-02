package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tiktoken-go/tokenizer"
	"go.uber.org/zap"
)

const (
	openAIResponsesInputItemTokenOverhead = 3
	openAIResponsesContentPartOverhead    = 1
	openAIInputTokensFallbackMinimum      = 1
)

type openAIInputTokensCountRequest struct {
	Model        string                    `json:"model"`
	Instructions string                    `json:"instructions,omitempty"`
	Input        json.RawMessage           `json:"input,omitempty"`
	Tools        []apicompat.ResponsesTool `json:"tools,omitempty"`
	ToolChoice   json.RawMessage           `json:"tool_choice,omitempty"`
REDACTED

type openAIInputTokensCountPrepared struct {
	Request         openAIInputTokensCountRequest
	OriginalModel   string
	NormalizedModel string
	BillingModel    string
	UpstreamModel   string
REDACTED

// ForwardCountTokensAsAnthropic bridges Anthropic /v1/messages/count_tokens to
// OpenAI POST /v1/responses/input_tokens and returns Anthropic-compatible output.
func (s *OpenAIGatewayService) ForwardCountTokensAsAnthropic(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) error {
	if account == nil {
		writeAnthropicCountTokensError(c, http.StatusServiceUnavailable, "api_error", "No available OpenAI accounts")
		return fmt.Errorf("count_tokens: missing account")
REDACTED

	prepared, err := prepareOpenAIInputTokensCountRequest(body, account, defaultMappedModel)
	if err != nil {
		writeAnthropicCountTokensError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return err
REDACTED

	upstreamBody, err := marshalOpenAIUpstreamJSON(prepared.Request)
	if err != nil {
		writeAnthropicCountTokensError(c, http.StatusInternalServerError, "api_error", "Failed to build request")
		return fmt.Errorf("marshal openai input_tokens body: %w", err)
REDACTED

	logger.L().Debug("openai count_tokens: model mapping applied",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", prepared.OriginalModel),
		zap.String("normalized_model", prepared.NormalizedModel),
		zap.String("billing_model", prepared.BillingModel),
		zap.String("upstream_model", prepared.UpstreamModel),
	)

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		writeAnthropicCountTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to get access token")
		return fmt.Errorf("get access token: %w", err)
REDACTED

	upstreamReq, err := s.buildInputTokensUpstreamRequest(ctx, c, account, upstreamBody, token)
	if err != nil {
		writeAnthropicCountTokensError(c, http.StatusInternalServerError, "api_error", "Failed to build request")
		return fmt.Errorf("build input_tokens request: %w", err)
REDACTED

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		writeAnthropicCountTokensError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		return fmt.Errorf("openai input_tokens upstream request failed: %s", safeErr)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		writeAnthropicCountTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to read response")
		return fmt.Errorf("read input_tokens response: %w", err)
REDACTED

	if resp.StatusCode >= 400 {
		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		if account.Type == AccountTypeOAuth && isOpenAIOAuthInputTokensUnsupported(resp.StatusCode, respBody) {
			writeOpenAIOAuthInputTokensFallback(c, account, prepared, resp.StatusCode)
			return nil
	REDACTED

		if s.rateLimitService != nil {
			s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
	REDACTED

		if isOpenAIInputTokensUnsupported(resp.StatusCode, respBody) {
			writeAnthropicCountTokensError(c, http.StatusNotFound, "not_found_error", "Token counting is not supported by upstream")
			return nil
	REDACTED

		upstreamDetail := ""
		if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
			maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
			if maxBytes <= 0 {
				maxBytes = 2048
		REDACTED
			upstreamDetail = truncateString(string(respBody), maxBytes)
	REDACTED
		setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)

		errMsg := "Upstream request failed"
		switch resp.StatusCode {
		case 429:
			errMsg = "Rate limit exceeded"
		case 500, 502, 503, 504, 529:
			errMsg = "Upstream service temporarily unavailable"
	REDACTED
		writeAnthropicCountTokensError(c, resp.StatusCode, "upstream_error", errMsg)
		if upstreamMsg == "" {
			return fmt.Errorf("input_tokens upstream error: %d", resp.StatusCode)
	REDACTED
		return fmt.Errorf("input_tokens upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
REDACTED

	inputTokens := gjson.GetBytes(respBody, "input_tokens")
	if !inputTokens.Exists() {
		writeAnthropicCountTokensError(c, http.StatusBadGateway, "upstream_error", "Upstream response missing input_tokens")
		return fmt.Errorf("input_tokens response missing input_tokens field")
REDACTED

	c.JSON(http.StatusOK, gin.H{
		"input_tokens": int(inputTokens.Int()),
REDACTED)
	return nil
REDACTED

func prepareOpenAIInputTokensCountRequest(
	body []byte,
	account *Account,
	defaultMappedModel string,
) (*openAIInputTokensCountPrepared, error) {
	var anthropicReq apicompat.AnthropicRequest
	if err := json.Unmarshal(body, &anthropicReq); err != nil {
		return nil, fmt.Errorf("parse anthropic count_tokens request: %w", err)
REDACTED

	originalModel := anthropicReq.Model
	applyOpenAICompatModelNormalization(&anthropicReq)
	normalizedModel := anthropicReq.Model
	billingModel := resolveOpenAIForwardModel(account, normalizedModel, strings.TrimSpace(defaultMappedModel))
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)

	responsesReq, err := apicompat.AnthropicToResponses(&anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("convert anthropic request to responses: %w", err)
REDACTED

	return &openAIInputTokensCountPrepared{
		Request: openAIInputTokensCountRequest{
			Model:        upstreamModel,
			Instructions: responsesReq.Instructions,
			Input:        responsesReq.Input,
			Tools:        responsesReq.Tools,
			ToolChoice:   responsesReq.ToolChoice,
	REDACTED,
		OriginalModel:   originalModel,
		NormalizedModel: normalizedModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
REDACTED, nil
REDACTED

func (s *OpenAIGatewayService) buildInputTokensUpstreamRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
) (*http.Request, error) {
	targetURL := openaiPlatformAPIInputTokensURL
	if account.Type == AccountTypeAPIKey {
		if baseURL := account.GetOpenAIBaseURL(); strings.TrimSpace(baseURL) != "" {
			validatedURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, err
		REDACTED
			targetURL = buildOpenAIResponsesInputTokensURL(validatedURL)
	REDACTED
REDACTED

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
REDACTED
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("authorization", "Bearer "+token)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")

	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lower := strings.ToLower(strings.TrimSpace(key))
			if lower != "user-agent" && lower != "accept-language" {
				continue
		REDACTED
			for _, v := range values {
				req.Header.Add(key, v)
		REDACTED
	REDACTED
REDACTED

	return req, nil
REDACTED

func writeAnthropicCountTokensError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
	REDACTED,
REDACTED)
REDACTED

func isOpenAIInputTokensUnsupported(statusCode int, body []byte) bool {
	if statusCode != http.StatusNotFound {
		return false
REDACTED
	msg := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	return strings.Contains(msg, "input_tokens") && strings.Contains(msg, "not found")
REDACTED

func writeOpenAIOAuthInputTokensFallback(c *gin.Context, account *Account, prepared *openAIInputTokensCountPrepared, statusCode int) {
	estimated := openAIInputTokensFallbackMinimum
	if got, err := estimateOpenAIInputTokens(prepared.Request); err == nil {
		if got > 0 {
			estimated = got
	REDACTED
		logger.L().Info("openai count_tokens: oauth fallback to local tiktoken estimate",
			zap.Int64("account_id", account.ID),
			zap.Int("upstream_status", statusCode),
			zap.Int("estimated_input_tokens", estimated),
			zap.String("upstream_model", prepared.UpstreamModel),
		)
REDACTED else {
		logger.L().Warn("openai count_tokens: oauth local tiktoken fallback failed, using minimum estimate",
			zap.Int64("account_id", account.ID),
			zap.Int("upstream_status", statusCode),
			zap.Int("estimated_input_tokens", estimated),
			zap.String("upstream_model", prepared.UpstreamModel),
			zap.Error(err),
		)
REDACTED

	c.JSON(http.StatusOK, gin.H{
		"input_tokens": estimated,
REDACTED)
REDACTED

func isOpenAIOAuthInputTokensUnsupported(statusCode int, body []byte) bool {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
	default:
		return false
REDACTED

	bodyLower := strings.ToLower(string(body))
	msg := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	code := strings.ToLower(strings.TrimSpace(extractUpstreamErrorCode(body)))

	if code == "missing_scope" ||
		strings.Contains(bodyLower, "api.responses.write") ||
		strings.Contains(bodyLower, "missing scopes") ||
		strings.Contains(bodyLower, "insufficient_scope") {
		return true
REDACTED

	if statusCode == http.StatusNotFound && isOpenAIInputTokensUnsupported(statusCode, body) {
		return true
REDACTED

	return strings.Contains(msg, "input_tokens") &&
		(strings.Contains(msg, "not found") ||
			strings.Contains(msg, "not supported") ||
			strings.Contains(msg, "unsupported"))
REDACTED

func estimateOpenAIInputTokens(req openAIInputTokensCountRequest) (int, error) {
	codec, err := openAIInputTokensCodecForModel(req.Model)
	if err != nil {
		return 0, err
REDACTED

	total := 0
	addCount := func(text string) error {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil
	REDACTED
		n, err := codec.Count(text)
		if err != nil {
			return err
	REDACTED
		total += n
		return nil
REDACTED

	if err := addCount(req.Instructions); err != nil {
		return 0, err
REDACTED
	inputTokens, err := estimateOpenAIInputTokensForInput(codec, req.Input)
	if err != nil {
		return 0, err
REDACTED
	total += inputTokens

	for _, tool := range req.Tools {
		raw, err := marshalOpenAIUpstreamJSON(tool)
		if err != nil {
			return 0, err
	REDACTED
		if err := addCount(string(raw)); err != nil {
			return 0, err
	REDACTED
REDACTED
	if len(req.ToolChoice) > 0 {
		compacted, err := compactOpenAIInputTokensJSON(req.ToolChoice)
		if err != nil {
			return 0, err
	REDACTED
		if err := addCount(compacted); err != nil {
			return 0, err
	REDACTED
REDACTED

	if total < 0 {
		return 0, nil
REDACTED
	return total, nil
REDACTED

func estimateOpenAIInputTokensForInput(codec tokenizer.Codec, raw json.RawMessage) (int, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return 0, nil
REDACTED

	var plainText string
	if err := json.Unmarshal(raw, &plainText); err == nil {
		return codec.Count(plainText)
REDACTED

	var items []apicompat.ResponsesInputItem
	if err := json.Unmarshal(raw, &items); err == nil {
		return estimateOpenAIInputTokensForInputItems(codec, items)
REDACTED

	compacted, err := compactOpenAIInputTokensJSON(raw)
	if err != nil {
		return 0, err
REDACTED
	return codec.Count(compacted)
REDACTED

func estimateOpenAIInputTokensForInputItems(codec tokenizer.Codec, items []apicompat.ResponsesInputItem) (int, error) {
	total := 0
	countText := func(text string) error {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil
	REDACTED
		n, err := codec.Count(text)
		if err != nil {
			return err
	REDACTED
		total += n
		return nil
REDACTED

	for _, item := range items {
		total += openAIResponsesInputItemTokenOverhead
		if err := countText(item.Role); err != nil {
			return 0, err
	REDACTED
		if item.Type != "" && item.Type != "message" {
			if err := countText(item.Type); err != nil {
				return 0, err
		REDACTED
	REDACTED
		if err := countText(item.Name); err != nil {
			return 0, err
	REDACTED
		if err := countText(item.Arguments); err != nil {
			return 0, err
	REDACTED
		if err := countText(item.Output); err != nil {
			return 0, err
	REDACTED
		if err := countText(item.CallID); err != nil {
			return 0, err
	REDACTED
		if err := countText(item.ID); err != nil {
			return 0, err
	REDACTED

		if len(bytes.TrimSpace(item.Content)) == 0 {
			continue
	REDACTED

		var contentText string
		if err := json.Unmarshal(item.Content, &contentText); err == nil {
			if err := countText(contentText); err != nil {
				return 0, err
		REDACTED
			continue
	REDACTED

		var parts []apicompat.ResponsesContentPart
		if err := json.Unmarshal(item.Content, &parts); err == nil {
			for _, part := range parts {
				total += openAIResponsesContentPartOverhead
				switch part.Type {
				case "input_text", "output_text", "text":
					if err := countText(part.Text); err != nil {
						return 0, err
				REDACTED
				case "input_image":
					if err := countText(estimateOpenAIInputImageText(part.ImageURL)); err != nil {
						return 0, err
				REDACTED
				default:
					if err := countText(part.Type); err != nil {
						return 0, err
				REDACTED
			REDACTED
		REDACTED
			continue
	REDACTED

		compacted, err := compactOpenAIInputTokensJSON(item.Content)
		if err != nil {
			return 0, err
	REDACTED
		if err := countText(compacted); err != nil {
			return 0, err
	REDACTED
REDACTED

	return total, nil
REDACTED

func estimateOpenAIInputImageText(imageURL string) string {
	trimmed := strings.TrimSpace(imageURL)
	if trimmed == "" {
		return ""
REDACTED
	if strings.HasPrefix(strings.ToLower(trimmed), "data:") {
		if comma := strings.Index(trimmed, ","); comma > 0 {
			return trimmed[:comma]
	REDACTED
REDACTED
	return trimmed
REDACTED

func compactOpenAIInputTokensJSON(raw json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil
REDACTED
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return "", err
REDACTED
	return buf.String(), nil
REDACTED

func openAIInputTokensCodecForModel(model string) (tokenizer.Codec, error) {
	switch openAIInputTokensEncodingForModel(model) {
	case tokenizer.Cl100kBase:
		return tokenizer.Get(tokenizer.Cl100kBase)
	default:
		return tokenizer.Get(tokenizer.O200kBase)
REDACTED
REDACTED

func openAIInputTokensEncodingForModel(model string) tokenizer.Encoding {
	normalized := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(normalized, "gpt-3.5"),
		(strings.HasPrefix(normalized, "gpt-4") &&
			!strings.HasPrefix(normalized, "gpt-4o") &&
			!strings.HasPrefix(normalized, "gpt-4.1")),
		strings.HasPrefix(normalized, "text-embedding-"):
		return tokenizer.Cl100kBase
	default:
		return tokenizer.O200kBase
REDACTED
REDACTED

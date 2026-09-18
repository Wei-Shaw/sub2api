package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

func (s *OpenAIGatewayService) ForwardEmbeddings(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	if account == nil || !account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityEmbeddings) {
		return nil, errors.New("embeddings is not supported for this provider account")
	}
	if account != nil && account.Platform == PlatformGemini {
		return s.forwardGeminiEmbeddings(ctx, c, account, body, defaultMappedModel)
	}
	return s.forwardOpenAIEmbeddings(ctx, c, account, body, defaultMappedModel)
}

func (s *OpenAIGatewayService) forwardOpenAIEmbeddings(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	originalModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if originalModel == "" {
		writeOpenAIEmbeddingsError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in request")
	}

	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	SetOpsUpstreamModel(c, upstreamModel)
	upstreamBody := body
	if upstreamModel != originalModel {
		upstreamBody = ReplaceModelInBody(body, upstreamModel)
	}

	logger.L().Debug("openai embeddings: forwarding",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("billing_model", billingModel),
		zap.String("upstream_model", upstreamModel),
	)

	apiKey := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	// 协议感知：Anthropic 协议账号的凭证 base_url 指向 /anthropic 端点，
	// embeddings 需使用 OpenAI 格式 base。
	baseURL := account.GetOpenAIFormatBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	targetURL := buildOpenAIEmbeddingsURL(validatedURL)
	if account.IsOpenRouterAPIKey() {
		targetURL = buildOpenRouterEmbeddingsURL(validatedURL)
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(upstreamBody))
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstreamReq = upstreamReq.WithContext(WithHTTPUpstreamProfile(upstreamReq.Context(), HTTPUpstreamProfileOpenAI))
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	upstreamReq.Header.Set("Accept", "application/json")
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if openaiCCRawAllowedHeaders[lowerKey] {
			for _, v := range values {
				upstreamReq.Header.Add(key, v)
			}
		}
	}
	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		upstreamReq.Header.Set("user-agent", customUA)
	}

	// 账号级请求头覆写（仅 openai api_key 账号启用时生效）
	account.ApplyHeaderOverrides(upstreamReq.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.doOpenAIUpstream(upstreamReq, proxyURL, account)
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			ProxyID:            opsUpstreamProxyID(account),
			ProxyName:          opsUpstreamProxyName(account),
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
		})
		writeOpenAIEmbeddingsError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(account, resp.StatusCode, upstreamMsg, respBody) {
			upstreamDetail := ""
			if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
				maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
				if maxBytes <= 0 {
					maxBytes = 2048
				}
				upstreamDetail = truncateString(string(respBody), maxBytes)
			}
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				ProxyID:            opsUpstreamProxyID(account),
				ProxyName:          opsUpstreamProxyName(account),
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
			})
			shouldDisable := s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody, upstreamModel)
			retryableOnSameAccount := !shouldDisable && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode)
			if account.IsOpenAIOAuth() && resp.StatusCode == http.StatusTooManyRequests {
				return nil, s.newOpenAIAccountFailoverError(account, resp.StatusCode, resp.Header, respBody, upstreamMsg, shouldDisable, retryableOnSameAccount)
			}
			if isOpenAIHTTPUpstreamAccessStateError(resp.StatusCode, upstreamMsg, respBody) {
				return nil, newOpenAIUpstreamFailoverError(resp.StatusCode, resp.Header, respBody, upstreamMsg, retryableOnSameAccount)
			}
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody, RetryableOnSameAccount: retryableOnSameAccount}
		}
		writeOpenAIEmbeddingsUpstreamResponse(c, resp, respBody, s.responseHeaderFilter)
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			writeOpenAIEmbeddingsError(c, http.StatusBadGateway, "api_error", "Failed to read upstream response")
		}
		return nil, fmt.Errorf("read upstream body: %w", err)
	}

	writeOpenAIEmbeddingsUpstreamResponse(c, resp, respBody, s.responseHeaderFilter)

	return &OpenAIForwardResult{
		RequestID:       firstNonEmptyString(resp.Header.Get("x-request-id"), resp.Header.Get("request-id")),
		UpstreamHeaders: resp.Header,
		Usage:           extractOpenAIEmbeddingsUsage(respBody),
		Model:           originalModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
		Stream:          false,
		Duration:        time.Since(startTime),
	}, nil
}

type geminiEmbeddingPart struct {
	Text string `json:"text"`
}

type geminiEmbeddingContent struct {
	Parts []geminiEmbeddingPart `json:"parts"`
}

type geminiEmbedRequest struct {
	Model                string                 `json:"model,omitempty"`
	Content              geminiEmbeddingContent `json:"content"`
	OutputDimensionality *int                   `json:"outputDimensionality,omitempty"`
}

// translateOpenAIEmbeddingsToGemini converts the stable caller contract to the
// official AI Studio embedContent/batchEmbedContents request shapes.
func translateOpenAIEmbeddingsToGemini(body []byte) (string, string, []byte, error) {
	var request struct {
		Model          string          `json:"model"`
		Input          json.RawMessage `json:"input"`
		Dimensions     *int            `json:"dimensions"`
		EncodingFormat string          `json:"encoding_format"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return "", "", nil, fmt.Errorf("parse embeddings request: %w", err)
	}
	model := strings.TrimPrefix(strings.TrimSpace(request.Model), "models/")
	if model == "" {
		return "", "", nil, errors.New("model is required")
	}
	if request.Dimensions != nil && *request.Dimensions <= 0 {
		return "", "", nil, errors.New("dimensions must be positive")
	}
	if format := strings.TrimSpace(request.EncodingFormat); format != "" && format != "float" && format != "base64" {
		return "", "", nil, fmt.Errorf("unsupported encoding_format: %s", format)
	}

	var inputs []string
	if len(request.Input) == 0 {
		return "", "", nil, errors.New("input is required")
	}
	if request.Input[0] == '"' {
		var input string
		if err := json.Unmarshal(request.Input, &input); err != nil {
			return "", "", nil, errors.New("input must be a string or an array of strings")
		}
		inputs = []string{input}
	} else {
		if err := json.Unmarshal(request.Input, &inputs); err != nil || len(inputs) == 0 {
			return "", "", nil, errors.New("input must be a string or an array of strings")
		}
	}
	for _, input := range inputs {
		if input == "" {
			return "", "", nil, errors.New("input values must not be empty")
		}
	}

	if len(inputs) == 1 {
		payload, err := json.Marshal(geminiEmbedRequest{
			Content:              geminiEmbeddingContent{Parts: []geminiEmbeddingPart{{Text: inputs[0]}}},
			OutputDimensionality: request.Dimensions,
		})
		return model, "embedContent", payload, err
	}
	requests := make([]geminiEmbedRequest, 0, len(inputs))
	for _, input := range inputs {
		requests = append(requests, geminiEmbedRequest{
			Model:                "models/" + model,
			Content:              geminiEmbeddingContent{Parts: []geminiEmbeddingPart{{Text: input}}},
			OutputDimensionality: request.Dimensions,
		})
	}
	payload, err := json.Marshal(struct {
		Requests []geminiEmbedRequest `json:"requests"`
	}{Requests: requests})
	return model, "batchEmbedContents", payload, err
}

type geminiEmbeddingValues struct {
	Values []float64 `json:"values"`
}

type geminiEmbeddingUsageMetadata struct {
	PromptTokenCount int `json:"promptTokenCount"`
	TotalTokenCount  int `json:"totalTokenCount"`
}

// translateGeminiEmbeddingResponse converts either the single or batch native
// response into the OpenAI embeddings list shape. Gemini does not always
// return usageMetadata, so the compatibility response carries an empty usage
// object in that case and billing remains zero rather than being invented.
func translateGeminiEmbeddingResponse(body []byte, model, encodingFormat string) ([]byte, OpenAIUsage, error) {
	var response struct {
		Embedding     *geminiEmbeddingValues        `json:"embedding"`
		Embeddings    []geminiEmbeddingValues       `json:"embeddings"`
		UsageMetadata *geminiEmbeddingUsageMetadata `json:"usageMetadata"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, OpenAIUsage{}, fmt.Errorf("parse Gemini embedding response: %w", err)
	}
	embeddings := response.Embeddings
	if response.Embedding != nil {
		embeddings = []geminiEmbeddingValues{*response.Embedding}
	}
	if len(embeddings) == 0 {
		return nil, OpenAIUsage{}, errors.New("Gemini response contains no embeddings")
	}

	data := make([]map[string]any, 0, len(embeddings))
	for index, embedding := range embeddings {
		value, err := formatGeminiEmbeddingValues(embedding.Values, encodingFormat)
		if err != nil {
			return nil, OpenAIUsage{}, err
		}
		data = append(data, map[string]any{
			"object":    "embedding",
			"index":     index,
			"embedding": value,
		})
	}
	usage := map[string]int{}
	resultUsage := OpenAIUsage{}
	if response.UsageMetadata != nil {
		if response.UsageMetadata.PromptTokenCount > 0 {
			usage["prompt_tokens"] = response.UsageMetadata.PromptTokenCount
			resultUsage.InputTokens = response.UsageMetadata.PromptTokenCount
		}
		if response.UsageMetadata.TotalTokenCount > 0 {
			usage["total_tokens"] = response.UsageMetadata.TotalTokenCount
		}
	}
	payload, err := json.Marshal(map[string]any{
		"object": "list",
		"data":   data,
		"model":  strings.TrimSpace(model),
		"usage":  usage,
	})
	return payload, resultUsage, err
}

func formatGeminiEmbeddingValues(values []float64, encodingFormat string) (any, error) {
	if strings.TrimSpace(encodingFormat) == "" || encodingFormat == "float" {
		return values, nil
	}
	if encodingFormat != "base64" {
		return nil, fmt.Errorf("unsupported encoding_format: %s", encodingFormat)
	}
	encoded := make([]byte, len(values)*4)
	for index, value := range values {
		binary.LittleEndian.PutUint32(encoded[index*4:], math.Float32bits(float32(value)))
	}
	return base64.StdEncoding.EncodeToString(encoded), nil
}

func (s *OpenAIGatewayService) forwardGeminiEmbeddings(ctx context.Context, c *gin.Context, account *Account, body []byte, defaultMappedModel string) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	originalModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	if upstreamModel == "" {
		upstreamModel = billingModel
	}
	forwardBody := body
	if upstreamModel != originalModel {
		forwardBody = ReplaceModelInBody(body, upstreamModel)
	}
	model, action, payload, err := translateOpenAIEmbeddingsToGemini(forwardBody)
	if err != nil {
		writeOpenAIEmbeddingsError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}
	format := strings.TrimSpace(gjson.GetBytes(body, "encoding_format").String())
	baseURL := account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	targetURL, err := buildGeminiAIStudioModelActionURL(normalizedBaseURL, model, action, false)
	if err != nil {
		return nil, err
	}
	SetOpsUpstreamModel(c, upstreamModel)
	SetActualOpenAIUpstreamEndpoint(c, "/v1beta/models/"+model+":"+action)
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	request, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(payload))
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	request = request.WithContext(WithHTTPUpstreamProfile(request.Context(), HTTPUpstreamProfileOpenAI))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("x-goog-api-key", strings.TrimSpace(account.GetCredential("api_key")))
	if request.Header.Get("x-goog-api-key") == "" {
		return nil, errors.New("gemini api_key not configured")
	}
	for key, values := range c.Request.Header {
		if openaiCCRawAllowedHeaders[strings.ToLower(key)] {
			for _, value := range values {
				request.Header.Add(key, value)
			}
		}
	}
	account.ApplyHeaderOverrides(request.Header)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.doOpenAIUpstream(request, proxyURL, account)
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		writeOpenAIEmbeddingsError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		if s.shouldFailoverOpenAIUpstreamResponse(account, resp.StatusCode, upstreamMsg, respBody) {
			shouldDisable := s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody, upstreamModel)
			retryableOnSameAccount := !shouldDisable && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode)
			return nil, s.newOpenAIAccountFailoverError(account, resp.StatusCode, resp.Header, respBody, upstreamMsg, shouldDisable, retryableOnSameAccount)
		}
		writeOpenAIEmbeddingsUpstreamResponse(c, resp, respBody, s.responseHeaderFilter)
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		writeOpenAIEmbeddingsError(c, http.StatusBadGateway, "api_error", "Failed to read upstream response")
		return nil, fmt.Errorf("read upstream body: %w", err)
	}
	openAIResponse, usage, err := translateGeminiEmbeddingResponse(respBody, originalModel, format)
	if err != nil {
		writeOpenAIEmbeddingsError(c, http.StatusBadGateway, "api_error", "Invalid Gemini embedding response")
		return nil, err
	}
	writeOpenAIEmbeddingsUpstreamResponse(c, resp, openAIResponse, s.responseHeaderFilter)
	return &OpenAIForwardResult{
		RequestID:       firstNonEmptyString(resp.Header.Get("x-request-id"), resp.Header.Get("request-id")),
		UpstreamHeaders: resp.Header,
		Usage:           usage,
		Model:           originalModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
		Stream:          false,
		Duration:        time.Since(startTime),
	}, nil
}

func writeOpenAIEmbeddingsUpstreamResponse(c *gin.Context, resp *http.Response, body []byte, filter *responseheaders.CompiledHeaderFilter) {
	if c == nil || resp == nil {
		return
	}
	if c.Writer.Written() {
		return
	}
	if resp.Header != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, filter)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Writer.Header().Set("Content-Type", ct)
	} else {
		c.Writer.Header().Set("Content-Type", "application/json")
	}
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(body)
}

func writeOpenAIEmbeddingsError(c *gin.Context, statusCode int, errType, message string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

func extractOpenAIEmbeddingsUsage(body []byte) OpenAIUsage {
	usage := gjson.GetBytes(body, "usage")
	if !usage.Exists() || !usage.IsObject() {
		return OpenAIUsage{}
	}
	inputTokens := firstPositiveGJSONInt(
		usage.Get("prompt_tokens"),
		usage.Get("input_tokens"),
		usage.Get("total_tokens"),
	)
	outputTokens := firstPositiveGJSONInt(
		usage.Get("completion_tokens"),
		usage.Get("output_tokens"),
	)
	cacheReadTokens := openAICacheReadTokensFromUsage(usage)
	cacheCreationTokens := openAICacheCreationTokensFromUsage(usage)
	// 多模态 embedding（如 doubao-embedding-vision）回传图文 token 拆分，
	// 用于图文不同价计费；纯文本 embedding 该字段为 0，行为不变。
	imageInputTokens := firstPositiveGJSONInt(
		usage.Get("prompt_tokens_details.image_tokens"),
		usage.Get("input_tokens_details.image_tokens"),
	)
	return OpenAIUsage{
		InputTokens:              inputTokens,
		ImageInputTokens:         imageInputTokens,
		OutputTokens:             outputTokens,
		CacheReadInputTokens:     cacheReadTokens,
		CacheCreationInputTokens: cacheCreationTokens,
	}
}

func firstPositiveGJSONInt(values ...gjson.Result) int {
	for _, value := range values {
		if !value.Exists() {
			continue
		}
		n := int(value.Int())
		if n > 0 {
			return n
		}
	}
	return 0
}

func buildOpenAIEmbeddingsURL(base string) string {
	return buildOpenAIEndpointURL(base, "/v1/embeddings")
}

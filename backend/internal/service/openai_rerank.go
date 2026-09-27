package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

// ForwardRerank 将 OpenAI 兼容的 /v1/rerank 请求（如硅基流动的 reranker 模型）
// 原样透传到账号上游。除 model 映射改写外请求体不做任何转换，
// 上游状态码与响应体原样回写，与 ForwardEmbeddings 保持一致。
func (s *OpenAIGatewayService) ForwardRerank(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	originalModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if originalModel == "" {
		writeOpenAIRerankError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in request")
	}

	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	SetOpsUpstreamModel(c, upstreamModel)
	upstreamBody := body
	if upstreamModel != originalModel {
		upstreamBody = ReplaceModelInBody(body, upstreamModel)
	}
	// 阿里百炼嵌套端点模型（如 qwen3.7-text-rerank）：改走百炼 text-rerank
	// 专用路径，并把扁平 Cohere 风格请求体转换为百炼嵌套格式；其余模型行为不变。
	useBailianRerank := isBailianTextRerankModel(upstreamModel)
	if useBailianRerank {
		upstreamBody = buildBailianTextRerankUpstreamBody(upstreamBody)
	}

	logger.L().Debug("openai rerank: forwarding",
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
	// rerank 需使用 OpenAI 格式 base。
	baseURL := account.GetOpenAIFormatBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	targetURL := buildOpenAIRerankURL(validatedURL)
	if useBailianRerank {
		targetURL = buildBailianTextRerankURL(validatedURL)
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
		writeOpenAIRerankError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
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
		writeOpenAIRerankUpstreamResponse(c, resp, respBody, s.responseHeaderFilter)
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			writeOpenAIRerankError(c, http.StatusBadGateway, "api_error", "Failed to read upstream response")
		}
		return nil, fmt.Errorf("read upstream body: %w", err)
	}

	if useBailianRerank {
		// 百炼嵌套响应先转扁平再回写：usage 已提到顶层，下方记账提取链直接兼容。
		respBody = convertBailianTextRerankResponse(respBody, upstreamModel)
	}

	writeOpenAIRerankUpstreamResponse(c, resp, respBody, s.responseHeaderFilter)

	return &OpenAIForwardResult{
		RequestID:       firstNonEmptyString(resp.Header.Get("x-request-id"), resp.Header.Get("request-id")),
		UpstreamHeaders: resp.Header,
		Usage:           extractOpenAIRerankUsage(respBody),
		Model:           originalModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
		Stream:          false,
		Duration:        time.Since(startTime),
	}, nil
}

func writeOpenAIRerankUpstreamResponse(c *gin.Context, resp *http.Response, body []byte, filter *responseheaders.CompiledHeaderFilter) {
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

func writeOpenAIRerankError(c *gin.Context, statusCode int, errType, message string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// extractOpenAIRerankUsage 解析 rerank 上游的 usage。多数 OpenAI 兼容 reranker
// （如硅基流动）只回传 total_tokens，部分实现回传 prompt_tokens/completion_tokens，
// 这里按 embeddings 同款取值链兜底。
//
// Jina/Cohere 风格的实现（硅基流动 rerank 即如此）没有顶层 usage，用量放在
// meta.tokens 中，因此顶层读不到时回退到该路径，否则记账 input_tokens=0。
// meta.billed_units 与 meta.tokens 数值相同，无需再读。
func extractOpenAIRerankUsage(body []byte) OpenAIUsage {
	usage := gjson.GetBytes(body, "usage")
	if !usage.Exists() || !usage.IsObject() {
		usage = gjson.GetBytes(body, "meta.tokens")
	}
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
	return OpenAIUsage{
		InputTokens:              inputTokens,
		OutputTokens:             outputTokens,
		CacheReadInputTokens:     openAICacheReadTokensFromUsage(usage),
		CacheCreationInputTokens: openAICacheCreationTokensFromUsage(usage),
	}
}

func buildOpenAIRerankURL(base string) string {
	return buildOpenAIEndpointURL(base, "/v1/rerank")
}

// bailianTextRerankModels 走阿里百炼嵌套 text-rerank 端点适配的模型集合。
// 键为最终发往上游的模型名（渠道映射与 normalizeOpenAIModelForUpstream 改写后
// 请求体中的 model 值）；后续 gte-rerank-v2 等百炼重排模型加入此处即可复用适配层。
var bailianTextRerankModels = map[string]struct{}{
	"qwen3.7-text-rerank": {},
}

// isBailianTextRerankModel 判断最终发往上游的模型名是否命中百炼嵌套端点适配集合。
func isBailianTextRerankModel(model string) bool {
	_, ok := bailianTextRerankModels[strings.ToLower(strings.TrimSpace(model))]
	return ok
}

// bailianTextRerankURLPath 百炼 text-rerank 端点的固定路径后缀。
const bailianTextRerankURLPath = "/api/v1/services/rerank/text-rerank/text-rerank"

// buildBailianTextRerankURL 构造百炼 text-rerank 端点 URL。base 为百炼域名根
// （如 https://dashscope.aliyuncs.com 或
// https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com）；base 已带完整路径后缀时
// 原样返回（幂等）。
func buildBailianTextRerankURL(base string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(trimmed, bailianTextRerankURLPath) {
		return trimmed
	}
	return trimmed + bailianTextRerankURLPath
}

// buildBailianTextRerankUpstreamBody 将扁平 Cohere 风格 rerank 请求体转换为百炼
// 嵌套格式：query/documents 移入 input，top_n/instruct（存在时）移入 parameters，
// return_documents 及其他百炼不支持的扁平参数丢弃，顶层仅保留 model。
// 请求体已含顶层 input 对象（客户端已按百炼格式发送）时原样返回。
func buildBailianTextRerankUpstreamBody(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	if gjson.GetBytes(body, "input").IsObject() {
		return body
	}
	nested := []byte(`{}`)
	if model := gjson.GetBytes(body, "model"); model.Exists() {
		nested, _ = sjson.SetRawBytes(nested, "model", []byte(model.Raw))
	}
	if query := gjson.GetBytes(body, "query"); query.Exists() {
		nested, _ = sjson.SetRawBytes(nested, "input.query", []byte(query.Raw))
	}
	if documents := gjson.GetBytes(body, "documents"); documents.Exists() {
		nested, _ = sjson.SetRawBytes(nested, "input.documents", []byte(documents.Raw))
	}
	if topN := gjson.GetBytes(body, "top_n"); topN.Exists() {
		nested, _ = sjson.SetRawBytes(nested, "parameters.top_n", []byte(topN.Raw))
	}
	if instruct := gjson.GetBytes(body, "instruct"); instruct.Exists() {
		nested, _ = sjson.SetRawBytes(nested, "parameters.instruct", []byte(instruct.Raw))
	}
	return nested
}

// convertBailianTextRerankResponse 将百炼嵌套成功响应转换为扁平 Cohere 风格：
// results 从 output.results 提到顶层，id 取 request_id，model 回填本次上游模型名，
// usage 与 request_id 保持顶层原样。响应体非 JSON 或无顶层 output 对象时原样返回。
// 必须在 extractOpenAIRerankUsage 之前调用，使百炼 usage.prompt_tokens 正常入账。
func convertBailianTextRerankResponse(body []byte, upstreamModel string) []byte {
	if len(body) == 0 {
		return body
	}
	output := gjson.GetBytes(body, "output")
	if !output.IsObject() {
		return body
	}
	flat := []byte(`{}`)
	if requestID := gjson.GetBytes(body, "request_id"); requestID.Exists() {
		flat, _ = sjson.SetRawBytes(flat, "id", []byte(requestID.Raw))
		flat, _ = sjson.SetRawBytes(flat, "request_id", []byte(requestID.Raw))
	}
	if upstreamModel != "" {
		flat, _ = sjson.SetBytes(flat, "model", upstreamModel)
	}
	if results := output.Get("results"); results.Exists() {
		flat, _ = sjson.SetRawBytes(flat, "results", []byte(results.Raw))
	}
	if usage := gjson.GetBytes(body, "usage"); usage.Exists() {
		flat, _ = sjson.SetRawBytes(flat, "usage", []byte(usage.Raw))
	}
	return flat
}

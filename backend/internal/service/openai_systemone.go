package service

import (
	"bytes"
	"context"
	"encoding/json"
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
	"go.uber.org/zap"
)

// ForwardSystemOne 将 TypeSafe AI 的 Jev 判断题请求原样透传到账号上游
// {base_url}/v1/systemone。请求体除 model 映射改写外不做任何转换
// （state/questions 完全不解析、不改写），无流式、不生成文本。
//
// 安全约束（硬约束）：上游错误体绝不回写调用方。TypeSafe 的错误响应可能回显
// 请求内容甚至凭据（见 internal/pkg/typesafe/client.go 的注释），而出站
// Authorization 用的是平台账号的 key；因此凡 resp.StatusCode >= 400 一律返回
// 自造错误体 {"error":{"type":...,"message":...}}，message 只带上游状态码与上游
// request id 供排障。成功响应体照常原样回写。
func (s *OpenAIGatewayService) ForwardSystemOne(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	originalModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if originalModel == "" {
		writeOpenAISystemOneError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in request")
	}

	// 渠道级映射已由 handler 写入 body；这里只叠加账号级映射（与 embeddings/rerank 同链路）。
	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	SetOpsUpstreamModel(c, upstreamModel)
	upstreamBody := body
	if upstreamModel != originalModel {
		upstreamBody = ReplaceModelInBody(body, upstreamModel)
	}

	logger.L().Debug("openai systemone: forwarding",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("billing_model", billingModel),
		zap.String("upstream_model", upstreamModel),
	)

	apiKey := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	// typesafe 只提供 base_url + api_key 一种凭据形态：GetOpenAIBaseURL 返回
	// credentials["base_url"]，缺失时回落 DefaultTypeSafeBaseURL（api.typesafe.ai）。
	// 这里不保留 "https://api.openai.com" 兜底——那会把 typesafe 的 key 发到 OpenAI。
	baseURL := strings.TrimSpace(account.GetOpenAIBaseURL())
	if baseURL == "" {
		return nil, fmt.Errorf("account %d missing base_url", account.ID)
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	targetURL := buildOpenAISystemOneURL(validatedURL)

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
		writeOpenAISystemOneError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		// 上游 body 只用于内部判定（failover 分类 / 账号熔断 / ops 事件），
		// 绝不进入客户端响应或 failover 错误的 ResponseBody。
		respBody := s.readUpstreamErrorBody(resp)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		upstreamRequestID := firstNonEmptyString(resp.Header.Get("x-request-id"), resp.Header.Get("request-id"))
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
				UpstreamRequestID:  upstreamRequestID,
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
			})
			shouldDisable := s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody, upstreamModel)
			retryableOnSameAccount := !shouldDisable && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode)
			failoverErr := &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseHeaders:        resp.Header.Clone(),
				RetryableOnSameAccount: retryableOnSameAccount,
			}
			if isOpenAIHTTPUpstreamAccessStateError(resp.StatusCode, upstreamMsg, respBody) {
				failoverErr = newOpenAIUpstreamFailoverError(resp.StatusCode, resp.Header, respBody, upstreamMsg, retryableOnSameAccount)
			}
			// 覆盖为自造错误体：上游 body 可能回显请求内容/凭据，任何下游
			// 写出路径（handleFailoverExhausted 的透传规则）都不得看到它。
			failoverErr.ResponseBody = buildOpenAISystemOneUpstreamErrorBody(resp.StatusCode, upstreamRequestID)
			return nil, failoverErr
		}
		writeOpenAISystemOneUpstreamError(c, resp.StatusCode, upstreamRequestID)
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			writeOpenAISystemOneError(c, http.StatusBadGateway, "api_error", "Failed to read upstream response")
		}
		return nil, fmt.Errorf("read upstream body: %w", err)
	}

	writeOpenAISystemOneUpstreamResponse(c, resp, respBody, s.responseHeaderFilter)

	return &OpenAIForwardResult{
		RequestID:       firstNonEmptyString(resp.Header.Get("x-request-id"), resp.Header.Get("request-id")),
		UpstreamHeaders: resp.Header,
		Usage:           extractOpenAISystemOneUsage(respBody),
		Model:           originalModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
		Stream:          false,
		Duration:        time.Since(startTime),
	}, nil
}

// writeOpenAISystemOneUpstreamResponse 只在成功分支使用：上游 2xx 响应体与（过滤后的）
// 响应头原样回写。
func writeOpenAISystemOneUpstreamResponse(c *gin.Context, resp *http.Response, body []byte, filter *responseheaders.CompiledHeaderFilter) {
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

// writeOpenAISystemOneUpstreamError 写自造的上游错误响应：保留上游状态码，
// 但响应体只带状态码与上游 request id，绝不包含上游 body。
func writeOpenAISystemOneUpstreamError(c *gin.Context, upstreamStatus int, upstreamRequestID string) {
	writeOpenAISystemOneError(c, upstreamStatus, "upstream_error", openAISystemOneUpstreamErrorMessage(upstreamStatus, upstreamRequestID))
}

func writeOpenAISystemOneError(c *gin.Context, statusCode int, errType, message string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

func openAISystemOneUpstreamErrorMessage(upstreamStatus int, upstreamRequestID string) string {
	message := fmt.Sprintf("Upstream returned status %d", upstreamStatus)
	if requestID := strings.TrimSpace(upstreamRequestID); requestID != "" {
		message += " (upstream request id: " + requestID + ")"
	}
	return message
}

// buildOpenAISystemOneUpstreamErrorBody 生成 failover 错误里携带的自造响应体。
func buildOpenAISystemOneUpstreamErrorBody(upstreamStatus int, upstreamRequestID string) []byte {
	body, err := json.Marshal(gin.H{
		"error": gin.H{
			"type":    "upstream_error",
			"message": openAISystemOneUpstreamErrorMessage(upstreamStatus, upstreamRequestID),
		},
	})
	if err != nil {
		return nil
	}
	return body
}

// extractOpenAISystemOneUsage 解析 Jev 判断题上游的 usage。
// TypeSafe 返回 {"usage":{"input_tokens":N,"output_tokens":M}}，命中取值链第二档；
// 前面保留 OpenAI 风格的 prompt_tokens/completion_tokens，后面用 total_tokens 兜底，
// 与 embeddings/rerank 的取值链一致。
func extractOpenAISystemOneUsage(body []byte) OpenAIUsage {
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
	return OpenAIUsage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}
}

func buildOpenAISystemOneURL(base string) string {
	return buildOpenAIEndpointURL(base, "/v1/systemone")
}

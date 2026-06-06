package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
)

// ForwardXAIResponses forwards an OpenAI Responses-shaped request to xAI's
// Responses API. It intentionally bypasses OpenAI Codex/ChatGPT transforms.
func (s *OpenAIGatewayService) ForwardXAIResponses(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	reqBody, err := getOpenAIRequestBodyMap(c, body)
	if err != nil {
		return nil, err
	}

	reqModel, reqStream, promptCacheKey := extractOpenAIRequestMetaFromBody(body)
	if model, ok := reqBody["model"].(string); ok && strings.TrimSpace(model) != "" {
		reqModel = strings.TrimSpace(model)
	}
	if stream, ok := reqBody["stream"].(bool); ok {
		reqStream = stream
	}
	if promptCacheKey == "" {
		if value, ok := reqBody["prompt_cache_key"].(string); ok {
			promptCacheKey = strings.TrimSpace(value)
		}
	}

	originalModel := reqModel
	upstreamModel := resolveOpenAIForwardModel(account, reqModel, "")
	if upstreamModel == "" {
		upstreamModel = reqModel
	}

	xai.NormalizeResponsesBodyMap(reqBody, upstreamModel, reqStream, promptCacheKey)
	normalizedBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("serialize xai request body: %w", err)
	}

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, reqStream)
	upstreamReq, err := s.buildXAIUpstreamRequest(upstreamCtx, c, account, normalizedBody, token, reqStream, promptCacheKey)
	releaseUpstreamCtx()
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
		})
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"type":    "upstream_error",
				"message": "Upstream request failed",
			},
		})
		return nil, fmt.Errorf("xai upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			upstreamDetail := ""
			if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
				maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
				if maxBytes <= 0 {
					maxBytes = 2048
				}
				upstreamDetail = truncateString(string(respBody), maxBytes)
			}
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
			})
			if s.rateLimitService != nil {
				s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
			}
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && (isPoolModeRetryableStatus(resp.StatusCode) || isOpenAITransientProcessingError(resp.StatusCode, upstreamMsg, respBody)),
			}
		}
		return s.handleErrorResponse(ctx, resp, c, account, normalizedBody)
	}

	var usage *OpenAIUsage
	var firstTokenMs *int
	if reqStream {
		streamResult, err := s.handleStreamingResponse(ctx, resp, c, account, startTime, originalModel, upstreamModel)
		if err != nil {
			return nil, err
		}
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
	} else {
		nonStreamResult, err := s.handleNonStreamingResponse(ctx, resp, c, account, originalModel, upstreamModel)
		if err != nil {
			return nil, err
		}
		usage = nonStreamResult.usage
	}
	if usage == nil {
		usage = &OpenAIUsage{}
	}

	return &OpenAIForwardResult{
		RequestID:       resp.Header.Get("x-request-id"),
		Usage:           *usage,
		Model:           originalModel,
		UpstreamModel:   upstreamModel,
		ServiceTier:     extractOpenAIServiceTier(reqBody),
		ReasoningEffort: extractOpenAIReasoningEffort(reqBody, originalModel),
		Stream:          reqStream,
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
	}, nil
}

func (s *OpenAIGatewayService) buildXAIUpstreamRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
	isStream bool,
	promptCacheKey string,
) (*http.Request, error) {
	baseURL := account.GetXAIBaseURL()
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, xai.BuildResponsesURL(validatedURL), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if isStream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("Connection", "Keep-Alive")
	if strings.TrimSpace(promptCacheKey) != "" {
		req.Header.Set("x-grok-conv-id", strings.TrimSpace(promptCacheKey))
	}
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lowerKey := strings.ToLower(key)
			if lowerKey != "accept-language" && lowerKey != "user-agent" {
				continue
			}
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	}
	return req, nil
}

func (s *OpenAIGatewayService) forwardXAIResponsesAsChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	responsesReq *apicompat.ResponsesRequest,
	originalModel string,
	billingModel string,
	upstreamModel string,
	clientStream bool,
	includeUsage bool,
	startTime time.Time,
	promptCacheKey string,
) (*OpenAIForwardResult, error) {
	responsesBody, err := json.Marshal(responsesReq)
	if err != nil {
		return nil, fmt.Errorf("marshal xai chat responses request: %w", err)
	}
	normalizedBody, err := xai.NormalizeResponsesBody(responsesBody, upstreamModel, true, promptCacheKey)
	if err != nil {
		return nil, fmt.Errorf("normalize xai chat responses request: %w", err)
	}
	result, err := s.forwardXAICompatStreaming(ctx, c, account, normalizedBody, originalModel, billingModel, upstreamModel, clientStream, includeUsage, startTime, promptCacheKey, true)
	if err == nil && result != nil {
		if responsesReq.ServiceTier != "" {
			st := responsesReq.ServiceTier
			result.ServiceTier = &st
		}
		if responsesReq.Reasoning != nil && responsesReq.Reasoning.Effort != "" {
			re := responsesReq.Reasoning.Effort
			result.ReasoningEffort = &re
		}
	}
	return result, err
}

func (s *OpenAIGatewayService) forwardXAIResponsesAsAnthropic(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	responsesReq *apicompat.ResponsesRequest,
	originalModel string,
	billingModel string,
	upstreamModel string,
	clientStream bool,
	startTime time.Time,
	promptCacheKey string,
) (*OpenAIForwardResult, error) {
	responsesBody, err := json.Marshal(responsesReq)
	if err != nil {
		return nil, fmt.Errorf("marshal xai messages responses request: %w", err)
	}
	normalizedBody, err := xai.NormalizeResponsesBody(responsesBody, upstreamModel, true, promptCacheKey)
	if err != nil {
		return nil, fmt.Errorf("normalize xai messages responses request: %w", err)
	}
	result, err := s.forwardXAICompatStreaming(ctx, c, account, normalizedBody, originalModel, billingModel, upstreamModel, clientStream, false, startTime, promptCacheKey, false)
	if err == nil && result != nil {
		if responsesReq.ServiceTier != "" {
			st := responsesReq.ServiceTier
			result.ServiceTier = &st
		}
		if responsesReq.Reasoning != nil && responsesReq.Reasoning.Effort != "" {
			re := responsesReq.Reasoning.Effort
			result.ReasoningEffort = &re
		}
	}
	return result, err
}

func (s *OpenAIGatewayService) forwardXAICompatStreaming(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	originalModel string,
	billingModel string,
	upstreamModel string,
	clientStream bool,
	includeUsage bool,
	startTime time.Time,
	promptCacheKey string,
	chatCompletions bool,
) (*OpenAIForwardResult, error) {
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("get xai access token: %w", err)
	}
	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, true)
	upstreamReq, err := s.buildXAIUpstreamRequest(upstreamCtx, c, account, body, token, true, promptCacheKey)
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build xai upstream request: %w", err)
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
		})
		if chatCompletions {
			writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		} else {
			writeAnthropicError(c, http.StatusBadGateway, "api_error", "Upstream request failed")
		}
		return nil, fmt.Errorf("xai upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			if s.rateLimitService != nil {
				s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
			}
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && (isPoolModeRetryableStatus(resp.StatusCode) || isOpenAITransientProcessingError(resp.StatusCode, upstreamMsg, respBody)),
			}
		}
		if chatCompletions {
			return s.handleChatCompletionsErrorResponse(resp, c, account)
		}
		return s.handleAnthropicErrorResponse(resp, c, account)
	}

	if chatCompletions {
		if clientStream {
			return s.handleChatStreamingResponse(resp, c, account, originalModel, billingModel, upstreamModel, startTime, len(body))
		}
		return s.handleChatBufferedStreamingResponse(resp, c, account, originalModel, billingModel, upstreamModel, startTime)
	}
	if clientStream {
		return s.handleAnthropicStreamingResponse(resp, c, account, originalModel, billingModel, upstreamModel, startTime)
	}
	return s.handleAnthropicBufferedStreamingResponse(resp, c, originalModel, billingModel, upstreamModel, startTime)
}

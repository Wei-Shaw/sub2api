package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// ForwardClaudeChatCompletionsRaw 直转客户端的 Chat Completions 请求到一个 OpenAI 兼容的
// /v1/chat/completions 上游（账号类型 = apikey-chat-completions），不做任何
// 协议转换。客户端的请求体本就是 CC 格式，上游也是 CC 端点，所以是纯透传。
//
// 入口：handler.GatewayHandler.ChatCompletions（Anthropic 平台分组下的 /v1/chat/completions）
// 在该 handler 中按 account.IsOpenAIChatCompletionsUpstream() 分流到此函数。
//
// 与 OpenAIGatewayService.forwardAsRawChatCompletions 高度相似，但归属于 GatewayService，
// 不依赖 OpenAI 专属的 fast policy / OAuth transform / WebSocket adapter。
//
// TODO: dedupe with openai_gateway_chat_completions_raw.go::forwardAsRawChatCompletions
// （计划在抽出公共 raw-CC 透传 helper 后合并）。
func (s *GatewayService) ForwardClaudeChatCompletionsRaw(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *ParsedRequest,
) (*ForwardResult, error) {
	startTime := time.Now()

	// 1. Parse minimal fields needed for routing/billing.
	originalModel := gjson.GetBytes(body, "model").String()
	if originalModel == "" {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in request")
	}
	clientStream := gjson.GetBytes(body, "stream").Bool()

	// 2. Model mapping.
	mappedModel := account.GetMappedModel(originalModel)
	upstreamBody := body
	if mappedModel != originalModel {
		upstreamBody = ReplaceModelInBody(body, mappedModel)
	}

	// 3. Stream usage enforcement so billing is recoverable from the closing chunk.
	if clientStream {
		var usageErr error
		upstreamBody, usageErr = ensureOpenAIChatStreamUsage(upstreamBody)
		if usageErr != nil {
			writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "Failed to prepare stream options")
			return nil, fmt.Errorf("enable stream usage: %w", usageErr)
		}
	}

	logger.L().Debug("gateway claude chat_completions raw: forwarding without protocol conversion",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("upstream_model", mappedModel),
		zap.Bool("stream", clientStream),
	)

	// 4. Resolve upstream URL + API key.
	apiKey := account.GetUpstreamAPIKey()
	if apiKey == "" {
		writeChatCompletionsError(c, http.StatusBadGateway, "api_error", fmt.Sprintf("account %d missing api_key", account.ID))
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	targetURL := account.GetOpenAIChatCompletionsURL()
	if targetURL == "" {
		writeChatCompletionsError(c, http.StatusBadGateway, "api_error", fmt.Sprintf("account %d missing chat_completions_url", account.ID))
		return nil, fmt.Errorf("account %d missing chat_completions_url", account.ID)
	}
	if _, err := s.validateUpstreamBaseURL(targetURL); err != nil {
		writeChatCompletionsError(c, http.StatusBadGateway, "api_error", "Invalid upstream URL")
		return nil, fmt.Errorf("invalid chat_completions_url: %w", err)
	}

	// 5. Build upstream request.
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(upstreamBody))
	releaseUpstreamCtx()
	if err != nil {
		writeChatCompletionsError(c, http.StatusBadGateway, "api_error", "Failed to build upstream request")
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	applyOpenAICompatibleAPIKeyAuth(upstreamReq, account, apiKey)
	if clientStream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			if openaiCCRawAllowedHeaders[strings.ToLower(key)] {
				for _, v := range values {
					upstreamReq.Header.Add(key, v)
				}
			}
		}
	}

	// 6. Send.
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
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
		writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	// 7. Error handling.
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if upstreamMsg == "" {
			upstreamMsg = http.StatusText(resp.StatusCode)
		}

		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			Kind:               "upstream_error",
			Message:            upstreamMsg,
		})
		if s.rateLimitService != nil {
			s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		}
		// 可 failover 的状态码：交给 handler 失败循环决定是否切换账号；不在此处写客户端响应。
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				ResponseHeaders:        resp.Header,
				RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		writeChatCompletionsError(c, mapUpstreamStatusCode(resp.StatusCode), "upstream_error", upstreamMsg)
		return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
	}

	// 上游已接受请求（2xx），提前释放用户串行锁——不等流完成。
	// 当前 CC handler 尚未注入 OnUpstreamAccepted（无队列串行化），此处保持向前兼容。
	if parsed != nil && parsed.OnUpstreamAccepted != nil {
		parsed.OnUpstreamAccepted()
	}

	// 8. Forward response.
	if clientStream {
		return s.streamClaudeRawChatCompletions(c, resp, originalModel, mappedModel, startTime)
	}
	return s.bufferClaudeRawChatCompletions(c, resp, originalModel, mappedModel, startTime)
}

func (s *GatewayService) streamClaudeRawChatCompletions(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	upstreamModel string,
	startTime time.Time,
) (*ForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	var usage ClaudeUsage
	var firstTokenMs *int
	clientDisconnected := false

	for scanner.Scan() {
		line := scanner.Text()
		if payload, ok := extractOpenAISSEDataLine(line); ok {
			trimmedPayload := strings.TrimSpace(payload)
			if trimmedPayload != "[DONE]" {
				usageOnlyChunk := isOpenAIChatUsageOnlyStreamChunk(payload)
				if u := extractCCStreamUsage(payload); u != nil {
					usage.InputTokens = u.InputTokens
					usage.OutputTokens = u.OutputTokens
					usage.CacheReadInputTokens = u.CacheReadInputTokens
				}
				if firstTokenMs == nil && !usageOnlyChunk {
					elapsed := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &elapsed
				}
			}
		}

		if !clientDisconnected {
			if _, werr := c.Writer.WriteString(line + "\n"); werr != nil {
				clientDisconnected = true
				logger.L().Debug("gateway claude chat_completions raw: client disconnected, draining for billing",
					zap.Error(werr),
					zap.String("request_id", requestID),
				)
			}
		}
		if !clientDisconnected {
			c.Writer.Flush()
		}
	}
	if err := scanner.Err(); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("gateway claude chat_completions raw: stream read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
	}

	return &ForwardResult{
		RequestID:        requestID,
		Usage:            usage,
		Model:            originalModel,
		UpstreamModel:    upstreamModel,
		Stream:           true,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnected,
	}, nil
}

func (s *GatewayService) bufferClaudeRawChatCompletions(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	upstreamModel string,
	startTime time.Time,
) (*ForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			writeChatCompletionsError(c, http.StatusBadGateway, "api_error", "Failed to read upstream response")
		}
		return nil, fmt.Errorf("read upstream body: %w", err)
	}

	var ccResp apicompat.ChatCompletionsResponse
	var usage ClaudeUsage
	if err := json.Unmarshal(respBody, &ccResp); err == nil && ccResp.Usage != nil {
		usage.InputTokens = ccResp.Usage.PromptTokens
		usage.OutputTokens = ccResp.Usage.CompletionTokens
		if ccResp.Usage.PromptTokensDetails != nil {
			usage.CacheReadInputTokens = ccResp.Usage.PromptTokensDetails.CachedTokens
		}
	}

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Writer.Header().Set("Content-Type", ct)
	} else {
		c.Writer.Header().Set("Content-Type", "application/json")
	}
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(respBody)

	return &ForwardResult{
		RequestID:     requestID,
		Usage:         usage,
		Model:         originalModel,
		UpstreamModel: upstreamModel,
		Stream:        false,
		Duration:      time.Since(startTime),
	}, nil
}

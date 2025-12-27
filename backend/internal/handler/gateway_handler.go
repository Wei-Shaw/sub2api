package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// GatewayHandler handles API gateway requests
type GatewayHandler struct {
	gatewayService      *service.GatewayService
	geminiCompatService *service.GeminiMessagesCompatService
	userService         *service.UserService
	billingCacheService *service.BillingCacheService
	concurrencyHelper   *ConcurrencyHelper
}

// NewGatewayHandler creates a new GatewayHandler
func NewGatewayHandler(
	gatewayService *service.GatewayService,
	geminiCompatService *service.GeminiMessagesCompatService,
	userService *service.UserService,
	concurrencyService *service.ConcurrencyService,
	billingCacheService *service.BillingCacheService,
) *GatewayHandler {
	return &GatewayHandler{
		gatewayService:      gatewayService,
		geminiCompatService: geminiCompatService,
		userService:         userService,
		billingCacheService: billingCacheService,
		concurrencyHelper:   NewConcurrencyHelper(concurrencyService, SSEPingFormatClaude),
	}
}

// Messages handles Claude API compatible messages endpoint
// POST /v1/messages
func (h *GatewayHandler) Messages(c *gin.Context) {
	// 从context获取apiKey和user（ApiKeyAuth中间件已设置）
	apiKey, ok := middleware2.GetApiKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}

	// 读取请求体
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}

	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	// 解析请求获取模型名和stream
	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	// Track if we've started streaming (for error handling)
	streamStarted := false

	// 获取订阅信息（可能为nil）- 提前获取用于后续检查
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	// 0. 检查wait队列是否已满
	maxWait := service.CalculateMaxWait(subject.Concurrency)
	canWait, err := h.concurrencyHelper.IncrementWaitCount(c.Request.Context(), subject.UserID, maxWait)
	if err != nil {
		log.Printf("Increment wait count failed: %v", err)
		// On error, allow request to proceed
	} else if !canWait {
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later")
		return
	}
	// 确保在函数退出时减少wait计数
	defer h.concurrencyHelper.DecrementWaitCount(c.Request.Context(), subject.UserID)

	// 1. 首先获取用户并发槽位
	userReleaseFunc, err := h.concurrencyHelper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, req.Stream, &streamStarted)
	if err != nil {
		log.Printf("User concurrency acquire failed: %v", err)
		h.handleConcurrencyError(c, err, "user", streamStarted)
		return
	}
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	// 2. 【新增】Wait后二次检查余额/订阅
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		log.Printf("Billing eligibility check failed after wait: %v", err)
		h.handleStreamingAwareError(c, http.StatusForbidden, "billing_error", err.Error(), streamStarted)
		return
	}

	// 计算粘性会话hash
	sessionHash := h.gatewayService.GenerateSessionHash(body)

	platform := ""
	if apiKey.Group != nil {
		platform = apiKey.Group.Platform
	}

	if platform == service.PlatformGemini {
		const maxAccountSwitches = 3
		switchCount := 0
		failedAccountIDs := make(map[int64]struct{})
		lastFailoverStatus := 0

		for {
			account, err := h.geminiCompatService.SelectAccountForModelWithExclusions(c.Request.Context(), apiKey.GroupID, sessionHash, req.Model, failedAccountIDs)
			if err != nil {
				if len(failedAccountIDs) == 0 {
					h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts: "+err.Error(), streamStarted)
					return
				}
				h.handleFailoverExhausted(c, lastFailoverStatus, streamStarted)
				return
			}

			// 检查预热请求拦截（在账号选择后、转发前检查）
			if account.IsInterceptWarmupEnabled() && isWarmupRequest(body) {
				if req.Stream {
					sendMockWarmupStream(c, req.Model)
				} else {
					sendMockWarmupResponse(c, req.Model)
				}
				return
			}

			// 3. 获取账号并发槽位
			accountReleaseFunc, err := h.concurrencyHelper.AcquireAccountSlotWithWait(c, account.ID, account.Concurrency, req.Stream, &streamStarted)
			if err != nil {
				log.Printf("Account concurrency acquire failed: %v", err)
				h.handleConcurrencyError(c, err, "account", streamStarted)
				return
			}

			// 转发请求
			result, err := h.geminiCompatService.Forward(c.Request.Context(), c, account, body)
			if accountReleaseFunc != nil {
				accountReleaseFunc()
			}
			if err != nil {
				var failoverErr *service.UpstreamFailoverError
				if errors.As(err, &failoverErr) {
					failedAccountIDs[account.ID] = struct{}{}
					if switchCount >= maxAccountSwitches {
						lastFailoverStatus = failoverErr.StatusCode
						h.handleFailoverExhausted(c, lastFailoverStatus, streamStarted)
						return
					}
					lastFailoverStatus = failoverErr.StatusCode
					switchCount++
					log.Printf("Account %d: upstream error %d, switching account %d/%d", account.ID, failoverErr.StatusCode, switchCount, maxAccountSwitches)
					continue
				}
				// 错误响应已在Forward中处理，这里只记录日志
				log.Printf("Forward request failed: %v", err)
				return
			}

			// 异步记录使用量（subscription已在函数开头获取）
			go func(result *service.ForwardResult, usedAccount *service.Account) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
					Result:       result,
					ApiKey:       apiKey,
					User:         apiKey.User,
					Account:      usedAccount,
					Subscription: subscription,
				}); err != nil {
					log.Printf("Record usage failed: %v", err)
				}
			}(result, account)
			return
		}
	}

	const maxAccountSwitches = 3
	switchCount := 0
	failedAccountIDs := make(map[int64]struct{})
	lastFailoverStatus := 0

	for {
		// 选择支持该模型的账号
		account, err := h.gatewayService.SelectAccountForModelWithExclusions(c.Request.Context(), apiKey.GroupID, sessionHash, req.Model, failedAccountIDs)
		if err != nil {
			if len(failedAccountIDs) == 0 {
				h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts: "+err.Error(), streamStarted)
				return
			}
			h.handleFailoverExhausted(c, lastFailoverStatus, streamStarted)
			return
		}

		// 检查预热请求拦截（在账号选择后、转发前检查）
		if account.IsInterceptWarmupEnabled() && isWarmupRequest(body) {
			if req.Stream {
				sendMockWarmupStream(c, req.Model)
			} else {
				sendMockWarmupResponse(c, req.Model)
			}
			return
		}

		// 3. 获取账号并发槽位
		accountReleaseFunc, err := h.concurrencyHelper.AcquireAccountSlotWithWait(c, account.ID, account.Concurrency, req.Stream, &streamStarted)
		if err != nil {
			log.Printf("Account concurrency acquire failed: %v", err)
			h.handleConcurrencyError(c, err, "account", streamStarted)
			return
		}

		// 转发请求
		result, err := h.gatewayService.Forward(c.Request.Context(), c, account, body)
		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}
		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				failedAccountIDs[account.ID] = struct{}{}
				if switchCount >= maxAccountSwitches {
					lastFailoverStatus = failoverErr.StatusCode
					h.handleFailoverExhausted(c, lastFailoverStatus, streamStarted)
					return
				}
				lastFailoverStatus = failoverErr.StatusCode
				switchCount++
				log.Printf("Account %d: upstream error %d, switching account %d/%d", account.ID, failoverErr.StatusCode, switchCount, maxAccountSwitches)
				continue
			}
			// 错误响应已在Forward中处理，这里只记录日志
			log.Printf("Forward request failed: %v", err)
			return
		}

		// 异步记录使用量（subscription已在函数开头获取）
		go func(result *service.ForwardResult, usedAccount *service.Account) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
				Result:       result,
				ApiKey:       apiKey,
				User:         apiKey.User,
				Account:      usedAccount,
				Subscription: subscription,
			}); err != nil {
				log.Printf("Record usage failed: %v", err)
			}
		}(result, account)
		return
	}
}

// Models handles listing available models
// GET /v1/models
// Returns different model lists based on the API key's group platform
func (h *GatewayHandler) Models(c *gin.Context) {
	apiKey, _ := middleware2.GetApiKeyFromContext(c)

	// Return OpenAI models for OpenAI platform groups
	if apiKey != nil && apiKey.Group != nil && apiKey.Group.Platform == "openai" {
		c.JSON(http.StatusOK, gin.H{
			"object": "list",
			"data":   openai.DefaultModels,
		})
		return
	}

	// Default: Claude models
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   claude.DefaultModels,
	})
}

// Usage handles getting account balance for CC Switch integration
// GET /v1/usage
func (h *GatewayHandler) Usage(c *gin.Context) {
	apiKey, ok := middleware2.GetApiKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	// 订阅模式：返回订阅限额信息
	if apiKey.Group != nil && apiKey.Group.IsSubscriptionType() {
		subscription, ok := middleware2.GetSubscriptionFromContext(c)
		if !ok {
			h.errorResponse(c, http.StatusForbidden, "subscription_error", "No active subscription")
			return
		}

		remaining := h.calculateSubscriptionRemaining(apiKey.Group, subscription)
		c.JSON(http.StatusOK, gin.H{
			"isValid":   true,
			"planName":  apiKey.Group.Name,
			"remaining": remaining,
			"unit":      "USD",
		})
		return
	}

	// 余额模式：返回钱包余额
	latestUser, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to get user info")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"isValid":   true,
		"planName":  "钱包余额",
		"remaining": latestUser.Balance,
		"unit":      "USD",
	})
}

// calculateSubscriptionRemaining 计算订阅剩余可用额度
// 逻辑：
// 1. 如果日/周/月任一限额达到100%，返回0
// 2. 否则返回所有已配置周期中剩余额度的最小值
func (h *GatewayHandler) calculateSubscriptionRemaining(group *service.Group, sub *service.UserSubscription) float64 {
	var remainingValues []float64

	// 检查日限额
	if group.HasDailyLimit() {
		remaining := *group.DailyLimitUSD - sub.DailyUsageUSD
		if remaining <= 0 {
			return 0
		}
		remainingValues = append(remainingValues, remaining)
	}

	// 检查周限额
	if group.HasWeeklyLimit() {
		remaining := *group.WeeklyLimitUSD - sub.WeeklyUsageUSD
		if remaining <= 0 {
			return 0
		}
		remainingValues = append(remainingValues, remaining)
	}

	// 检查月限额
	if group.HasMonthlyLimit() {
		remaining := *group.MonthlyLimitUSD - sub.MonthlyUsageUSD
		if remaining <= 0 {
			return 0
		}
		remainingValues = append(remainingValues, remaining)
	}

	// 如果没有配置任何限额，返回-1表示无限制
	if len(remainingValues) == 0 {
		return -1
	}

	// 返回最小值
	min := remainingValues[0]
	for _, v := range remainingValues[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

// handleConcurrencyError handles concurrency-related errors with proper 429 response
func (h *GatewayHandler) handleConcurrencyError(c *gin.Context, err error, slotType string, streamStarted bool) {
	h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error",
		fmt.Sprintf("Concurrency limit exceeded for %s, please retry later", slotType), streamStarted)
}

func (h *GatewayHandler) handleFailoverExhausted(c *gin.Context, statusCode int, streamStarted bool) {
	status, errType, errMsg := h.mapUpstreamError(statusCode)
	h.handleStreamingAwareError(c, status, errType, errMsg, streamStarted)
}

func (h *GatewayHandler) mapUpstreamError(statusCode int) (int, string, string) {
	switch statusCode {
	case 401:
		return http.StatusBadGateway, "upstream_error", "Upstream authentication failed, please contact administrator"
	case 403:
		return http.StatusBadGateway, "upstream_error", "Upstream access forbidden, please contact administrator"
	case 429:
		return http.StatusTooManyRequests, "rate_limit_error", "Upstream rate limit exceeded, please retry later"
	case 529:
		return http.StatusServiceUnavailable, "overloaded_error", "Upstream service overloaded, please retry later"
	case 500, 502, 503, 504:
		return http.StatusBadGateway, "upstream_error", "Upstream service temporarily unavailable"
	default:
		return http.StatusBadGateway, "upstream_error", "Upstream request failed"
	}
}

// handleStreamingAwareError handles errors that may occur after streaming has started
func (h *GatewayHandler) handleStreamingAwareError(c *gin.Context, status int, errType, message string, streamStarted bool) {
	if streamStarted {
		// Stream already started, send error as SSE event then close
		flusher, ok := c.Writer.(http.Flusher)
		if ok {
			// Send error event in SSE format
			errorEvent := fmt.Sprintf(`data: {"type": "error", "error": {"type": "%s", "message": "%s"}}`+"\n\n", errType, message)
			if _, err := fmt.Fprint(c.Writer, errorEvent); err != nil {
				_ = c.Error(err)
			}
			flusher.Flush()
		}
		return
	}

	// Normal case: return JSON response with proper status code
	h.errorResponse(c, status, errType, message)
}

// errorResponse 返回Claude API格式的错误响应
func (h *GatewayHandler) errorResponse(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// CountTokens handles token counting endpoint
// POST /v1/messages/count_tokens
// 特点：校验订阅/余额，但不计算并发、不记录使用量
func (h *GatewayHandler) CountTokens(c *gin.Context) {
	// 从context获取apiKey和user（ApiKeyAuth中间件已设置）
	apiKey, ok := middleware2.GetApiKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	_, ok = middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}

	// 读取请求体
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}

	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	// 解析请求获取模型名
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	// 获取订阅信息（可能为nil）
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	// 校验 billing eligibility（订阅/余额）
	// 【注意】不计算并发，但需要校验订阅/余额
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		h.errorResponse(c, http.StatusForbidden, "billing_error", err.Error())
		return
	}

	// 计算粘性会话 hash
	sessionHash := h.gatewayService.GenerateSessionHash(body)

	// 选择支持该模型的账号
	account, err := h.gatewayService.SelectAccountForModel(c.Request.Context(), apiKey.GroupID, sessionHash, req.Model)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available accounts: "+err.Error())
		return
	}

	// 转发请求（不记录使用量）
	if err := h.gatewayService.ForwardCountTokens(c.Request.Context(), c, account, body); err != nil {
		log.Printf("Forward count_tokens request failed: %v", err)
		// 错误响应已在 ForwardCountTokens 中处理
		return
	}
}

// isWarmupRequest 检测是否为预热请求（标题生成、Warmup等）
func isWarmupRequest(body []byte) bool {
	// 快速检查：如果body不包含关键字，直接返回false
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "title") && !strings.Contains(bodyStr, "Warmup") {
		return false
	}

	// 解析完整请求
	var req struct {
		Messages []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
		System []struct {
			Text string `json:"text"`
		} `json:"system"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}

	// 检查 messages 中的标题提示模式
	for _, msg := range req.Messages {
		for _, content := range msg.Content {
			if content.Type == "text" {
				if strings.Contains(content.Text, "Please write a 5-10 word title for the following conversation:") ||
					content.Text == "Warmup" {
					return true
				}
			}
		}
	}

	// 检查 system 中的标题提取模式
	for _, system := range req.System {
		if strings.Contains(system.Text, "nalyze if this message indicates a new conversation topic. If it does, extract a 2-3 word title") {
			return true
		}
	}

	return false
}

// sendMockWarmupStream 发送流式 mock 响应（用于预热请求拦截）
func sendMockWarmupStream(c *gin.Context, model string) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	events := []string{
		`event: message_start` + "\n" + `data: {"message":{"content":[],"id":"msg_mock_warmup","model":"` + model + `","role":"assistant","stop_reason":null,"stop_sequence":null,"type":"message","usage":{"input_tokens":10,"output_tokens":0}},"type":"message_start"}`,
		`event: content_block_start` + "\n" + `data: {"content_block":{"text":"","type":"text"},"index":0,"type":"content_block_start"}`,
		`event: content_block_delta` + "\n" + `data: {"delta":{"text":"New","type":"text_delta"},"index":0,"type":"content_block_delta"}`,
		`event: content_block_delta` + "\n" + `data: {"delta":{"text":" Conversation","type":"text_delta"},"index":0,"type":"content_block_delta"}`,
		`event: content_block_stop` + "\n" + `data: {"index":0,"type":"content_block_stop"}`,
		`event: message_delta` + "\n" + `data: {"delta":{"stop_reason":"end_turn","stop_sequence":null},"type":"message_delta","usage":{"input_tokens":10,"output_tokens":2}}`,
		`event: message_stop` + "\n" + `data: {"type":"message_stop"}`,
	}

	for _, event := range events {
		_, _ = c.Writer.WriteString(event + "\n\n")
		c.Writer.Flush()
		time.Sleep(20 * time.Millisecond)
	}
}

// sendMockWarmupResponse 发送非流式 mock 响应（用于预热请求拦截）
func sendMockWarmupResponse(c *gin.Context, model string) {
	c.JSON(http.StatusOK, gin.H{
		"id":          "msg_mock_warmup",
		"type":        "message",
		"role":        "assistant",
		"model":       model,
		"content":     []gin.H{{"type": "text", "text": "New Conversation"}},
		"stop_reason": "end_turn",
		"usage": gin.H{
			"input_tokens":  10,
			"output_tokens": 2,
		},
	})
}

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// OpenAIGatewayHandler handles OpenAI API gateway requests
type OpenAIGatewayHandler struct {
	gatewayService      *service.OpenAIGatewayService
	billingCacheService *service.BillingCacheService
	concurrencyHelper   *ConcurrencyHelper
}

// NewOpenAIGatewayHandler creates a new OpenAIGatewayHandler
func NewOpenAIGatewayHandler(
	gatewayService *service.OpenAIGatewayService,
	concurrencyService *service.ConcurrencyService,
	billingCacheService *service.BillingCacheService,
) *OpenAIGatewayHandler {
	return &OpenAIGatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billingCacheService,
		concurrencyHelper:   NewConcurrencyHelper(concurrencyService, SSEPingFormatNone),
	}
}

// Responses handles OpenAI Responses API endpoint
// POST /openai/v1/responses
func (h *OpenAIGatewayHandler) Responses(c *gin.Context) {
	// Get apiKey and user from context (set by ApiKeyAuth middleware)
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

	// Read request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}

	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	// Parse request body to map for potential modification
	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	// Extract model and stream
	reqModel, _ := reqBody["model"].(string)
	reqStream, _ := reqBody["stream"].(bool)

	// For non-Codex CLI requests, set default instructions
	userAgent := c.GetHeader("User-Agent")
	if !openai.IsCodexCLIRequest(userAgent) {
		reqBody["instructions"] = openai.DefaultInstructions
		// Re-serialize body
		body, err = json.Marshal(reqBody)
		if err != nil {
			h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to process request")
			return
		}
	}

	// Track if we've started streaming (for error handling)
	streamStarted := false

	// Get subscription info (may be nil)
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	// 0. Check if wait queue is full
	maxWait := service.CalculateMaxWait(subject.Concurrency)
	canWait, err := h.concurrencyHelper.IncrementWaitCount(c.Request.Context(), subject.UserID, maxWait)
	if err != nil {
		log.Printf("Increment wait count failed: %v", err)
		// On error, allow request to proceed
	} else if !canWait {
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later")
		return
	}
	// Ensure wait count is decremented when function exits
	defer h.concurrencyHelper.DecrementWaitCount(c.Request.Context(), subject.UserID)

	// 1. First acquire user concurrency slot
	userReleaseFunc, err := h.concurrencyHelper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, reqStream, &streamStarted)
	if err != nil {
		log.Printf("User concurrency acquire failed: %v", err)
		h.handleConcurrencyError(c, err, "user", streamStarted)
		return
	}
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	// 2. Re-check billing eligibility after wait
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		log.Printf("Billing eligibility check failed after wait: %v", err)
		h.handleStreamingAwareError(c, http.StatusForbidden, "billing_error", err.Error(), streamStarted)
		return
	}

	// Generate session hash (from header for OpenAI)
	sessionHash := h.gatewayService.GenerateSessionHash(c)

	const maxAccountSwitches = 3
	switchCount := 0
	failedAccountIDs := make(map[int64]struct{})
	lastFailoverStatus := 0

	for {
		// Select account supporting the requested model
		log.Printf("[OpenAI Handler] Selecting account: groupID=%v model=%s", apiKey.GroupID, reqModel)
		account, err := h.gatewayService.SelectAccountForModelWithExclusions(c.Request.Context(), apiKey.GroupID, sessionHash, reqModel, failedAccountIDs)
		if err != nil {
			log.Printf("[OpenAI Handler] SelectAccount failed: %v", err)
			if len(failedAccountIDs) == 0 {
				h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts: "+err.Error(), streamStarted)
				return
			}
			h.handleFailoverExhausted(c, lastFailoverStatus, streamStarted)
			return
		}
		log.Printf("[OpenAI Handler] Selected account: id=%d name=%s", account.ID, account.Name)

		// 3. Acquire account concurrency slot
		accountReleaseFunc, err := h.concurrencyHelper.AcquireAccountSlotWithWait(c, account.ID, account.Concurrency, reqStream, &streamStarted)
		if err != nil {
			log.Printf("Account concurrency acquire failed: %v", err)
			h.handleConcurrencyError(c, err, "account", streamStarted)
			return
		}

		// Forward request
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
			// Error response already handled in Forward, just log
			log.Printf("Forward request failed: %v", err)
			return
		}

		// Async record usage
		go func(result *service.OpenAIForwardResult, usedAccount *service.Account) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
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

// handleConcurrencyError handles concurrency-related errors with proper 429 response
func (h *OpenAIGatewayHandler) handleConcurrencyError(c *gin.Context, err error, slotType string, streamStarted bool) {
	h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error",
		fmt.Sprintf("Concurrency limit exceeded for %s, please retry later", slotType), streamStarted)
}

func (h *OpenAIGatewayHandler) handleFailoverExhausted(c *gin.Context, statusCode int, streamStarted bool) {
	status, errType, errMsg := h.mapUpstreamError(statusCode)
	h.handleStreamingAwareError(c, status, errType, errMsg, streamStarted)
}

func (h *OpenAIGatewayHandler) mapUpstreamError(statusCode int) (int, string, string) {
	switch statusCode {
	case 401:
		return http.StatusBadGateway, "upstream_error", "Upstream authentication failed, please contact administrator"
	case 403:
		return http.StatusBadGateway, "upstream_error", "Upstream access forbidden, please contact administrator"
	case 429:
		return http.StatusTooManyRequests, "rate_limit_error", "Upstream rate limit exceeded, please retry later"
	case 529:
		return http.StatusServiceUnavailable, "upstream_error", "Upstream service overloaded, please retry later"
	case 500, 502, 503, 504:
		return http.StatusBadGateway, "upstream_error", "Upstream service temporarily unavailable"
	default:
		return http.StatusBadGateway, "upstream_error", "Upstream request failed"
	}
}

// handleStreamingAwareError handles errors that may occur after streaming has started
func (h *OpenAIGatewayHandler) handleStreamingAwareError(c *gin.Context, status int, errType, message string, streamStarted bool) {
	if streamStarted {
		// Stream already started, send error as SSE event then close
		flusher, ok := c.Writer.(http.Flusher)
		if ok {
			// Send error event in OpenAI SSE format
			errorEvent := fmt.Sprintf(`event: error`+"\n"+`data: {"error": {"type": "%s", "message": "%s"}}`+"\n\n", errType, message)
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

// errorResponse returns OpenAI API format error response
func (h *OpenAIGatewayHandler) errorResponse(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

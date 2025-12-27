package handler

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// GeminiV1BetaListModels proxies:
// GET /v1beta/models
func (h *GatewayHandler) GeminiV1BetaListModels(c *gin.Context) {
	apiKey, ok := middleware.GetApiKeyFromContext(c)
	if !ok || apiKey == nil {
		googleError(c, http.StatusUnauthorized, "Invalid API key")
		return
	}
	if apiKey.Group == nil || apiKey.Group.Platform != service.PlatformGemini {
		googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
		return
	}

	account, err := h.geminiCompatService.SelectAccountForAIStudioEndpoints(c.Request.Context(), apiKey.GroupID)
	if err != nil {
		googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts: "+err.Error())
		return
	}

	res, err := h.geminiCompatService.ForwardAIStudioGET(c.Request.Context(), account, "/v1beta/models")
	if err != nil {
		googleError(c, http.StatusBadGateway, err.Error())
		return
	}
	if shouldFallbackGeminiModels(res) {
		c.JSON(http.StatusOK, gemini.FallbackModelsList())
		return
	}
	writeUpstreamResponse(c, res)
}

// GeminiV1BetaGetModel proxies:
// GET /v1beta/models/{model}
func (h *GatewayHandler) GeminiV1BetaGetModel(c *gin.Context) {
	apiKey, ok := middleware.GetApiKeyFromContext(c)
	if !ok || apiKey == nil {
		googleError(c, http.StatusUnauthorized, "Invalid API key")
		return
	}
	if apiKey.Group == nil || apiKey.Group.Platform != service.PlatformGemini {
		googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
		return
	}

	modelName := strings.TrimSpace(c.Param("model"))
	if modelName == "" {
		googleError(c, http.StatusBadRequest, "Missing model in URL")
		return
	}

	account, err := h.geminiCompatService.SelectAccountForAIStudioEndpoints(c.Request.Context(), apiKey.GroupID)
	if err != nil {
		googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts: "+err.Error())
		return
	}

	res, err := h.geminiCompatService.ForwardAIStudioGET(c.Request.Context(), account, "/v1beta/models/"+modelName)
	if err != nil {
		googleError(c, http.StatusBadGateway, err.Error())
		return
	}
	if shouldFallbackGeminiModels(res) {
		c.JSON(http.StatusOK, gemini.FallbackModel(modelName))
		return
	}
	writeUpstreamResponse(c, res)
}

// GeminiV1BetaModels proxies Gemini native REST endpoints like:
// POST /v1beta/models/{model}:generateContent
// POST /v1beta/models/{model}:streamGenerateContent?alt=sse
func (h *GatewayHandler) GeminiV1BetaModels(c *gin.Context) {
	apiKey, ok := middleware.GetApiKeyFromContext(c)
	if !ok || apiKey == nil {
		googleError(c, http.StatusUnauthorized, "Invalid API key")
		return
	}
	authSubject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		googleError(c, http.StatusInternalServerError, "User context not found")
		return
	}

	if apiKey.Group == nil || apiKey.Group.Platform != service.PlatformGemini {
		googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
		return
	}

	modelName, action, err := parseGeminiModelAction(strings.TrimPrefix(c.Param("modelAction"), "/"))
	if err != nil {
		googleError(c, http.StatusNotFound, err.Error())
		return
	}

	stream := action == "streamGenerateContent"

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		googleError(c, http.StatusBadRequest, "Failed to read request body")
		return
	}
	if len(body) == 0 {
		googleError(c, http.StatusBadRequest, "Request body is empty")
		return
	}

	// Get subscription (may be nil)
	subscription, _ := middleware.GetSubscriptionFromContext(c)

	// For Gemini native API, do not send Claude-style ping frames.
	geminiConcurrency := NewConcurrencyHelper(h.concurrencyHelper.concurrencyService, SSEPingFormatNone)

	// 0) wait queue check
	maxWait := service.CalculateMaxWait(authSubject.Concurrency)
	canWait, err := geminiConcurrency.IncrementWaitCount(c.Request.Context(), authSubject.UserID, maxWait)
	if err != nil {
		log.Printf("Increment wait count failed: %v", err)
	} else if !canWait {
		googleError(c, http.StatusTooManyRequests, "Too many pending requests, please retry later")
		return
	}
	defer geminiConcurrency.DecrementWaitCount(c.Request.Context(), authSubject.UserID)

	// 1) user concurrency slot
	streamStarted := false
	userReleaseFunc, err := geminiConcurrency.AcquireUserSlotWithWait(c, authSubject.UserID, authSubject.Concurrency, stream, &streamStarted)
	if err != nil {
		googleError(c, http.StatusTooManyRequests, err.Error())
		return
	}
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	// 2) billing eligibility check (after wait)
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		googleError(c, http.StatusForbidden, err.Error())
		return
	}

	// 3) select account (sticky session based on request body)
	sessionHash := h.gatewayService.GenerateSessionHash(body)
	const maxAccountSwitches = 3
	switchCount := 0
	failedAccountIDs := make(map[int64]struct{})
	lastFailoverStatus := 0

	for {
		account, err := h.geminiCompatService.SelectAccountForModelWithExclusions(c.Request.Context(), apiKey.GroupID, sessionHash, modelName, failedAccountIDs)
		if err != nil {
			if len(failedAccountIDs) == 0 {
				googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts: "+err.Error())
				return
			}
			handleGeminiFailoverExhausted(c, lastFailoverStatus)
			return
		}

		// 4) account concurrency slot
		accountReleaseFunc, err := geminiConcurrency.AcquireAccountSlotWithWait(c, account.ID, account.Concurrency, stream, &streamStarted)
		if err != nil {
			googleError(c, http.StatusTooManyRequests, err.Error())
			return
		}

		// 5) forward (writes response to client)
		result, err := h.geminiCompatService.ForwardNative(c.Request.Context(), c, account, modelName, action, stream, body)
		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}
		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				failedAccountIDs[account.ID] = struct{}{}
				if switchCount >= maxAccountSwitches {
					lastFailoverStatus = failoverErr.StatusCode
					handleGeminiFailoverExhausted(c, lastFailoverStatus)
					return
				}
				lastFailoverStatus = failoverErr.StatusCode
				switchCount++
				log.Printf("Gemini account %d: upstream error %d, switching account %d/%d", account.ID, failoverErr.StatusCode, switchCount, maxAccountSwitches)
				continue
			}
			// ForwardNative already wrote the response
			log.Printf("Gemini native forward failed: %v", err)
			return
		}

		// 6) record usage async
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

func parseGeminiModelAction(rest string) (model string, action string, err error) {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", "", &pathParseError{"missing path"}
	}

	// Standard: {model}:{action}
	if i := strings.Index(rest, ":"); i > 0 && i < len(rest)-1 {
		return rest[:i], rest[i+1:], nil
	}

	// Fallback: {model}/{action}
	if i := strings.Index(rest, "/"); i > 0 && i < len(rest)-1 {
		return rest[:i], rest[i+1:], nil
	}

	return "", "", &pathParseError{"invalid model action path"}
}

func handleGeminiFailoverExhausted(c *gin.Context, statusCode int) {
	status, message := mapGeminiUpstreamError(statusCode)
	googleError(c, status, message)
}

func mapGeminiUpstreamError(statusCode int) (int, string) {
	switch statusCode {
	case 401:
		return http.StatusBadGateway, "Upstream authentication failed, please contact administrator"
	case 403:
		return http.StatusBadGateway, "Upstream access forbidden, please contact administrator"
	case 429:
		return http.StatusTooManyRequests, "Upstream rate limit exceeded, please retry later"
	case 529:
		return http.StatusServiceUnavailable, "Upstream service overloaded, please retry later"
	case 500, 502, 503, 504:
		return http.StatusBadGateway, "Upstream service temporarily unavailable"
	default:
		return http.StatusBadGateway, "Upstream request failed"
	}
}

type pathParseError struct{ msg string }

func (e *pathParseError) Error() string { return e.msg }

func googleError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    status,
			"message": message,
			"status":  googleapi.HTTPStatusToGoogleStatus(status),
		},
	})
}

func writeUpstreamResponse(c *gin.Context, res *service.UpstreamHTTPResult) {
	if res == nil {
		googleError(c, http.StatusBadGateway, "Empty upstream response")
		return
	}
	for k, vv := range res.Headers {
		// Avoid overriding content-length and hop-by-hop headers.
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Connection") {
			continue
		}
		for _, v := range vv {
			c.Writer.Header().Add(k, v)
		}
	}
	contentType := res.Headers.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(res.StatusCode, contentType, res.Body)
}

func shouldFallbackGeminiModels(res *service.UpstreamHTTPResult) bool {
	if res == nil {
		return true
	}
	if res.StatusCode != http.StatusUnauthorized && res.StatusCode != http.StatusForbidden {
		return false
	}
	if strings.Contains(strings.ToLower(res.Headers.Get("Www-Authenticate")), "insufficient_scope") {
		return true
	}
	if strings.Contains(strings.ToLower(string(res.Body)), "insufficient authentication scopes") {
		return true
	}
	if strings.Contains(strings.ToLower(string(res.Body)), "access_token_scope_insufficient") {
		return true
	}
	return false
}

package handler

import (
	"context"
	"encoding/json"
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
REDACTED

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
REDACTED
REDACTED

// Responses handles OpenAI Responses API endpoint
// POST /openai/v1/responses
func (h *OpenAIGatewayHandler) Responses(c *gin.Context) {
	// Get apiKey and user from context (set by ApiKeyAuth middleware)
	apiKey, ok := middleware2.GetApiKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
REDACTED

	user, ok := middleware2.GetUserFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
REDACTED

	// Read request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
REDACTED

	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
REDACTED

	// Parse request body to map for potential modification
	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
REDACTED

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
	REDACTED
REDACTED

	// Track if we've started streaming (for error handling)
	streamStarted := false

	// Get subscription info (may be nil)
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	// 0. Check if wait queue is full
	maxWait := service.CalculateMaxWait(user.Concurrency)
	canWait, err := h.concurrencyHelper.IncrementWaitCount(c.Request.Context(), user.ID, maxWait)
	if err != nil {
		log.Printf("Increment wait count failed: %v", err)
		// On error, allow request to proceed
REDACTED else if !canWait {
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later")
		return
REDACTED
	// Ensure wait count is decremented when function exits
	defer h.concurrencyHelper.DecrementWaitCount(c.Request.Context(), user.ID)

	// 1. First acquire user concurrency slot
	userReleaseFunc, err := h.concurrencyHelper.AcquireUserSlotWithWait(c, user, reqStream, &streamStarted)
	if err != nil {
		log.Printf("User concurrency acquire failed: %v", err)
		h.handleConcurrencyError(c, err, "user", streamStarted)
		return
REDACTED
	if userReleaseFunc != nil {
		defer userReleaseFunc()
REDACTED

	// 2. Re-check billing eligibility after wait
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), user, apiKey, apiKey.Group, subscription); err != nil {
		log.Printf("Billing eligibility check failed after wait: %v", err)
		h.handleStreamingAwareError(c, http.StatusForbidden, "billing_error", err.Error(), streamStarted)
		return
REDACTED

	// Generate session hash (from header for OpenAI)
	sessionHash := h.gatewayService.GenerateSessionHash(c)

	// Select account supporting the requested model
	log.Printf("[OpenAI Handler] Selecting account: groupID=%v model=%s", apiKey.GroupID, reqModel)
	account, err := h.gatewayService.SelectAccountForModel(c.Request.Context(), apiKey.GroupID, sessionHash, reqModel)
	if err != nil {
		log.Printf("[OpenAI Handler] SelectAccount failed: %v", err)
		h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts: "+err.Error(), streamStarted)
		return
REDACTED
	log.Printf("[OpenAI Handler] Selected account: id=%d name=%s", account.ID, account.Name)

	// 3. Acquire account concurrency slot
	accountReleaseFunc, err := h.concurrencyHelper.AcquireAccountSlotWithWait(c, account, reqStream, &streamStarted)
	if err != nil {
		log.Printf("Account concurrency acquire failed: %v", err)
		h.handleConcurrencyError(c, err, "account", streamStarted)
		return
REDACTED
	if accountReleaseFunc != nil {
		defer accountReleaseFunc()
REDACTED

	// Forward request
	result, err := h.gatewayService.Forward(c.Request.Context(), c, account, body)
	if err != nil {
		// Error response already handled in Forward, just log
		log.Printf("Forward request failed: %v", err)
		return
REDACTED

	// Async record usage
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
			Result:       result,
			ApiKey:       apiKey,
			User:         user,
			Account:      account,
			Subscription: subscription,
	REDACTED); err != nil {
			log.Printf("Record usage failed: %v", err)
	REDACTED
REDACTED()
REDACTED

// handleConcurrencyError handles concurrency-related errors with proper 429 response
func (h *OpenAIGatewayHandler) handleConcurrencyError(c *gin.Context, err error, slotType string, streamStarted bool) {
	h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error",
		fmt.Sprintf("Concurrency limit exceeded for %s, please retry later", slotType), streamStarted)
REDACTED

// handleStreamingAwareError handles errors that may occur after streaming has started
func (h *OpenAIGatewayHandler) handleStreamingAwareError(c *gin.Context, status int, errType, message string, streamStarted bool) {
	if streamStarted {
		// Stream already started, send error as SSE event then close
		flusher, ok := c.Writer.(http.Flusher)
		if ok {
			// Send error event in OpenAI SSE format
			errorEvent := fmt.Sprintf(`event: error`+"\n"+`data: {"error": {"type": "%s", "message": "%s"REDACTEDREDACTED`+"\n\n", errType, message)
			if _, err := fmt.Fprint(c.Writer, errorEvent); err != nil {
				_ = c.Error(err)
		REDACTED
			flusher.Flush()
	REDACTED
		return
REDACTED

	// Normal case: return JSON response with proper status code
	h.errorResponse(c, status, errType, message)
REDACTED

// errorResponse returns OpenAI API format error response
func (h *OpenAIGatewayHandler) errorResponse(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
	REDACTED,
REDACTED)
REDACTED

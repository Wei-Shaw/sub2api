package handler

import (
	"context"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/middleware"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
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
REDACTED
	if apiKey.Group == nil || apiKey.Group.Platform != model.PlatformGemini {
		googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
		return
REDACTED

	account, err := h.geminiCompatService.SelectAccountForAIStudioEndpoints(c.Request.Context(), apiKey.GroupID)
	if err != nil {
		googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts: "+err.Error())
		return
REDACTED

	res, err := h.geminiCompatService.ForwardAIStudioGET(c.Request.Context(), account, "/v1beta/models")
	if err != nil {
		googleError(c, http.StatusBadGateway, err.Error())
		return
REDACTED
	if shouldFallbackGeminiModels(res) {
		c.JSON(http.StatusOK, gemini.FallbackModelsList())
		return
REDACTED
	writeUpstreamResponse(c, res)
REDACTED

// GeminiV1BetaGetModel proxies:
// GET /v1beta/models/{modelREDACTED
func (h *GatewayHandler) GeminiV1BetaGetModel(c *gin.Context) {
	apiKey, ok := middleware.GetApiKeyFromContext(c)
	if !ok || apiKey == nil {
		googleError(c, http.StatusUnauthorized, "Invalid API key")
		return
REDACTED
	if apiKey.Group == nil || apiKey.Group.Platform != model.PlatformGemini {
		googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
		return
REDACTED

	modelName := strings.TrimSpace(c.Param("model"))
	if modelName == "" {
		googleError(c, http.StatusBadRequest, "Missing model in URL")
		return
REDACTED

	account, err := h.geminiCompatService.SelectAccountForAIStudioEndpoints(c.Request.Context(), apiKey.GroupID)
	if err != nil {
		googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts: "+err.Error())
		return
REDACTED

	res, err := h.geminiCompatService.ForwardAIStudioGET(c.Request.Context(), account, "/v1beta/models/"+modelName)
	if err != nil {
		googleError(c, http.StatusBadGateway, err.Error())
		return
REDACTED
	if shouldFallbackGeminiModels(res) {
		c.JSON(http.StatusOK, gemini.FallbackModel(modelName))
		return
REDACTED
	writeUpstreamResponse(c, res)
REDACTED

// GeminiV1BetaModels proxies Gemini native REST endpoints like:
// POST /v1beta/models/{modelREDACTED:generateContent
// POST /v1beta/models/{modelREDACTED:streamGenerateContent?alt=sse
func (h *GatewayHandler) GeminiV1BetaModels(c *gin.Context) {
	apiKey, ok := middleware.GetApiKeyFromContext(c)
	if !ok || apiKey == nil {
		googleError(c, http.StatusUnauthorized, "Invalid API key")
		return
REDACTED
	user, ok := middleware.GetUserFromContext(c)
	if !ok || user == nil {
		googleError(c, http.StatusInternalServerError, "User context not found")
		return
REDACTED

	if apiKey.Group == nil || apiKey.Group.Platform != model.PlatformGemini {
		googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
		return
REDACTED

	modelName, action, err := parseGeminiModelAction(strings.TrimPrefix(c.Param("modelAction"), "/"))
	if err != nil {
		googleError(c, http.StatusNotFound, err.Error())
		return
REDACTED

	stream := action == "streamGenerateContent"

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		googleError(c, http.StatusBadRequest, "Failed to read request body")
		return
REDACTED
	if len(body) == 0 {
		googleError(c, http.StatusBadRequest, "Request body is empty")
		return
REDACTED

	// Get subscription (may be nil)
	subscription, _ := middleware.GetSubscriptionFromContext(c)

	// For Gemini native API, do not send Claude-style ping frames.
	geminiConcurrency := NewConcurrencyHelper(h.concurrencyHelper.concurrencyService, SSEPingFormatNone)

	// 0) wait queue check
	maxWait := service.CalculateMaxWait(user.Concurrency)
	canWait, err := geminiConcurrency.IncrementWaitCount(c.Request.Context(), user.ID, maxWait)
	if err != nil {
		log.Printf("Increment wait count failed: %v", err)
REDACTED else if !canWait {
		googleError(c, http.StatusTooManyRequests, "Too many pending requests, please retry later")
		return
REDACTED
	defer geminiConcurrency.DecrementWaitCount(c.Request.Context(), user.ID)

	// 1) user concurrency slot
	streamStarted := false
	userReleaseFunc, err := geminiConcurrency.AcquireUserSlotWithWait(c, user, stream, &streamStarted)
	if err != nil {
		googleError(c, http.StatusTooManyRequests, err.Error())
		return
REDACTED
	if userReleaseFunc != nil {
		defer userReleaseFunc()
REDACTED

	// 2) billing eligibility check (after wait)
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), user, apiKey, apiKey.Group, subscription); err != nil {
		googleError(c, http.StatusForbidden, err.Error())
		return
REDACTED

	// 3) select account (sticky session based on request body)
	sessionHash := h.gatewayService.GenerateSessionHash(body)
	account, err := h.geminiCompatService.SelectAccountForModel(c.Request.Context(), apiKey.GroupID, sessionHash, modelName)
	if err != nil {
		googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts: "+err.Error())
		return
REDACTED

	// 4) account concurrency slot
	accountReleaseFunc, err := geminiConcurrency.AcquireAccountSlotWithWait(c, account, stream, &streamStarted)
	if err != nil {
		googleError(c, http.StatusTooManyRequests, err.Error())
		return
REDACTED
	if accountReleaseFunc != nil {
		defer accountReleaseFunc()
REDACTED

	// 5) forward (writes response to client)
	result, err := h.geminiCompatService.ForwardNative(c.Request.Context(), c, account, modelName, action, stream, body)
	if err != nil {
		// ForwardNative already wrote the response
		log.Printf("Gemini native forward failed: %v", err)
		return
REDACTED

	// 6) record usage async
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
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

func parseGeminiModelAction(rest string) (model string, action string, err error) {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", "", &pathParseError{"missing path"REDACTED
REDACTED

	// Standard: {modelREDACTED:{actionREDACTED
	if i := strings.Index(rest, ":"); i > 0 && i < len(rest)-1 {
		return rest[:i], rest[i+1:], nil
REDACTED

	// Fallback: {modelREDACTED/{actionREDACTED
	if i := strings.Index(rest, "/"); i > 0 && i < len(rest)-1 {
		return rest[:i], rest[i+1:], nil
REDACTED

	return "", "", &pathParseError{"invalid model action path"REDACTED
REDACTED

type pathParseError struct{ msg string REDACTED

func (e *pathParseError) Error() string { return e.msg REDACTED

func googleError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    status,
			"message": message,
			"status":  googleapi.HTTPStatusToGoogleStatus(status),
	REDACTED,
REDACTED)
REDACTED

func writeUpstreamResponse(c *gin.Context, res *service.UpstreamHTTPResult) {
	if res == nil {
		googleError(c, http.StatusBadGateway, "Empty upstream response")
		return
REDACTED
	for k, vv := range res.Headers {
		// Avoid overriding content-length and hop-by-hop headers.
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Connection") {
			continue
	REDACTED
		for _, v := range vv {
			c.Writer.Header().Add(k, v)
	REDACTED
REDACTED
	contentType := res.Headers.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
REDACTED
	c.Data(res.StatusCode, contentType, res.Body)
REDACTED

func shouldFallbackGeminiModels(res *service.UpstreamHTTPResult) bool {
	if res == nil {
		return true
REDACTED
	if res.StatusCode != http.StatusUnauthorized && res.StatusCode != http.StatusForbidden {
		return false
REDACTED
	if strings.Contains(strings.ToLower(res.Headers.Get("Www-Authenticate")), "insufficient_scope") {
		return true
REDACTED
	if strings.Contains(strings.ToLower(string(res.Body)), "insufficient authentication scopes") {
		return true
REDACTED
	if strings.Contains(strings.ToLower(string(res.Body)), "access_token_scope_insufficient") {
		return true
REDACTED
	return false
REDACTED

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/websearch"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const (
	defaultGrokWebSearchResults = 5
	maxGrokWebSearchResults     = 20
)

func (h *GatewayHandler) WebSearch(c *gin.Context) {
	type webSearchReq struct {
		Query      string `json:"query" binding:"required"`
		MaxResults int    `json:"max_results"`
REDACTED

	var req webSearchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "invalid_request_error",
			"message": err.Error(),
	REDACTEDREDACTED)
		return
REDACTED
	req.MaxResults = normalizeGrokWebSearchMaxResults(req.MaxResults)

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
			"type":    "authentication_error",
			"message": "API key required",
	REDACTEDREDACTED)
		return
REDACTED

	if apiKey.Group == nil || apiKey.Group.Platform != "grok" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "invalid_request_error",
			"message": "web search is only supported for grok groups",
	REDACTEDREDACTED)
		return
REDACTED

	// Billing eligibility (same as other requests)
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
	REDACTED
		c.JSON(status, gin.H{"error": gin.H{"type": code, "message": messageREDACTEDREDACTED)
		return
REDACTED

	// Use exactly the same scheduling as other requests (SelectAccountWithLoadAwareness handles load, rate limit, sticky, etc.)
	groupID := apiKey.GroupID
	if groupID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "invalid_request_error",
			"message": "group required",
	REDACTEDREDACTED)
		return
REDACTED

	selected, err := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), groupID, "", xai.DefaultTextModel, nil, "", 0)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
			"type":    "scheduling_error",
			"message": err.Error(),
	REDACTEDREDACTED)
		return
REDACTED
	if selected == nil || selected.Account == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
			"type":    "scheduling_error",
			"message": "No available accounts",
	REDACTEDREDACTED)
		return
REDACTED
	account := selected.Account
	accountReleaseFunc := selected.ReleaseFunc
	if !selected.Acquired {
		if selected.WaitPlan == nil || h.concurrencyHelper == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
				"type":    "scheduling_error",
				"message": "No available accounts",
		REDACTEDREDACTED)
			return
	REDACTED
		accountWaitCounted := false
		canWait, waitErr := h.concurrencyHelper.IncrementAccountWaitCount(c.Request.Context(), account.ID, selected.WaitPlan.MaxWaiting)
		if waitErr != nil {
			logger.L().Warn("gateway.web_search.account_wait_counter_increment_failed",
				zap.Int64("account_id", account.ID),
				zap.Error(waitErr),
			)
	REDACTED else if !canWait {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": gin.H{
				"type":    "rate_limit_error",
				"message": "Too many pending requests, please retry later",
		REDACTEDREDACTED)
			return
	REDACTED else {
			accountWaitCounted = true
	REDACTED
		releaseWait := func() {
			if accountWaitCounted {
				h.concurrencyHelper.DecrementAccountWaitCount(c.Request.Context(), account.ID)
				accountWaitCounted = false
		REDACTED
	REDACTED
		streamStarted := false
		release, acquireErr := h.concurrencyHelper.AcquireAccountSlotWithWaitTimeout(
			c,
			account.ID,
			selected.WaitPlan.MaxConcurrency,
			selected.WaitPlan.Timeout,
			false,
			&streamStarted,
		)
		releaseWait()
		if acquireErr != nil {
			h.handleConcurrencyError(c, acquireErr, "account", streamStarted)
			return
	REDACTED
		accountReleaseFunc = release
REDACTED
	if accountReleaseFunc != nil {
		defer accountReleaseFunc()
REDACTED

	// Scheduling is 100% the same as other requests:
	// SelectAccountWithLoadAwareness handles load balancing, rate limits, failover, sticky sessions, concurrency, proxies etc.
	// Downstream rate limiting, billing etc. can be wired the same way.

	// Use Grok *native* web search via the selected Grok account + responses API + web_search tool.
	// This ensures results come from Grok's own search (not third-party emulation like Tavily/Brave).
	// Output is normalized to the same unified format for clients/agents/MCP.

	nativeResp, providerName, err := h.doGrokNativeWebSearch(c.Request.Context(), c, account, req.Query, req.MaxResults)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
			"type":    "web_search_error",
			"message": err.Error(),
	REDACTEDREDACTED)
		return
REDACTED

	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	requestPayloadHash := service.HashUsageRequestPayload([]byte(req.Query))
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	// Unique per invocation (not hash(query) alone) so identical queries still bill.
	searchRequestID := "web_search:" + service.HashUsageRequestPayload([]byte(req.Query+"|"+clientIP+"|"+userAgent))
	if apiKey.Group != nil && (apiKey.Group.GetSearchPricePer1k() == nil || *apiKey.Group.GetSearchPricePer1k() <= 0) {
		logger.L().With(
			zap.String("component", "handler.gateway.web_search"),
			zap.Int64("group_id", apiKey.Group.ID),
		).Warn("gateway.web_search.search_price_per_1k_unset_free")
REDACTED
	h.submitUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
			Result: &service.ForwardResult{
				RequestID:   searchRequestID,
				Model:       "grok-web-search",
				SearchCount: 1,
				Duration:    0,
		REDACTED,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   upstreamEndpoint,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			RequestPayloadHash: requestPayloadHash,
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      quotaPlatform,
	REDACTED); err != nil {
			logger.L().With(
				zap.String("component", "handler.gateway.web_search"),
				zap.Int64("user_id", apiKey.User.ID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Int64("account_id", account.ID),
			).Error("gateway.web_search.record_usage_failed", zap.Error(err))
	REDACTED
REDACTED)

	c.JSON(http.StatusOK, gin.H{
		"query":       req.Query,
		"results":     nativeResp.Results,
		"provider":    providerName,
		"max_results": req.MaxResults,
REDACTED)
REDACTED

// doGrokNativeWebSearch executes web search using the Grok account's native capability
// by calling the responses endpoint with web_search tool, then normalizes sources to unified format.
func (h *GatewayHandler) doGrokNativeWebSearch(ctx context.Context, c *gin.Context, account *service.Account, query string, maxResults int) (*websearch.SearchResponse, string, error) {
	maxResults = normalizeGrokWebSearchMaxResults(maxResults)

	// Build a minimal responses request that triggers Grok web search tool.
	// Ask for structured metadata because xAI action.sources commonly contains URLs only.
	searchBody := map[string]any{
		"model":   xai.DefaultTextModel,
		"input":   buildGrokWebSearchPrompt(query, maxResults),
		"tools":   []map[string]any{{"type": "web_search"REDACTEDREDACTED,
		"include": []string{"web_search_call.action.sources"REDACTED,
		"store":   false,
		"stream":  false,
REDACTED
	bodyBytes, _ := json.Marshal(searchBody)

	respBytes, err := h.gatewayService.DoGrokNativeResponsesJSON(ctx, c, account, bodyBytes)
	if err != nil {
		return nil, "", err
REDACTED

	// Extract sources from Grok responses output.
	// Prefer web_search_call.action.sources (standardized), fallback to annotations or text links.
	results := extractGrokWebSearchSources(respBytes, maxResults)

	return &websearch.SearchResponse{
		Results: results,
		Query:   query,
REDACTED, "grok-native", nil
REDACTED

func normalizeGrokWebSearchMaxResults(maxResults int) int {
	if maxResults <= 0 {
		return defaultGrokWebSearchResults
REDACTED
	if maxResults > maxGrokWebSearchResults {
		return maxGrokWebSearchResults
REDACTED
	return maxResults
REDACTED

func buildGrokWebSearchPrompt(query string, maxResults int) string {
	return fmt.Sprintf(`Search the web for the user query below. Return ONLY valid JSON with this exact shape: {"results":[{"url":"https://...","title":"page title","snippet":"concise factual summary"REDACTED]REDACTED. Return at most %d unique results. Every URL must be an actual web_search source. Populate a non-empty title and snippet for every result. Do not wrap the JSON in markdown.

User query:
%s`, normalizeGrokWebSearchMaxResults(maxResults), query)
REDACTED

// extractGrokWebSearchSources returns model-enriched results only when their URLs
// are present in the actual web_search sources, then falls back to raw sources.
func extractGrokWebSearchSources(body []byte, maxResults int) []websearch.SearchResult {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil
REDACTED
	maxResults = normalizeGrokWebSearchMaxResults(maxResults)

	sources := make(map[string]websearch.SearchResult)
	var sourceOrder []string
	addSource := func(rawURL, title, snippet string) {
		key, ok := normalizeGrokWebSearchURL(rawURL)
		if !ok {
			return
	REDACTED
		result, exists := sources[key]
		if !exists {
			result.URL = strings.TrimSpace(rawURL)
			sourceOrder = append(sourceOrder, key)
	REDACTED
		if result.Title == "" {
			result.Title = usableGrokWebSearchTitle(title, result.URL)
	REDACTED
		if result.Snippet == "" {
			result.Snippet = strings.TrimSpace(snippet)
	REDACTED
		sources[key] = result
REDACTED

	output := gjson.GetBytes(body, "output")
	output.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "web_search_call" {
			sources := item.Get("action.sources")
			if sources.IsArray() {
				sources.ForEach(func(_, src gjson.Result) bool {
					addSource(src.Get("url").String(), src.Get("title").String(), src.Get("snippet").String())
					return true
			REDACTED)
		REDACTED
	REDACTED
		if item.Get("type").String() == "message" {
			item.Get("content").ForEach(func(_, part gjson.Result) bool {
				if part.Get("type").String() != "output_text" {
					return true
			REDACTED
				part.Get("annotations").ForEach(func(_, ann gjson.Result) bool {
					if ann.Get("type").String() == "url_citation" || ann.Get("type").String() == "web" {
						addSource(ann.Get("url").String(), ann.Get("title").String(), "")
				REDACTED
					return true
			REDACTED)
				return true
		REDACTED)
	REDACTED
		return true
REDACTED)

	var out []websearch.SearchResult
	seen := make(map[string]bool)
	output.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() != "message" {
			return true
	REDACTED
		item.Get("content").ForEach(func(_, part gjson.Result) bool {
			if part.Get("type").String() != "output_text" || len(out) >= maxResults {
				return true
		REDACTED
			for _, result := range parseGrokWebSearchStructuredResults(part.Get("text").String()) {
				key, ok := normalizeGrokWebSearchURL(result.URL)
				if !ok || seen[key] {
					continue
			REDACTED
				source, allowed := sources[key]
				if !allowed {
					continue
			REDACTED
				seen[key] = true
				result.URL = source.URL
				result.Title = usableGrokWebSearchTitle(result.Title, result.URL)
				if result.Title == "" {
					result.Title = source.Title
			REDACTED
				result.Snippet = strings.TrimSpace(result.Snippet)
				if result.Snippet == "" {
					result.Snippet = source.Snippet
			REDACTED
				out = append(out, result)
				if len(out) >= maxResults {
					break
			REDACTED
		REDACTED
			return true
	REDACTED)
		return len(out) < maxResults
REDACTED)

	for _, key := range sourceOrder {
		if len(out) >= maxResults {
			break
	REDACTED
		if seen[key] {
			continue
	REDACTED
		result := sources[key]
		if result.Title == "" {
			result.Title = grokWebSearchTitleFromURL(result.URL)
	REDACTED
		seen[key] = true
		out = append(out, result)
REDACTED
	return out
REDACTED

func parseGrokWebSearchStructuredResults(text string) []websearch.SearchResult {
	text = strings.TrimSpace(text)
	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, 'REDACTED')
	if start < 0 || end < start {
		return nil
REDACTED
	var payload struct {
		Results []websearch.SearchResult `json:"results"`
REDACTED
	if err := json.Unmarshal([]byte(text[start:end+1]), &payload); err != nil {
		return nil
REDACTED
	return payload.Results
REDACTED

func normalizeGrokWebSearchURL(rawURL string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
REDACTED
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	if u.Path == "" {
		u.Path = "/"
REDACTED
	return u.String(), true
REDACTED

func usableGrokWebSearchTitle(title, rawURL string) string {
	title = strings.TrimSpace(title)
	if title == "" || title == rawURL {
		return ""
REDACTED
	if _, err := strconv.Atoi(title); err == nil {
		return ""
REDACTED
	return title
REDACTED

func grokWebSearchTitleFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
REDACTED
	return strings.TrimPrefix(strings.ToLower(u.Host), "www.")
REDACTED

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/websearch"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	}

	var req webSearchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "invalid_request_error",
			"message": err.Error(),
		}})
		return
	}
	req.MaxResults = normalizeGrokWebSearchMaxResults(req.MaxResults)

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
			"type":    "authentication_error",
			"message": "API key required",
		}})
		return
	}

	if apiKey.Group == nil || apiKey.Group.Platform != "grok" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "invalid_request_error",
			"message": "web search is only supported for grok groups",
		}})
		return
	}

	// Billing eligibility (same as other requests)
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		c.JSON(status, gin.H{"error": gin.H{"type": code, "message": message}})
		return
	}

	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	reqLog := requestLogger(c, "handler.gateway.web_search")
	// Audit user search query before upstream Grok web_search traffic.
	auditBody, _ := json.Marshal(map[string]any{
		"messages": []map[string]any{{
			"role": "user", "content": req.Query,
		}},
	})
	if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIChat, xai.DefaultTextModel, auditBody); decision != nil && !decision.AllowNextStage {
		status := decision.HTTPStatus
		if status == 0 {
			status = http.StatusForbidden
		}
		code := decision.ErrorCode
		if code == "" {
			code = "content_policy_violation"
		}
		msg := decision.ClientMessage
		if msg == "" {
			msg = "Request blocked by content policy"
		}
		c.JSON(status, gin.H{"error": gin.H{"type": code, "message": msg}})
		return
	}

	// Use exactly the same scheduling as other requests (SelectAccountWithLoadAwareness handles load, rate limit, sticky, etc.)
	groupID := apiKey.GroupID
	if groupID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "invalid_request_error",
			"message": "group required",
		}})
		return
	}

	failedAccounts := make(map[int64]struct{})
	var account *service.Account
	var accountReleaseFunc func()
	var nativeResp *websearch.SearchResponse
	var providerName string
	var err error

	// Acquire + release holder for the whole handler (including failover retries).
	defer func() {
		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}
	}()

	// First attempt + up to 3 failover accounts (max 4 total).
	for attempt := 0; attempt < 4; attempt++ {
		selected, selectErr := h.gatewayService.SelectAccountWithLoadAwareness(
			c.Request.Context(), groupID, "", xai.DefaultTextModel, failedAccounts, "", 0,
		)
		if selectErr != nil {
			if attempt == 0 {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
					"type":    "scheduling_error",
					"message": selectErr.Error(),
				}})
				return
			}
			break
		}
		if selected == nil || selected.Account == nil {
			if attempt == 0 {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
					"type":    "scheduling_error",
					"message": "No available accounts",
				}})
				return
			}
			break
		}

		release, acquireOK, acquireErr := h.acquireWebSearchAccountSlot(c, selected)
		if !acquireOK {
			// First hop: surface concurrency errors; later hops try another account.
			if attempt == 0 && acquireErr != nil {
				h.handleConcurrencyError(c, acquireErr, "account", false)
				return
			}
			failedAccounts[selected.Account.ID] = struct{}{}
			continue
		}
		account = selected.Account
		accountReleaseFunc = release

		nativeResp, providerName, err = h.doGrokNativeWebSearch(c.Request.Context(), c, account, req.Query, req.MaxResults)
		if err == nil {
			break
		}
		var failoverErr *service.UpstreamFailoverError
		if !errors.As(err, &failoverErr) || !failoverErr.ShouldRetryNextAccount() {
			break
		}
		failedAccounts[account.ID] = struct{}{}
		if accountReleaseFunc != nil {
			accountReleaseFunc()
			accountReleaseFunc = nil
		}
		account = nil
	}
	if err != nil || nativeResp == nil {
		msg := "web search failed"
		if err != nil {
			msg = err.Error()
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"type": "web_search_error", "message": msg}})
		return
	}
	if account == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
			"type":    "scheduling_error",
			"message": "No available accounts",
		}})
		return
	}

	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	requestPayloadHash := service.HashUsageRequestPayload([]byte(req.Query))
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	// Request IDs are billing idempotency keys, so they must be unique per invocation.
	// Query/IP/UA hashes would collapse repeated identical searches into one charge.
	searchRequestID := "web_search:" + uuid.NewString()
	if apiKey.Group != nil {
		if p := apiKey.Group.GetSearchPricePer1k(); p != nil && *p == 0 {
			logger.L().With(
				zap.String("component", "handler.gateway.web_search"),
				zap.Int64("group_id", apiKey.Group.ID),
			).Info("gateway.web_search.search_price_per_1k_explicit_free")
		}
	}
	h.submitMandatoryUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
			Result: &service.ForwardResult{
				RequestID:   searchRequestID,
				Model:       "grok-web-search",
				SearchCount: 1,
				Duration:    0,
			},
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
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.gateway.web_search"),
				zap.Int64("user_id", apiKey.User.ID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Int64("account_id", account.ID),
			).Error("gateway.web_search.record_usage_failed", zap.Error(err))
		}
	})

	c.JSON(http.StatusOK, gin.H{
		"query":       req.Query,
		"results":     nativeResp.Results,
		"provider":    providerName,
		"max_results": req.MaxResults,
	})
}

// acquireWebSearchAccountSlot resolves an immediate slot or WaitPlan wait.
// On failure returns (nil, false, err); err is non-nil for concurrency acquire
// failures so the first hop can map them to HTTP. Wait-queue full returns
// (nil, false, nil) so failover can try another account.
func (h *GatewayHandler) acquireWebSearchAccountSlot(
	c *gin.Context,
	selected *service.AccountSelectionResult,
) (release func(), ok bool, acquireErr error) {
	if selected == nil || selected.Account == nil {
		return nil, false, nil
	}
	if selected.Acquired {
		return selected.ReleaseFunc, true, nil
	}
	if selected.WaitPlan == nil || h.concurrencyHelper == nil {
		return nil, false, nil
	}
	account := selected.Account
	accountWaitCounted := false
	canWait, waitErr := h.concurrencyHelper.IncrementAccountWaitCount(c.Request.Context(), account.ID, selected.WaitPlan.MaxWaiting)
	if waitErr != nil {
		logger.L().Warn("gateway.web_search.account_wait_counter_increment_failed",
			zap.Int64("account_id", account.ID),
			zap.Error(waitErr),
		)
		// Best-effort wait without counter (same as first-hop legacy path).
	} else if !canWait {
		return nil, false, nil
	} else {
		accountWaitCounted = true
	}
	releaseWait := func() {
		if accountWaitCounted {
			h.concurrencyHelper.DecrementAccountWaitCount(c.Request.Context(), account.ID)
			accountWaitCounted = false
		}
	}
	streamStarted := false
	slotRelease, err := h.concurrencyHelper.AcquireAccountSlotWithWaitTimeout(
		c,
		account.ID,
		selected.WaitPlan.MaxConcurrency,
		selected.WaitPlan.Timeout,
		false,
		&streamStarted,
	)
	releaseWait()
	if err != nil {
		return nil, false, err
	}
	return slotRelease, true, nil
}

// doGrokNativeWebSearch executes web search using the Grok account's native capability
// by calling the responses endpoint with web_search tool, then normalizes sources to unified format.
func (h *GatewayHandler) doGrokNativeWebSearch(ctx context.Context, c *gin.Context, account *service.Account, query string, maxResults int) (*websearch.SearchResponse, string, error) {
	maxResults = normalizeGrokWebSearchMaxResults(maxResults)

	// Build a minimal responses request that triggers Grok web search tool.
	// Ask for structured metadata because xAI action.sources commonly contains URLs only.
	searchBody := map[string]any{
		"model":   xai.DefaultTextModel,
		"input":   buildGrokWebSearchPrompt(query, maxResults),
		"tools":   []map[string]any{{"type": "web_search"}},
		"include": []string{"web_search_call.action.sources"},
		"store":   false,
		"stream":  false,
	}
	bodyBytes, _ := json.Marshal(searchBody)

	respBytes, err := h.gatewayService.DoGrokNativeResponsesJSON(ctx, account, bodyBytes)
	if err != nil {
		return nil, "", err
	}

	// Extract sources from Grok responses output.
	// Prefer web_search_call.action.sources (standardized), fallback to annotations or text links.
	results := extractGrokWebSearchSources(respBytes, maxResults)

	return &websearch.SearchResponse{
		Results: results,
		Query:   query,
	}, "grok-native", nil
}

func normalizeGrokWebSearchMaxResults(maxResults int) int {
	if maxResults <= 0 {
		return defaultGrokWebSearchResults
	}
	if maxResults > maxGrokWebSearchResults {
		return maxGrokWebSearchResults
	}
	return maxResults
}

func buildGrokWebSearchPrompt(query string, maxResults int) string {
	return fmt.Sprintf(`Search the web for the user query below. Return ONLY valid JSON with this exact shape: {"results":[{"url":"https://...","title":"page title","snippet":"concise factual summary"}]}. Return at most %d unique results. Every URL must be an actual web_search source. Populate a non-empty title and snippet for every result. Do not wrap the JSON in markdown.

User query:
%s`, normalizeGrokWebSearchMaxResults(maxResults), query)
}

// extractGrokWebSearchSources returns model-enriched results only when their URLs
// are present in the actual web_search sources, then falls back to raw sources.
func extractGrokWebSearchSources(body []byte, maxResults int) []websearch.SearchResult {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil
	}
	maxResults = normalizeGrokWebSearchMaxResults(maxResults)

	sources := make(map[string]websearch.SearchResult)
	var sourceOrder []string
	addSource := func(rawURL, title, snippet string) {
		key, ok := normalizeGrokWebSearchURL(rawURL)
		if !ok {
			return
		}
		result, exists := sources[key]
		if !exists {
			result.URL = strings.TrimSpace(rawURL)
			sourceOrder = append(sourceOrder, key)
		}
		if result.Title == "" {
			result.Title = usableGrokWebSearchTitle(title, result.URL)
		}
		if result.Snippet == "" {
			result.Snippet = strings.TrimSpace(snippet)
		}
		sources[key] = result
	}

	output := gjson.GetBytes(body, "output")
	output.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "web_search_call" {
			sources := item.Get("action.sources")
			if sources.IsArray() {
				sources.ForEach(func(_, src gjson.Result) bool {
					addSource(src.Get("url").String(), src.Get("title").String(), src.Get("snippet").String())
					return true
				})
			}
		}
		if item.Get("type").String() == "message" {
			item.Get("content").ForEach(func(_, part gjson.Result) bool {
				if part.Get("type").String() != "output_text" {
					return true
				}
				part.Get("annotations").ForEach(func(_, ann gjson.Result) bool {
					if ann.Get("type").String() == "url_citation" || ann.Get("type").String() == "web" {
						addSource(ann.Get("url").String(), ann.Get("title").String(), "")
					}
					return true
				})
				return true
			})
		}
		return true
	})

	var out []websearch.SearchResult
	seen := make(map[string]bool)
	output.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() != "message" {
			return true
		}
		item.Get("content").ForEach(func(_, part gjson.Result) bool {
			if part.Get("type").String() != "output_text" || len(out) >= maxResults {
				return true
			}
			for _, result := range parseGrokWebSearchStructuredResults(part.Get("text").String()) {
				key, ok := normalizeGrokWebSearchURL(result.URL)
				if !ok || seen[key] {
					continue
				}
				source, allowed := sources[key]
				if !allowed {
					continue
				}
				seen[key] = true
				result.URL = source.URL
				result.Title = usableGrokWebSearchTitle(result.Title, result.URL)
				if result.Title == "" {
					result.Title = source.Title
				}
				result.Snippet = strings.TrimSpace(result.Snippet)
				if result.Snippet == "" {
					result.Snippet = source.Snippet
				}
				out = append(out, result)
				if len(out) >= maxResults {
					break
				}
			}
			return true
		})
		return len(out) < maxResults
	})

	for _, key := range sourceOrder {
		if len(out) >= maxResults {
			break
		}
		if seen[key] {
			continue
		}
		result := sources[key]
		if result.Title == "" {
			result.Title = grokWebSearchTitleFromURL(result.URL)
		}
		seen[key] = true
		out = append(out, result)
	}
	return out
}

func parseGrokWebSearchStructuredResults(text string) []websearch.SearchResult {
	text = strings.TrimSpace(text)
	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start < 0 || end < start {
		return nil
	}
	var payload struct {
		Results []websearch.SearchResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &payload); err != nil {
		return nil
	}
	return payload.Results
}

func normalizeGrokWebSearchURL(rawURL string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), true
}

func usableGrokWebSearchTitle(title, rawURL string) string {
	title = strings.TrimSpace(title)
	if title == "" || title == rawURL {
		return ""
	}
	if _, err := strconv.Atoi(title); err == nil {
		return ""
	}
	return title
}

func grokWebSearchTitleFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return strings.TrimPrefix(strings.ToLower(u.Host), "www.")
}

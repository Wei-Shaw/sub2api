package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/gin-gonic/gin"
)

const (
	// ChatGPT internal API for OAuth accounts
	chatgptCodexURL = "https://chatgpt.com/backend-api/codex/responses"
	// OpenAI Platform API for API Key accounts (fallback)
	openaiPlatformAPIURL   = "https://api.openai.com/v1/responses"
	openaiStickySessionTTL = time.Hour // 粘性会话TTL
)

// openaiSSEDataRe matches SSE data lines with optional whitespace after colon.
// Some upstream APIs return non-standard "data:" without space (should be "data: ").
var openaiSSEDataRe = regexp.MustCompile(`^data:\s*`)

// OpenAI allowed headers whitelist (for non-OAuth accounts)
var openaiAllowedHeaders = map[string]bool{
	"accept-language": true,
	"content-type":    true,
	"conversation_id": true,
	"user-agent":      true,
	"originator":      true,
	"session_id":      true,
REDACTED

// OpenAICodexUsageSnapshot represents Codex API usage limits from response headers
type OpenAICodexUsageSnapshot struct {
	PrimaryUsedPercent          *float64 `json:"primary_used_percent,omitempty"`
	PrimaryResetAfterSeconds    *int     `json:"primary_reset_after_seconds,omitempty"`
	PrimaryWindowMinutes        *int     `json:"primary_window_minutes,omitempty"`
	SecondaryUsedPercent        *float64 `json:"secondary_used_percent,omitempty"`
	SecondaryResetAfterSeconds  *int     `json:"secondary_reset_after_seconds,omitempty"`
	SecondaryWindowMinutes      *int     `json:"secondary_window_minutes,omitempty"`
	PrimaryOverSecondaryPercent *float64 `json:"primary_over_secondary_percent,omitempty"`
	UpdatedAt                   string   `json:"updated_at,omitempty"`
REDACTED

// OpenAIUsage represents OpenAI API response usage
type OpenAIUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
REDACTED

// OpenAIForwardResult represents the result of forwarding
type OpenAIForwardResult struct {
	RequestID    string
	Usage        OpenAIUsage
	Model        string
	Stream       bool
	Duration     time.Duration
	FirstTokenMs *int
REDACTED

// OpenAIGatewayService handles OpenAI API gateway operations
type OpenAIGatewayService struct {
	accountRepo         AccountRepository
	usageLogRepo        UsageLogRepository
	userRepo            UserRepository
	userSubRepo         UserSubscriptionRepository
	cache               GatewayCache
	cfg                 *config.Config
	schedulerSnapshot   *SchedulerSnapshotService
	concurrencyService  *ConcurrencyService
	billingService      *BillingService
	rateLimitService    *RateLimitService
	billingCacheService *BillingCacheService
	httpUpstream        HTTPUpstream
	deferredService     *DeferredService
	openAITokenProvider *OpenAITokenProvider
	toolCorrector       *CodexToolCorrector
REDACTED

// NewOpenAIGatewayService creates a new OpenAIGatewayService
func NewOpenAIGatewayService(
	accountRepo AccountRepository,
	usageLogRepo UsageLogRepository,
	userRepo UserRepository,
	userSubRepo UserSubscriptionRepository,
	cache GatewayCache,
	cfg *config.Config,
	schedulerSnapshot *SchedulerSnapshotService,
	concurrencyService *ConcurrencyService,
	billingService *BillingService,
	rateLimitService *RateLimitService,
	billingCacheService *BillingCacheService,
	httpUpstream HTTPUpstream,
	deferredService *DeferredService,
	openAITokenProvider *OpenAITokenProvider,
) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		accountRepo:         accountRepo,
		usageLogRepo:        usageLogRepo,
		userRepo:            userRepo,
		userSubRepo:         userSubRepo,
		cache:               cache,
		cfg:                 cfg,
		schedulerSnapshot:   schedulerSnapshot,
		concurrencyService:  concurrencyService,
		billingService:      billingService,
		rateLimitService:    rateLimitService,
		billingCacheService: billingCacheService,
		httpUpstream:        httpUpstream,
		deferredService:     deferredService,
		openAITokenProvider: openAITokenProvider,
		toolCorrector:       NewCodexToolCorrector(),
REDACTED
REDACTED

// GenerateSessionHash generates session hash from header (OpenAI uses session_id header)
func (s *OpenAIGatewayService) GenerateSessionHash(c *gin.Context) string {
	sessionID := c.GetHeader("session_id")
	if sessionID == "" {
		return ""
REDACTED
	hash := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(hash[:])
REDACTED

// BindStickySession sets session -> account binding with standard TTL.
func (s *OpenAIGatewayService) BindStickySession(ctx context.Context, groupID *int64, sessionHash string, accountID int64) error {
	if sessionHash == "" || accountID <= 0 {
		return nil
REDACTED
	return s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), "openai:"+sessionHash, accountID, openaiStickySessionTTL)
REDACTED

// SelectAccount selects an OpenAI account with sticky session support
func (s *OpenAIGatewayService) SelectAccount(ctx context.Context, groupID *int64, sessionHash string) (*Account, error) {
	return s.SelectAccountForModel(ctx, groupID, sessionHash, "")
REDACTED

// SelectAccountForModel selects an account supporting the requested model
func (s *OpenAIGatewayService) SelectAccountForModel(ctx context.Context, groupID *int64, sessionHash string, requestedModel string) (*Account, error) {
	return s.SelectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, nil)
REDACTED

// SelectAccountForModelWithExclusions selects an account supporting the requested model while excluding specified accounts.
func (s *OpenAIGatewayService) SelectAccountForModelWithExclusions(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{REDACTED) (*Account, error) {
	// 1. Check sticky session
	if sessionHash != "" {
		accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), "openai:"+sessionHash)
		if err == nil && accountID > 0 {
			if _, excluded := excludedIDs[accountID]; !excluded {
				account, err := s.getSchedulableAccount(ctx, accountID)
				if err == nil && account.IsSchedulable() && account.IsOpenAI() && (requestedModel == "" || account.IsModelSupported(requestedModel)) {
					// Refresh sticky session TTL
					_ = s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), "openai:"+sessionHash, openaiStickySessionTTL)
					return account, nil
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// 2. Get schedulable OpenAI accounts
	accounts, err := s.listSchedulableAccounts(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
REDACTED

	// 3. Select by priority + LRU
	var selected *Account
	for i := range accounts {
		acc := &accounts[i]
		if _, excluded := excludedIDs[acc.ID]; excluded {
			continue
	REDACTED
		// Scheduler snapshots can be temporarily stale; re-check schedulability here to
		// avoid selecting accounts that were recently rate-limited/overloaded.
		if !acc.IsSchedulable() {
			continue
	REDACTED
		// Check model support
		if requestedModel != "" && !acc.IsModelSupported(requestedModel) {
			continue
	REDACTED
		if selected == nil {
			selected = acc
			continue
	REDACTED
		// Lower priority value means higher priority
		if acc.Priority < selected.Priority {
			selected = acc
	REDACTED else if acc.Priority == selected.Priority {
			switch {
			case acc.LastUsedAt == nil && selected.LastUsedAt != nil:
				selected = acc
			case acc.LastUsedAt != nil && selected.LastUsedAt == nil:
				// keep selected (never used is preferred)
			case acc.LastUsedAt == nil && selected.LastUsedAt == nil:
				// keep selected (both never used)
			default:
				// Same priority, select least recently used
				if acc.LastUsedAt.Before(*selected.LastUsedAt) {
					selected = acc
			REDACTED
		REDACTED
	REDACTED
REDACTED

	if selected == nil {
		if requestedModel != "" {
			return nil, fmt.Errorf("no available OpenAI accounts supporting model: %s", requestedModel)
	REDACTED
		return nil, errors.New("no available OpenAI accounts")
REDACTED

	// 4. Set sticky session
	if sessionHash != "" {
		_ = s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), "openai:"+sessionHash, selected.ID, openaiStickySessionTTL)
REDACTED

	return selected, nil
REDACTED

// SelectAccountWithLoadAwareness selects an account with load-awareness and wait plan.
func (s *OpenAIGatewayService) SelectAccountWithLoadAwareness(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{REDACTED) (*AccountSelectionResult, error) {
	cfg := s.schedulingConfig()
	var stickyAccountID int64
	if sessionHash != "" && s.cache != nil {
		if accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), "openai:"+sessionHash); err == nil {
			stickyAccountID = accountID
	REDACTED
REDACTED
	if s.concurrencyService == nil || !cfg.LoadBatchEnabled {
		account, err := s.SelectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, excludedIDs)
		if err != nil {
			return nil, err
	REDACTED
		result, err := s.tryAcquireAccountSlot(ctx, account.ID, account.Concurrency)
		if err == nil && result.Acquired {
			return &AccountSelectionResult{
				Account:     account,
				Acquired:    true,
				ReleaseFunc: result.ReleaseFunc,
		REDACTED, nil
	REDACTED
		if stickyAccountID > 0 && stickyAccountID == account.ID && s.concurrencyService != nil {
			waitingCount, _ := s.concurrencyService.GetAccountWaitingCount(ctx, account.ID)
			if waitingCount < cfg.StickySessionMaxWaiting {
				return &AccountSelectionResult{
					Account: account,
					WaitPlan: &AccountWaitPlan{
						AccountID:      account.ID,
						MaxConcurrency: account.Concurrency,
						Timeout:        cfg.StickySessionWaitTimeout,
						MaxWaiting:     cfg.StickySessionMaxWaiting,
				REDACTED,
			REDACTED, nil
		REDACTED
	REDACTED
		return &AccountSelectionResult{
			Account: account,
			WaitPlan: &AccountWaitPlan{
				AccountID:      account.ID,
				MaxConcurrency: account.Concurrency,
				Timeout:        cfg.FallbackWaitTimeout,
				MaxWaiting:     cfg.FallbackMaxWaiting,
		REDACTED,
	REDACTED, nil
REDACTED

	accounts, err := s.listSchedulableAccounts(ctx, groupID)
	if err != nil {
		return nil, err
REDACTED
	if len(accounts) == 0 {
		return nil, errors.New("no available accounts")
REDACTED

	isExcluded := func(accountID int64) bool {
		if excludedIDs == nil {
			return false
	REDACTED
		_, excluded := excludedIDs[accountID]
		return excluded
REDACTED

	// ============ Layer 1: Sticky session ============
	if sessionHash != "" {
		accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), "openai:"+sessionHash)
		if err == nil && accountID > 0 && !isExcluded(accountID) {
			account, err := s.getSchedulableAccount(ctx, accountID)
			if err == nil && account.IsSchedulable() && account.IsOpenAI() &&
				(requestedModel == "" || account.IsModelSupported(requestedModel)) {
				result, err := s.tryAcquireAccountSlot(ctx, accountID, account.Concurrency)
				if err == nil && result.Acquired {
					_ = s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), "openai:"+sessionHash, openaiStickySessionTTL)
					return &AccountSelectionResult{
						Account:     account,
						Acquired:    true,
						ReleaseFunc: result.ReleaseFunc,
				REDACTED, nil
			REDACTED

				waitingCount, _ := s.concurrencyService.GetAccountWaitingCount(ctx, accountID)
				if waitingCount < cfg.StickySessionMaxWaiting {
					return &AccountSelectionResult{
						Account: account,
						WaitPlan: &AccountWaitPlan{
							AccountID:      accountID,
							MaxConcurrency: account.Concurrency,
							Timeout:        cfg.StickySessionWaitTimeout,
							MaxWaiting:     cfg.StickySessionMaxWaiting,
					REDACTED,
				REDACTED, nil
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// ============ Layer 2: Load-aware selection ============
	candidates := make([]*Account, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if isExcluded(acc.ID) {
			continue
	REDACTED
		// Scheduler snapshots can be temporarily stale (bucket rebuild is throttled);
		// re-check schedulability here so recently rate-limited/overloaded accounts
		// are not selected again before the bucket is rebuilt.
		if !acc.IsSchedulable() {
			continue
	REDACTED
		if requestedModel != "" && !acc.IsModelSupported(requestedModel) {
			continue
	REDACTED
		candidates = append(candidates, acc)
REDACTED

	if len(candidates) == 0 {
		return nil, errors.New("no available accounts")
REDACTED

	accountLoads := make([]AccountWithConcurrency, 0, len(candidates))
	for _, acc := range candidates {
		accountLoads = append(accountLoads, AccountWithConcurrency{
			ID:             acc.ID,
			MaxConcurrency: acc.Concurrency,
	REDACTED)
REDACTED

	loadMap, err := s.concurrencyService.GetAccountsLoadBatch(ctx, accountLoads)
	if err != nil {
		ordered := append([]*Account(nil), candidates...)
		sortAccountsByPriorityAndLastUsed(ordered, false)
		for _, acc := range ordered {
			result, err := s.tryAcquireAccountSlot(ctx, acc.ID, acc.Concurrency)
			if err == nil && result.Acquired {
				if sessionHash != "" {
					_ = s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), "openai:"+sessionHash, acc.ID, openaiStickySessionTTL)
			REDACTED
				return &AccountSelectionResult{
					Account:     acc,
					Acquired:    true,
					ReleaseFunc: result.ReleaseFunc,
			REDACTED, nil
		REDACTED
	REDACTED
REDACTED else {
		type accountWithLoad struct {
			account  *Account
			loadInfo *AccountLoadInfo
	REDACTED
		var available []accountWithLoad
		for _, acc := range candidates {
			loadInfo := loadMap[acc.ID]
			if loadInfo == nil {
				loadInfo = &AccountLoadInfo{AccountID: acc.IDREDACTED
		REDACTED
			if loadInfo.LoadRate < 100 {
				available = append(available, accountWithLoad{
					account:  acc,
					loadInfo: loadInfo,
			REDACTED)
		REDACTED
	REDACTED

		if len(available) > 0 {
			sort.SliceStable(available, func(i, j int) bool {
				a, b := available[i], available[j]
				if a.account.Priority != b.account.Priority {
					return a.account.Priority < b.account.Priority
			REDACTED
				if a.loadInfo.LoadRate != b.loadInfo.LoadRate {
					return a.loadInfo.LoadRate < b.loadInfo.LoadRate
			REDACTED
				switch {
				case a.account.LastUsedAt == nil && b.account.LastUsedAt != nil:
					return true
				case a.account.LastUsedAt != nil && b.account.LastUsedAt == nil:
					return false
				case a.account.LastUsedAt == nil && b.account.LastUsedAt == nil:
					return false
				default:
					return a.account.LastUsedAt.Before(*b.account.LastUsedAt)
			REDACTED
		REDACTED)

			for _, item := range available {
				result, err := s.tryAcquireAccountSlot(ctx, item.account.ID, item.account.Concurrency)
				if err == nil && result.Acquired {
					if sessionHash != "" {
						_ = s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), "openai:"+sessionHash, item.account.ID, openaiStickySessionTTL)
				REDACTED
					return &AccountSelectionResult{
						Account:     item.account,
						Acquired:    true,
						ReleaseFunc: result.ReleaseFunc,
				REDACTED, nil
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// ============ Layer 3: Fallback wait ============
	sortAccountsByPriorityAndLastUsed(candidates, false)
	for _, acc := range candidates {
		return &AccountSelectionResult{
			Account: acc,
			WaitPlan: &AccountWaitPlan{
				AccountID:      acc.ID,
				MaxConcurrency: acc.Concurrency,
				Timeout:        cfg.FallbackWaitTimeout,
				MaxWaiting:     cfg.FallbackMaxWaiting,
		REDACTED,
	REDACTED, nil
REDACTED

	return nil, errors.New("no available accounts")
REDACTED

func (s *OpenAIGatewayService) listSchedulableAccounts(ctx context.Context, groupID *int64) ([]Account, error) {
	if s.schedulerSnapshot != nil {
		accounts, _, err := s.schedulerSnapshot.ListSchedulableAccounts(ctx, groupID, PlatformOpenAI, false)
		return accounts, err
REDACTED
	var accounts []Account
	var err error
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, PlatformOpenAI)
REDACTED else if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, PlatformOpenAI)
REDACTED else {
		accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, PlatformOpenAI)
REDACTED
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
REDACTED
	return accounts, nil
REDACTED

func (s *OpenAIGatewayService) tryAcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (*AcquireResult, error) {
	if s.concurrencyService == nil {
		return &AcquireResult{Acquired: true, ReleaseFunc: func() {REDACTEDREDACTED, nil
REDACTED
	return s.concurrencyService.AcquireAccountSlot(ctx, accountID, maxConcurrency)
REDACTED

func (s *OpenAIGatewayService) getSchedulableAccount(ctx context.Context, accountID int64) (*Account, error) {
	if s.schedulerSnapshot != nil {
		return s.schedulerSnapshot.GetAccount(ctx, accountID)
REDACTED
	return s.accountRepo.GetByID(ctx, accountID)
REDACTED

func (s *OpenAIGatewayService) schedulingConfig() config.GatewaySchedulingConfig {
	if s.cfg != nil {
		return s.cfg.Gateway.Scheduling
REDACTED
	return config.GatewaySchedulingConfig{
		StickySessionMaxWaiting:  3,
		StickySessionWaitTimeout: 45 * time.Second,
		FallbackWaitTimeout:      30 * time.Second,
		FallbackMaxWaiting:       100,
		LoadBatchEnabled:         true,
		SlotCleanupInterval:      30 * time.Second,
REDACTED
REDACTED

// GetAccessToken gets the access token for an OpenAI account
func (s *OpenAIGatewayService) GetAccessToken(ctx context.Context, account *Account) (string, string, error) {
	switch account.Type {
	case AccountTypeOAuth:
		// 使用 TokenProvider 获取缓存的 token
		if s.openAITokenProvider != nil {
			accessToken, err := s.openAITokenProvider.GetAccessToken(ctx, account)
			if err != nil {
				return "", "", err
		REDACTED
			return accessToken, "oauth", nil
	REDACTED
		// 降级：TokenProvider 未配置时直接从账号读取
		accessToken := account.GetOpenAIAccessToken()
		if accessToken == "" {
			return "", "", errors.New("access_token not found in credentials")
	REDACTED
		return accessToken, "oauth", nil
	case AccountTypeAPIKey:
		apiKey := account.GetOpenAIApiKey()
		if apiKey == "" {
			return "", "", errors.New("api_key not found in credentials")
	REDACTED
		return apiKey, "apikey", nil
	default:
		return "", "", fmt.Errorf("unsupported account type: %s", account.Type)
REDACTED
REDACTED

func (s *OpenAIGatewayService) shouldFailoverUpstreamError(statusCode int) bool {
	switch statusCode {
	case 401, 402, 403, 429, 529:
		return true
	default:
		return statusCode >= 500
REDACTED
REDACTED

func (s *OpenAIGatewayService) handleFailoverSideEffects(ctx context.Context, resp *http.Response, account *Account) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
REDACTED

// Forward forwards request to OpenAI API
func (s *OpenAIGatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	// Parse request body once (avoid multiple parse/serialize cycles)
	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return nil, fmt.Errorf("parse request: %w", err)
REDACTED

	// Extract model and stream from parsed body
	reqModel, _ := reqBody["model"].(string)
	reqStream, _ := reqBody["stream"].(bool)
	promptCacheKey := ""
	if v, ok := reqBody["prompt_cache_key"].(string); ok {
		promptCacheKey = strings.TrimSpace(v)
REDACTED

	// Track if body needs re-serialization
	bodyModified := false
	originalModel := reqModel

	isCodexCLI := openai.IsCodexCLIRequest(c.GetHeader("User-Agent"))

	// 对所有请求执行模型映射（包含 Codex CLI）。
	mappedModel := account.GetMappedModel(reqModel)
	if mappedModel != reqModel {
		log.Printf("[OpenAI] Model mapping applied: %s -> %s (account: %s, isCodexCLI: %v)", reqModel, mappedModel, account.Name, isCodexCLI)
		reqBody["model"] = mappedModel
		bodyModified = true
REDACTED

	// 针对所有 OpenAI 账号执行 Codex 模型名规范化，确保上游识别一致。
	if model, ok := reqBody["model"].(string); ok {
		normalizedModel := normalizeCodexModel(model)
		if normalizedModel != "" && normalizedModel != model {
			log.Printf("[OpenAI] Codex model normalization: %s -> %s (account: %s, type: %s, isCodexCLI: %v)",
				model, normalizedModel, account.Name, account.Type, isCodexCLI)
			reqBody["model"] = normalizedModel
			mappedModel = normalizedModel
			bodyModified = true
	REDACTED
REDACTED

	// 规范化 reasoning.effort 参数（minimal -> none），与上游允许值对齐。
	if reasoning, ok := reqBody["reasoning"].(map[string]any); ok {
		if effort, ok := reasoning["effort"].(string); ok && effort == "minimal" {
			reasoning["effort"] = "none"
			bodyModified = true
			log.Printf("[OpenAI] Normalized reasoning.effort: minimal -> none (account: %s)", account.Name)
	REDACTED
REDACTED

	if account.Type == AccountTypeOAuth && !isCodexCLI {
		codexResult := applyCodexOAuthTransform(reqBody)
		if codexResult.Modified {
			bodyModified = true
	REDACTED
		if codexResult.NormalizedModel != "" {
			mappedModel = codexResult.NormalizedModel
	REDACTED
		if codexResult.PromptCacheKey != "" {
			promptCacheKey = codexResult.PromptCacheKey
	REDACTED
REDACTED

	// Handle max_output_tokens based on platform and account type
	if !isCodexCLI {
		if maxOutputTokens, hasMaxOutputTokens := reqBody["max_output_tokens"]; hasMaxOutputTokens {
			switch account.Platform {
			case PlatformOpenAI:
				// For OpenAI API Key, remove max_output_tokens (not supported)
				// For OpenAI OAuth (Responses API), keep it (supported)
				if account.Type == AccountTypeAPIKey {
					delete(reqBody, "max_output_tokens")
					bodyModified = true
			REDACTED
			case PlatformAnthropic:
				// For Anthropic (Claude), convert to max_tokens
				delete(reqBody, "max_output_tokens")
				if _, hasMaxTokens := reqBody["max_tokens"]; !hasMaxTokens {
					reqBody["max_tokens"] = maxOutputTokens
			REDACTED
				bodyModified = true
			case PlatformGemini:
				// For Gemini, remove (will be handled by Gemini-specific transform)
				delete(reqBody, "max_output_tokens")
				bodyModified = true
			default:
				// For unknown platforms, remove to be safe
				delete(reqBody, "max_output_tokens")
				bodyModified = true
		REDACTED
	REDACTED

		// Also handle max_completion_tokens (similar logic)
		if _, hasMaxCompletionTokens := reqBody["max_completion_tokens"]; hasMaxCompletionTokens {
			if account.Type == AccountTypeAPIKey || account.Platform != PlatformOpenAI {
				delete(reqBody, "max_completion_tokens")
				bodyModified = true
		REDACTED
	REDACTED

		// Remove prompt_cache_retention (not supported by upstream OpenAI API)
		if _, has := reqBody["prompt_cache_retention"]; has {
			delete(reqBody, "prompt_cache_retention")
			bodyModified = true
	REDACTED
REDACTED

	// Re-serialize body only if modified
	if bodyModified {
		var err error
		body, err = json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("serialize request body: %w", err)
	REDACTED
REDACTED

	// Get access token
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED

	// Build upstream request
	upstreamReq, err := s.buildUpstreamRequest(ctx, c, account, body, token, reqStream, promptCacheKey, isCodexCLI)
	if err != nil {
		return nil, err
REDACTED

	// Get proxy URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	// Capture upstream request body for ops retry of this attempt.
	if c != nil {
		c.Set(OpsUpstreamRequestBodyKey, string(body))
REDACTED

	// Send request
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		// Ensure the client receives an error response (handlers assume Forward writes on non-failover errors).
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
	REDACTED)
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"type":    "upstream_error",
				"message": "Upstream request failed",
		REDACTED,
	REDACTED)
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	// Handle error response
	if resp.StatusCode >= 400 {
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(respBody))

			upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
			upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
			upstreamDetail := ""
			if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
				maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
				if maxBytes <= 0 {
					maxBytes = 2048
			REDACTED
				upstreamDetail = truncateString(string(respBody), maxBytes)
		REDACTED
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
		REDACTED)

			s.handleFailoverSideEffects(ctx, resp, account)
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCodeREDACTED
	REDACTED
		return s.handleErrorResponse(ctx, resp, c, account)
REDACTED

	// Handle normal response
	var usage *OpenAIUsage
	var firstTokenMs *int
	if reqStream {
		streamResult, err := s.handleStreamingResponse(ctx, resp, c, account, startTime, originalModel, mappedModel)
		if err != nil {
			return nil, err
	REDACTED
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
REDACTED else {
		usage, err = s.handleNonStreamingResponse(ctx, resp, c, account, originalModel, mappedModel)
		if err != nil {
			return nil, err
	REDACTED
REDACTED

	// Extract and save Codex usage snapshot from response headers (for OAuth accounts)
	if account.Type == AccountTypeOAuth {
		if snapshot := extractCodexUsageHeaders(resp.Header); snapshot != nil {
			s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
	REDACTED
REDACTED

	return &OpenAIForwardResult{
		RequestID:    resp.Header.Get("x-request-id"),
		Usage:        *usage,
		Model:        originalModel,
		Stream:       reqStream,
		Duration:     time.Since(startTime),
		FirstTokenMs: firstTokenMs,
REDACTED, nil
REDACTED

func (s *OpenAIGatewayService) buildUpstreamRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token string, isStream bool, promptCacheKey string, isCodexCLI bool) (*http.Request, error) {
	// Determine target URL based on account type
	var targetURL string
	switch account.Type {
	case AccountTypeOAuth:
		// OAuth accounts use ChatGPT internal API
		targetURL = chatgptCodexURL
	case AccountTypeAPIKey:
		// API Key accounts use Platform API or custom base URL
		baseURL := account.GetOpenAIBaseURL()
		if baseURL == "" {
			targetURL = openaiPlatformAPIURL
	REDACTED else {
			validatedURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, err
		REDACTED
			targetURL = validatedURL + "/responses"
	REDACTED
	default:
		targetURL = openaiPlatformAPIURL
REDACTED

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
REDACTED

	// Set authentication header
	req.Header.Set("authorization", "Bearer "+token)

	// Set headers specific to OAuth accounts (ChatGPT internal API)
	if account.Type == AccountTypeOAuth {
		// Required: set Host for ChatGPT API (must use req.Host, not Header.Set)
		req.Host = "chatgpt.com"
		// Required: set chatgpt-account-id header
		chatgptAccountID := account.GetChatGPTAccountID()
		if chatgptAccountID != "" {
			req.Header.Set("chatgpt-account-id", chatgptAccountID)
	REDACTED
REDACTED

	// Whitelist passthrough headers
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if openaiAllowedHeaders[lowerKey] {
			for _, v := range values {
				req.Header.Add(key, v)
		REDACTED
	REDACTED
REDACTED
	if account.Type == AccountTypeOAuth {
		req.Header.Set("OpenAI-Beta", "responses=experimental")
		if isCodexCLI {
			req.Header.Set("originator", "codex_cli_rs")
	REDACTED else {
			req.Header.Set("originator", "opencode")
	REDACTED
		req.Header.Set("accept", "text/event-stream")
		if promptCacheKey != "" {
			req.Header.Set("conversation_id", promptCacheKey)
			req.Header.Set("session_id", promptCacheKey)
	REDACTED
REDACTED

	// Apply custom User-Agent if configured
	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("user-agent", customUA)
REDACTED

	// Ensure required headers exist
	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
REDACTED

	return req, nil
REDACTED

func (s *OpenAIGatewayService) handleErrorResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account) (*OpenAIForwardResult, error) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(body))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
	REDACTED
		upstreamDetail = truncateString(string(body), maxBytes)
REDACTED
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)

	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		log.Printf(
			"OpenAI upstream error %d (account=%d platform=%s type=%s): %s",
			resp.StatusCode,
			account.ID,
			account.Platform,
			account.Type,
			truncateForLog(body, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes),
		)
REDACTED

	// Check custom error codes
	if !account.ShouldHandleErrorCode(resp.StatusCode) {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			Kind:               "http_error",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
	REDACTED)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "upstream_error",
				"message": "Upstream gateway error",
		REDACTED,
	REDACTED)
		if upstreamMsg == "" {
			return nil, fmt.Errorf("upstream error: %d (not in custom error codes)", resp.StatusCode)
	REDACTED
		return nil, fmt.Errorf("upstream error: %d (not in custom error codes) message=%s", resp.StatusCode, upstreamMsg)
REDACTED

	// Handle upstream error (mark account status)
	shouldDisable := false
	if s.rateLimitService != nil {
		shouldDisable = s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
REDACTED
	kind := "http_error"
	if shouldDisable {
		kind = "failover"
REDACTED
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  resp.Header.Get("x-request-id"),
		Kind:               kind,
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
REDACTED)
	if shouldDisable {
		return nil, &UpstreamFailoverError{StatusCode: resp.StatusCodeREDACTED
REDACTED

	// Return appropriate error response
	var errType, errMsg string
	var statusCode int

	switch resp.StatusCode {
	case 401:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream authentication failed, please contact administrator"
	case 402:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream payment required: insufficient balance or billing issue"
	case 403:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream access forbidden, please contact administrator"
	case 429:
		statusCode = http.StatusTooManyRequests
		errType = "rate_limit_error"
		errMsg = "Upstream rate limit exceeded, please retry later"
	default:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream request failed"
REDACTED

	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": errMsg,
	REDACTED,
REDACTED)

	if upstreamMsg == "" {
		return nil, fmt.Errorf("upstream error: %d", resp.StatusCode)
REDACTED
	return nil, fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
REDACTED

// openaiStreamingResult streaming response result
type openaiStreamingResult struct {
	usage        *OpenAIUsage
	firstTokenMs *int
REDACTED

func (s *OpenAIGatewayService) handleStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, startTime time.Time, originalModel, mappedModel string) (*openaiStreamingResult, error) {
	if s.cfg != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.cfg.Security.ResponseHeaders)
REDACTED

	// Set SSE response headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Pass through other headers
	if v := resp.Header.Get("x-request-id"); v != "" {
		c.Header("x-request-id", v)
REDACTED

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
REDACTED

	usage := &OpenAIUsage{REDACTED
	var firstTokenMs *int
	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
REDACTED
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)

	type scanEvent struct {
		line string
		err  error
REDACTED
	// 独立 goroutine 读取上游，避免读取阻塞影响 keepalive/超时处理
	events := make(chan scanEvent, 16)
	done := make(chan struct{REDACTED)
	sendEvent := func(ev scanEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
	REDACTED
REDACTED
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	go func() {
		defer close(events)
		for scanner.Scan() {
			atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			if !sendEvent(scanEvent{line: scanner.Text()REDACTED) {
				return
		REDACTED
	REDACTED
		if err := scanner.Err(); err != nil {
			_ = sendEvent(scanEvent{err: errREDACTED)
	REDACTED
REDACTED()
	defer close(done)

	streamInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
REDACTED
	// 仅监控上游数据间隔超时，不被下游写入阻塞影响
	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
REDACTED
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
REDACTED

	keepaliveInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamKeepaliveInterval > 0 {
		keepaliveInterval = time.Duration(s.cfg.Gateway.StreamKeepaliveInterval) * time.Second
REDACTED
	// 下游 keepalive 仅用于防止代理空闲断开
	var keepaliveTicker *time.Ticker
	if keepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(keepaliveInterval)
		defer keepaliveTicker.Stop()
REDACTED
	var keepaliveCh <-chan time.Time
	if keepaliveTicker != nil {
		keepaliveCh = keepaliveTicker.C
REDACTED
	// 记录上次收到上游数据的时间，用于控制 keepalive 发送频率
	lastDataAt := time.Now()

	// 仅发送一次错误事件，避免多次写入导致协议混乱（写失败时尽力通知客户端）
	errorEventSent := false
	sendErrorEvent := func(reason string) {
		if errorEventSent {
			return
	REDACTED
		errorEventSent = true
		_, _ = fmt.Fprintf(w, "event: error\ndata: {\"error\":\"%s\"REDACTED\n\n", reason)
		flusher.Flush()
REDACTED

	needModelReplace := originalModel != mappedModel

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return &openaiStreamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, nil
		REDACTED
			if ev.err != nil {
				if errors.Is(ev.err, bufio.ErrTooLong) {
					log.Printf("SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, ev.err)
					sendErrorEvent("response_too_large")
					return &openaiStreamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, ev.err
			REDACTED
				sendErrorEvent("stream_read_error")
				return &openaiStreamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, fmt.Errorf("stream read error: %w", ev.err)
		REDACTED

			line := ev.line
			lastDataAt = time.Now()

			// Extract data from SSE line (supports both "data: " and "data:" formats)
			if openaiSSEDataRe.MatchString(line) {
				data := openaiSSEDataRe.ReplaceAllString(line, "")

				// Replace model in response if needed
				if needModelReplace {
					line = s.replaceModelInSSELine(line, mappedModel, originalModel)
			REDACTED

				// Correct Codex tool calls if needed (apply_patch -> edit, etc.)
				if correctedData, corrected := s.toolCorrector.CorrectToolCallsInSSEData(data); corrected {
					line = "data: " + correctedData
			REDACTED

				// Forward line
				if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
					sendErrorEvent("write_failed")
					return &openaiStreamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, err
			REDACTED
				flusher.Flush()

				// Record first token time
				if firstTokenMs == nil && data != "" && data != "[DONE]" {
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
			REDACTED
				s.parseSSEUsage(data, usage)
		REDACTED else {
				// Forward non-data lines as-is
				if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
					sendErrorEvent("write_failed")
					return &openaiStreamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, err
			REDACTED
				flusher.Flush()
		REDACTED

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
		REDACTED
			log.Printf("Stream data interval timeout: account=%d model=%s interval=%s", account.ID, originalModel, streamInterval)
			// 处理流超时，可能标记账户为临时不可调度或错误状态
			if s.rateLimitService != nil {
				s.rateLimitService.HandleStreamTimeout(ctx, account, originalModel)
		REDACTED
			sendErrorEvent("stream_timeout")
			return &openaiStreamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, fmt.Errorf("stream data interval timeout")

		case <-keepaliveCh:
			if time.Since(lastDataAt) < keepaliveInterval {
				continue
		REDACTED
			if _, err := fmt.Fprint(w, ":\n\n"); err != nil {
				return &openaiStreamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, err
		REDACTED
			flusher.Flush()
	REDACTED
REDACTED

REDACTED

func (s *OpenAIGatewayService) replaceModelInSSELine(line, fromModel, toModel string) string {
	if !openaiSSEDataRe.MatchString(line) {
		return line
REDACTED
	data := openaiSSEDataRe.ReplaceAllString(line, "")
	if data == "" || data == "[DONE]" {
		return line
REDACTED

	var event map[string]any
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return line
REDACTED

	// Replace model in response
	if m, ok := event["model"].(string); ok && m == fromModel {
		event["model"] = toModel
		newData, err := json.Marshal(event)
		if err != nil {
			return line
	REDACTED
		return "data: " + string(newData)
REDACTED

	// Check nested response
	if response, ok := event["response"].(map[string]any); ok {
		if m, ok := response["model"].(string); ok && m == fromModel {
			response["model"] = toModel
			newData, err := json.Marshal(event)
			if err != nil {
				return line
		REDACTED
			return "data: " + string(newData)
	REDACTED
REDACTED

	return line
REDACTED

// correctToolCallsInResponseBody 修正响应体中的工具调用
func (s *OpenAIGatewayService) correctToolCallsInResponseBody(body []byte) []byte {
	if len(body) == 0 {
		return body
REDACTED

	bodyStr := string(body)
	corrected, changed := s.toolCorrector.CorrectToolCallsInSSEData(bodyStr)
	if changed {
		return []byte(corrected)
REDACTED
	return body
REDACTED

func (s *OpenAIGatewayService) parseSSEUsage(data string, usage *OpenAIUsage) {
	// Parse response.completed event for usage (OpenAI Responses format)
	var event struct {
		Type     string `json:"type"`
		Response struct {
			Usage struct {
				InputTokens       int `json:"input_tokens"`
				OutputTokens      int `json:"output_tokens"`
				InputTokenDetails struct {
					CachedTokens int `json:"cached_tokens"`
			REDACTED `json:"input_tokens_details"`
		REDACTED `json:"usage"`
	REDACTED `json:"response"`
REDACTED

	if json.Unmarshal([]byte(data), &event) == nil && event.Type == "response.completed" {
		usage.InputTokens = event.Response.Usage.InputTokens
		usage.OutputTokens = event.Response.Usage.OutputTokens
		usage.CacheReadInputTokens = event.Response.Usage.InputTokenDetails.CachedTokens
REDACTED
REDACTED

func (s *OpenAIGatewayService) handleNonStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, originalModel, mappedModel string) (*OpenAIUsage, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
REDACTED

	if account.Type == AccountTypeOAuth {
		bodyLooksLikeSSE := bytes.Contains(body, []byte("data:")) || bytes.Contains(body, []byte("event:"))
		if isEventStreamResponse(resp.Header) || bodyLooksLikeSSE {
			return s.handleOAuthSSEToJSON(resp, c, body, originalModel, mappedModel)
	REDACTED
REDACTED

	// Parse usage
	var response struct {
		Usage struct {
			InputTokens       int `json:"input_tokens"`
			OutputTokens      int `json:"output_tokens"`
			InputTokenDetails struct {
				CachedTokens int `json:"cached_tokens"`
		REDACTED `json:"input_tokens_details"`
	REDACTED `json:"usage"`
REDACTED
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
REDACTED

	usage := &OpenAIUsage{
		InputTokens:          response.Usage.InputTokens,
		OutputTokens:         response.Usage.OutputTokens,
		CacheReadInputTokens: response.Usage.InputTokenDetails.CachedTokens,
REDACTED

	// Replace model in response if needed
	if originalModel != mappedModel {
		body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
REDACTED

	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.cfg.Security.ResponseHeaders)

	contentType := "application/json"
	if s.cfg != nil && !s.cfg.Security.ResponseHeaders.Enabled {
		if upstreamType := resp.Header.Get("Content-Type"); upstreamType != "" {
			contentType = upstreamType
	REDACTED
REDACTED

	c.Data(resp.StatusCode, contentType, body)

	return usage, nil
REDACTED

func isEventStreamResponse(header http.Header) bool {
	contentType := strings.ToLower(header.Get("Content-Type"))
	return strings.Contains(contentType, "text/event-stream")
REDACTED

func (s *OpenAIGatewayService) handleOAuthSSEToJSON(resp *http.Response, c *gin.Context, body []byte, originalModel, mappedModel string) (*OpenAIUsage, error) {
	bodyText := string(body)
	finalResponse, ok := extractCodexFinalResponse(bodyText)

	usage := &OpenAIUsage{REDACTED
	if ok {
		var response struct {
			Usage struct {
				InputTokens       int `json:"input_tokens"`
				OutputTokens      int `json:"output_tokens"`
				InputTokenDetails struct {
					CachedTokens int `json:"cached_tokens"`
			REDACTED `json:"input_tokens_details"`
		REDACTED `json:"usage"`
	REDACTED
		if err := json.Unmarshal(finalResponse, &response); err == nil {
			usage.InputTokens = response.Usage.InputTokens
			usage.OutputTokens = response.Usage.OutputTokens
			usage.CacheReadInputTokens = response.Usage.InputTokenDetails.CachedTokens
	REDACTED
		body = finalResponse
		if originalModel != mappedModel {
			body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
	REDACTED
		// Correct tool calls in final response
		body = s.correctToolCallsInResponseBody(body)
REDACTED else {
		usage = s.parseSSEUsageFromBody(bodyText)
		if originalModel != mappedModel {
			bodyText = s.replaceModelInSSEBody(bodyText, mappedModel, originalModel)
	REDACTED
		body = []byte(bodyText)
REDACTED

	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.cfg.Security.ResponseHeaders)

	contentType := "application/json; charset=utf-8"
	if !ok {
		contentType = resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "text/event-stream"
	REDACTED
REDACTED
	c.Data(resp.StatusCode, contentType, body)

	return usage, nil
REDACTED

func extractCodexFinalResponse(body string) ([]byte, bool) {
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		if !openaiSSEDataRe.MatchString(line) {
			continue
	REDACTED
		data := openaiSSEDataRe.ReplaceAllString(line, "")
		if data == "" || data == "[DONE]" {
			continue
	REDACTED
		var event struct {
			Type     string          `json:"type"`
			Response json.RawMessage `json:"response"`
	REDACTED
		if json.Unmarshal([]byte(data), &event) != nil {
			continue
	REDACTED
		if event.Type == "response.done" || event.Type == "response.completed" {
			if len(event.Response) > 0 {
				return event.Response, true
		REDACTED
	REDACTED
REDACTED
	return nil, false
REDACTED

func (s *OpenAIGatewayService) parseSSEUsageFromBody(body string) *OpenAIUsage {
	usage := &OpenAIUsage{REDACTED
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		if !openaiSSEDataRe.MatchString(line) {
			continue
	REDACTED
		data := openaiSSEDataRe.ReplaceAllString(line, "")
		if data == "" || data == "[DONE]" {
			continue
	REDACTED
		s.parseSSEUsage(data, usage)
REDACTED
	return usage
REDACTED

func (s *OpenAIGatewayService) replaceModelInSSEBody(body, fromModel, toModel string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if !openaiSSEDataRe.MatchString(line) {
			continue
	REDACTED
		lines[i] = s.replaceModelInSSELine(line, fromModel, toModel)
REDACTED
	return strings.Join(lines, "\n")
REDACTED

func (s *OpenAIGatewayService) validateUpstreamBaseURL(raw string) (string, error) {
	if s.cfg != nil && !s.cfg.Security.URLAllowlist.Enabled {
		normalized, err := urlvalidator.ValidateURLFormat(raw, s.cfg.Security.URLAllowlist.AllowInsecureHTTP)
		if err != nil {
			return "", fmt.Errorf("invalid base_url: %w", err)
	REDACTED
		return normalized, nil
REDACTED
	normalized, err := urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     s.cfg.Security.URLAllowlist.UpstreamHosts,
		RequireAllowlist: true,
		AllowPrivate:     s.cfg.Security.URLAllowlist.AllowPrivateHosts,
REDACTED)
	if err != nil {
		return "", fmt.Errorf("invalid base_url: %w", err)
REDACTED
	return normalized, nil
REDACTED

func (s *OpenAIGatewayService) replaceModelInResponseBody(body []byte, fromModel, toModel string) []byte {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return body
REDACTED

	model, ok := resp["model"].(string)
	if !ok || model != fromModel {
		return body
REDACTED

	resp["model"] = toModel
	newBody, err := json.Marshal(resp)
	if err != nil {
		return body
REDACTED

	return newBody
REDACTED

// OpenAIRecordUsageInput input for recording usage
type OpenAIRecordUsageInput struct {
	Result       *OpenAIForwardResult
	APIKey       *APIKey
	User         *User
	Account      *Account
	Subscription *UserSubscription
	UserAgent    string // 请求的 User-Agent
	IPAddress    string // 请求的客户端 IP 地址
REDACTED

// RecordUsage records usage and deducts balance
func (s *OpenAIGatewayService) RecordUsage(ctx context.Context, input *OpenAIRecordUsageInput) error {
	result := input.Result
	apiKey := input.APIKey
	user := input.User
	account := input.Account
	subscription := input.Subscription

	// 计算实际的新输入token（减去缓存读取的token）
	// 因为 input_tokens 包含了 cache_read_tokens，而缓存读取的token不应按输入价格计费
	actualInputTokens := result.Usage.InputTokens - result.Usage.CacheReadInputTokens
	if actualInputTokens < 0 {
		actualInputTokens = 0
REDACTED

	// Calculate cost
	tokens := UsageTokens{
		InputTokens:         actualInputTokens,
		OutputTokens:        result.Usage.OutputTokens,
		CacheCreationTokens: result.Usage.CacheCreationInputTokens,
		CacheReadTokens:     result.Usage.CacheReadInputTokens,
REDACTED

	// Get rate multiplier
	multiplier := s.cfg.Default.RateMultiplier
	if apiKey.GroupID != nil && apiKey.Group != nil {
		multiplier = apiKey.Group.RateMultiplier
REDACTED

	cost, err := s.billingService.CalculateCost(result.Model, tokens, multiplier)
	if err != nil {
		cost = &CostBreakdown{ActualCost: 0REDACTED
REDACTED

	// Determine billing type
	isSubscriptionBilling := subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
	billingType := BillingTypeBalance
	if isSubscriptionBilling {
		billingType = BillingTypeSubscription
REDACTED

	// Create usage log
	durationMs := int(result.Duration.Milliseconds())
	accountRateMultiplier := account.BillingRateMultiplier()
	usageLog := &UsageLog{
		UserID:                user.ID,
		APIKeyID:              apiKey.ID,
		AccountID:             account.ID,
		RequestID:             result.RequestID,
		Model:                 result.Model,
		InputTokens:           actualInputTokens,
		OutputTokens:          result.Usage.OutputTokens,
		CacheCreationTokens:   result.Usage.CacheCreationInputTokens,
		CacheReadTokens:       result.Usage.CacheReadInputTokens,
		InputCost:             cost.InputCost,
		OutputCost:            cost.OutputCost,
		CacheCreationCost:     cost.CacheCreationCost,
		CacheReadCost:         cost.CacheReadCost,
		TotalCost:             cost.TotalCost,
		ActualCost:            cost.ActualCost,
		RateMultiplier:        multiplier,
		AccountRateMultiplier: &accountRateMultiplier,
		BillingType:           billingType,
		Stream:                result.Stream,
		DurationMs:            &durationMs,
		FirstTokenMs:          result.FirstTokenMs,
		CreatedAt:             time.Now(),
REDACTED

	// 添加 UserAgent
	if input.UserAgent != "" {
		usageLog.UserAgent = &input.UserAgent
REDACTED

	// 添加 IPAddress
	if input.IPAddress != "" {
		usageLog.IPAddress = &input.IPAddress
REDACTED

	if apiKey.GroupID != nil {
		usageLog.GroupID = apiKey.GroupID
REDACTED
	if subscription != nil {
		usageLog.SubscriptionID = &subscription.ID
REDACTED

	inserted, err := s.usageLogRepo.Create(ctx, usageLog)
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		log.Printf("[SIMPLE MODE] Usage recorded (not billed): user=%d, tokens=%d", usageLog.UserID, usageLog.TotalTokens())
		s.deferredService.ScheduleLastUsedUpdate(account.ID)
		return nil
REDACTED

	shouldBill := inserted || err != nil

	// Deduct based on billing type
	if isSubscriptionBilling {
		if shouldBill && cost.TotalCost > 0 {
			_ = s.userSubRepo.IncrementUsage(ctx, subscription.ID, cost.TotalCost)
			s.billingCacheService.QueueUpdateSubscriptionUsage(user.ID, *apiKey.GroupID, cost.TotalCost)
	REDACTED
REDACTED else {
		if shouldBill && cost.ActualCost > 0 {
			_ = s.userRepo.DeductBalance(ctx, user.ID, cost.ActualCost)
			s.billingCacheService.QueueDeductBalance(user.ID, cost.ActualCost)
	REDACTED
REDACTED

	// Schedule batch update for account last_used_at
	s.deferredService.ScheduleLastUsedUpdate(account.ID)

	return nil
REDACTED

// extractCodexUsageHeaders extracts Codex usage limits from response headers
func extractCodexUsageHeaders(headers http.Header) *OpenAICodexUsageSnapshot {
	snapshot := &OpenAICodexUsageSnapshot{REDACTED
	hasData := false

	// Helper to parse float64 from header
	parseFloat := func(key string) *float64 {
		if v := headers.Get(key); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return &f
		REDACTED
	REDACTED
		return nil
REDACTED

	// Helper to parse int from header
	parseInt := func(key string) *int {
		if v := headers.Get(key); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				return &i
		REDACTED
	REDACTED
		return nil
REDACTED

	// Primary (weekly) limits
	if v := parseFloat("x-codex-primary-used-percent"); v != nil {
		snapshot.PrimaryUsedPercent = v
		hasData = true
REDACTED
	if v := parseInt("x-codex-primary-reset-after-seconds"); v != nil {
		snapshot.PrimaryResetAfterSeconds = v
		hasData = true
REDACTED
	if v := parseInt("x-codex-primary-window-minutes"); v != nil {
		snapshot.PrimaryWindowMinutes = v
		hasData = true
REDACTED

	// Secondary (5h) limits
	if v := parseFloat("x-codex-secondary-used-percent"); v != nil {
		snapshot.SecondaryUsedPercent = v
		hasData = true
REDACTED
	if v := parseInt("x-codex-secondary-reset-after-seconds"); v != nil {
		snapshot.SecondaryResetAfterSeconds = v
		hasData = true
REDACTED
	if v := parseInt("x-codex-secondary-window-minutes"); v != nil {
		snapshot.SecondaryWindowMinutes = v
		hasData = true
REDACTED

	// Overflow ratio
	if v := parseFloat("x-codex-primary-over-secondary-limit-percent"); v != nil {
		snapshot.PrimaryOverSecondaryPercent = v
		hasData = true
REDACTED

	if !hasData {
		return nil
REDACTED

	snapshot.UpdatedAt = time.Now().Format(time.RFC3339)
	return snapshot
REDACTED

// updateCodexUsageSnapshot saves the Codex usage snapshot to account's Extra field
func (s *OpenAIGatewayService) updateCodexUsageSnapshot(ctx context.Context, accountID int64, snapshot *OpenAICodexUsageSnapshot) {
	if snapshot == nil {
		return
REDACTED

	// Convert snapshot to map for merging into Extra
	updates := make(map[string]any)
	if snapshot.PrimaryUsedPercent != nil {
		updates["codex_primary_used_percent"] = *snapshot.PrimaryUsedPercent
REDACTED
	if snapshot.PrimaryResetAfterSeconds != nil {
		updates["codex_primary_reset_after_seconds"] = *snapshot.PrimaryResetAfterSeconds
REDACTED
	if snapshot.PrimaryWindowMinutes != nil {
		updates["codex_primary_window_minutes"] = *snapshot.PrimaryWindowMinutes
REDACTED
	if snapshot.SecondaryUsedPercent != nil {
		updates["codex_secondary_used_percent"] = *snapshot.SecondaryUsedPercent
REDACTED
	if snapshot.SecondaryResetAfterSeconds != nil {
		updates["codex_secondary_reset_after_seconds"] = *snapshot.SecondaryResetAfterSeconds
REDACTED
	if snapshot.SecondaryWindowMinutes != nil {
		updates["codex_secondary_window_minutes"] = *snapshot.SecondaryWindowMinutes
REDACTED
	if snapshot.PrimaryOverSecondaryPercent != nil {
		updates["codex_primary_over_secondary_percent"] = *snapshot.PrimaryOverSecondaryPercent
REDACTED
	updates["codex_usage_updated_at"] = snapshot.UpdatedAt

	// Normalize to canonical 5h/7d fields based on window_minutes
	// This fixes the issue where OpenAI's primary/secondary naming is reversed
	// Strategy: Compare the two windows and assign the smaller one to 5h, larger one to 7d

	// IMPORTANT: We can only reliably determine window type from window_minutes field
	// The reset_after_seconds is remaining time, not window size, so it cannot be used for comparison

	var primaryWindowMins, secondaryWindowMins int
	var hasPrimaryWindow, hasSecondaryWindow bool

	// Only use window_minutes for reliable window size comparison
	if snapshot.PrimaryWindowMinutes != nil {
		primaryWindowMins = *snapshot.PrimaryWindowMinutes
		hasPrimaryWindow = true
REDACTED

	if snapshot.SecondaryWindowMinutes != nil {
		secondaryWindowMins = *snapshot.SecondaryWindowMinutes
		hasSecondaryWindow = true
REDACTED

	// Determine which is 5h and which is 7d
	var use5hFromPrimary, use7dFromPrimary bool
	var use5hFromSecondary, use7dFromSecondary bool

	if hasPrimaryWindow && hasSecondaryWindow {
		// Both window sizes known: compare and assign smaller to 5h, larger to 7d
		if primaryWindowMins < secondaryWindowMins {
			use5hFromPrimary = true
			use7dFromSecondary = true
	REDACTED else {
			use5hFromSecondary = true
			use7dFromPrimary = true
	REDACTED
REDACTED else if hasPrimaryWindow {
		// Only primary window size known: classify by absolute threshold
		if primaryWindowMins <= 360 {
			use5hFromPrimary = true
	REDACTED else {
			use7dFromPrimary = true
	REDACTED
REDACTED else if hasSecondaryWindow {
		// Only secondary window size known: classify by absolute threshold
		if secondaryWindowMins <= 360 {
			use5hFromSecondary = true
	REDACTED else {
			use7dFromSecondary = true
	REDACTED
REDACTED else {
		// No window_minutes available: cannot reliably determine window types
		// Fall back to legacy assumption (may be incorrect)
		// Assume primary=7d, secondary=5h based on historical observation
		if snapshot.SecondaryUsedPercent != nil || snapshot.SecondaryResetAfterSeconds != nil || snapshot.SecondaryWindowMinutes != nil {
			use5hFromSecondary = true
	REDACTED
		if snapshot.PrimaryUsedPercent != nil || snapshot.PrimaryResetAfterSeconds != nil || snapshot.PrimaryWindowMinutes != nil {
			use7dFromPrimary = true
	REDACTED
REDACTED

	// Write canonical 5h fields
	if use5hFromPrimary {
		if snapshot.PrimaryUsedPercent != nil {
			updates["codex_5h_used_percent"] = *snapshot.PrimaryUsedPercent
	REDACTED
		if snapshot.PrimaryResetAfterSeconds != nil {
			updates["codex_5h_reset_after_seconds"] = *snapshot.PrimaryResetAfterSeconds
	REDACTED
		if snapshot.PrimaryWindowMinutes != nil {
			updates["codex_5h_window_minutes"] = *snapshot.PrimaryWindowMinutes
	REDACTED
REDACTED else if use5hFromSecondary {
		if snapshot.SecondaryUsedPercent != nil {
			updates["codex_5h_used_percent"] = *snapshot.SecondaryUsedPercent
	REDACTED
		if snapshot.SecondaryResetAfterSeconds != nil {
			updates["codex_5h_reset_after_seconds"] = *snapshot.SecondaryResetAfterSeconds
	REDACTED
		if snapshot.SecondaryWindowMinutes != nil {
			updates["codex_5h_window_minutes"] = *snapshot.SecondaryWindowMinutes
	REDACTED
REDACTED

	// Write canonical 7d fields
	if use7dFromPrimary {
		if snapshot.PrimaryUsedPercent != nil {
			updates["codex_7d_used_percent"] = *snapshot.PrimaryUsedPercent
	REDACTED
		if snapshot.PrimaryResetAfterSeconds != nil {
			updates["codex_7d_reset_after_seconds"] = *snapshot.PrimaryResetAfterSeconds
	REDACTED
		if snapshot.PrimaryWindowMinutes != nil {
			updates["codex_7d_window_minutes"] = *snapshot.PrimaryWindowMinutes
	REDACTED
REDACTED else if use7dFromSecondary {
		if snapshot.SecondaryUsedPercent != nil {
			updates["codex_7d_used_percent"] = *snapshot.SecondaryUsedPercent
	REDACTED
		if snapshot.SecondaryResetAfterSeconds != nil {
			updates["codex_7d_reset_after_seconds"] = *snapshot.SecondaryResetAfterSeconds
	REDACTED
		if snapshot.SecondaryWindowMinutes != nil {
			updates["codex_7d_window_minutes"] = *snapshot.SecondaryWindowMinutes
	REDACTED
REDACTED

	// Update account's Extra field asynchronously
	go func() {
		updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.accountRepo.UpdateExtra(updateCtx, accountID, updates)
REDACTED()
REDACTED

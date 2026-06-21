package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	mathrand "math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const geminiStickySessionTTL = time.Hour

const (
	geminiMaxRetries     = 5
	geminiRetryBaseDelay = 1 * time.Second
	geminiRetryMaxDelay  = 16 * time.Second
)

// Gemini tool calling now requires `thoughtSignature` in parts that include `functionCall`.
// Many clients don't send it; we inject a known dummy signature to satisfy the validator.
// Ref: https://ai.google.dev/gemini-api/docs/thought-signatures
const geminiDummyThoughtSignature = "skip_thought_signature_validator"

const (
	geminiNativeCompatibleRelayTextChunkRunes = 32
	geminiNativeCompatibleRelayChunkDelay     = 150 * time.Millisecond
)

type GeminiMessagesCompatService struct {
	accountRepo               AccountRepository
	groupRepo                 GroupRepository
	cache                     GatewayCache
	schedulerSnapshot         *SchedulerSnapshotService
	tokenProvider             *GeminiTokenProvider
	rateLimitService          *RateLimitService
	httpUpstream              HTTPUpstream
	antigravityGatewayService *AntigravityGatewayService
	cfg                       *config.Config
	responseHeaderFilter      *responseheaders.CompiledHeaderFilter
}

func (s *GeminiMessagesCompatService) readUpstreamErrorBody(resp *http.Response) []byte {
	if resp == nil || resp.Body == nil {
		return nil
	}
	limit := gatewayUpstreamErrorBodyReadLimit
	if s != nil && s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody && s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes > int(limit) {
		limit = int64(s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, limit))
	return body
}

func NewGeminiMessagesCompatService(
	accountRepo AccountRepository,
	groupRepo GroupRepository,
	cache GatewayCache,
	schedulerSnapshot *SchedulerSnapshotService,
	tokenProvider *GeminiTokenProvider,
	rateLimitService *RateLimitService,
	httpUpstream HTTPUpstream,
	antigravityGatewayService *AntigravityGatewayService,
	cfg *config.Config,
) *GeminiMessagesCompatService {
	return &GeminiMessagesCompatService{
		accountRepo:               accountRepo,
		groupRepo:                 groupRepo,
		cache:                     cache,
		schedulerSnapshot:         schedulerSnapshot,
		tokenProvider:             tokenProvider,
		rateLimitService:          rateLimitService,
		httpUpstream:              httpUpstream,
		antigravityGatewayService: antigravityGatewayService,
		cfg:                       cfg,
		responseHeaderFilter:      compileResponseHeaderFilter(cfg),
	}
}

// GetTokenProvider returns the token provider for OAuth accounts
func (s *GeminiMessagesCompatService) GetTokenProvider() *GeminiTokenProvider {
	return s.tokenProvider
}

func (s *GeminiMessagesCompatService) SelectAccountForModel(ctx context.Context, groupID *int64, sessionHash string, requestedModel string) (*Account, error) {
	return s.SelectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, nil)
}

func (s *GeminiMessagesCompatService) SelectAccountForModelWithExclusions(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}) (*Account, error) {
	// 1. 确定目标平台和调度模式
	// Determine target platform and scheduling mode
	platform, useMixedScheduling, hasForcePlatform, err := s.resolvePlatformAndSchedulingMode(ctx, groupID)
	if err != nil {
		return nil, err
	}

	cacheKey := "gemini:" + sessionHash

	// 2. 尝试粘性会话命中
	// Try sticky session hit
	if account := s.tryStickySessionHit(ctx, groupID, sessionHash, cacheKey, requestedModel, excludedIDs, platform, useMixedScheduling); account != nil {
		return account, nil
	}

	// 3. 查询可调度账户（强制平台模式：优先按分组查找，找不到再查全部）
	// Query schedulable accounts (force platform mode: try group first, fallback to all)
	accounts, err := s.listSchedulableAccountsOnce(ctx, groupID, platform, hasForcePlatform)
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
	}
	// 强制平台模式下，分组中找不到账户时回退查询全部
	if len(accounts) == 0 && groupID != nil && hasForcePlatform {
		accounts, err = s.listSchedulableAccountsOnce(ctx, nil, platform, hasForcePlatform)
		if err != nil {
			return nil, fmt.Errorf("query accounts failed: %w", err)
		}
	}

	// 4. 按优先级 + LRU 选择最佳账号
	// Select best account by priority + LRU
	selected := s.selectBestGeminiAccount(ctx, accounts, requestedModel, excludedIDs, platform, useMixedScheduling)

	if selected == nil {
		if requestedModel != "" {
			return nil, fmt.Errorf("no available Gemini accounts supporting model: %s", requestedModel)
		}
		return nil, errors.New("no available Gemini accounts")
	}

	// 5. 设置粘性会话绑定
	// Set sticky session binding
	if sessionHash != "" {
		_ = s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), cacheKey, selected.ID, geminiStickySessionTTL)
	}

	return s.hydrateSelectedAccount(ctx, selected)
}

// resolvePlatformAndSchedulingMode 解析目标平台和调度模式。
// 返回：平台名称、是否使用混合调度、是否强制平台、错误。
//
// resolvePlatformAndSchedulingMode resolves target platform and scheduling mode.
// Returns: platform name, whether to use mixed scheduling, whether force platform, error.
func (s *GeminiMessagesCompatService) resolvePlatformAndSchedulingMode(ctx context.Context, groupID *int64) (platform string, useMixedScheduling bool, hasForcePlatform bool, err error) {
	// 优先检查 context 中的强制平台（/antigravity 路由）
	forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string)
	if hasForcePlatform && forcePlatform != "" {
		return forcePlatform, false, true, nil
	}

	if groupID != nil {
		// 根据分组 platform 决定查询哪种账号
		var group *Group
		if ctxGroup, ok := ctx.Value(ctxkey.Group).(*Group); ok && IsGroupContextValid(ctxGroup) && ctxGroup.ID == *groupID {
			group = ctxGroup
		} else {
			group, err = s.groupRepo.GetByIDLite(ctx, *groupID)
			if err != nil {
				return "", false, false, fmt.Errorf("get group failed: %w", err)
			}
		}
		// gemini 分组支持混合调度（包含启用了 mixed_scheduling 的 antigravity 账户）
		return group.Platform, group.Platform == PlatformGemini, false, nil
	}

	// 无分组时只使用原生 gemini 平台
	return PlatformGemini, true, false, nil
}

// tryStickySessionHit 尝试从粘性会话获取账号。
// 如果命中且账号可用则返回账号；如果账号不可用则清理会话并返回 nil。
//
// tryStickySessionHit attempts to get account from sticky session.
// Returns account if hit and usable; clears session and returns nil if account unavailable.
func (s *GeminiMessagesCompatService) tryStickySessionHit(
	ctx context.Context,
	groupID *int64,
	sessionHash, cacheKey, requestedModel string,
	excludedIDs map[int64]struct{},
	platform string,
	useMixedScheduling bool,
) *Account {
	if sessionHash == "" {
		return nil
	}

	accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), cacheKey)
	if err != nil || accountID <= 0 {
		return nil
	}

	if _, excluded := excludedIDs[accountID]; excluded {
		return nil
	}

	account, err := s.getSchedulableAccount(ctx, accountID)
	if err != nil {
		return nil
	}

	// 检查账号是否需要清理粘性会话
	// Check if sticky session should be cleared
	if shouldClearStickySession(account, requestedModel) {
		_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), cacheKey)
		return nil
	}

	// 验证账号是否可用于当前请求
	// Verify account is usable for current request
	if !s.isAccountUsableForRequest(ctx, account, requestedModel, platform, useMixedScheduling) {
		return nil
	}

	// 刷新会话 TTL 并返回账号
	// Refresh session TTL and return account
	_ = s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), cacheKey, geminiStickySessionTTL)
	return account
}

// isAccountUsableForRequest 检查账号是否可用于当前请求。
// 验证：模型调度、模型支持、平台匹配、速率限制预检。
//
// isAccountUsableForRequest checks if account is usable for current request.
// Validates: model scheduling, model support, platform matching, rate limit precheck.
func (s *GeminiMessagesCompatService) isAccountUsableForRequest(
	ctx context.Context,
	account *Account,
	requestedModel, platform string,
	useMixedScheduling bool,
) bool {
	return s.isAccountUsableForRequestWithPrecheck(ctx, account, requestedModel, platform, useMixedScheduling, nil)
}

func (s *GeminiMessagesCompatService) isAccountUsableForRequestWithPrecheck(
	ctx context.Context,
	account *Account,
	requestedModel, platform string,
	useMixedScheduling bool,
	precheckResult map[int64]bool,
) bool {
	// 检查模型调度能力
	// Check model scheduling capability
	if !account.IsSchedulableForModelWithContext(ctx, requestedModel) {
		return false
	}

	// 检查模型支持
	// Check model support
	if requestedModel != "" && !s.isModelSupportedByAccount(account, requestedModel) {
		return false
	}

	// 检查平台匹配
	// Check platform matching
	if !s.isAccountValidForPlatform(account, platform, useMixedScheduling) {
		return false
	}

	// 速率限制预检
	// Rate limit precheck
	if !s.passesRateLimitPreCheckWithCache(ctx, account, requestedModel, precheckResult) {
		return false
	}

	return true
}

// isAccountValidForPlatform 检查账号是否匹配目标平台。
// 原生平台直接匹配；混合调度模式下 antigravity 需要启用 mixed_scheduling。
//
// isAccountValidForPlatform checks if account matches target platform.
// Native platform matches directly; mixed scheduling mode requires antigravity to enable mixed_scheduling.
func (s *GeminiMessagesCompatService) isAccountValidForPlatform(account *Account, platform string, useMixedScheduling bool) bool {
	if account.Platform == platform {
		return true
	}
	if useMixedScheduling && account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled() {
		return true
	}
	return false
}

func (s *GeminiMessagesCompatService) passesRateLimitPreCheckWithCache(ctx context.Context, account *Account, requestedModel string, precheckResult map[int64]bool) bool {
	if s.rateLimitService == nil || requestedModel == "" {
		return true
	}

	if precheckResult != nil {
		if ok, exists := precheckResult[account.ID]; exists {
			return ok
		}
	}

	ok, err := s.rateLimitService.PreCheckUsage(ctx, account, requestedModel)
	if err != nil {
		logger.LegacyPrintf("service.gemini_messages_compat", "[Gemini PreCheck] Account %d precheck error: %v", account.ID, err)
	}
	return ok
}

// selectBestGeminiAccount 从候选账号中选择最佳账号（优先级 + LRU + OAuth 优先）。
// 返回 nil 表示无可用账号。
//
// selectBestGeminiAccount selects best account from candidates (priority + LRU + OAuth preferred).
// Returns nil if no available account.
func (s *GeminiMessagesCompatService) selectBestGeminiAccount(
	ctx context.Context,
	accounts []Account,
	requestedModel string,
	excludedIDs map[int64]struct{},
	platform string,
	useMixedScheduling bool,
) *Account {
	var selected *Account
	precheckResult := s.buildPreCheckUsageResultMap(ctx, accounts, requestedModel)

	for i := range accounts {
		acc := &accounts[i]

		// 跳过被排除的账号
		if _, excluded := excludedIDs[acc.ID]; excluded {
			continue
		}

		// 检查账号是否可用于当前请求
		if !s.isAccountUsableForRequestWithPrecheck(ctx, acc, requestedModel, platform, useMixedScheduling, precheckResult) {
			continue
		}

		// 选择最佳账号
		if selected == nil {
			selected = acc
			continue
		}

		if s.isBetterGeminiAccount(acc, selected) {
			selected = acc
		}
	}

	return selected
}

func (s *GeminiMessagesCompatService) buildPreCheckUsageResultMap(ctx context.Context, accounts []Account, requestedModel string) map[int64]bool {
	if s.rateLimitService == nil || requestedModel == "" || len(accounts) == 0 {
		return nil
	}

	candidates := make([]*Account, 0, len(accounts))
	for i := range accounts {
		candidates = append(candidates, &accounts[i])
	}

	result, err := s.rateLimitService.PreCheckUsageBatch(ctx, candidates, requestedModel)
	if err != nil {
		logger.LegacyPrintf("service.gemini_messages_compat", "[Gemini PreCheckBatch] failed: %v", err)
	}
	return result
}

// isBetterGeminiAccount 判断 candidate 是否比 current 更优。
// 规则：优先级更高（数值更小）优先；同优先级时，未使用过的优先（OAuth > 非 OAuth），其次是最久未使用的。
//
// isBetterGeminiAccount checks if candidate is better than current.
// Rules: higher priority (lower value) wins; same priority: never used (OAuth > non-OAuth) > least recently used.
func (s *GeminiMessagesCompatService) isBetterGeminiAccount(candidate, current *Account) bool {
	// 优先级更高（数值更小）
	if candidate.Priority < current.Priority {
		return true
	}
	if candidate.Priority > current.Priority {
		return false
	}

	// 同优先级，比较最后使用时间
	switch {
	case candidate.LastUsedAt == nil && current.LastUsedAt != nil:
		// candidate 从未使用，优先
		return true
	case candidate.LastUsedAt != nil && current.LastUsedAt == nil:
		// current 从未使用，保持
		return false
	case candidate.LastUsedAt == nil && current.LastUsedAt == nil:
		// 都未使用，优先选择 OAuth 账号（更兼容 Code Assist 流程）
		return candidate.Type == AccountTypeOAuth && current.Type != AccountTypeOAuth
	default:
		// 都使用过，选择最久未使用的
		return candidate.LastUsedAt.Before(*current.LastUsedAt)
	}
}

// isModelSupportedByAccount 根据账户平台检查模型支持
func (s *GeminiMessagesCompatService) isModelSupportedByAccount(account *Account, requestedModel string) bool {
	if account.Platform == PlatformAntigravity {
		if strings.TrimSpace(requestedModel) == "" {
			return true
		}
		return mapAntigravityModel(account, requestedModel) != ""
	}
	return account.IsModelSupported(requestedModel)
}

// GetAntigravityGatewayService 返回 AntigravityGatewayService
func (s *GeminiMessagesCompatService) GetAntigravityGatewayService() *AntigravityGatewayService {
	return s.antigravityGatewayService
}

func (s *GeminiMessagesCompatService) getSchedulableAccount(ctx context.Context, accountID int64) (*Account, error) {
	if s.schedulerSnapshot != nil {
		return s.schedulerSnapshot.GetAccount(ctx, accountID)
	}
	return s.accountRepo.GetByID(ctx, accountID)
}

func (s *GeminiMessagesCompatService) hydrateSelectedAccount(ctx context.Context, account *Account) (*Account, error) {
	if account == nil || s.schedulerSnapshot == nil {
		return account, nil
	}
	hydrated, err := s.schedulerSnapshot.GetAccount(ctx, account.ID)
	if err != nil {
		return nil, err
	}
	if hydrated == nil {
		return nil, fmt.Errorf("selected gemini account %d not found during hydration", account.ID)
	}
	return hydrated, nil
}

func (s *GeminiMessagesCompatService) listSchedulableAccountsOnce(ctx context.Context, groupID *int64, platform string, hasForcePlatform bool) ([]Account, error) {
	if s.schedulerSnapshot != nil {
		accounts, _, err := s.schedulerSnapshot.ListSchedulableAccounts(ctx, groupID, platform, hasForcePlatform)
		return accounts, err
	}

	useMixedScheduling := platform == PlatformGemini && !hasForcePlatform
	queryPlatforms := []string{platform}
	if useMixedScheduling {
		queryPlatforms = []string{platform, PlatformAntigravity}
	}

	if groupID != nil {
		return s.accountRepo.ListSchedulableByGroupIDAndPlatforms(ctx, *groupID, queryPlatforms)
	}
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return s.accountRepo.ListSchedulableByPlatforms(ctx, queryPlatforms)
	}
	return s.accountRepo.ListSchedulableUngroupedByPlatforms(ctx, queryPlatforms)
}

func (s *GeminiMessagesCompatService) validateUpstreamBaseURL(raw string) (string, error) {
	if s.cfg != nil && !s.cfg.Security.URLAllowlist.Enabled {
		normalized, err := urlvalidator.ValidateURLFormat(raw, s.cfg.Security.URLAllowlist.AllowInsecureHTTP)
		if err != nil {
			return "", fmt.Errorf("invalid base_url: %w", err)
		}
		return normalized, nil
	}
	normalized, err := urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     s.cfg.Security.URLAllowlist.UpstreamHosts,
		RequireAllowlist: true,
		AllowPrivate:     s.cfg.Security.URLAllowlist.AllowPrivateHosts,
	})
	if err != nil {
		return "", fmt.Errorf("invalid base_url: %w", err)
	}
	return normalized, nil
}

// HasAntigravityAccounts 检查是否有可用的 antigravity 账户
func (s *GeminiMessagesCompatService) HasAntigravityAccounts(ctx context.Context, groupID *int64) (bool, error) {
	accounts, err := s.listSchedulableAccountsOnce(ctx, groupID, PlatformAntigravity, false)
	if err != nil {
		return false, err
	}
	return len(accounts) > 0, nil
}

// SelectAccountForAIStudioEndpoints selects an account that is likely to succeed against
// generativelanguage.googleapis.com (e.g. GET /v1beta/models).
//
// Preference order:
// 1) API key accounts (AI Studio)
// 2) OAuth accounts without project_id (AI Studio OAuth)
// 3) OAuth accounts explicitly marked as ai_studio
// 4) Any remaining Gemini accounts (fallback)
func (s *GeminiMessagesCompatService) SelectAccountForAIStudioEndpoints(ctx context.Context, groupID *int64) (*Account, error) {
	accounts, err := s.listSchedulableAccountsOnce(ctx, groupID, PlatformGemini, true)
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
	}
	if len(accounts) == 0 {
		return nil, errors.New("no available Gemini accounts")
	}

	rank := func(a *Account) int {
		if a == nil {
			return 999
		}
		switch a.Type {
		case AccountTypeAPIKey:
			if strings.TrimSpace(a.GetCredential("api_key")) != "" {
				return 0
			}
			return 9
		case AccountTypeOAuth:
			if strings.TrimSpace(a.GetCredential("project_id")) == "" {
				return 1
			}
			if strings.TrimSpace(a.GetCredential("oauth_type")) == "ai_studio" {
				return 2
			}
			// Code Assist OAuth tokens often lack AI Studio scopes for models listing.
			return 3
		case AccountTypeServiceAccount:
			// Vertex service accounts use aiplatform.googleapis.com, not the AI Studio
			// endpoint (generativelanguage.googleapis.com), so they cannot serve these requests.
			return 999
		default:
			return 10
		}
	}

	var selected *Account
	for i := range accounts {
		acc := &accounts[i]
		if selected == nil {
			selected = acc
			continue
		}

		r1, r2 := rank(acc), rank(selected)
		if r1 < r2 {
			selected = acc
			continue
		}
		if r1 > r2 {
			continue
		}

		if acc.Priority < selected.Priority {
			selected = acc
		} else if acc.Priority == selected.Priority {
			switch {
			case acc.LastUsedAt == nil && selected.LastUsedAt != nil:
				selected = acc
			case acc.LastUsedAt != nil && selected.LastUsedAt == nil:
				// keep selected
			case acc.LastUsedAt == nil && selected.LastUsedAt == nil:
				if acc.Type == AccountTypeOAuth && selected.Type != AccountTypeOAuth {
					selected = acc
				}
			default:
				if acc.LastUsedAt.Before(*selected.LastUsedAt) {
					selected = acc
				}
			}
		}
	}

	if selected == nil {
		return nil, errors.New("no available Gemini accounts")
	}
	return s.hydrateSelectedAccount(ctx, selected)
}

func (s *GeminiMessagesCompatService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
	account = NormalizeGeminiAPIKeyAccount(account)
	startTime := time.Now()

	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("parse request: %w", err)
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, fmt.Errorf("missing model")
	}

	originalModel := req.Model
	mappedModel := req.Model
	if account.Type == AccountTypeAPIKey || account.Type == AccountTypeServiceAccount {
		mappedModel = account.GetMappedModel(req.Model)
	}

	if account != nil && account.IsGeminiOpenAICompatibleUpstream() {
		result, err := s.forwardCompatibleRelayMessagesAsChatCompletions(ctx, c, account, body, originalModel, mappedModel, startTime)
		if !errors.Is(err, errGeminiCompatibleRelayOpenAIPathUnsupported) {
			return result, err
		}
	}

	geminiReq, err := convertClaudeMessagesToGeminiGenerateContent(body)
	if err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
	}
	geminiReq = ensureGeminiFunctionCallThoughtSignatures(geminiReq)
	originalClaudeBody := body

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	var requestIDHeader string
	var buildReq func(ctx context.Context) (*http.Request, string, error)
	useUpstreamStream := req.Stream
	if account.Type == AccountTypeOAuth && !req.Stream && strings.TrimSpace(account.GetCredential("project_id")) != "" {
		// Code Assist's non-streaming generateContent may return no content; use streaming upstream and aggregate.
		useUpstreamStream = true
	}

	switch account.Type {
	case AccountTypeAPIKey:
		buildReq = func(ctx context.Context) (*http.Request, string, error) {
			apiKey := account.GetCredential("api_key")
			if strings.TrimSpace(apiKey) == "" {
				return nil, "", errors.New("gemini api_key not configured")
			}

			baseURL := account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
			baseURL = geminiNativeBaseURLFromOpenAICompatible(baseURL)
			normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, "", err
			}

			action := "generateContent"
			if req.Stream {
				action = "streamGenerateContent"
			}
			fullURL := fmt.Sprintf("%s/v1beta/models/%s:%s", strings.TrimRight(normalizedBaseURL, "/"), mappedModel, action)
			if req.Stream {
				fullURL += "?alt=sse"
			}

			restGeminiReq := normalizeGeminiRequestForAIStudio(geminiReq)
			upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(restGeminiReq))
			if err != nil {
				return nil, "", err
			}
			upstreamReq.Header.Set("Content-Type", "application/json")
			upstreamReq.Header.Set("x-goog-api-key", apiKey)
			return upstreamReq, "x-request-id", nil
		}
		requestIDHeader = "x-request-id"

	case AccountTypeOAuth:
		buildReq = func(ctx context.Context) (*http.Request, string, error) {
			if s.tokenProvider == nil {
				return nil, "", errors.New("gemini token provider not configured")
			}
			accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
			if err != nil {
				return nil, "", err
			}

			projectID := strings.TrimSpace(account.GetCredential("project_id"))

			action := "generateContent"
			if useUpstreamStream {
				action = "streamGenerateContent"
			}

			// Two modes for OAuth:
			// 1. With project_id -> Code Assist API (wrapped request)
			// 2. Without project_id -> AI Studio API (direct OAuth, like API key but with Bearer token)
			if projectID != "" {
				// Mode 1: Code Assist API
				baseURL, err := s.validateUpstreamBaseURL(geminicli.GeminiCliBaseURL)
				if err != nil {
					return nil, "", err
				}
				fullURL := fmt.Sprintf("%s/v1internal:%s", strings.TrimRight(baseURL, "/"), action)
				if useUpstreamStream {
					fullURL += "?alt=sse"
				}

				wrapped := map[string]any{
					"model":   mappedModel,
					"project": projectID,
				}
				var inner any
				if err := json.Unmarshal(geminiReq, &inner); err != nil {
					return nil, "", fmt.Errorf("failed to parse gemini request: %w", err)
				}
				wrapped["request"] = inner
				wrappedBytes, _ := json.Marshal(wrapped)

				upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(wrappedBytes))
				if err != nil {
					return nil, "", err
				}
				upstreamReq.Header.Set("Content-Type", "application/json")
				upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
				upstreamReq.Header.Set("User-Agent", geminicli.GeminiCLIUserAgent)
				return upstreamReq, "x-request-id", nil
			} else {
				// Mode 2: AI Studio API with OAuth (like API key mode, but using Bearer token)
				baseURL := account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
				baseURL = geminiNativeBaseURLFromOpenAICompatible(baseURL)
				normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
				if err != nil {
					return nil, "", err
				}

				fullURL := fmt.Sprintf("%s/v1beta/models/%s:%s", strings.TrimRight(normalizedBaseURL, "/"), mappedModel, action)
				if useUpstreamStream {
					fullURL += "?alt=sse"
				}

				restGeminiReq := normalizeGeminiRequestForAIStudio(geminiReq)
				upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(restGeminiReq))
				if err != nil {
					return nil, "", err
				}
				upstreamReq.Header.Set("Content-Type", "application/json")
				upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
				return upstreamReq, "x-request-id", nil
			}
		}
		requestIDHeader = "x-request-id"

	case AccountTypeServiceAccount:
		buildReq = func(ctx context.Context) (*http.Request, string, error) {
			if s.tokenProvider == nil {
				return nil, "", errors.New("gemini token provider not configured")
			}
			accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
			if err != nil {
				return nil, "", err
			}

			action := "generateContent"
			if req.Stream {
				action = "streamGenerateContent"
			}
			fullURL, err := buildVertexGeminiURL(account.VertexProjectID(), account.VertexLocation(mappedModel), mappedModel, action, req.Stream)
			if err != nil {
				return nil, "", err
			}

			restGeminiReq := normalizeGeminiRequestForAIStudio(geminiReq)
			upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(restGeminiReq))
			if err != nil {
				return nil, "", err
			}
			upstreamReq.Header.Set("Content-Type", "application/json")
			upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
			return upstreamReq, "x-request-id", nil
		}
		requestIDHeader = "x-request-id"

	default:
		return nil, fmt.Errorf("unsupported account type: %s", account.Type)
	}

	var resp *http.Response
	signatureRetryStage := 0
	for attempt := 1; attempt <= geminiMaxRetries; attempt++ {
		upstreamReq, idHeader, err := buildReq(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			// Local build error: don't retry.
			if strings.Contains(err.Error(), "missing project_id") {
				return nil, s.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
			}
			return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", err.Error())
		}
		requestIDHeader = idHeader

		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
		if err != nil {
			safeErr := sanitizeUpstreamErrorMessage(err.Error())
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: 0,
				Kind:               "request_error",
				Message:            safeErr,
			})
			if attempt < geminiMaxRetries {
				logger.LegacyPrintf("service.gemini_messages_compat", "Gemini account %d: upstream request failed, retry %d/%d: %v", account.ID, attempt, geminiMaxRetries, err)
				sleepGeminiBackoff(attempt)
				continue
			}
			setOpsUpstreamError(c, 0, safeErr, "")
			return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed after retries: "+safeErr)
		}

		// Special-case: signature/thought_signature validation errors are not transient, but may be fixed by
		// downgrading Claude thinking/tool history to plain text (conservative two-stage retry).
		if resp.StatusCode == http.StatusBadRequest && signatureRetryStage < 2 {
			respBody := s.readUpstreamErrorBody(resp)
			_ = resp.Body.Close()

			if isGeminiSignatureRelatedError(respBody) {
				upstreamReqID := resp.Header.Get(requestIDHeader)
				if upstreamReqID == "" {
					upstreamReqID = resp.Header.Get("x-goog-request-id")
				}
				upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
				upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
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
					UpstreamRequestID:  upstreamReqID,
					Kind:               "signature_error",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})

				var strippedClaudeBody []byte
				stageName := ""
				// 路径说明：本处上游是 Gemini，但被剥离的 body 是 Anthropic 格式。传 originalModel
				// （客户端原 Anthropic model）而非 mappedModel（上游 Gemini model），让剥离逻辑按
				// 客户端请求的 Anthropic 子协议族判定（详见 ResolveThinkingProtocol 文档）。
				switch signatureRetryStage {
				case 0:
					// Stage 1: disable thinking + thinking->text
					strippedClaudeBody = FilterThinkingBlocksForRetry(originalClaudeBody, originalModel)
					stageName = "thinking-only"
					signatureRetryStage = 1
				default:
					// Stage 2: additionally downgrade tool_use/tool_result blocks to text
					strippedClaudeBody = FilterSignatureSensitiveBlocksForRetry(originalClaudeBody, originalModel)
					stageName = "thinking+tools"
					signatureRetryStage = 2
				}
				retryGeminiReq, txErr := convertClaudeMessagesToGeminiGenerateContent(strippedClaudeBody)
				if txErr == nil {
					logger.LegacyPrintf("service.gemini_messages_compat", "Gemini account %d: detected signature-related 400, retrying with downgraded Claude blocks (%s)", account.ID, stageName)
					geminiReq = retryGeminiReq
					// Consume one retry budget attempt and continue with the updated request payload.
					sleepGeminiBackoff(1)
					continue
				}
			}

			// Restore body for downstream error handling.
			resp = &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     resp.Header.Clone(),
				Body:       io.NopCloser(bytes.NewReader(respBody)),
			}
			break
		}

		// 错误策略优先：匹配则跳过重试直接处理。
		if matched, rebuilt := s.checkErrorPolicyInLoop(ctx, account, resp); matched {
			resp = rebuilt
			break
		} else {
			resp = rebuilt
		}

		if resp.StatusCode >= 400 && s.shouldRetryGeminiUpstreamError(account, resp.StatusCode) {
			respBody := s.readUpstreamErrorBody(resp)
			_ = resp.Body.Close()
			// Don't treat insufficient-scope as transient.
			if resp.StatusCode == 403 && isGeminiInsufficientScope(resp.Header, respBody) {
				resp = &http.Response{
					StatusCode: resp.StatusCode,
					Header:     resp.Header.Clone(),
					Body:       io.NopCloser(bytes.NewReader(respBody)),
				}
				break
			}
			if resp.StatusCode == 429 {
				// Mark as rate-limited early so concurrent requests avoid this account.
				s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
			}
			if attempt < geminiMaxRetries {
				upstreamReqID := resp.Header.Get(requestIDHeader)
				if upstreamReqID == "" {
					upstreamReqID = resp.Header.Get("x-goog-request-id")
				}
				upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
				upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
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
					UpstreamRequestID:  upstreamReqID,
					Kind:               "retry",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})

				logger.LegacyPrintf("service.gemini_messages_compat", "Gemini account %d: upstream status %d, retry %d/%d", account.ID, resp.StatusCode, attempt, geminiMaxRetries)
				sleepGeminiBackoff(attempt)
				continue
			}
			// Final attempt: surface the upstream error body (mapped below) instead of a generic retry error.
			resp = &http.Response{
				StatusCode: resp.StatusCode,
				Header:     resp.Header.Clone(),
				Body:       io.NopCloser(bytes.NewReader(respBody)),
			}
			break
		}

		break
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		// 统一错误策略：自定义错误码 + 临时不可调度
		if s.rateLimitService != nil {
			switch s.rateLimitService.CheckErrorPolicy(ctx, account, resp.StatusCode, respBody) {
			case ErrorPolicySkipped:
				upstreamReqID := resp.Header.Get(requestIDHeader)
				if upstreamReqID == "" {
					upstreamReqID = resp.Header.Get("x-goog-request-id")
				}
				return nil, s.writeGeminiMappedError(c, account, http.StatusInternalServerError, upstreamReqID, respBody)
			case ErrorPolicyMatched, ErrorPolicyTempUnscheduled:
				s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
				upstreamReqID := resp.Header.Get(requestIDHeader)
				if upstreamReqID == "" {
					upstreamReqID = resp.Header.Get("x-goog-request-id")
				}
				upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
				upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
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
					UpstreamRequestID:  upstreamReqID,
					Kind:               "failover",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})
				return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody}
			}
		}

		// ErrorPolicyNone → 原有逻辑
		s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		// 精确匹配服务端配置类 400 错误，触发 failover + 临时封禁
		if resp.StatusCode == http.StatusBadRequest {
			msg400 := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
			if isGoogleProjectConfigError(msg400) {
				upstreamReqID := resp.Header.Get(requestIDHeader)
				if upstreamReqID == "" {
					upstreamReqID = resp.Header.Get("x-goog-request-id")
				}
				upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
				upstreamDetail := ""
				if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
					maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
					if maxBytes <= 0 {
						maxBytes = 2048
					}
					upstreamDetail = truncateString(string(respBody), maxBytes)
				}
				log.Printf("[Gemini] status=400 google_config_error failover=true upstream_message=%q account=%d", upstreamMsg, account.ID)
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  upstreamReqID,
					Kind:               "failover",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})
				return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody, RetryableOnSameAccount: true}
			}
		}
		if s.shouldFailoverGeminiUpstreamError(resp.StatusCode) {
			upstreamReqID := resp.Header.Get(requestIDHeader)
			if upstreamReqID == "" {
				upstreamReqID = resp.Header.Get("x-goog-request-id")
			}
			upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
			upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
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
				UpstreamRequestID:  upstreamReqID,
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
			})
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody}
		}
		upstreamReqID := resp.Header.Get(requestIDHeader)
		if upstreamReqID == "" {
			upstreamReqID = resp.Header.Get("x-goog-request-id")
		}
		return nil, s.writeGeminiMappedError(c, account, resp.StatusCode, upstreamReqID, respBody)
	}

	requestID := resp.Header.Get(requestIDHeader)
	if requestID == "" {
		requestID = resp.Header.Get("x-goog-request-id")
	}
	if requestID != "" {
		c.Header("x-request-id", requestID)
	}

	var usage *ClaudeUsage
	var firstTokenMs *int
	if req.Stream {
		streamRes, err := s.handleStreamingResponse(c, resp, startTime, originalModel)
		if err != nil {
			return nil, err
		}
		usage = streamRes.usage
		firstTokenMs = streamRes.firstTokenMs
	} else {
		if useUpstreamStream {
			collected, usageObj, err := collectGeminiSSE(resp.Body, true)
			if err != nil {
				return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Failed to read upstream stream")
			}
			collectedBytes, _ := json.Marshal(collected)
			claudeResp, usageObj2 := convertGeminiToClaudeMessage(collected, originalModel, collectedBytes)
			c.JSON(http.StatusOK, claudeResp)
			usage = usageObj2
			if usageObj != nil && (usageObj.InputTokens > 0 || usageObj.OutputTokens > 0) {
				usage = usageObj
			}
		} else {
			usage, err = s.handleNonStreamingResponse(c, resp, originalModel)
			if err != nil {
				return nil, err
			}
		}
	}

	// 图片生成计费
	imageCount := 0
	imageInputSize := s.extractImageInputSize(body)
	imageSize := normalizeOpenAIImageSizeTier(imageInputSize)
	if isImageGenerationModel(originalModel) {
		imageCount = 1
	}

	return &ForwardResult{
		RequestID:      requestID,
		Usage:          *usage,
		Model:          originalModel,
		UpstreamModel:  mappedModel,
		Stream:         req.Stream,
		Duration:       time.Since(startTime),
		FirstTokenMs:   firstTokenMs,
		ImageCount:     imageCount,
		ImageSize:      imageSize,
		ImageInputSize: imageInputSize,
	}, nil
}

func (s *GeminiMessagesCompatService) forwardCompatibleRelayMessagesAsChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	originalModel string,
	upstreamModel string,
	startTime time.Time,
) (*ForwardResult, error) {
	upstreamBody, clientStream, reasoningEffort, err := buildGeminiCompatibleRelayMessagesChatBody(body, upstreamModel)
	if err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
	}

	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "gemini compatible relay api_key not configured")
	}
	baseURL := account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", err.Error())
	}

	targetURL := buildOpenAIChatCompletionsURL(normalizedBaseURL)
	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(upstreamBody))
	if err != nil {
		return nil, fmt.Errorf("build compatible relay messages request: %w", err)
	}
	upstreamReq = upstreamReq.WithContext(WithHTTPUpstreamProfile(upstreamReq.Context(), HTTPUpstreamProfileOpenAI))
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	if clientStream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}

	for key, values := range c.Request.Header {
		if openaiCCRawAllowedHeaders[strings.ToLower(key)] {
			for _, value := range values {
				upstreamReq.Header.Add(key, value)
			}
		}
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
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
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
	}
	defer func() { _ = resp.Body.Close() }()

	if isOpenAIChatCompletionsEndpointUnsupported(resp.StatusCode) {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, gatewayUpstreamErrorBodyReadLimit))
		return nil, errGeminiCompatibleRelayOpenAIPathUnsupported
	}

	requestID := resp.Header.Get("x-request-id")
	if requestID != "" {
		c.Header("x-request-id", requestID)
	}

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if upstreamMsg == "" {
			upstreamMsg = fmt.Sprintf("upstream returned HTTP %d", resp.StatusCode)
		}
		setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  requestID,
			Kind:               "http_error",
			Message:            upstreamMsg,
		})
		if s.shouldFailoverGeminiUpstreamError(resp.StatusCode) {
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody}
		}
		return nil, s.writeClaudeError(c, resp.StatusCode, "upstream_error", upstreamMsg)
	}

	if clientStream {
		return s.streamCompatibleRelayMessagesChatCompletions(c, resp, account, originalModel, upstreamModel, reasoningEffort, startTime, body)
	}
	return s.bufferCompatibleRelayMessagesChatCompletions(c, resp, originalModel, upstreamModel, reasoningEffort, startTime, body)
}

func buildGeminiCompatibleRelayMessagesChatBody(body []byte, upstreamModel string) ([]byte, bool, *string, error) {
	var anthropicReq apicompat.AnthropicRequest
	if err := json.Unmarshal(body, &anthropicReq); err != nil {
		return nil, false, nil, fmt.Errorf("parse Anthropic request: %w", err)
	}

	responsesReq, err := apicompat.AnthropicToResponses(&anthropicReq)
	if err != nil {
		return nil, false, nil, fmt.Errorf("convert Anthropic request to Responses: %w", err)
	}
	chatReq, err := apicompat.ResponsesToChatCompletionsRequest(responsesReq)
	if err != nil {
		return nil, false, nil, fmt.Errorf("convert Responses request to Chat Completions: %w", err)
	}

	if strings.TrimSpace(upstreamModel) != "" {
		chatReq.Model = upstreamModel
	}
	chatReq.Stream = anthropicReq.Stream
	if !shouldForwardAnthropicReasoningToGeminiCompatibleRelay(&anthropicReq) {
		chatReq.ReasoningEffort = ""
	}
	if chatReq.Stream {
		if chatReq.StreamOptions == nil {
			chatReq.StreamOptions = &apicompat.ChatStreamOptions{}
		}
		chatReq.StreamOptions.IncludeUsage = true
	}

	var reasoningEffort *string
	if strings.TrimSpace(chatReq.ReasoningEffort) != "" {
		value := chatReq.ReasoningEffort
		reasoningEffort = &value
	}

	upstreamBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, false, nil, fmt.Errorf("marshal Chat Completions request: %w", err)
	}
	return upstreamBody, chatReq.Stream, reasoningEffort, nil
}

func shouldForwardAnthropicReasoningToGeminiCompatibleRelay(req *apicompat.AnthropicRequest) bool {
	if req == nil {
		return false
	}
	if req.OutputConfig != nil && strings.TrimSpace(req.OutputConfig.Effort) != "" {
		return true
	}
	if req.Thinking == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(req.Thinking.Type)) {
	case "enabled", "adaptive":
		return true
	default:
		return false
	}
}

func (s *GeminiMessagesCompatService) streamCompatibleRelayMessagesChatCompletions(
	c *gin.Context,
	resp *http.Response,
	account *Account,
	originalModel string,
	upstreamModel string,
	reasoningEffort *string,
	startTime time.Time,
	originalBody []byte,
) (*ForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	headersWritten := false
	writeStreamHeaders := func() {
		if headersWritten {
			return
		}
		headersWritten = true
		if s.responseHeaderFilter != nil {
			responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
		}
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.Writer.WriteHeader(http.StatusOK)
	}

	chatState := apicompat.NewChatCompletionsToResponsesStreamState(originalModel)
	anthropicState := apicompat.NewResponsesEventToAnthropicState()
	anthropicState.Model = originalModel

	var usage ClaudeUsage
	var firstTokenMs *int
	clientDisconnected := false
	clientOutputStarted := false
	refusalDetector := newOpenAIChatSilentRefusalDetector(len(originalBody))

	writeResponsesEvents := func(events []apicompat.ResponsesStreamEvent) {
		if clientDisconnected || len(events) == 0 {
			return
		}
		for _, responseEvent := range events {
			anthropicEvents := apicompat.ResponsesEventToAnthropicEvents(&responseEvent, anthropicState)
			for _, anthropicEvent := range anthropicEvents {
				sse, err := apicompat.ResponsesAnthropicEventToSSE(anthropicEvent)
				if err != nil {
					continue
				}
				writeStreamHeaders()
				clientOutputStarted = true
				if _, err := fmt.Fprint(c.Writer, sse); err != nil {
					clientDisconnected = true
					return
				}
			}
			if responseEvent.Usage != nil {
				usage = claudeUsageFromOpenAIUsage(copyOpenAIUsageFromResponsesUsage(responseEvent.Usage))
			}
			if responseEvent.Response != nil && responseEvent.Response.Usage != nil {
				usage = claudeUsageFromOpenAIUsage(copyOpenAIUsageFromResponsesUsage(responseEvent.Response.Usage))
			}
		}
		if !clientDisconnected && clientOutputStarted {
			c.Writer.Flush()
		}
	}

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	sawDone := false
	for scanner.Scan() {
		line := scanner.Text()
		refusalDetector.ObserveSSELine(line)
		payload, ok := extractOpenAISSEDataLine(line)
		if !ok {
			continue
		}
		payload = strings.TrimSpace(payload)
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			sawDone = true
			break
		}

		if u := extractCCStreamUsage(payload); u != nil {
			usage = claudeUsageFromOpenAIUsage(*u)
		}

		var chunk apicompat.ChatCompletionsChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if firstTokenMs == nil && !isOpenAIChatUsageOnlyStreamChunk(payload) && chatChunkStartsResponsesOutput(&chunk) {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}
		writeResponsesEvents(apicompat.ChatCompletionsChunkToResponsesEvents(&chunk, chatState))
	}

	if err := scanner.Err(); err != nil {
		result := &ForwardResult{
			RequestID:        requestID,
			Usage:            usage,
			Model:            originalModel,
			UpstreamModel:    upstreamModel,
			Stream:           true,
			Duration:         time.Since(startTime),
			FirstTokenMs:     firstTokenMs,
			ReasoningEffort:  reasoningEffort,
			ClientDisconnect: clientDisconnected,
		}
		if !clientOutputStarted && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Failed to read upstream stream")
		}
		return result, fmt.Errorf("read compatible relay messages stream: %w", err)
	}

	if !sawDone && !clientOutputStarted && refusalDetector.IsSilentRefusal() {
		return nil, newOpenAISilentRefusalFailoverError(c, account, requestID)
	}

	writeResponsesEvents(apicompat.FinalizeChatCompletionsResponsesStream(chatState))
	if finalEvents := apicompat.FinalizeResponsesAnthropicStream(anthropicState); len(finalEvents) > 0 && !clientDisconnected {
		for _, event := range finalEvents {
			sse, err := apicompat.ResponsesAnthropicEventToSSE(event)
			if err != nil {
				continue
			}
			writeStreamHeaders()
			clientOutputStarted = true
			if _, err := fmt.Fprint(c.Writer, sse); err != nil {
				clientDisconnected = true
				break
			}
		}
		if !clientDisconnected && clientOutputStarted {
			c.Writer.Flush()
		}
	}

	imageCount := 0
	imageInputSize := s.extractImageInputSize(originalBody)
	imageSize := normalizeOpenAIImageSizeTier(imageInputSize)
	if isImageGenerationModel(originalModel) {
		imageCount = 1
	}

	return &ForwardResult{
		RequestID:        requestID,
		Usage:            usage,
		Model:            originalModel,
		UpstreamModel:    upstreamModel,
		Stream:           true,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ReasoningEffort:  reasoningEffort,
		ImageCount:       imageCount,
		ImageSize:        imageSize,
		ImageInputSize:   imageInputSize,
		ClientDisconnect: clientDisconnected,
	}, nil
}

func (s *GeminiMessagesCompatService) bufferCompatibleRelayMessagesChatCompletions(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	upstreamModel string,
	reasoningEffort *string,
	startTime time.Time,
	originalBody []byte,
) (*ForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, anthropicTooLargeError)
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Failed to read upstream response")
		}
		return nil, fmt.Errorf("read compatible relay messages response body: %w", err)
	}

	var chatResp apicompat.ChatCompletionsResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Failed to parse upstream response")
	}

	responsesResp := apicompat.ChatCompletionsResponseToResponses(&chatResp, originalModel)
	anthropicResp := apicompat.ResponsesToAnthropic(responsesResp, originalModel)

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.JSON(http.StatusOK, anthropicResp)

	usage := ClaudeUsage{}
	if responsesResp.Usage != nil {
		usage = claudeUsageFromOpenAIUsage(copyOpenAIUsageFromResponsesUsage(responsesResp.Usage))
	}
	imageCount := 0
	imageInputSize := s.extractImageInputSize(originalBody)
	imageSize := normalizeOpenAIImageSizeTier(imageInputSize)
	if isImageGenerationModel(originalModel) {
		imageCount = 1
	}

	return &ForwardResult{
		RequestID:       requestID,
		Usage:           usage,
		Model:           originalModel,
		UpstreamModel:   upstreamModel,
		Stream:          false,
		Duration:        time.Since(startTime),
		ReasoningEffort: reasoningEffort,
		ImageCount:      imageCount,
		ImageSize:       imageSize,
		ImageInputSize:  imageInputSize,
	}, nil
}

func (s *GeminiMessagesCompatService) forwardCompatibleRelayNativeAsChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	originalModel string,
	upstreamModel string,
	action string,
	stream bool,
	body []byte,
	startTime time.Time,
) (*ForwardResult, error) {
	clientStream := stream || action == "streamGenerateContent"

	if action == "countTokens" {
		estimated := estimateGeminiCountTokens(body)
		c.JSON(http.StatusOK, map[string]any{"totalTokens": estimated})
		return &ForwardResult{
			Usage:         ClaudeUsage{},
			Model:         originalModel,
			UpstreamModel: upstreamModel,
			Stream:        false,
			Duration:      time.Since(startTime),
		}, nil
	}

	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		return nil, s.writeGoogleError(c, http.StatusBadGateway, "gemini compatible relay api_key not configured")
	}
	baseURL := account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, s.writeGoogleError(c, http.StatusBadGateway, err.Error())
	}

	upstreamBody, err := buildGeminiNativeCompatibleRelayChatBody(body, upstreamModel, clientStream)
	if err != nil {
		return nil, s.writeGoogleError(c, http.StatusBadRequest, err.Error())
	}

	if clientStream {
		updated, usageErr := ensureOpenAIChatStreamUsage(upstreamBody)
		if usageErr != nil {
			return nil, fmt.Errorf("enable compatible relay native stream usage: %w", usageErr)
		}
		upstreamBody = updated
	}

	targetURL := buildOpenAIChatCompletionsURL(normalizedBaseURL)
	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(upstreamBody))
	if err != nil {
		return nil, fmt.Errorf("build compatible relay native request: %w", err)
	}
	upstreamReq = upstreamReq.WithContext(WithHTTPUpstreamProfile(upstreamReq.Context(), HTTPUpstreamProfileOpenAI))
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	if clientStream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}

	var proxyURL string
	if account.ProxyID != nil && account.Proxy != nil {
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
		return nil, s.writeGoogleError(c, http.StatusBadGateway, "Upstream request failed")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, gatewayUpstreamErrorBodyReadLimit))
		return nil, errGeminiCompatibleRelayOpenAIPathUnsupported
	}

	requestID := resp.Header.Get("x-request-id")
	if requestID != "" {
		c.Header("x-request-id", requestID)
	}

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if upstreamMsg == "" {
			upstreamMsg = fmt.Sprintf("upstream returned HTTP %d", resp.StatusCode)
		}
		setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  requestID,
			Kind:               "http_error",
			Message:            upstreamMsg,
		})
		if s.shouldFailoverGeminiUpstreamError(resp.StatusCode) {
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody}
		}
		return nil, s.writeGoogleError(c, resp.StatusCode, upstreamMsg)
	}

	if clientStream {
		return s.streamCompatibleRelayNativeChatCompletions(c, resp, account, originalModel, upstreamModel, startTime, body)
	}
	return s.bufferCompatibleRelayNativeChatCompletions(c, resp, originalModel, upstreamModel, startTime, body)
}

func buildGeminiNativeCompatibleRelayChatBody(body []byte, upstreamModel string, stream bool) ([]byte, error) {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini request: %w", err)
	}

	messages, err := geminiNativeContentsToChatMessages(req)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, errors.New("Gemini request contents must include at least one message")
	}

	chatReq := apicompat.ChatCompletionsRequest{
		Model:    upstreamModel,
		Messages: messages,
		Stream:   stream,
	}

	if gen, ok := req["generationConfig"].(map[string]any); ok {
		applyGeminiGenerationConfigToChatRequest(gen, &chatReq)
	}
	if tools := geminiNativeToolsToChatTools(req["tools"]); len(tools) > 0 {
		chatReq.Tools = tools
	}
	if choice := geminiNativeToolChoiceToChatToolChoice(req["toolConfig"]); len(choice) > 0 {
		chatReq.ToolChoice = choice
	}
	if stream {
		chatReq.StreamOptions = &apicompat.ChatStreamOptions{IncludeUsage: true}
	}

	return json.Marshal(chatReq)
}

type geminiNativeToolCallTracker struct {
	next  int
	byKey map[string]string
}

func (t *geminiNativeToolCallTracker) idFor(name string) string {
	if t.byKey == nil {
		t.byKey = make(map[string]string)
	}
	key := strings.TrimSpace(name)
	if key == "" {
		key = "tool"
	}
	if id := t.byKey[key]; id != "" {
		return id
	}
	t.next++
	id := fmt.Sprintf("call_%s_%d", sanitizeGeminiToolCallName(key), t.next)
	t.byKey[key] = id
	return id
}

func sanitizeGeminiToolCallName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "tool"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			_ = b.WriteByte(byte(r))
		case r == '_' || r == '-':
			_ = b.WriteByte(byte(r))
		default:
			_ = b.WriteByte('_')
		}
		if b.Len() >= 48 {
			break
		}
	}
	out := strings.Trim(b.String(), "_-")
	if out == "" {
		return "tool"
	}
	return out
}

func geminiNativeContentsToChatMessages(req map[string]any) ([]apicompat.ChatMessage, error) {
	messages := make([]apicompat.ChatMessage, 0)
	if systemText := geminiNativeSystemInstructionText(req["systemInstruction"]); systemText != "" {
		content, _ := json.Marshal(systemText)
		messages = append(messages, apicompat.ChatMessage{Role: "system", Content: content})
	}

	contents, ok := req["contents"].([]any)
	if !ok || len(contents) == 0 {
		return messages, nil
	}

	tracker := &geminiNativeToolCallTracker{}
	for _, rawContent := range contents {
		content, ok := rawContent.(map[string]any)
		if !ok {
			continue
		}
		role := geminiNativeRoleToChatRole(fmt.Sprint(content["role"]))
		parts, _ := content["parts"].([]any)

		var textParts []string
		var chatParts []apicompat.ChatContentPart
		var toolCalls []apicompat.ChatToolCall
		var toolMessages []apicompat.ChatMessage
		hasImage := false

		for _, rawPart := range parts {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := part["text"].(string); ok && text != "" {
				textParts = append(textParts, text)
				chatParts = append(chatParts, apicompat.ChatContentPart{Type: "text", Text: text})
				continue
			}
			if imageURL := geminiNativeImageURLFromPart(part); imageURL != "" {
				if role == "user" {
					hasImage = true
					chatParts = append(chatParts, apicompat.ChatContentPart{
						Type:     "image_url",
						ImageURL: &apicompat.ChatImageURL{URL: imageURL},
					})
				} else {
					textParts = append(textParts, "[image]")
				}
				continue
			}
			if fc, ok := part["functionCall"].(map[string]any); ok && fc != nil {
				name := strings.TrimSpace(fmt.Sprint(fc["name"]))
				if name == "" {
					name = "tool"
				}
				args := "{}"
				if rawArgs, exists := fc["args"]; exists && rawArgs != nil {
					switch v := rawArgs.(type) {
					case string:
						if strings.TrimSpace(v) != "" {
							args = v
						}
					default:
						if b, err := json.Marshal(v); err == nil && len(b) > 0 {
							args = string(b)
						}
					}
				}
				toolCalls = append(toolCalls, apicompat.ChatToolCall{
					ID:   tracker.idFor(name),
					Type: "function",
					Function: apicompat.ChatFunctionCall{
						Name:      name,
						Arguments: args,
					},
				})
				continue
			}
			if fr, ok := part["functionResponse"].(map[string]any); ok && fr != nil {
				name := strings.TrimSpace(fmt.Sprint(fr["name"]))
				if name == "" {
					name = "tool"
				}
				responseText := geminiNativeFunctionResponseText(fr["response"])
				responseContent, _ := json.Marshal(responseText)
				toolMessages = append(toolMessages, apicompat.ChatMessage{
					Role:       "tool",
					Content:    responseContent,
					ToolCallID: tracker.idFor(name),
				})
			}
		}

		if role == "assistant" && len(toolCalls) > 0 {
			msg := apicompat.ChatMessage{Role: role, ToolCalls: toolCalls}
			if text := strings.Join(textParts, "\n"); text != "" {
				msg.Content, _ = json.Marshal(text)
			}
			messages = append(messages, msg)
		} else if len(chatParts) > 0 || len(textParts) > 0 {
			msg := apicompat.ChatMessage{Role: role}
			if hasImage && role == "user" {
				msg.Content, _ = json.Marshal(chatParts)
			} else {
				msg.Content, _ = json.Marshal(strings.Join(textParts, "\n"))
			}
			messages = append(messages, msg)
		}
		messages = append(messages, toolMessages...)
	}
	return messages, nil
}

func geminiNativeRoleToChatRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "model", "assistant":
		return "assistant"
	case "system":
		return "system"
	case "tool", "function":
		return "tool"
	default:
		return "user"
	}
}

func geminiNativeSystemInstructionText(raw any) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		return strings.TrimSpace(strings.Join(geminiNativeTextParts(v["parts"]), "\n"))
	default:
		return ""
	}
}

func geminiNativeTextParts(raw any) []string {
	parts, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(parts))
	for _, rawPart := range parts {
		part, ok := rawPart.(map[string]any)
		if !ok {
			continue
		}
		if text, ok := part["text"].(string); ok && strings.TrimSpace(text) != "" {
			out = append(out, text)
		}
	}
	return out
}

func geminiNativeImageURLFromPart(part map[string]any) string {
	for _, key := range []string{"inlineData", "inline_data"} {
		if data, ok := part[key].(map[string]any); ok {
			mimeType := firstNonEmptyGeminiString(data["mimeType"], data["mime_type"])
			encoded := strings.TrimSpace(firstNonEmptyGeminiString(data["data"]))
			if encoded == "" {
				continue
			}
			if strings.HasPrefix(encoded, "data:") {
				return encoded
			}
			if mimeType == "" {
				mimeType = "image/png"
			}
			if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
				continue
			}
			return "data:" + mimeType + ";base64," + encoded
		}
	}

	for _, key := range []string{"fileData", "file_data"} {
		if data, ok := part[key].(map[string]any); ok {
			uri := strings.TrimSpace(firstNonEmptyGeminiString(data["fileUri"], data["file_uri"], data["uri"], data["url"]))
			if uri == "" {
				continue
			}
			mimeType := strings.ToLower(strings.TrimSpace(firstNonEmptyGeminiString(data["mimeType"], data["mime_type"])))
			if strings.HasPrefix(mimeType, "image/") || strings.HasPrefix(uri, "data:image/") || strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
				return uri
			}
		}
	}
	return ""
}

func firstNonEmptyGeminiString(values ...any) string {
	for _, value := range values {
		if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func geminiNativeFunctionResponseText(raw any) string {
	switch v := raw.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(b)
	}
}

func applyGeminiGenerationConfigToChatRequest(gen map[string]any, chatReq *apicompat.ChatCompletionsRequest) {
	if chatReq == nil {
		return
	}
	if v, ok := numericPointerFromAny(gen["temperature"]); ok {
		chatReq.Temperature = v
	}
	if v, ok := numericPointerFromAny(gen["topP"]); ok {
		chatReq.TopP = v
	}
	if v, ok := intPointerFromAny(gen["maxOutputTokens"]); ok {
		chatReq.MaxTokens = v
	}
	if effort := geminiThinkingConfigReasoningEffort(gen["thinkingConfig"]); effort != "" {
		chatReq.ReasoningEffort = effort
	}
	if stopRaw, ok := gen["stopSequences"]; ok {
		if b, err := json.Marshal(stopRaw); err == nil && len(b) > 0 && string(b) != "null" {
			chatReq.Stop = b
		}
	}
}

func geminiThinkingConfigReasoningEffort(raw any) string {
	cfg, ok := raw.(map[string]any)
	if !ok || cfg == nil {
		return ""
	}
	includeThoughts, hasIncludeThoughts := boolFromAny(cfg["includeThoughts"])
	budget, hasBudget := intFromAny(cfg["thinkingBudget"])
	if !hasIncludeThoughts && !hasBudget {
		return ""
	}
	if hasIncludeThoughts && !includeThoughts && (!hasBudget || budget <= 0) {
		return ""
	}
	if hasBudget {
		switch {
		case budget >= 32768:
			return "xhigh"
		case budget >= 8192:
			return "high"
		case budget >= 2048:
			return "medium"
		case budget > 0:
			return "low"
		default:
			return ""
		}
	}
	return "medium"
}

func numericPointerFromAny(v any) (*float64, bool) {
	switch n := v.(type) {
	case float64:
		return &n, true
	case float32:
		f := float64(n)
		return &f, true
	case int:
		f := float64(n)
		return &f, true
	case int64:
		f := float64(n)
		return &f, true
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return &f, true
		}
	}
	return nil, false
}

func boolFromAny(v any) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		switch strings.ToLower(strings.TrimSpace(b)) {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	}
	return false, false
}

func intFromAny(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i), true
		}
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return i, true
		}
	}
	return 0, false
}

func intPointerFromAny(v any) (*int, bool) {
	switch n := v.(type) {
	case float64:
		i := int(n)
		return &i, true
	case int:
		return &n, true
	case int64:
		i := int(n)
		return &i, true
	case json.Number:
		if i, err := n.Int64(); err == nil {
			out := int(i)
			return &out, true
		}
	}
	return nil, false
}

func geminiNativeToolsToChatTools(raw any) []apicompat.ChatTool {
	tools, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]apicompat.ChatTool, 0)
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		decls, ok := tool["functionDeclarations"].([]any)
		if !ok {
			continue
		}
		for _, rawDecl := range decls {
			decl, ok := rawDecl.(map[string]any)
			if !ok {
				continue
			}
			name := strings.TrimSpace(firstNonEmptyGeminiString(decl["name"]))
			if name == "" {
				continue
			}
			fn := &apicompat.ChatFunction{
				Name:        name,
				Description: firstNonEmptyGeminiString(decl["description"]),
			}
			if params, exists := decl["parameters"]; exists && params != nil {
				if b, err := json.Marshal(params); err == nil {
					fn.Parameters = b
				}
			}
			out = append(out, apicompat.ChatTool{Type: "function", Function: fn})
		}
	}
	return out
}

func geminiNativeToolChoiceToChatToolChoice(raw any) json.RawMessage {
	cfg, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	funcCfg, ok := cfg["functionCallingConfig"].(map[string]any)
	if !ok {
		return nil
	}
	mode := strings.ToUpper(strings.TrimSpace(firstNonEmptyGeminiString(funcCfg["mode"])))
	switch mode {
	case "NONE":
		b, _ := json.Marshal("none")
		return b
	case "ANY":
		allowed, _ := funcCfg["allowedFunctionNames"].([]any)
		if len(allowed) == 1 {
			name := strings.TrimSpace(fmt.Sprint(allowed[0]))
			if name != "" {
				b, _ := json.Marshal(map[string]any{
					"type": "function",
					"function": map[string]any{
						"name": name,
					},
				})
				return b
			}
		}
		b, _ := json.Marshal("required")
		return b
	default:
		return nil
	}
}

func (s *GeminiMessagesCompatService) bufferCompatibleRelayNativeChatCompletions(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	upstreamModel string,
	startTime time.Time,
	originalBody []byte,
) (*ForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			return nil, s.writeGoogleError(c, http.StatusBadGateway, "Failed to read upstream response")
		}
		return nil, fmt.Errorf("read compatible relay native response body: %w", err)
	}

	var chatResp apicompat.ChatCompletionsResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, s.writeGoogleError(c, http.StatusBadGateway, "Failed to parse upstream response")
	}
	geminiResp, usage := chatCompletionsResponseToGeminiNative(&chatResp, originalModel)

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.JSON(http.StatusOK, geminiResp)

	imageCount := 0
	imageInputSize := s.extractImageInputSize(originalBody)
	imageSize := normalizeOpenAIImageSizeTier(imageInputSize)
	if isImageGenerationModel(originalModel) {
		imageCount = 1
	}

	return &ForwardResult{
		RequestID:      requestID,
		Usage:          usage,
		Model:          originalModel,
		UpstreamModel:  upstreamModel,
		Stream:         false,
		Duration:       time.Since(startTime),
		ImageCount:     imageCount,
		ImageSize:      imageSize,
		ImageInputSize: imageInputSize,
	}, nil
}

func chatCompletionsResponseToGeminiNative(resp *apicompat.ChatCompletionsResponse, model string) (map[string]any, ClaudeUsage) {
	usage := claudeUsageFromChatUsage(nil)
	if resp != nil {
		usage = claudeUsageFromChatUsage(resp.Usage)
	}

	choice := apicompat.ChatChoice{}
	if resp != nil && len(resp.Choices) > 0 {
		choice = resp.Choices[0]
	}
	parts := chatMessageToGeminiNativeParts(choice.Message)
	if len(parts) == 0 {
		parts = []any{map[string]any{"text": ""}}
	}

	out := map[string]any{
		"candidates": []any{
			map[string]any{
				"content": map[string]any{
					"role":  "model",
					"parts": parts,
				},
				"finishReason": openAIFinishReasonToGemini(choice.FinishReason),
				"index":        0,
			},
		},
		"modelVersion": model,
	}
	if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		out["usageMetadata"] = geminiUsageMetadataFromClaudeUsage(usage)
	}
	return out, usage
}

func chatMessageToGeminiNativeParts(msg apicompat.ChatMessage) []any {
	parts := make([]any, 0)
	if len(msg.Content) > 0 && string(bytes.TrimSpace(msg.Content)) != "null" {
		var content any
		if err := json.Unmarshal(msg.Content, &content); err == nil {
			parts = append(parts, openAIChatContentToGeminiParts(content)...)
		} else if text := chatMessageContentText(msg.Content); text != "" {
			parts = append(parts, map[string]any{"text": text})
		}
	}
	for _, tc := range msg.ToolCalls {
		name := strings.TrimSpace(tc.Function.Name)
		if name == "" {
			name = "tool"
		}
		parts = append(parts, map[string]any{
			"functionCall": map[string]any{
				"name": name,
				"args": parseChatToolArguments(tc.Function.Arguments),
			},
		})
	}
	if msg.FunctionCall != nil {
		name := strings.TrimSpace(msg.FunctionCall.Name)
		if name == "" {
			name = "tool"
		}
		parts = append(parts, map[string]any{
			"functionCall": map[string]any{
				"name": name,
				"args": parseChatToolArguments(msg.FunctionCall.Arguments),
			},
		})
	}
	return parts
}

func chatMessageContentText(raw json.RawMessage) string {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var parts []apicompat.ChatContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		textParts := make([]string, 0, len(parts))
		for _, part := range parts {
			if part.Type == "text" && part.Text != "" {
				textParts = append(textParts, part.Text)
			}
		}
		return strings.Join(textParts, "\n")
	}
	return ""
}

func parseChatToolArguments(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
		return decoded
	}
	return map[string]any{"_arguments": raw}
}

func trimmedStringFromAny(raw any) string {
	s, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func normalizeGeminiNativeRequestBody(body []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse request body: %w", err)
	}

	if contents, ok := payload["contents"].([]any); ok && len(contents) > 0 {
		return body, nil
	}
	if _, ok := payload["messages"]; !ok {
		return body, nil
	}

	return openAIChatPayloadToGeminiNative(payload)
}

func openAIChatPayloadToGeminiNative(req map[string]any) ([]byte, error) {
	rawMessages, ok := req["messages"].([]any)
	if !ok {
		return nil, errors.New("OpenAI messages must be an array")
	}
	if len(rawMessages) == 0 {
		return nil, errors.New("OpenAI messages must include at least one message")
	}

	out := make(map[string]any)
	for _, key := range []string{"systemInstruction", "safetySettings", "cachedContent"} {
		if value, exists := req[key]; exists {
			out[key] = value
		}
	}

	systemTexts := make([]string, 0)
	if text := strings.TrimSpace(openAIChatContentText(req["instructions"])); text != "" {
		systemTexts = append(systemTexts, text)
	}

	toolCallIDToName := make(map[string]string)
	contents := make([]any, 0, len(rawMessages))
	for _, rawMessage := range rawMessages {
		message, ok := rawMessage.(map[string]any)
		if !ok {
			continue
		}

		role := strings.ToLower(trimmedStringFromAny(message["role"]))
		switch role {
		case "system", "developer":
			if text := strings.TrimSpace(openAIChatContentText(message["content"])); text != "" {
				systemTexts = append(systemTexts, text)
			}
			continue
		}

		geminiRole := "user"
		if role == "assistant" {
			geminiRole = "model"
		}

		parts := openAIChatMessageToGeminiParts(message, role, toolCallIDToName)
		if len(parts) == 0 {
			parts = append(parts, map[string]any{"text": ""})
		}
		contents = append(contents, map[string]any{
			"role":  geminiRole,
			"parts": parts,
		})
	}
	if len(contents) == 0 {
		return nil, errors.New("OpenAI messages must include at least one non-system message")
	}

	if _, exists := out["systemInstruction"]; !exists && len(systemTexts) > 0 {
		out["systemInstruction"] = map[string]any{
			"parts": []any{map[string]any{"text": strings.Join(systemTexts, "\n\n")}},
		}
	}
	out["contents"] = contents

	if generationConfig := openAIChatGenerationConfigToGemini(req); len(generationConfig) > 0 {
		out["generationConfig"] = generationConfig
	}
	if tools := openAIChatToolsToGeminiTools(req); len(tools) > 0 {
		out["tools"] = tools
	}
	if toolConfig, exists := req["toolConfig"]; exists {
		out["toolConfig"] = toolConfig
	} else if toolConfig := openAIChatToolChoiceToGeminiToolConfig(req["tool_choice"]); len(toolConfig) > 0 {
		out["toolConfig"] = toolConfig
	}

	return json.Marshal(out)
}

func openAIChatMessageToGeminiParts(message map[string]any, role string, toolCallIDToName map[string]string) []any {
	if role == "tool" || role == "function" {
		name := trimmedStringFromAny(message["name"])
		if name == "" {
			if id := trimmedStringFromAny(message["tool_call_id"]); id != "" {
				name = toolCallIDToName[id]
			}
		}
		if name == "" {
			name = "tool"
		}
		return []any{map[string]any{
			"functionResponse": map[string]any{
				"name": name,
				"response": map[string]any{
					"content": openAIChatContentText(message["content"]),
				},
			},
		}}
	}

	parts := openAIChatContentToGeminiParts(message["content"])
	if role == "assistant" {
		parts = append(parts, openAIChatFunctionCallsToGeminiParts(message, toolCallIDToName)...)
	}
	return parts
}

func openAIChatFunctionCallsToGeminiParts(message map[string]any, toolCallIDToName map[string]string) []any {
	parts := make([]any, 0)
	if rawCalls, ok := message["tool_calls"].([]any); ok {
		for _, rawCall := range rawCalls {
			call, ok := rawCall.(map[string]any)
			if !ok {
				continue
			}
			fn, _ := call["function"].(map[string]any)
			name := trimmedStringFromAny(fn["name"])
			if name == "" {
				name = "tool"
			}
			if id := trimmedStringFromAny(call["id"]); id != "" {
				toolCallIDToName[id] = name
			}
			parts = append(parts, map[string]any{
				"functionCall": map[string]any{
					"name": name,
					"args": openAIChatToolArguments(fn["arguments"]),
				},
			})
		}
	}

	if fn, ok := message["function_call"].(map[string]any); ok {
		name := trimmedStringFromAny(fn["name"])
		if name == "" {
			name = "tool"
		}
		parts = append(parts, map[string]any{
			"functionCall": map[string]any{
				"name": name,
				"args": openAIChatToolArguments(fn["arguments"]),
			},
		})
	}
	return parts
}

func openAIChatToolArguments(raw any) any {
	switch v := raw.(type) {
	case nil:
		return map[string]any{}
	case string:
		return parseChatToolArguments(v)
	default:
		return v
	}
}

func openAIChatContentToGeminiParts(content any) []any {
	switch v := content.(type) {
	case nil:
		return nil
	case string:
		return []any{map[string]any{"text": v}}
	case []any:
		parts := make([]any, 0, len(v))
		for _, rawPart := range v {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			partType := strings.ToLower(trimmedStringFromAny(part["type"]))
			switch partType {
			case "text", "input_text", "output_text":
				if text, ok := part["text"].(string); ok {
					parts = append(parts, map[string]any{"text": text})
				}
			case "image_url", "input_image", "image":
				if imagePart := openAIChatImagePartToGemini(part); imagePart != nil {
					parts = append(parts, imagePart)
				}
			default:
				if text, ok := part["text"].(string); ok {
					parts = append(parts, map[string]any{"text": text})
				}
			}
		}
		return parts
	default:
		if b, err := json.Marshal(v); err == nil {
			return []any{map[string]any{"text": string(b)}}
		}
		return []any{map[string]any{"text": fmt.Sprint(v)}}
	}
}

func openAIChatContentText(content any) string {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		texts := make([]string, 0, len(v))
		for _, rawPart := range v {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := part["text"].(string); ok && text != "" {
				texts = append(texts, text)
			}
		}
		return strings.Join(texts, "\n")
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
		return fmt.Sprint(v)
	}
}

func openAIChatImagePartToGemini(part map[string]any) map[string]any {
	if src, ok := part["source"].(map[string]any); ok {
		srcType := strings.ToLower(trimmedStringFromAny(src["type"]))
		mediaType := trimmedStringFromAny(src["media_type"])
		data := trimmedStringFromAny(src["data"])
		if srcType == "base64" && mediaType != "" && data != "" {
			return map[string]any{"inlineData": map[string]any{"mimeType": mediaType, "data": data}}
		}
	}

	imageURL := extractOpenAIChatImageURL(part)
	if imageURL == "" {
		return nil
	}
	if mimeType, data, ok := parseImageDataURL(imageURL); ok {
		return map[string]any{"inlineData": map[string]any{"mimeType": mimeType, "data": data}}
	}
	return map[string]any{"fileData": map[string]any{"mimeType": guessImageMimeType(imageURL), "fileUri": imageURL}}
}

func extractOpenAIChatImageURL(part map[string]any) string {
	for _, key := range []string{"image_url", "image", "url"} {
		value, exists := part[key]
		if !exists || value == nil {
			continue
		}
		switch v := value.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		case map[string]any:
			if url, ok := v["url"].(string); ok && strings.TrimSpace(url) != "" {
				return strings.TrimSpace(url)
			}
		}
	}
	return ""
}

func parseImageDataURL(raw string) (mimeType string, data string, ok bool) {
	if !strings.HasPrefix(raw, "data:") {
		return "", "", false
	}
	rest := strings.TrimPrefix(raw, "data:")
	semi := strings.Index(rest, ";")
	if semi < 0 {
		return "", "", false
	}
	mimeType = strings.TrimSpace(rest[:semi])
	rest = rest[semi+1:]
	if !strings.HasPrefix(rest, "base64,") {
		return "", "", false
	}
	data = strings.TrimSpace(strings.TrimPrefix(rest, "base64,"))
	if mimeType == "" || data == "" {
		return "", "", false
	}
	return mimeType, data, true
}

func guessImageMimeType(rawURL string) string {
	lower := strings.ToLower(rawURL)
	if idx := strings.IndexAny(lower, "?#"); idx >= 0 {
		lower = lower[:idx]
	}
	switch {
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	default:
		return "image/jpeg"
	}
}

func openAIChatGenerationConfigToGemini(req map[string]any) map[string]any {
	out := make(map[string]any)
	if existing, ok := req["generationConfig"].(map[string]any); ok {
		for key, value := range existing {
			out[key] = value
		}
	}
	if _, exists := out["temperature"]; !exists {
		if v, ok := numericValueFromAny(req["temperature"]); ok {
			out["temperature"] = v
		}
	}
	if _, exists := out["topP"]; !exists {
		if v, ok := numericValueFromAny(req["top_p"]); ok {
			out["topP"] = v
		}
	}
	if _, exists := out["maxOutputTokens"]; !exists {
		if v, ok := asInt(req["max_completion_tokens"]); ok && v > 0 {
			out["maxOutputTokens"] = v
		} else if v, ok := asInt(req["max_tokens"]); ok && v > 0 {
			out["maxOutputTokens"] = v
		}
	}
	if _, exists := out["stopSequences"]; !exists {
		if stopSeqs := openAIStopToGeminiStopSequences(req["stop"]); len(stopSeqs) > 0 {
			out["stopSequences"] = stopSeqs
		}
	}
	return out
}

func numericValueFromAny(v any) (float64, bool) {
	ptr, ok := numericPointerFromAny(v)
	if !ok || ptr == nil {
		return 0, false
	}
	return *ptr, true
}

func openAIStopToGeminiStopSequences(raw any) []any {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []any{v}
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func openAIChatToolsToGeminiTools(req map[string]any) []any {
	decls := make([]any, 0)
	if tools, ok := req["tools"].([]any); ok {
		for _, rawTool := range tools {
			tool, ok := rawTool.(map[string]any)
			if !ok {
				continue
			}
			if strings.ToLower(trimmedStringFromAny(tool["type"])) != "function" {
				continue
			}
			if decl := openAIChatFunctionToGeminiDeclaration(tool["function"]); decl != nil {
				decls = append(decls, decl)
			}
		}
	}
	if functions, ok := req["functions"].([]any); ok {
		for _, rawFunction := range functions {
			if decl := openAIChatFunctionToGeminiDeclaration(rawFunction); decl != nil {
				decls = append(decls, decl)
			}
		}
	}
	if len(decls) == 0 {
		return nil
	}
	return []any{map[string]any{"functionDeclarations": decls}}
}

func openAIChatFunctionToGeminiDeclaration(raw any) map[string]any {
	fn, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	name := trimmedStringFromAny(fn["name"])
	if name == "" {
		return nil
	}
	decl := map[string]any{"name": name}
	if desc, ok := fn["description"].(string); ok && desc != "" {
		decl["description"] = desc
	}
	if params, exists := fn["parameters"]; exists && params != nil {
		decl["parameters"] = params
	}
	return decl
}

func openAIChatToolChoiceToGeminiToolConfig(raw any) map[string]any {
	switch v := raw.(type) {
	case string:
		mode := ""
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "none":
			mode = "NONE"
		case "auto":
			mode = "AUTO"
		case "required":
			mode = "ANY"
		}
		if mode == "" {
			return nil
		}
		return map[string]any{"functionCallingConfig": map[string]any{"mode": mode}}
	case map[string]any:
		fn, _ := v["function"].(map[string]any)
		name := trimmedStringFromAny(fn["name"])
		if name == "" {
			return nil
		}
		return map[string]any{
			"functionCallingConfig": map[string]any{
				"mode":                 "ANY",
				"allowedFunctionNames": []string{name},
			},
		}
	default:
		return nil
	}
}

func openAIFinishReasonToGemini(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "length":
		return "MAX_TOKENS"
	case "content_filter":
		return "SAFETY"
	case "tool_calls", "function_call", "stop":
		return "STOP"
	default:
		return "STOP"
	}
}

func claudeUsageFromChatUsage(usage *apicompat.ChatUsage) ClaudeUsage {
	if usage == nil {
		return ClaudeUsage{}
	}
	out := ClaudeUsage{
		InputTokens:  usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
	}
	if usage.PromptTokensDetails != nil {
		out.CacheReadInputTokens = usage.PromptTokensDetails.CachedTokens
	}
	return out
}

func geminiUsageMetadataFromClaudeUsage(usage ClaudeUsage) map[string]any {
	meta := map[string]any{
		"promptTokenCount":     usage.InputTokens,
		"candidatesTokenCount": usage.OutputTokens,
		"totalTokenCount":      usage.InputTokens + usage.OutputTokens,
	}
	if usage.CacheReadInputTokens > 0 {
		meta["cachedContentTokenCount"] = usage.CacheReadInputTokens
	}
	return meta
}

func (s *GeminiMessagesCompatService) streamCompatibleRelayNativeChatCompletions(
	c *gin.Context,
	resp *http.Response,
	account *Account,
	originalModel string,
	upstreamModel string,
	startTime time.Time,
	originalBody []byte,
) (*ForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	var usage ClaudeUsage
	var firstTokenMs *int
	clientDisconnected := false
	toolState := newOpenAIChatToolCallStreamState()
	wroteAny := false
	pendingFinishReason := ""

	writeEvent := func(payload map[string]any) {
		if clientDisconnected {
			return
		}
		b, err := json.Marshal(payload)
		if err != nil {
			return
		}
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", b); err != nil {
			clientDisconnected = true
			return
		}
		wroteAny = true
		flusher.Flush()
	}

	for scanner.Scan() {
		line := scanner.Text()
		payload, ok := extractOpenAISSEDataLine(line)
		if !ok {
			continue
		}
		payload = strings.TrimSpace(payload)
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}

		if u := extractCCStreamUsage(payload); u != nil {
			usage = claudeUsageFromOpenAIUsage(*u)
			continue
		}

		var chunk apicompat.ChatCompletionsChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != nil && *choice.Delta.Content != "" {
				if firstTokenMs == nil {
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
				}
				textChunks := splitGeminiNativeStreamText(*choice.Delta.Content)
				for i, textChunk := range textChunks {
					writeEvent(geminiNativeTextStreamPayload(textChunk))
					if clientDisconnected {
						break
					}
					if i < len(textChunks)-1 && geminiNativeCompatibleRelayChunkDelay > 0 {
						select {
						case <-c.Request.Context().Done():
							clientDisconnected = true
						case <-time.After(geminiNativeCompatibleRelayChunkDelay):
						}
					}
				}
			}
			if len(choice.Delta.ToolCalls) > 0 {
				toolState.observe(choice.Delta.ToolCalls)
			}
			if choice.FinishReason != nil {
				if strings.EqualFold(strings.TrimSpace(*choice.FinishReason), "tool_calls") {
					for _, part := range toolState.parts() {
						if firstTokenMs == nil {
							ms := int(time.Since(startTime).Milliseconds())
							firstTokenMs = &ms
						}
						writeEvent(geminiNativePartsStreamPayload([]any{part}, "STOP"))
					}
				}
				pendingFinishReason = openAIFinishReasonToGemini(*choice.FinishReason)
			}
		}
	}

	if pendingFinishReason != "" {
		writeEvent(geminiNativeFinishStreamPayload(pendingFinishReason, usage))
	} else if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		writeEvent(map[string]any{"usageMetadata": geminiUsageMetadataFromClaudeUsage(usage)})
	}

	if err := scanner.Err(); err != nil {
		if !wroteAny && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			return nil, s.writeGoogleError(c, http.StatusBadGateway, "Failed to read upstream stream")
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
		}, fmt.Errorf("read compatible relay native stream: %w", err)
	}

	imageCount := 0
	imageInputSize := s.extractImageInputSize(originalBody)
	imageSize := normalizeOpenAIImageSizeTier(imageInputSize)
	if isImageGenerationModel(originalModel) {
		imageCount = 1
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
		ImageCount:       imageCount,
		ImageSize:        imageSize,
		ImageInputSize:   imageInputSize,
	}, nil
}

func geminiNativeTextStreamPayload(text string) map[string]any {
	return geminiNativePartsStreamPayload([]any{map[string]any{"text": text}}, "")
}

func splitGeminiNativeStreamText(text string) []string {
	if text == "" {
		return nil
	}
	runes := []rune(text)
	if len(runes) <= geminiNativeCompatibleRelayTextChunkRunes {
		return []string{text}
	}

	chunks := make([]string, 0, (len(runes)+geminiNativeCompatibleRelayTextChunkRunes-1)/geminiNativeCompatibleRelayTextChunkRunes)
	for start := 0; start < len(runes); start += geminiNativeCompatibleRelayTextChunkRunes {
		end := start + geminiNativeCompatibleRelayTextChunkRunes
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func geminiNativePartsStreamPayload(parts []any, finishReason string) map[string]any {
	candidate := map[string]any{
		"content": map[string]any{
			"role":  "model",
			"parts": parts,
		},
		"index": 0,
	}
	if finishReason != "" {
		candidate["finishReason"] = finishReason
	}
	return map[string]any{
		"candidates": []any{candidate},
	}
}

func geminiNativeFinishStreamPayload(finishReason string, usage ClaudeUsage) map[string]any {
	payload := map[string]any{
		"candidates": []any{
			map[string]any{
				"finishReason": finishReason,
				"index":        0,
			},
		},
	}
	if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		payload["usageMetadata"] = geminiUsageMetadataFromClaudeUsage(usage)
	}
	return payload
}

type openAIChatToolCallStreamState struct {
	calls map[int]*openAIChatToolCallStreamItem
	order []int
}

type openAIChatToolCallStreamItem struct {
	id        string
	name      string
	arguments strings.Builder
}

func newOpenAIChatToolCallStreamState() *openAIChatToolCallStreamState {
	return &openAIChatToolCallStreamState{calls: make(map[int]*openAIChatToolCallStreamItem)}
}

func (s *openAIChatToolCallStreamState) observe(calls []apicompat.ChatToolCall) {
	for fallbackIndex, call := range calls {
		index := fallbackIndex
		if call.Index != nil {
			index = *call.Index
		}
		item := s.calls[index]
		if item == nil {
			item = &openAIChatToolCallStreamItem{}
			s.calls[index] = item
			s.order = append(s.order, index)
		}
		if strings.TrimSpace(call.ID) != "" {
			item.id = call.ID
		}
		if strings.TrimSpace(call.Function.Name) != "" {
			item.name = call.Function.Name
		}
		if call.Function.Arguments != "" {
			_, _ = item.arguments.WriteString(call.Function.Arguments)
		}
	}
}

func (s *openAIChatToolCallStreamState) parts() []any {
	if s == nil || len(s.order) == 0 {
		return nil
	}
	parts := make([]any, 0, len(s.order))
	for _, index := range s.order {
		item := s.calls[index]
		if item == nil {
			continue
		}
		name := strings.TrimSpace(item.name)
		if name == "" {
			name = "tool"
		}
		parts = append(parts, map[string]any{
			"functionCall": map[string]any{
				"name": name,
				"args": parseChatToolArguments(item.arguments.String()),
			},
		})
	}
	return parts
}

func isGeminiSignatureRelatedError(respBody []byte) bool {
	msg := strings.ToLower(strings.TrimSpace(extractAntigravityErrorMessage(respBody)))
	if msg == "" {
		msg = strings.ToLower(string(respBody))
	}
	return strings.Contains(msg, "thought_signature") || strings.Contains(msg, "signature")
}

func (s *GeminiMessagesCompatService) ForwardNative(ctx context.Context, c *gin.Context, account *Account, originalModel string, action string, stream bool, body []byte) (*ForwardResult, error) {
	account = NormalizeGeminiAPIKeyAccount(account)
	startTime := time.Now()

	if strings.TrimSpace(originalModel) == "" {
		return nil, s.writeGoogleError(c, http.StatusBadRequest, "Missing model in URL")
	}
	if strings.TrimSpace(action) == "" {
		return nil, s.writeGoogleError(c, http.StatusBadRequest, "Missing action in URL")
	}
	if len(body) == 0 {
		return nil, s.writeGoogleError(c, http.StatusBadRequest, "Request body is empty")
	}

	if normalizedBody, err := normalizeGeminiNativeRequestBody(body); err != nil {
		return nil, s.writeGoogleError(c, http.StatusBadRequest, err.Error())
	} else {
		body = normalizedBody
	}

	// 过滤掉 parts 为空的消息（Gemini API 不接受空 parts）
	if filteredBody, err := filterEmptyPartsFromGeminiRequest(body); err == nil {
		body = filteredBody
	}

	switch action {
	case "generateContent", "streamGenerateContent", "countTokens":
		// ok
	default:
		return nil, s.writeGoogleError(c, http.StatusNotFound, "Unsupported action: "+action)
	}

	// Some Gemini upstreams validate tool call parts strictly; ensure any `functionCall` part includes a
	// `thoughtSignature` to avoid frequent INVALID_ARGUMENT 400s.
	body = ensureGeminiFunctionCallThoughtSignatures(body)

	mappedModel := originalModel
	if account.Type == AccountTypeAPIKey || account.Type == AccountTypeServiceAccount {
		mappedModel = account.GetMappedModel(originalModel)
	}

	if account != nil && account.IsGeminiOpenAICompatibleUpstream() {
		result, err := s.forwardCompatibleRelayNativeAsChatCompletions(ctx, c, account, originalModel, mappedModel, action, stream, body, startTime)
		if !errors.Is(err, errGeminiCompatibleRelayOpenAIPathUnsupported) {
			return result, err
		}
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	useUpstreamStream := stream
	upstreamAction := action
	if account.Type == AccountTypeOAuth && !stream && action == "generateContent" && strings.TrimSpace(account.GetCredential("project_id")) != "" {
		// Code Assist's non-streaming generateContent may return no content; use streaming upstream and aggregate.
		useUpstreamStream = true
		upstreamAction = "streamGenerateContent"
	}
	forceAIStudio := action == "countTokens"

	var requestIDHeader string
	var buildReq func(ctx context.Context) (*http.Request, string, error)

	switch account.Type {
	case AccountTypeAPIKey:
		buildReq = func(ctx context.Context) (*http.Request, string, error) {
			apiKey := account.GetCredential("api_key")
			if strings.TrimSpace(apiKey) == "" {
				return nil, "", errors.New("gemini api_key not configured")
			}

			baseURL := account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
			baseURL = geminiNativeBaseURLFromOpenAICompatible(baseURL)
			normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, "", err
			}

			fullURL := fmt.Sprintf("%s/v1beta/models/%s:%s", strings.TrimRight(normalizedBaseURL, "/"), mappedModel, upstreamAction)
			if useUpstreamStream {
				fullURL += "?alt=sse"
			}

			upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(body))
			if err != nil {
				return nil, "", err
			}
			upstreamReq.Header.Set("Content-Type", "application/json")
			upstreamReq.Header.Set("x-goog-api-key", apiKey)
			return upstreamReq, "x-request-id", nil
		}
		requestIDHeader = "x-request-id"

	case AccountTypeOAuth:
		buildReq = func(ctx context.Context) (*http.Request, string, error) {
			if s.tokenProvider == nil {
				return nil, "", errors.New("gemini token provider not configured")
			}
			accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
			if err != nil {
				return nil, "", err
			}

			projectID := strings.TrimSpace(account.GetCredential("project_id"))

			// Two modes for OAuth:
			// 1. With project_id -> Code Assist API (wrapped request)
			// 2. Without project_id -> AI Studio API (direct OAuth, like API key but with Bearer token)
			if projectID != "" && !forceAIStudio {
				// Mode 1: Code Assist API
				baseURL, err := s.validateUpstreamBaseURL(geminicli.GeminiCliBaseURL)
				if err != nil {
					return nil, "", err
				}
				fullURL := fmt.Sprintf("%s/v1internal:%s", strings.TrimRight(baseURL, "/"), upstreamAction)
				if useUpstreamStream {
					fullURL += "?alt=sse"
				}

				wrapped := map[string]any{
					"model":   mappedModel,
					"project": projectID,
				}
				var inner any
				if err := json.Unmarshal(body, &inner); err != nil {
					return nil, "", fmt.Errorf("failed to parse gemini request: %w", err)
				}
				wrapped["request"] = inner
				wrappedBytes, _ := json.Marshal(wrapped)

				upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(wrappedBytes))
				if err != nil {
					return nil, "", err
				}
				upstreamReq.Header.Set("Content-Type", "application/json")
				upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
				upstreamReq.Header.Set("User-Agent", geminicli.GeminiCLIUserAgent)
				return upstreamReq, "x-request-id", nil
			} else {
				// Mode 2: AI Studio API with OAuth (like API key mode, but using Bearer token)
				baseURL := account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
				baseURL = geminiNativeBaseURLFromOpenAICompatible(baseURL)
				normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
				if err != nil {
					return nil, "", err
				}

				fullURL := fmt.Sprintf("%s/v1beta/models/%s:%s", strings.TrimRight(normalizedBaseURL, "/"), mappedModel, upstreamAction)
				if useUpstreamStream {
					fullURL += "?alt=sse"
				}

				upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(body))
				if err != nil {
					return nil, "", err
				}
				upstreamReq.Header.Set("Content-Type", "application/json")
				upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
				return upstreamReq, "x-request-id", nil
			}
		}
		requestIDHeader = "x-request-id"

	case AccountTypeServiceAccount:
		buildReq = func(ctx context.Context) (*http.Request, string, error) {
			if s.tokenProvider == nil {
				return nil, "", errors.New("gemini token provider not configured")
			}
			accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
			if err != nil {
				return nil, "", err
			}

			fullURL, err := buildVertexGeminiURL(account.VertexProjectID(), account.VertexLocation(mappedModel), mappedModel, upstreamAction, useUpstreamStream)
			if err != nil {
				return nil, "", err
			}

			upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(body))
			if err != nil {
				return nil, "", err
			}
			upstreamReq.Header.Set("Content-Type", "application/json")
			upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
			return upstreamReq, "x-request-id", nil
		}
		requestIDHeader = "x-request-id"

	default:
		return nil, s.writeGoogleError(c, http.StatusBadGateway, "Unsupported account type: "+account.Type)
	}

	var resp *http.Response
	for attempt := 1; attempt <= geminiMaxRetries; attempt++ {
		upstreamReq, idHeader, err := buildReq(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			// Local build error: don't retry.
			if strings.Contains(err.Error(), "missing project_id") {
				return nil, s.writeGoogleError(c, http.StatusBadRequest, err.Error())
			}
			return nil, s.writeGoogleError(c, http.StatusBadGateway, err.Error())
		}
		requestIDHeader = idHeader

		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
		if err != nil {
			safeErr := sanitizeUpstreamErrorMessage(err.Error())
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: 0,
				Kind:               "request_error",
				Message:            safeErr,
			})
			if attempt < geminiMaxRetries {
				logger.LegacyPrintf("service.gemini_messages_compat", "Gemini account %d: upstream request failed, retry %d/%d: %v", account.ID, attempt, geminiMaxRetries, err)
				sleepGeminiBackoff(attempt)
				continue
			}
			if action == "countTokens" {
				estimated := estimateGeminiCountTokens(body)
				c.JSON(http.StatusOK, map[string]any{"totalTokens": estimated})
				return &ForwardResult{
					RequestID:     "",
					Usage:         ClaudeUsage{},
					Model:         originalModel,
					UpstreamModel: mappedModel,
					Stream:        false,
					Duration:      time.Since(startTime),
					FirstTokenMs:  nil,
				}, nil
			}
			setOpsUpstreamError(c, 0, safeErr, "")
			return nil, s.writeGoogleError(c, http.StatusBadGateway, "Upstream request failed after retries: "+safeErr)
		}

		// 错误策略优先：匹配则跳过重试直接处理。
		if matched, rebuilt := s.checkErrorPolicyInLoop(ctx, account, resp); matched {
			resp = rebuilt
			break
		} else {
			resp = rebuilt
		}

		if resp.StatusCode >= 400 && s.shouldRetryGeminiUpstreamError(account, resp.StatusCode) {
			respBody := s.readUpstreamErrorBody(resp)
			_ = resp.Body.Close()
			// Don't treat insufficient-scope as transient.
			if resp.StatusCode == 403 && isGeminiInsufficientScope(resp.Header, respBody) {
				resp = &http.Response{
					StatusCode: resp.StatusCode,
					Header:     resp.Header.Clone(),
					Body:       io.NopCloser(bytes.NewReader(respBody)),
				}
				break
			}
			if resp.StatusCode == 429 {
				s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
			}
			if attempt < geminiMaxRetries {
				upstreamReqID := resp.Header.Get(requestIDHeader)
				if upstreamReqID == "" {
					upstreamReqID = resp.Header.Get("x-goog-request-id")
				}
				upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
				upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
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
					UpstreamRequestID:  upstreamReqID,
					Kind:               "retry",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})

				logger.LegacyPrintf("service.gemini_messages_compat", "Gemini account %d: upstream status %d, retry %d/%d", account.ID, resp.StatusCode, attempt, geminiMaxRetries)
				sleepGeminiBackoff(attempt)
				continue
			}
			if action == "countTokens" {
				estimated := estimateGeminiCountTokens(body)
				c.JSON(http.StatusOK, map[string]any{"totalTokens": estimated})
				return &ForwardResult{
					RequestID:     "",
					Usage:         ClaudeUsage{},
					Model:         originalModel,
					UpstreamModel: mappedModel,
					Stream:        false,
					Duration:      time.Since(startTime),
					FirstTokenMs:  nil,
				}, nil
			}
			// Final attempt: surface the upstream error body (passed through below) instead of a generic retry error.
			resp = &http.Response{
				StatusCode: resp.StatusCode,
				Header:     resp.Header.Clone(),
				Body:       io.NopCloser(bytes.NewReader(respBody)),
			}
			break
		}

		break
	}
	defer func() { _ = resp.Body.Close() }()

	requestID := resp.Header.Get(requestIDHeader)
	if requestID == "" {
		requestID = resp.Header.Get("x-goog-request-id")
	}
	if requestID != "" {
		c.Header("x-request-id", requestID)
	}

	isOAuth := account.Type == AccountTypeOAuth

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		// Best-effort fallback for OAuth tokens missing AI Studio scopes when calling countTokens.
		// This avoids Gemini SDKs failing hard during preflight token counting.
		// Checked before error policy so it always works regardless of custom error codes.
		if action == "countTokens" && isOAuth && isGeminiInsufficientScope(resp.Header, respBody) {
			estimated := estimateGeminiCountTokens(body)
			c.JSON(http.StatusOK, map[string]any{"totalTokens": estimated})
			return &ForwardResult{
				RequestID:     requestID,
				Usage:         ClaudeUsage{},
				Model:         originalModel,
				UpstreamModel: mappedModel,
				Stream:        false,
				Duration:      time.Since(startTime),
				FirstTokenMs:  nil,
			}, nil
		}

		// 统一错误策略：自定义错误码 + 临时不可调度
		if s.rateLimitService != nil {
			switch s.rateLimitService.CheckErrorPolicy(ctx, account, resp.StatusCode, respBody) {
			case ErrorPolicySkipped:
				respBody = unwrapIfNeeded(isOAuth, respBody)
				contentType := resp.Header.Get("Content-Type")
				if contentType == "" {
					contentType = "application/json"
				}
				MarkResponseCommitted(c)
				c.Data(http.StatusInternalServerError, contentType, respBody)
				return nil, fmt.Errorf("gemini upstream error: %d (skipped by error policy)", resp.StatusCode)
			case ErrorPolicyMatched, ErrorPolicyTempUnscheduled:
				s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
				evBody := unwrapIfNeeded(isOAuth, respBody)
				upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(evBody))
				upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
				upstreamDetail := ""
				if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
					maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
					if maxBytes <= 0 {
						maxBytes = 2048
					}
					upstreamDetail = truncateString(string(evBody), maxBytes)
				}
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  requestID,
					Kind:               "failover",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})
				return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody}
			}
		}

		// ErrorPolicyNone → 原有逻辑
		s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		// 精确匹配服务端配置类 400 错误，触发 failover + 临时封禁
		if resp.StatusCode == http.StatusBadRequest {
			msg400 := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
			if isGoogleProjectConfigError(msg400) {
				evBody := unwrapIfNeeded(isOAuth, respBody)
				upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(evBody)))
				upstreamDetail := ""
				if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
					maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
					if maxBytes <= 0 {
						maxBytes = 2048
					}
					upstreamDetail = truncateString(string(evBody), maxBytes)
				}
				log.Printf("[Gemini] status=400 google_config_error failover=true upstream_message=%q account=%d", upstreamMsg, account.ID)
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  requestID,
					Kind:               "failover",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})
				return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: evBody, RetryableOnSameAccount: true}
			}
		}
		if s.shouldFailoverGeminiUpstreamError(resp.StatusCode) {
			evBody := unwrapIfNeeded(isOAuth, respBody)
			upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(evBody))
			upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
			upstreamDetail := ""
			if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
				maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
				if maxBytes <= 0 {
					maxBytes = 2048
				}
				upstreamDetail = truncateString(string(evBody), maxBytes)
			}
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  requestID,
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
			})
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: evBody}
		}

		respBody = unwrapIfNeeded(isOAuth, respBody)
		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		upstreamDetail := ""
		if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
			maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
			if maxBytes <= 0 {
				maxBytes = 2048
			}
			upstreamDetail = truncateString(string(respBody), maxBytes)
			logger.LegacyPrintf("service.gemini_messages_compat", "[Gemini] native upstream error %d: %s", resp.StatusCode, truncateForLog(respBody, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes))
		}
		setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  requestID,
			Kind:               "http_error",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
		})

		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/json"
		}
		MarkResponseCommitted(c)
		c.Data(resp.StatusCode, contentType, respBody)
		if upstreamMsg == "" {
			return nil, fmt.Errorf("gemini upstream error: %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("gemini upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
	}

	var usage *ClaudeUsage
	var firstTokenMs *int

	if stream {
		streamRes, err := s.handleNativeStreamingResponse(c, resp, startTime, isOAuth)
		if err != nil {
			return nil, err
		}
		usage = streamRes.usage
		firstTokenMs = streamRes.firstTokenMs
	} else {
		if useUpstreamStream {
			collected, usageObj, err := collectGeminiSSE(resp.Body, isOAuth)
			if err != nil {
				return nil, s.writeGoogleError(c, http.StatusBadGateway, "Failed to read upstream stream")
			}
			b, _ := json.Marshal(collected)
			c.Data(http.StatusOK, "application/json", b)
			usage = usageObj
		} else {
			usageResp, err := s.handleNativeNonStreamingResponse(c, resp, isOAuth)
			if err != nil {
				return nil, err
			}
			usage = usageResp
		}
	}

	if usage == nil {
		usage = &ClaudeUsage{}
	}

	// 图片生成计费
	imageCount := 0
	imageInputSize := s.extractImageInputSize(body)
	imageSize := normalizeOpenAIImageSizeTier(imageInputSize)
	if isImageGenerationModel(originalModel) {
		imageCount = 1
	}

	return &ForwardResult{
		RequestID:      requestID,
		Usage:          *usage,
		Model:          originalModel,
		UpstreamModel:  mappedModel,
		Stream:         stream,
		Duration:       time.Since(startTime),
		FirstTokenMs:   firstTokenMs,
		ImageCount:     imageCount,
		ImageSize:      imageSize,
		ImageInputSize: imageInputSize,
	}, nil
}

// checkErrorPolicyInLoop 在重试循环内预检查错误策略。
// 返回 true 表示策略已匹配（调用者应 break），resp 已重建可直接使用。
// 返回 false 表示 ErrorPolicyNone，resp 已重建，调用者继续走重试逻辑。
func (s *GeminiMessagesCompatService) checkErrorPolicyInLoop(
	ctx context.Context, account *Account, resp *http.Response,
) (matched bool, rebuilt *http.Response) {
	if resp.StatusCode < 400 || s.rateLimitService == nil {
		return false, resp
	}
	body := s.readUpstreamErrorBody(resp)
	_ = resp.Body.Close()
	rebuilt = &http.Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	policy := s.rateLimitService.CheckErrorPolicy(ctx, account, resp.StatusCode, body)
	return policy != ErrorPolicyNone, rebuilt
}

func (s *GeminiMessagesCompatService) shouldRetryGeminiUpstreamError(account *Account, statusCode int) bool {
	switch statusCode {
	case 429, 500, 502, 503, 504, 529:
		return true
	case 403:
		// GeminiCli OAuth occasionally returns 403 transiently (activation/quota propagation); allow retry.
		if account == nil || account.Type != AccountTypeOAuth {
			return false
		}
		oauthType := strings.ToLower(strings.TrimSpace(account.GetCredential("oauth_type")))
		if oauthType == "" && strings.TrimSpace(account.GetCredential("project_id")) != "" {
			// Legacy/implicit Code Assist OAuth accounts.
			oauthType = "code_assist"
		}
		return oauthType == "code_assist"
	default:
		return false
	}
}

func (s *GeminiMessagesCompatService) shouldFailoverGeminiUpstreamError(statusCode int) bool {
	switch statusCode {
	case 401, 403, 429, 529:
		return true
	default:
		return statusCode >= 500
	}
}

func sleepGeminiBackoff(attempt int) {
	delay := geminiRetryBaseDelay * time.Duration(1<<uint(attempt-1))
	if delay > geminiRetryMaxDelay {
		delay = geminiRetryMaxDelay
	}

	// +/- 20% jitter
	r := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	jitter := time.Duration(float64(delay) * 0.2 * (r.Float64()*2 - 1))
	sleepFor := delay + jitter
	if sleepFor < 0 {
		sleepFor = 0
	}
	time.Sleep(sleepFor)
}

var (
	sensitiveQueryParamRegex = regexp.MustCompile(`(?i)([?&](?:key|client_secret|access_token|refresh_token)=)[^&"\s]+`)
	retryInRegex             = regexp.MustCompile(`Please retry in ([0-9.]+)s`)
)

func sanitizeUpstreamErrorMessage(msg string) string {
	if msg == "" {
		return msg
	}
	return sensitiveQueryParamRegex.ReplaceAllString(msg, `$1***`)
}

func (s *GeminiMessagesCompatService) writeGeminiMappedError(c *gin.Context, account *Account, upstreamStatus int, upstreamRequestID string, body []byte) error {
	MarkResponseCommitted(c)
	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(body))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, upstreamStatus, upstreamMsg, upstreamDetail)
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: upstreamStatus,
		UpstreamRequestID:  upstreamRequestID,
		Kind:               "http_error",
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})

	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		logger.LegacyPrintf("service.gemini_messages_compat", "[Gemini] upstream error %d: %s", upstreamStatus, truncateForLog(body, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes))
	}

	if status, errType, errMsg, matched := applyErrorPassthroughRule(
		c,
		PlatformGemini,
		upstreamStatus,
		body,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	); matched {
		c.JSON(status, gin.H{
			"type":  "error",
			"error": gin.H{"type": errType, "message": errMsg},
		})
		if upstreamMsg == "" {
			upstreamMsg = errMsg
		}
		if upstreamMsg == "" {
			return fmt.Errorf("upstream error: %d (passthrough rule matched)", upstreamStatus)
		}
		return fmt.Errorf("upstream error: %d (passthrough rule matched) message=%s", upstreamStatus, upstreamMsg)
	}

	var statusCode int
	var errType, errMsg string

	if mapped := mapGeminiErrorBodyToClaudeError(body); mapped != nil {
		errType = mapped.Type
		if mapped.Message != "" {
			errMsg = mapped.Message
		}
		if mapped.StatusCode > 0 {
			statusCode = mapped.StatusCode
		}
	}

	switch upstreamStatus {
	case 400:
		if statusCode == 0 {
			statusCode = http.StatusBadRequest
		}
		if errType == "" {
			errType = "invalid_request_error"
		}
		if errMsg == "" {
			errMsg = "Invalid request"
		}
	case 401:
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
		}
		if errType == "" {
			errType = "authentication_error"
		}
		if errMsg == "" {
			errMsg = "Upstream authentication failed, please contact administrator"
		}
	case 403:
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
		}
		if errType == "" {
			errType = "permission_error"
		}
		if errMsg == "" {
			errMsg = "Upstream access forbidden, please contact administrator"
		}
	case 404:
		if statusCode == 0 {
			statusCode = http.StatusNotFound
		}
		if errType == "" {
			errType = "not_found_error"
		}
		if errMsg == "" {
			errMsg = "Resource not found"
		}
	case 429:
		if statusCode == 0 {
			statusCode = http.StatusTooManyRequests
		}
		if errType == "" {
			errType = "rate_limit_error"
		}
		if errMsg == "" {
			errMsg = "Upstream rate limit exceeded, please retry later"
		}
	case 529:
		if statusCode == 0 {
			statusCode = http.StatusServiceUnavailable
		}
		if errType == "" {
			errType = "overloaded_error"
		}
		if errMsg == "" {
			errMsg = "Upstream service overloaded, please retry later"
		}
	case 500, 502, 503, 504:
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
		}
		if errType == "" {
			switch upstreamStatus {
			case 504:
				errType = "timeout_error"
			case 503:
				errType = "overloaded_error"
			default:
				errType = "api_error"
			}
		}
		if errMsg == "" {
			errMsg = "Upstream service temporarily unavailable"
		}
	default:
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
		}
		if errType == "" {
			errType = "upstream_error"
		}
		if errMsg == "" {
			errMsg = "Upstream request failed"
		}
	}

	c.JSON(statusCode, gin.H{
		"type":  "error",
		"error": gin.H{"type": errType, "message": errMsg},
	})
	if upstreamMsg == "" {
		return fmt.Errorf("upstream error: %d", upstreamStatus)
	}
	return fmt.Errorf("upstream error: %d message=%s", upstreamStatus, upstreamMsg)
}

type claudeErrorMapping struct {
	Type       string
	Message    string
	StatusCode int
}

func mapGeminiErrorBodyToClaudeError(body []byte) *claudeErrorMapping {
	if len(body) == 0 {
		return nil
	}

	var parsed struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}
	if strings.TrimSpace(parsed.Error.Status) == "" && parsed.Error.Code == 0 && strings.TrimSpace(parsed.Error.Message) == "" {
		return nil
	}

	mapped := &claudeErrorMapping{
		Type:    mapGeminiStatusToClaudeErrorType(parsed.Error.Status),
		Message: "",
	}
	if mapped.Type == "" {
		mapped.Type = "upstream_error"
	}

	switch strings.ToUpper(strings.TrimSpace(parsed.Error.Status)) {
	case "INVALID_ARGUMENT":
		mapped.StatusCode = http.StatusBadRequest
	case "NOT_FOUND":
		mapped.StatusCode = http.StatusNotFound
	case "RESOURCE_EXHAUSTED":
		mapped.StatusCode = http.StatusTooManyRequests
	default:
		// Keep StatusCode unset and let HTTP status mapping decide.
	}

	// Keep messages generic by default; upstream error message can be long or include sensitive fragments.
	return mapped
}

func mapGeminiStatusToClaudeErrorType(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "INVALID_ARGUMENT":
		return "invalid_request_error"
	case "PERMISSION_DENIED":
		return "permission_error"
	case "NOT_FOUND":
		return "not_found_error"
	case "RESOURCE_EXHAUSTED":
		return "rate_limit_error"
	case "UNAUTHENTICATED":
		return "authentication_error"
	case "UNAVAILABLE":
		return "overloaded_error"
	case "INTERNAL":
		return "api_error"
	case "DEADLINE_EXCEEDED":
		return "timeout_error"
	default:
		return ""
	}
}

type geminiStreamResult struct {
	usage        *ClaudeUsage
	firstTokenMs *int
}

func (s *GeminiMessagesCompatService) handleNonStreamingResponse(c *gin.Context, resp *http.Response, originalModel string) (*ClaudeUsage, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Failed to read upstream response")
	}

	unwrappedBody, err := unwrapGeminiResponse(body)
	if err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Failed to parse upstream response")
	}

	var geminiResp map[string]any
	if err := json.Unmarshal(unwrappedBody, &geminiResp); err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Failed to parse upstream response")
	}

	claudeResp, usage := convertGeminiToClaudeMessage(geminiResp, originalModel, unwrappedBody)
	c.JSON(http.StatusOK, claudeResp)

	return usage, nil
}

func (s *GeminiMessagesCompatService) handleStreamingResponse(c *gin.Context, resp *http.Response, startTime time.Time, originalModel string) (*geminiStreamResult, error) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	messageID := "msg_" + randomHex(12)
	messageStart := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         originalModel,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]any{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	}
	writeSSE(c.Writer, "message_start", messageStart)
	flusher.Flush()

	var firstTokenMs *int
	var usage ClaudeUsage
	finishReason := ""
	sawToolUse := false

	nextBlockIndex := 0
	openBlockIndex := -1
	openBlockType := ""
	seenText := ""
	openToolIndex := -1
	openToolID := ""
	openToolName := ""
	seenToolJSON := ""

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("stream read error: %w", err)
		}

		if !strings.HasPrefix(line, "data:") {
			if errors.Is(err, io.EOF) {
				break
			}
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			if errors.Is(err, io.EOF) {
				break
			}
			continue
		}

		unwrappedBytes, err := unwrapGeminiResponse([]byte(payload))
		if err != nil {
			continue
		}

		var geminiResp map[string]any
		if err := json.Unmarshal(unwrappedBytes, &geminiResp); err != nil {
			continue
		}

		if fr := extractGeminiFinishReason(geminiResp); fr != "" {
			finishReason = fr
		}

		parts := extractGeminiParts(geminiResp)
		for _, part := range parts {
			if text, ok := part["text"].(string); ok && text != "" {
				// Close an open tool_use block before starting text, mirroring
				// the functionCall branch (which closes open text blocks) and
				// the chat-completions sibling's closeOpenTool(). Otherwise a
				// tool→text sequence keeps the tool_use block open while the
				// text block starts, emitting overlapping Anthropic content
				// blocks that violate the SSE contract.
				if openToolIndex >= 0 {
					writeSSE(c.Writer, "content_block_stop", map[string]any{
						"type":  "content_block_stop",
						"index": openToolIndex,
					})
					openToolIndex = -1
					openToolName = ""
					seenToolJSON = ""
				}

				delta, newSeen := computeGeminiTextDelta(seenText, text)
				seenText = newSeen
				if delta == "" {
					continue
				}

				if openBlockType != "text" {
					if openBlockIndex >= 0 {
						writeSSE(c.Writer, "content_block_stop", map[string]any{
							"type":  "content_block_stop",
							"index": openBlockIndex,
						})
					}
					openBlockType = "text"
					openBlockIndex = nextBlockIndex
					nextBlockIndex++
					writeSSE(c.Writer, "content_block_start", map[string]any{
						"type":  "content_block_start",
						"index": openBlockIndex,
						"content_block": map[string]any{
							"type": "text",
							"text": "",
						},
					})
				}

				if firstTokenMs == nil {
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
				}
				writeSSE(c.Writer, "content_block_delta", map[string]any{
					"type":  "content_block_delta",
					"index": openBlockIndex,
					"delta": map[string]any{
						"type": "text_delta",
						"text": delta,
					},
				})
				flusher.Flush()
				continue
			}

			if fc, ok := part["functionCall"].(map[string]any); ok && fc != nil {
				name, _ := fc["name"].(string)
				args := fc["args"]
				if strings.TrimSpace(name) == "" {
					name = "tool"
				}

				// Close any open text block before tool_use.
				if openBlockIndex >= 0 {
					writeSSE(c.Writer, "content_block_stop", map[string]any{
						"type":  "content_block_stop",
						"index": openBlockIndex,
					})
					openBlockIndex = -1
					openBlockType = ""
				}

				// If we receive streamed tool args in pieces, keep a single tool block open and emit deltas.
				if openToolIndex >= 0 && openToolName != name {
					writeSSE(c.Writer, "content_block_stop", map[string]any{
						"type":  "content_block_stop",
						"index": openToolIndex,
					})
					openToolIndex = -1
					openToolName = ""
					seenToolJSON = ""
				}

				if openToolIndex < 0 {
					openToolID = "toolu_" + randomHex(8)
					openToolIndex = nextBlockIndex
					openToolName = name
					nextBlockIndex++
					sawToolUse = true

					writeSSE(c.Writer, "content_block_start", map[string]any{
						"type":  "content_block_start",
						"index": openToolIndex,
						"content_block": map[string]any{
							"type":  "tool_use",
							"id":    openToolID,
							"name":  name,
							"input": map[string]any{},
						},
					})
				}

				argsJSONText := "{}"
				switch v := args.(type) {
				case nil:
					// keep default "{}"
				case string:
					if strings.TrimSpace(v) != "" {
						argsJSONText = v
					}
				default:
					if b, err := json.Marshal(args); err == nil && len(b) > 0 {
						argsJSONText = string(b)
					}
				}

				delta, newSeen := computeGeminiTextDelta(seenToolJSON, argsJSONText)
				seenToolJSON = newSeen
				if delta != "" {
					writeSSE(c.Writer, "content_block_delta", map[string]any{
						"type":  "content_block_delta",
						"index": openToolIndex,
						"delta": map[string]any{
							"type":         "input_json_delta",
							"partial_json": delta,
						},
					})
				}
				flusher.Flush()
			}
		}

		if u := extractGeminiUsage(unwrappedBytes); u != nil {
			usage = *u
		}

		// Process the final unterminated line at EOF as well.
		if errors.Is(err, io.EOF) {
			break
		}
	}

	if openBlockIndex >= 0 {
		writeSSE(c.Writer, "content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": openBlockIndex,
		})
	}
	if openToolIndex >= 0 {
		writeSSE(c.Writer, "content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": openToolIndex,
		})
	}

	stopReason := mapGeminiFinishReasonToClaudeStopReason(finishReason)
	if sawToolUse {
		stopReason = "tool_use"
	}

	usageObj := map[string]any{
		"output_tokens": usage.OutputTokens,
	}
	if usage.InputTokens > 0 {
		usageObj["input_tokens"] = usage.InputTokens
	}
	writeSSE(c.Writer, "message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": usageObj,
	})
	writeSSE(c.Writer, "message_stop", map[string]any{
		"type": "message_stop",
	})
	flusher.Flush()

	return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMs}, nil
}

func writeSSE(w io.Writer, event string, data any) {
	if event != "" {
		_, _ = fmt.Fprintf(w, "event: %s\n", event)
	}
	b, _ := json.Marshal(data)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", string(b))
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *GeminiMessagesCompatService) writeClaudeError(c *gin.Context, status int, errType, message string) error {
	MarkResponseCommitted(c)
	c.JSON(status, gin.H{
		"type":  "error",
		"error": gin.H{"type": errType, "message": message},
	})
	return fmt.Errorf("%s", message)
}

func (s *GeminiMessagesCompatService) writeGoogleError(c *gin.Context, status int, message string) error {
	MarkResponseCommitted(c)
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    status,
			"message": message,
			"status":  googleapi.HTTPStatusToGoogleStatus(status),
		},
	})
	return fmt.Errorf("%s", message)
}

func unwrapIfNeeded(isOAuth bool, raw []byte) []byte {
	if !isOAuth {
		return raw
	}
	inner, err := unwrapGeminiResponse(raw)
	if err != nil {
		return raw
	}
	return inner
}

func collectGeminiSSE(body io.Reader, isOAuth bool) (map[string]any, *ClaudeUsage, error) {
	reader := bufio.NewReader(body)

	var last map[string]any
	var lastWithParts map[string]any
	var collectedTextParts []string // Collect all text parts for aggregation
	usage := &ClaudeUsage{}

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(trimmed, "data:") {
				payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
				switch payload {
				case "", "[DONE]":
					if payload == "[DONE]" {
						return mergeCollectedTextParts(pickGeminiCollectResult(last, lastWithParts), collectedTextParts), usage, nil
					}
				default:
					var parsed map[string]any
					var rawBytes []byte
					if isOAuth {
						innerBytes, err := unwrapGeminiResponse([]byte(payload))
						if err == nil {
							rawBytes = innerBytes
							_ = json.Unmarshal(innerBytes, &parsed)
						}
					} else {
						rawBytes = []byte(payload)
						_ = json.Unmarshal(rawBytes, &parsed)
					}
					if parsed != nil {
						last = parsed
						if u := extractGeminiUsage(rawBytes); u != nil {
							usage = u
						}
						if parts := extractGeminiParts(parsed); len(parts) > 0 {
							lastWithParts = parsed
							// Collect text from each part for aggregation
							for _, part := range parts {
								if text, ok := part["text"].(string); ok && text != "" {
									collectedTextParts = append(collectedTextParts, text)
								}
							}
						}
					}
				}
			}
		}

		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, err
		}
	}

	return mergeCollectedTextParts(pickGeminiCollectResult(last, lastWithParts), collectedTextParts), usage, nil
}

func pickGeminiCollectResult(last map[string]any, lastWithParts map[string]any) map[string]any {
	if lastWithParts != nil {
		return lastWithParts
	}
	if last != nil {
		return last
	}
	return map[string]any{}
}

// mergeCollectedTextParts merges all collected text chunks into the final response.
// This fixes the issue where non-streaming responses only returned the last chunk
// instead of the complete aggregated text.
func mergeCollectedTextParts(response map[string]any, textParts []string) map[string]any {
	if len(textParts) == 0 {
		return response
	}

	// Join all text parts
	mergedText := strings.Join(textParts, "")

	// Deep copy response
	result := make(map[string]any)
	for k, v := range response {
		result[k] = v
	}

	// Get or create candidates
	candidates, ok := result["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		candidates = []any{map[string]any{}}
	}

	// Get first candidate
	candidate, ok := candidates[0].(map[string]any)
	if !ok {
		candidate = make(map[string]any)
		candidates[0] = candidate
	}

	// Get or create content
	content, ok := candidate["content"].(map[string]any)
	if !ok {
		content = map[string]any{"role": "model"}
		candidate["content"] = content
	}

	// Get existing parts
	existingParts, ok := content["parts"].([]any)
	if !ok {
		existingParts = []any{}
	}

	// Find and update first text part, or create new one
	newParts := make([]any, 0, len(existingParts)+1)
	textUpdated := false

	for _, p := range existingParts {
		pm, ok := p.(map[string]any)
		if !ok {
			newParts = append(newParts, p)
			continue
		}
		if _, hasText := pm["text"]; hasText && !textUpdated {
			// Replace with merged text
			newPart := make(map[string]any)
			for k, v := range pm {
				newPart[k] = v
			}
			newPart["text"] = mergedText
			newParts = append(newParts, newPart)
			textUpdated = true
		} else {
			newParts = append(newParts, pm)
		}
	}

	if !textUpdated {
		newParts = append([]any{map[string]any{"text": mergedText}}, newParts...)
	}

	content["parts"] = newParts
	result["candidates"] = candidates

	return result
}

type geminiNativeStreamResult struct {
	usage        *ClaudeUsage
	firstTokenMs *int
}

func isGeminiInsufficientScope(headers http.Header, body []byte) bool {
	if strings.Contains(strings.ToLower(headers.Get("Www-Authenticate")), "insufficient_scope") {
		return true
	}
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "insufficient authentication scopes") || strings.Contains(lower, "access_token_scope_insufficient")
}

func estimateGeminiCountTokens(reqBody []byte) int {
	total := 0

	// systemInstruction.parts[].text
	gjson.GetBytes(reqBody, "systemInstruction.parts").ForEach(func(_, part gjson.Result) bool {
		if t := strings.TrimSpace(part.Get("text").String()); t != "" {
			total += estimateTokensForText(t)
		}
		return true
	})

	// contents[].parts[].text
	gjson.GetBytes(reqBody, "contents").ForEach(func(_, content gjson.Result) bool {
		content.Get("parts").ForEach(func(_, part gjson.Result) bool {
			if t := strings.TrimSpace(part.Get("text").String()); t != "" {
				total += estimateTokensForText(t)
			}
			return true
		})
		return true
	})

	if total < 0 {
		return 0
	}
	return total
}

func estimateTokensForText(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	runes := []rune(s)
	if len(runes) == 0 {
		return 0
	}
	ascii := 0
	for _, r := range runes {
		if r <= 0x7f {
			ascii++
		}
	}
	asciiRatio := float64(ascii) / float64(len(runes))
	if asciiRatio >= 0.8 {
		// Roughly 4 chars per token for English-like text.
		return (len(runes) + 3) / 4
	}
	// For CJK-heavy text, approximate 1 rune per token.
	return len(runes)
}

type UpstreamHTTPResult struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

func (s *GeminiMessagesCompatService) handleNativeNonStreamingResponse(c *gin.Context, resp *http.Response, isOAuth bool) (*ClaudeUsage, error) {
	if s.cfg != nil && s.cfg.Gateway.GeminiDebugResponseHeaders {
		logger.LegacyPrintf("service.gemini_messages_compat", "[GeminiAPI] ========== Response Headers ==========")
		for key, values := range resp.Header {
			if strings.HasPrefix(strings.ToLower(key), "x-ratelimit") {
				logger.LegacyPrintf("service.gemini_messages_compat", "[GeminiAPI] %s: %v", key, values)
			}
		}
		logger.LegacyPrintf("service.gemini_messages_compat", "[GeminiAPI] ========================================")
	}

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}

	if isOAuth {
		unwrappedBody, uwErr := unwrapGeminiResponse(respBody)
		if uwErr == nil {
			respBody = unwrappedBody
		}
	}

	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, respBody)

	if u := extractGeminiUsage(respBody); u != nil {
		return u, nil
	}
	return &ClaudeUsage{}, nil
}

func (s *GeminiMessagesCompatService) handleNativeStreamingResponse(c *gin.Context, resp *http.Response, startTime time.Time, isOAuth bool) (*geminiNativeStreamResult, error) {
	if s.cfg != nil && s.cfg.Gateway.GeminiDebugResponseHeaders {
		logger.LegacyPrintf("service.gemini_messages_compat", "[GeminiAPI] ========== Streaming Response Headers ==========")
		for key, values := range resp.Header {
			if strings.HasPrefix(strings.ToLower(key), "x-ratelimit") {
				logger.LegacyPrintf("service.gemini_messages_compat", "[GeminiAPI] %s: %v", key, values)
			}
		}
		logger.LegacyPrintf("service.gemini_messages_compat", "[GeminiAPI] ====================================================")
	}

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}

	c.Status(resp.StatusCode)
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/event-stream; charset=utf-8"
	}
	c.Header("Content-Type", contentType)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	reader := bufio.NewReader(resp.Body)
	usage := &ClaudeUsage{}
	var firstTokenMs *int

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(trimmed, "data:") {
				payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
				// Keepalive / done markers
				if payload == "" || payload == "[DONE]" {
					_, _ = io.WriteString(c.Writer, line)
					flusher.Flush()
				} else {
					var rawToWrite string
					rawToWrite = payload

					var rawBytes []byte
					if isOAuth {
						innerBytes, err := unwrapGeminiResponse([]byte(payload))
						if err == nil {
							rawToWrite = string(innerBytes)
							rawBytes = innerBytes
						}
					} else {
						rawBytes = []byte(payload)
					}

					if u := extractGeminiUsage(rawBytes); u != nil {
						usage = u
					}

					if firstTokenMs == nil {
						ms := int(time.Since(startTime).Milliseconds())
						firstTokenMs = &ms
					}

					if isOAuth {
						// SSE format requires double newline (\n\n) to separate events
						_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", rawToWrite)
					} else {
						// Pass-through for AI Studio responses.
						_, _ = io.WriteString(c.Writer, line)
					}
					flusher.Flush()
				}
			} else {
				_, _ = io.WriteString(c.Writer, line)
				flusher.Flush()
			}
		}

		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	return &geminiNativeStreamResult{usage: usage, firstTokenMs: firstTokenMs}, nil
}

// ForwardAIStudioGET forwards a GET request to AI Studio (generativelanguage.googleapis.com) for
// endpoints like /v1beta/models and /v1beta/models/{model}.
//
// This is used to support Gemini SDKs that call models listing endpoints before generation.
func (s *GeminiMessagesCompatService) ForwardAIStudioGET(ctx context.Context, account *Account, path string) (*UpstreamHTTPResult, error) {
	if account == nil {
		return nil, errors.New("account is nil")
	}
	account = NormalizeGeminiAPIKeyAccount(account)
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return nil, errors.New("invalid path")
	}

	if account.IsGeminiOpenAICompatibleUpstream() {
		result, err := s.forwardCompatibleRelayAIStudioGET(ctx, account, path)
		if !errors.Is(err, errGeminiCompatibleRelayOpenAIPathUnsupported) {
			return result, err
		}
	}

	baseURL := account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
	baseURL = geminiNativeBaseURLFromOpenAICompatible(baseURL)
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	fullURL := strings.TrimRight(normalizedBaseURL, "/") + path

	var proxyURL string
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}

	switch account.Type {
	case AccountTypeAPIKey:
		apiKey := strings.TrimSpace(account.GetCredential("api_key"))
		if apiKey == "" {
			return nil, errors.New("gemini api_key not configured")
		}
		req.Header.Set("x-goog-api-key", apiKey)
	case AccountTypeOAuth:
		if s.tokenProvider == nil {
			return nil, errors.New("gemini token provider not configured")
		}
		accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
	default:
		return nil, fmt.Errorf("unsupported account type: %s", account.Type)
	}

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	wwwAuthenticate := resp.Header.Get("Www-Authenticate")
	filteredHeaders := responseheaders.FilterHeaders(resp.Header, s.responseHeaderFilter)
	if wwwAuthenticate != "" {
		filteredHeaders.Set("Www-Authenticate", wwwAuthenticate)
	}
	return &UpstreamHTTPResult{
		StatusCode: resp.StatusCode,
		Headers:    filteredHeaders,
		Body:       body,
	}, nil
}

func (s *GeminiMessagesCompatService) forwardCompatibleRelayAIStudioGET(ctx context.Context, account *Account, path string) (*UpstreamHTTPResult, error) {
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		return nil, errors.New("gemini compatible relay api_key not configured")
	}

	baseURL := account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildOpenAIModelsURL(normalizedBaseURL), nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	var proxyURL string
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	filteredHeaders := responseheaders.FilterHeaders(resp.Header, s.responseHeaderFilter)

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return nil, errGeminiCompatibleRelayOpenAIPathUnsupported
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &UpstreamHTTPResult{StatusCode: resp.StatusCode, Headers: filteredHeaders, Body: body}, nil
	}

	models, err := extractUpstreamModelIDs(body)
	if err != nil {
		return nil, err
	}

	switch {
	case path == "/v1beta/models":
		converted, err := json.Marshal(geminiNativeModelsListResponse(models))
		if err != nil {
			return nil, err
		}
		filteredHeaders.Set("Content-Type", "application/json")
		return &UpstreamHTTPResult{StatusCode: http.StatusOK, Headers: filteredHeaders, Body: converted}, nil
	case strings.HasPrefix(path, "/v1beta/models/"):
		requested := strings.TrimPrefix(path, "/v1beta/models/")
		requested = strings.TrimPrefix(requested, "models/")
		requested = strings.TrimSpace(requested)
		if requested == "" {
			return nil, errors.New("invalid model path")
		}
		selected := requested
		for _, model := range models {
			if model == requested || "models/"+model == requested {
				selected = model
				break
			}
		}
		converted, err := json.Marshal(geminiNativeModelObject(selected))
		if err != nil {
			return nil, err
		}
		filteredHeaders.Set("Content-Type", "application/json")
		return &UpstreamHTTPResult{StatusCode: http.StatusOK, Headers: filteredHeaders, Body: converted}, nil
	default:
		return nil, errGeminiCompatibleRelayOpenAIPathUnsupported
	}
}

func geminiNativeModelsListResponse(models []string) map[string]any {
	out := make([]any, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(strings.TrimPrefix(model, "models/"))
		if model == "" {
			continue
		}
		out = append(out, geminiNativeModelObject(model))
	}
	return map[string]any{"models": out}
}

func geminiNativeModelObject(model string) map[string]any {
	model = strings.TrimSpace(strings.TrimPrefix(model, "models/"))
	return map[string]any{
		"name":                       "models/" + model,
		"baseModelId":                model,
		"version":                    model,
		"displayName":                model,
		"supportedGenerationMethods": []string{"generateContent", "streamGenerateContent", "countTokens"},
	}
}

// unwrapGeminiResponse 解包 Gemini OAuth 响应中的 response 字段
// 使用 gjson 零拷贝提取，避免完整 Unmarshal+Marshal
func unwrapGeminiResponse(raw []byte) ([]byte, error) {
	result := gjson.GetBytes(raw, "response")
	if result.Exists() && result.Type == gjson.JSON {
		return []byte(result.Raw), nil
	}
	return raw, nil
}

func convertGeminiToClaudeMessage(geminiResp map[string]any, originalModel string, rawData []byte) (map[string]any, *ClaudeUsage) {
	usage := extractGeminiUsage(rawData)
	if usage == nil {
		usage = &ClaudeUsage{}
	}

	contentBlocks := make([]any, 0)
	sawToolUse := false
	if candidates, ok := geminiResp["candidates"].([]any); ok && len(candidates) > 0 {
		if cand, ok := candidates[0].(map[string]any); ok {
			if content, ok := cand["content"].(map[string]any); ok {
				if parts, ok := content["parts"].([]any); ok {
					for _, part := range parts {
						pm, ok := part.(map[string]any)
						if !ok {
							continue
						}
						if text, ok := pm["text"].(string); ok && text != "" {
							contentBlocks = append(contentBlocks, map[string]any{
								"type": "text",
								"text": text,
							})
						}
						if fc, ok := pm["functionCall"].(map[string]any); ok {
							name, _ := fc["name"].(string)
							if strings.TrimSpace(name) == "" {
								name = "tool"
							}
							args := fc["args"]
							sawToolUse = true
							contentBlocks = append(contentBlocks, map[string]any{
								"type":  "tool_use",
								"id":    "toolu_" + randomHex(8),
								"name":  name,
								"input": args,
							})
						}
					}
				}
			}
		}
	}

	stopReason := mapGeminiFinishReasonToClaudeStopReason(extractGeminiFinishReason(geminiResp))
	if sawToolUse {
		stopReason = "tool_use"
	}

	resp := map[string]any{
		"id":            "msg_" + randomHex(12),
		"type":          "message",
		"role":          "assistant",
		"model":         originalModel,
		"content":       contentBlocks,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
		},
	}

	return resp, usage
}

func extractGeminiUsage(data []byte) *ClaudeUsage {
	usage := gjson.GetBytes(data, "usageMetadata")
	if !usage.Exists() {
		return nil
	}
	prompt := int(usage.Get("promptTokenCount").Int())
	cand := int(usage.Get("candidatesTokenCount").Int())
	cached := int(usage.Get("cachedContentTokenCount").Int())
	thoughts := int(usage.Get("thoughtsTokenCount").Int())

	// 从 candidatesTokensDetails 提取 IMAGE 模态 token 数
	imageTokens := 0
	candidateDetails := usage.Get("candidatesTokensDetails")
	if candidateDetails.Exists() {
		candidateDetails.ForEach(func(_, detail gjson.Result) bool {
			if detail.Get("modality").String() == "IMAGE" {
				imageTokens = int(detail.Get("tokenCount").Int())
				return false
			}
			return true
		})
	}

	// 注意：Gemini 的 promptTokenCount 包含 cachedContentTokenCount，
	// 但 Claude 的 input_tokens 不包含 cache_read_input_tokens，需要减去
	return &ClaudeUsage{
		InputTokens:          prompt - cached,
		OutputTokens:         cand + thoughts,
		CacheReadInputTokens: cached,
		ImageOutputTokens:    imageTokens,
	}
}

func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	case int64:
		return int(t), true
	case json.Number:
		i, err := t.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

func (s *GeminiMessagesCompatService) handleGeminiUpstreamError(ctx context.Context, account *Account, statusCode int, headers http.Header, body []byte) {
	// 遵守自定义错误码策略：未命中则跳过所有限流处理
	if !account.ShouldHandleErrorCode(statusCode) {
		return
	}
	if s.rateLimitService != nil && (statusCode == 401 || statusCode == 403 || statusCode == 529) {
		s.rateLimitService.HandleUpstreamError(ctx, account, statusCode, headers, body)
		return
	}
	if statusCode != 429 {
		return
	}

	oauthType := account.GeminiOAuthType()
	tierID := account.GeminiTierID()
	projectID := strings.TrimSpace(account.GetCredential("project_id"))
	isCodeAssist := account.IsGeminiCodeAssist()

	resetAt := ParseGeminiRateLimitResetTime(body)
	if resetAt == nil {
		// 根据账号类型使用不同的默认重置时间
		var ra time.Time
		if isCodeAssist || oauthType == "google_one" {
			// Gemini CLI / Google One: fallback cooldown by tier
			cooldown := geminiCooldownForTier(tierID)
			if s.rateLimitService != nil {
				cooldown = s.rateLimitService.GeminiCooldown(ctx, account)
			}
			ra = time.Now().Add(cooldown)
			if isCodeAssist {
				logger.LegacyPrintf("service.gemini_messages_compat", "[Gemini 429] Account %d (Code Assist, tier=%s, project=%s) rate limited, cooldown=%v", account.ID, tierID, projectID, time.Until(ra).Truncate(time.Second))
			} else {
				logger.LegacyPrintf("service.gemini_messages_compat", "[Gemini 429] Account %d (Google One OAuth, tier=%s, project=%s) rate limited, cooldown=%v", account.ID, tierID, projectID, time.Until(ra).Truncate(time.Second))
			}
		} else {
			// API Key / AI Studio OAuth: PST 午夜
			if ts := nextGeminiDailyResetUnix(); ts != nil {
				ra = time.Unix(*ts, 0)
				logger.LegacyPrintf("service.gemini_messages_compat", "[Gemini 429] Account %d (API Key/AI Studio, type=%s) rate limited, reset at PST midnight (%v)", account.ID, account.Type, ra)
			} else {
				// 兜底：5 分钟
				ra = time.Now().Add(5 * time.Minute)
				logger.LegacyPrintf("service.gemini_messages_compat", "[Gemini 429] Account %d rate limited, fallback to 5min", account.ID)
			}
		}
		_ = s.accountRepo.SetRateLimited(ctx, account.ID, ra)
		return
	}

	// 使用解析到的重置时间
	resetTime := time.Unix(*resetAt, 0)
	_ = s.accountRepo.SetRateLimited(ctx, account.ID, resetTime)
	logger.LegacyPrintf("service.gemini_messages_compat", "[Gemini 429] Account %d rate limited until %v (oauth_type=%s, tier=%s)",
		account.ID, resetTime, oauthType, tierID)
}

// ParseGeminiRateLimitResetTime 解析 Gemini 格式的 429 响应，返回重置时间的 Unix 时间戳
func ParseGeminiRateLimitResetTime(body []byte) *int64 {
	// 第一阶段：gjson 结构化提取
	errMsg := gjson.GetBytes(body, "error.message").String()
	if looksLikeGeminiDailyQuota(errMsg) {
		if ts := nextGeminiDailyResetUnix(); ts != nil {
			return ts
		}
	}

	// 遍历 error.details 查找 quotaResetDelay
	var found *int64
	gjson.GetBytes(body, "error.details").ForEach(func(_, detail gjson.Result) bool {
		v := detail.Get("metadata.quotaResetDelay").String()
		if v == "" {
			return true
		}
		if dur, err := time.ParseDuration(v); err == nil {
			// Use ceil to avoid undercounting fractional seconds (e.g. 10.1s should not become 10s),
			// which can affect scheduling decisions around thresholds (like 10s).
			ts := time.Now().Unix() + int64(math.Ceil(dur.Seconds()))
			found = &ts
			return false
		}
		return true
	})
	if found != nil {
		return found
	}

	// 第二阶段：regex 回退匹配 "Please retry in Xs"
	matches := retryInRegex.FindStringSubmatch(string(body))
	if len(matches) == 2 {
		if dur, err := time.ParseDuration(matches[1] + "s"); err == nil {
			ts := time.Now().Unix() + int64(math.Ceil(dur.Seconds()))
			return &ts
		}
	}

	return nil
}

func looksLikeGeminiDailyQuota(message string) bool {
	m := strings.ToLower(message)
	if strings.Contains(m, "per day") || strings.Contains(m, "requests per day") || strings.Contains(m, "quota") && strings.Contains(m, "per day") {
		return true
	}
	return false
}

func nextGeminiDailyResetUnix() *int64 {
	reset := geminiDailyResetTime(time.Now())
	ts := reset.Unix()
	return &ts
}

func ensureGeminiFunctionCallThoughtSignatures(body []byte) []byte {
	// Fast path: only run when functionCall is present.
	if !bytes.Contains(body, []byte(`"functionCall"`)) {
		return body
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}

	contentsAny, ok := payload["contents"].([]any)
	if !ok || len(contentsAny) == 0 {
		return body
	}

	modified := false
	for _, c := range contentsAny {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		partsAny, ok := cm["parts"].([]any)
		if !ok || len(partsAny) == 0 {
			continue
		}
		for _, p := range partsAny {
			pm, ok := p.(map[string]any)
			if !ok || pm == nil {
				continue
			}
			if fc, ok := pm["functionCall"].(map[string]any); !ok || fc == nil {
				continue
			}
			ts, _ := pm["thoughtSignature"].(string)
			if strings.TrimSpace(ts) == "" {
				pm["thoughtSignature"] = geminiDummyThoughtSignature
				modified = true
			}
		}
	}

	if !modified {
		return body
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return b
}

func extractGeminiFinishReason(geminiResp map[string]any) string {
	if candidates, ok := geminiResp["candidates"].([]any); ok && len(candidates) > 0 {
		if cand, ok := candidates[0].(map[string]any); ok {
			if fr, ok := cand["finishReason"].(string); ok {
				return fr
			}
		}
	}
	return ""
}

func extractGeminiParts(geminiResp map[string]any) []map[string]any {
	if candidates, ok := geminiResp["candidates"].([]any); ok && len(candidates) > 0 {
		if cand, ok := candidates[0].(map[string]any); ok {
			if content, ok := cand["content"].(map[string]any); ok {
				if partsAny, ok := content["parts"].([]any); ok && len(partsAny) > 0 {
					out := make([]map[string]any, 0, len(partsAny))
					for _, p := range partsAny {
						pm, ok := p.(map[string]any)
						if !ok {
							continue
						}
						out = append(out, pm)
					}
					return out
				}
			}
		}
	}
	return nil
}

func computeGeminiTextDelta(seen, incoming string) (delta, newSeen string) {
	incoming = strings.TrimSuffix(incoming, "\u0000")
	if incoming == "" {
		return "", seen
	}

	// Cumulative mode: incoming contains full text so far.
	if strings.HasPrefix(incoming, seen) {
		return strings.TrimPrefix(incoming, seen), incoming
	}
	// Duplicate/rewind: ignore.
	if strings.HasPrefix(seen, incoming) {
		return "", seen
	}
	// Delta mode: treat incoming as incremental chunk.
	return incoming, seen + incoming
}

func mapGeminiFinishReasonToClaudeStopReason(finishReason string) string {
	switch strings.ToUpper(strings.TrimSpace(finishReason)) {
	case "MAX_TOKENS":
		return "max_tokens"
	case "STOP":
		return "end_turn"
	default:
		return "end_turn"
	}
}

func convertClaudeMessagesToGeminiGenerateContent(body []byte) ([]byte, error) {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	toolUseIDToName := make(map[string]string)

	systemText := extractClaudeSystemText(req["system"])
	contents, err := convertClaudeMessagesToGeminiContents(req["messages"], toolUseIDToName)
	if err != nil {
		return nil, err
	}

	out := make(map[string]any)
	if systemText != "" {
		out["systemInstruction"] = map[string]any{
			"parts": []any{map[string]any{"text": systemText}},
		}
	}
	out["contents"] = contents

	if tools := convertClaudeToolsToGeminiTools(req["tools"]); tools != nil {
		out["tools"] = tools
	}

	generationConfig := convertClaudeGenerationConfig(req)
	if generationConfig != nil {
		out["generationConfig"] = generationConfig
	}

	stripGeminiFunctionIDs(out)
	return json.Marshal(out)
}

func stripGeminiFunctionIDs(req map[string]any) {
	// Defensive cleanup: some upstreams reject unexpected `id` fields in functionCall/functionResponse.
	contents, ok := req["contents"].([]any)
	if !ok {
		return
	}
	for _, c := range contents {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		contentParts, ok := cm["parts"].([]any)
		if !ok {
			continue
		}
		for _, p := range contentParts {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if fc, ok := pm["functionCall"].(map[string]any); ok && fc != nil {
				delete(fc, "id")
			}
			if fr, ok := pm["functionResponse"].(map[string]any); ok && fr != nil {
				delete(fr, "id")
			}
		}
	}
}

func extractClaudeSystemText(system any) string {
	switch v := system.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, p := range v {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := pm["type"].(string); t != "text" {
				continue
			}
			if text, ok := pm["text"].(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return ""
	}
}

func convertClaudeMessagesToGeminiContents(messages any, toolUseIDToName map[string]string) ([]any, error) {
	arr, ok := messages.([]any)
	if !ok {
		return nil, errors.New("messages must be an array")
	}

	out := make([]any, 0, len(arr))
	for _, m := range arr {
		mm, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := mm["role"].(string)
		role = strings.ToLower(strings.TrimSpace(role))
		gRole := "user"
		if role == "assistant" {
			gRole = "model"
		}

		parts := make([]any, 0)
		switch content := mm["content"].(type) {
		case string:
			// 字符串形式的 content，保留所有内容（包括空白）
			parts = append(parts, map[string]any{"text": content})
		case []any:
			// 如果只有一个 block，不过滤空白（让上游 API 报错）
			singleBlock := len(content) == 1

			for _, block := range content {
				bm, ok := block.(map[string]any)
				if !ok {
					continue
				}
				bt, _ := bm["type"].(string)
				switch bt {
				case "text":
					if text, ok := bm["text"].(string); ok {
						// 单个 block 时保留所有内容（包括空白）
						// 多个 blocks 时过滤掉空白
						if singleBlock || strings.TrimSpace(text) != "" {
							parts = append(parts, map[string]any{"text": text})
						}
					}
				case "tool_use":
					id, _ := bm["id"].(string)
					name, _ := bm["name"].(string)
					if strings.TrimSpace(id) != "" && strings.TrimSpace(name) != "" {
						toolUseIDToName[id] = name
					}
					signature, _ := bm["signature"].(string)
					signature = strings.TrimSpace(signature)
					if signature == "" {
						signature = geminiDummyThoughtSignature
					}
					parts = append(parts, map[string]any{
						"thoughtSignature": signature,
						"functionCall": map[string]any{
							"name": name,
							"args": bm["input"],
						},
					})
				case "tool_result":
					toolUseID, _ := bm["tool_use_id"].(string)
					name := toolUseIDToName[toolUseID]
					if name == "" {
						name = "tool"
					}
					parts = append(parts, map[string]any{
						"functionResponse": map[string]any{
							"name": name,
							"response": map[string]any{
								"content": extractClaudeContentText(bm["content"]),
							},
						},
					})
				case "image":
					if src, ok := bm["source"].(map[string]any); ok {
						if srcType, _ := src["type"].(string); srcType == "base64" {
							mediaType, _ := src["media_type"].(string)
							data, _ := src["data"].(string)
							if mediaType != "" && data != "" {
								parts = append(parts, map[string]any{
									"inlineData": map[string]any{
										"mimeType": mediaType,
										"data":     data,
									},
								})
							}
						}
					}
				default:
					// best-effort: preserve unknown blocks as text
					if b, err := json.Marshal(bm); err == nil {
						parts = append(parts, map[string]any{"text": string(b)})
					}
				}
			}
		default:
			// ignore
		}

		out = append(out, map[string]any{
			"role":  gRole,
			"parts": parts,
		})
	}
	return out, nil
}

func extractClaudeContentText(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var sb strings.Builder
		for _, part := range t {
			pm, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if pm["type"] == "text" {
				if text, ok := pm["text"].(string); ok {
					_, _ = sb.WriteString(text)
				}
			}
		}
		return sb.String()
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func convertClaudeToolsToGeminiTools(tools any) []any {
	arr, ok := tools.([]any)
	if !ok || len(arr) == 0 {
		return nil
	}

	hasWebSearch := false
	funcDecls := make([]any, 0, len(arr))
	for _, t := range arr {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		if isClaudeWebSearchToolMap(tm) {
			hasWebSearch = true
			continue
		}

		var name, desc string
		var params any

		// 检查是否为 custom 类型工具 (MCP)
		toolType, _ := tm["type"].(string)
		if toolType == "custom" {
			// Custom 格式: 从 custom 字段获取 description 和 input_schema
			custom, ok := tm["custom"].(map[string]any)
			if !ok {
				continue
			}
			name, _ = tm["name"].(string)
			desc, _ = custom["description"].(string)
			params = custom["input_schema"]
		} else {
			// 标准格式: 从顶层字段获取
			name, _ = tm["name"].(string)
			desc, _ = tm["description"].(string)
			params = tm["input_schema"]
		}

		if name == "" {
			continue
		}

		// 为 nil params 提供默认值
		if params == nil {
			params = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}
		// 清理 JSON Schema
		cleanedParams := cleanToolSchema(params)

		funcDecls = append(funcDecls, map[string]any{
			"name":        name,
			"description": desc,
			"parameters":  cleanedParams,
		})
	}

	out := make([]any, 0, 2)
	if len(funcDecls) > 0 {
		out = append(out, map[string]any{
			"functionDeclarations": funcDecls,
		})
	}
	if hasWebSearch {
		out = append(out, map[string]any{
			"googleSearch": map[string]any{},
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeGeminiRequestForAIStudio(body []byte) []byte {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}

	tools, ok := payload["tools"].([]any)
	if !ok || len(tools) == 0 {
		return body
	}

	modified := false
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		googleSearch, ok := tool["googleSearch"]
		if !ok {
			continue
		}
		if _, exists := tool["google_search"]; exists {
			continue
		}
		tool["google_search"] = googleSearch
		delete(tool, "googleSearch")
		modified = true
	}

	if !modified {
		return body
	}

	normalized, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return normalized
}

func isClaudeWebSearchToolMap(tool map[string]any) bool {
	toolType, _ := tool["type"].(string)
	if strings.HasPrefix(toolType, "web_search") || toolType == "google_search" {
		return true
	}

	name, _ := tool["name"].(string)
	switch strings.TrimSpace(name) {
	case "web_search", "google_search", "web_search_20250305":
		return true
	default:
		return false
	}
}

// cleanToolSchema 清理工具的 JSON Schema，移除 Gemini 不支持的字段
func cleanToolSchema(schema any) any {
	if schema == nil {
		return nil
	}

	switch v := schema.(type) {
	case map[string]any:
		cleaned := make(map[string]any)
		for key, value := range v {
			// 跳过不支持的字段
			if key == "$schema" || key == "$id" || key == "$ref" ||
				key == "additionalProperties" || key == "patternProperties" || key == "minLength" ||
				key == "maxLength" || key == "minItems" || key == "maxItems" {
				continue
			}
			// 递归清理嵌套对象
			cleaned[key] = cleanToolSchema(value)
		}
		// 规范化 type 字段为大写
		if typeVal, ok := cleaned["type"].(string); ok {
			cleaned["type"] = strings.ToUpper(typeVal)
		}
		return cleaned
	case []any:
		cleaned := make([]any, len(v))
		for i, item := range v {
			cleaned[i] = cleanToolSchema(item)
		}
		return cleaned
	default:
		return v
	}
}

func convertClaudeGenerationConfig(req map[string]any) map[string]any {
	out := make(map[string]any)
	if mt, ok := asInt(req["max_tokens"]); ok && mt > 0 {
		out["maxOutputTokens"] = mt
	}
	if temp, ok := req["temperature"].(float64); ok {
		out["temperature"] = temp
	}
	if topP, ok := req["top_p"].(float64); ok {
		out["topP"] = topP
	}
	if stopSeq, ok := req["stop_sequences"].([]any); ok && len(stopSeq) > 0 {
		out["stopSequences"] = stopSeq
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *GeminiMessagesCompatService) extractImageInputSize(body []byte) string {
	var req struct {
		GenerationConfig *struct {
			ImageConfig *struct {
				ImageSize string `json:"imageSize"`
			} `json:"imageConfig"`
		} `json:"generationConfig"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}

	if req.GenerationConfig != nil && req.GenerationConfig.ImageConfig != nil {
		return strings.TrimSpace(req.GenerationConfig.ImageConfig.ImageSize)
	}

	return ""
}

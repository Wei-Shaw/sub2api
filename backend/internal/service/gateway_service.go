package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	mathrand "math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/cespare/xxhash/v2"
	"github.com/google/uuid"
	gocache "github.com/patrickmn/go-cache"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/sync/singleflight"

	"github.com/gin-gonic/gin"
)

const (
	claudeAPIURL            = "https://api.anthropic.com/v1/messages?beta=true"
	claudeAPICountTokensURL = "https://api.anthropic.com/v1/messages/count_tokens?beta=true"
	stickySessionTTL        = time.Hour // 粘性会话TTL
	defaultMaxLineSize      = 500 * 1024 * 1024
	// Canonical Claude Code banner. Keep it EXACT (no trailing whitespace/newlines)
	// to match real Claude CLI traffic as closely as possible. When we need a visual
	// separator between system blocks, we add "\n\n" at concatenation time.
	claudeCodeSystemPrompt = "You are Claude Code, Anthropic's official CLI for Claude."
	maxCacheControlBlocks  = 4 // Anthropic API 允许的最大 cache_control 块数量

	defaultUserGroupRateCacheTTL = 30 * time.Second
	defaultModelsListCacheTTL    = 15 * time.Second
	postUsageBillingTimeout      = 15 * time.Second
	debugGatewayBodyEnv          = "SUB2API_DEBUG_GATEWAY_BODY"
)

const (
	claudeMimicDebugInfoKey = "claude_mimic_debug_info"
)

// ForceCacheBillingContextKey 强制缓存计费上下文键
// 用于粘性会话切换时，将 input_tokens 转为 cache_read_input_tokens 计费
type forceCacheBillingKeyType struct{}

// accountWithLoad 账号与负载信息的组合，用于负载感知调度
type accountWithLoad struct {
	account  *Account
	loadInfo *AccountLoadInfo
}

var ForceCacheBillingContextKey = forceCacheBillingKeyType{}

var (
	windowCostPrefetchCacheHitTotal  atomic.Int64
	windowCostPrefetchCacheMissTotal atomic.Int64
	windowCostPrefetchBatchSQLTotal  atomic.Int64
	windowCostPrefetchFallbackTotal  atomic.Int64
	windowCostPrefetchErrorTotal     atomic.Int64

	userGroupRateCacheHitTotal      atomic.Int64
	userGroupRateCacheMissTotal     atomic.Int64
	userGroupRateCacheLoadTotal     atomic.Int64
	userGroupRateCacheSFSharedTotal atomic.Int64
	userGroupRateCacheFallbackTotal atomic.Int64

	modelsListCacheHitTotal   atomic.Int64
	modelsListCacheMissTotal  atomic.Int64
	modelsListCacheStoreTotal atomic.Int64
)

func GatewayWindowCostPrefetchStats() (cacheHit, cacheMiss, batchSQL, fallback, errCount int64) {
	return windowCostPrefetchCacheHitTotal.Load(),
		windowCostPrefetchCacheMissTotal.Load(),
		windowCostPrefetchBatchSQLTotal.Load(),
		windowCostPrefetchFallbackTotal.Load(),
		windowCostPrefetchErrorTotal.Load()
}

func GatewayUserGroupRateCacheStats() (cacheHit, cacheMiss, load, singleflightShared, fallback int64) {
	return userGroupRateCacheHitTotal.Load(),
		userGroupRateCacheMissTotal.Load(),
		userGroupRateCacheLoadTotal.Load(),
		userGroupRateCacheSFSharedTotal.Load(),
		userGroupRateCacheFallbackTotal.Load()
}

func GatewayModelsListCacheStats() (cacheHit, cacheMiss, store int64) {
	return modelsListCacheHitTotal.Load(), modelsListCacheMissTotal.Load(), modelsListCacheStoreTotal.Load()
}

func openAIStreamEventIsTerminal(data string) bool {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return false
	}
	if trimmed == "[DONE]" {
		return true
	}
	switch gjson.Get(trimmed, "type").String() {
	case "response.completed", "response.done", "response.failed":
		return true
	default:
		return false
	}
}

func anthropicStreamEventIsTerminal(eventName, data string) bool {
	if strings.EqualFold(strings.TrimSpace(eventName), "message_stop") {
		return true
	}
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return false
	}
	if trimmed == "[DONE]" {
		return true
	}
	return gjson.Get(trimmed, "type").String() == "message_stop"
}

func cloneStringSlice(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

// IsForceCacheBilling 检查是否启用强制缓存计费
func IsForceCacheBilling(ctx context.Context) bool {
	v, _ := ctx.Value(ForceCacheBillingContextKey).(bool)
	return v
}

// WithForceCacheBilling 返回带有强制缓存计费标记的上下文
func WithForceCacheBilling(ctx context.Context) context.Context {
	return context.WithValue(ctx, ForceCacheBillingContextKey, true)
}

func (s *GatewayService) debugModelRoutingEnabled() bool {
	if s == nil {
		return false
	}
	return s.debugModelRouting.Load()
}

func (s *GatewayService) debugClaudeMimicEnabled() bool {
	if s == nil {
		return false
	}
	return s.debugClaudeMimic.Load()
}

func parseDebugEnvBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func shortSessionHash(sessionHash string) string {
	if sessionHash == "" {
		return ""
	}
	if len(sessionHash) <= 8 {
		return sessionHash
	}
	return sessionHash[:8]
}

func redactAuthHeaderValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	// Keep scheme for debugging, redact secret.
	if strings.HasPrefix(strings.ToLower(v), "bearer ") {
		return "Bearer [redacted]"
	}
	return "[redacted]"
}

func safeHeaderValueForLog(key string, v string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "authorization", "x-api-key":
		return redactAuthHeaderValue(v)
	default:
		return strings.TrimSpace(v)
	}
}

func extractSystemPreviewFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	sys := gjson.GetBytes(body, "system")
	if !sys.Exists() {
		return ""
	}

	switch {
	case sys.IsArray():
		for _, item := range sys.Array() {
			if !item.IsObject() {
				continue
			}
			if strings.EqualFold(item.Get("type").String(), "text") {
				if t := item.Get("text").String(); strings.TrimSpace(t) != "" {
					return t
				}
			}
		}
		return ""
	case sys.Type == gjson.String:
		return sys.String()
	default:
		return ""
	}
}

func buildClaudeMimicDebugLine(req *http.Request, body []byte, account *Account, tokenType string, mimicClaudeCode bool) string {
	if req == nil {
		return ""
	}

	// Only log a minimal fingerprint to avoid leaking user content.
	interesting := []string{
		"user-agent",
		"x-app",
		"anthropic-dangerous-direct-browser-access",
		"anthropic-version",
		"anthropic-beta",
		"x-stainless-lang",
		"x-stainless-package-version",
		"x-stainless-os",
		"x-stainless-arch",
		"x-stainless-runtime",
		"x-stainless-runtime-version",
		"x-stainless-retry-count",
		"x-stainless-timeout",
		"authorization",
		"x-api-key",
		"content-type",
		"accept",
		"x-stainless-helper-method",
	}

	h := make([]string, 0, len(interesting))
	for _, k := range interesting {
		if v := req.Header.Get(k); v != "" {
			h = append(h, fmt.Sprintf("%s=%q", k, safeHeaderValueForLog(k, v)))
		}
	}

	metaUserID := strings.TrimSpace(gjson.GetBytes(body, "metadata.user_id").String())
	sysPreview := strings.TrimSpace(extractSystemPreviewFromBody(body))

	// Truncate preview to keep logs sane.
	if len(sysPreview) > 300 {
		sysPreview = sysPreview[:300] + "..."
	}
	sysPreview = strings.ReplaceAll(sysPreview, "\n", "\\n")
	sysPreview = strings.ReplaceAll(sysPreview, "\r", "\\r")

	aid := int64(0)
	aname := ""
	if account != nil {
		aid = account.ID
		aname = account.Name
	}

	return fmt.Sprintf(
		"url=%s account=%d(%s) tokenType=%s mimic=%t meta.user_id=%q system.preview=%q headers={%s}",
		req.URL.String(),
		aid,
		aname,
		tokenType,
		mimicClaudeCode,
		metaUserID,
		sysPreview,
		strings.Join(h, " "),
	)
}

func logClaudeMimicDebug(req *http.Request, body []byte, account *Account, tokenType string, mimicClaudeCode bool) {
	line := buildClaudeMimicDebugLine(req, body, account, tokenType, mimicClaudeCode)
	if line == "" {
		return
	}
	logger.LegacyPrintf("service.gateway", "[ClaudeMimicDebug] %s", line)
}

func isClaudeCodeCredentialScopeError(msg string) bool {
	m := strings.ToLower(strings.TrimSpace(msg))
	if m == "" {
		return false
	}
	return strings.Contains(m, "only authorized for use with claude code") &&
		strings.Contains(m, "cannot be used for other api requests")
}

// sseDataRe matches SSE data lines with optional whitespace after colon.
// Some upstream APIs return non-standard "data:" without space (should be "data: ").
var (
	sseDataRe            = regexp.MustCompile(`^data:\s*`)
	claudeCliUserAgentRe = regexp.MustCompile(`^claude-cli/\d+\.\d+\.\d+`)

	// claudeCodePromptPrefixes 用于检测 Claude Code 系统提示词的前缀列表
	// 支持多种变体：标准版、Agent SDK 版、Explore Agent 版、Compact 版等
	// 注意：前缀之间不应存在包含关系，否则会导致冗余匹配
	claudeCodePromptPrefixes = []string{
		"You are Claude Code, Anthropic's official CLI for Claude",             // 标准版 & Agent SDK 版（含 running within...）
		"You are a Claude agent, built on Anthropic's Claude Agent SDK",        // Agent SDK 变体
		"You are a file search specialist for Claude Code",                     // Explore Agent 版
		"You are a helpful AI assistant tasked with summarizing conversations", // Compact 版
	}
)

// ErrNoAvailableAccounts 表示没有可用的账号
var ErrNoAvailableAccounts = errors.New("no available accounts")

// ErrClaudeCodeOnly 表示分组仅允许 Claude Code 客户端访问
var ErrClaudeCodeOnly = errors.New("this group only allows Claude Code clients")

// allowedHeaders 白名单headers（参考CRS项目）
var allowedHeaders = map[string]bool{
	"accept":                                    true,
	"x-stainless-retry-count":                   true,
	"x-stainless-timeout":                       true,
	"x-stainless-lang":                          true,
	"x-stainless-package-version":               true,
	"x-stainless-os":                            true,
	"x-stainless-arch":                          true,
	"x-stainless-runtime":                       true,
	"x-stainless-runtime-version":               true,
	"x-stainless-helper-method":                 true,
	"anthropic-dangerous-direct-browser-access": true,
	"anthropic-version":                         true,
	"x-app":                                     true,
	"anthropic-beta":                            true,
	"accept-language":                           true,
	"sec-fetch-mode":                            true,
	"user-agent":                                true,
	"content-type":                              true,
	"accept-encoding":                           true,
	"x-claude-code-session-id":                  true,
	"x-client-request-id":                       true,
}

// GatewayCache 定义网关服务的缓存操作接口。
// 提供粘性会话（Sticky Session）的存储、查询、刷新和删除功能。
//
// GatewayCache defines cache operations for gateway service.
// Provides sticky session storage, retrieval, refresh and deletion capabilities.
type GatewayCache interface {
	// GetSessionAccountID 获取粘性会话绑定的账号 ID
	// Get the account ID bound to a sticky session
	GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error)
	// SetSessionAccountID 设置粘性会话与账号的绑定关系
	// Set the binding between sticky session and account
	SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error
	// RefreshSessionTTL 刷新粘性会话的过期时间
	// Refresh the expiration time of a sticky session
	RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error
	// DeleteSessionAccountID 删除粘性会话绑定，用于账号不可用时主动清理
	// Delete sticky session binding, used to proactively clean up when account becomes unavailable
	DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error
}

// derefGroupID safely dereferences *int64 to int64, returning 0 if nil
func derefGroupID(groupID *int64) int64 {
	if groupID == nil {
		return 0
	}
	return *groupID
}

func resolveUserGroupRateCacheTTL(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.Gateway.UserGroupRateCacheTTLSeconds <= 0 {
		return defaultUserGroupRateCacheTTL
	}
	return time.Duration(cfg.Gateway.UserGroupRateCacheTTLSeconds) * time.Second
}

func resolveModelsListCacheTTL(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.Gateway.ModelsListCacheTTLSeconds <= 0 {
		return defaultModelsListCacheTTL
	}
	return time.Duration(cfg.Gateway.ModelsListCacheTTLSeconds) * time.Second
}

func modelsListCacheKey(groupID *int64, platform string) string {
	return fmt.Sprintf("%d|%s", derefGroupID(groupID), strings.TrimSpace(platform))
}

func prefetchedStickyGroupIDFromContext(ctx context.Context) (int64, bool) {
	return PrefetchedStickyGroupIDFromContext(ctx)
}

func prefetchedStickyAccountIDFromContext(ctx context.Context, groupID *int64) int64 {
	prefetchedGroupID, ok := prefetchedStickyGroupIDFromContext(ctx)
	if !ok || prefetchedGroupID != derefGroupID(groupID) {
		return 0
	}
	if accountID, ok := PrefetchedStickyAccountIDFromContext(ctx); ok && accountID > 0 {
		return accountID
	}
	return 0
}

// shouldClearStickySession 检查账号是否处于不可调度状态，需要清理粘性会话绑定。
// 当账号状态为错误、禁用、不可调度、处于临时不可调度期间，
// 或请求的模型处于限流状态时，返回 true。
// 这确保后续请求不会继续使用不可用的账号。
//
// shouldClearStickySession checks if an account is in an unschedulable state
// and the sticky session binding should be cleared.
// Returns true when account status is error/disabled, schedulable is false,
// within temporary unschedulable period, or the requested model is rate-limited.
// This ensures subsequent requests won't continue using unavailable accounts.
func shouldClearStickySession(account *Account, requestedModel string) bool {
	if account == nil {
		return false
	}
	if account.Status == StatusError || account.Status == StatusDisabled || !account.Schedulable {
		return true
	}
	if account.TempUnschedulableUntil != nil && time.Now().Before(*account.TempUnschedulableUntil) {
		return true
	}
	// 检查模型限流和 scope 限流，有限流即清除粘性会话
	if remaining := account.GetRateLimitRemainingTimeWithContext(context.Background(), requestedModel); remaining > 0 {
		return true
	}
	return false
}

type AccountWaitPlan struct {
	AccountID      int64
	MaxConcurrency int
	Timeout        time.Duration
	MaxWaiting     int
}

type AccountSelectionResult struct {
	Account     *Account
	Acquired    bool
	ReleaseFunc func()
	WaitPlan    *AccountWaitPlan // nil means no wait allowed
}

// ClaudeUsage 表示Claude API返回的usage信息
type ClaudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreation5mTokens    int // 5分钟缓存创建token（来自嵌套 cache_creation 对象）
	CacheCreation1hTokens    int // 1小时缓存创建token（来自嵌套 cache_creation 对象）
	ImageOutputTokens        int `json:"image_output_tokens,omitempty"`
}

// ForwardResult 转发结果
type ForwardResult struct {
	RequestID string
	Usage     ClaudeUsage
	Model     string
	// UpstreamModel is the actual upstream model after mapping.
	// Prefer empty when it is identical to Model; persistence normalizes equal values away as no-op mappings.
	UpstreamModel    string
	Stream           bool
	Duration         time.Duration
	FirstTokenMs     *int // 首字时间（流式请求）
	ClientDisconnect bool // 客户端是否在流式传输过程中断开
	ReasoningEffort  *string

	// 图片生成计费字段（图片生成模型使用）
	ImageCount int    // 生成的图片数量
	ImageSize  string // 图片尺寸 "1K", "2K", "4K"
}

// UpstreamFailoverError indicates an upstream error that should trigger account failover.
type UpstreamFailoverError struct {
	StatusCode             int
	ResponseBody           []byte      // 上游响应体，用于错误透传规则匹配
	ResponseHeaders        http.Header // 上游响应头，用于透传 cf-ray/cf-mitigated/content-type 等诊断信息
	ForceCacheBilling      bool        // Antigravity 粘性会话切换时设为 true
	RetryableOnSameAccount bool        // 临时性错误（如 Google 间歇性 400、空响应），应在同一账号上重试 N 次再切换
}

func (e *UpstreamFailoverError) Error() string {
	return fmt.Sprintf("upstream error: %d (failover)", e.StatusCode)
}

// TempUnscheduleRetryableError 对 RetryableOnSameAccount 类型的 failover 错误触发临时封禁。
// 由 handler 层在同账号重试全部用尽、切换账号时调用。
func (s *GatewayService) TempUnscheduleRetryableError(ctx context.Context, accountID int64, failoverErr *UpstreamFailoverError) {
	if failoverErr == nil || !failoverErr.RetryableOnSameAccount {
		return
	}
	// 根据状态码选择封禁策略
	switch failoverErr.StatusCode {
	case http.StatusBadRequest:
		tempUnscheduleGoogleConfigError(ctx, s.accountRepo, accountID, "[handler]")
	case http.StatusBadGateway:
		tempUnscheduleEmptyResponse(ctx, s.accountRepo, accountID, "[handler]")
	}
}

// GatewayService handles API gateway operations
type GatewayService struct {
	accountRepo           AccountRepository
	groupRepo             GroupRepository
	usageLogRepo          UsageLogRepository
	usageBillingRepo      UsageBillingRepository
	userRepo              UserRepository
	userSubRepo           UserSubscriptionRepository
	userGroupRateRepo     UserGroupRateRepository
	cache                 GatewayCache
	digestStore           *DigestSessionStore
	cfg                   *config.Config
	schedulerSnapshot     *SchedulerSnapshotService
	billingService        *BillingService
	rateLimitService      *RateLimitService
	billingCacheService   *BillingCacheService
	identityService       *IdentityService
	httpUpstream          HTTPUpstream
	deferredService       *DeferredService
	concurrencyService    *ConcurrencyService
	claudeTokenProvider   *ClaudeTokenProvider
	sessionLimitCache     SessionLimitCache // 会话数量限制缓存（仅 Anthropic OAuth/SetupToken）
	rpmCache              RPMCache          // RPM 计数缓存（仅 Anthropic OAuth/SetupToken）
	userGroupRateResolver *userGroupRateResolver
	userGroupRateCache    *gocache.Cache
	userGroupRateSF       singleflight.Group
	modelsListCache       *gocache.Cache
	modelsListCacheTTL    time.Duration
	settingService        *SettingService
	responseHeaderFilter  *responseheaders.CompiledHeaderFilter
	debugModelRouting     atomic.Bool
	debugClaudeMimic      atomic.Bool
	channelService        *ChannelService
	resolver              *ModelPricingResolver
	debugGatewayBodyFile  atomic.Pointer[os.File] // non-nil when SUB2API_DEBUG_GATEWAY_BODY is set
	tlsFPProfileService   *TLSFingerprintProfileService
	balanceNotifyService  *BalanceNotifyService
}

// NewGatewayService creates a new GatewayService
func NewGatewayService(
	accountRepo AccountRepository,
	groupRepo GroupRepository,
	usageLogRepo UsageLogRepository,
	usageBillingRepo UsageBillingRepository,
	userRepo UserRepository,
	userSubRepo UserSubscriptionRepository,
	userGroupRateRepo UserGroupRateRepository,
	cache GatewayCache,
	cfg *config.Config,
	schedulerSnapshot *SchedulerSnapshotService,
	concurrencyService *ConcurrencyService,
	billingService *BillingService,
	rateLimitService *RateLimitService,
	billingCacheService *BillingCacheService,
	identityService *IdentityService,
	httpUpstream HTTPUpstream,
	deferredService *DeferredService,
	claudeTokenProvider *ClaudeTokenProvider,
	sessionLimitCache SessionLimitCache,
	rpmCache RPMCache,
	digestStore *DigestSessionStore,
	settingService *SettingService,
	tlsFPProfileService *TLSFingerprintProfileService,
	channelService *ChannelService,
	resolver *ModelPricingResolver,
	balanceNotifyService *BalanceNotifyService,
) *GatewayService {
	userGroupRateTTL := resolveUserGroupRateCacheTTL(cfg)
	modelsListTTL := resolveModelsListCacheTTL(cfg)

	svc := &GatewayService{
		accountRepo:          accountRepo,
		groupRepo:            groupRepo,
		usageLogRepo:         usageLogRepo,
		usageBillingRepo:     usageBillingRepo,
		userRepo:             userRepo,
		userSubRepo:          userSubRepo,
		userGroupRateRepo:    userGroupRateRepo,
		cache:                cache,
		digestStore:          digestStore,
		cfg:                  cfg,
		schedulerSnapshot:    schedulerSnapshot,
		concurrencyService:   concurrencyService,
		billingService:       billingService,
		rateLimitService:     rateLimitService,
		billingCacheService:  billingCacheService,
		identityService:      identityService,
		httpUpstream:         httpUpstream,
		deferredService:      deferredService,
		claudeTokenProvider:  claudeTokenProvider,
		sessionLimitCache:    sessionLimitCache,
		rpmCache:             rpmCache,
		userGroupRateCache:   gocache.New(userGroupRateTTL, time.Minute),
		settingService:       settingService,
		modelsListCache:      gocache.New(modelsListTTL, time.Minute),
		modelsListCacheTTL:   modelsListTTL,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
		tlsFPProfileService:  tlsFPProfileService,
		channelService:       channelService,
		resolver:             resolver,
		balanceNotifyService: balanceNotifyService,
	}
	svc.userGroupRateResolver = newUserGroupRateResolver(
		userGroupRateRepo,
		svc.userGroupRateCache,
		userGroupRateTTL,
		&svc.userGroupRateSF,
		"service.gateway",
	)
	svc.debugModelRouting.Store(parseDebugEnvBool(os.Getenv("SUB2API_DEBUG_MODEL_ROUTING")))
	svc.debugClaudeMimic.Store(parseDebugEnvBool(os.Getenv("SUB2API_DEBUG_CLAUDE_MIMIC")))
	if path := strings.TrimSpace(os.Getenv(debugGatewayBodyEnv)); path != "" {
		svc.initDebugGatewayBodyFile(path)
	}
	return svc
}

// GenerateSessionHash 从预解析请求计算粘性会话 hash
func (s *GatewayService) GenerateSessionHash(parsed *ParsedRequest) string {
	if parsed == nil {
		return ""
	}

	// 1. 最高优先级：从 metadata.user_id 提取 session_xxx
	if parsed.MetadataUserID != "" {
		if uid := ParseMetadataUserID(parsed.MetadataUserID); uid != nil && uid.SessionID != "" {
			return uid.SessionID
		}
	}

	// 2. 提取带 cache_control: {type: "ephemeral"} 的内容
	cacheableContent := s.extractCacheableContent(parsed)
	if cacheableContent != "" {
		return s.hashContent(cacheableContent)
	}

	// 3. 最后 fallback: 使用 session上下文 + system + 所有消息的完整摘要串
	var combined strings.Builder
	// 混入请求上下文区分因子，避免不同用户相同消息产生相同 hash
	if parsed.SessionContext != nil {
		_, _ = combined.WriteString(parsed.SessionContext.ClientIP)
		_, _ = combined.WriteString(":")
		_, _ = combined.WriteString(NormalizeSessionUserAgent(parsed.SessionContext.UserAgent))
		_, _ = combined.WriteString(":")
		_, _ = combined.WriteString(strconv.FormatInt(parsed.SessionContext.APIKeyID, 10))
		_, _ = combined.WriteString("|")
	}
	if parsed.System != nil {
		systemText := s.extractTextFromSystem(parsed.System)
		if systemText != "" {
			_, _ = combined.WriteString(systemText)
		}
	}
	for _, msg := range parsed.Messages {
		if m, ok := msg.(map[string]any); ok {
			if content, exists := m["content"]; exists {
				// Anthropic: messages[].content
				if msgText := s.extractTextFromContent(content); msgText != "" {
					_, _ = combined.WriteString(msgText)
				}
			} else if parts, ok := m["parts"].([]any); ok {
				// Gemini: contents[].parts[].text
				for _, part := range parts {
					if partMap, ok := part.(map[string]any); ok {
						if text, ok := partMap["text"].(string); ok {
							_, _ = combined.WriteString(text)
						}
					}
				}
			}
		}
	}
	if combined.Len() > 0 {
		return s.hashContent(combined.String())
	}

	return ""
}

// BindStickySession sets session -> account binding with standard TTL.
func (s *GatewayService) BindStickySession(ctx context.Context, groupID *int64, sessionHash string, accountID int64) error {
	if sessionHash == "" || accountID <= 0 || s.cache == nil {
		return nil
	}
	return s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, accountID, stickySessionTTL)
}

// GetCachedSessionAccountID retrieves the account ID bound to a sticky session.
// Returns 0 if no binding exists or on error.
func (s *GatewayService) GetCachedSessionAccountID(ctx context.Context, groupID *int64, sessionHash string) (int64, error) {
	if sessionHash == "" || s.cache == nil {
		return 0, nil
	}
	accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
	if err != nil {
		return 0, err
	}
	return accountID, nil
}

// FindGeminiSession 查找 Gemini 会话（基于内容摘要链的 Fallback 匹配）
// 返回最长匹配的会话信息（uuid, accountID）
func (s *GatewayService) FindGeminiSession(_ context.Context, groupID int64, prefixHash, digestChain string) (uuid string, accountID int64, matchedChain string, found bool) {
	if digestChain == "" || s.digestStore == nil {
		return "", 0, "", false
	}
	return s.digestStore.Find(groupID, prefixHash, digestChain)
}

// SaveGeminiSession 保存 Gemini 会话。oldDigestChain 为 Find 返回的 matchedChain，用于删旧 key。
func (s *GatewayService) SaveGeminiSession(_ context.Context, groupID int64, prefixHash, digestChain, uuid string, accountID int64, oldDigestChain string) error {
	if digestChain == "" || s.digestStore == nil {
		return nil
	}
	s.digestStore.Save(groupID, prefixHash, digestChain, uuid, accountID, oldDigestChain)
	return nil
}

// FindAnthropicSession 查找 Anthropic 会话（基于内容摘要链的 Fallback 匹配）
func (s *GatewayService) FindAnthropicSession(_ context.Context, groupID int64, prefixHash, digestChain string) (uuid string, accountID int64, matchedChain string, found bool) {
	if digestChain == "" || s.digestStore == nil {
		return "", 0, "", false
	}
	return s.digestStore.Find(groupID, prefixHash, digestChain)
}

// SaveAnthropicSession 保存 Anthropic 会话
func (s *GatewayService) SaveAnthropicSession(_ context.Context, groupID int64, prefixHash, digestChain, uuid string, accountID int64, oldDigestChain string) error {
	if digestChain == "" || s.digestStore == nil {
		return nil
	}
	s.digestStore.Save(groupID, prefixHash, digestChain, uuid, accountID, oldDigestChain)
	return nil
}

func (s *GatewayService) extractCacheableContent(parsed *ParsedRequest) string {
	if parsed == nil {
		return ""
	}

	var builder strings.Builder

	// 检查 system 中的 cacheable 内容
	if system, ok := parsed.System.([]any); ok {
		for _, part := range system {
			if partMap, ok := part.(map[string]any); ok {
				if cc, ok := partMap["cache_control"].(map[string]any); ok {
					if cc["type"] == "ephemeral" {
						if text, ok := partMap["text"].(string); ok {
							_, _ = builder.WriteString(text)
						}
					}
				}
			}
		}
	}
	systemText := builder.String()

	// 检查 messages 中的 cacheable 内容
	for _, msg := range parsed.Messages {
		if msgMap, ok := msg.(map[string]any); ok {
			if msgContent, ok := msgMap["content"].([]any); ok {
				for _, part := range msgContent {
					if partMap, ok := part.(map[string]any); ok {
						if cc, ok := partMap["cache_control"].(map[string]any); ok {
							if cc["type"] == "ephemeral" {
								return s.extractTextFromContent(msgMap["content"])
							}
						}
					}
				}
			}
		}
	}

	return systemText
}

func (s *GatewayService) extractTextFromSystem(system any) string {
	switch v := system.(type) {
	case string:
		return v
	case []any:
		var texts []string
		for _, part := range v {
			if partMap, ok := part.(map[string]any); ok {
				if text, ok := partMap["text"].(string); ok {
					texts = append(texts, text)
				}
			}
		}
		return strings.Join(texts, "")
	}
	return ""
}

func (s *GatewayService) extractTextFromContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var texts []string
		for _, part := range v {
			if partMap, ok := part.(map[string]any); ok {
				if partMap["type"] == "text" {
					if text, ok := partMap["text"].(string); ok {
						texts = append(texts, text)
					}
				}
			}
		}
		return strings.Join(texts, "")
	}
	return ""
}

func (s *GatewayService) hashContent(content string) string {
	h := xxhash.Sum64String(content)
	return strconv.FormatUint(h, 36)
}

type anthropicCacheControlPayload struct {
	Type string `json:"type"`
}

type anthropicSystemTextBlockPayload struct {
	Type         string                        `json:"type"`
	Text         string                        `json:"text"`
	CacheControl *anthropicCacheControlPayload `json:"cache_control,omitempty"`
}

type anthropicMetadataPayload struct {
	UserID string `json:"user_id"`
}

// replaceModelInBody 替换请求体中的model字段
// 优先使用定点修改，尽量保持客户端原始字段顺序。
func (s *GatewayService) replaceModelInBody(body []byte, newModel string) []byte {
	return ReplaceModelInBody(body, newModel)
}

type claudeOAuthNormalizeOptions struct {
	injectMetadata          bool
	metadataUserID          string
	stripSystemCacheControl bool
}

// sanitizeSystemText rewrites only the fixed OpenCode identity sentence (if present).
// We intentionally avoid broad keyword replacement in system prompts to prevent
// accidentally changing user-provided instructions.
func sanitizeSystemText(text string) string {
	if text == "" {
		return text
	}
	// Some clients include a fixed OpenCode identity sentence. Anthropic may treat
	// this as a non-Claude-Code fingerprint, so rewrite it to the canonical
	// Claude Code banner before generic "OpenCode"/"opencode" replacements.
	text = strings.ReplaceAll(
		text,
		"You are OpenCode, the best coding agent on the planet.",
		strings.TrimSpace(claudeCodeSystemPrompt),
	)
	return text
}

func marshalAnthropicSystemTextBlock(text string, includeCacheControl bool) ([]byte, error) {
	block := anthropicSystemTextBlockPayload{
		Type: "text",
		Text: text,
	}
	if includeCacheControl {
		block.CacheControl = &anthropicCacheControlPayload{Type: "ephemeral"}
	}
	return json.Marshal(block)
}

func marshalAnthropicMetadata(userID string) ([]byte, error) {
	return json.Marshal(anthropicMetadataPayload{UserID: userID})
}

func buildJSONArrayRaw(items [][]byte) []byte {
	if len(items) == 0 {
		return []byte("[]")
	}

	total := 2
	for _, item := range items {
		total += len(item)
	}
	total += len(items) - 1

	buf := make([]byte, 0, total)
	buf = append(buf, '[')
	for i, item := range items {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, item...)
	}
	buf = append(buf, ']')
	return buf
}

func setJSONValueBytes(body []byte, path string, value any) ([]byte, bool) {
	next, err := sjson.SetBytes(body, path, value)
	if err != nil {
		return body, false
	}
	return next, true
}

func setJSONRawBytes(body []byte, path string, raw []byte) ([]byte, bool) {
	next, err := sjson.SetRawBytes(body, path, raw)
	if err != nil {
		return body, false
	}
	return next, true
}

func deleteJSONPathBytes(body []byte, path string) ([]byte, bool) {
	next, err := sjson.DeleteBytes(body, path)
	if err != nil {
		return body, false
	}
	return next, true
}

func normalizeClaudeOAuthSystemBody(body []byte, opts claudeOAuthNormalizeOptions) ([]byte, bool) {
	sys := gjson.GetBytes(body, "system")
	if !sys.Exists() {
		return body, false
	}

	out := body
	modified := false

	switch {
	case sys.Type == gjson.String:
		sanitized := sanitizeSystemText(sys.String())
		if sanitized != sys.String() {
			if next, ok := setJSONValueBytes(out, "system", sanitized); ok {
				out = next
				modified = true
			}
		}
	case sys.IsArray():
		index := 0
		sys.ForEach(func(_, item gjson.Result) bool {
			if item.Get("type").String() == "text" {
				textResult := item.Get("text")
				if textResult.Exists() && textResult.Type == gjson.String {
					text := textResult.String()
					sanitized := sanitizeSystemText(text)
					if sanitized != text {
						if next, ok := setJSONValueBytes(out, fmt.Sprintf("system.%d.text", index), sanitized); ok {
							out = next
							modified = true
						}
					}
				}
			}

			if opts.stripSystemCacheControl && item.Get("cache_control").Exists() {
				if next, ok := deleteJSONPathBytes(out, fmt.Sprintf("system.%d.cache_control", index)); ok {
					out = next
					modified = true
				}
			}

			index++
			return true
		})
	}

	return out, modified
}

func ensureClaudeOAuthMetadataUserID(body []byte, userID string) ([]byte, bool) {
	if strings.TrimSpace(userID) == "" {
		return body, false
	}

	metadata := gjson.GetBytes(body, "metadata")
	if !metadata.Exists() || metadata.Type == gjson.Null {
		raw, err := marshalAnthropicMetadata(userID)
		if err != nil {
			return body, false
		}
		return setJSONRawBytes(body, "metadata", raw)
	}

	trimmedRaw := strings.TrimSpace(metadata.Raw)
	if strings.HasPrefix(trimmedRaw, "{") {
		existing := metadata.Get("user_id")
		if existing.Exists() && existing.Type == gjson.String && existing.String() != "" {
			return body, false
		}
		return setJSONValueBytes(body, "metadata.user_id", userID)
	}

	raw, err := marshalAnthropicMetadata(userID)
	if err != nil {
		return body, false
	}
	return setJSONRawBytes(body, "metadata", raw)
}

func normalizeClaudeOAuthRequestBody(body []byte, modelID string, opts claudeOAuthNormalizeOptions) ([]byte, string) {
	if len(body) == 0 {
		return body, modelID
	}

	out := body
	modified := false

	if next, changed := normalizeClaudeOAuthSystemBody(out, opts); changed {
		out = next
		modified = true
	}

	rawModel := gjson.GetBytes(out, "model")
	if rawModel.Exists() && rawModel.Type == gjson.String {
		normalized := claude.NormalizeModelID(rawModel.String())
		if normalized != rawModel.String() {
			if next, ok := setJSONValueBytes(out, "model", normalized); ok {
				out = next
				modified = true
			}
			modelID = normalized
		}
	}

	// 确保 tools 字段存在（即使为空数组）
	if !gjson.GetBytes(out, "tools").Exists() {
		if next, ok := setJSONRawBytes(out, "tools", []byte("[]")); ok {
			out = next
			modified = true
		}
	}

	if opts.injectMetadata && opts.metadataUserID != "" {
		if next, changed := ensureClaudeOAuthMetadataUserID(out, opts.metadataUserID); changed {
			out = next
			modified = true
		}
	}

	if gjson.GetBytes(out, "temperature").Exists() {
		if next, ok := deleteJSONPathBytes(out, "temperature"); ok {
			out = next
			modified = true
		}
	}
	if gjson.GetBytes(out, "tool_choice").Exists() {
		if next, ok := deleteJSONPathBytes(out, "tool_choice"); ok {
			out = next
			modified = true
		}
	}

	if !modified {
		return body, modelID
	}

	return out, modelID
}

func (s *GatewayService) buildOAuthMetadataUserID(parsed *ParsedRequest, account *Account, fp *Fingerprint) string {
	if parsed == nil || account == nil {
		return ""
	}
	if parsed.MetadataUserID != "" {
		return ""
	}

	userID := strings.TrimSpace(account.GetClaudeUserID())
	if userID == "" && fp != nil {
		userID = fp.ClientID
	}
	if userID == "" {
		// Fall back to a random, well-formed client id so we can still satisfy
		// Claude Code OAuth requirements when account metadata is incomplete.
		userID = generateClientID()
	}

	sessionHash := s.GenerateSessionHash(parsed)
	sessionID := uuid.NewString()
	if sessionHash != "" {
		seed := fmt.Sprintf("%d::%s", account.ID, sessionHash)
		sessionID = generateSessionUUID(seed)
	}

	// 根据指纹 UA 版本选择输出格式
	var uaVersion string
	if fp != nil {
		uaVersion = ExtractCLIVersion(fp.UserAgent)
	}
	accountUUID := strings.TrimSpace(account.GetExtraString("account_uuid"))
	return FormatMetadataUserID(userID, accountUUID, sessionID, uaVersion)
}

// GenerateSessionUUID creates a deterministic UUID4 from a seed string.
func GenerateSessionUUID(seed string) string {
	return generateSessionUUID(seed)
}

func generateSessionUUID(seed string) string {
	if seed == "" {
		return uuid.NewString()
	}
	hash := sha256.Sum256([]byte(seed))
	bytes := hash[:16]
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
}

// SelectAccount 选择账号（粘性会话+优先级）
func (s *GatewayService) SelectAccount(ctx context.Context, groupID *int64, sessionHash string) (*Account, error) {
	return s.SelectAccountForModel(ctx, groupID, sessionHash, "")
}

// SelectAccountForModel 选择支持指定模型的账号（粘性会话+优先级+模型映射）
func (s *GatewayService) SelectAccountForModel(ctx context.Context, groupID *int64, sessionHash string, requestedModel string) (*Account, error) {
	return s.SelectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, nil)
}

// SelectAccountForModelWithExclusions selects an account supporting the requested model while excluding specified accounts.
func (s *GatewayService) SelectAccountForModelWithExclusions(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}) (*Account, error) {
	// 优先检查 context 中的强制平台（/antigravity 路由）
	var platform string
	forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string)
	if hasForcePlatform && forcePlatform != "" {
		platform = forcePlatform
	} else if groupID != nil {
		group, resolvedGroupID, err := s.resolveGatewayGroup(ctx, groupID)
		if err != nil {
			return nil, err
		}
		groupID = resolvedGroupID
		ctx = s.withGroupContext(ctx, group)
		platform = group.Platform
	} else {
		// 无分组时只使用原生 anthropic 平台
		platform = PlatformAnthropic
	}

	// Claude Code 限制可能已将 groupID 解析为 fallback group，
	// 渠道限制预检查必须使用解析后的分组。
	if s.checkChannelPricingRestriction(ctx, groupID, requestedModel) {
		slog.Warn("channel pricing restriction blocked request",
			"group_id", derefGroupID(groupID),
			"model", requestedModel)
		return nil, fmt.Errorf("%w supporting model: %s (channel pricing restriction)", ErrNoAvailableAccounts, requestedModel)
	}

	// anthropic/gemini 分组支持混合调度（包含启用了 mixed_scheduling 的 antigravity 账户）
	// 注意：强制平台模式不走混合调度
	if (platform == PlatformAnthropic || platform == PlatformGemini) && !hasForcePlatform {
		account, err := s.selectAccountWithMixedScheduling(ctx, groupID, sessionHash, requestedModel, excludedIDs, platform)
		if err != nil {
			return nil, err
		}
		return s.hydrateSelectedAccount(ctx, account)
	}

	// antigravity 分组、强制平台模式或无分组使用单平台选择
	// 注意：强制平台模式也必须遵守分组限制，不再回退到全平台查询
	account, err := s.selectAccountForModelWithPlatform(ctx, groupID, sessionHash, requestedModel, excludedIDs, platform)
	if err != nil {
		return nil, err
	}
	return s.hydrateSelectedAccount(ctx, account)
}

// SelectAccountWithLoadAwareness selects account with load-awareness and wait plan.
// 调度流程文档见 docs/ACCOUNT_SCHEDULING_FLOW.md 。
// metadataUserID: 用于客户端亲和调度，从中提取客户端 ID
// sub2apiUserID: 系统用户 ID，用于二维亲和调度
func (s *GatewayService) SelectAccountWithLoadAwareness(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, metadataUserID string, sub2apiUserID int64) (*AccountSelectionResult, error) {
	// 调试日志：记录调度入口参数
	excludedIDsList := make([]int64, 0, len(excludedIDs))
	for id := range excludedIDs {
		excludedIDsList = append(excludedIDsList, id)
	}
	slog.Debug("account_scheduling_starting",
		"group_id", derefGroupID(groupID),
		"model", requestedModel,
		"session", shortSessionHash(sessionHash),
		"excluded_ids", excludedIDsList)

	cfg := s.schedulingConfig()

	// 检查 Claude Code 客户端限制（可能会替换 groupID 为降级分组）
	group, groupID, err := s.checkClaudeCodeRestriction(ctx, groupID)
	if err != nil {
		return nil, err
	}
	ctx = s.withGroupContext(ctx, group)

	// Claude Code 限制可能已将 groupID 解析为 fallback group，
	// 渠道限制预检查必须使用解析后的分组。
	if s.checkChannelPricingRestriction(ctx, groupID, requestedModel) {
		slog.Warn("channel pricing restriction blocked request",
			"group_id", derefGroupID(groupID),
			"model", requestedModel)
		return nil, fmt.Errorf("%w supporting model: %s (channel pricing restriction)", ErrNoAvailableAccounts, requestedModel)
	}

	var stickyAccountID int64
	if prefetch := prefetchedStickyAccountIDFromContext(ctx, groupID); prefetch > 0 {
		stickyAccountID = prefetch
	} else if sessionHash != "" && s.cache != nil {
		if accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash); err == nil {
			stickyAccountID = accountID
		}
	}

	if s.debugModelRoutingEnabled() && requestedModel != "" {
		groupPlatform := ""
		if group != nil {
			groupPlatform = group.Platform
		}
		logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] select entry: group_id=%v group_platform=%s model=%s session=%s sticky_account=%d load_batch=%v concurrency=%v",
			derefGroupID(groupID), groupPlatform, requestedModel, shortSessionHash(sessionHash), stickyAccountID, cfg.LoadBatchEnabled, s.concurrencyService != nil)
	}

	if s.concurrencyService == nil || !cfg.LoadBatchEnabled {
		// 复制排除列表，用于会话限制拒绝时的重试
		localExcluded := make(map[int64]struct{})
		for k, v := range excludedIDs {
			localExcluded[k] = v
		}

		for {
			account, err := s.SelectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, localExcluded)
			if err != nil {
				return nil, err
			}

			result, err := s.tryAcquireAccountSlot(ctx, account.ID, account.Concurrency)
			if err == nil && result.Acquired {
				// 获取槽位后检查会话限制（使用 sessionHash 作为会话标识符）
				if !s.checkAndRegisterSession(ctx, account, sessionHash) {
					result.ReleaseFunc()                   // 释放槽位
					localExcluded[account.ID] = struct{}{} // 排除此账号
					continue                               // 重新选择
				}
				return s.newSelectionResult(ctx, account, true, result.ReleaseFunc, nil)
			}

			// 对于等待计划的情况，也需要先检查会话限制
			if !s.checkAndRegisterSession(ctx, account, sessionHash) {
				localExcluded[account.ID] = struct{}{}
				continue
			}

			if stickyAccountID > 0 && stickyAccountID == account.ID && s.concurrencyService != nil {
				waitingCount, _ := s.concurrencyService.GetAccountWaitingCount(ctx, account.ID)
				if waitingCount < cfg.StickySessionMaxWaiting {
					return s.newSelectionResult(ctx, account, false, nil, &AccountWaitPlan{
						AccountID:      account.ID,
						MaxConcurrency: account.Concurrency,
						Timeout:        cfg.StickySessionWaitTimeout,
						MaxWaiting:     cfg.StickySessionMaxWaiting,
					})
				}
			}
			return s.newSelectionResult(ctx, account, false, nil, &AccountWaitPlan{
				AccountID:      account.ID,
				MaxConcurrency: account.Concurrency,
				Timeout:        cfg.FallbackWaitTimeout,
				MaxWaiting:     cfg.FallbackMaxWaiting,
			})
		}
	}

	platform, hasForcePlatform, err := s.resolvePlatform(ctx, groupID, group)
	if err != nil {
		return nil, err
	}
	preferOAuth := platform == PlatformGemini
	if s.debugModelRoutingEnabled() && platform == PlatformAnthropic && requestedModel != "" {
		logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] load-aware enabled: group_id=%v model=%s session=%s platform=%s", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), platform)
	}

	accounts, useMixed, err := s.listSchedulableAccounts(ctx, groupID, platform, hasForcePlatform)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, ErrNoAvailableAccounts
	}
	ctx = s.withWindowCostPrefetch(ctx, accounts)
	ctx = s.withRPMPrefetch(ctx, accounts)

	// 提前构建 accountByID（供 Layer 1 和 Layer 1.5 使用）
	accountByID := make(map[int64]*Account, len(accounts))
	for i := range accounts {
		accountByID[accounts[i].ID] = &accounts[i]
	}
	isExcluded := func(accountID int64) bool {
		if excludedIDs == nil {
			return false
		}
		_, excluded := excludedIDs[accountID]
		return excluded
	}

	// 获取模型路由配置（仅 anthropic 平台）
	var routingAccountIDs []int64
	if group != nil && requestedModel != "" && group.Platform == PlatformAnthropic {
		routingAccountIDs = group.GetRoutingAccountIDs(requestedModel)
		if s.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] context group routing: group_id=%d model=%s enabled=%v rules=%d matched_ids=%v session=%s sticky_account=%d",
				group.ID, requestedModel, group.ModelRoutingEnabled, len(group.ModelRouting), routingAccountIDs, shortSessionHash(sessionHash), stickyAccountID)
			if len(routingAccountIDs) == 0 && group.ModelRoutingEnabled && len(group.ModelRouting) > 0 {
				keys := make([]string, 0, len(group.ModelRouting))
				for k := range group.ModelRouting {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				const maxKeys = 20
				if len(keys) > maxKeys {
					keys = keys[:maxKeys]
				}
				logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] context group routing miss: group_id=%d model=%s patterns(sample)=%v", group.ID, requestedModel, keys)
			}
		}
	}

	// ============ Layer 1: 模型路由优先选择（优先级高于粘性会话） ============
	if len(routingAccountIDs) > 0 && s.concurrencyService != nil {
		// 1. 过滤出路由列表中可调度的账号
		var routingCandidates []*Account
		var filteredExcluded, filteredMissing, filteredUnsched, filteredPlatform, filteredModelScope, filteredModelMapping, filteredWindowCost int
		var modelScopeSkippedIDs []int64 // 记录因模型限流被跳过的账号 ID
		for _, routingAccountID := range routingAccountIDs {
			if isExcluded(routingAccountID) {
				filteredExcluded++
				continue
			}
			account, ok := accountByID[routingAccountID]
			if !ok || !s.isAccountSchedulableForSelection(account) {
				if !ok {
					filteredMissing++
				} else {
					filteredUnsched++
				}
				continue
			}
			if !s.isAccountAllowedForPlatform(account, platform, useMixed) {
				filteredPlatform++
				continue
			}
			if requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, account, requestedModel) {
				filteredModelMapping++
				continue
			}
			if !s.isAccountSchedulableForModelSelection(ctx, account, requestedModel) {
				filteredModelScope++
				modelScopeSkippedIDs = append(modelScopeSkippedIDs, account.ID)
				continue
			}
			// 配额检查
			if !s.isAccountSchedulableForQuota(account) {
				continue
			}
			// 窗口费用检查（非粘性会话路径）
			if !s.isAccountSchedulableForWindowCost(ctx, account, false) {
				filteredWindowCost++
				continue
			}
			// RPM 检查（非粘性会话路径）
			if !s.isAccountSchedulableForRPM(ctx, account, false) {
				continue
			}
			routingCandidates = append(routingCandidates, account)
		}

		if s.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routed candidates: group_id=%v model=%s routed=%d candidates=%d filtered(excluded=%d missing=%d unsched=%d platform=%d model_scope=%d model_mapping=%d window_cost=%d)",
				derefGroupID(groupID), requestedModel, len(routingAccountIDs), len(routingCandidates),
				filteredExcluded, filteredMissing, filteredUnsched, filteredPlatform, filteredModelScope, filteredModelMapping, filteredWindowCost)
			if len(modelScopeSkippedIDs) > 0 {
				logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] model_rate_limited accounts skipped: group_id=%v model=%s account_ids=%v",
					derefGroupID(groupID), requestedModel, modelScopeSkippedIDs)
			}
		}

		if len(routingCandidates) > 0 {
			// 1.5. 在路由账号范围内检查粘性会话
			if sessionHash != "" && stickyAccountID > 0 {
				if containsInt64(routingAccountIDs, stickyAccountID) && !isExcluded(stickyAccountID) {
					// 粘性账号在路由列表中，优先使用
					if stickyAccount, ok := accountByID[stickyAccountID]; ok {
						var stickyCacheMissReason string

						gatePass := s.isAccountSchedulableForSelection(stickyAccount) &&
							s.isAccountAllowedForPlatform(stickyAccount, platform, useMixed) &&
							(requestedModel == "" || s.isModelSupportedByAccountWithContext(ctx, stickyAccount, requestedModel)) &&
							s.isAccountSchedulableForModelSelection(ctx, stickyAccount, requestedModel) &&
							s.isAccountSchedulableForQuota(stickyAccount) &&
							s.isAccountSchedulableForWindowCost(ctx, stickyAccount, true)

						rpmPass := gatePass && s.isAccountSchedulableForRPM(ctx, stickyAccount, true)

						if rpmPass { // 粘性会话窗口费用+RPM 检查
							result, err := s.tryAcquireAccountSlot(ctx, stickyAccountID, stickyAccount.Concurrency)
							if err == nil && result.Acquired {
								// 会话数量限制检查
								if !s.checkAndRegisterSession(ctx, stickyAccount, sessionHash) {
									result.ReleaseFunc() // 释放槽位
									stickyCacheMissReason = "session_limit"
									// 继续到负载感知选择
								} else {
									if s.debugModelRoutingEnabled() {
										logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routed sticky hit: group_id=%v model=%s session=%s account=%d", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), stickyAccountID)
									}
									return s.newSelectionResult(ctx, stickyAccount, true, result.ReleaseFunc, nil)
								}
							}

							if stickyCacheMissReason == "" {
								waitingCount, _ := s.concurrencyService.GetAccountWaitingCount(ctx, stickyAccountID)
								if waitingCount < cfg.StickySessionMaxWaiting {
									// 会话数量限制检查（等待计划也需要占用会话配额）
									if !s.checkAndRegisterSession(ctx, stickyAccount, sessionHash) {
										stickyCacheMissReason = "session_limit"
										// 会话限制已满，继续到负载感知选择
									} else {
										return &AccountSelectionResult{
											Account: stickyAccount,
											WaitPlan: &AccountWaitPlan{
												AccountID:      stickyAccountID,
												MaxConcurrency: stickyAccount.Concurrency,
												Timeout:        cfg.StickySessionWaitTimeout,
												MaxWaiting:     cfg.StickySessionMaxWaiting,
											},
										}, nil
									}
								} else {
									stickyCacheMissReason = "wait_queue_full"
								}
							}
							// 粘性账号槽位满且等待队列已满，继续使用负载感知选择
						} else if !gatePass {
							stickyCacheMissReason = "gate_check"
						} else {
							stickyCacheMissReason = "rpm_red"
						}

						// 记录粘性缓存未命中的结构化日志
						if stickyCacheMissReason != "" {
							baseRPM := stickyAccount.GetBaseRPM()
							var currentRPM int
							if count, ok := rpmFromPrefetchContext(ctx, stickyAccount.ID); ok {
								currentRPM = count
							}
							logger.LegacyPrintf("service.gateway", "[StickyCacheMiss] reason=%s account_id=%d session=%s current_rpm=%d base_rpm=%d",
								stickyCacheMissReason, stickyAccountID, shortSessionHash(sessionHash), currentRPM, baseRPM)
						}
					} else {
						_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
						logger.LegacyPrintf("service.gateway", "[StickyCacheMiss] reason=account_cleared account_id=%d session=%s current_rpm=0 base_rpm=0",
							stickyAccountID, shortSessionHash(sessionHash))
					}
				}
			}

			// 2. 批量获取负载信息
			routingLoads := make([]AccountWithConcurrency, 0, len(routingCandidates))
			for _, acc := range routingCandidates {
				routingLoads = append(routingLoads, AccountWithConcurrency{
					ID:             acc.ID,
					MaxConcurrency: acc.EffectiveLoadFactor(),
				})
			}
			routingLoadMap, _ := s.concurrencyService.GetAccountsLoadBatch(ctx, routingLoads)

			// 3. 按负载感知排序
			var routingAvailable []accountWithLoad
			for _, acc := range routingCandidates {
				loadInfo := routingLoadMap[acc.ID]
				if loadInfo == nil {
					loadInfo = &AccountLoadInfo{AccountID: acc.ID}
				}
				if loadInfo.LoadRate < 100 {
					routingAvailable = append(routingAvailable, accountWithLoad{account: acc, loadInfo: loadInfo})
				}
			}

			if len(routingAvailable) > 0 {
				// 排序：优先级 > 负载率 > 最后使用时间
				sort.SliceStable(routingAvailable, func(i, j int) bool {
					a, b := routingAvailable[i], routingAvailable[j]
					if a.account.Priority != b.account.Priority {
						return a.account.Priority < b.account.Priority
					}
					if a.loadInfo.LoadRate != b.loadInfo.LoadRate {
						return a.loadInfo.LoadRate < b.loadInfo.LoadRate
					}
					switch {
					case a.account.LastUsedAt == nil && b.account.LastUsedAt != nil:
						return true
					case a.account.LastUsedAt != nil && b.account.LastUsedAt == nil:
						return false
					case a.account.LastUsedAt == nil && b.account.LastUsedAt == nil:
						return false
					default:
						return a.account.LastUsedAt.Before(*b.account.LastUsedAt)
					}
				})
				shuffleWithinSortGroups(routingAvailable)

				// 4. 尝试获取槽位
				for _, item := range routingAvailable {
					result, err := s.tryAcquireAccountSlot(ctx, item.account.ID, item.account.Concurrency)
					if err == nil && result.Acquired {
						// 会话数量限制检查
						if !s.checkAndRegisterSession(ctx, item.account, sessionHash) {
							result.ReleaseFunc() // 释放槽位，继续尝试下一个账号
							continue
						}
						if sessionHash != "" && s.cache != nil {
							_ = s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, item.account.ID, stickySessionTTL)
						}
						if s.debugModelRoutingEnabled() {
							logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routed select: group_id=%v model=%s session=%s account=%d", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), item.account.ID)
						}
						return s.newSelectionResult(ctx, item.account, true, result.ReleaseFunc, nil)
					}
				}

				// 5. 所有路由账号槽位满，尝试返回等待计划（选择负载最低的）
				// 遍历找到第一个满足会话限制的账号
				for _, item := range routingAvailable {
					if !s.checkAndRegisterSession(ctx, item.account, sessionHash) {
						continue // 会话限制已满，尝试下一个
					}
					if s.debugModelRoutingEnabled() {
						logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routed wait: group_id=%v model=%s session=%s account=%d", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), item.account.ID)
					}
					return s.newSelectionResult(ctx, item.account, false, nil, &AccountWaitPlan{
						AccountID:      item.account.ID,
						MaxConcurrency: item.account.Concurrency,
						Timeout:        cfg.StickySessionWaitTimeout,
						MaxWaiting:     cfg.StickySessionMaxWaiting,
					})
				}
				// 所有路由账号会话限制都已满，继续到 Layer 2 回退
			}
			// 路由列表中的账号都不可用（负载率 >= 100），继续到 Layer 2 回退
			logger.LegacyPrintf("service.gateway", "[ModelRouting] All routed accounts unavailable for model=%s, falling back to normal selection", requestedModel)
		}
	}

	// ============ Layer 1.5: 粘性会话（仅在无模型路由配置时生效） ============
	if len(routingAccountIDs) == 0 && sessionHash != "" && stickyAccountID > 0 && !isExcluded(stickyAccountID) {
		accountID := stickyAccountID
		if accountID > 0 && !isExcluded(accountID) {
			account, ok := accountByID[accountID]
			if ok {
				// 检查账户是否需要清理粘性会话绑定
				clearSticky := shouldClearStickySession(account, requestedModel)
				if clearSticky {
					_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
				}
				if !clearSticky && s.isAccountInGroup(account, groupID) &&
					s.isAccountAllowedForPlatform(account, platform, useMixed) &&
					(requestedModel == "" || s.isModelSupportedByAccountWithContext(ctx, account, requestedModel)) &&
					s.isAccountSchedulableForModelSelection(ctx, account, requestedModel) &&
					s.isAccountSchedulableForQuota(account) &&
					s.isAccountSchedulableForWindowCost(ctx, account, true) &&

					s.isAccountSchedulableForRPM(ctx, account, true) { // 粘性会话窗口费用+RPM 检查
					result, err := s.tryAcquireAccountSlot(ctx, accountID, account.Concurrency)
					if err == nil && result.Acquired {
						// 会话数量限制检查
						if !s.checkAndRegisterSession(ctx, account, sessionHash) {
							result.ReleaseFunc() // 释放槽位，继续到 Layer 2
						} else {
							if s.cache != nil {
								_ = s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), sessionHash, stickySessionTTL)
							}
							return s.newSelectionResult(ctx, account, true, result.ReleaseFunc, nil)
						}
					}

					waitingCount, _ := s.concurrencyService.GetAccountWaitingCount(ctx, accountID)
					if waitingCount < cfg.StickySessionMaxWaiting {
						// 会话数量限制检查（等待计划也需要占用会话配额）
						if !s.checkAndRegisterSession(ctx, account, sessionHash) {
							// 会话限制已满，继续到 Layer 2
						} else {
							return s.newSelectionResult(ctx, account, false, nil, &AccountWaitPlan{
								AccountID:      accountID,
								MaxConcurrency: account.Concurrency,
								Timeout:        cfg.StickySessionWaitTimeout,
								MaxWaiting:     cfg.StickySessionMaxWaiting,
							})
						}
					}
				}
			}
		}
	}

	// ============ Layer 2: 负载感知选择 ============
	candidates := make([]*Account, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if isExcluded(acc.ID) {
			continue
		}
		// Scheduler snapshots can be temporarily stale (bucket rebuild is throttled);
		// re-check schedulability here so recently rate-limited/overloaded accounts
		// are not selected again before the bucket is rebuilt.
		if !s.isAccountSchedulableForSelection(acc) {
			continue
		}
		if !s.isAccountAllowedForPlatform(acc, platform, useMixed) {
			continue
		}
		if requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, acc, requestedModel) {
			continue
		}
		if !s.isAccountSchedulableForModelSelection(ctx, acc, requestedModel) {
			continue
		}
		// 配额检查
		if !s.isAccountSchedulableForQuota(acc) {
			continue
		}
		// 窗口费用检查（非粘性会话路径）
		if !s.isAccountSchedulableForWindowCost(ctx, acc, false) {
			continue
		}
		// RPM 检查（非粘性会话路径）
		if !s.isAccountSchedulableForRPM(ctx, acc, false) {
			continue
		}
		candidates = append(candidates, acc)
	}

	if len(candidates) == 0 {
		return nil, ErrNoAvailableAccounts
	}

	accountLoads := make([]AccountWithConcurrency, 0, len(candidates))
	for _, acc := range candidates {
		accountLoads = append(accountLoads, AccountWithConcurrency{
			ID:             acc.ID,
			MaxConcurrency: acc.EffectiveLoadFactor(),
		})
	}

	loadMap, err := s.concurrencyService.GetAccountsLoadBatch(ctx, accountLoads)
	if err != nil {
		if result, ok, legacyErr := s.tryAcquireByLegacyOrder(ctx, candidates, groupID, sessionHash, preferOAuth); legacyErr != nil {
			return nil, legacyErr
		} else if ok {
			return result, nil
		}
	} else {
		var available []accountWithLoad
		for _, acc := range candidates {
			loadInfo := loadMap[acc.ID]
			if loadInfo == nil {
				loadInfo = &AccountLoadInfo{AccountID: acc.ID}
			}
			if loadInfo.LoadRate < 100 {
				available = append(available, accountWithLoad{
					account:  acc,
					loadInfo: loadInfo,
				})
			}
		}

		// 分层过滤选择：优先级 → 负载率 → LRU
		for len(available) > 0 {
			// 1. 取优先级最小的集合
			candidates := filterByMinPriority(available)
			// 2. 取负载率最低的集合
			candidates = filterByMinLoadRate(candidates)
			// 3. LRU 选择最久未用的账号
			selected := selectByLRU(candidates, preferOAuth)
			if selected == nil {
				break
			}

			result, err := s.tryAcquireAccountSlot(ctx, selected.account.ID, selected.account.Concurrency)
			if err == nil && result.Acquired {
				// 会话数量限制检查
				if !s.checkAndRegisterSession(ctx, selected.account, sessionHash) {
					result.ReleaseFunc() // 释放槽位，继续尝试下一个账号
				} else {
					if sessionHash != "" && s.cache != nil {
						_ = s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, selected.account.ID, stickySessionTTL)
					}
					return s.newSelectionResult(ctx, selected.account, true, result.ReleaseFunc, nil)
				}
			}

			// 移除已尝试的账号，重新进行分层过滤
			selectedID := selected.account.ID
			newAvailable := make([]accountWithLoad, 0, len(available)-1)
			for _, acc := range available {
				if acc.account.ID != selectedID {
					newAvailable = append(newAvailable, acc)
				}
			}
			available = newAvailable
		}
	}

	// ============ Layer 3: 兜底排队 ============
	s.sortCandidatesForFallback(candidates, preferOAuth, cfg.FallbackSelectionMode)
	for _, acc := range candidates {
		// 会话数量限制检查（等待计划也需要占用会话配额）
		if !s.checkAndRegisterSession(ctx, acc, sessionHash) {
			continue // 会话限制已满，尝试下一个账号
		}
		return s.newSelectionResult(ctx, acc, false, nil, &AccountWaitPlan{
			AccountID:      acc.ID,
			MaxConcurrency: acc.Concurrency,
			Timeout:        cfg.FallbackWaitTimeout,
			MaxWaiting:     cfg.FallbackMaxWaiting,
		})
	}
	return nil, ErrNoAvailableAccounts
}

func (s *GatewayService) tryAcquireByLegacyOrder(ctx context.Context, candidates []*Account, groupID *int64, sessionHash string, preferOAuth bool) (*AccountSelectionResult, bool, error) {
	ordered := append([]*Account(nil), candidates...)
	sortAccountsByPriorityAndLastUsed(ordered, preferOAuth)

	for _, acc := range ordered {
		result, err := s.tryAcquireAccountSlot(ctx, acc.ID, acc.Concurrency)
		if err == nil && result.Acquired {
			// 会话数量限制检查
			if !s.checkAndRegisterSession(ctx, acc, sessionHash) {
				result.ReleaseFunc() // 释放槽位，继续尝试下一个账号
				continue
			}
			if sessionHash != "" && s.cache != nil {
				_ = s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, acc.ID, stickySessionTTL)
			}
			selection, err := s.newSelectionResult(ctx, acc, true, result.ReleaseFunc, nil)
			if err != nil {
				return nil, false, err
			}
			return selection, true, nil
		}
	}

	return nil, false, nil
}

func (s *GatewayService) schedulingConfig() config.GatewaySchedulingConfig {
	if s.cfg != nil {
		return s.cfg.Gateway.Scheduling
	}
	return config.GatewaySchedulingConfig{
		StickySessionMaxWaiting:  3,
		StickySessionWaitTimeout: 45 * time.Second,
		FallbackWaitTimeout:      30 * time.Second,
		FallbackMaxWaiting:       100,
		LoadBatchEnabled:         true,
		SlotCleanupInterval:      30 * time.Second,
	}
}

func (s *GatewayService) withGroupContext(ctx context.Context, group *Group) context.Context {
	if !IsGroupContextValid(group) {
		return ctx
	}
	if existing, ok := ctx.Value(ctxkey.Group).(*Group); ok && existing != nil && existing.ID == group.ID && IsGroupContextValid(existing) {
		return ctx
	}
	return context.WithValue(ctx, ctxkey.Group, group)
}

func (s *GatewayService) groupFromContext(ctx context.Context, groupID int64) *Group {
	if group, ok := ctx.Value(ctxkey.Group).(*Group); ok && IsGroupContextValid(group) && group.ID == groupID {
		return group
	}
	return nil
}

func (s *GatewayService) resolveGroupByID(ctx context.Context, groupID int64) (*Group, error) {
	if group := s.groupFromContext(ctx, groupID); group != nil {
		return group, nil
	}
	group, err := s.groupRepo.GetByIDLite(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("get group failed: %w", err)
	}
	return group, nil
}

func (s *GatewayService) ResolveGroupByID(ctx context.Context, groupID int64) (*Group, error) {
	return s.resolveGroupByID(ctx, groupID)
}

func (s *GatewayService) routingAccountIDsForRequest(ctx context.Context, groupID *int64, requestedModel string, platform string) []int64 {
	if groupID == nil || requestedModel == "" || platform != PlatformAnthropic {
		return nil
	}
	group, err := s.resolveGroupByID(ctx, *groupID)
	if err != nil || group == nil {
		if s.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] resolve group failed: group_id=%v model=%s platform=%s err=%v", derefGroupID(groupID), requestedModel, platform, err)
		}
		return nil
	}
	// Preserve existing behavior: model routing only applies to anthropic groups.
	if group.Platform != PlatformAnthropic {
		if s.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] skip: non-anthropic group platform: group_id=%d group_platform=%s model=%s", group.ID, group.Platform, requestedModel)
		}
		return nil
	}
	ids := group.GetRoutingAccountIDs(requestedModel)
	if s.debugModelRoutingEnabled() {
		logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] routing lookup: group_id=%d model=%s enabled=%v rules=%d matched_ids=%v",
			group.ID, requestedModel, group.ModelRoutingEnabled, len(group.ModelRouting), ids)
	}
	return ids
}

func (s *GatewayService) resolveGatewayGroup(ctx context.Context, groupID *int64) (*Group, *int64, error) {
	if groupID == nil {
		return nil, nil, nil
	}

	currentID := *groupID
	visited := map[int64]struct{}{}
	for {
		if _, seen := visited[currentID]; seen {
			return nil, nil, fmt.Errorf("fallback group cycle detected")
		}
		visited[currentID] = struct{}{}

		group, err := s.resolveGroupByID(ctx, currentID)
		if err != nil {
			return nil, nil, err
		}

		if !group.ClaudeCodeOnly || IsClaudeCodeClient(ctx) {
			return group, &currentID, nil
		}

		if group.FallbackGroupID == nil {
			return nil, nil, ErrClaudeCodeOnly
		}
		currentID = *group.FallbackGroupID
	}
}

// checkClaudeCodeRestriction 检查分组的 Claude Code 客户端限制
// 如果分组启用了 claude_code_only 且请求不是来自 Claude Code 客户端：
//   - 有降级分组：返回降级分组的 ID
//   - 无降级分组：返回 ErrClaudeCodeOnly 错误
func (s *GatewayService) checkClaudeCodeRestriction(ctx context.Context, groupID *int64) (*Group, *int64, error) {
	if groupID == nil {
		return nil, groupID, nil
	}

	// 强制平台模式不检查 Claude Code 限制
	if forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string); hasForcePlatform && forcePlatform != "" {
		return nil, groupID, nil
	}

	group, resolvedID, err := s.resolveGatewayGroup(ctx, groupID)
	if err != nil {
		return nil, nil, err
	}

	return group, resolvedID, nil
}

func (s *GatewayService) resolvePlatform(ctx context.Context, groupID *int64, group *Group) (string, bool, error) {
	forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string)
	if hasForcePlatform && forcePlatform != "" {
		return forcePlatform, true, nil
	}
	if group != nil {
		return group.Platform, false, nil
	}
	if groupID != nil {
		group, err := s.resolveGroupByID(ctx, *groupID)
		if err != nil {
			return "", false, err
		}
		return group.Platform, false, nil
	}
	return PlatformAnthropic, false, nil
}

func (s *GatewayService) listSchedulableAccounts(ctx context.Context, groupID *int64, platform string, hasForcePlatform bool) ([]Account, bool, error) {
	if s.schedulerSnapshot != nil {
		accounts, useMixed, err := s.schedulerSnapshot.ListSchedulableAccounts(ctx, groupID, platform, hasForcePlatform)
		if err == nil {
			slog.Debug("account_scheduling_list_snapshot",
				"group_id", derefGroupID(groupID),
				"platform", platform,
				"use_mixed", useMixed,
				"count", len(accounts))
			for _, acc := range accounts {
				slog.Debug("account_scheduling_account_detail",
					"account_id", acc.ID,
					"name", acc.Name,
					"platform", acc.Platform,
					"type", acc.Type,
					"status", acc.Status,
					"tls_fingerprint", acc.IsTLSFingerprintEnabled())
			}
		}
		return accounts, useMixed, err
	}
	useMixed := (platform == PlatformAnthropic || platform == PlatformGemini) && !hasForcePlatform
	if useMixed {
		platforms := []string{platform, PlatformAntigravity}
		var accounts []Account
		var err error
		if groupID != nil {
			accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatforms(ctx, *groupID, platforms)
		} else if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
			accounts, err = s.accountRepo.ListSchedulableByPlatforms(ctx, platforms)
		} else {
			accounts, err = s.accountRepo.ListSchedulableUngroupedByPlatforms(ctx, platforms)
		}
		if err != nil {
			slog.Debug("account_scheduling_list_failed",
				"group_id", derefGroupID(groupID),
				"platform", platform,
				"error", err)
			return nil, useMixed, err
		}
		filtered := make([]Account, 0, len(accounts))
		for _, acc := range accounts {
			if acc.Platform == PlatformAntigravity && !acc.IsMixedSchedulingEnabled() {
				continue
			}
			filtered = append(filtered, acc)
		}
		slog.Debug("account_scheduling_list_mixed",
			"group_id", derefGroupID(groupID),
			"platform", platform,
			"raw_count", len(accounts),
			"filtered_count", len(filtered))
		for _, acc := range filtered {
			slog.Debug("account_scheduling_account_detail",
				"account_id", acc.ID,
				"name", acc.Name,
				"platform", acc.Platform,
				"type", acc.Type,
				"status", acc.Status,
				"tls_fingerprint", acc.IsTLSFingerprintEnabled())
		}
		return filtered, useMixed, nil
	}

	var accounts []Account
	var err error
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, platform)
	} else if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, platform)
		// 分组内无账号则返回空列表，由上层处理错误，不再回退到全平台查询
	} else {
		accounts, err = s.accountRepo.ListSchedulableUngroupedByPlatform(ctx, platform)
	}
	if err != nil {
		slog.Debug("account_scheduling_list_failed",
			"group_id", derefGroupID(groupID),
			"platform", platform,
			"error", err)
		return nil, useMixed, err
	}
	slog.Debug("account_scheduling_list_single",
		"group_id", derefGroupID(groupID),
		"platform", platform,
		"count", len(accounts))
	for _, acc := range accounts {
		slog.Debug("account_scheduling_account_detail",
			"account_id", acc.ID,
			"name", acc.Name,
			"platform", acc.Platform,
			"type", acc.Type,
			"status", acc.Status,
			"tls_fingerprint", acc.IsTLSFingerprintEnabled())
	}
	return accounts, useMixed, nil
}

// IsSingleAntigravityAccountGroup 检查指定分组是否只有一个 antigravity 平台的可调度账号。
// 用于 Handler 层在首次请求时提前设置 SingleAccountRetry context，
// 避免单账号分组收到 503 时错误地设置模型限流标记导致后续请求连续快速失败。
func (s *GatewayService) IsSingleAntigravityAccountGroup(ctx context.Context, groupID *int64) bool {
	accounts, _, err := s.listSchedulableAccounts(ctx, groupID, PlatformAntigravity, true)
	if err != nil {
		return false
	}
	return len(accounts) == 1
}

func (s *GatewayService) isAccountAllowedForPlatform(account *Account, platform string, useMixed bool) bool {
	if account == nil {
		return false
	}
	if useMixed {
		if account.Platform == platform {
			return true
		}
		return account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled()
	}
	return account.Platform == platform
}

func (s *GatewayService) isAccountSchedulableForSelection(account *Account) bool {
	if account == nil {
		return false
	}
	return account.IsSchedulable()
}

func (s *GatewayService) isAccountSchedulableForModelSelection(ctx context.Context, account *Account, requestedModel string) bool {
	if account == nil {
		return false
	}
	return account.IsSchedulableForModelWithContext(ctx, requestedModel)
}

// isAccountInGroup checks if the account belongs to the specified group.
// When groupID is nil, returns true only for ungrouped accounts (no group assignments).
func (s *GatewayService) isAccountInGroup(account *Account, groupID *int64) bool {
	if account == nil {
		return false
	}
	if groupID == nil {
		// 无分组的 API Key 只能使用未分组的账号
		return len(account.AccountGroups) == 0
	}
	for _, ag := range account.AccountGroups {
		if ag.GroupID == *groupID {
			return true
		}
	}
	return false
}

func (s *GatewayService) tryAcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (*AcquireResult, error) {
	if s.concurrencyService == nil {
		return &AcquireResult{Acquired: true, ReleaseFunc: func() {}}, nil
	}
	return s.concurrencyService.AcquireAccountSlot(ctx, accountID, maxConcurrency)
}

type usageLogWindowStatsBatchProvider interface {
	GetAccountWindowStatsBatch(ctx context.Context, accountIDs []int64, startTime time.Time) (map[int64]*usagestats.AccountStats, error)
}

type windowCostPrefetchContextKeyType struct{}

var windowCostPrefetchContextKey = windowCostPrefetchContextKeyType{}

func windowCostFromPrefetchContext(ctx context.Context, accountID int64) (float64, bool) {
	if ctx == nil || accountID <= 0 {
		return 0, false
	}
	m, ok := ctx.Value(windowCostPrefetchContextKey).(map[int64]float64)
	if !ok || len(m) == 0 {
		return 0, false
	}
	v, exists := m[accountID]
	return v, exists
}

func (s *GatewayService) withWindowCostPrefetch(ctx context.Context, accounts []Account) context.Context {
	if ctx == nil || len(accounts) == 0 || s.sessionLimitCache == nil || s.usageLogRepo == nil {
		return ctx
	}

	accountByID := make(map[int64]*Account)
	accountIDs := make([]int64, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if account == nil || !account.IsAnthropicOAuthOrSetupToken() {
			continue
		}
		if account.GetWindowCostLimit() <= 0 {
			continue
		}
		accountByID[account.ID] = account
		accountIDs = append(accountIDs, account.ID)
	}
	if len(accountIDs) == 0 {
		return ctx
	}

	costs := make(map[int64]float64, len(accountIDs))
	cacheValues, err := s.sessionLimitCache.GetWindowCostBatch(ctx, accountIDs)
	if err == nil {
		for accountID, cost := range cacheValues {
			costs[accountID] = cost
		}
		windowCostPrefetchCacheHitTotal.Add(int64(len(cacheValues)))
	} else {
		windowCostPrefetchErrorTotal.Add(1)
		logger.LegacyPrintf("service.gateway", "window_cost batch cache read failed: %v", err)
	}
	cacheMissCount := len(accountIDs) - len(costs)
	if cacheMissCount < 0 {
		cacheMissCount = 0
	}
	windowCostPrefetchCacheMissTotal.Add(int64(cacheMissCount))

	missingByStart := make(map[int64][]int64)
	startTimes := make(map[int64]time.Time)
	for _, accountID := range accountIDs {
		if _, ok := costs[accountID]; ok {
			continue
		}
		account := accountByID[accountID]
		if account == nil {
			continue
		}
		startTime := account.GetCurrentWindowStartTime()
		startKey := startTime.Unix()
		missingByStart[startKey] = append(missingByStart[startKey], accountID)
		startTimes[startKey] = startTime
	}
	if len(missingByStart) == 0 {
		return context.WithValue(ctx, windowCostPrefetchContextKey, costs)
	}

	batchReader, hasBatch := s.usageLogRepo.(usageLogWindowStatsBatchProvider)
	for startKey, ids := range missingByStart {
		startTime := startTimes[startKey]

		if hasBatch {
			windowCostPrefetchBatchSQLTotal.Add(1)
			queryStart := time.Now()
			statsByAccount, err := batchReader.GetAccountWindowStatsBatch(ctx, ids, startTime)
			if err == nil {
				slog.Debug("window_cost_batch_query_ok",
					"accounts", len(ids),
					"window_start", startTime.Format(time.RFC3339),
					"duration_ms", time.Since(queryStart).Milliseconds())
				for _, accountID := range ids {
					stats := statsByAccount[accountID]
					cost := 0.0
					if stats != nil {
						cost = stats.StandardCost
					}
					costs[accountID] = cost
					_ = s.sessionLimitCache.SetWindowCost(ctx, accountID, cost)
				}
				continue
			}
			windowCostPrefetchErrorTotal.Add(1)
			logger.LegacyPrintf("service.gateway", "window_cost batch db query failed: start=%s err=%v", startTime.Format(time.RFC3339), err)
		}

		// 回退路径：缺少批量仓储能力或批量查询失败时，按账号单查（失败开放）。
		windowCostPrefetchFallbackTotal.Add(int64(len(ids)))
		for _, accountID := range ids {
			stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, accountID, startTime)
			if err != nil {
				windowCostPrefetchErrorTotal.Add(1)
				continue
			}
			cost := stats.StandardCost
			costs[accountID] = cost
			_ = s.sessionLimitCache.SetWindowCost(ctx, accountID, cost)
		}
	}

	return context.WithValue(ctx, windowCostPrefetchContextKey, costs)
}

// isAccountSchedulableForQuota 检查账号是否在配额限制内
// 适用于配置了 quota_limit 的 apikey 和 bedrock 类型账号
func (s *GatewayService) isAccountSchedulableForQuota(account *Account) bool {
	if !account.IsAPIKeyOrBedrock() {
		return true
	}
	return !account.IsQuotaExceeded()
}

// isAccountSchedulableForWindowCost 检查账号是否可根据窗口费用进行调度
// 仅适用于 Anthropic OAuth/SetupToken 账号
// 返回 true 表示可调度，false 表示不可调度
func (s *GatewayService) isAccountSchedulableForWindowCost(ctx context.Context, account *Account, isSticky bool) bool {
	// 只检查 Anthropic OAuth/SetupToken 账号
	if !account.IsAnthropicOAuthOrSetupToken() {
		return true
	}

	limit := account.GetWindowCostLimit()
	if limit <= 0 {
		return true // 未启用窗口费用限制
	}

	// 尝试从缓存获取窗口费用
	var currentCost float64
	if cost, ok := windowCostFromPrefetchContext(ctx, account.ID); ok {
		currentCost = cost
		goto checkSchedulability
	}
	if s.sessionLimitCache != nil {
		if cost, hit, err := s.sessionLimitCache.GetWindowCost(ctx, account.ID); err == nil && hit {
			currentCost = cost
			goto checkSchedulability
		}
	}

	// 缓存未命中，从数据库查询
	{
		// 使用统一的窗口开始时间计算逻辑（考虑窗口过期情况）
		startTime := account.GetCurrentWindowStartTime()

		stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, account.ID, startTime)
		if err != nil {
			// 失败开放：查询失败时允许调度
			return true
		}

		// 使用标准费用（不含账号倍率）
		currentCost = stats.StandardCost

		// 设置缓存（忽略错误）
		if s.sessionLimitCache != nil {
			_ = s.sessionLimitCache.SetWindowCost(ctx, account.ID, currentCost)
		}
	}

checkSchedulability:
	schedulability := account.CheckWindowCostSchedulability(currentCost)

	switch schedulability {
	case WindowCostSchedulable:
		return true
	case WindowCostStickyOnly:
		return isSticky
	case WindowCostNotSchedulable:
		return false
	}
	return true
}

// rpmPrefetchContextKey is the context key for prefetched RPM counts.
type rpmPrefetchContextKeyType struct{}

var rpmPrefetchContextKey = rpmPrefetchContextKeyType{}

func rpmFromPrefetchContext(ctx context.Context, accountID int64) (int, bool) {
	if v, ok := ctx.Value(rpmPrefetchContextKey).(map[int64]int); ok {
		count, found := v[accountID]
		return count, found
	}
	return 0, false
}

// withRPMPrefetch 批量预取所有候选账号的 RPM 计数
func (s *GatewayService) withRPMPrefetch(ctx context.Context, accounts []Account) context.Context {
	if s.rpmCache == nil {
		return ctx
	}

	var ids []int64
	for i := range accounts {
		if accounts[i].IsAnthropicOAuthOrSetupToken() && accounts[i].GetBaseRPM() > 0 {
			ids = append(ids, accounts[i].ID)
		}
	}
	if len(ids) == 0 {
		return ctx
	}

	counts, err := s.rpmCache.GetRPMBatch(ctx, ids)
	if err != nil {
		return ctx // 失败开放
	}
	return context.WithValue(ctx, rpmPrefetchContextKey, counts)
}

// isAccountSchedulableForRPM 检查账号是否可根据 RPM 进行调度
// 仅适用于 Anthropic OAuth/SetupToken 账号
func (s *GatewayService) isAccountSchedulableForRPM(ctx context.Context, account *Account, isSticky bool) bool {
	if !account.IsAnthropicOAuthOrSetupToken() {
		return true
	}
	baseRPM := account.GetBaseRPM()
	if baseRPM <= 0 {
		return true
	}

	// 尝试从预取缓存获取
	var currentRPM int
	if count, ok := rpmFromPrefetchContext(ctx, account.ID); ok {
		currentRPM = count
	} else if s.rpmCache != nil {
		if count, err := s.rpmCache.GetRPM(ctx, account.ID); err == nil {
			currentRPM = count
		}
		// 失败开放：GetRPM 错误时允许调度
	}

	schedulability := account.CheckRPMSchedulability(currentRPM)
	switch schedulability {
	case WindowCostSchedulable:
		return true
	case WindowCostStickyOnly:
		return isSticky
	case WindowCostNotSchedulable:
		return false
	}
	return true
}

// IncrementAccountRPM increments the RPM counter for the given account.
// 已知 TOCTOU 竞态：调度时读取 RPM 计数与此处递增之间存在时间窗口，
// 高并发下可能短暂超出 RPM 限制。这是与 WindowCost 一致的 soft-limit
// 设计权衡——可接受的少量超额优于加锁带来的延迟和复杂度。
func (s *GatewayService) IncrementAccountRPM(ctx context.Context, accountID int64) error {
	if s.rpmCache == nil {
		return nil
	}
	_, err := s.rpmCache.IncrementRPM(ctx, accountID)
	return err
}

// checkAndRegisterSession 检查并注册会话，用于会话数量限制
// 仅适用于 Anthropic OAuth/SetupToken 账号
// sessionID: 会话标识符（使用粘性会话的 hash）
// 返回 true 表示允许（在限制内或会话已存在），false 表示拒绝（超出限制且是新会话）
func (s *GatewayService) checkAndRegisterSession(ctx context.Context, account *Account, sessionID string) bool {
	// 只检查 Anthropic OAuth/SetupToken 账号
	if !account.IsAnthropicOAuthOrSetupToken() {
		return true
	}

	maxSessions := account.GetMaxSessions()
	if maxSessions <= 0 || sessionID == "" {
		return true // 未启用会话限制或无会话ID
	}

	if s.sessionLimitCache == nil {
		return true // 缓存不可用时允许通过
	}

	idleTimeout := time.Duration(account.GetSessionIdleTimeoutMinutes()) * time.Minute

	allowed, err := s.sessionLimitCache.RegisterSession(ctx, account.ID, sessionID, maxSessions, idleTimeout)
	if err != nil {
		// 失败开放：缓存错误时允许通过
		return true
	}
	return allowed
}

func (s *GatewayService) getSchedulableAccount(ctx context.Context, accountID int64) (*Account, error) {
	if s.schedulerSnapshot != nil {
		return s.schedulerSnapshot.GetAccount(ctx, accountID)
	}
	return s.accountRepo.GetByID(ctx, accountID)
}

func (s *GatewayService) hydrateSelectedAccount(ctx context.Context, account *Account) (*Account, error) {
	if account == nil || s.schedulerSnapshot == nil {
		return account, nil
	}
	hydrated, err := s.schedulerSnapshot.GetAccount(ctx, account.ID)
	if err != nil {
		return nil, err
	}
	if hydrated == nil {
		return nil, fmt.Errorf("selected gateway account %d not found during hydration", account.ID)
	}
	return hydrated, nil
}

func (s *GatewayService) newSelectionResult(ctx context.Context, account *Account, acquired bool, release func(), waitPlan *AccountWaitPlan) (*AccountSelectionResult, error) {
	hydrated, err := s.hydrateSelectedAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	return &AccountSelectionResult{
		Account:     hydrated,
		Acquired:    acquired,
		ReleaseFunc: release,
		WaitPlan:    waitPlan,
	}, nil
}

// filterByMinPriority 过滤出优先级最小的账号集合
func filterByMinPriority(accounts []accountWithLoad) []accountWithLoad {
	if len(accounts) == 0 {
		return accounts
	}
	minPriority := accounts[0].account.Priority
	for _, acc := range accounts[1:] {
		if acc.account.Priority < minPriority {
			minPriority = acc.account.Priority
		}
	}
	result := make([]accountWithLoad, 0, len(accounts))
	for _, acc := range accounts {
		if acc.account.Priority == minPriority {
			result = append(result, acc)
		}
	}
	return result
}

// filterByMinLoadRate 过滤出负载率最低的账号集合
func filterByMinLoadRate(accounts []accountWithLoad) []accountWithLoad {
	if len(accounts) == 0 {
		return accounts
	}
	minLoadRate := accounts[0].loadInfo.LoadRate
	for _, acc := range accounts[1:] {
		if acc.loadInfo.LoadRate < minLoadRate {
			minLoadRate = acc.loadInfo.LoadRate
		}
	}
	result := make([]accountWithLoad, 0, len(accounts))
	for _, acc := range accounts {
		if acc.loadInfo.LoadRate == minLoadRate {
			result = append(result, acc)
		}
	}
	return result
}

// selectByLRU 从集合中选择最久未用的账号
// 如果有多个账号具有相同的最小 LastUsedAt，则随机选择一个
func selectByLRU(accounts []accountWithLoad, preferOAuth bool) *accountWithLoad {
	if len(accounts) == 0 {
		return nil
	}
	if len(accounts) == 1 {
		return &accounts[0]
	}

	// 1. 找到最小的 LastUsedAt（nil 被视为最小）
	var minTime *time.Time
	hasNil := false
	for _, acc := range accounts {
		if acc.account.LastUsedAt == nil {
			hasNil = true
			break
		}
		if minTime == nil || acc.account.LastUsedAt.Before(*minTime) {
			minTime = acc.account.LastUsedAt
		}
	}

	// 2. 收集所有具有最小 LastUsedAt 的账号索引
	var candidateIdxs []int
	for i, acc := range accounts {
		if hasNil {
			if acc.account.LastUsedAt == nil {
				candidateIdxs = append(candidateIdxs, i)
			}
		} else {
			if acc.account.LastUsedAt != nil && acc.account.LastUsedAt.Equal(*minTime) {
				candidateIdxs = append(candidateIdxs, i)
			}
		}
	}

	// 3. 如果只有一个候选，直接返回
	if len(candidateIdxs) == 1 {
		return &accounts[candidateIdxs[0]]
	}

	// 4. 如果有多个候选且 preferOAuth，优先选择 OAuth 类型
	if preferOAuth {
		var oauthIdxs []int
		for _, idx := range candidateIdxs {
			if accounts[idx].account.Type == AccountTypeOAuth {
				oauthIdxs = append(oauthIdxs, idx)
			}
		}
		if len(oauthIdxs) > 0 {
			candidateIdxs = oauthIdxs
		}
	}

	// 5. 随机选择一个
	selectedIdx := candidateIdxs[mathrand.Intn(len(candidateIdxs))]
	return &accounts[selectedIdx]
}

func sortAccountsByPriorityAndLastUsed(accounts []*Account, preferOAuth bool) {
	sort.SliceStable(accounts, func(i, j int) bool {
		a, b := accounts[i], accounts[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		switch {
		case a.LastUsedAt == nil && b.LastUsedAt != nil:
			return true
		case a.LastUsedAt != nil && b.LastUsedAt == nil:
			return false
		case a.LastUsedAt == nil && b.LastUsedAt == nil:
			if preferOAuth && a.Type != b.Type {
				return a.Type == AccountTypeOAuth
			}
			return false
		default:
			return a.LastUsedAt.Before(*b.LastUsedAt)
		}
	})
	shuffleWithinPriorityAndLastUsed(accounts, preferOAuth)
}

// shuffleWithinSortGroups 对排序后的 accountWithLoad 切片，按 (Priority, LoadRate, LastUsedAt) 分组后组内随机打乱。
// 防止并发请求读取同一快照时，确定性排序导致所有请求命中相同账号。
func shuffleWithinSortGroups(accounts []accountWithLoad) {
	if len(accounts) <= 1 {
		return
	}
	i := 0
	for i < len(accounts) {
		j := i + 1
		for j < len(accounts) && sameAccountWithLoadGroup(accounts[i], accounts[j]) {
			j++
		}
		if j-i > 1 {
			mathrand.Shuffle(j-i, func(a, b int) {
				accounts[i+a], accounts[i+b] = accounts[i+b], accounts[i+a]
			})
		}
		i = j
	}
}

// sameAccountWithLoadGroup 判断两个 accountWithLoad 是否属于同一排序组
func sameAccountWithLoadGroup(a, b accountWithLoad) bool {
	if a.account.Priority != b.account.Priority {
		return false
	}
	if a.loadInfo.LoadRate != b.loadInfo.LoadRate {
		return false
	}
	return sameLastUsedAt(a.account.LastUsedAt, b.account.LastUsedAt)
}

// shuffleWithinPriorityAndLastUsed 对排序后的 []*Account 切片，按 (Priority, LastUsedAt) 分组后组内随机打乱。
//
// 注意：当 preferOAuth=true 时，需要保证 OAuth 账号在同组内仍然优先，否则会把排序时的偏好打散掉。
// 因此这里采用"组内分区 + 分区内 shuffle"的方式：
// - 先把同组账号按 (OAuth / 非 OAuth) 拆成两段，保持 OAuth 段在前；
// - 再分别在各段内随机打散，避免热点。
func shuffleWithinPriorityAndLastUsed(accounts []*Account, preferOAuth bool) {
	if len(accounts) <= 1 {
		return
	}
	i := 0
	for i < len(accounts) {
		j := i + 1
		for j < len(accounts) && sameAccountGroup(accounts[i], accounts[j]) {
			j++
		}
		if j-i > 1 {
			if preferOAuth {
				oauth := make([]*Account, 0, j-i)
				others := make([]*Account, 0, j-i)
				for _, acc := range accounts[i:j] {
					if acc.Type == AccountTypeOAuth {
						oauth = append(oauth, acc)
					} else {
						others = append(others, acc)
					}
				}
				if len(oauth) > 1 {
					mathrand.Shuffle(len(oauth), func(a, b int) { oauth[a], oauth[b] = oauth[b], oauth[a] })
				}
				if len(others) > 1 {
					mathrand.Shuffle(len(others), func(a, b int) { others[a], others[b] = others[b], others[a] })
				}
				copy(accounts[i:], oauth)
				copy(accounts[i+len(oauth):], others)
			} else {
				mathrand.Shuffle(j-i, func(a, b int) {
					accounts[i+a], accounts[i+b] = accounts[i+b], accounts[i+a]
				})
			}
		}
		i = j
	}
}

// sameAccountGroup 判断两个 Account 是否属于同一排序组（Priority + LastUsedAt）
func sameAccountGroup(a, b *Account) bool {
	if a.Priority != b.Priority {
		return false
	}
	return sameLastUsedAt(a.LastUsedAt, b.LastUsedAt)
}

// sameLastUsedAt 判断两个 LastUsedAt 是否相同（精度到秒）
func sameLastUsedAt(a, b *time.Time) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.Unix() == b.Unix()
	}
}

// sortCandidatesForFallback 根据配置选择排序策略
// mode: "last_used"(按最后使用时间) 或 "random"(随机)
func (s *GatewayService) sortCandidatesForFallback(accounts []*Account, preferOAuth bool, mode string) {
	if mode == "random" {
		// 先按优先级排序，然后在同优先级内随机打乱
		sortAccountsByPriorityOnly(accounts, preferOAuth)
		shuffleWithinPriority(accounts)
	} else {
		// 默认按最后使用时间排序
		sortAccountsByPriorityAndLastUsed(accounts, preferOAuth)
	}
}

// sortAccountsByPriorityOnly 仅按优先级排序
func sortAccountsByPriorityOnly(accounts []*Account, preferOAuth bool) {
	sort.SliceStable(accounts, func(i, j int) bool {
		a, b := accounts[i], accounts[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if preferOAuth && a.Type != b.Type {
			return a.Type == AccountTypeOAuth
		}
		return false
	})
}

// shuffleWithinPriority 在同优先级内随机打乱顺序
func shuffleWithinPriority(accounts []*Account) {
	if len(accounts) <= 1 {
		return
	}
	r := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	start := 0
	for start < len(accounts) {
		priority := accounts[start].Priority
		end := start + 1
		for end < len(accounts) && accounts[end].Priority == priority {
			end++
		}
		// 对 [start, end) 范围内的账户随机打乱
		if end-start > 1 {
			r.Shuffle(end-start, func(i, j int) {
				accounts[start+i], accounts[start+j] = accounts[start+j], accounts[start+i]
			})
		}
		start = end
	}
}

// selectAccountForModelWithPlatform 选择单平台账户（完全隔离）
func (s *GatewayService) selectAccountForModelWithPlatform(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, platform string) (*Account, error) {
	preferOAuth := platform == PlatformGemini
	routingAccountIDs := s.routingAccountIDsForRequest(ctx, groupID, requestedModel, platform)

	// require_privacy_set: 获取分组信息
	var schedGroup *Group
	if groupID != nil && s.groupRepo != nil {
		schedGroup, _ = s.groupRepo.GetByID(ctx, *groupID)
	}

	var accounts []Account
	accountsLoaded := false

	// ============ Model Routing (legacy path): apply before sticky session ============
	// When load-awareness is disabled (e.g. concurrency service not configured), we still honor model routing
	// so switching model can switch upstream account within the same sticky session.
	if len(routingAccountIDs) > 0 {
		if s.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] legacy routed begin: group_id=%v model=%s platform=%s session=%s routed_ids=%v",
				derefGroupID(groupID), requestedModel, platform, shortSessionHash(sessionHash), routingAccountIDs)
		}
		// 1) Sticky session only applies if the bound account is within the routing set.
		if sessionHash != "" && s.cache != nil {
			accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
			if err == nil && accountID > 0 && containsInt64(routingAccountIDs, accountID) {
				if _, excluded := excludedIDs[accountID]; !excluded {
					account, err := s.getSchedulableAccount(ctx, accountID)
					// 检查账号分组归属和平台匹配（确保粘性会话不会跨分组或跨平台）
					if err == nil {
						clearSticky := shouldClearStickySession(account, requestedModel)
						if clearSticky {
							_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
						}
						if !clearSticky && s.isAccountInGroup(account, groupID) && account.Platform == platform && (requestedModel == "" || s.isModelSupportedByAccountWithContext(ctx, account, requestedModel)) && s.isAccountSchedulableForModelSelection(ctx, account, requestedModel) && s.isAccountSchedulableForQuota(account) && s.isAccountSchedulableForWindowCost(ctx, account, true) && s.isAccountSchedulableForRPM(ctx, account, true) && !s.isStickyAccountUpstreamRestricted(ctx, groupID, account, requestedModel) {
							if s.debugModelRoutingEnabled() {
								logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] legacy routed sticky hit: group_id=%v model=%s session=%s account=%d", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), accountID)
							}
							return account, nil
						}
					}
				}
			}
		}

		// 2) Select an account from the routed candidates.
		forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string)
		if hasForcePlatform && forcePlatform == "" {
			hasForcePlatform = false
		}
		var err error
		accounts, _, err = s.listSchedulableAccounts(ctx, groupID, platform, hasForcePlatform)
		if err != nil {
			return nil, fmt.Errorf("query accounts failed: %w", err)
		}
		accountsLoaded = true

		// 提前预取窗口费用+RPM 计数，确保 routing 段内的调度检查调用能命中缓存
		ctx = s.withWindowCostPrefetch(ctx, accounts)
		ctx = s.withRPMPrefetch(ctx, accounts)

		routingSet := make(map[int64]struct{}, len(routingAccountIDs))
		for _, id := range routingAccountIDs {
			if id > 0 {
				routingSet[id] = struct{}{}
			}
		}

		var selected *Account
		for i := range accounts {
			acc := &accounts[i]
			if _, ok := routingSet[acc.ID]; !ok {
				continue
			}
			if _, excluded := excludedIDs[acc.ID]; excluded {
				continue
			}
			// Scheduler snapshots can be temporarily stale; re-check schedulability here to
			// avoid selecting accounts that were recently rate-limited/overloaded.
			if !s.isAccountSchedulableForSelection(acc) {
				continue
			}
			// require_privacy_set: 跳过 privacy 未设置的账号并标记异常
			if schedGroup != nil && schedGroup.RequirePrivacySet && !acc.IsPrivacySet() {
				_ = s.accountRepo.SetError(ctx, acc.ID,
					fmt.Sprintf("Privacy not set, required by group [%s]", schedGroup.Name))
				continue
			}
			if requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, acc, requestedModel) {
				continue
			}
			if !s.isAccountSchedulableForModelSelection(ctx, acc, requestedModel) {
				continue
			}
			if !s.isAccountSchedulableForQuota(acc) {
				continue
			}
			if !s.isAccountSchedulableForWindowCost(ctx, acc, false) {
				continue
			}
			if !s.isAccountSchedulableForRPM(ctx, acc, false) {
				continue
			}
			if selected == nil {
				selected = acc
				continue
			}
			if acc.Priority < selected.Priority {
				selected = acc
			} else if acc.Priority == selected.Priority {
				switch {
				case acc.LastUsedAt == nil && selected.LastUsedAt != nil:
					selected = acc
				case acc.LastUsedAt != nil && selected.LastUsedAt == nil:
					// keep selected (never used is preferred)
				case acc.LastUsedAt == nil && selected.LastUsedAt == nil:
					if preferOAuth && acc.Type != selected.Type && acc.Type == AccountTypeOAuth {
						selected = acc
					}
				default:
					if acc.LastUsedAt.Before(*selected.LastUsedAt) {
						selected = acc
					}
				}
			}
		}

		if selected != nil {
			if sessionHash != "" && s.cache != nil {
				if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, selected.ID, stickySessionTTL); err != nil {
					logger.LegacyPrintf("service.gateway", "set session account failed: session=%s account_id=%d err=%v", sessionHash, selected.ID, err)
				}
			}
			if s.debugModelRoutingEnabled() {
				logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] legacy routed select: group_id=%v model=%s session=%s account=%d", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), selected.ID)
			}
			return selected, nil
		}
		logger.LegacyPrintf("service.gateway", "[ModelRouting] No routed accounts available for model=%s, falling back to normal selection", requestedModel)
	}

	// 1. 查询粘性会话
	if sessionHash != "" && s.cache != nil {
		accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
		if err == nil && accountID > 0 {
			if _, excluded := excludedIDs[accountID]; !excluded {
				account, err := s.getSchedulableAccount(ctx, accountID)
				// 检查账号分组归属和平台匹配（确保粘性会话不会跨分组或跨平台）
				if err == nil {
					clearSticky := shouldClearStickySession(account, requestedModel)
					if clearSticky {
						_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
					}
					if !clearSticky && s.isAccountInGroup(account, groupID) && account.Platform == platform && (requestedModel == "" || s.isModelSupportedByAccountWithContext(ctx, account, requestedModel)) && s.isAccountSchedulableForModelSelection(ctx, account, requestedModel) && s.isAccountSchedulableForQuota(account) && s.isAccountSchedulableForWindowCost(ctx, account, true) && s.isAccountSchedulableForRPM(ctx, account, true) {
						return account, nil
					}
				}
			}
		}
	}

	// 2. 获取可调度账号列表（单平台）
	if !accountsLoaded {
		forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string)
		if hasForcePlatform && forcePlatform == "" {
			hasForcePlatform = false
		}
		var err error
		accounts, _, err = s.listSchedulableAccounts(ctx, groupID, platform, hasForcePlatform)
		if err != nil {
			return nil, fmt.Errorf("query accounts failed: %w", err)
		}
	}

	// 批量预取窗口费用+RPM 计数，避免逐个账号查询（N+1）
	ctx = s.withWindowCostPrefetch(ctx, accounts)
	ctx = s.withRPMPrefetch(ctx, accounts)

	// 3. 按优先级+最久未用选择（考虑模型支持）
	// needsUpstreamCheck 仅在主选择循环中使用；粘性会话命中时跳过此检查，
	// 因为粘性会话优先保持连接一致性，且 upstream 计费基准极少使用。
	needsUpstreamCheck := s.needsUpstreamChannelRestrictionCheck(ctx, groupID)
	var selected *Account
	for i := range accounts {
		acc := &accounts[i]
		if _, excluded := excludedIDs[acc.ID]; excluded {
			continue
		}
		// Scheduler snapshots can be temporarily stale; re-check schedulability here to
		// avoid selecting accounts that were recently rate-limited/overloaded.
		if !s.isAccountSchedulableForSelection(acc) {
			continue
		}
		// require_privacy_set: 跳过 privacy 未设置的账号并标记异常
		if schedGroup != nil && schedGroup.RequirePrivacySet && !acc.IsPrivacySet() {
			_ = s.accountRepo.SetError(ctx, acc.ID,
				fmt.Sprintf("Privacy not set, required by group [%s]", schedGroup.Name))
			continue
		}
		if requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, acc, requestedModel) {
			continue
		}
		if needsUpstreamCheck && s.isUpstreamModelRestrictedByChannel(ctx, *groupID, acc, requestedModel) {
			continue
		}
		if !s.isAccountSchedulableForModelSelection(ctx, acc, requestedModel) {
			continue
		}
		if !s.isAccountSchedulableForQuota(acc) {
			continue
		}
		if !s.isAccountSchedulableForWindowCost(ctx, acc, false) {
			continue
		}
		if !s.isAccountSchedulableForRPM(ctx, acc, false) {
			continue
		}
		if selected == nil {
			selected = acc
			continue
		}
		if acc.Priority < selected.Priority {
			selected = acc
		} else if acc.Priority == selected.Priority {
			switch {
			case acc.LastUsedAt == nil && selected.LastUsedAt != nil:
				selected = acc
			case acc.LastUsedAt != nil && selected.LastUsedAt == nil:
				// keep selected (never used is preferred)
			case acc.LastUsedAt == nil && selected.LastUsedAt == nil:
				if preferOAuth && acc.Type != selected.Type && acc.Type == AccountTypeOAuth {
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
		stats := s.logDetailedSelectionFailure(ctx, groupID, sessionHash, requestedModel, platform, accounts, excludedIDs, false)
		if requestedModel != "" {
			return nil, fmt.Errorf("%w supporting model: %s (%s)", ErrNoAvailableAccounts, requestedModel, summarizeSelectionFailureStats(stats))
		}
		return nil, ErrNoAvailableAccounts
	}

	// 4. 建立粘性绑定
	if sessionHash != "" && s.cache != nil {
		if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, selected.ID, stickySessionTTL); err != nil {
			logger.LegacyPrintf("service.gateway", "set session account failed: session=%s account_id=%d err=%v", sessionHash, selected.ID, err)
		}
	}

	return selected, nil
}

// selectAccountWithMixedScheduling 选择账户（支持混合调度）
// 查询原生平台账户 + 启用 mixed_scheduling 的 antigravity 账户
func (s *GatewayService) selectAccountWithMixedScheduling(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, nativePlatform string) (*Account, error) {
	preferOAuth := nativePlatform == PlatformGemini
	routingAccountIDs := s.routingAccountIDsForRequest(ctx, groupID, requestedModel, nativePlatform)

	// require_privacy_set: 获取分组信息
	var schedGroup *Group
	if groupID != nil && s.groupRepo != nil {
		schedGroup, _ = s.groupRepo.GetByID(ctx, *groupID)
	}

	var accounts []Account
	accountsLoaded := false

	// ============ Model Routing (legacy path): apply before sticky session ============
	if len(routingAccountIDs) > 0 {
		if s.debugModelRoutingEnabled() {
			logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] legacy mixed routed begin: group_id=%v model=%s platform=%s session=%s routed_ids=%v",
				derefGroupID(groupID), requestedModel, nativePlatform, shortSessionHash(sessionHash), routingAccountIDs)
		}
		// 1) Sticky session only applies if the bound account is within the routing set.
		if sessionHash != "" && s.cache != nil {
			accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
			if err == nil && accountID > 0 && containsInt64(routingAccountIDs, accountID) {
				if _, excluded := excludedIDs[accountID]; !excluded {
					account, err := s.getSchedulableAccount(ctx, accountID)
					// 检查账号分组归属和有效性：原生平台直接匹配，antigravity 需要启用混合调度
					if err == nil {
						clearSticky := shouldClearStickySession(account, requestedModel)
						if clearSticky {
							_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
						}
						if !clearSticky && s.isAccountInGroup(account, groupID) && (requestedModel == "" || s.isModelSupportedByAccountWithContext(ctx, account, requestedModel)) && s.isAccountSchedulableForModelSelection(ctx, account, requestedModel) && s.isAccountSchedulableForQuota(account) && s.isAccountSchedulableForWindowCost(ctx, account, true) && s.isAccountSchedulableForRPM(ctx, account, true) {
							if account.Platform == nativePlatform || (account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled()) {
								if s.debugModelRoutingEnabled() {
									logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] legacy mixed routed sticky hit: group_id=%v model=%s session=%s account=%d", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), accountID)
								}
								return account, nil
							}
						}
					}
				}
			}
		}

		// 2) Select an account from the routed candidates.
		var err error
		accounts, _, err = s.listSchedulableAccounts(ctx, groupID, nativePlatform, false)
		if err != nil {
			return nil, fmt.Errorf("query accounts failed: %w", err)
		}
		accountsLoaded = true

		// 提前预取窗口费用+RPM 计数，确保 routing 段内的调度检查调用能命中缓存
		ctx = s.withWindowCostPrefetch(ctx, accounts)
		ctx = s.withRPMPrefetch(ctx, accounts)

		routingSet := make(map[int64]struct{}, len(routingAccountIDs))
		for _, id := range routingAccountIDs {
			if id > 0 {
				routingSet[id] = struct{}{}
			}
		}

		var selected *Account
		for i := range accounts {
			acc := &accounts[i]
			if _, ok := routingSet[acc.ID]; !ok {
				continue
			}
			if _, excluded := excludedIDs[acc.ID]; excluded {
				continue
			}
			// Scheduler snapshots can be temporarily stale; re-check schedulability here to
			// avoid selecting accounts that were recently rate-limited/overloaded.
			if !s.isAccountSchedulableForSelection(acc) {
				continue
			}
			// require_privacy_set: 跳过 privacy 未设置的账号并标记异常
			if schedGroup != nil && schedGroup.RequirePrivacySet && !acc.IsPrivacySet() {
				_ = s.accountRepo.SetError(ctx, acc.ID,
					fmt.Sprintf("Privacy not set, required by group [%s]", schedGroup.Name))
				continue
			}
			// 过滤：原生平台直接通过，antigravity 需要启用混合调度
			if acc.Platform == PlatformAntigravity && !acc.IsMixedSchedulingEnabled() {
				continue
			}
			if requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, acc, requestedModel) {
				continue
			}
			if !s.isAccountSchedulableForModelSelection(ctx, acc, requestedModel) {
				continue
			}
			if !s.isAccountSchedulableForQuota(acc) {
				continue
			}
			if !s.isAccountSchedulableForWindowCost(ctx, acc, false) {
				continue
			}
			if !s.isAccountSchedulableForRPM(ctx, acc, false) {
				continue
			}
			if selected == nil {
				selected = acc
				continue
			}
			if acc.Priority < selected.Priority {
				selected = acc
			} else if acc.Priority == selected.Priority {
				switch {
				case acc.LastUsedAt == nil && selected.LastUsedAt != nil:
					selected = acc
				case acc.LastUsedAt != nil && selected.LastUsedAt == nil:
					// keep selected (never used is preferred)
				case acc.LastUsedAt == nil && selected.LastUsedAt == nil:
					if preferOAuth && acc.Platform == PlatformGemini && selected.Platform == PlatformGemini && acc.Type != selected.Type && acc.Type == AccountTypeOAuth {
						selected = acc
					}
				default:
					if acc.LastUsedAt.Before(*selected.LastUsedAt) {
						selected = acc
					}
				}
			}
		}

		if selected != nil {
			if sessionHash != "" && s.cache != nil {
				if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, selected.ID, stickySessionTTL); err != nil {
					logger.LegacyPrintf("service.gateway", "set session account failed: session=%s account_id=%d err=%v", sessionHash, selected.ID, err)
				}
			}
			if s.debugModelRoutingEnabled() {
				logger.LegacyPrintf("service.gateway", "[ModelRoutingDebug] legacy mixed routed select: group_id=%v model=%s session=%s account=%d", derefGroupID(groupID), requestedModel, shortSessionHash(sessionHash), selected.ID)
			}
			return selected, nil
		}
		logger.LegacyPrintf("service.gateway", "[ModelRouting] No routed accounts available for model=%s, falling back to normal selection", requestedModel)
	}

	// 1. 查询粘性会话
	if sessionHash != "" && s.cache != nil {
		accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
		if err == nil && accountID > 0 {
			if _, excluded := excludedIDs[accountID]; !excluded {
				account, err := s.getSchedulableAccount(ctx, accountID)
				// 检查账号分组归属和有效性：原生平台直接匹配，antigravity 需要启用混合调度
				if err == nil {
					clearSticky := shouldClearStickySession(account, requestedModel)
					if clearSticky {
						_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
					}
					if !clearSticky && s.isAccountInGroup(account, groupID) && (requestedModel == "" || s.isModelSupportedByAccountWithContext(ctx, account, requestedModel)) && s.isAccountSchedulableForModelSelection(ctx, account, requestedModel) && s.isAccountSchedulableForQuota(account) && s.isAccountSchedulableForWindowCost(ctx, account, true) && s.isAccountSchedulableForRPM(ctx, account, true) && !s.isStickyAccountUpstreamRestricted(ctx, groupID, account, requestedModel) {
						if account.Platform == nativePlatform || (account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled()) {
							return account, nil
						}
					}
				}
			}
		}
	}

	// 2. 获取可调度账号列表
	if !accountsLoaded {
		var err error
		accounts, _, err = s.listSchedulableAccounts(ctx, groupID, nativePlatform, false)
		if err != nil {
			return nil, fmt.Errorf("query accounts failed: %w", err)
		}
	}

	// 批量预取窗口费用+RPM 计数，避免逐个账号查询（N+1）
	ctx = s.withWindowCostPrefetch(ctx, accounts)
	ctx = s.withRPMPrefetch(ctx, accounts)

	// 3. 按优先级+最久未用选择（考虑模型支持和混合调度）
	// needsUpstreamCheck 仅在主选择循环中使用；粘性会话命中时跳过此检查。
	needsUpstreamCheck := s.needsUpstreamChannelRestrictionCheck(ctx, groupID)
	var selected *Account
	for i := range accounts {
		acc := &accounts[i]
		if _, excluded := excludedIDs[acc.ID]; excluded {
			continue
		}
		// Scheduler snapshots can be temporarily stale; re-check schedulability here to
		// avoid selecting accounts that were recently rate-limited/overloaded.
		if !s.isAccountSchedulableForSelection(acc) {
			continue
		}
		// require_privacy_set: 跳过 privacy 未设置的账号并标记异常
		if schedGroup != nil && schedGroup.RequirePrivacySet && !acc.IsPrivacySet() {
			_ = s.accountRepo.SetError(ctx, acc.ID,
				fmt.Sprintf("Privacy not set, required by group [%s]", schedGroup.Name))
			continue
		}
		// 过滤：原生平台直接通过，antigravity 需要启用混合调度
		if acc.Platform == PlatformAntigravity && !acc.IsMixedSchedulingEnabled() {
			continue
		}
		if requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, acc, requestedModel) {
			continue
		}
		if needsUpstreamCheck && s.isUpstreamModelRestrictedByChannel(ctx, *groupID, acc, requestedModel) {
			continue
		}
		if !s.isAccountSchedulableForModelSelection(ctx, acc, requestedModel) {
			continue
		}
		if !s.isAccountSchedulableForQuota(acc) {
			continue
		}
		if !s.isAccountSchedulableForWindowCost(ctx, acc, false) {
			continue
		}
		if !s.isAccountSchedulableForRPM(ctx, acc, false) {
			continue
		}
		if selected == nil {
			selected = acc
			continue
		}
		if acc.Priority < selected.Priority {
			selected = acc
		} else if acc.Priority == selected.Priority {
			switch {
			case acc.LastUsedAt == nil && selected.LastUsedAt != nil:
				selected = acc
			case acc.LastUsedAt != nil && selected.LastUsedAt == nil:
				// keep selected (never used is preferred)
			case acc.LastUsedAt == nil && selected.LastUsedAt == nil:
				if preferOAuth && acc.Platform == PlatformGemini && selected.Platform == PlatformGemini && acc.Type != selected.Type && acc.Type == AccountTypeOAuth {
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
		stats := s.logDetailedSelectionFailure(ctx, groupID, sessionHash, requestedModel, nativePlatform, accounts, excludedIDs, true)
		if requestedModel != "" {
			return nil, fmt.Errorf("%w supporting model: %s (%s)", ErrNoAvailableAccounts, requestedModel, summarizeSelectionFailureStats(stats))
		}
		return nil, ErrNoAvailableAccounts
	}

	// 4. 建立粘性绑定
	if sessionHash != "" && s.cache != nil {
		if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, selected.ID, stickySessionTTL); err != nil {
			logger.LegacyPrintf("service.gateway", "set session account failed: session=%s account_id=%d err=%v", sessionHash, selected.ID, err)
		}
	}

	return selected, nil
}

type selectionFailureStats struct {
	Total              int
	Eligible           int
	Excluded           int
	Unschedulable      int
	PlatformFiltered   int
	ModelUnsupported   int
	ModelRateLimited   int
	SamplePlatformIDs  []int64
	SampleMappingIDs   []int64
	SampleRateLimitIDs []string
}

type selectionFailureDiagnosis struct {
	Category string
	Detail   string
}

func (s *GatewayService) logDetailedSelectionFailure(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	platform string,
	accounts []Account,
	excludedIDs map[int64]struct{},
	allowMixedScheduling bool,
) selectionFailureStats {
	stats := s.collectSelectionFailureStats(ctx, accounts, requestedModel, platform, excludedIDs, allowMixedScheduling)
	logger.LegacyPrintf(
		"service.gateway",
		"[SelectAccountDetailed] group_id=%v model=%s platform=%s session=%s total=%d eligible=%d excluded=%d unschedulable=%d platform_filtered=%d model_unsupported=%d model_rate_limited=%d sample_platform_filtered=%v sample_model_unsupported=%v sample_model_rate_limited=%v",
		derefGroupID(groupID),
		requestedModel,
		platform,
		shortSessionHash(sessionHash),
		stats.Total,
		stats.Eligible,
		stats.Excluded,
		stats.Unschedulable,
		stats.PlatformFiltered,
		stats.ModelUnsupported,
		stats.ModelRateLimited,
		stats.SamplePlatformIDs,
		stats.SampleMappingIDs,
		stats.SampleRateLimitIDs,
	)
	return stats
}

func (s *GatewayService) collectSelectionFailureStats(
	ctx context.Context,
	accounts []Account,
	requestedModel string,
	platform string,
	excludedIDs map[int64]struct{},
	allowMixedScheduling bool,
) selectionFailureStats {
	stats := selectionFailureStats{
		Total: len(accounts),
	}

	for i := range accounts {
		acc := &accounts[i]
		diagnosis := s.diagnoseSelectionFailure(ctx, acc, requestedModel, platform, excludedIDs, allowMixedScheduling)
		switch diagnosis.Category {
		case "excluded":
			stats.Excluded++
		case "unschedulable":
			stats.Unschedulable++
		case "platform_filtered":
			stats.PlatformFiltered++
			stats.SamplePlatformIDs = appendSelectionFailureSampleID(stats.SamplePlatformIDs, acc.ID)
		case "model_unsupported":
			stats.ModelUnsupported++
			stats.SampleMappingIDs = appendSelectionFailureSampleID(stats.SampleMappingIDs, acc.ID)
		case "model_rate_limited":
			stats.ModelRateLimited++
			remaining := acc.GetRateLimitRemainingTimeWithContext(ctx, requestedModel).Truncate(time.Second)
			stats.SampleRateLimitIDs = appendSelectionFailureRateSample(stats.SampleRateLimitIDs, acc.ID, remaining)
		default:
			stats.Eligible++
		}
	}

	return stats
}

func (s *GatewayService) diagnoseSelectionFailure(
	ctx context.Context,
	acc *Account,
	requestedModel string,
	platform string,
	excludedIDs map[int64]struct{},
	allowMixedScheduling bool,
) selectionFailureDiagnosis {
	if acc == nil {
		return selectionFailureDiagnosis{Category: "unschedulable", Detail: "account_nil"}
	}
	if _, excluded := excludedIDs[acc.ID]; excluded {
		return selectionFailureDiagnosis{Category: "excluded"}
	}
	if !s.isAccountSchedulableForSelection(acc) {
		return selectionFailureDiagnosis{Category: "unschedulable", Detail: "generic_unschedulable"}
	}
	if isPlatformFilteredForSelection(acc, platform, allowMixedScheduling) {
		return selectionFailureDiagnosis{
			Category: "platform_filtered",
			Detail:   fmt.Sprintf("account_platform=%s requested_platform=%s", acc.Platform, strings.TrimSpace(platform)),
		}
	}
	if requestedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, acc, requestedModel) {
		return selectionFailureDiagnosis{
			Category: "model_unsupported",
			Detail:   fmt.Sprintf("model=%s", requestedModel),
		}
	}
	if !s.isAccountSchedulableForModelSelection(ctx, acc, requestedModel) {
		remaining := acc.GetRateLimitRemainingTimeWithContext(ctx, requestedModel).Truncate(time.Second)
		return selectionFailureDiagnosis{
			Category: "model_rate_limited",
			Detail:   fmt.Sprintf("remaining=%s", remaining),
		}
	}
	return selectionFailureDiagnosis{Category: "eligible"}
}

func isPlatformFilteredForSelection(acc *Account, platform string, allowMixedScheduling bool) bool {
	if acc == nil {
		return true
	}
	if allowMixedScheduling {
		if acc.Platform == PlatformAntigravity {
			return !acc.IsMixedSchedulingEnabled()
		}
		return acc.Platform != platform
	}
	if strings.TrimSpace(platform) == "" {
		return false
	}
	return acc.Platform != platform
}

func appendSelectionFailureSampleID(samples []int64, id int64) []int64 {
	const limit = 5
	if len(samples) >= limit {
		return samples
	}
	return append(samples, id)
}

func appendSelectionFailureRateSample(samples []string, accountID int64, remaining time.Duration) []string {
	const limit = 5
	if len(samples) >= limit {
		return samples
	}
	return append(samples, fmt.Sprintf("%d(%s)", accountID, remaining))
}

func summarizeSelectionFailureStats(stats selectionFailureStats) string {
	return fmt.Sprintf(
		"total=%d eligible=%d excluded=%d unschedulable=%d platform_filtered=%d model_unsupported=%d model_rate_limited=%d",
		stats.Total,
		stats.Eligible,
		stats.Excluded,
		stats.Unschedulable,
		stats.PlatformFiltered,
		stats.ModelUnsupported,
		stats.ModelRateLimited,
	)
}

// isModelSupportedByAccountWithContext 根据账户平台检查模型支持（带 context）
// 对于 Antigravity 平台，会先获取映射后的最终模型名（包括 thinking 后缀）再检查支持
func (s *GatewayService) isModelSupportedByAccountWithContext(ctx context.Context, account *Account, requestedModel string) bool {
	if account.Platform == PlatformAntigravity {
		if strings.TrimSpace(requestedModel) == "" {
			return true
		}
		// 使用与转发阶段一致的映射逻辑：自定义映射优先 → 默认映射兜底
		mapped := mapAntigravityModel(account, requestedModel)
		if mapped == "" {
			return false
		}
		// 应用 thinking 后缀后检查最终模型是否在账号映射中
		if enabled, ok := ThinkingEnabledFromContext(ctx); ok {
			finalModel := applyThinkingModelSuffix(mapped, enabled)
			if finalModel == mapped {
				return true // thinking 后缀未改变模型名，映射已通过
			}
			return account.IsModelSupported(finalModel)
		}
		return true
	}
	return s.isModelSupportedByAccount(account, requestedModel)
}

// isModelSupportedByAccount 根据账户平台检查模型支持（无 context，用于非 Antigravity 平台）
func (s *GatewayService) isModelSupportedByAccount(account *Account, requestedModel string) bool {
	if account.Platform == PlatformAntigravity {
		if strings.TrimSpace(requestedModel) == "" {
			return true
		}
		return mapAntigravityModel(account, requestedModel) != ""
	}
	if account.IsBedrock() {
		_, ok := ResolveBedrockModelID(account, requestedModel)
		return ok
	}
	// OpenAI 透传模式：仅替换认证，允许所有模型
	if account.Platform == PlatformOpenAI && account.IsOpenAIPassthroughEnabled() {
		return true
	}
	// OAuth/SetupToken 账号使用 Anthropic 标准映射（短ID → 长ID）
	if account.Platform == PlatformAnthropic && account.Type != AccountTypeAPIKey {
		requestedModel = claude.NormalizeModelID(requestedModel)
	}
	// 其他平台使用账户的模型支持检查
	return account.IsModelSupported(requestedModel)
}

// GetAccessToken 获取账号凭证
func (s *GatewayService) GetAccessToken(ctx context.Context, account *Account) (string, string, error) {
	switch account.Type {
	case AccountTypeOAuth, AccountTypeSetupToken:
		// Both oauth and setup-token use OAuth token flow
		return s.getOAuthToken(ctx, account)
	case AccountTypeAPIKey:
		apiKey := account.GetCredential("api_key")
		if apiKey == "" {
			return "", "", errors.New("api_key not found in credentials")
		}
		return apiKey, "apikey", nil
	case AccountTypeBedrock:
		return "", "bedrock", nil // Bedrock 使用 SigV4 签名或 API Key，由 forwardBedrock 处理
	default:
		return "", "", fmt.Errorf("unsupported account type: %s", account.Type)
	}
}

func (s *GatewayService) getOAuthToken(ctx context.Context, account *Account) (string, string, error) {
	// 对于 Anthropic OAuth 账号，使用 ClaudeTokenProvider 获取缓存的 token
	if account.Platform == PlatformAnthropic && account.Type == AccountTypeOAuth && s.claudeTokenProvider != nil {
		accessToken, err := s.claudeTokenProvider.GetAccessToken(ctx, account)
		if err != nil {
			return "", "", err
		}
		return accessToken, "oauth", nil
	}

	// 其他情况（Gemini 有自己的 TokenProvider，setup-token 类型等）直接从账号读取
	accessToken := account.GetCredential("access_token")
	if accessToken == "" {
		return "", "", errors.New("access_token not found in credentials")
	}
	// Token刷新由后台 TokenRefreshService 处理，此处只返回当前token
	return accessToken, "oauth", nil
}

// 重试相关常量
const (
	// 最大尝试次数（包含首次请求）。过多重试会导致请求堆积与资源耗尽。
	maxRetryAttempts = 5

	// 指数退避：第 N 次失败后的等待 = retryBaseDelay * 2^(N-1)，并且上限为 retryMaxDelay。
	retryBaseDelay = 300 * time.Millisecond
	retryMaxDelay  = 3 * time.Second

	// 最大重试耗时（包含请求本身耗时 + 退避等待时间）。
	// 用于防止极端情况下 goroutine 长时间堆积导致资源耗尽。
	maxRetryElapsed = 10 * time.Second
)

func (s *GatewayService) shouldRetryUpstreamError(account *Account, statusCode int) bool {
	// OAuth/Setup Token 账号：仅 403 重试
	if account.IsOAuth() {
		return statusCode == 403
	}

	// API Key 账号：未配置的错误码重试
	return !account.ShouldHandleErrorCode(statusCode)
}

// shouldFailoverUpstreamError determines whether an upstream error should trigger account failover.
func (s *GatewayService) shouldFailoverUpstreamError(statusCode int) bool {
	switch statusCode {
	case 401, 403, 429, 529:
		return true
	default:
		return statusCode >= 500
	}
}

func retryBackoffDelay(attempt int) time.Duration {
	// attempt 从 1 开始，表示第 attempt 次请求刚失败，需要等待后进行第 attempt+1 次请求。
	if attempt <= 0 {
		return retryBaseDelay
	}
	delay := retryBaseDelay * time.Duration(1<<(attempt-1))
	if delay > retryMaxDelay {
		return retryMaxDelay
	}
	return delay
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// isClaudeCodeClient 判断请求是否来自 Claude Code 客户端
// 简化判断：User-Agent 匹配 + metadata.user_id 存在
func isClaudeCodeClient(userAgent string, metadataUserID string) bool {
	if metadataUserID == "" {
		return false
	}
	return claudeCliUserAgentRe.MatchString(userAgent)
}

func isClaudeCodeRequest(ctx context.Context, c *gin.Context, parsed *ParsedRequest) bool {
	if IsClaudeCodeClient(ctx) {
		return true
	}
	if parsed == nil || c == nil {
		return false
	}
	return isClaudeCodeClient(c.GetHeader("User-Agent"), parsed.MetadataUserID)
}

// normalizeSystemParam 将 json.RawMessage 类型的 system 参数转为标准 Go 类型（string / []any / nil），
// 避免 type switch 中 json.RawMessage（底层 []byte）无法匹配 case string / case []any / case nil 的问题。
// 这是 Go 的 typed nil 陷阱：(json.RawMessage, nil) ≠ (nil, nil)。
func normalizeSystemParam(system any) any {
	raw, ok := system.(json.RawMessage)
	if !ok {
		return system
	}
	if len(raw) == 0 {
		return nil
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}
	return parsed
}

// systemIncludesClaudeCodePrompt 检查 system 中是否已包含 Claude Code 提示词
// 使用前缀匹配支持多种变体（标准版、Agent SDK 版等）
func systemIncludesClaudeCodePrompt(system any) bool {
	system = normalizeSystemParam(system)
	switch v := system.(type) {
	case string:
		return hasClaudeCodePrefix(v)
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if text, ok := m["text"].(string); ok && hasClaudeCodePrefix(text) {
					return true
				}
			}
		}
	}
	return false
}

// hasClaudeCodePrefix 检查文本是否以 Claude Code 提示词的特征前缀开头
func hasClaudeCodePrefix(text string) bool {
	for _, prefix := range claudeCodePromptPrefixes {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

// injectClaudeCodePrompt 在 system 开头注入 Claude Code 提示词
// 处理 null、字符串、数组三种格式
func injectClaudeCodePrompt(body []byte, system any) []byte {
	system = normalizeSystemParam(system)
	claudeCodeBlock, err := marshalAnthropicSystemTextBlock(claudeCodeSystemPrompt, true)
	if err != nil {
		logger.LegacyPrintf("service.gateway", "Warning: failed to build Claude Code prompt block: %v", err)
		return body
	}
	// Opencode plugin applies an extra safeguard: it not only prepends the Claude Code
	// banner, it also prefixes the next system instruction with the same banner plus
	// a blank line. This helps when upstream concatenates system instructions.
	claudeCodePrefix := strings.TrimSpace(claudeCodeSystemPrompt)

	var items [][]byte

	switch v := system.(type) {
	case nil:
		items = [][]byte{claudeCodeBlock}
	case string:
		// Be tolerant of older/newer clients that may differ only by trailing whitespace/newlines.
		if strings.TrimSpace(v) == "" || strings.TrimSpace(v) == strings.TrimSpace(claudeCodeSystemPrompt) {
			items = [][]byte{claudeCodeBlock}
		} else {
			// Mirror opencode behavior: keep the banner as a separate system entry,
			// but also prefix the next system text with the banner.
			merged := v
			if !strings.HasPrefix(v, claudeCodePrefix) {
				merged = claudeCodePrefix + "\n\n" + v
			}
			nextBlock, buildErr := marshalAnthropicSystemTextBlock(merged, false)
			if buildErr != nil {
				logger.LegacyPrintf("service.gateway", "Warning: failed to build prefixed Claude Code system block: %v", buildErr)
				return body
			}
			items = [][]byte{claudeCodeBlock, nextBlock}
		}
	case []any:
		items = make([][]byte, 0, len(v)+1)
		items = append(items, claudeCodeBlock)
		prefixedNext := false
		systemResult := gjson.GetBytes(body, "system")
		if systemResult.IsArray() {
			systemResult.ForEach(func(_, item gjson.Result) bool {
				textResult := item.Get("text")
				if textResult.Exists() && textResult.Type == gjson.String &&
					strings.TrimSpace(textResult.String()) == strings.TrimSpace(claudeCodeSystemPrompt) {
					return true
				}

				raw := []byte(item.Raw)
				// Prefix the first subsequent text system block once.
				if !prefixedNext && item.Get("type").String() == "text" && textResult.Exists() && textResult.Type == gjson.String {
					text := textResult.String()
					if strings.TrimSpace(text) != "" && !strings.HasPrefix(text, claudeCodePrefix) {
						next, setErr := sjson.SetBytes(raw, "text", claudeCodePrefix+"\n\n"+text)
						if setErr == nil {
							raw = next
							prefixedNext = true
						}
					}
				}
				items = append(items, raw)
				return true
			})
		} else {
			for _, item := range v {
				m, ok := item.(map[string]any)
				if !ok {
					raw, marshalErr := json.Marshal(item)
					if marshalErr == nil {
						items = append(items, raw)
					}
					continue
				}
				if text, ok := m["text"].(string); ok && strings.TrimSpace(text) == strings.TrimSpace(claudeCodeSystemPrompt) {
					continue
				}
				if !prefixedNext {
					if blockType, _ := m["type"].(string); blockType == "text" {
						if text, ok := m["text"].(string); ok && strings.TrimSpace(text) != "" && !strings.HasPrefix(text, claudeCodePrefix) {
							m["text"] = claudeCodePrefix + "\n\n" + text
							prefixedNext = true
						}
					}
				}
				raw, marshalErr := json.Marshal(m)
				if marshalErr == nil {
					items = append(items, raw)
				}
			}
		}
	default:
		items = [][]byte{claudeCodeBlock}
	}

	result, ok := setJSONRawBytes(body, "system", buildJSONArrayRaw(items))
	if !ok {
		logger.LegacyPrintf("service.gateway", "Warning: failed to inject Claude Code prompt")
		return body
	}
	return result
}

// rewriteSystemForNonClaudeCode 将非 Claude Code 客户端的 system prompt 迁移至 messages，
// system 字段仅保留 Claude Code 标识提示词。
// Anthropic 基于 system 参数内容检测第三方应用，仅前置追加 Claude Code 提示词
// 无法通过检测，因为后续内容仍为非 Claude Code 格式。
// 策略：将原始 system prompt 提取并注入为 user/assistant 消息对，system 仅保留 Claude Code 标识。
func rewriteSystemForNonClaudeCode(body []byte, system any) []byte {
	system = normalizeSystemParam(system)

	// 1. 提取原始 system prompt 文本
	var originalSystemText string
	switch v := system.(type) {
	case string:
		originalSystemText = strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if text, ok := m["text"].(string); ok && strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			}
		}
		originalSystemText = strings.Join(parts, "\n\n")
	}

	// 2. 将 system 替换为 Claude Code 标准提示词（array 格式，与真实 Claude Code 一致）
	//    真实 Claude Code 始终以 [{type: "text", text: "...", cache_control: {type: "ephemeral"}}] 发送 system。
	//    使用 string 格式会被 Anthropic 检测为第三方应用。
	claudeCodeSystemBlock := []map[string]any{
		{
			"type":          "text",
			"text":          claudeCodeSystemPrompt,
			"cache_control": map[string]string{"type": "ephemeral"},
		},
	}
	out, ok := setJSONValueBytes(body, "system", claudeCodeSystemBlock)
	if !ok {
		logger.LegacyPrintf("service.gateway", "Warning: failed to set Claude Code system prompt")
		return body
	}

	// 3. 将原始 system prompt 作为 user/assistant 消息对注入到 messages 开头
	//    模型仍通过 messages 接收完整指令，保留客户端功能
	ccPromptTrimmed := strings.TrimSpace(claudeCodeSystemPrompt)
	if originalSystemText != "" && originalSystemText != ccPromptTrimmed && !hasClaudeCodePrefix(originalSystemText) {
		instrMsg, err1 := json.Marshal(map[string]any{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "[System Instructions]\n" + originalSystemText},
			},
		})
		ackMsg, err2 := json.Marshal(map[string]any{
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": "Understood. I will follow these instructions."},
			},
		})
		if err1 != nil || err2 != nil {
			logger.LegacyPrintf("service.gateway", "Warning: failed to marshal system-to-messages injection")
			return out
		}

		// 重建 messages 数组：[instruction, ack, ...originalMessages]
		items := [][]byte{instrMsg, ackMsg}
		messagesResult := gjson.GetBytes(out, "messages")
		if messagesResult.IsArray() {
			messagesResult.ForEach(func(_, msg gjson.Result) bool {
				items = append(items, []byte(msg.Raw))
				return true
			})
		}

		if next, setOk := setJSONRawBytes(out, "messages", buildJSONArrayRaw(items)); setOk {
			out = next
		}
	}

	return out
}

type cacheControlPath struct {
	path string
	log  string
}

func collectCacheControlPaths(body []byte) (invalidThinking []cacheControlPath, messagePaths []string, systemPaths []string) {
	system := gjson.GetBytes(body, "system")
	if system.IsArray() {
		sysIndex := 0
		system.ForEach(func(_, item gjson.Result) bool {
			if item.Get("cache_control").Exists() {
				path := fmt.Sprintf("system.%d.cache_control", sysIndex)
				if item.Get("type").String() == "thinking" {
					invalidThinking = append(invalidThinking, cacheControlPath{
						path: path,
						log:  "[Warning] Removed illegal cache_control from thinking block in system",
					})
				} else {
					systemPaths = append(systemPaths, path)
				}
			}
			sysIndex++
			return true
		})
	}

	messages := gjson.GetBytes(body, "messages")
	if messages.IsArray() {
		msgIndex := 0
		messages.ForEach(func(_, msg gjson.Result) bool {
			content := msg.Get("content")
			if content.IsArray() {
				contentIndex := 0
				content.ForEach(func(_, item gjson.Result) bool {
					if item.Get("cache_control").Exists() {
						path := fmt.Sprintf("messages.%d.content.%d.cache_control", msgIndex, contentIndex)
						if item.Get("type").String() == "thinking" {
							invalidThinking = append(invalidThinking, cacheControlPath{
								path: path,
								log:  fmt.Sprintf("[Warning] Removed illegal cache_control from thinking block in messages[%d].content[%d]", msgIndex, contentIndex),
							})
						} else {
							messagePaths = append(messagePaths, path)
						}
					}
					contentIndex++
					return true
				})
			}
			msgIndex++
			return true
		})
	}

	return invalidThinking, messagePaths, systemPaths
}

// enforceCacheControlLimit 强制执行 cache_control 块数量限制（最多 4 个）
// 超限时优先从 messages 中移除 cache_control，保护 system 中的缓存控制
func enforceCacheControlLimit(body []byte) []byte {
	if len(body) == 0 {
		return body
	}

	invalidThinking, messagePaths, systemPaths := collectCacheControlPaths(body)
	out := body
	modified := false

	// 先清理 thinking 块中的非法 cache_control（thinking 块不支持该字段）
	for _, item := range invalidThinking {
		if !gjson.GetBytes(out, item.path).Exists() {
			continue
		}
		next, ok := deleteJSONPathBytes(out, item.path)
		if !ok {
			continue
		}
		out = next
		modified = true
		logger.LegacyPrintf("service.gateway", "%s", item.log)
	}

	count := len(messagePaths) + len(systemPaths)
	if count <= maxCacheControlBlocks {
		if modified {
			return out
		}
		return body
	}

	// 超限：优先从 messages 中移除，再从 system 中移除
	remaining := count - maxCacheControlBlocks
	for _, path := range messagePaths {
		if remaining <= 0 {
			break
		}
		if !gjson.GetBytes(out, path).Exists() {
			continue
		}
		next, ok := deleteJSONPathBytes(out, path)
		if !ok {
			continue
		}
		out = next
		modified = true
		remaining--
	}

	for i := len(systemPaths) - 1; i >= 0 && remaining > 0; i-- {
		path := systemPaths[i]
		if !gjson.GetBytes(out, path).Exists() {
			continue
		}
		next, ok := deleteJSONPathBytes(out, path)
		if !ok {
			continue
		}
		out = next
		modified = true
		remaining--
	}

	if modified {
		return out
	}
	return body
}

// Forward 转发请求到Claude API
func (s *GatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, parsed *ParsedRequest) (*ForwardResult, error) {
	startTime := time.Now()
	if parsed == nil {
		return nil, fmt.Errorf("parse request: empty request")
	}

	// Web Search 模拟：纯 web_search 请求时，直接调用搜索 API 构造响应
	if account != nil && s.shouldEmulateWebSearch(ctx, account, parsed.GroupID, parsed.Body) {
		return s.handleWebSearchEmulation(ctx, c, account, parsed)
	}

	if account != nil && account.IsAnthropicAPIKeyPassthroughEnabled() {
		passthroughBody := parsed.Body
		passthroughModel := parsed.Model
		if passthroughModel != "" {
			if mappedModel := account.GetMappedModel(passthroughModel); mappedModel != passthroughModel {
				passthroughBody = s.replaceModelInBody(passthroughBody, mappedModel)
				logger.LegacyPrintf("service.gateway", "Passthrough model mapping: %s -> %s (account: %s)", parsed.Model, mappedModel, account.Name)
				passthroughModel = mappedModel
			}
		}
		return s.forwardAnthropicAPIKeyPassthroughWithInput(ctx, c, account, anthropicPassthroughForwardInput{
			Body:          passthroughBody,
			RequestModel:  passthroughModel,
			OriginalModel: parsed.Model,
			RequestStream: parsed.Stream,
			StartTime:     startTime,
		})
	}

	if account != nil && account.IsBedrock() {
		return s.forwardBedrock(ctx, c, account, parsed, startTime)
	}

	// Beta policy: evaluate once; block check + cache filter set for buildUpstreamRequest.
	// Always overwrite the cache to prevent stale values from a previous retry with a different account.
	if account.Platform == PlatformAnthropic && c != nil {
		policy := s.evaluateBetaPolicy(ctx, c.GetHeader("anthropic-beta"), account, parsed.Model)
		if policy.blockErr != nil {
			return nil, policy.blockErr
		}
		filterSet := policy.filterSet
		if filterSet == nil {
			filterSet = map[string]struct{}{}
		}
		c.Set(betaPolicyFilterSetKey, filterSet)
	}

	body := parsed.Body
	reqModel := parsed.Model
	reqStream := parsed.Stream
	originalModel := reqModel

	// === DEBUG: 打印客户端原始请求（headers + body 摘要）===
	if c != nil {
		s.debugLogGatewaySnapshot("CLIENT_ORIGINAL", c.Request.Header, body, map[string]string{
			"account":      fmt.Sprintf("%d(%s)", account.ID, account.Name),
			"account_type": string(account.Type),
			"model":        reqModel,
			"stream":       strconv.FormatBool(reqStream),
		})
	}

	isClaudeCode := isClaudeCodeRequest(ctx, c, parsed)
	shouldMimicClaudeCode := account.IsOAuth() && !isClaudeCode

	if shouldMimicClaudeCode {
		// 非 Claude Code 客户端：将 system 替换为 Claude Code 标识，原始 system 迁移至 messages
		// 条件：1) OAuth/SetupToken 账号  2) 不是 Claude Code 客户端  3) 不是 Haiku 模型  4) system 中还没有 Claude Code 提示词
		systemRewritten := false
		if !strings.Contains(strings.ToLower(reqModel), "haiku") &&
			!systemIncludesClaudeCodePrompt(parsed.System) {
			body = rewriteSystemForNonClaudeCode(body, parsed.System)
			systemRewritten = true
		}

		// system 被重写时保留 CC prompt 的 cache_control: ephemeral（匹配真实 Claude Code 行为）；
		// 未重写时（haiku / 已含 CC 前缀）剥离客户端 cache_control，与原有行为一致。
		// 两种情况下 enforceCacheControlLimit 都会兜底处理上限。
		normalizeOpts := claudeOAuthNormalizeOptions{stripSystemCacheControl: !systemRewritten}
		if s.identityService != nil {
			fp, err := s.identityService.GetOrCreateFingerprint(ctx, account.ID, c.Request.Header)
			if err == nil && fp != nil {
				// metadata 透传开启时跳过 metadata 注入
				_, mimicMPT, _ := s.settingService.GetGatewayForwardingSettings(ctx)
				if !mimicMPT {
					if metadataUserID := s.buildOAuthMetadataUserID(parsed, account, fp); metadataUserID != "" {
						normalizeOpts.injectMetadata = true
						normalizeOpts.metadataUserID = metadataUserID
					}
				}
			}
		}

		body, reqModel = normalizeClaudeOAuthRequestBody(body, reqModel, normalizeOpts)
	}

	// 强制执行 cache_control 块数量限制（最多 4 个）
	body = enforceCacheControlLimit(body)

	// 应用模型映射：
	// - APIKey 账号：使用账号级别的显式映射（如果配置），否则透传原始模型名
	// - OAuth/SetupToken 账号：使用 Anthropic 标准映射（短ID → 长ID）
	mappedModel := reqModel
	mappingSource := ""
	if account.Type == AccountTypeAPIKey {
		mappedModel = account.GetMappedModel(reqModel)
		if mappedModel != reqModel {
			mappingSource = "account"
		}
	}
	if mappingSource == "" && account.Platform == PlatformAnthropic && account.Type != AccountTypeAPIKey {
		normalized := claude.NormalizeModelID(reqModel)
		if normalized != reqModel {
			mappedModel = normalized
			mappingSource = "prefix"
		}
	}
	if mappedModel != reqModel {
		// 替换请求体中的模型名
		body = s.replaceModelInBody(body, mappedModel)
		reqModel = mappedModel
		logger.LegacyPrintf("service.gateway", "Model mapping applied: %s -> %s (account: %s, source=%s)", originalModel, mappedModel, account.Name, mappingSource)
	}

	// 获取凭证
	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	// 获取代理URL（自定义 base URL 模式下，proxy 通过 buildCustomRelayURL 作为查询参数传递）
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		if !account.IsCustomBaseURLEnabled() || account.GetCustomBaseURL() == "" {
			proxyURL = account.Proxy.URL()
		}
	}

	// 解析 TLS 指纹 profile（同一请求生命周期内不变，避免重试循环中重复解析）
	tlsProfile := s.tlsFPProfileService.ResolveTLSProfile(account)

	// 调试日志：记录即将转发的账号信息
	logger.LegacyPrintf("service.gateway", "[Forward] Using account: ID=%d Name=%s Platform=%s Type=%s TLSFingerprint=%v Proxy=%s",
		account.ID, account.Name, account.Platform, account.Type, tlsProfile, proxyURL)
	// Pre-filter: strip empty text blocks (including nested in tool_result) to prevent upstream 400.
	body = StripEmptyTextBlocks(body)

	// 重试间复用同一请求体，避免每次 string(body) 产生额外分配。
	setOpsUpstreamRequestBody(c, body)

	// 重试循环
	var resp *http.Response
	retryStart := time.Now()
	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		// 构建上游请求（每次重试需要重新构建，因为请求体需要重新读取）
		upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, reqStream)
		upstreamReq, err := s.buildUpstreamRequest(upstreamCtx, c, account, body, token, tokenType, reqModel, reqStream, shouldMimicClaudeCode)
		releaseUpstreamCtx()
		if err != nil {
			return nil, err
		}

		// 发送请求
		resp, err = s.httpUpstream.DoWithTLS(upstreamReq, proxyURL, account.ID, account.Concurrency, tlsProfile)
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			// Ensure the client receives an error response (handlers assume Forward writes on non-failover errors).
			safeErr := sanitizeUpstreamErrorMessage(err.Error())
			setOpsUpstreamError(c, 0, safeErr, "")
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: 0,
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "request_error",
				Message:            safeErr,
			})
			c.JSON(http.StatusBadGateway, gin.H{
				"type": "error",
				"error": gin.H{
					"type":    "upstream_error",
					"message": "Upstream request failed",
				},
			})
			return nil, fmt.Errorf("upstream request failed: %s", safeErr)
		}

		// 优先检测thinking block签名错误（400）并重试一次
		if resp.StatusCode == 400 {
			respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			if readErr == nil {
				_ = resp.Body.Close()

				if s.shouldRectifySignatureError(ctx, account, respBody) {
					appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
						Platform:           account.Platform,
						AccountID:          account.ID,
						AccountName:        account.Name,
						UpstreamStatusCode: resp.StatusCode,
						UpstreamRequestID:  resp.Header.Get("x-request-id"),
						UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
						Kind:               "signature_error",
						Message:            extractUpstreamErrorMessage(respBody),
						Detail: func() string {
							if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
								return truncateString(string(respBody), s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
							}
							return ""
						}(),
					})

					looksLikeToolSignatureError := func(msg string) bool {
						m := strings.ToLower(msg)
						return strings.Contains(m, "tool_use") ||
							strings.Contains(m, "tool_result") ||
							strings.Contains(m, "functioncall") ||
							strings.Contains(m, "function_call") ||
							strings.Contains(m, "functionresponse") ||
							strings.Contains(m, "function_response")
					}

					// 避免在重试预算已耗尽时再发起额外请求
					if time.Since(retryStart) >= maxRetryElapsed {
						resp.Body = io.NopCloser(bytes.NewReader(respBody))
						break
					}
					logger.LegacyPrintf("service.gateway", "[warn] Account %d: thinking blocks have invalid signature, retrying with filtered blocks", account.ID)

					// Conservative two-stage fallback:
					// 1) Disable thinking + thinking->text (preserve content)
					// 2) Only if upstream still errors AND error message points to tool/function signature issues:
					//    also downgrade tool_use/tool_result blocks to text.

					filteredBody := FilterThinkingBlocksForRetry(body)
					retryCtx, releaseRetryCtx := detachStreamUpstreamContext(ctx, reqStream)
					retryReq, buildErr := s.buildUpstreamRequest(retryCtx, c, account, filteredBody, token, tokenType, reqModel, reqStream, shouldMimicClaudeCode)
					releaseRetryCtx()
					if buildErr == nil {
						retryResp, retryErr := s.httpUpstream.DoWithTLS(retryReq, proxyURL, account.ID, account.Concurrency, tlsProfile)
						if retryErr == nil {
							if retryResp.StatusCode < 400 {
								logger.LegacyPrintf("service.gateway", "Account %d: thinking block retry succeeded (blocks downgraded)", account.ID)
								resp = retryResp
								break
							}

							retryRespBody, retryReadErr := io.ReadAll(io.LimitReader(retryResp.Body, 2<<20))
							_ = retryResp.Body.Close()
							if retryReadErr == nil && retryResp.StatusCode == 400 && s.isSignatureErrorPattern(ctx, account, retryRespBody) {
								appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
									Platform:           account.Platform,
									AccountID:          account.ID,
									AccountName:        account.Name,
									UpstreamStatusCode: retryResp.StatusCode,
									UpstreamRequestID:  retryResp.Header.Get("x-request-id"),
									UpstreamURL:        safeUpstreamURL(retryReq.URL.String()),
									Kind:               "signature_retry_thinking",
									Message:            extractUpstreamErrorMessage(retryRespBody),
									Detail: func() string {
										if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
											return truncateString(string(retryRespBody), s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
										}
										return ""
									}(),
								})
								msg2 := extractUpstreamErrorMessage(retryRespBody)
								if looksLikeToolSignatureError(msg2) && time.Since(retryStart) < maxRetryElapsed {
									logger.LegacyPrintf("service.gateway", "Account %d: signature retry still failing and looks tool-related, retrying with tool blocks downgraded", account.ID)
									filteredBody2 := FilterSignatureSensitiveBlocksForRetry(body)
									retryCtx2, releaseRetryCtx2 := detachStreamUpstreamContext(ctx, reqStream)
									retryReq2, buildErr2 := s.buildUpstreamRequest(retryCtx2, c, account, filteredBody2, token, tokenType, reqModel, reqStream, shouldMimicClaudeCode)
									releaseRetryCtx2()
									if buildErr2 == nil {
										retryResp2, retryErr2 := s.httpUpstream.DoWithTLS(retryReq2, proxyURL, account.ID, account.Concurrency, tlsProfile)
										if retryErr2 == nil {
											resp = retryResp2
											break
										}
										if retryResp2 != nil && retryResp2.Body != nil {
											_ = retryResp2.Body.Close()
										}
										appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
											Platform:           account.Platform,
											AccountID:          account.ID,
											AccountName:        account.Name,
											UpstreamStatusCode: 0,
											UpstreamURL:        safeUpstreamURL(retryReq2.URL.String()),
											Kind:               "signature_retry_tools_request_error",
											Message:            sanitizeUpstreamErrorMessage(retryErr2.Error()),
										})
										logger.LegacyPrintf("service.gateway", "Account %d: tool-downgrade signature retry failed: %v", account.ID, retryErr2)
									} else {
										logger.LegacyPrintf("service.gateway", "Account %d: tool-downgrade signature retry build failed: %v", account.ID, buildErr2)
									}
								}
							}

							// Fall back to the original retry response context.
							resp = &http.Response{
								StatusCode: retryResp.StatusCode,
								Header:     retryResp.Header.Clone(),
								Body:       io.NopCloser(bytes.NewReader(retryRespBody)),
							}
							break
						}
						if retryResp != nil && retryResp.Body != nil {
							_ = retryResp.Body.Close()
						}
						logger.LegacyPrintf("service.gateway", "Account %d: signature error retry failed: %v", account.ID, retryErr)
					} else {
						logger.LegacyPrintf("service.gateway", "Account %d: signature error retry build request failed: %v", account.ID, buildErr)
					}

					// Retry failed: restore original response body and continue handling.
					resp.Body = io.NopCloser(bytes.NewReader(respBody))
					break
				}
				// 不是签名错误（或整流器已关闭），继续检查 budget 约束
				errMsg := extractUpstreamErrorMessage(respBody)
				if isThinkingBudgetConstraintError(errMsg) && s.settingService.IsBudgetRectifierEnabled(ctx) {
					appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
						Platform:           account.Platform,
						AccountID:          account.ID,
						AccountName:        account.Name,
						UpstreamStatusCode: resp.StatusCode,
						UpstreamRequestID:  resp.Header.Get("x-request-id"),
						UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
						Kind:               "budget_constraint_error",
						Message:            errMsg,
						Detail: func() string {
							if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
								return truncateString(string(respBody), s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
							}
							return ""
						}(),
					})

					rectifiedBody, applied := RectifyThinkingBudget(body)
					if applied && time.Since(retryStart) < maxRetryElapsed {
						logger.LegacyPrintf("service.gateway", "Account %d: detected budget_tokens constraint error, retrying with rectified budget (budget_tokens=%d, max_tokens=%d)", account.ID, BudgetRectifyBudgetTokens, BudgetRectifyMaxTokens)
						budgetRetryCtx, releaseBudgetRetryCtx := detachStreamUpstreamContext(ctx, reqStream)
						budgetRetryReq, buildErr := s.buildUpstreamRequest(budgetRetryCtx, c, account, rectifiedBody, token, tokenType, reqModel, reqStream, shouldMimicClaudeCode)
						releaseBudgetRetryCtx()
						if buildErr == nil {
							budgetRetryResp, retryErr := s.httpUpstream.DoWithTLS(budgetRetryReq, proxyURL, account.ID, account.Concurrency, tlsProfile)
							if retryErr == nil {
								resp = budgetRetryResp
								break
							}
							if budgetRetryResp != nil && budgetRetryResp.Body != nil {
								_ = budgetRetryResp.Body.Close()
							}
							logger.LegacyPrintf("service.gateway", "Account %d: budget rectifier retry failed: %v", account.ID, retryErr)
						} else {
							logger.LegacyPrintf("service.gateway", "Account %d: budget rectifier retry build failed: %v", account.ID, buildErr)
						}
					}
				}

				resp.Body = io.NopCloser(bytes.NewReader(respBody))
			}
		}

		// 检查是否需要通用重试（排除400，因为400已经在上面特殊处理过了）
		if resp.StatusCode >= 400 && resp.StatusCode != 400 && s.shouldRetryUpstreamError(account, resp.StatusCode) {
			if attempt < maxRetryAttempts {
				elapsed := time.Since(retryStart)
				if elapsed >= maxRetryElapsed {
					break
				}

				delay := retryBackoffDelay(attempt)
				remaining := maxRetryElapsed - elapsed
				if delay > remaining {
					delay = remaining
				}
				if delay <= 0 {
					break
				}

				respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
				_ = resp.Body.Close()
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  resp.Header.Get("x-request-id"),
					UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
					Kind:               "retry",
					Message:            extractUpstreamErrorMessage(respBody),
					Detail: func() string {
						if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
							return truncateString(string(respBody), s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
						}
						return ""
					}(),
				})
				logger.LegacyPrintf("service.gateway", "Account %d: upstream error %d, retry %d/%d after %v (elapsed=%v/%v)",
					account.ID, resp.StatusCode, attempt, maxRetryAttempts, delay, elapsed, maxRetryElapsed)
				if err := sleepWithContext(ctx, delay); err != nil {
					return nil, err
				}
				continue
			}
			// 最后一次尝试也失败，跳出循环处理重试耗尽
			break
		}

		// 不需要重试（成功或不可重试的错误），跳出循环
		// DEBUG: 输出响应 headers（用于检测 rate limit 信息）
		if account.Platform == PlatformGemini && resp.StatusCode < 400 && s.cfg != nil && s.cfg.Gateway.GeminiDebugResponseHeaders {
			logger.LegacyPrintf("service.gateway", "[DEBUG] Gemini API Response Headers for account %d:", account.ID)
			for k, v := range resp.Header {
				logger.LegacyPrintf("service.gateway", "[DEBUG]   %s: %v", k, v)
			}
		}
		break
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.New("upstream request failed: empty response")
	}
	defer func() { _ = resp.Body.Close() }()

	// 处理重试耗尽的情况
	if resp.StatusCode >= 400 && s.shouldRetryUpstreamError(account, resp.StatusCode) {
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(respBody))

			// 调试日志：打印重试耗尽后的错误响应
			logger.LegacyPrintf("service.gateway", "[Forward] Upstream error (retry exhausted, failover): Account=%d(%s) Status=%d RequestID=%s Body=%s",
				account.ID, account.Name, resp.StatusCode, resp.Header.Get("x-request-id"), truncateString(string(respBody), 1000))

			s.handleRetryExhaustedSideEffects(ctx, resp, account)
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "retry_exhausted_failover",
				Message:            extractUpstreamErrorMessage(respBody),
				Detail: func() string {
					if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
						return truncateString(string(respBody), s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
					}
					return ""
				}(),
			})
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return s.handleRetryExhaustedError(ctx, resp, c, account)
	}

	// 处理可切换账号的错误
	if resp.StatusCode >= 400 && s.shouldFailoverUpstreamError(resp.StatusCode) {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		// 调试日志：打印上游错误响应
		logger.LegacyPrintf("service.gateway", "[Forward] Upstream error (failover): Account=%d(%s) Status=%d RequestID=%s Body=%s",
			account.ID, account.Name, resp.StatusCode, resp.Header.Get("x-request-id"), truncateString(string(respBody), 1000))

		s.handleFailoverSideEffects(ctx, resp, account)
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			Kind:               "failover",
			Message:            extractUpstreamErrorMessage(respBody),
			Detail: func() string {
				if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
					return truncateString(string(respBody), s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
				}
				return ""
			}(),
		})
		return nil, &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           respBody,
			RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
		}
	}
	if resp.StatusCode >= 400 {
		// 可选：对部分 400 触发 failover（默认关闭以保持语义）
		if resp.StatusCode == 400 && s.cfg != nil && s.cfg.Gateway.FailoverOn400 {
			respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			if readErr != nil {
				// ReadAll failed, fall back to normal error handling without consuming the stream
				return s.handleErrorResponse(ctx, resp, c, account)
			}
			_ = resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(respBody))

			if s.shouldFailoverOn400(respBody) {
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
					UpstreamRequestID:  resp.Header.Get("x-request-id"),
					Kind:               "failover_on_400",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})

				if s.cfg.Gateway.LogUpstreamErrorBody {
					logger.LegacyPrintf("service.gateway",
						"Account %d: 400 error, attempting failover: %s",
						account.ID,
						truncateForLog(respBody, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes),
					)
				} else {
					logger.LegacyPrintf("service.gateway", "Account %d: 400 error, attempting failover", account.ID)
				}
				s.handleFailoverSideEffects(ctx, resp, account)
				return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody}
			}
		}
		return s.handleErrorResponse(ctx, resp, c, account)
	}

	// 处理正常响应

	// 触发上游接受回调（提前释放串行锁，不等流完成）
	if parsed.OnUpstreamAccepted != nil {
		parsed.OnUpstreamAccepted()
	}

	var usage *ClaudeUsage
	var firstTokenMs *int
	var clientDisconnect bool
	if reqStream {
		streamResult, err := s.handleStreamingResponse(ctx, resp, c, account, startTime, originalModel, reqModel, shouldMimicClaudeCode)
		if err != nil {
			if err.Error() == "have error in stream" {
				return nil, &UpstreamFailoverError{
					StatusCode: 403,
				}
			}
			return nil, err
		}
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
		clientDisconnect = streamResult.clientDisconnect
	} else {
		usage, err = s.handleNonStreamingResponse(ctx, resp, c, account, originalModel, reqModel)
		if err != nil {
			return nil, err
		}
	}

	return &ForwardResult{
		RequestID:        resp.Header.Get("x-request-id"),
		Usage:            *usage,
		Model:            originalModel, // 使用原始模型用于计费和日志
		UpstreamModel:    mappedModel,
		Stream:           reqStream,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnect,
	}, nil
}

type anthropicPassthroughForwardInput struct {
	Body          []byte
	RequestModel  string
	OriginalModel string
	RequestStream bool
	StartTime     time.Time
}

func (s *GatewayService) forwardAnthropicAPIKeyPassthrough(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	reqModel string,
	originalModel string,
	reqStream bool,
	startTime time.Time,
) (*ForwardResult, error) {
	return s.forwardAnthropicAPIKeyPassthroughWithInput(ctx, c, account, anthropicPassthroughForwardInput{
		Body:          body,
		RequestModel:  reqModel,
		OriginalModel: originalModel,
		RequestStream: reqStream,
		StartTime:     startTime,
	})
}

func (s *GatewayService) forwardAnthropicAPIKeyPassthroughWithInput(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	input anthropicPassthroughForwardInput,
) (*ForwardResult, error) {
	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	if tokenType != "apikey" {
		return nil, fmt.Errorf("anthropic api key passthrough requires apikey token, got: %s", tokenType)
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	logger.LegacyPrintf("service.gateway", "[Anthropic 自动透传] 命中 API Key 透传分支: account=%d name=%s model=%s stream=%v",
		account.ID, account.Name, input.RequestModel, input.RequestStream)

	if c != nil {
		c.Set("anthropic_passthrough", true)
	}
	// Pre-filter: strip empty text blocks (including nested in tool_result) to prevent upstream 400.
	input.Body = StripEmptyTextBlocks(input.Body)

	// 重试间复用同一请求体，避免每次 string(body) 产生额外分配。
	setOpsUpstreamRequestBody(c, input.Body)

	var resp *http.Response
	retryStart := time.Now()
	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, input.RequestStream)
		upstreamReq, err := s.buildUpstreamRequestAnthropicAPIKeyPassthrough(upstreamCtx, c, account, input.Body, token)
		releaseUpstreamCtx()
		if err != nil {
			return nil, err
		}

		resp, err = s.httpUpstream.DoWithTLS(upstreamReq, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			safeErr := sanitizeUpstreamErrorMessage(err.Error())
			setOpsUpstreamError(c, 0, safeErr, "")
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: 0,
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Passthrough:        true,
				Kind:               "request_error",
				Message:            safeErr,
			})
			c.JSON(http.StatusBadGateway, gin.H{
				"type": "error",
				"error": gin.H{
					"type":    "upstream_error",
					"message": "Upstream request failed",
				},
			})
			return nil, fmt.Errorf("upstream request failed: %s", safeErr)
		}

		// 透传分支禁止 400 请求体降级重试（该重试会改写请求体）
		if resp.StatusCode >= 400 && resp.StatusCode != 400 && s.shouldRetryUpstreamError(account, resp.StatusCode) {
			if attempt < maxRetryAttempts {
				elapsed := time.Since(retryStart)
				if elapsed >= maxRetryElapsed {
					break
				}

				delay := retryBackoffDelay(attempt)
				remaining := maxRetryElapsed - elapsed
				if delay > remaining {
					delay = remaining
				}
				if delay <= 0 {
					break
				}

				respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
				_ = resp.Body.Close()
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  resp.Header.Get("x-request-id"),
					UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
					Passthrough:        true,
					Kind:               "retry",
					Message:            extractUpstreamErrorMessage(respBody),
					Detail: func() string {
						if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
							return truncateString(string(respBody), s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
						}
						return ""
					}(),
				})
				logger.LegacyPrintf("service.gateway", "Anthropic passthrough account %d: upstream error %d, retry %d/%d after %v (elapsed=%v/%v)",
					account.ID, resp.StatusCode, attempt, maxRetryAttempts, delay, elapsed, maxRetryElapsed)
				if err := sleepWithContext(ctx, delay); err != nil {
					return nil, err
				}
				continue
			}
			break
		}

		break
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.New("upstream request failed: empty response")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 && s.shouldRetryUpstreamError(account, resp.StatusCode) {
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(respBody))

			logger.LegacyPrintf("service.gateway", "[Anthropic Passthrough] Upstream error (retry exhausted, failover): Account=%d(%s) Status=%d RequestID=%s Body=%s",
				account.ID, account.Name, resp.StatusCode, resp.Header.Get("x-request-id"), truncateString(string(respBody), 1000))

			s.handleRetryExhaustedSideEffects(ctx, resp, account)
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Passthrough:        true,
				Kind:               "retry_exhausted_failover",
				Message:            extractUpstreamErrorMessage(respBody),
				Detail: func() string {
					if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
						return truncateString(string(respBody), s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
					}
					return ""
				}(),
			})
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return s.handleRetryExhaustedError(ctx, resp, c, account)
	}

	if resp.StatusCode >= 400 && s.shouldFailoverUpstreamError(resp.StatusCode) {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		logger.LegacyPrintf("service.gateway", "[Anthropic Passthrough] Upstream error (failover): Account=%d(%s) Status=%d RequestID=%s Body=%s",
			account.ID, account.Name, resp.StatusCode, resp.Header.Get("x-request-id"), truncateString(string(respBody), 1000))

		s.handleFailoverSideEffects(ctx, resp, account)
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			Passthrough:        true,
			Kind:               "failover",
			Message:            extractUpstreamErrorMessage(respBody),
			Detail: func() string {
				if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
					return truncateString(string(respBody), s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
				}
				return ""
			}(),
		})
		return nil, &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           respBody,
			RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
		}
	}

	if resp.StatusCode >= 400 {
		return s.handleErrorResponse(ctx, resp, c, account)
	}

	var usage *ClaudeUsage
	var firstTokenMs *int
	var clientDisconnect bool
	if input.RequestStream {
		streamResult, err := s.handleStreamingResponseAnthropicAPIKeyPassthrough(ctx, resp, c, account, input.StartTime, input.RequestModel)
		if err != nil {
			return nil, err
		}
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
		clientDisconnect = streamResult.clientDisconnect
	} else {
		usage, err = s.handleNonStreamingResponseAnthropicAPIKeyPassthrough(ctx, resp, c, account)
		if err != nil {
			return nil, err
		}
	}
	if usage == nil {
		usage = &ClaudeUsage{}
	}

	return &ForwardResult{
		RequestID:        resp.Header.Get("x-request-id"),
		Usage:            *usage,
		Model:            input.OriginalModel,
		UpstreamModel:    input.RequestModel,
		Stream:           input.RequestStream,
		Duration:         time.Since(input.StartTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnect,
	}, nil
}

func (s *GatewayService) buildUpstreamRequestAnthropicAPIKeyPassthrough(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
) (*http.Request, error) {
	targetURL := claudeAPIURL
	baseURL := account.GetBaseURL()
	if baseURL != "" {
		validatedURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, err
		}
		targetURL = validatedURL + "/v1/messages?beta=true"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			if !allowedHeaders[lowerKey] {
				continue
			}
			wireKey := resolveWireCasing(key)
			for _, v := range values {
				addHeaderRaw(req.Header, wireKey, v)
			}
		}
	}

	// 覆盖入站鉴权残留，并注入上游认证
	req.Header.Del("authorization")
	req.Header.Del("x-api-key")
	req.Header.Del("x-goog-api-key")
	req.Header.Del("cookie")
	setHeaderRaw(req.Header, "x-api-key", token)

	if getHeaderRaw(req.Header, "content-type") == "" {
		setHeaderRaw(req.Header, "content-type", "application/json")
	}
	if getHeaderRaw(req.Header, "anthropic-version") == "" {
		setHeaderRaw(req.Header, "anthropic-version", "2023-06-01")
	}

	return req, nil
}

func (s *GatewayService) handleStreamingResponseAnthropicAPIKeyPassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	startTime time.Time,
	model string,
) (*streamingResult, error) {
	if s.rateLimitService != nil {
		s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)
	}

	writeAnthropicPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "text/event-stream"
	}
	c.Header("Content-Type", contentType)
	if c.Writer.Header().Get("Cache-Control") == "" {
		c.Header("Cache-Control", "no-cache")
	}
	if c.Writer.Header().Get("Connection") == "" {
		c.Header("Connection", "keep-alive")
	}
	c.Header("X-Accel-Buffering", "no")
	if v := resp.Header.Get("x-request-id"); v != "" {
		c.Header("x-request-id", v)
	}

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	usage := &ClaudeUsage{}
	var firstTokenMs *int
	clientDisconnected := false
	sawTerminalEvent := false

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanBuf := getSSEScannerBuf64K()
	scanner.Buffer(scanBuf[:0], maxLineSize)

	type scanEvent struct {
		line string
		err  error
	}
	events := make(chan scanEvent, 16)
	done := make(chan struct{})
	sendEvent := func(ev scanEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
		}
	}
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	go func(scanBuf *sseScannerBuf64K) {
		defer putSSEScannerBuf64K(scanBuf)
		defer close(events)
		for scanner.Scan() {
			atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			if !sendEvent(scanEvent{line: scanner.Text()}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			_ = sendEvent(scanEvent{err: err})
		}
	}(scanBuf)
	defer close(done)

	streamInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
	}
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
	}

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				if !clientDisconnected {
					// 兜底补刷，确保最后一个未以空行结尾的事件也能及时送达客户端。
					flusher.Flush()
				}
				if !sawTerminalEvent {
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: clientDisconnected}, fmt.Errorf("stream usage incomplete: missing terminal event")
				}
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: clientDisconnected}, nil
			}
			if ev.err != nil {
				if sawTerminalEvent {
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: clientDisconnected}, nil
				}
				if clientDisconnected {
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: true}, fmt.Errorf("stream usage incomplete after disconnect: %w", ev.err)
				}
				if errors.Is(ev.err, context.Canceled) || errors.Is(ev.err, context.DeadlineExceeded) {
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: true}, fmt.Errorf("stream usage incomplete: %w", ev.err)
				}
				if errors.Is(ev.err, bufio.ErrTooLong) {
					logger.LegacyPrintf("service.gateway", "[Anthropic passthrough] SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, ev.err)
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs}, ev.err
				}
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMs}, fmt.Errorf("stream read error: %w", ev.err)
			}

			line := ev.line
			if data, ok := extractAnthropicSSEDataLine(line); ok {
				trimmed := strings.TrimSpace(data)
				if anthropicStreamEventIsTerminal("", trimmed) {
					sawTerminalEvent = true
				}
				if firstTokenMs == nil && trimmed != "" && trimmed != "[DONE]" {
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
				}
				s.parseSSEUsagePassthrough(data, usage)
			} else {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "event:") && anthropicStreamEventIsTerminal(strings.TrimSpace(strings.TrimPrefix(trimmed, "event:")), "") {
					sawTerminalEvent = true
				}
			}

			if !clientDisconnected {
				if _, err := io.WriteString(w, line); err != nil {
					clientDisconnected = true
					logger.LegacyPrintf("service.gateway", "[Anthropic passthrough] Client disconnected during streaming, continue draining upstream for usage: account=%d", account.ID)
				} else if _, err := io.WriteString(w, "\n"); err != nil {
					clientDisconnected = true
					logger.LegacyPrintf("service.gateway", "[Anthropic passthrough] Client disconnected during streaming, continue draining upstream for usage: account=%d", account.ID)
				} else if line == "" {
					// 按 SSE 事件边界刷出，减少每行 flush 带来的 syscall 开销。
					flusher.Flush()
				}
			}

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
			}
			if clientDisconnected {
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: true}, fmt.Errorf("stream usage incomplete after timeout")
			}
			logger.LegacyPrintf("service.gateway", "[Anthropic passthrough] Stream data interval timeout: account=%d model=%s interval=%s", account.ID, model, streamInterval)
			if s.rateLimitService != nil {
				s.rateLimitService.HandleStreamTimeout(ctx, account, model)
			}
			return &streamingResult{usage: usage, firstTokenMs: firstTokenMs}, fmt.Errorf("stream data interval timeout")
		}
	}
}

func extractAnthropicSSEDataLine(line string) (string, bool) {
	if !strings.HasPrefix(line, "data:") {
		return "", false
	}
	start := len("data:")
	for start < len(line) {
		if line[start] != ' ' && line[start] != '\t' {
			break
		}
		start++
	}
	return line[start:], true
}

func (s *GatewayService) parseSSEUsagePassthrough(data string, usage *ClaudeUsage) {
	if usage == nil || data == "" || data == "[DONE]" {
		return
	}

	parsed := gjson.Parse(data)
	switch parsed.Get("type").String() {
	case "message_start":
		msgUsage := parsed.Get("message.usage")
		if msgUsage.Exists() {
			usage.InputTokens = int(msgUsage.Get("input_tokens").Int())
			usage.CacheCreationInputTokens = int(msgUsage.Get("cache_creation_input_tokens").Int())
			usage.CacheReadInputTokens = int(msgUsage.Get("cache_read_input_tokens").Int())

			// 保持与通用解析一致：message_start 允许覆盖 5m/1h 明细（包括 0）。
			cc5m := msgUsage.Get("cache_creation.ephemeral_5m_input_tokens")
			cc1h := msgUsage.Get("cache_creation.ephemeral_1h_input_tokens")
			if cc5m.Exists() || cc1h.Exists() {
				usage.CacheCreation5mTokens = int(cc5m.Int())
				usage.CacheCreation1hTokens = int(cc1h.Int())
			}
		}
	case "message_delta":
		deltaUsage := parsed.Get("usage")
		if deltaUsage.Exists() {
			if v := deltaUsage.Get("input_tokens").Int(); v > 0 {
				usage.InputTokens = int(v)
			}
			if v := deltaUsage.Get("output_tokens").Int(); v > 0 {
				usage.OutputTokens = int(v)
			}
			if v := deltaUsage.Get("cache_creation_input_tokens").Int(); v > 0 {
				usage.CacheCreationInputTokens = int(v)
			}
			if v := deltaUsage.Get("cache_read_input_tokens").Int(); v > 0 {
				usage.CacheReadInputTokens = int(v)
			}

			cc5m := deltaUsage.Get("cache_creation.ephemeral_5m_input_tokens")
			cc1h := deltaUsage.Get("cache_creation.ephemeral_1h_input_tokens")
			if cc5m.Exists() && cc5m.Int() > 0 {
				usage.CacheCreation5mTokens = int(cc5m.Int())
			}
			if cc1h.Exists() && cc1h.Int() > 0 {
				usage.CacheCreation1hTokens = int(cc1h.Int())
			}
		}
	}

	if usage.CacheReadInputTokens == 0 {
		if cached := parsed.Get("message.usage.cached_tokens").Int(); cached > 0 {
			usage.CacheReadInputTokens = int(cached)
		}
		if cached := parsed.Get("usage.cached_tokens").Int(); usage.CacheReadInputTokens == 0 && cached > 0 {
			usage.CacheReadInputTokens = int(cached)
		}
	}
	if usage.CacheCreationInputTokens == 0 {
		cc5m := parsed.Get("message.usage.cache_creation.ephemeral_5m_input_tokens").Int()
		cc1h := parsed.Get("message.usage.cache_creation.ephemeral_1h_input_tokens").Int()
		if cc5m == 0 && cc1h == 0 {
			cc5m = parsed.Get("usage.cache_creation.ephemeral_5m_input_tokens").Int()
			cc1h = parsed.Get("usage.cache_creation.ephemeral_1h_input_tokens").Int()
		}
		total := cc5m + cc1h
		if total > 0 {
			usage.CacheCreationInputTokens = int(total)
		}
	}
}

func parseClaudeUsageFromResponseBody(body []byte) *ClaudeUsage {
	usage := &ClaudeUsage{}
	if len(body) == 0 {
		return usage
	}

	parsed := gjson.ParseBytes(body)
	usageNode := parsed.Get("usage")
	if !usageNode.Exists() {
		return usage
	}

	usage.InputTokens = int(usageNode.Get("input_tokens").Int())
	usage.OutputTokens = int(usageNode.Get("output_tokens").Int())
	usage.CacheCreationInputTokens = int(usageNode.Get("cache_creation_input_tokens").Int())
	usage.CacheReadInputTokens = int(usageNode.Get("cache_read_input_tokens").Int())

	cc5m := usageNode.Get("cache_creation.ephemeral_5m_input_tokens").Int()
	cc1h := usageNode.Get("cache_creation.ephemeral_1h_input_tokens").Int()
	if cc5m > 0 || cc1h > 0 {
		usage.CacheCreation5mTokens = int(cc5m)
		usage.CacheCreation1hTokens = int(cc1h)
	}
	if usage.CacheCreationInputTokens == 0 && (cc5m > 0 || cc1h > 0) {
		usage.CacheCreationInputTokens = int(cc5m + cc1h)
	}
	if usage.CacheReadInputTokens == 0 {
		if cached := usageNode.Get("cached_tokens").Int(); cached > 0 {
			usage.CacheReadInputTokens = int(cached)
		}
	}
	return usage
}

func (s *GatewayService) handleNonStreamingResponseAnthropicAPIKeyPassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
) (*ClaudeUsage, error) {
	if s.rateLimitService != nil {
		s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)
	}

	maxBytes := resolveUpstreamResponseReadLimit(s.cfg)
	body, err := readUpstreamResponseBodyLimited(resp.Body, maxBytes)
	if err != nil {
		if errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			setOpsUpstreamError(c, http.StatusBadGateway, "upstream response too large", "")
			c.JSON(http.StatusBadGateway, gin.H{
				"type": "error",
				"error": gin.H{
					"type":    "upstream_error",
					"message": "Upstream response too large",
				},
			})
		}
		return nil, err
	}

	usage := parseClaudeUsageFromResponseBody(body)

	writeAnthropicPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)
	return usage, nil
}

func writeAnthropicPassthroughResponseHeaders(dst http.Header, src http.Header, filter *responseheaders.CompiledHeaderFilter) {
	if dst == nil || src == nil {
		return
	}
	if filter != nil {
		responseheaders.WriteFilteredHeaders(dst, src, filter)
		return
	}
	if v := strings.TrimSpace(src.Get("Content-Type")); v != "" {
		dst.Set("Content-Type", v)
	}
	if v := strings.TrimSpace(src.Get("x-request-id")); v != "" {
		dst.Set("x-request-id", v)
	}
}

// forwardBedrock 转发请求到 AWS Bedrock
func (s *GatewayService) forwardBedrock(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *ParsedRequest,
	startTime time.Time,
) (*ForwardResult, error) {
	reqModel := parsed.Model
	reqStream := parsed.Stream
	body := parsed.Body

	region := bedrockRuntimeRegion(account)
	mappedModel, ok := ResolveBedrockModelID(account, reqModel)
	if !ok {
		return nil, fmt.Errorf("unsupported bedrock model: %s", reqModel)
	}
	if mappedModel != reqModel {
		logger.LegacyPrintf("service.gateway", "[Bedrock] Model mapping: %s -> %s (account: %s)", reqModel, mappedModel, account.Name)
	}

	betaHeader := ""
	if c != nil && c.Request != nil {
		betaHeader = c.GetHeader("anthropic-beta")
	}

	// 准备请求体（注入 anthropic_version/anthropic_beta，移除 Bedrock 不支持的字段，清理 cache_control）
	betaTokens, err := s.resolveBedrockBetaTokensForRequest(ctx, account, betaHeader, body, mappedModel)
	if err != nil {
		return nil, err
	}

	bedrockBody, err := PrepareBedrockRequestBodyWithTokens(body, mappedModel, betaTokens)
	if err != nil {
		return nil, fmt.Errorf("prepare bedrock request body: %w", err)
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	logger.LegacyPrintf("service.gateway", "[Bedrock] 命中 Bedrock 分支: account=%d name=%s model=%s->%s stream=%v",
		account.ID, account.Name, reqModel, mappedModel, reqStream)

	// 根据账号类型选择认证方式
	var signer *BedrockSigner
	var bedrockAPIKey string
	if account.IsBedrockAPIKey() {
		bedrockAPIKey = account.GetCredential("api_key")
		if bedrockAPIKey == "" {
			return nil, fmt.Errorf("api_key not found in bedrock credentials")
		}
	} else {
		signer, err = NewBedrockSignerFromAccount(account)
		if err != nil {
			return nil, fmt.Errorf("create bedrock signer: %w", err)
		}
	}

	// 执行上游请求（含重试）
	resp, err := s.executeBedrockUpstream(ctx, c, account, bedrockBody, mappedModel, region, reqStream, signer, bedrockAPIKey, proxyURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// 将 Bedrock 的 x-amzn-requestid 映射到 x-request-id，
	// 使通用错误处理函数（handleErrorResponse、handleRetryExhaustedError）能正确提取 AWS request ID。
	if awsReqID := resp.Header.Get("x-amzn-requestid"); awsReqID != "" && resp.Header.Get("x-request-id") == "" {
		resp.Header.Set("x-request-id", awsReqID)
	}

	// 错误/failover 处理
	if resp.StatusCode >= 400 {
		return s.handleBedrockUpstreamErrors(ctx, resp, c, account)
	}

	// 响应处理
	var usage *ClaudeUsage
	var firstTokenMs *int
	var clientDisconnect bool
	if reqStream {
		streamResult, err := s.handleBedrockStreamingResponse(ctx, resp, c, account, startTime, reqModel)
		if err != nil {
			return nil, err
		}
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
		clientDisconnect = streamResult.clientDisconnect
	} else {
		usage, err = s.handleBedrockNonStreamingResponse(ctx, resp, c, account)
		if err != nil {
			return nil, err
		}
	}
	if usage == nil {
		usage = &ClaudeUsage{}
	}

	return &ForwardResult{
		RequestID:        resp.Header.Get("x-amzn-requestid"),
		Usage:            *usage,
		Model:            reqModel,
		UpstreamModel:    mappedModel,
		Stream:           reqStream,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnect,
	}, nil
}

// executeBedrockUpstream 执行 Bedrock 上游请求（含重试逻辑）
func (s *GatewayService) executeBedrockUpstream(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	modelID string,
	region string,
	stream bool,
	signer *BedrockSigner,
	apiKey string,
	proxyURL string,
) (*http.Response, error) {
	var resp *http.Response
	var err error
	retryStart := time.Now()
	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		var upstreamReq *http.Request
		if account.IsBedrockAPIKey() {
			upstreamReq, err = s.buildUpstreamRequestBedrockAPIKey(ctx, body, modelID, region, stream, apiKey)
		} else {
			upstreamReq, err = s.buildUpstreamRequestBedrock(ctx, body, modelID, region, stream, signer)
		}
		if err != nil {
			return nil, err
		}

		resp, err = s.httpUpstream.DoWithTLS(upstreamReq, proxyURL, account.ID, account.Concurrency, nil)
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			safeErr := sanitizeUpstreamErrorMessage(err.Error())
			setOpsUpstreamError(c, 0, safeErr, "")
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: 0,
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "request_error",
				Message:            safeErr,
			})
			c.JSON(http.StatusBadGateway, gin.H{
				"type": "error",
				"error": gin.H{
					"type":    "upstream_error",
					"message": "Upstream request failed",
				},
			})
			return nil, fmt.Errorf("upstream request failed: %s", safeErr)
		}

		if resp.StatusCode >= 400 && resp.StatusCode != 400 && s.shouldRetryUpstreamError(account, resp.StatusCode) {
			if attempt < maxRetryAttempts {
				elapsed := time.Since(retryStart)
				if elapsed >= maxRetryElapsed {
					break
				}

				delay := retryBackoffDelay(attempt)
				remaining := maxRetryElapsed - elapsed
				if delay > remaining {
					delay = remaining
				}
				if delay <= 0 {
					break
				}

				respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
				_ = resp.Body.Close()
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
					Kind:               "retry",
					Message:            extractUpstreamErrorMessage(respBody),
					Detail: func() string {
						if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
							return truncateString(string(respBody), s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
						}
						return ""
					}(),
				})
				logger.LegacyPrintf("service.gateway", "[Bedrock] account %d: upstream error %d, retry %d/%d after %v",
					account.ID, resp.StatusCode, attempt, maxRetryAttempts, delay)
				if err := sleepWithContext(ctx, delay); err != nil {
					return nil, err
				}
				continue
			}
			break
		}

		break
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.New("upstream request failed: empty response")
	}
	return resp, nil
}

// handleBedrockUpstreamErrors 处理 Bedrock 上游 4xx/5xx 错误（failover + 错误响应）
func (s *GatewayService) handleBedrockUpstreamErrors(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
) (*ForwardResult, error) {
	// retry exhausted + failover
	if s.shouldRetryUpstreamError(account, resp.StatusCode) {
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(respBody))

			logger.LegacyPrintf("service.gateway", "[Bedrock] Upstream error (retry exhausted, failover): Account=%d(%s) Status=%d Body=%s",
				account.ID, account.Name, resp.StatusCode, truncateString(string(respBody), 1000))

			s.handleRetryExhaustedSideEffects(ctx, resp, account)
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				Kind:               "retry_exhausted_failover",
				Message:            extractUpstreamErrorMessage(respBody),
			})
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return s.handleRetryExhaustedError(ctx, resp, c, account)
	}

	// non-retryable failover
	if s.shouldFailoverUpstreamError(resp.StatusCode) {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		s.handleFailoverSideEffects(ctx, resp, account)
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			Kind:               "failover",
			Message:            extractUpstreamErrorMessage(respBody),
		})
		return nil, &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           respBody,
			RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
		}
	}

	// other errors
	return s.handleErrorResponse(ctx, resp, c, account)
}

// buildUpstreamRequestBedrock 构建 Bedrock 上游请求
func (s *GatewayService) buildUpstreamRequestBedrock(
	ctx context.Context,
	body []byte,
	modelID string,
	region string,
	stream bool,
	signer *BedrockSigner,
) (*http.Request, error) {
	targetURL := BuildBedrockURL(region, modelID, stream)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// SigV4 签名
	if err := signer.SignRequest(ctx, req, body); err != nil {
		return nil, fmt.Errorf("sign bedrock request: %w", err)
	}

	return req, nil
}

// buildUpstreamRequestBedrockAPIKey 构建 Bedrock API Key (Bearer Token) 上游请求
func (s *GatewayService) buildUpstreamRequestBedrockAPIKey(
	ctx context.Context,
	body []byte,
	modelID string,
	region string,
	stream bool,
	apiKey string,
) (*http.Request, error) {
	targetURL := BuildBedrockURL(region, modelID, stream)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	return req, nil
}

// handleBedrockNonStreamingResponse 处理 Bedrock 非流式响应
// Bedrock InvokeModel 非流式响应的 body 格式与 Claude API 兼容
func (s *GatewayService) handleBedrockNonStreamingResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
) (*ClaudeUsage, error) {
	maxBytes := resolveUpstreamResponseReadLimit(s.cfg)
	body, err := readUpstreamResponseBodyLimited(resp.Body, maxBytes)
	if err != nil {
		if errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			setOpsUpstreamError(c, http.StatusBadGateway, "upstream response too large", "")
			c.JSON(http.StatusBadGateway, gin.H{
				"type": "error",
				"error": gin.H{
					"type":    "upstream_error",
					"message": "Upstream response too large",
				},
			})
		}
		return nil, err
	}

	// 转换 Bedrock 特有的 amazon-bedrock-invocationMetrics 为标准 Anthropic usage 格式
	// 并移除该字段避免透传给客户端
	body = transformBedrockInvocationMetrics(body)

	usage := parseClaudeUsageFromResponseBody(body)

	c.Header("Content-Type", "application/json")
	if v := resp.Header.Get("x-amzn-requestid"); v != "" {
		c.Header("x-request-id", v)
	}
	c.Data(resp.StatusCode, "application/json", body)
	return usage, nil
}

func (s *GatewayService) buildUpstreamRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token, tokenType, modelID string, reqStream bool, mimicClaudeCode bool) (*http.Request, error) {
	// 确定目标URL
	targetURL := claudeAPIURL
	if account.Type == AccountTypeAPIKey {
		baseURL := account.GetBaseURL()
		if baseURL != "" {
			validatedURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, err
			}
			targetURL = validatedURL + "/v1/messages?beta=true"
		}
	} else if account.IsCustomBaseURLEnabled() {
		customURL := account.GetCustomBaseURL()
		if customURL == "" {
			return nil, fmt.Errorf("custom_base_url is enabled but not configured for account %d", account.ID)
		}
		validatedURL, err := s.validateUpstreamBaseURL(customURL)
		if err != nil {
			return nil, err
		}
		targetURL = s.buildCustomRelayURL(validatedURL, "/v1/messages", account)
	}

	clientHeaders := http.Header{}
	if c != nil && c.Request != nil {
		clientHeaders = c.Request.Header
	}

	// OAuth账号：应用统一指纹和metadata重写（受设置开关控制）
	var fingerprint *Fingerprint
	enableFP, enableMPT, enableCCH := true, false, false
	if s.settingService != nil {
		enableFP, enableMPT, enableCCH = s.settingService.GetGatewayForwardingSettings(ctx)
	}
	if account.IsOAuth() && s.identityService != nil {
		// 1. 获取或创建指纹（包含随机生成的ClientID）
		fp, err := s.identityService.GetOrCreateFingerprint(ctx, account.ID, clientHeaders)
		if err != nil {
			logger.LegacyPrintf("service.gateway", "Warning: failed to get fingerprint for account %d: %v", account.ID, err)
			// 失败时降级为透传原始headers
		} else {
			if enableFP {
				fingerprint = fp
			}

			// 2. 重写metadata.user_id（需要指纹中的ClientID和账号的account_uuid）
			// 如果启用了会话ID伪装，会在重写后替换 session 部分为固定值
			// 当 metadata 透传开启时跳过重写
			if !enableMPT {
				accountUUID := account.GetExtraString("account_uuid")
				if accountUUID != "" && fp.ClientID != "" {
					if newBody, err := s.identityService.RewriteUserIDWithMasking(ctx, body, account, accountUUID, fp.ClientID, fp.UserAgent); err == nil && len(newBody) > 0 {
						body = newBody
					}
				}
			}
		}
	}

	// 同步 billing header cc_version 与实际发送的 User-Agent 版本
	if fingerprint != nil {
		body = syncBillingHeaderVersion(body, fingerprint.UserAgent)
	}
	// CCH 签名：将 cch=00000 占位符替换为 xxHash64 签名（需在所有 body 修改之后）
	if enableCCH {
		body = signBillingHeaderCCH(body)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// 设置认证头（保持原始大小写）
	if tokenType == "oauth" {
		setHeaderRaw(req.Header, "authorization", "Bearer "+token)
	} else {
		setHeaderRaw(req.Header, "x-api-key", token)
	}

	// 白名单透传headers（恢复真实 wire casing）
	for key, values := range clientHeaders {
		lowerKey := strings.ToLower(key)
		if allowedHeaders[lowerKey] {
			wireKey := resolveWireCasing(key)
			for _, v := range values {
				addHeaderRaw(req.Header, wireKey, v)
			}
		}
	}

	// OAuth账号：应用缓存的指纹到请求头（覆盖白名单透传的头）
	if fingerprint != nil {
		s.identityService.ApplyFingerprint(req, fingerprint)
	}

	// 确保必要的headers存在（保持原始大小写）
	if getHeaderRaw(req.Header, "content-type") == "" {
		setHeaderRaw(req.Header, "content-type", "application/json")
	}
	if getHeaderRaw(req.Header, "anthropic-version") == "" {
		setHeaderRaw(req.Header, "anthropic-version", "2023-06-01")
	}
	if tokenType == "oauth" {
		applyClaudeOAuthHeaderDefaults(req)
	}

	// Build effective drop set: merge static defaults with dynamic beta policy filter rules
	policyFilterSet := s.getBetaPolicyFilterSet(ctx, c, account, modelID)
	effectiveDropSet := mergeDropSets(policyFilterSet)

	// 处理 anthropic-beta header（OAuth 账号需要包含 oauth beta）
	if tokenType == "oauth" {
		if mimicClaudeCode {
			// 非 Claude Code 客户端：按 opencode 的策略处理：
			// - 强制 Claude Code 指纹相关请求头（尤其是 user-agent/x-stainless/x-app）
			// - 保留 incoming beta 的同时，确保 OAuth 所需 beta 存在
			applyClaudeCodeMimicHeaders(req, reqStream)

			incomingBeta := getHeaderRaw(req.Header, "anthropic-beta")
			// Claude Code OAuth credentials are scoped to Claude Code.
			// Non-haiku models MUST include claude-code beta for Anthropic to recognize
			// this as a legitimate Claude Code request; without it, the request is
			// rejected as third-party ("out of extra usage").
			// Haiku models are exempt from third-party detection and don't need it.
			requiredBetas := []string{claude.BetaOAuth, claude.BetaInterleavedThinking}
			if !strings.Contains(strings.ToLower(modelID), "haiku") {
				requiredBetas = []string{claude.BetaClaudeCode, claude.BetaOAuth, claude.BetaInterleavedThinking}
			}
			setHeaderRaw(req.Header, "anthropic-beta", mergeAnthropicBetaDropping(requiredBetas, incomingBeta, effectiveDropSet))
		} else {
			// Claude Code 客户端：尽量透传原始 header，仅补齐 oauth beta
			clientBetaHeader := getHeaderRaw(req.Header, "anthropic-beta")
			setHeaderRaw(req.Header, "anthropic-beta", stripBetaTokensWithSet(s.getBetaHeader(modelID, clientBetaHeader), effectiveDropSet))
		}
	} else {
		// API-key accounts: apply beta policy filter to strip controlled tokens
		if existingBeta := getHeaderRaw(req.Header, "anthropic-beta"); existingBeta != "" {
			setHeaderRaw(req.Header, "anthropic-beta", stripBetaTokensWithSet(existingBeta, effectiveDropSet))
		} else if s.cfg != nil && s.cfg.Gateway.InjectBetaForAPIKey {
			// API-key：仅在请求显式使用 beta 特性且客户端未提供时，按需补齐（默认关闭）
			if requestNeedsBetaFeatures(body) {
				if beta := defaultAPIKeyBetaHeader(body); beta != "" {
					setHeaderRaw(req.Header, "anthropic-beta", beta)
				}
			}
		}
	}

	// 同步 X-Claude-Code-Session-Id 头：取 body 中已处理的 metadata.user_id 的 session_id 覆盖
	if sessionHeader := getHeaderRaw(req.Header, "X-Claude-Code-Session-Id"); sessionHeader != "" {
		if uid := gjson.GetBytes(body, "metadata.user_id").String(); uid != "" {
			if parsed := ParseMetadataUserID(uid); parsed != nil {
				setHeaderRaw(req.Header, "X-Claude-Code-Session-Id", parsed.SessionID)
			}
		}
	}

	// === DEBUG: 打印上游转发请求（headers + body 摘要），与 CLIENT_ORIGINAL 对比 ===
	s.debugLogGatewaySnapshot("UPSTREAM_FORWARD", req.Header, body, map[string]string{
		"url":                 req.URL.String(),
		"token_type":          tokenType,
		"mimic_claude_code":   strconv.FormatBool(mimicClaudeCode),
		"fingerprint_applied": strconv.FormatBool(fingerprint != nil),
		"enable_fp":           strconv.FormatBool(enableFP),
		"enable_mpt":          strconv.FormatBool(enableMPT),
	})

	// Always capture a compact fingerprint line for later error diagnostics.
	// We only print it when needed (or when the explicit debug flag is enabled).
	if c != nil && tokenType == "oauth" {
		c.Set(claudeMimicDebugInfoKey, buildClaudeMimicDebugLine(req, body, account, tokenType, mimicClaudeCode))
	}
	if s.debugClaudeMimicEnabled() {
		logClaudeMimicDebug(req, body, account, tokenType, mimicClaudeCode)
	}

	return req, nil
}

// getBetaHeader 处理anthropic-beta header
// 对于OAuth账号，需要确保包含oauth-2025-04-20
func (s *GatewayService) getBetaHeader(modelID string, clientBetaHeader string) string {
	// 如果客户端传了anthropic-beta
	if clientBetaHeader != "" {
		// 已包含oauth beta则直接返回
		if strings.Contains(clientBetaHeader, claude.BetaOAuth) {
			return clientBetaHeader
		}

		// 需要添加oauth beta
		parts := strings.Split(clientBetaHeader, ",")
		for i, p := range parts {
			parts[i] = strings.TrimSpace(p)
		}

		// 在claude-code-20250219后面插入oauth beta
		claudeCodeIdx := -1
		for i, p := range parts {
			if p == claude.BetaClaudeCode {
				claudeCodeIdx = i
				break
			}
		}

		if claudeCodeIdx >= 0 {
			// 在claude-code后面插入
			newParts := make([]string, 0, len(parts)+1)
			newParts = append(newParts, parts[:claudeCodeIdx+1]...)
			newParts = append(newParts, claude.BetaOAuth)
			newParts = append(newParts, parts[claudeCodeIdx+1:]...)
			return strings.Join(newParts, ",")
		}

		// 没有claude-code，放在第一位
		return claude.BetaOAuth + "," + clientBetaHeader
	}

	// 客户端没传，根据模型生成
	// haiku 模型不需要 claude-code beta
	if strings.Contains(strings.ToLower(modelID), "haiku") {
		return claude.HaikuBetaHeader
	}

	return claude.DefaultBetaHeader
}

func requestNeedsBetaFeatures(body []byte) bool {
	tools := gjson.GetBytes(body, "tools")
	if tools.Exists() && tools.IsArray() && len(tools.Array()) > 0 {
		return true
	}
	thinkingType := gjson.GetBytes(body, "thinking.type").String()
	if strings.EqualFold(thinkingType, "enabled") || strings.EqualFold(thinkingType, "adaptive") {
		return true
	}
	return false
}

func defaultAPIKeyBetaHeader(body []byte) string {
	modelID := gjson.GetBytes(body, "model").String()
	if strings.Contains(strings.ToLower(modelID), "haiku") {
		return claude.APIKeyHaikuBetaHeader
	}
	return claude.APIKeyBetaHeader
}

func applyClaudeOAuthHeaderDefaults(req *http.Request) {
	if req == nil {
		return
	}
	if getHeaderRaw(req.Header, "Accept") == "" {
		setHeaderRaw(req.Header, "Accept", "application/json")
	}
	for key, value := range claude.DefaultHeaders {
		if value == "" {
			continue
		}
		if getHeaderRaw(req.Header, key) == "" {
			setHeaderRaw(req.Header, resolveWireCasing(key), value)
		}
	}
}

func mergeAnthropicBeta(required []string, incoming string) string {
	seen := make(map[string]struct{}, len(required)+8)
	out := make([]string, 0, len(required)+8)

	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}

	for _, r := range required {
		add(r)
	}
	for _, p := range strings.Split(incoming, ",") {
		add(p)
	}
	return strings.Join(out, ",")
}

func mergeAnthropicBetaDropping(required []string, incoming string, drop map[string]struct{}) string {
	merged := mergeAnthropicBeta(required, incoming)
	if merged == "" || len(drop) == 0 {
		return merged
	}
	out := make([]string, 0, 8)
	for _, p := range strings.Split(merged, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := drop[p]; ok {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, ",")
}

// stripBetaTokens removes the given beta tokens from a comma-separated header value.
func stripBetaTokens(header string, tokens []string) string {
	if header == "" || len(tokens) == 0 {
		return header
	}
	return stripBetaTokensWithSet(header, buildBetaTokenSet(tokens))
}

func stripBetaTokensWithSet(header string, drop map[string]struct{}) string {
	if header == "" || len(drop) == 0 {
		return header
	}
	parts := strings.Split(header, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := drop[p]; ok {
			continue
		}
		out = append(out, p)
	}
	if len(out) == len(parts) {
		return header // no change, avoid allocation
	}
	return strings.Join(out, ",")
}

// BetaBlockedError indicates a request was blocked by a beta policy rule.
type BetaBlockedError struct {
	Message string
}

func (e *BetaBlockedError) Error() string { return e.Message }

// betaPolicyResult holds the evaluated result of beta policy rules for a single request.
type betaPolicyResult struct {
	blockErr  *BetaBlockedError   // non-nil if a block rule matched
	filterSet map[string]struct{} // tokens to filter (may be nil)
}

// evaluateBetaPolicy loads settings once and evaluates all rules against the given request.
func (s *GatewayService) evaluateBetaPolicy(ctx context.Context, betaHeader string, account *Account, model string) betaPolicyResult {
	if s.settingService == nil {
		return betaPolicyResult{}
	}
	settings, err := s.settingService.GetBetaPolicySettings(ctx)
	if err != nil || settings == nil {
		return betaPolicyResult{}
	}
	isOAuth := account.IsOAuth()
	isBedrock := account.IsBedrock()
	var result betaPolicyResult
	for _, rule := range settings.Rules {
		if !betaPolicyScopeMatches(rule.Scope, isOAuth, isBedrock) {
			continue
		}
		effectiveAction, effectiveErrMsg := resolveRuleAction(rule, model)
		switch effectiveAction {
		case BetaPolicyActionBlock:
			if result.blockErr == nil && betaHeader != "" && containsBetaToken(betaHeader, rule.BetaToken) {
				msg := effectiveErrMsg
				if msg == "" {
					msg = "beta feature " + rule.BetaToken + " is not allowed"
				}
				result.blockErr = &BetaBlockedError{Message: msg}
			}
		case BetaPolicyActionFilter:
			if result.filterSet == nil {
				result.filterSet = make(map[string]struct{})
			}
			result.filterSet[rule.BetaToken] = struct{}{}
		}
	}
	return result
}

// mergeDropSets merges the static defaultDroppedBetasSet with dynamic policy filter tokens.
// Returns defaultDroppedBetasSet directly when policySet is empty (zero allocation).
func mergeDropSets(policySet map[string]struct{}, extra ...string) map[string]struct{} {
	if len(policySet) == 0 && len(extra) == 0 {
		return defaultDroppedBetasSet
	}
	m := make(map[string]struct{}, len(defaultDroppedBetasSet)+len(policySet)+len(extra))
	for t := range defaultDroppedBetasSet {
		m[t] = struct{}{}
	}
	for t := range policySet {
		m[t] = struct{}{}
	}
	for _, t := range extra {
		m[t] = struct{}{}
	}
	return m
}

// betaPolicyFilterSetKey is the gin.Context key for caching the policy filter set within a request.
const betaPolicyFilterSetKey = "betaPolicyFilterSet"

// getBetaPolicyFilterSet returns the beta policy filter set, using the gin context cache if available.
// In the /v1/messages path, Forward() evaluates the policy first and caches the result;
// buildUpstreamRequest reuses it (zero extra DB calls). In the count_tokens path, this
// evaluates on demand (one DB call).
func (s *GatewayService) getBetaPolicyFilterSet(ctx context.Context, c *gin.Context, account *Account, model string) map[string]struct{} {
	if c != nil {
		if v, ok := c.Get(betaPolicyFilterSetKey); ok {
			if fs, ok := v.(map[string]struct{}); ok {
				return fs
			}
		}
	}
	return s.evaluateBetaPolicy(ctx, "", account, model).filterSet
}

// betaPolicyScopeMatches checks whether a rule's scope matches the current account type.
func betaPolicyScopeMatches(scope string, isOAuth bool, isBedrock bool) bool {
	switch scope {
	case BetaPolicyScopeAll:
		return true
	case BetaPolicyScopeOAuth:
		return isOAuth
	case BetaPolicyScopeAPIKey:
		return !isOAuth && !isBedrock
	case BetaPolicyScopeBedrock:
		return isBedrock
	default:
		return true // unknown scope → match all (fail-open)
	}
}

// matchModelWhitelist checks if a model matches any pattern in the whitelist.
// Reuses matchModelPattern from group.go which supports exact and wildcard prefix matching.
func matchModelWhitelist(model string, whitelist []string) bool {
	for _, pattern := range whitelist {
		if matchModelPattern(pattern, model) {
			return true
		}
	}
	return false
}

// resolveRuleAction determines the effective action and error message for a rule given the request model.
// When ModelWhitelist is empty, the rule's primary Action/ErrorMessage applies unconditionally.
// When non-empty, Action applies to matching models; FallbackAction/FallbackErrorMessage applies to others.
func resolveRuleAction(rule BetaPolicyRule, model string) (action, errorMessage string) {
	if len(rule.ModelWhitelist) == 0 {
		return rule.Action, rule.ErrorMessage
	}
	if matchModelWhitelist(model, rule.ModelWhitelist) {
		return rule.Action, rule.ErrorMessage
	}
	if rule.FallbackAction != "" {
		return rule.FallbackAction, rule.FallbackErrorMessage
	}
	return BetaPolicyActionPass, "" // default fallback: pass (fail-open)
}

// droppedBetaSet returns claude.DroppedBetas as a set, with optional extra tokens.
func droppedBetaSet(extra ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(defaultDroppedBetasSet)+len(extra))
	for t := range defaultDroppedBetasSet {
		m[t] = struct{}{}
	}
	for _, t := range extra {
		m[t] = struct{}{}
	}
	return m
}

// containsBetaToken checks if a comma-separated header value contains the given token.
func containsBetaToken(header, token string) bool {
	if header == "" || token == "" {
		return false
	}
	for _, p := range strings.Split(header, ",") {
		if strings.TrimSpace(p) == token {
			return true
		}
	}
	return false
}

func filterBetaTokens(tokens []string, filterSet map[string]struct{}) []string {
	if len(tokens) == 0 || len(filterSet) == 0 {
		return tokens
	}
	kept := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if _, filtered := filterSet[token]; !filtered {
			kept = append(kept, token)
		}
	}
	return kept
}

func (s *GatewayService) resolveBedrockBetaTokensForRequest(
	ctx context.Context,
	account *Account,
	betaHeader string,
	body []byte,
	modelID string,
) ([]string, error) {
	// 1. 对原始 header 中的 beta token 做 block 检查（快速失败）
	policy := s.evaluateBetaPolicy(ctx, betaHeader, account, modelID)
	if policy.blockErr != nil {
		return nil, policy.blockErr
	}

	// 2. 解析 header + body 自动注入 + Bedrock 转换/过滤
	betaTokens := ResolveBedrockBetaTokens(betaHeader, body, modelID)

	// 3. 对最终 token 列表再做 block 检查，捕获通过 body 自动注入绕过 header block 的情况。
	//    例如：管理员 block 了 interleaved-thinking，客户端不在 header 中带该 token，
	//    但请求体中包含 thinking 字段 → autoInjectBedrockBetaTokens 会自动补齐 →
	//    如果不做此检查，block 规则会被绕过。
	if blockErr := s.checkBetaPolicyBlockForTokens(ctx, betaTokens, account, modelID); blockErr != nil {
		return nil, blockErr
	}

	return filterBetaTokens(betaTokens, policy.filterSet), nil
}

// checkBetaPolicyBlockForTokens 检查 token 列表中是否有被管理员 block 规则命中的 token。
// 用于补充 evaluateBetaPolicy 对 header 的检查，覆盖 body 自动注入的 token。
func (s *GatewayService) checkBetaPolicyBlockForTokens(ctx context.Context, tokens []string, account *Account, model string) *BetaBlockedError {
	if s.settingService == nil || len(tokens) == 0 {
		return nil
	}
	settings, err := s.settingService.GetBetaPolicySettings(ctx)
	if err != nil || settings == nil {
		return nil
	}
	isOAuth := account.IsOAuth()
	isBedrock := account.IsBedrock()
	tokenSet := buildBetaTokenSet(tokens)
	for _, rule := range settings.Rules {
		effectiveAction, effectiveErrMsg := resolveRuleAction(rule, model)
		if effectiveAction != BetaPolicyActionBlock {
			continue
		}
		if !betaPolicyScopeMatches(rule.Scope, isOAuth, isBedrock) {
			continue
		}
		if _, present := tokenSet[rule.BetaToken]; present {
			msg := effectiveErrMsg
			if msg == "" {
				msg = "beta feature " + rule.BetaToken + " is not allowed"
			}
			return &BetaBlockedError{Message: msg}
		}
	}
	return nil
}

func buildBetaTokenSet(tokens []string) map[string]struct{} {
	m := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		if t == "" {
			continue
		}
		m[t] = struct{}{}
	}
	return m
}

var defaultDroppedBetasSet = buildBetaTokenSet(claude.DroppedBetas)

// applyClaudeCodeMimicHeaders forces "Claude Code-like" request headers.
// This mirrors opencode-anthropic-auth behavior: do not trust downstream
// headers when using Claude Code-scoped OAuth credentials.
func applyClaudeCodeMimicHeaders(req *http.Request, isStream bool) {
	if req == nil {
		return
	}
	// Start with the standard defaults (fill missing).
	applyClaudeOAuthHeaderDefaults(req)
	// Then force key headers to match Claude Code fingerprint regardless of what the client sent.
	// 使用 resolveWireCasing 确保 key 与真实 wire format 一致（如 "x-app" 而非 "X-App"）
	for key, value := range claude.DefaultHeaders {
		if value == "" {
			continue
		}
		setHeaderRaw(req.Header, resolveWireCasing(key), value)
	}
	// Real Claude CLI uses Accept: application/json (even for streaming).
	setHeaderRaw(req.Header, "Accept", "application/json")
	if isStream {
		setHeaderRaw(req.Header, "x-stainless-helper-method", "stream")
	}
}

func truncateForLog(b []byte, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = 2048
	}
	if len(b) > maxBytes {
		b = b[:maxBytes]
	}
	s := string(b)
	// 保持一行，避免污染日志格式
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}

// shouldRectifySignatureError 统一判断是否应触发签名整流（strip thinking blocks 并重试）。
// 根据账号类型检查对应的开关和匹配模式。
func (s *GatewayService) shouldRectifySignatureError(ctx context.Context, account *Account, respBody []byte) bool {
	if account.Type == AccountTypeAPIKey {
		// API Key 账号：独立开关，一次读取配置
		settings, err := s.settingService.GetRectifierSettings(ctx)
		if err != nil || !settings.Enabled || !settings.APIKeySignatureEnabled {
			return false
		}
		// 先检查内置模式（同 OAuth），再检查自定义关键词
		if s.isThinkingBlockSignatureError(respBody) {
			return true
		}
		return matchSignaturePatterns(respBody, settings.APIKeySignaturePatterns)
	}
	// OAuth/SetupToken/Upstream/Bedrock 等：保持原有行为（内置模式 + 原开关）
	return s.isThinkingBlockSignatureError(respBody) && s.settingService.IsSignatureRectifierEnabled(ctx)
}

// isSignatureErrorPattern 仅做模式匹配，不检查开关。
// 用于已进入重试流程后的二阶段检测（此时开关已在首次调用时验证过）。
func (s *GatewayService) isSignatureErrorPattern(ctx context.Context, account *Account, respBody []byte) bool {
	if s.isThinkingBlockSignatureError(respBody) {
		return true
	}
	if account.Type == AccountTypeAPIKey {
		settings, err := s.settingService.GetRectifierSettings(ctx)
		if err != nil {
			return false
		}
		return matchSignaturePatterns(respBody, settings.APIKeySignaturePatterns)
	}
	return false
}

// matchSignaturePatterns 检查响应体是否匹配自定义关键词列表（不区分大小写）。
func matchSignaturePatterns(respBody []byte, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	bodyLower := strings.ToLower(string(respBody))
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(bodyLower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// isThinkingBlockSignatureError 检测是否是thinking block相关错误
// 这类错误可以通过过滤thinking blocks并重试来解决
func (s *GatewayService) isThinkingBlockSignatureError(respBody []byte) bool {
	msg := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
	if msg == "" {
		return false
	}

	// 检测signature相关的错误（更宽松的匹配）
	// 例如: "Invalid `signature` in `thinking` block", "***.signature" 等
	if strings.Contains(msg, "signature") {
		return true
	}

	// 检测 thinking block 顺序/类型错误
	// 例如: "Expected `thinking` or `redacted_thinking`, but found `text`"
	if strings.Contains(msg, "expected") && (strings.Contains(msg, "thinking") || strings.Contains(msg, "redacted_thinking")) {
		logger.LegacyPrintf("service.gateway", "[SignatureCheck] Detected thinking block type error")
		return true
	}

	// 检测 thinking block 被修改的错误
	// 例如: "thinking or redacted_thinking blocks in the latest assistant message cannot be modified"
	if strings.Contains(msg, "cannot be modified") && (strings.Contains(msg, "thinking") || strings.Contains(msg, "redacted_thinking")) {
		logger.LegacyPrintf("service.gateway", "[SignatureCheck] Detected thinking block modification error")
		return true
	}

	// 检测空消息内容错误（可能是过滤 thinking blocks 后导致的，或客户端发送了空 text block）
	// 例如: "all messages must have non-empty content"
	//       "messages: text content blocks must be non-empty"
	if strings.Contains(msg, "non-empty content") || strings.Contains(msg, "empty content") ||
		strings.Contains(msg, "content blocks must be non-empty") {
		logger.LegacyPrintf("service.gateway", "[SignatureCheck] Detected empty content error")
		return true
	}

	return false
}

func (s *GatewayService) shouldFailoverOn400(respBody []byte) bool {
	// 只对"可能是兼容性差异导致"的 400 允许切换，避免无意义重试。
	// 默认保守：无法识别则不切换。
	msg := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
	if msg == "" {
		return false
	}

	// 缺少/错误的 beta header：换账号/链路可能成功（尤其是混合调度时）。
	// 更精确匹配 beta 相关的兼容性问题，避免误触发切换。
	if strings.Contains(msg, "anthropic-beta") ||
		strings.Contains(msg, "beta feature") ||
		strings.Contains(msg, "requires beta") {
		return true
	}

	// thinking/tool streaming 等兼容性约束（常见于中间转换链路）
	if strings.Contains(msg, "thinking") || strings.Contains(msg, "thought_signature") || strings.Contains(msg, "signature") {
		return true
	}
	if strings.Contains(msg, "tool_use") || strings.Contains(msg, "tool_result") || strings.Contains(msg, "tools") {
		return true
	}

	return false
}

// ExtractUpstreamErrorMessage 从上游响应体中提取错误消息
// 支持 Claude 风格的错误格式：{"type":"error","error":{"type":"...","message":"..."}}
func ExtractUpstreamErrorMessage(body []byte) string {
	return extractUpstreamErrorMessage(body)
}

func extractUpstreamErrorMessage(body []byte) string {
	// Claude 风格：{"type":"error","error":{"type":"...","message":"..."}}
	if m := gjson.GetBytes(body, "error.message").String(); strings.TrimSpace(m) != "" {
		inner := strings.TrimSpace(m)
		// 有些上游会把完整 JSON 作为字符串塞进 message
		if strings.HasPrefix(inner, "{") {
			if innerMsg := gjson.Get(inner, "error.message").String(); strings.TrimSpace(innerMsg) != "" {
				return innerMsg
			}
		}
		return m
	}

	// ChatGPT 内部 API 风格：{"detail":"..."}
	if d := gjson.GetBytes(body, "detail").String(); strings.TrimSpace(d) != "" {
		return d
	}

	// 兜底：尝试顶层 message
	return gjson.GetBytes(body, "message").String()
}

func extractUpstreamErrorCode(body []byte) string {
	if code := strings.TrimSpace(gjson.GetBytes(body, "error.code").String()); code != "" {
		return code
	}

	inner := strings.TrimSpace(gjson.GetBytes(body, "error.message").String())
	if !strings.HasPrefix(inner, "{") {
		return ""
	}

	if code := strings.TrimSpace(gjson.Get(inner, "error.code").String()); code != "" {
		return code
	}

	if lastBrace := strings.LastIndex(inner, "}"); lastBrace >= 0 {
		if code := strings.TrimSpace(gjson.Get(inner[:lastBrace+1], "error.code").String()); code != "" {
			return code
		}
	}

	return ""
}

func isCountTokensUnsupported404(statusCode int, body []byte) bool {
	if statusCode != http.StatusNotFound {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	if msg == "" {
		return false
	}
	if strings.Contains(msg, "/v1/messages/count_tokens") {
		return true
	}
	return strings.Contains(msg, "count_tokens") && strings.Contains(msg, "not found")
}

func (s *GatewayService) handleErrorResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account) (*ForwardResult, error) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))

	// 调试日志：打印上游错误响应
	logger.LegacyPrintf("service.gateway", "[Forward] Upstream error (non-retryable): Account=%d(%s) Status=%d RequestID=%s Body=%s",
		account.ID, account.Name, resp.StatusCode, resp.Header.Get("x-request-id"), truncateString(string(body), 1000))

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(body))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)

	// Print a compact upstream request fingerprint when we hit the Claude Code OAuth
	// credential scope error. This avoids requiring env-var tweaks in a fixed deploy.
	if isClaudeCodeCredentialScopeError(upstreamMsg) && c != nil {
		if v, ok := c.Get(claudeMimicDebugInfoKey); ok {
			if line, ok := v.(string); ok && strings.TrimSpace(line) != "" {
				logger.LegacyPrintf("service.gateway", "[ClaudeMimicDebugOnError] status=%d request_id=%s %s",
					resp.StatusCode,
					resp.Header.Get("x-request-id"),
					line,
				)
			}
		}
	}

	// Enrich Ops error logs with upstream status + message, and optionally a truncated body snippet.
	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  resp.Header.Get("x-request-id"),
		Kind:               "http_error",
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})

	// 处理上游错误，标记账号状态
	shouldDisable := false
	if s.rateLimitService != nil {
		shouldDisable = s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
	}
	if shouldDisable {
		return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: body}
	}

	// 记录上游错误响应体摘要便于排障（可选：由配置控制；不回显到客户端）
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		logger.LegacyPrintf("service.gateway",
			"Upstream error %d (account=%d platform=%s type=%s): %s",
			resp.StatusCode,
			account.ID,
			account.Platform,
			account.Type,
			truncateForLog(body, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes),
		)
	}

	// 非 failover 错误也支持错误透传规则匹配。
	if status, errType, errMsg, matched := applyErrorPassthroughRule(
		c,
		account.Platform,
		resp.StatusCode,
		body,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	); matched {
		c.JSON(status, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    errType,
				"message": errMsg,
			},
		})

		summary := upstreamMsg
		if summary == "" {
			summary = errMsg
		}
		if summary == "" {
			return nil, fmt.Errorf("upstream error: %d (passthrough rule matched)", resp.StatusCode)
		}
		return nil, fmt.Errorf("upstream error: %d (passthrough rule matched) message=%s", resp.StatusCode, summary)
	}

	// 根据状态码返回适当的自定义错误响应（不透传上游详细信息）
	var errType, errMsg string
	var statusCode int

	switch resp.StatusCode {
	case 400:
		c.Data(http.StatusBadRequest, "application/json", body)
		summary := upstreamMsg
		if summary == "" {
			summary = truncateForLog(body, 512)
		}
		if summary == "" {
			return nil, fmt.Errorf("upstream error: %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, summary)
	case 401:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream authentication failed, please contact administrator"
	case 403:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream access forbidden, please contact administrator"
	case 429:
		statusCode = http.StatusTooManyRequests
		errType = "rate_limit_error"
		errMsg = "Upstream rate limit exceeded, please retry later"
	case 529:
		statusCode = http.StatusServiceUnavailable
		errType = "overloaded_error"
		errMsg = "Upstream service overloaded, please retry later"
	case 500, 502, 503, 504:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream service temporarily unavailable"
	default:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream request failed"
	}

	// 返回自定义错误响应
	c.JSON(statusCode, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": errMsg,
		},
	})

	if upstreamMsg == "" {
		return nil, fmt.Errorf("upstream error: %d", resp.StatusCode)
	}
	return nil, fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
}

func (s *GatewayService) handleRetryExhaustedSideEffects(ctx context.Context, resp *http.Response, account *Account) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	statusCode := resp.StatusCode

	// OAuth/Setup Token 账号的 403：标记账号异常
	if account.IsOAuth() && statusCode == 403 {
		s.rateLimitService.HandleUpstreamError(ctx, account, statusCode, resp.Header, body)
		logger.LegacyPrintf("service.gateway", "Account %d: marked as error after %d retries for status %d", account.ID, maxRetryAttempts, statusCode)
	} else {
		// API Key 未配置错误码：不标记账号状态
		logger.LegacyPrintf("service.gateway", "Account %d: upstream error %d after %d retries (not marking account)", account.ID, statusCode, maxRetryAttempts)
	}
}

func (s *GatewayService) handleFailoverSideEffects(ctx context.Context, resp *http.Response, account *Account) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
}

// handleRetryExhaustedError 处理重试耗尽后的错误
// OAuth 403：标记账号异常
// API Key 未配置错误码：仅返回错误，不标记账号
func (s *GatewayService) handleRetryExhaustedError(ctx context.Context, resp *http.Response, c *gin.Context, account *Account) (*ForwardResult, error) {
	// Capture upstream error body before side-effects consume the stream.
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(respBody))

	s.handleRetryExhaustedSideEffects(ctx, resp, account)

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)

	if isClaudeCodeCredentialScopeError(upstreamMsg) && c != nil {
		if v, ok := c.Get(claudeMimicDebugInfoKey); ok {
			if line, ok := v.(string); ok && strings.TrimSpace(line) != "" {
				logger.LegacyPrintf("service.gateway", "[ClaudeMimicDebugOnError] status=%d request_id=%s %s",
					resp.StatusCode,
					resp.Header.Get("x-request-id"),
					line,
				)
			}
		}
	}

	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(respBody), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  resp.Header.Get("x-request-id"),
		Kind:               "retry_exhausted",
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})

	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		logger.LegacyPrintf("service.gateway",
			"Upstream error %d retries_exhausted (account=%d platform=%s type=%s): %s",
			resp.StatusCode,
			account.ID,
			account.Platform,
			account.Type,
			truncateForLog(respBody, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes),
		)
	}

	if status, errType, errMsg, matched := applyErrorPassthroughRule(
		c,
		account.Platform,
		resp.StatusCode,
		respBody,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed after retries",
	); matched {
		c.JSON(status, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    errType,
				"message": errMsg,
			},
		})

		summary := upstreamMsg
		if summary == "" {
			summary = errMsg
		}
		if summary == "" {
			return nil, fmt.Errorf("upstream error: %d (retries exhausted, passthrough rule matched)", resp.StatusCode)
		}
		return nil, fmt.Errorf("upstream error: %d (retries exhausted, passthrough rule matched) message=%s", resp.StatusCode, summary)
	}

	// 返回统一的重试耗尽错误响应
	c.JSON(http.StatusBadGateway, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    "upstream_error",
			"message": "Upstream request failed after retries",
		},
	})

	if upstreamMsg == "" {
		return nil, fmt.Errorf("upstream error: %d (retries exhausted)", resp.StatusCode)
	}
	return nil, fmt.Errorf("upstream error: %d (retries exhausted) message=%s", resp.StatusCode, upstreamMsg)
}

// streamingResult 流式响应结果
type streamingResult struct {
	usage            *ClaudeUsage
	firstTokenMs     *int
	clientDisconnect bool // 客户端是否在流式传输过程中断开
}

func (s *GatewayService) handleStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, startTime time.Time, originalModel, mappedModel string, mimicClaudeCode bool) (*streamingResult, error) {
	// 更新5h窗口状态
	s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}

	// 设置SSE响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// 透传其他响应头
	if v := resp.Header.Get("x-request-id"); v != "" {
		c.Header("x-request-id", v)
	}

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	usage := &ClaudeUsage{}
	var firstTokenMs *int
	scanner := bufio.NewScanner(resp.Body)
	// 设置更大的buffer以处理长行
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanBuf := getSSEScannerBuf64K()
	scanner.Buffer(scanBuf[:0], maxLineSize)

	type scanEvent struct {
		line string
		err  error
	}
	// 独立 goroutine 读取上游，避免读取阻塞导致超时/keepalive无法处理
	events := make(chan scanEvent, 16)
	done := make(chan struct{})
	sendEvent := func(ev scanEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
		}
	}
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	go func(scanBuf *sseScannerBuf64K) {
		defer putSSEScannerBuf64K(scanBuf)
		defer close(events)
		for scanner.Scan() {
			atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			if !sendEvent(scanEvent{line: scanner.Text()}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			_ = sendEvent(scanEvent{err: err})
		}
	}(scanBuf)
	defer close(done)

	streamInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	// 仅监控上游数据间隔超时，避免下游写入阻塞导致误判
	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
	}
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
	}

	// 下游 keepalive：防止代理/Cloudflare Tunnel 因连接空闲而断开
	keepaliveInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamKeepaliveInterval > 0 {
		keepaliveInterval = time.Duration(s.cfg.Gateway.StreamKeepaliveInterval) * time.Second
	}
	var keepaliveTicker *time.Ticker
	if keepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(keepaliveInterval)
		defer keepaliveTicker.Stop()
	}
	var keepaliveCh <-chan time.Time
	if keepaliveTicker != nil {
		keepaliveCh = keepaliveTicker.C
	}
	lastDataAt := time.Now()

	// 仅发送一次错误事件，避免多次写入导致协议混乱（写失败时尽力通知客户端）
	errorEventSent := false
	sendErrorEvent := func(reason string) {
		if errorEventSent {
			return
		}
		errorEventSent = true
		_, _ = fmt.Fprintf(w, "event: error\ndata: {\"error\":\"%s\"}\n\n", reason)
		flusher.Flush()
	}

	needModelReplace := originalModel != mappedModel
	clientDisconnected := false // 客户端断开标志，断开后继续读取上游以获取完整usage
	sawTerminalEvent := false

	pendingEventLines := make([]string, 0, 4)

	processSSEEvent := func(lines []string) ([]string, string, *sseUsagePatch, error) {
		if len(lines) == 0 {
			return nil, "", nil, nil
		}

		eventName := ""
		dataLine := ""
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "event:") {
				eventName = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
				continue
			}
			if dataLine == "" && sseDataRe.MatchString(trimmed) {
				dataLine = sseDataRe.ReplaceAllString(trimmed, "")
			}
		}

		if eventName == "error" {
			return nil, dataLine, nil, errors.New("have error in stream")
		}

		if dataLine == "" {
			return []string{strings.Join(lines, "\n") + "\n\n"}, "", nil, nil
		}

		if dataLine == "[DONE]" {
			sawTerminalEvent = true
			block := ""
			if eventName != "" {
				block = "event: " + eventName + "\n"
			}
			block += "data: " + dataLine + "\n\n"
			return []string{block}, dataLine, nil, nil
		}

		var event map[string]any
		if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
			// JSON 解析失败，直接透传原始数据
			block := ""
			if eventName != "" {
				block = "event: " + eventName + "\n"
			}
			block += "data: " + dataLine + "\n\n"
			return []string{block}, dataLine, nil, nil
		}

		eventType, _ := event["type"].(string)
		if eventName == "" {
			eventName = eventType
		}
		eventChanged := false

		// 兼容 Kimi cached_tokens → cache_read_input_tokens
		if eventType == "message_start" {
			if msg, ok := event["message"].(map[string]any); ok {
				if u, ok := msg["usage"].(map[string]any); ok {
					eventChanged = reconcileCachedTokens(u) || eventChanged
				}
			}
		}
		if eventType == "message_delta" {
			if u, ok := event["usage"].(map[string]any); ok {
				eventChanged = reconcileCachedTokens(u) || eventChanged
			}
		}

		// Cache TTL Override: 重写 SSE 事件中的 cache_creation 分类
		if account.IsCacheTTLOverrideEnabled() {
			overrideTarget := account.GetCacheTTLOverrideTarget()
			if eventType == "message_start" {
				if msg, ok := event["message"].(map[string]any); ok {
					if u, ok := msg["usage"].(map[string]any); ok {
						eventChanged = rewriteCacheCreationJSON(u, overrideTarget) || eventChanged
					}
				}
			}
			if eventType == "message_delta" {
				if u, ok := event["usage"].(map[string]any); ok {
					eventChanged = rewriteCacheCreationJSON(u, overrideTarget) || eventChanged
				}
			}
		}

		if needModelReplace {
			if msg, ok := event["message"].(map[string]any); ok {
				if model, ok := msg["model"].(string); ok && model == mappedModel {
					msg["model"] = originalModel
					eventChanged = true
				}
			}
		}

		usagePatch := s.extractSSEUsagePatch(event)
		if anthropicStreamEventIsTerminal(eventName, dataLine) {
			sawTerminalEvent = true
		}
		if !eventChanged {
			block := ""
			if eventName != "" {
				block = "event: " + eventName + "\n"
			}
			block += "data: " + dataLine + "\n\n"
			return []string{block}, dataLine, usagePatch, nil
		}

		newData, err := json.Marshal(event)
		if err != nil {
			// 序列化失败，直接透传原始数据
			block := ""
			if eventName != "" {
				block = "event: " + eventName + "\n"
			}
			block += "data: " + dataLine + "\n\n"
			return []string{block}, dataLine, usagePatch, nil
		}

		block := ""
		if eventName != "" {
			block = "event: " + eventName + "\n"
		}
		block += "data: " + string(newData) + "\n\n"
		return []string{block}, string(newData), usagePatch, nil
	}

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				// 上游完成，返回结果
				if !sawTerminalEvent {
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: clientDisconnected}, fmt.Errorf("stream usage incomplete: missing terminal event")
				}
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: clientDisconnected}, nil
			}
			if ev.err != nil {
				if sawTerminalEvent {
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: clientDisconnected}, nil
				}
				// 检测 context 取消（客户端断开会导致 context 取消，进而影响上游读取）
				if errors.Is(ev.err, context.Canceled) || errors.Is(ev.err, context.DeadlineExceeded) {
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: true}, fmt.Errorf("stream usage incomplete: %w", ev.err)
				}
				// 客户端已通过写入失败检测到断开，上游也出错了，返回已收集的 usage
				if clientDisconnected {
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: true}, fmt.Errorf("stream usage incomplete after disconnect: %w", ev.err)
				}
				// 客户端未断开，正常的错误处理
				if errors.Is(ev.err, bufio.ErrTooLong) {
					logger.LegacyPrintf("service.gateway", "SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, ev.err)
					sendErrorEvent("response_too_large")
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs}, ev.err
				}
				sendErrorEvent("stream_read_error")
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMs}, fmt.Errorf("stream read error: %w", ev.err)
			}
			line := ev.line
			trimmed := strings.TrimSpace(line)

			if trimmed == "" {
				if len(pendingEventLines) == 0 {
					continue
				}

				outputBlocks, data, usagePatch, err := processSSEEvent(pendingEventLines)
				pendingEventLines = pendingEventLines[:0]
				if err != nil {
					if clientDisconnected {
						return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: true}, nil
					}
					return nil, err
				}

				for _, block := range outputBlocks {
					if !clientDisconnected {
						if _, werr := fmt.Fprint(w, block); werr != nil {
							clientDisconnected = true
							logger.LegacyPrintf("service.gateway", "Client disconnected during streaming, continuing to drain upstream for billing")
							break
						}
						flusher.Flush()
						lastDataAt = time.Now()
					}
					if data != "" {
						if firstTokenMs == nil && data != "[DONE]" {
							ms := int(time.Since(startTime).Milliseconds())
							firstTokenMs = &ms
						}
						if usagePatch != nil {
							mergeSSEUsagePatch(usage, usagePatch)
						}
					}
				}
				continue
			}

			pendingEventLines = append(pendingEventLines, line)

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
			}
			if clientDisconnected {
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: true}, fmt.Errorf("stream usage incomplete after timeout")
			}
			logger.LegacyPrintf("service.gateway", "Stream data interval timeout: account=%d model=%s interval=%s", account.ID, originalModel, streamInterval)
			// 处理流超时，可能标记账户为临时不可调度或错误状态
			if s.rateLimitService != nil {
				s.rateLimitService.HandleStreamTimeout(ctx, account, originalModel)
			}
			sendErrorEvent("stream_timeout")
			return &streamingResult{usage: usage, firstTokenMs: firstTokenMs}, fmt.Errorf("stream data interval timeout")

		case <-keepaliveCh:
			if clientDisconnected {
				continue
			}
			if time.Since(lastDataAt) < keepaliveInterval {
				continue
			}
			// SSE ping 事件：Anthropic 原生格式，客户端会正确处理，
			// 同时保持连接活跃防止 Cloudflare Tunnel 等代理断开
			if _, werr := fmt.Fprint(w, "event: ping\ndata: {\"type\": \"ping\"}\n\n"); werr != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.gateway", "Client disconnected during keepalive ping, continuing to drain upstream for billing")
				continue
			}
			flusher.Flush()
		}
	}

}

func (s *GatewayService) parseSSEUsage(data string, usage *ClaudeUsage) {
	if usage == nil {
		return
	}

	var event map[string]any
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return
	}

	if patch := s.extractSSEUsagePatch(event); patch != nil {
		mergeSSEUsagePatch(usage, patch)
	}
}

type sseUsagePatch struct {
	inputTokens              int
	hasInputTokens           bool
	outputTokens             int
	hasOutputTokens          bool
	cacheCreationInputTokens int
	hasCacheCreationInput    bool
	cacheReadInputTokens     int
	hasCacheReadInput        bool
	cacheCreation5mTokens    int
	hasCacheCreation5m       bool
	cacheCreation1hTokens    int
	hasCacheCreation1h       bool
}

func (s *GatewayService) extractSSEUsagePatch(event map[string]any) *sseUsagePatch {
	if len(event) == 0 {
		return nil
	}

	eventType, _ := event["type"].(string)
	switch eventType {
	case "message_start":
		msg, _ := event["message"].(map[string]any)
		usageObj, _ := msg["usage"].(map[string]any)
		if len(usageObj) == 0 {
			return nil
		}

		patch := &sseUsagePatch{}
		patch.hasInputTokens = true
		if v, ok := parseSSEUsageInt(usageObj["input_tokens"]); ok {
			patch.inputTokens = v
		}
		patch.hasCacheCreationInput = true
		if v, ok := parseSSEUsageInt(usageObj["cache_creation_input_tokens"]); ok {
			patch.cacheCreationInputTokens = v
		}
		patch.hasCacheReadInput = true
		if v, ok := parseSSEUsageInt(usageObj["cache_read_input_tokens"]); ok {
			patch.cacheReadInputTokens = v
		}
		if cc, ok := usageObj["cache_creation"].(map[string]any); ok {
			if v, exists := parseSSEUsageInt(cc["ephemeral_5m_input_tokens"]); exists {
				patch.cacheCreation5mTokens = v
				patch.hasCacheCreation5m = true
			}
			if v, exists := parseSSEUsageInt(cc["ephemeral_1h_input_tokens"]); exists {
				patch.cacheCreation1hTokens = v
				patch.hasCacheCreation1h = true
			}
		}
		return patch

	case "message_delta":
		usageObj, _ := event["usage"].(map[string]any)
		if len(usageObj) == 0 {
			return nil
		}

		patch := &sseUsagePatch{}
		if v, ok := parseSSEUsageInt(usageObj["input_tokens"]); ok && v > 0 {
			patch.inputTokens = v
			patch.hasInputTokens = true
		}
		if v, ok := parseSSEUsageInt(usageObj["output_tokens"]); ok && v > 0 {
			patch.outputTokens = v
			patch.hasOutputTokens = true
		}
		if v, ok := parseSSEUsageInt(usageObj["cache_creation_input_tokens"]); ok && v > 0 {
			patch.cacheCreationInputTokens = v
			patch.hasCacheCreationInput = true
		}
		if v, ok := parseSSEUsageInt(usageObj["cache_read_input_tokens"]); ok && v > 0 {
			patch.cacheReadInputTokens = v
			patch.hasCacheReadInput = true
		}
		if cc, ok := usageObj["cache_creation"].(map[string]any); ok {
			if v, exists := parseSSEUsageInt(cc["ephemeral_5m_input_tokens"]); exists && v > 0 {
				patch.cacheCreation5mTokens = v
				patch.hasCacheCreation5m = true
			}
			if v, exists := parseSSEUsageInt(cc["ephemeral_1h_input_tokens"]); exists && v > 0 {
				patch.cacheCreation1hTokens = v
				patch.hasCacheCreation1h = true
			}
		}
		return patch
	}

	return nil
}

func mergeSSEUsagePatch(usage *ClaudeUsage, patch *sseUsagePatch) {
	if usage == nil || patch == nil {
		return
	}

	if patch.hasInputTokens {
		usage.InputTokens = patch.inputTokens
	}
	if patch.hasCacheCreationInput {
		usage.CacheCreationInputTokens = patch.cacheCreationInputTokens
	}
	if patch.hasCacheReadInput {
		usage.CacheReadInputTokens = patch.cacheReadInputTokens
	}
	if patch.hasOutputTokens {
		usage.OutputTokens = patch.outputTokens
	}
	if patch.hasCacheCreation5m {
		usage.CacheCreation5mTokens = patch.cacheCreation5mTokens
	}
	if patch.hasCacheCreation1h {
		usage.CacheCreation1hTokens = patch.cacheCreation1hTokens
	}
}

func parseSSEUsageInt(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case int32:
		return int(v), true
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i), true
		}
		if f, err := v.Float64(); err == nil {
			return int(f), true
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

// applyCacheTTLOverride 将所有 cache creation tokens 归入指定的 TTL 类型。
// target 为 "5m" 或 "1h"。返回 true 表示发生了变更。
func applyCacheTTLOverride(usage *ClaudeUsage, target string) bool {
	// Fallback: 如果只有聚合字段但无 5m/1h 明细，将聚合字段归入 5m 默认类别
	if usage.CacheCreation5mTokens == 0 && usage.CacheCreation1hTokens == 0 && usage.CacheCreationInputTokens > 0 {
		usage.CacheCreation5mTokens = usage.CacheCreationInputTokens
	}

	total := usage.CacheCreation5mTokens + usage.CacheCreation1hTokens
	if total == 0 {
		return false
	}
	switch target {
	case "1h":
		if usage.CacheCreation1hTokens == total {
			return false // 已经全是 1h
		}
		usage.CacheCreation1hTokens = total
		usage.CacheCreation5mTokens = 0
	default: // "5m"
		if usage.CacheCreation5mTokens == total {
			return false // 已经全是 5m
		}
		usage.CacheCreation5mTokens = total
		usage.CacheCreation1hTokens = 0
	}
	return true
}

// rewriteCacheCreationJSON 在 JSON usage 对象中重写 cache_creation 嵌套对象的 TTL 分类。
// usageObj 是 usage JSON 对象（map[string]any）。
func rewriteCacheCreationJSON(usageObj map[string]any, target string) bool {
	ccObj, ok := usageObj["cache_creation"].(map[string]any)
	if !ok {
		return false
	}
	v5m, _ := parseSSEUsageInt(ccObj["ephemeral_5m_input_tokens"])
	v1h, _ := parseSSEUsageInt(ccObj["ephemeral_1h_input_tokens"])
	total := v5m + v1h
	if total == 0 {
		return false
	}
	switch target {
	case "1h":
		if v1h == total {
			return false
		}
		ccObj["ephemeral_1h_input_tokens"] = float64(total)
		ccObj["ephemeral_5m_input_tokens"] = float64(0)
	default: // "5m"
		if v5m == total {
			return false
		}
		ccObj["ephemeral_5m_input_tokens"] = float64(total)
		ccObj["ephemeral_1h_input_tokens"] = float64(0)
	}
	return true
}

func (s *GatewayService) handleNonStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, originalModel, mappedModel string) (*ClaudeUsage, error) {
	// 更新5h窗口状态
	s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)

	maxBytes := resolveUpstreamResponseReadLimit(s.cfg)
	body, err := readUpstreamResponseBodyLimited(resp.Body, maxBytes)
	if err != nil {
		if errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			setOpsUpstreamError(c, http.StatusBadGateway, "upstream response too large", "")
			c.JSON(http.StatusBadGateway, gin.H{
				"type": "error",
				"error": gin.H{
					"type":    "upstream_error",
					"message": "Upstream response too large",
				},
			})
		}
		return nil, err
	}

	// 解析usage
	var response struct {
		Usage ClaudeUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// 解析嵌套的 cache_creation 对象中的 5m/1h 明细
	cc5m := gjson.GetBytes(body, "usage.cache_creation.ephemeral_5m_input_tokens")
	cc1h := gjson.GetBytes(body, "usage.cache_creation.ephemeral_1h_input_tokens")
	if cc5m.Exists() || cc1h.Exists() {
		response.Usage.CacheCreation5mTokens = int(cc5m.Int())
		response.Usage.CacheCreation1hTokens = int(cc1h.Int())
	}

	// 兼容 Kimi cached_tokens → cache_read_input_tokens
	if response.Usage.CacheReadInputTokens == 0 {
		cachedTokens := gjson.GetBytes(body, "usage.cached_tokens").Int()
		if cachedTokens > 0 {
			response.Usage.CacheReadInputTokens = int(cachedTokens)
			if newBody, err := sjson.SetBytes(body, "usage.cache_read_input_tokens", cachedTokens); err == nil {
				body = newBody
			}
		}
	}

	// Cache TTL Override: 重写 non-streaming 响应中的 cache_creation 分类
	if account.IsCacheTTLOverrideEnabled() {
		overrideTarget := account.GetCacheTTLOverrideTarget()
		if applyCacheTTLOverride(&response.Usage, overrideTarget) {
			// 同步更新 body JSON 中的嵌套 cache_creation 对象
			if newBody, err := sjson.SetBytes(body, "usage.cache_creation.ephemeral_5m_input_tokens", response.Usage.CacheCreation5mTokens); err == nil {
				body = newBody
			}
			if newBody, err := sjson.SetBytes(body, "usage.cache_creation.ephemeral_1h_input_tokens", response.Usage.CacheCreation1hTokens); err == nil {
				body = newBody
			}
		}
	}

	// 如果有模型映射，替换响应中的model字段
	if originalModel != mappedModel {
		body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
	}

	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	contentType := "application/json"
	if s.cfg != nil && !s.cfg.Security.ResponseHeaders.Enabled {
		if upstreamType := resp.Header.Get("Content-Type"); upstreamType != "" {
			contentType = upstreamType
		}
	}

	// 写入响应
	c.Data(resp.StatusCode, contentType, body)

	return &response.Usage, nil
}

// replaceModelInResponseBody 替换响应体中的model字段
// 使用 gjson/sjson 精确替换，避免全量 JSON 反序列化
func (s *GatewayService) replaceModelInResponseBody(body []byte, fromModel, toModel string) []byte {
	if m := gjson.GetBytes(body, "model"); m.Exists() && m.Str == fromModel {
		newBody, err := sjson.SetBytes(body, "model", toModel)
		if err != nil {
			return body
		}
		return newBody
	}
	return body
}

func (s *GatewayService) getUserGroupRateMultiplier(ctx context.Context, userID, groupID int64, groupDefaultMultiplier float64) float64 {
	if s == nil {
		return groupDefaultMultiplier
	}
	resolver := s.userGroupRateResolver
	if resolver == nil {
		resolver = newUserGroupRateResolver(
			s.userGroupRateRepo,
			s.userGroupRateCache,
			resolveUserGroupRateCacheTTL(s.cfg),
			&s.userGroupRateSF,
			"service.gateway",
		)
	}
	return resolver.Resolve(ctx, userID, groupID, groupDefaultMultiplier)
}

// RecordUsageInput 记录使用量的输入参数
type RecordUsageInput struct {
	Result             *ForwardResult
	ParsedRequest      *ParsedRequest
	APIKey             *APIKey
	User               *User
	Account            *Account
	Subscription       *UserSubscription  // 可选：订阅信息
	InboundEndpoint    string             // 入站端点（客户端请求路径）
	UpstreamEndpoint   string             // 上游端点（标准化后的上游路径）
	UserAgent          string             // 请求的 User-Agent
	IPAddress          string             // 请求的客户端 IP 地址
	RequestPayloadHash string             // 请求体语义哈希，用于降低 request_id 误复用时的静默误去重风险
	ForceCacheBilling  bool               // 强制缓存计费：将 input_tokens 转为 cache_read 计费（用于粘性会话切换）
	APIKeyService      APIKeyQuotaUpdater // 可选：用于更新API Key配额

	ChannelUsageFields // 渠道映射信息（由 handler 在 Forward 前解析）
}

// APIKeyQuotaUpdater defines the interface for updating API Key quota and rate limit usage
type APIKeyQuotaUpdater interface {
	UpdateQuotaUsed(ctx context.Context, apiKeyID int64, cost float64) error
	UpdateRateLimitUsage(ctx context.Context, apiKeyID int64, cost float64) error
}

type apiKeyAuthCacheInvalidator interface {
	InvalidateAuthCacheByKey(ctx context.Context, key string)
}

type usageLogBestEffortWriter interface {
	CreateBestEffort(ctx context.Context, log *UsageLog) error
}

// postUsageBillingParams 统一扣费所需的参数
type postUsageBillingParams struct {
	Cost                  *CostBreakdown
	User                  *User
	APIKey                *APIKey
	Account               *Account
	Subscription          *UserSubscription
	RequestPayloadHash    string
	IsSubscriptionBill    bool
	AccountRateMultiplier float64
	APIKeyService         APIKeyQuotaUpdater
}

func (p *postUsageBillingParams) shouldDeductAPIKeyQuota() bool {
	return p.Cost.ActualCost > 0 && p.APIKey.Quota > 0 && p.APIKeyService != nil
}

func (p *postUsageBillingParams) shouldUpdateRateLimits() bool {
	return p.Cost.ActualCost > 0 && p.APIKey.HasRateLimits() && p.APIKeyService != nil
}

func (p *postUsageBillingParams) shouldUpdateAccountQuota() bool {
	return p.Cost.TotalCost > 0 && p.Account.IsAPIKeyOrBedrock() && p.Account.HasAnyQuotaLimit()
}

// postUsageBilling is the legacy fallback billing path used when the unified
// billing repo is unavailable (nil). Production uses applyUsageBilling → repo.Apply
// for atomic billing. This path only runs in tests or degraded mode.
func postUsageBilling(ctx context.Context, p *postUsageBillingParams, deps *billingDeps) {
	billingCtx, cancel := detachedBillingContext(ctx)
	defer cancel()

	cost := p.Cost

	if p.IsSubscriptionBill {
		if cost.TotalCost > 0 {
			if err := deps.userSubRepo.IncrementUsage(billingCtx, p.Subscription.ID, cost.TotalCost); err != nil {
				slog.Error("increment subscription usage failed", "subscription_id", p.Subscription.ID, "error", err)
			}
		}
	} else {
		if cost.ActualCost > 0 {
			if err := deps.userRepo.DeductBalance(billingCtx, p.User.ID, cost.ActualCost); err != nil {
				slog.Error("deduct balance failed", "user_id", p.User.ID, "error", err)
			}
		}
	}

	if p.shouldDeductAPIKeyQuota() {
		if err := p.APIKeyService.UpdateQuotaUsed(billingCtx, p.APIKey.ID, cost.ActualCost); err != nil {
			slog.Error("update api key quota failed", "api_key_id", p.APIKey.ID, "error", err)
		}
	}

	if p.shouldUpdateRateLimits() {
		if err := p.APIKeyService.UpdateRateLimitUsage(billingCtx, p.APIKey.ID, cost.ActualCost); err != nil {
			slog.Error("update api key rate limit usage failed", "api_key_id", p.APIKey.ID, "error", err)
		}
	}

	if p.shouldUpdateAccountQuota() {
		accountCost := cost.TotalCost * p.AccountRateMultiplier
		if err := deps.accountRepo.IncrementQuotaUsed(billingCtx, p.Account.ID, accountCost); err != nil {
			slog.Error("increment account quota used failed", "account_id", p.Account.ID, "cost", accountCost, "error", err)
		}
	}

	// NOTE: finalizePostUsageBilling is NOT called here to avoid double-queuing
	// cache updates. The legacy path does DB writes directly; the finalize path
	// does cache queue + notifications. Notifications are dispatched separately
	// by the caller after recording the usage log.
}

func resolveUsageBillingRequestID(ctx context.Context, upstreamRequestID string) string {
	if ctx != nil {
		if clientRequestID, _ := ctx.Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(clientRequestID) != "" {
			return "client:" + strings.TrimSpace(clientRequestID)
		}
		if requestID, _ := ctx.Value(ctxkey.RequestID).(string); strings.TrimSpace(requestID) != "" {
			return "local:" + strings.TrimSpace(requestID)
		}
	}
	if requestID := strings.TrimSpace(upstreamRequestID); requestID != "" {
		return requestID
	}
	return "generated:" + generateRequestID()
}

func resolveUsageBillingPayloadFingerprint(ctx context.Context, requestPayloadHash string) string {
	if payloadHash := strings.TrimSpace(requestPayloadHash); payloadHash != "" {
		return payloadHash
	}
	if ctx != nil {
		if clientRequestID, _ := ctx.Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(clientRequestID) != "" {
			return "client:" + strings.TrimSpace(clientRequestID)
		}
		if requestID, _ := ctx.Value(ctxkey.RequestID).(string); strings.TrimSpace(requestID) != "" {
			return "local:" + strings.TrimSpace(requestID)
		}
	}
	return ""
}

func buildUsageBillingCommand(requestID string, usageLog *UsageLog, p *postUsageBillingParams) *UsageBillingCommand {
	if p == nil || p.Cost == nil || p.APIKey == nil || p.User == nil || p.Account == nil {
		return nil
	}

	cmd := &UsageBillingCommand{
		RequestID:          requestID,
		APIKeyID:           p.APIKey.ID,
		UserID:             p.User.ID,
		AccountID:          p.Account.ID,
		AccountType:        p.Account.Type,
		RequestPayloadHash: strings.TrimSpace(p.RequestPayloadHash),
	}
	if usageLog != nil {
		cmd.Model = usageLog.Model
		cmd.BillingType = usageLog.BillingType
		cmd.InputTokens = usageLog.InputTokens
		cmd.OutputTokens = usageLog.OutputTokens
		cmd.CacheCreationTokens = usageLog.CacheCreationTokens
		cmd.CacheReadTokens = usageLog.CacheReadTokens
		cmd.ImageCount = usageLog.ImageCount
		if usageLog.ServiceTier != nil {
			cmd.ServiceTier = *usageLog.ServiceTier
		}
		if usageLog.ReasoningEffort != nil {
			cmd.ReasoningEffort = *usageLog.ReasoningEffort
		}
		if usageLog.SubscriptionID != nil {
			cmd.SubscriptionID = usageLog.SubscriptionID
		}
	}

	if p.IsSubscriptionBill && p.Subscription != nil && p.Cost.TotalCost > 0 {
		cmd.SubscriptionID = &p.Subscription.ID
		cmd.SubscriptionCost = p.Cost.TotalCost
	} else if p.Cost.ActualCost > 0 {
		cmd.BalanceCost = p.Cost.ActualCost
	}

	if p.shouldDeductAPIKeyQuota() {
		cmd.APIKeyQuotaCost = p.Cost.ActualCost
	}
	if p.shouldUpdateRateLimits() {
		cmd.APIKeyRateLimitCost = p.Cost.ActualCost
	}
	if p.shouldUpdateAccountQuota() {
		cmd.AccountQuotaCost = p.Cost.TotalCost * p.AccountRateMultiplier
	}

	cmd.Normalize()
	return cmd
}

func applyUsageBilling(ctx context.Context, requestID string, usageLog *UsageLog, p *postUsageBillingParams, deps *billingDeps, repo UsageBillingRepository) (bool, error) {
	if p == nil || deps == nil {
		return false, nil
	}

	cmd := buildUsageBillingCommand(requestID, usageLog, p)
	if cmd == nil || cmd.RequestID == "" || repo == nil {
		postUsageBilling(ctx, p, deps)
		return true, nil
	}

	billingCtx, cancel := detachedBillingContext(ctx)
	defer cancel()

	result, err := repo.Apply(billingCtx, cmd)
	if err != nil {
		return false, err
	}

	if result == nil || !result.Applied {
		deps.deferredService.ScheduleLastUsedUpdate(p.Account.ID)
		return false, nil
	}

	if result.APIKeyQuotaExhausted {
		if invalidator, ok := p.APIKeyService.(apiKeyAuthCacheInvalidator); ok && p.APIKey != nil && p.APIKey.Key != "" {
			invalidator.InvalidateAuthCacheByKey(billingCtx, p.APIKey.Key)
		}
	}

	finalizePostUsageBilling(p, deps, result)
	return true, nil
}

func finalizePostUsageBilling(p *postUsageBillingParams, deps *billingDeps, result *UsageBillingApplyResult) {
	if p == nil || p.Cost == nil || deps == nil {
		return
	}

	if p.IsSubscriptionBill {
		if p.Cost.TotalCost > 0 && p.User != nil && p.APIKey != nil && p.APIKey.GroupID != nil {
			deps.billingCacheService.QueueUpdateSubscriptionUsage(p.User.ID, *p.APIKey.GroupID, p.Cost.TotalCost)
		}
	} else if p.Cost.ActualCost > 0 && p.User != nil {
		deps.billingCacheService.QueueDeductBalance(p.User.ID, p.Cost.ActualCost)
	}

	if p.Cost.ActualCost > 0 && p.APIKey != nil && p.APIKey.HasRateLimits() {
		deps.billingCacheService.QueueUpdateAPIKeyRateLimitUsage(p.APIKey.ID, p.Cost.ActualCost)
	}

	deps.deferredService.ScheduleLastUsedUpdate(p.Account.ID)

	// Notification checks run async — all parameters are already captured,
	// no dependency on the request context or upstream connection.
	go notifyBalanceLow(p, deps, result)
	go notifyAccountQuota(p, deps, result)
}

// notifyBalanceLow sends balance low notification after deduction.
// When result.NewBalance is available (from DB transaction RETURNING), it is used directly
// to reconstruct oldBalance, avoiding stale Redis reads and concurrent-deduction races.
func notifyBalanceLow(p *postUsageBillingParams, deps *billingDeps, result *UsageBillingApplyResult) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in notifyBalanceLow", "recover", r)
		}
	}()
	if p.IsSubscriptionBill || p.Cost.ActualCost <= 0 || p.User == nil || deps.balanceNotifyService == nil {
		slog.Debug("notifyBalanceLow: skipped",
			"is_subscription", p.IsSubscriptionBill,
			"actual_cost", p.Cost.ActualCost,
			"user_nil", p.User == nil,
			"service_nil", deps.balanceNotifyService == nil,
		)
		return
	}

	oldBalance := resolveOldBalance(p, result)
	slog.Debug("notifyBalanceLow: calling CheckBalanceAfterDeduction",
		"user_id", p.User.ID,
		"old_balance", oldBalance,
		"cost", p.Cost.ActualCost,
		"notify_enabled", p.User.BalanceNotifyEnabled,
		"threshold", p.User.BalanceNotifyThreshold,
		"result_has_new_balance", result != nil && result.NewBalance != nil,
	)
	deps.balanceNotifyService.CheckBalanceAfterDeduction(context.Background(), p.User, oldBalance, p.Cost.ActualCost)
}

// resolveOldBalance returns the pre-deduction balance.
// Prefers the DB transaction result (newBalance + cost) over snapshot.
func resolveOldBalance(p *postUsageBillingParams, result *UsageBillingApplyResult) float64 {
	if result != nil && result.NewBalance != nil {
		return *result.NewBalance + p.Cost.ActualCost
	}
	// Legacy fallback: snapshot balance from request context
	return p.User.Balance
}

// notifyAccountQuota sends account quota threshold notification after increment.
// When result.QuotaState is available (from DB transaction RETURNING), it is passed directly
// to avoid a separate DB read that may see stale or concurrently-modified data.
func notifyAccountQuota(p *postUsageBillingParams, deps *billingDeps, result *UsageBillingApplyResult) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in notifyAccountQuota", "recover", r)
		}
	}()
	if p.Cost.TotalCost <= 0 || p.Account == nil || !p.Account.IsAPIKeyOrBedrock() || deps.balanceNotifyService == nil {
		slog.Debug("notifyAccountQuota: skipped",
			"total_cost", p.Cost.TotalCost,
			"account_nil", p.Account == nil,
			"is_apikey_or_bedrock", p.Account != nil && p.Account.IsAPIKeyOrBedrock(),
			"service_nil", deps.balanceNotifyService == nil,
		)
		return
	}
	accountCost := p.Cost.TotalCost * p.AccountRateMultiplier
	var quotaState *AccountQuotaState
	if result != nil {
		quotaState = result.QuotaState
	}
	slog.Debug("notifyAccountQuota: calling CheckAccountQuotaAfterIncrement",
		"account_id", p.Account.ID,
		"account_cost", accountCost,
		"has_quota_state", quotaState != nil,
	)
	deps.balanceNotifyService.CheckAccountQuotaAfterIncrement(context.Background(), p.Account, accountCost, quotaState)
}

func detachedBillingContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, postUsageBillingTimeout)
}

func detachStreamUpstreamContext(ctx context.Context, stream bool) (context.Context, context.CancelFunc) {
	if !stream {
		return ctx, func() {}
	}
	if ctx == nil {
		return context.Background(), func() {}
	}
	return context.WithoutCancel(ctx), func() {}
}

// billingDeps 扣费逻辑依赖的服务（由各 gateway service 提供）
type billingDeps struct {
	accountRepo          AccountRepository
	userRepo             UserRepository
	userSubRepo          UserSubscriptionRepository
	billingCacheService  *BillingCacheService
	deferredService      *DeferredService
	balanceNotifyService *BalanceNotifyService
}

func (s *GatewayService) billingDeps() *billingDeps {
	return &billingDeps{
		accountRepo:          s.accountRepo,
		userRepo:             s.userRepo,
		userSubRepo:          s.userSubRepo,
		billingCacheService:  s.billingCacheService,
		deferredService:      s.deferredService,
		balanceNotifyService: s.balanceNotifyService,
	}
}

func writeUsageLogBestEffort(ctx context.Context, repo UsageLogRepository, usageLog *UsageLog, logKey string) {
	if repo == nil || usageLog == nil {
		return
	}
	usageCtx, cancel := detachedBillingContext(ctx)
	defer cancel()

	if writer, ok := repo.(usageLogBestEffortWriter); ok {
		if err := writer.CreateBestEffort(usageCtx, usageLog); err != nil {
			logger.LegacyPrintf(logKey, "Create usage log failed: %v", err)
			if IsUsageLogCreateDropped(err) {
				return
			}
			if _, syncErr := repo.Create(usageCtx, usageLog); syncErr != nil {
				logger.LegacyPrintf(logKey, "Create usage log sync fallback failed: %v", syncErr)
			}
		}
		return
	}

	if _, err := repo.Create(usageCtx, usageLog); err != nil {
		logger.LegacyPrintf(logKey, "Create usage log failed: %v", err)
	}
}

// recordUsageOpts 内部选项，参数化 RecordUsage 与 RecordUsageWithLongContext 的差异点。
type recordUsageOpts struct {
	// Claude Max 策略所需的 ParsedRequest（可选，仅 Claude 路径传入）
	ParsedRequest *ParsedRequest

	// EnableClaudePath 启用 Claude 路径特有逻辑：
	// - Claude Max 缓存计费策略
	EnableClaudePath bool

	// 长上下文计费（仅 Gemini 路径需要）
	LongContextThreshold  int
	LongContextMultiplier float64
}

// RecordUsage 记录使用量并扣费（或更新订阅用量）
func (s *GatewayService) RecordUsage(ctx context.Context, input *RecordUsageInput) error {
	return s.recordUsageCore(ctx, &recordUsageCoreInput{
		Result:             input.Result,
		APIKey:             input.APIKey,
		User:               input.User,
		Account:            input.Account,
		Subscription:       input.Subscription,
		InboundEndpoint:    input.InboundEndpoint,
		UpstreamEndpoint:   input.UpstreamEndpoint,
		UserAgent:          input.UserAgent,
		IPAddress:          input.IPAddress,
		RequestPayloadHash: input.RequestPayloadHash,
		ForceCacheBilling:  input.ForceCacheBilling,
		APIKeyService:      input.APIKeyService,
		ChannelUsageFields: input.ChannelUsageFields,
	}, &recordUsageOpts{
		EnableClaudePath: true,
	})
}

// RecordUsageLongContextInput 记录使用量的输入参数（支持长上下文双倍计费）
type RecordUsageLongContextInput struct {
	Result                *ForwardResult
	APIKey                *APIKey
	User                  *User
	Account               *Account
	Subscription          *UserSubscription  // 可选：订阅信息
	InboundEndpoint       string             // 入站端点（客户端请求路径）
	UpstreamEndpoint      string             // 上游端点（标准化后的上游路径）
	UserAgent             string             // 请求的 User-Agent
	IPAddress             string             // 请求的客户端 IP 地址
	RequestPayloadHash    string             // 请求体语义哈希，用于降低 request_id 误复用时的静默误去重风险
	LongContextThreshold  int                // 长上下文阈值（如 200000）
	LongContextMultiplier float64            // 超出阈值部分的倍率（如 2.0）
	ForceCacheBilling     bool               // 强制缓存计费：将 input_tokens 转为 cache_read 计费（用于粘性会话切换）
	APIKeyService         APIKeyQuotaUpdater // API Key 配额服务（可选）

	ChannelUsageFields // 渠道映射信息（由 handler 在 Forward 前解析）
}

// RecordUsageWithLongContext 记录使用量并扣费，支持长上下文双倍计费（用于 Gemini）
func (s *GatewayService) RecordUsageWithLongContext(ctx context.Context, input *RecordUsageLongContextInput) error {
	return s.recordUsageCore(ctx, &recordUsageCoreInput{
		Result:             input.Result,
		APIKey:             input.APIKey,
		User:               input.User,
		Account:            input.Account,
		Subscription:       input.Subscription,
		InboundEndpoint:    input.InboundEndpoint,
		UpstreamEndpoint:   input.UpstreamEndpoint,
		UserAgent:          input.UserAgent,
		IPAddress:          input.IPAddress,
		RequestPayloadHash: input.RequestPayloadHash,
		ForceCacheBilling:  input.ForceCacheBilling,
		APIKeyService:      input.APIKeyService,
		ChannelUsageFields: input.ChannelUsageFields,
	}, &recordUsageOpts{
		LongContextThreshold:  input.LongContextThreshold,
		LongContextMultiplier: input.LongContextMultiplier,
	})
}

// recordUsageCoreInput 是 recordUsageCore 的公共输入字段，从两种输入结构体中提取。
type recordUsageCoreInput struct {
	Result             *ForwardResult
	APIKey             *APIKey
	User               *User
	Account            *Account
	Subscription       *UserSubscription
	InboundEndpoint    string
	UpstreamEndpoint   string
	UserAgent          string
	IPAddress          string
	RequestPayloadHash string
	ForceCacheBilling  bool
	APIKeyService      APIKeyQuotaUpdater
	ChannelUsageFields
}

// recordUsageCore 是 RecordUsage 和 RecordUsageWithLongContext 的统一实现。
// opts 中的字段控制两者之间的差异行为：
// - ParsedRequest != nil → 启用 Claude Max 缓存计费策略
// - LongContextThreshold > 0 → Token 计费回退走 CalculateCostWithLongContext
func (s *GatewayService) recordUsageCore(ctx context.Context, input *recordUsageCoreInput, opts *recordUsageOpts) error {
	result := input.Result
	apiKey := input.APIKey
	user := input.User
	account := input.Account
	subscription := input.Subscription

	// 强制缓存计费：将 input_tokens 转为 cache_read_input_tokens
	// 用于粘性会话切换时的特殊计费处理
	if input.ForceCacheBilling && result.Usage.InputTokens > 0 {
		logger.LegacyPrintf("service.gateway", "force_cache_billing: %d input_tokens → cache_read_input_tokens (account=%d)",
			result.Usage.InputTokens, account.ID)
		result.Usage.CacheReadInputTokens += result.Usage.InputTokens
		result.Usage.InputTokens = 0
	}

	// Cache TTL Override: 确保计费时 token 分类与账号设置一致
	cacheTTLOverridden := false
	if account.IsCacheTTLOverrideEnabled() {
		applyCacheTTLOverride(&result.Usage, account.GetCacheTTLOverrideTarget())
		cacheTTLOverridden = (result.Usage.CacheCreation5mTokens + result.Usage.CacheCreation1hTokens) > 0
	}

	// 获取费率倍数（优先级：用户专属 > 分组默认 > 系统默认）
	multiplier := 1.0
	if s.cfg != nil {
		multiplier = s.cfg.Default.RateMultiplier
	}
	if apiKey.GroupID != nil && apiKey.Group != nil {
		groupDefault := apiKey.Group.RateMultiplier
		multiplier = s.getUserGroupRateMultiplier(ctx, user.ID, *apiKey.GroupID, groupDefault)
	}

	// 确定计费模型
	billingModel := forwardResultBillingModel(result.Model, result.UpstreamModel)
	if input.BillingModelSource == BillingModelSourceChannelMapped && input.ChannelMappedModel != "" {
		billingModel = input.ChannelMappedModel
	}
	if input.BillingModelSource == BillingModelSourceRequested && input.OriginalModel != "" {
		billingModel = input.OriginalModel
	}

	// 确定 RequestedModel（渠道映射前的原始模型）
	requestedModel := result.Model
	if input.OriginalModel != "" {
		requestedModel = input.OriginalModel
	}

	// 计算费用
	cost := s.calculateRecordUsageCost(ctx, result, apiKey, billingModel, multiplier, opts)

	// 判断计费方式：订阅模式 vs 余额模式
	isSubscriptionBilling := subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
	billingType := BillingTypeBalance
	if isSubscriptionBilling {
		billingType = BillingTypeSubscription
	}

	// 创建使用日志
	accountRateMultiplier := account.BillingRateMultiplier()
	usageLog := s.buildRecordUsageLog(ctx, input, result, apiKey, user, account, subscription,
		requestedModel, multiplier, accountRateMultiplier, billingType, cacheTTLOverridden, cost, opts)

	// 计算账号统计定价费用（使用最终上游模型匹配自定义规则）
	if apiKey.GroupID != nil {
		applyAccountStatsCost(ctx, usageLog, s.channelService, s.billingService,
			account.ID, *apiKey.GroupID, result.UpstreamModel, result.Model,
			// Anthropic's input_tokens excludes cache_read and cache_creation (billed separately);
			// OpenAI gateway uses actualInputTokens which also excludes cache_read for the same reason.
			UsageTokens{
				InputTokens:         result.Usage.InputTokens,
				OutputTokens:        result.Usage.OutputTokens,
				CacheCreationTokens: result.Usage.CacheCreationInputTokens,
				CacheReadTokens:     result.Usage.CacheReadInputTokens,
				ImageOutputTokens:   result.Usage.ImageOutputTokens,
			},
			cost.TotalCost,
		)
	}

	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.gateway")
		logger.LegacyPrintf("service.gateway", "[SIMPLE MODE] Usage recorded (not billed): user=%d, tokens=%d", usageLog.UserID, usageLog.TotalTokens())
		s.deferredService.ScheduleLastUsedUpdate(account.ID)
		return nil
	}

	requestID := usageLog.RequestID
	_, billingErr := applyUsageBilling(ctx, requestID, usageLog, &postUsageBillingParams{
		Cost:                  cost,
		User:                  user,
		APIKey:                apiKey,
		Account:               account,
		Subscription:          subscription,
		RequestPayloadHash:    resolveUsageBillingPayloadFingerprint(ctx, input.RequestPayloadHash),
		IsSubscriptionBill:    isSubscriptionBilling,
		AccountRateMultiplier: accountRateMultiplier,
		APIKeyService:         input.APIKeyService,
	}, s.billingDeps(), s.usageBillingRepo)

	if billingErr != nil {
		return billingErr
	}
	writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.gateway")

	return nil
}

// calculateRecordUsageCost 根据请求类型和选项计算费用。
func (s *GatewayService) calculateRecordUsageCost(
	ctx context.Context,
	result *ForwardResult,
	apiKey *APIKey,
	billingModel string,
	multiplier float64,
	opts *recordUsageOpts,
) *CostBreakdown {
	// 图片生成计费
	if result.ImageCount > 0 {
		return s.calculateImageCost(ctx, result, apiKey, billingModel, multiplier)
	}

	// Token 计费
	return s.calculateTokenCost(ctx, result, apiKey, billingModel, multiplier, opts)
}

// resolveChannelPricing 检查指定模型是否存在渠道级别定价。
// 返回非 nil 的 ResolvedPricing 表示有渠道定价，nil 表示走默认定价路径。
func (s *GatewayService) resolveChannelPricing(ctx context.Context, billingModel string, apiKey *APIKey) *ResolvedPricing {
	if s.resolver == nil || apiKey.Group == nil {
		return nil
	}
	gid := apiKey.Group.ID
	resolved := s.resolver.Resolve(ctx, PricingInput{Model: billingModel, GroupID: &gid})
	if resolved.Source == PricingSourceChannel {
		return resolved
	}
	return nil
}

// calculateImageCost 计算图片生成费用：渠道级别定价优先，否则走按次计费。
func (s *GatewayService) calculateImageCost(
	ctx context.Context,
	result *ForwardResult,
	apiKey *APIKey,
	billingModel string,
	multiplier float64,
) *CostBreakdown {
	if resolved := s.resolveChannelPricing(ctx, billingModel, apiKey); resolved != nil {
		tokens := UsageTokens{
			InputTokens:       result.Usage.InputTokens,
			OutputTokens:      result.Usage.OutputTokens,
			ImageOutputTokens: result.Usage.ImageOutputTokens,
		}
		gid := apiKey.Group.ID
		cost, err := s.billingService.CalculateCostUnified(CostInput{
			Ctx:            ctx,
			Model:          billingModel,
			GroupID:        &gid,
			Tokens:         tokens,
			RequestCount:   1,
			RateMultiplier: multiplier,
			Resolver:       s.resolver,
			Resolved:       resolved,
		})
		if err != nil {
			logger.LegacyPrintf("service.gateway", "Calculate image token cost failed: %v", err)
			return &CostBreakdown{ActualCost: 0}
		}
		return cost
	}

	var groupConfig *ImagePriceConfig
	if apiKey.Group != nil {
		groupConfig = &ImagePriceConfig{
			Price1K: apiKey.Group.ImagePrice1K,
			Price2K: apiKey.Group.ImagePrice2K,
			Price4K: apiKey.Group.ImagePrice4K,
		}
	}
	return s.billingService.CalculateImageCost(billingModel, result.ImageSize, result.ImageCount, groupConfig, multiplier)
}

// calculateTokenCost 计算 Token 计费：根据 opts 决定走普通/长上下文/渠道统一计费。
func (s *GatewayService) calculateTokenCost(
	ctx context.Context,
	result *ForwardResult,
	apiKey *APIKey,
	billingModel string,
	multiplier float64,
	opts *recordUsageOpts,
) *CostBreakdown {
	tokens := UsageTokens{
		InputTokens:           result.Usage.InputTokens,
		OutputTokens:          result.Usage.OutputTokens,
		CacheCreationTokens:   result.Usage.CacheCreationInputTokens,
		CacheReadTokens:       result.Usage.CacheReadInputTokens,
		CacheCreation5mTokens: result.Usage.CacheCreation5mTokens,
		CacheCreation1hTokens: result.Usage.CacheCreation1hTokens,
		ImageOutputTokens:     result.Usage.ImageOutputTokens,
	}

	var cost *CostBreakdown
	var err error

	// 优先尝试渠道定价 → CalculateCostUnified
	if resolved := s.resolveChannelPricing(ctx, billingModel, apiKey); resolved != nil {
		gid := apiKey.Group.ID
		cost, err = s.billingService.CalculateCostUnified(CostInput{
			Ctx:            ctx,
			Model:          billingModel,
			GroupID:        &gid,
			Tokens:         tokens,
			RequestCount:   1,
			RateMultiplier: multiplier,
			Resolver:       s.resolver,
			Resolved:       resolved,
		})
	} else if opts.LongContextThreshold > 0 {
		// 长上下文双倍计费（如 Gemini 200K 阈值）
		cost, err = s.billingService.CalculateCostWithLongContext(
			billingModel, tokens, multiplier,
			opts.LongContextThreshold, opts.LongContextMultiplier,
		)
	} else {
		cost, err = s.billingService.CalculateCost(billingModel, tokens, multiplier)
	}
	if err != nil {
		logger.LegacyPrintf("service.gateway", "Calculate cost failed: %v", err)
		return &CostBreakdown{ActualCost: 0}
	}
	return cost
}

// buildRecordUsageLog 构建使用日志并设置计费模式。
func (s *GatewayService) buildRecordUsageLog(
	ctx context.Context,
	input *recordUsageCoreInput,
	result *ForwardResult,
	apiKey *APIKey,
	user *User,
	account *Account,
	subscription *UserSubscription,
	requestedModel string,
	multiplier float64,
	accountRateMultiplier float64,
	billingType int8,
	cacheTTLOverridden bool,
	cost *CostBreakdown,
	opts *recordUsageOpts,
) *UsageLog {
	durationMs := int(result.Duration.Milliseconds())
	requestID := resolveUsageBillingRequestID(ctx, result.RequestID)
	usageLog := &UsageLog{
		UserID:                user.ID,
		APIKeyID:              apiKey.ID,
		AccountID:             account.ID,
		RequestID:             requestID,
		Model:                 result.Model,
		RequestedModel:        requestedModel,
		UpstreamModel:         optionalNonEqualStringPtr(result.UpstreamModel, result.Model),
		ReasoningEffort:       result.ReasoningEffort,
		InboundEndpoint:       optionalTrimmedStringPtr(input.InboundEndpoint),
		UpstreamEndpoint:      optionalTrimmedStringPtr(input.UpstreamEndpoint),
		InputTokens:           result.Usage.InputTokens,
		OutputTokens:          result.Usage.OutputTokens,
		CacheCreationTokens:   result.Usage.CacheCreationInputTokens,
		CacheReadTokens:       result.Usage.CacheReadInputTokens,
		CacheCreation5mTokens: result.Usage.CacheCreation5mTokens,
		CacheCreation1hTokens: result.Usage.CacheCreation1hTokens,
		ImageOutputTokens:     result.Usage.ImageOutputTokens,
		RateMultiplier:        multiplier,
		AccountRateMultiplier: &accountRateMultiplier,
		BillingType:           billingType,
		BillingMode:           resolveBillingMode(result, cost),
		Stream:                result.Stream,
		DurationMs:            &durationMs,
		FirstTokenMs:          result.FirstTokenMs,
		ImageCount:            result.ImageCount,
		ImageSize:             optionalTrimmedStringPtr(result.ImageSize),
		CacheTTLOverridden:    cacheTTLOverridden,
		ChannelID:             optionalInt64Ptr(input.ChannelID),
		ModelMappingChain:     optionalTrimmedStringPtr(input.ModelMappingChain),
		UserAgent:             optionalTrimmedStringPtr(input.UserAgent),
		IPAddress:             optionalTrimmedStringPtr(input.IPAddress),
		GroupID:               apiKey.GroupID,
		SubscriptionID:        optionalSubscriptionID(subscription),
		CreatedAt:             time.Now(),
	}
	if cost != nil {
		usageLog.InputCost = cost.InputCost
		usageLog.OutputCost = cost.OutputCost
		usageLog.ImageOutputCost = cost.ImageOutputCost
		usageLog.CacheCreationCost = cost.CacheCreationCost
		usageLog.CacheReadCost = cost.CacheReadCost
		usageLog.TotalCost = cost.TotalCost
		usageLog.ActualCost = cost.ActualCost
	}

	return usageLog
}

// resolveBillingMode 根据计费结果和请求类型确定计费模式。
func resolveBillingMode(result *ForwardResult, cost *CostBreakdown) *string {
	var mode string
	switch {
	case cost != nil && cost.BillingMode != "":
		mode = cost.BillingMode
	case result.ImageCount > 0:
		mode = string(BillingModeImage)
	default:
		mode = string(BillingModeToken)
	}
	return &mode
}

func optionalSubscriptionID(subscription *UserSubscription) *int64 {
	if subscription != nil {
		return &subscription.ID
	}
	return nil
}

// ResolveChannelMapping 委托渠道服务解析模型映射
func (s *GatewayService) ResolveChannelMapping(ctx context.Context, groupID int64, model string) ChannelMappingResult {
	if s.channelService == nil {
		return ChannelMappingResult{MappedModel: model}
	}
	return s.channelService.ResolveChannelMapping(ctx, groupID, model)
}

// ReplaceModelInBody 替换请求体中的模型名（导出供 handler 使用）
func (s *GatewayService) ReplaceModelInBody(body []byte, newModel string) []byte {
	return ReplaceModelInBody(body, newModel)
}

// IsModelRestricted 检查模型是否被渠道限制
func (s *GatewayService) IsModelRestricted(ctx context.Context, groupID int64, model string) bool {
	if s.channelService == nil {
		return false
	}
	return s.channelService.IsModelRestricted(ctx, groupID, model)
}

// ResolveChannelMappingAndRestrict 解析渠道映射。
// 模型限制检查已移至调度阶段（checkChannelPricingRestriction），restricted 始终返回 false。
func (s *GatewayService) ResolveChannelMappingAndRestrict(ctx context.Context, groupID *int64, model string) (ChannelMappingResult, bool) {
	if s.channelService == nil {
		return ChannelMappingResult{MappedModel: model}, false
	}
	return s.channelService.ResolveChannelMappingAndRestrict(ctx, groupID, model)
}

// checkChannelPricingRestriction 根据渠道计费基准检查模型是否受定价列表限制。
// 供调度阶段预检查（requested / channel_mapped）。
// upstream 需逐账号检查，此处返回 false。
func (s *GatewayService) checkChannelPricingRestriction(ctx context.Context, groupID *int64, requestedModel string) bool {
	if groupID == nil || s.channelService == nil || requestedModel == "" {
		return false
	}
	mapping := s.channelService.ResolveChannelMapping(ctx, *groupID, requestedModel)
	billingModel := billingModelForRestriction(mapping.BillingModelSource, requestedModel, mapping.MappedModel)
	if billingModel == "" {
		return false
	}
	return s.channelService.IsModelRestricted(ctx, *groupID, billingModel)
}

// billingModelForRestriction 根据计费基准确定限制检查使用的模型。
// upstream 返回空（需逐账号检查）。
func billingModelForRestriction(source, requestedModel, channelMappedModel string) string {
	switch source {
	case BillingModelSourceRequested:
		return requestedModel
	case BillingModelSourceUpstream:
		return ""
	case BillingModelSourceChannelMapped:
		return channelMappedModel
	default:
		return channelMappedModel
	}
}

// isUpstreamModelRestrictedByChannel 检查账号映射后的上游模型是否受渠道定价限制。
// 仅在 BillingModelSource="upstream" 且 RestrictModels=true 时由调度循环调用。
func (s *GatewayService) isUpstreamModelRestrictedByChannel(ctx context.Context, groupID int64, account *Account, requestedModel string) bool {
	if s.channelService == nil {
		return false
	}
	upstreamModel := resolveAccountUpstreamModel(account, requestedModel)
	if upstreamModel == "" {
		return false
	}
	return s.channelService.IsModelRestricted(ctx, groupID, upstreamModel)
}

// resolveAccountUpstreamModel 确定账号将请求模型映射为什么上游模型。
func resolveAccountUpstreamModel(account *Account, requestedModel string) string {
	if account.Platform == PlatformAntigravity {
		return mapAntigravityModel(account, requestedModel)
	}
	return account.GetMappedModel(requestedModel)
}

// needsUpstreamChannelRestrictionCheck 判断是否需要在调度循环中逐账号检查上游模型的渠道限制。
func (s *GatewayService) needsUpstreamChannelRestrictionCheck(ctx context.Context, groupID *int64) bool {
	if groupID == nil || s.channelService == nil {
		return false
	}
	ch, err := s.channelService.GetChannelForGroup(ctx, *groupID)
	if err != nil {
		slog.Warn("failed to check channel upstream restriction", "group_id", *groupID, "error", err)
		return false
	}
	if ch == nil || !ch.RestrictModels {
		return false
	}
	return ch.BillingModelSource == BillingModelSourceUpstream
}

// isStickyAccountUpstreamRestricted 检查粘性会话命中的账号是否受 upstream 渠道限制。
// 合并 needsUpstreamChannelRestrictionCheck + isUpstreamModelRestrictedByChannel 两步调用，
// 供 sticky session 条件链使用，避免内联多个函数调用导致行过长。
func (s *GatewayService) isStickyAccountUpstreamRestricted(ctx context.Context, groupID *int64, account *Account, requestedModel string) bool {
	if groupID == nil {
		return false
	}
	if !s.needsUpstreamChannelRestrictionCheck(ctx, groupID) {
		return false
	}
	return s.isUpstreamModelRestrictedByChannel(ctx, *groupID, account, requestedModel)
}

// ForwardCountTokens 转发 count_tokens 请求到上游 API
// 特点：不记录使用量、仅支持非流式响应
func (s *GatewayService) ForwardCountTokens(ctx context.Context, c *gin.Context, account *Account, parsed *ParsedRequest) error {
	if parsed == nil {
		s.countTokensError(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return fmt.Errorf("parse request: empty request")
	}

	if account != nil && account.IsAnthropicAPIKeyPassthroughEnabled() {
		passthroughBody := parsed.Body
		if reqModel := parsed.Model; reqModel != "" {
			if mappedModel := account.GetMappedModel(reqModel); mappedModel != reqModel {
				passthroughBody = s.replaceModelInBody(passthroughBody, mappedModel)
				logger.LegacyPrintf("service.gateway", "CountTokens passthrough model mapping: %s -> %s (account: %s)", reqModel, mappedModel, account.Name)
			}
		}
		return s.forwardCountTokensAnthropicAPIKeyPassthrough(ctx, c, account, passthroughBody)
	}

	// Bedrock 不支持 count_tokens 端点
	if account != nil && account.IsBedrock() {
		s.countTokensError(c, http.StatusNotFound, "not_found_error", "count_tokens endpoint is not supported for Bedrock")
		return nil
	}

	body := parsed.Body
	reqModel := parsed.Model

	// Pre-filter: strip empty text blocks to prevent upstream 400.
	body = StripEmptyTextBlocks(body)

	isClaudeCode := isClaudeCodeRequest(ctx, c, parsed)
	shouldMimicClaudeCode := account.IsOAuth() && !isClaudeCode

	if shouldMimicClaudeCode {
		normalizeOpts := claudeOAuthNormalizeOptions{stripSystemCacheControl: true}
		body, reqModel = normalizeClaudeOAuthRequestBody(body, reqModel, normalizeOpts)
	}

	// Antigravity 账户不支持 count_tokens，返回 404 让客户端 fallback 到本地估算。
	// 返回 nil 避免 handler 层记录为错误，也不设置 ops 上游错误上下文。
	if account.Platform == PlatformAntigravity {
		s.countTokensError(c, http.StatusNotFound, "not_found_error", "count_tokens endpoint is not supported for this platform")
		return nil
	}

	// 应用模型映射：
	// - APIKey 账号：使用账号级别的显式映射（如果配置），否则透传原始模型名
	// - OAuth/SetupToken 账号：使用 Anthropic 标准映射（短ID → 长ID）
	if reqModel != "" {
		mappedModel := reqModel
		mappingSource := ""
		if account.Type == AccountTypeAPIKey {
			mappedModel = account.GetMappedModel(reqModel)
			if mappedModel != reqModel {
				mappingSource = "account"
			}
		}
		if mappingSource == "" && account.Platform == PlatformAnthropic && account.Type != AccountTypeAPIKey {
			normalized := claude.NormalizeModelID(reqModel)
			if normalized != reqModel {
				mappedModel = normalized
				mappingSource = "prefix"
			}
		}
		if mappedModel != reqModel {
			body = s.replaceModelInBody(body, mappedModel)
			reqModel = mappedModel
			logger.LegacyPrintf("service.gateway", "CountTokens model mapping applied: %s -> %s (account: %s, source=%s)", parsed.Model, mappedModel, account.Name, mappingSource)
		}
	}

	// 获取凭证
	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to get access token")
		return err
	}

	// 构建上游请求
	upstreamReq, err := s.buildCountTokensRequest(ctx, c, account, body, token, tokenType, reqModel, shouldMimicClaudeCode)
	if err != nil {
		s.countTokensError(c, http.StatusInternalServerError, "api_error", "Failed to build request")
		return err
	}

	// 获取代理URL（自定义 base URL 模式下，proxy 通过 buildCustomRelayURL 作为查询参数传递）
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		if !account.IsCustomBaseURLEnabled() || account.GetCustomBaseURL() == "" {
			proxyURL = account.Proxy.URL()
		}
	}

	// 发送请求
	resp, err := s.httpUpstream.DoWithTLS(upstreamReq, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		setOpsUpstreamError(c, 0, sanitizeUpstreamErrorMessage(err.Error()), "")
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Request failed")
		return fmt.Errorf("upstream request failed: %w", err)
	}

	// 读取响应体
	maxReadBytes := resolveUpstreamResponseReadLimit(s.cfg)
	respBody, err := readUpstreamResponseBodyLimited(resp.Body, maxReadBytes)
	_ = resp.Body.Close()
	if err != nil {
		if errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			setOpsUpstreamError(c, http.StatusBadGateway, "upstream response too large", "")
			s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Upstream response too large")
			return err
		}
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to read response")
		return err
	}

	// 检测 thinking block 签名错误（400）并重试一次（过滤 thinking blocks）
	if resp.StatusCode == 400 && s.shouldRectifySignatureError(ctx, account, respBody) {
		logger.LegacyPrintf("service.gateway", "Account %d: detected thinking block signature error on count_tokens, retrying with filtered thinking blocks", account.ID)

		filteredBody := FilterThinkingBlocksForRetry(body)
		retryReq, buildErr := s.buildCountTokensRequest(ctx, c, account, filteredBody, token, tokenType, reqModel, shouldMimicClaudeCode)
		if buildErr == nil {
			retryResp, retryErr := s.httpUpstream.DoWithTLS(retryReq, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
			if retryErr == nil {
				resp = retryResp
				respBody, err = readUpstreamResponseBodyLimited(resp.Body, maxReadBytes)
				_ = resp.Body.Close()
				if err != nil {
					if errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
						setOpsUpstreamError(c, http.StatusBadGateway, "upstream response too large", "")
						s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Upstream response too large")
						return err
					}
					s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to read response")
					return err
				}
			}
		}
	}

	// 处理错误响应
	if resp.StatusCode >= 400 {
		// 标记账号状态（429/529等）
		s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)

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
		setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)

		// 记录上游错误摘要便于排障（不回显请求内容）
		if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
			logger.LegacyPrintf("service.gateway",
				"count_tokens upstream error %d (account=%d platform=%s type=%s): %s",
				resp.StatusCode,
				account.ID,
				account.Platform,
				account.Type,
				truncateForLog(respBody, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes),
			)
		}

		// 返回简化的错误响应
		errMsg := "Upstream request failed"
		switch resp.StatusCode {
		case 429:
			errMsg = "Rate limit exceeded"
		case 529:
			errMsg = "Service overloaded"
		}
		s.countTokensError(c, resp.StatusCode, "upstream_error", errMsg)
		if upstreamMsg == "" {
			return fmt.Errorf("upstream error: %d", resp.StatusCode)
		}
		return fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
	}

	// 透传成功响应
	c.Data(resp.StatusCode, "application/json", respBody)
	return nil
}

func (s *GatewayService) forwardCountTokensAnthropicAPIKeyPassthrough(ctx context.Context, c *gin.Context, account *Account, body []byte) error {
	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to get access token")
		return err
	}
	if tokenType != "apikey" {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Invalid account token type")
		return fmt.Errorf("anthropic api key passthrough requires apikey token, got: %s", tokenType)
	}

	upstreamReq, err := s.buildCountTokensRequestAnthropicAPIKeyPassthrough(ctx, c, account, body, token)
	if err != nil {
		s.countTokensError(c, http.StatusInternalServerError, "api_error", "Failed to build request")
		return err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.DoWithTLS(upstreamReq, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		setOpsUpstreamError(c, 0, sanitizeUpstreamErrorMessage(err.Error()), "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
			Passthrough:        true,
			Kind:               "request_error",
			Message:            sanitizeUpstreamErrorMessage(err.Error()),
		})
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Request failed")
		return fmt.Errorf("upstream request failed: %w", err)
	}

	maxReadBytes := resolveUpstreamResponseReadLimit(s.cfg)
	respBody, err := readUpstreamResponseBodyLimited(resp.Body, maxReadBytes)
	_ = resp.Body.Close()
	if err != nil {
		if errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			setOpsUpstreamError(c, http.StatusBadGateway, "upstream response too large", "")
			s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Upstream response too large")
			return err
		}
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to read response")
		return err
	}

	if resp.StatusCode >= 400 {
		if s.rateLimitService != nil {
			s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		}

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)

		// 中转站不支持 count_tokens 端点时（404），返回 404 让客户端 fallback 到本地估算。
		// 仅在错误消息明确指向 count_tokens endpoint 不存在时生效，避免误吞其他 404（如错误 base_url）。
		// 返回 nil 避免 handler 层记录为错误，也不设置 ops 上游错误上下文。
		if isCountTokensUnsupported404(resp.StatusCode, respBody) {
			logger.LegacyPrintf("service.gateway",
				"[count_tokens] Upstream does not support count_tokens (404), returning 404: account=%d name=%s msg=%s",
				account.ID, account.Name, truncateString(upstreamMsg, 512))
			s.countTokensError(c, http.StatusNotFound, "not_found_error", "count_tokens endpoint is not supported by upstream")
			return nil
		}

		upstreamDetail := ""
		if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
			maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
			if maxBytes <= 0 {
				maxBytes = 2048
			}
			upstreamDetail = truncateString(string(respBody), maxBytes)
		}
		setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
			Passthrough:        true,
			Kind:               "http_error",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
		})

		errMsg := "Upstream request failed"
		switch resp.StatusCode {
		case 429:
			errMsg = "Rate limit exceeded"
		case 529:
			errMsg = "Service overloaded"
		}
		s.countTokensError(c, resp.StatusCode, "upstream_error", errMsg)
		if upstreamMsg == "" {
			return fmt.Errorf("upstream error: %d", resp.StatusCode)
		}
		return fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
	}

	writeAnthropicPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, respBody)
	return nil
}

func (s *GatewayService) buildCountTokensRequestAnthropicAPIKeyPassthrough(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
) (*http.Request, error) {
	targetURL := claudeAPICountTokensURL
	baseURL := account.GetBaseURL()
	if baseURL != "" {
		validatedURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, err
		}
		targetURL = validatedURL + "/v1/messages/count_tokens?beta=true"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			if !allowedHeaders[lowerKey] {
				continue
			}
			wireKey := resolveWireCasing(key)
			for _, v := range values {
				addHeaderRaw(req.Header, wireKey, v)
			}
		}
	}

	req.Header.Del("authorization")
	req.Header.Del("x-api-key")
	req.Header.Del("x-goog-api-key")
	req.Header.Del("cookie")
	req.Header.Set("x-api-key", token)

	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
	}
	if req.Header.Get("anthropic-version") == "" {
		req.Header.Set("anthropic-version", "2023-06-01")
	}

	return req, nil
}

// buildCountTokensRequest 构建 count_tokens 上游请求
func (s *GatewayService) buildCountTokensRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token, tokenType, modelID string, mimicClaudeCode bool) (*http.Request, error) {
	// 确定目标 URL
	targetURL := claudeAPICountTokensURL
	if account.Type == AccountTypeAPIKey {
		baseURL := account.GetBaseURL()
		if baseURL != "" {
			validatedURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, err
			}
			targetURL = validatedURL + "/v1/messages/count_tokens?beta=true"
		}
	} else if account.IsCustomBaseURLEnabled() {
		customURL := account.GetCustomBaseURL()
		if customURL == "" {
			return nil, fmt.Errorf("custom_base_url is enabled but not configured for account %d", account.ID)
		}
		validatedURL, err := s.validateUpstreamBaseURL(customURL)
		if err != nil {
			return nil, err
		}
		targetURL = s.buildCustomRelayURL(validatedURL, "/v1/messages/count_tokens", account)
	}

	clientHeaders := http.Header{}
	if c != nil && c.Request != nil {
		clientHeaders = c.Request.Header
	}

	// OAuth 账号：应用统一指纹和重写 userID（受设置开关控制）
	// 如果启用了会话ID伪装，会在重写后替换 session 部分为固定值
	ctEnableFP, ctEnableMPT, ctEnableCCH := true, false, false
	if s.settingService != nil {
		ctEnableFP, ctEnableMPT, ctEnableCCH = s.settingService.GetGatewayForwardingSettings(ctx)
	}
	var ctFingerprint *Fingerprint
	if account.IsOAuth() && s.identityService != nil {
		fp, err := s.identityService.GetOrCreateFingerprint(ctx, account.ID, clientHeaders)
		if err == nil {
			ctFingerprint = fp
			if !ctEnableMPT {
				accountUUID := account.GetExtraString("account_uuid")
				if accountUUID != "" && fp.ClientID != "" {
					if newBody, err := s.identityService.RewriteUserIDWithMasking(ctx, body, account, accountUUID, fp.ClientID, fp.UserAgent); err == nil && len(newBody) > 0 {
						body = newBody
					}
				}
			}
		}
	}

	// 同步 billing header cc_version 与实际发送的 User-Agent 版本
	if ctFingerprint != nil && ctEnableFP {
		body = syncBillingHeaderVersion(body, ctFingerprint.UserAgent)
	}
	if ctEnableCCH {
		body = signBillingHeaderCCH(body)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// 设置认证头（保持原始大小写）
	if tokenType == "oauth" {
		setHeaderRaw(req.Header, "authorization", "Bearer "+token)
	} else {
		setHeaderRaw(req.Header, "x-api-key", token)
	}

	// 白名单透传 headers（恢复真实 wire casing）
	for key, values := range clientHeaders {
		lowerKey := strings.ToLower(key)
		if allowedHeaders[lowerKey] {
			wireKey := resolveWireCasing(key)
			for _, v := range values {
				addHeaderRaw(req.Header, wireKey, v)
			}
		}
	}

	// OAuth 账号：应用指纹到请求头（受设置开关控制）
	if ctEnableFP && ctFingerprint != nil {
		s.identityService.ApplyFingerprint(req, ctFingerprint)
	}

	// 确保必要的 headers 存在（保持原始大小写）
	if getHeaderRaw(req.Header, "content-type") == "" {
		setHeaderRaw(req.Header, "content-type", "application/json")
	}
	if getHeaderRaw(req.Header, "anthropic-version") == "" {
		setHeaderRaw(req.Header, "anthropic-version", "2023-06-01")
	}
	if tokenType == "oauth" {
		applyClaudeOAuthHeaderDefaults(req)
	}

	// Build effective drop set for count_tokens: merge static defaults with dynamic beta policy filter rules
	ctEffectiveDropSet := mergeDropSets(s.getBetaPolicyFilterSet(ctx, c, account, modelID))

	// OAuth 账号：处理 anthropic-beta header
	if tokenType == "oauth" {
		if mimicClaudeCode {
			applyClaudeCodeMimicHeaders(req, false)

			incomingBeta := getHeaderRaw(req.Header, "anthropic-beta")
			requiredBetas := []string{claude.BetaClaudeCode, claude.BetaOAuth, claude.BetaInterleavedThinking, claude.BetaTokenCounting}
			setHeaderRaw(req.Header, "anthropic-beta", mergeAnthropicBetaDropping(requiredBetas, incomingBeta, ctEffectiveDropSet))
		} else {
			clientBetaHeader := getHeaderRaw(req.Header, "anthropic-beta")
			if clientBetaHeader == "" {
				setHeaderRaw(req.Header, "anthropic-beta", claude.CountTokensBetaHeader)
			} else {
				beta := s.getBetaHeader(modelID, clientBetaHeader)
				if !strings.Contains(beta, claude.BetaTokenCounting) {
					beta = beta + "," + claude.BetaTokenCounting
				}
				setHeaderRaw(req.Header, "anthropic-beta", stripBetaTokensWithSet(beta, ctEffectiveDropSet))
			}
		}
	} else {
		// API-key accounts: apply beta policy filter to strip controlled tokens
		if existingBeta := getHeaderRaw(req.Header, "anthropic-beta"); existingBeta != "" {
			setHeaderRaw(req.Header, "anthropic-beta", stripBetaTokensWithSet(existingBeta, ctEffectiveDropSet))
		} else if s.cfg != nil && s.cfg.Gateway.InjectBetaForAPIKey {
			// API-key：与 messages 同步的按需 beta 注入（默认关闭）
			if requestNeedsBetaFeatures(body) {
				if beta := defaultAPIKeyBetaHeader(body); beta != "" {
					setHeaderRaw(req.Header, "anthropic-beta", beta)
				}
			}
		}
	}

	// 同步 X-Claude-Code-Session-Id 头：取 body 中已处理的 metadata.user_id 的 session_id 覆盖
	if sessionHeader := getHeaderRaw(req.Header, "X-Claude-Code-Session-Id"); sessionHeader != "" {
		if uid := gjson.GetBytes(body, "metadata.user_id").String(); uid != "" {
			if parsed := ParseMetadataUserID(uid); parsed != nil {
				setHeaderRaw(req.Header, "X-Claude-Code-Session-Id", parsed.SessionID)
			}
		}
	}

	if c != nil && tokenType == "oauth" {
		c.Set(claudeMimicDebugInfoKey, buildClaudeMimicDebugLine(req, body, account, tokenType, mimicClaudeCode))
	}
	if s.debugClaudeMimicEnabled() {
		logClaudeMimicDebug(req, body, account, tokenType, mimicClaudeCode)
	}

	return req, nil
}

// countTokensError 返回 count_tokens 错误响应
func (s *GatewayService) countTokensError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// buildCustomRelayURL 构建自定义中继转发 URL
// 在 path 后附加 beta=true 和可选的 proxy 查询参数
func (s *GatewayService) buildCustomRelayURL(baseURL, path string, account *Account) string {
	u := strings.TrimRight(baseURL, "/") + path + "?beta=true"
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL := account.Proxy.URL()
		if proxyURL != "" {
			u += "&proxy=" + url.QueryEscape(proxyURL)
		}
	}
	return u
}

func (s *GatewayService) validateUpstreamBaseURL(raw string) (string, error) {
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

// GetAvailableModels returns the list of models available for a group
// It aggregates model_mapping keys from all schedulable accounts in the group
func (s *GatewayService) GetAvailableModels(ctx context.Context, groupID *int64, platform string) []string {
	cacheKey := modelsListCacheKey(groupID, platform)
	if s.modelsListCache != nil {
		if cached, found := s.modelsListCache.Get(cacheKey); found {
			if models, ok := cached.([]string); ok {
				modelsListCacheHitTotal.Add(1)
				return cloneStringSlice(models)
			}
		}
	}
	modelsListCacheMissTotal.Add(1)

	var accounts []Account
	var err error

	if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupID(ctx, *groupID)
	} else {
		accounts, err = s.accountRepo.ListSchedulable(ctx)
	}

	if err != nil || len(accounts) == 0 {
		return nil
	}

	// Filter by platform if specified
	if platform != "" {
		filtered := make([]Account, 0)
		for _, acc := range accounts {
			if acc.Platform == platform {
				filtered = append(filtered, acc)
			}
		}
		accounts = filtered
	}

	// Collect unique models from all accounts
	modelSet := make(map[string]struct{})
	hasAnyMapping := false

	for _, acc := range accounts {
		mapping := acc.GetModelMapping()
		if len(mapping) > 0 {
			hasAnyMapping = true
			for model := range mapping {
				modelSet[model] = struct{}{}
			}
		}
	}

	// If no account has model_mapping, return nil (use default)
	if !hasAnyMapping {
		if s.modelsListCache != nil {
			s.modelsListCache.Set(cacheKey, []string(nil), s.modelsListCacheTTL)
			modelsListCacheStoreTotal.Add(1)
		}
		return nil
	}

	// Convert to slice
	models := make([]string, 0, len(modelSet))
	for model := range modelSet {
		models = append(models, model)
	}
	sort.Strings(models)

	if s.modelsListCache != nil {
		s.modelsListCache.Set(cacheKey, cloneStringSlice(models), s.modelsListCacheTTL)
		modelsListCacheStoreTotal.Add(1)
	}
	return cloneStringSlice(models)
}

func (s *GatewayService) InvalidateAvailableModelsCache(groupID *int64, platform string) {
	if s == nil || s.modelsListCache == nil {
		return
	}

	normalizedPlatform := strings.TrimSpace(platform)
	// 完整匹配时精准失效；否则按维度批量失效。
	if groupID != nil && normalizedPlatform != "" {
		s.modelsListCache.Delete(modelsListCacheKey(groupID, normalizedPlatform))
		return
	}

	targetGroup := derefGroupID(groupID)
	for key := range s.modelsListCache.Items() {
		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 {
			continue
		}
		groupPart, parseErr := strconv.ParseInt(parts[0], 10, 64)
		if parseErr != nil {
			continue
		}
		if groupID != nil && groupPart != targetGroup {
			continue
		}
		if normalizedPlatform != "" && parts[1] != normalizedPlatform {
			continue
		}
		s.modelsListCache.Delete(key)
	}
}

// reconcileCachedTokens 兼容 Kimi 等上游：
// 将 OpenAI 风格的 cached_tokens 映射到 Claude 标准的 cache_read_input_tokens
func reconcileCachedTokens(usage map[string]any) bool {
	if usage == nil {
		return false
	}
	cacheRead, _ := usage["cache_read_input_tokens"].(float64)
	if cacheRead > 0 {
		return false // 已有标准字段，无需处理
	}
	cached, _ := usage["cached_tokens"].(float64)
	if cached <= 0 {
		return false
	}
	usage["cache_read_input_tokens"] = cached
	return true
}

const debugGatewayBodyDefaultFilename = "gateway_debug.log"

// initDebugGatewayBodyFile 初始化网关调试日志文件。
//
//   - "1"/"true" 等布尔值 → 当前目录下 gateway_debug.log
//   - 已有目录路径        → 该目录下 gateway_debug.log
//   - 其他               → 视为完整文件路径
func (s *GatewayService) initDebugGatewayBodyFile(path string) {
	if parseDebugEnvBool(path) {
		path = debugGatewayBodyDefaultFilename
	}

	// 如果 path 指向一个已存在的目录，自动追加默认文件名
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		path = filepath.Join(path, debugGatewayBodyDefaultFilename)
	}

	// 确保父目录存在
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			slog.Error("failed to create gateway debug log directory", "dir", dir, "error", err)
			return
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		slog.Error("failed to open gateway debug log file", "path", path, "error", err)
		return
	}
	s.debugGatewayBodyFile.Store(f)
	slog.Info("gateway debug logging enabled", "path", path)
}

// debugLogGatewaySnapshot 将网关请求的完整快照（headers + body）写入独立的调试日志文件，
// 用于对比客户端原始请求和上游转发请求。
//
// 启用方式（环境变量）：
//
//	SUB2API_DEBUG_GATEWAY_BODY=1                          # 写入 gateway_debug.log
//	SUB2API_DEBUG_GATEWAY_BODY=/tmp/gateway_debug.log     # 写入指定路径
//
// tag: "CLIENT_ORIGINAL" 或 "UPSTREAM_FORWARD"
func (s *GatewayService) debugLogGatewaySnapshot(tag string, headers http.Header, body []byte, extra map[string]string) {
	f := s.debugGatewayBodyFile.Load()
	if f == nil {
		return
	}

	var buf strings.Builder
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	fmt.Fprintf(&buf, "\n========== [%s] %s ==========\n", ts, tag)

	// 1. context
	if len(extra) > 0 {
		fmt.Fprint(&buf, "--- context ---\n")
		extraKeys := make([]string, 0, len(extra))
		for k := range extra {
			extraKeys = append(extraKeys, k)
		}
		sort.Strings(extraKeys)
		for _, k := range extraKeys {
			fmt.Fprintf(&buf, "  %s: %s\n", k, extra[k])
		}
	}

	// 2. headers（按真实 Claude CLI wire 顺序排列，便于与抓包对比；auth 脱敏）
	fmt.Fprint(&buf, "--- headers ---\n")
	for _, k := range sortHeadersByWireOrder(headers) {
		for _, v := range headers[k] {
			fmt.Fprintf(&buf, "  %s: %s\n", k, safeHeaderValueForLog(k, v))
		}
	}

	// 3. body（完整输出，格式化 JSON 便于 diff）
	fmt.Fprint(&buf, "--- body ---\n")
	if len(body) == 0 {
		fmt.Fprint(&buf, "  (empty)\n")
	} else {
		var pretty bytes.Buffer
		if json.Indent(&pretty, body, "  ", "  ") == nil {
			fmt.Fprintf(&buf, "  %s\n", pretty.Bytes())
		} else {
			// JSON 格式化失败时原样输出
			fmt.Fprintf(&buf, "  %s\n", body)
		}
	}

	// 写入文件（调试用，并发写入可能交错但不影响可读性）
	_, _ = f.WriteString(buf.String())
}

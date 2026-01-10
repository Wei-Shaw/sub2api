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
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/gin-gonic/gin"
)

const (
	claudeAPIURL            = "https://api.anthropic.com/v1/messages?beta=true"
	claudeAPICountTokensURL = "https://api.anthropic.com/v1/messages/count_tokens?beta=true"
	stickySessionTTL        = time.Hour // 粘性会话TTL
	defaultMaxLineSize      = 40 * 1024 * 1024
	claudeCodeSystemPrompt  = "You are Claude Code, Anthropic's official CLI for Claude."
	maxCacheControlBlocks   = 4 // Anthropic API 允许的最大 cache_control 块数量
)

// sseDataRe matches SSE data lines with optional whitespace after colon.
// Some upstream APIs return non-standard "data:" without space (should be "data: ").
var (
	sseDataRe            = regexp.MustCompile(`^data:\s*`)
	sessionIDRegex       = regexp.MustCompile(`session_([a-f0-9-]{36REDACTED)`)
	claudeCliUserAgentRe = regexp.MustCompile(`^claude-cli/\d+\.\d+\.\d+`)

	// claudeCodePromptPrefixes 用于检测 Claude Code 系统提示词的前缀列表
	// 支持多种变体：标准版、Agent SDK 版、Explore Agent 版、Compact 版等
	// 注意：前缀之间不应存在包含关系，否则会导致冗余匹配
	claudeCodePromptPrefixes = []string{
		"You are Claude Code, Anthropic's official CLI for Claude",             // 标准版 & Agent SDK 版（含 running within...）
		"You are a Claude agent, built on Anthropic's Claude Agent SDK",        // Agent SDK 变体
		"You are a file search specialist for Claude Code",                     // Explore Agent 版
		"You are a helpful AI assistant tasked with summarizing conversations", // Compact 版
REDACTED
)

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
REDACTED

// GatewayCache defines cache operations for gateway service
type GatewayCache interface {
	GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error)
	SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error
	RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error
REDACTED

// derefGroupID safely dereferences *int64 to int64, returning 0 if nil
func derefGroupID(groupID *int64) int64 {
	if groupID == nil {
		return 0
REDACTED
	return *groupID
REDACTED

type AccountWaitPlan struct {
	AccountID      int64
	MaxConcurrency int
	Timeout        time.Duration
	MaxWaiting     int
REDACTED

type AccountSelectionResult struct {
	Account     *Account
	Acquired    bool
	ReleaseFunc func()
	WaitPlan    *AccountWaitPlan // nil means no wait allowed
REDACTED

// ClaudeUsage 表示Claude API返回的usage信息
type ClaudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
REDACTED

// ForwardResult 转发结果
type ForwardResult struct {
	RequestID        string
	Usage            ClaudeUsage
	Model            string
	Stream           bool
	Duration         time.Duration
	FirstTokenMs     *int // 首字时间（流式请求）
	ClientDisconnect bool // 客户端是否在流式传输过程中断开

	// 图片生成计费字段（仅 gemini-3-pro-image 使用）
	ImageCount int    // 生成的图片数量
	ImageSize  string // 图片尺寸 "1K", "2K", "4K"
REDACTED

// UpstreamFailoverError indicates an upstream error that should trigger account failover.
type UpstreamFailoverError struct {
	StatusCode int
REDACTED

func (e *UpstreamFailoverError) Error() string {
	return fmt.Sprintf("upstream error: %d (failover)", e.StatusCode)
REDACTED

// GatewayService handles API gateway operations
type GatewayService struct {
	accountRepo         AccountRepository
	groupRepo           GroupRepository
	usageLogRepo        UsageLogRepository
	userRepo            UserRepository
	userSubRepo         UserSubscriptionRepository
	cache               GatewayCache
	cfg                 *config.Config
	billingService      *BillingService
	rateLimitService    *RateLimitService
	billingCacheService *BillingCacheService
	identityService     *IdentityService
	httpUpstream        HTTPUpstream
	deferredService     *DeferredService
	concurrencyService  *ConcurrencyService
REDACTED

// NewGatewayService creates a new GatewayService
func NewGatewayService(
	accountRepo AccountRepository,
	groupRepo GroupRepository,
	usageLogRepo UsageLogRepository,
	userRepo UserRepository,
	userSubRepo UserSubscriptionRepository,
	cache GatewayCache,
	cfg *config.Config,
	concurrencyService *ConcurrencyService,
	billingService *BillingService,
	rateLimitService *RateLimitService,
	billingCacheService *BillingCacheService,
	identityService *IdentityService,
	httpUpstream HTTPUpstream,
	deferredService *DeferredService,
) *GatewayService {
	return &GatewayService{
		accountRepo:         accountRepo,
		groupRepo:           groupRepo,
		usageLogRepo:        usageLogRepo,
		userRepo:            userRepo,
		userSubRepo:         userSubRepo,
		cache:               cache,
		cfg:                 cfg,
		concurrencyService:  concurrencyService,
		billingService:      billingService,
		rateLimitService:    rateLimitService,
		billingCacheService: billingCacheService,
		identityService:     identityService,
		httpUpstream:        httpUpstream,
		deferredService:     deferredService,
REDACTED
REDACTED

// GenerateSessionHash 从预解析请求计算粘性会话 hash
func (s *GatewayService) GenerateSessionHash(parsed *ParsedRequest) string {
	if parsed == nil {
		return ""
REDACTED

	// 1. 最高优先级：从 metadata.user_id 提取 session_xxx
	if parsed.MetadataUserID != "" {
		if match := sessionIDRegex.FindStringSubmatch(parsed.MetadataUserID); len(match) > 1 {
			return match[1]
	REDACTED
REDACTED

	// 2. 提取带 cache_control: {type: "ephemeral"REDACTED 的内容
	cacheableContent := s.extractCacheableContent(parsed)
	if cacheableContent != "" {
		return s.hashContent(cacheableContent)
REDACTED

	// 3. Fallback: 使用 system 内容
	if parsed.System != nil {
		systemText := s.extractTextFromSystem(parsed.System)
		if systemText != "" {
			return s.hashContent(systemText)
	REDACTED
REDACTED

	// 4. 最后 fallback: 使用第一条消息
	if len(parsed.Messages) > 0 {
		if firstMsg, ok := parsed.Messages[0].(map[string]any); ok {
			msgText := s.extractTextFromContent(firstMsg["content"])
			if msgText != "" {
				return s.hashContent(msgText)
		REDACTED
	REDACTED
REDACTED

	return ""
REDACTED

// BindStickySession sets session -> account binding with standard TTL.
func (s *GatewayService) BindStickySession(ctx context.Context, groupID *int64, sessionHash string, accountID int64) error {
	if sessionHash == "" || accountID <= 0 || s.cache == nil {
		return nil
REDACTED
	return s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, accountID, stickySessionTTL)
REDACTED

func (s *GatewayService) extractCacheableContent(parsed *ParsedRequest) string {
	if parsed == nil {
		return ""
REDACTED

	var builder strings.Builder

	// 检查 system 中的 cacheable 内容
	if system, ok := parsed.System.([]any); ok {
		for _, part := range system {
			if partMap, ok := part.(map[string]any); ok {
				if cc, ok := partMap["cache_control"].(map[string]any); ok {
					if cc["type"] == "ephemeral" {
						if text, ok := partMap["text"].(string); ok {
							_, _ = builder.WriteString(text)
					REDACTED
				REDACTED
			REDACTED
		REDACTED
	REDACTED
REDACTED
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
						REDACTED
					REDACTED
				REDACTED
			REDACTED
		REDACTED
	REDACTED
REDACTED

	return systemText
REDACTED

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
			REDACTED
		REDACTED
	REDACTED
		return strings.Join(texts, "")
REDACTED
	return ""
REDACTED

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
				REDACTED
			REDACTED
		REDACTED
	REDACTED
		return strings.Join(texts, "")
REDACTED
	return ""
REDACTED

func (s *GatewayService) hashContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:16]) // 32字符
REDACTED

// replaceModelInBody 替换请求体中的model字段
func (s *GatewayService) replaceModelInBody(body []byte, newModel string) []byte {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body
REDACTED
	req["model"] = newModel
	newBody, err := json.Marshal(req)
	if err != nil {
		return body
REDACTED
	return newBody
REDACTED

// SelectAccount 选择账号（粘性会话+优先级）
func (s *GatewayService) SelectAccount(ctx context.Context, groupID *int64, sessionHash string) (*Account, error) {
	return s.SelectAccountForModel(ctx, groupID, sessionHash, "")
REDACTED

// SelectAccountForModel 选择支持指定模型的账号（粘性会话+优先级+模型映射）
func (s *GatewayService) SelectAccountForModel(ctx context.Context, groupID *int64, sessionHash string, requestedModel string) (*Account, error) {
	return s.SelectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, nil)
REDACTED

// SelectAccountForModelWithExclusions selects an account supporting the requested model while excluding specified accounts.
func (s *GatewayService) SelectAccountForModelWithExclusions(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{REDACTED) (*Account, error) {
	// 优先检查 context 中的强制平台（/antigravity 路由）
	var platform string
	forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string)
	if hasForcePlatform && forcePlatform != "" {
		platform = forcePlatform
REDACTED else if groupID != nil {
		group, resolvedGroupID, err := s.resolveGatewayGroup(ctx, groupID)
		if err != nil {
			return nil, err
	REDACTED
		groupID = resolvedGroupID
		ctx = s.withGroupContext(ctx, group)
		platform = group.Platform
REDACTED else {
		// 无分组时只使用原生 anthropic 平台
		platform = PlatformAnthropic
REDACTED

	// anthropic/gemini 分组支持混合调度（包含启用了 mixed_scheduling 的 antigravity 账户）
	// 注意：强制平台模式不走混合调度
	if (platform == PlatformAnthropic || platform == PlatformGemini) && !hasForcePlatform {
		return s.selectAccountWithMixedScheduling(ctx, groupID, sessionHash, requestedModel, excludedIDs, platform)
REDACTED

	// antigravity 分组、强制平台模式或无分组使用单平台选择
	// 注意：强制平台模式也必须遵守分组限制，不再回退到全平台查询
	return s.selectAccountForModelWithPlatform(ctx, groupID, sessionHash, requestedModel, excludedIDs, platform)
REDACTED

// SelectAccountWithLoadAwareness selects account with load-awareness and wait plan.
func (s *GatewayService) SelectAccountWithLoadAwareness(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{REDACTED) (*AccountSelectionResult, error) {
	cfg := s.schedulingConfig()
	var stickyAccountID int64
	if sessionHash != "" && s.cache != nil {
		if accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash); err == nil {
			stickyAccountID = accountID
	REDACTED
REDACTED

	// 检查 Claude Code 客户端限制（可能会替换 groupID 为降级分组）
	group, groupID, err := s.checkClaudeCodeRestriction(ctx, groupID)
	if err != nil {
		return nil, err
REDACTED
	ctx = s.withGroupContext(ctx, group)

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

	platform, hasForcePlatform, err := s.resolvePlatform(ctx, groupID, group)
	if err != nil {
		return nil, err
REDACTED
	preferOAuth := platform == PlatformGemini

	accounts, useMixed, err := s.listSchedulableAccounts(ctx, groupID, platform, hasForcePlatform)
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

	// ============ Layer 1: 粘性会话优先 ============
	if sessionHash != "" && s.cache != nil {
		accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
		if err == nil && accountID > 0 && !isExcluded(accountID) {
			// 粘性命中仅在当前可调度候选集中生效。
			accountByID := make(map[int64]*Account, len(accounts))
			for i := range accounts {
				accountByID[accounts[i].ID] = &accounts[i]
		REDACTED
			account, ok := accountByID[accountID]
			if ok && s.isAccountInGroup(account, groupID) &&
				s.isAccountAllowedForPlatform(account, platform, useMixed) &&
				account.IsSchedulableForModel(requestedModel) &&
				(requestedModel == "" || s.isModelSupportedByAccount(account, requestedModel)) {
				result, err := s.tryAcquireAccountSlot(ctx, accountID, account.Concurrency)
				if err == nil && result.Acquired {
					_ = s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), sessionHash, stickySessionTTL)
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

	// ============ Layer 2: 负载感知选择 ============
	candidates := make([]*Account, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if isExcluded(acc.ID) {
			continue
	REDACTED
		if !s.isAccountAllowedForPlatform(acc, platform, useMixed) {
			continue
	REDACTED
		if !acc.IsSchedulableForModel(requestedModel) {
			continue
	REDACTED
		if requestedModel != "" && !s.isModelSupportedByAccount(acc, requestedModel) {
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
		if result, ok := s.tryAcquireByLegacyOrder(ctx, candidates, groupID, sessionHash, preferOAuth); ok {
			return result, nil
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
					if preferOAuth && a.account.Type != b.account.Type {
						return a.account.Type == AccountTypeOAuth
				REDACTED
					return false
				default:
					return a.account.LastUsedAt.Before(*b.account.LastUsedAt)
			REDACTED
		REDACTED)

			for _, item := range available {
				result, err := s.tryAcquireAccountSlot(ctx, item.account.ID, item.account.Concurrency)
				if err == nil && result.Acquired {
					if sessionHash != "" && s.cache != nil {
						_ = s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, item.account.ID, stickySessionTTL)
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

	// ============ Layer 3: 兜底排队 ============
	sortAccountsByPriorityAndLastUsed(candidates, preferOAuth)
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

func (s *GatewayService) tryAcquireByLegacyOrder(ctx context.Context, candidates []*Account, groupID *int64, sessionHash string, preferOAuth bool) (*AccountSelectionResult, bool) {
	ordered := append([]*Account(nil), candidates...)
	sortAccountsByPriorityAndLastUsed(ordered, preferOAuth)

	for _, acc := range ordered {
		result, err := s.tryAcquireAccountSlot(ctx, acc.ID, acc.Concurrency)
		if err == nil && result.Acquired {
			if sessionHash != "" && s.cache != nil {
				_ = s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, acc.ID, stickySessionTTL)
		REDACTED
			return &AccountSelectionResult{
				Account:     acc,
				Acquired:    true,
				ReleaseFunc: result.ReleaseFunc,
		REDACTED, true
	REDACTED
REDACTED

	return nil, false
REDACTED

func (s *GatewayService) schedulingConfig() config.GatewaySchedulingConfig {
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

func (s *GatewayService) withGroupContext(ctx context.Context, group *Group) context.Context {
	if !IsGroupContextValid(group) {
		return ctx
REDACTED
	if existing, ok := ctx.Value(ctxkey.Group).(*Group); ok && existing != nil && existing.ID == group.ID && IsGroupContextValid(existing) {
		return ctx
REDACTED
	return context.WithValue(ctx, ctxkey.Group, group)
REDACTED

func (s *GatewayService) groupFromContext(ctx context.Context, groupID int64) *Group {
	if group, ok := ctx.Value(ctxkey.Group).(*Group); ok && IsGroupContextValid(group) && group.ID == groupID {
		return group
REDACTED
	return nil
REDACTED

func (s *GatewayService) resolveGroupByID(ctx context.Context, groupID int64) (*Group, error) {
	if group := s.groupFromContext(ctx, groupID); group != nil {
		return group, nil
REDACTED
	group, err := s.groupRepo.GetByIDLite(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("get group failed: %w", err)
REDACTED
	return group, nil
REDACTED

func (s *GatewayService) resolveGatewayGroup(ctx context.Context, groupID *int64) (*Group, *int64, error) {
	if groupID == nil {
		return nil, nil, nil
REDACTED

	currentID := *groupID
	visited := map[int64]struct{REDACTED{REDACTED
	for {
		if _, seen := visited[currentID]; seen {
			return nil, nil, fmt.Errorf("fallback group cycle detected")
	REDACTED
		visited[currentID] = struct{REDACTED{REDACTED

		group, err := s.resolveGroupByID(ctx, currentID)
		if err != nil {
			return nil, nil, err
	REDACTED

		if !group.ClaudeCodeOnly || IsClaudeCodeClient(ctx) {
			return group, &currentID, nil
	REDACTED

		if group.FallbackGroupID == nil {
			return nil, nil, ErrClaudeCodeOnly
	REDACTED
		currentID = *group.FallbackGroupID
REDACTED
REDACTED

// checkClaudeCodeRestriction 检查分组的 Claude Code 客户端限制
// 如果分组启用了 claude_code_only 且请求不是来自 Claude Code 客户端：
//   - 有降级分组：返回降级分组的 ID
//   - 无降级分组：返回 ErrClaudeCodeOnly 错误
func (s *GatewayService) checkClaudeCodeRestriction(ctx context.Context, groupID *int64) (*Group, *int64, error) {
	if groupID == nil {
		return nil, groupID, nil
REDACTED

	// 强制平台模式不检查 Claude Code 限制
	if _, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string); hasForcePlatform {
		return nil, groupID, nil
REDACTED

	group, resolvedID, err := s.resolveGatewayGroup(ctx, groupID)
	if err != nil {
		return nil, nil, err
REDACTED

	return group, resolvedID, nil
REDACTED

func (s *GatewayService) resolvePlatform(ctx context.Context, groupID *int64, group *Group) (string, bool, error) {
	forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string)
	if hasForcePlatform && forcePlatform != "" {
		return forcePlatform, true, nil
REDACTED
	if group != nil {
		return group.Platform, false, nil
REDACTED
	if groupID != nil {
		group, err := s.resolveGroupByID(ctx, *groupID)
		if err != nil {
			return "", false, err
	REDACTED
		return group.Platform, false, nil
REDACTED
	return PlatformAnthropic, false, nil
REDACTED

func (s *GatewayService) listSchedulableAccounts(ctx context.Context, groupID *int64, platform string, hasForcePlatform bool) ([]Account, bool, error) {
	useMixed := (platform == PlatformAnthropic || platform == PlatformGemini) && !hasForcePlatform
	if useMixed {
		platforms := []string{platform, PlatformAntigravityREDACTED
		var accounts []Account
		var err error
		if groupID != nil {
			accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatforms(ctx, *groupID, platforms)
	REDACTED else {
			accounts, err = s.accountRepo.ListSchedulableByPlatforms(ctx, platforms)
	REDACTED
		if err != nil {
			return nil, useMixed, err
	REDACTED
		filtered := make([]Account, 0, len(accounts))
		for _, acc := range accounts {
			if acc.Platform == PlatformAntigravity && !acc.IsMixedSchedulingEnabled() {
				continue
		REDACTED
			filtered = append(filtered, acc)
	REDACTED
		return filtered, useMixed, nil
REDACTED

	var accounts []Account
	var err error
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, platform)
REDACTED else if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, platform)
		// 分组内无账号则返回空列表，由上层处理错误，不再回退到全平台查询
REDACTED else {
		accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, platform)
REDACTED
	if err != nil {
		return nil, useMixed, err
REDACTED
	return accounts, useMixed, nil
REDACTED

func (s *GatewayService) isAccountAllowedForPlatform(account *Account, platform string, useMixed bool) bool {
	if account == nil {
		return false
REDACTED
	if useMixed {
		if account.Platform == platform {
			return true
	REDACTED
		return account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled()
REDACTED
	return account.Platform == platform
REDACTED

// isAccountInGroup checks if the account belongs to the specified group.
// Returns true if groupID is nil (no group restriction) or account belongs to the group.
func (s *GatewayService) isAccountInGroup(account *Account, groupID *int64) bool {
	if groupID == nil {
		return true // 无分组限制
REDACTED
	if account == nil {
		return false
REDACTED
	for _, ag := range account.AccountGroups {
		if ag.GroupID == *groupID {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func (s *GatewayService) tryAcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (*AcquireResult, error) {
	if s.concurrencyService == nil {
		return &AcquireResult{Acquired: true, ReleaseFunc: func() {REDACTEDREDACTED, nil
REDACTED
	return s.concurrencyService.AcquireAccountSlot(ctx, accountID, maxConcurrency)
REDACTED

func sortAccountsByPriorityAndLastUsed(accounts []*Account, preferOAuth bool) {
	sort.SliceStable(accounts, func(i, j int) bool {
		a, b := accounts[i], accounts[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
	REDACTED
		switch {
		case a.LastUsedAt == nil && b.LastUsedAt != nil:
			return true
		case a.LastUsedAt != nil && b.LastUsedAt == nil:
			return false
		case a.LastUsedAt == nil && b.LastUsedAt == nil:
			if preferOAuth && a.Type != b.Type {
				return a.Type == AccountTypeOAuth
		REDACTED
			return false
		default:
			return a.LastUsedAt.Before(*b.LastUsedAt)
	REDACTED
REDACTED)
REDACTED

// selectAccountForModelWithPlatform 选择单平台账户（完全隔离）
func (s *GatewayService) selectAccountForModelWithPlatform(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{REDACTED, platform string) (*Account, error) {
	preferOAuth := platform == PlatformGemini
	// 1. 查询粘性会话
	if sessionHash != "" && s.cache != nil {
		accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
		if err == nil && accountID > 0 {
			if _, excluded := excludedIDs[accountID]; !excluded {
				account, err := s.accountRepo.GetByID(ctx, accountID)
				// 检查账号分组归属和平台匹配（确保粘性会话不会跨分组或跨平台）
				if err == nil && s.isAccountInGroup(account, groupID) && account.Platform == platform && account.IsSchedulableForModel(requestedModel) && (requestedModel == "" || s.isModelSupportedByAccount(account, requestedModel)) {
					if err := s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), sessionHash, stickySessionTTL); err != nil {
						log.Printf("refresh session ttl failed: session=%s err=%v", sessionHash, err)
				REDACTED
					return account, nil
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// 2. 获取可调度账号列表（单平台）
	var accounts []Account
	var err error
	if s.cfg.RunMode == config.RunModeSimple {
		// 简易模式：忽略 groupID，查询所有可用账号
		accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, platform)
REDACTED else if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, platform)
REDACTED else {
		accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, platform)
REDACTED
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
REDACTED

	// 3. 按优先级+最久未用选择（考虑模型支持）
	var selected *Account
	for i := range accounts {
		acc := &accounts[i]
		if _, excluded := excludedIDs[acc.ID]; excluded {
			continue
	REDACTED
		if !acc.IsSchedulableForModel(requestedModel) {
			continue
	REDACTED
		if requestedModel != "" && !s.isModelSupportedByAccount(acc, requestedModel) {
			continue
	REDACTED
		if selected == nil {
			selected = acc
			continue
	REDACTED
		if acc.Priority < selected.Priority {
			selected = acc
	REDACTED else if acc.Priority == selected.Priority {
			switch {
			case acc.LastUsedAt == nil && selected.LastUsedAt != nil:
				selected = acc
			case acc.LastUsedAt != nil && selected.LastUsedAt == nil:
				// keep selected (never used is preferred)
			case acc.LastUsedAt == nil && selected.LastUsedAt == nil:
				if preferOAuth && acc.Type != selected.Type && acc.Type == AccountTypeOAuth {
					selected = acc
			REDACTED
			default:
				if acc.LastUsedAt.Before(*selected.LastUsedAt) {
					selected = acc
			REDACTED
		REDACTED
	REDACTED
REDACTED

	if selected == nil {
		if requestedModel != "" {
			return nil, fmt.Errorf("no available accounts supporting model: %s", requestedModel)
	REDACTED
		return nil, errors.New("no available accounts")
REDACTED

	// 4. 建立粘性绑定
	if sessionHash != "" && s.cache != nil {
		if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, selected.ID, stickySessionTTL); err != nil {
			log.Printf("set session account failed: session=%s account_id=%d err=%v", sessionHash, selected.ID, err)
	REDACTED
REDACTED

	return selected, nil
REDACTED

// selectAccountWithMixedScheduling 选择账户（支持混合调度）
// 查询原生平台账户 + 启用 mixed_scheduling 的 antigravity 账户
func (s *GatewayService) selectAccountWithMixedScheduling(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{REDACTED, nativePlatform string) (*Account, error) {
	platforms := []string{nativePlatform, PlatformAntigravityREDACTED
	preferOAuth := nativePlatform == PlatformGemini

	// 1. 查询粘性会话
	if sessionHash != "" && s.cache != nil {
		accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
		if err == nil && accountID > 0 {
			if _, excluded := excludedIDs[accountID]; !excluded {
				account, err := s.accountRepo.GetByID(ctx, accountID)
				// 检查账号分组归属和有效性：原生平台直接匹配，antigravity 需要启用混合调度
				if err == nil && s.isAccountInGroup(account, groupID) && account.IsSchedulableForModel(requestedModel) && (requestedModel == "" || s.isModelSupportedByAccount(account, requestedModel)) {
					if account.Platform == nativePlatform || (account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled()) {
						if err := s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), sessionHash, stickySessionTTL); err != nil {
							log.Printf("refresh session ttl failed: session=%s err=%v", sessionHash, err)
					REDACTED
						return account, nil
				REDACTED
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// 2. 获取可调度账号列表
	var accounts []Account
	var err error
	if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatforms(ctx, *groupID, platforms)
REDACTED else {
		accounts, err = s.accountRepo.ListSchedulableByPlatforms(ctx, platforms)
REDACTED
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
REDACTED

	// 3. 按优先级+最久未用选择（考虑模型支持和混合调度）
	var selected *Account
	for i := range accounts {
		acc := &accounts[i]
		if _, excluded := excludedIDs[acc.ID]; excluded {
			continue
	REDACTED
		// 过滤：原生平台直接通过，antigravity 需要启用混合调度
		if acc.Platform == PlatformAntigravity && !acc.IsMixedSchedulingEnabled() {
			continue
	REDACTED
		if !acc.IsSchedulableForModel(requestedModel) {
			continue
	REDACTED
		if requestedModel != "" && !s.isModelSupportedByAccount(acc, requestedModel) {
			continue
	REDACTED
		if selected == nil {
			selected = acc
			continue
	REDACTED
		if acc.Priority < selected.Priority {
			selected = acc
	REDACTED else if acc.Priority == selected.Priority {
			switch {
			case acc.LastUsedAt == nil && selected.LastUsedAt != nil:
				selected = acc
			case acc.LastUsedAt != nil && selected.LastUsedAt == nil:
				// keep selected (never used is preferred)
			case acc.LastUsedAt == nil && selected.LastUsedAt == nil:
				if preferOAuth && acc.Platform == PlatformGemini && selected.Platform == PlatformGemini && acc.Type != selected.Type && acc.Type == AccountTypeOAuth {
					selected = acc
			REDACTED
			default:
				if acc.LastUsedAt.Before(*selected.LastUsedAt) {
					selected = acc
			REDACTED
		REDACTED
	REDACTED
REDACTED

	if selected == nil {
		if requestedModel != "" {
			return nil, fmt.Errorf("no available accounts supporting model: %s", requestedModel)
	REDACTED
		return nil, errors.New("no available accounts")
REDACTED

	// 4. 建立粘性绑定
	if sessionHash != "" && s.cache != nil {
		if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, selected.ID, stickySessionTTL); err != nil {
			log.Printf("set session account failed: session=%s account_id=%d err=%v", sessionHash, selected.ID, err)
	REDACTED
REDACTED

	return selected, nil
REDACTED

// isModelSupportedByAccount 根据账户平台检查模型支持
func (s *GatewayService) isModelSupportedByAccount(account *Account, requestedModel string) bool {
	if account.Platform == PlatformAntigravity {
		// Antigravity 平台使用专门的模型支持检查
		return IsAntigravityModelSupported(requestedModel)
REDACTED
	// 其他平台使用账户的模型支持检查
	return account.IsModelSupported(requestedModel)
REDACTED

// IsAntigravityModelSupported 检查 Antigravity 平台是否支持指定模型
// 所有 claude- 和 gemini- 前缀的模型都能通过映射或透传支持
func IsAntigravityModelSupported(requestedModel string) bool {
	return strings.HasPrefix(requestedModel, "claude-") ||
		strings.HasPrefix(requestedModel, "gemini-")
REDACTED

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
	REDACTED
		return apiKey, "apikey", nil
	default:
		return "", "", fmt.Errorf("unsupported account type: %s", account.Type)
REDACTED
REDACTED

func (s *GatewayService) getOAuthToken(ctx context.Context, account *Account) (string, string, error) {
	accessToken := account.GetCredential("access_token")
	if accessToken == "" {
		return "", "", errors.New("access_token not found in credentials")
REDACTED
	// Token刷新由后台 TokenRefreshService 处理，此处只返回当前token
	return accessToken, "oauth", nil
REDACTED

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
REDACTED

	// API Key 账号：未配置的错误码重试
	return !account.ShouldHandleErrorCode(statusCode)
REDACTED

// shouldFailoverUpstreamError determines whether an upstream error should trigger account failover.
func (s *GatewayService) shouldFailoverUpstreamError(statusCode int) bool {
	switch statusCode {
	case 401, 403, 429, 529:
		return true
	default:
		return statusCode >= 500
REDACTED
REDACTED

func retryBackoffDelay(attempt int) time.Duration {
	// attempt 从 1 开始，表示第 attempt 次请求刚失败，需要等待后进行第 attempt+1 次请求。
	if attempt <= 0 {
		return retryBaseDelay
REDACTED
	delay := retryBaseDelay * time.Duration(1<<(attempt-1))
	if delay > retryMaxDelay {
		return retryMaxDelay
REDACTED
	return delay
REDACTED

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
REDACTED
	timer := time.NewTimer(d)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
		REDACTED
	REDACTED
REDACTED()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
REDACTED
REDACTED

// isClaudeCodeClient 判断请求是否来自 Claude Code 客户端
// 简化判断：User-Agent 匹配 + metadata.user_id 存在
func isClaudeCodeClient(userAgent string, metadataUserID string) bool {
	if metadataUserID == "" {
		return false
REDACTED
	return claudeCliUserAgentRe.MatchString(userAgent)
REDACTED

// systemIncludesClaudeCodePrompt 检查 system 中是否已包含 Claude Code 提示词
// 使用前缀匹配支持多种变体（标准版、Agent SDK 版等）
func systemIncludesClaudeCodePrompt(system any) bool {
	switch v := system.(type) {
	case string:
		return hasClaudeCodePrefix(v)
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if text, ok := m["text"].(string); ok && hasClaudeCodePrefix(text) {
					return true
			REDACTED
		REDACTED
	REDACTED
REDACTED
	return false
REDACTED

// hasClaudeCodePrefix 检查文本是否以 Claude Code 提示词的特征前缀开头
func hasClaudeCodePrefix(text string) bool {
	for _, prefix := range claudeCodePromptPrefixes {
		if strings.HasPrefix(text, prefix) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// injectClaudeCodePrompt 在 system 开头注入 Claude Code 提示词
// 处理 null、字符串、数组三种格式
func injectClaudeCodePrompt(body []byte, system any) []byte {
	claudeCodeBlock := map[string]any{
		"type":          "text",
		"text":          claudeCodeSystemPrompt,
		"cache_control": map[string]string{"type": "ephemeral"REDACTED,
REDACTED

	var newSystem []any

	switch v := system.(type) {
	case nil:
		newSystem = []any{claudeCodeBlockREDACTED
	case string:
		if v == "" || v == claudeCodeSystemPrompt {
			newSystem = []any{claudeCodeBlockREDACTED
	REDACTED else {
			newSystem = []any{claudeCodeBlock, map[string]any{"type": "text", "text": vREDACTEDREDACTED
	REDACTED
	case []any:
		newSystem = make([]any, 0, len(v)+1)
		newSystem = append(newSystem, claudeCodeBlock)
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if text, ok := m["text"].(string); ok && text == claudeCodeSystemPrompt {
					continue
			REDACTED
		REDACTED
			newSystem = append(newSystem, item)
	REDACTED
	default:
		newSystem = []any{claudeCodeBlockREDACTED
REDACTED

	result, err := sjson.SetBytes(body, "system", newSystem)
	if err != nil {
		log.Printf("Warning: failed to inject Claude Code prompt: %v", err)
		return body
REDACTED
	return result
REDACTED

// enforceCacheControlLimit 强制执行 cache_control 块数量限制（最多 4 个）
// 超限时优先从 messages 中移除 cache_control，保护 system 中的缓存控制
func enforceCacheControlLimit(body []byte) []byte {
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return body
REDACTED

	// 计算当前 cache_control 块数量
	count := countCacheControlBlocks(data)
	if count <= maxCacheControlBlocks {
		return body
REDACTED

	// 超限：优先从 messages 中移除，再从 system 中移除
	for count > maxCacheControlBlocks {
		if removeCacheControlFromMessages(data) {
			count--
			continue
	REDACTED
		if removeCacheControlFromSystem(data) {
			count--
			continue
	REDACTED
		break
REDACTED

	result, err := json.Marshal(data)
	if err != nil {
		return body
REDACTED
	return result
REDACTED

// countCacheControlBlocks 统计 system 和 messages 中的 cache_control 块数量
func countCacheControlBlocks(data map[string]any) int {
	count := 0

	// 统计 system 中的块
	if system, ok := data["system"].([]any); ok {
		for _, item := range system {
			if m, ok := item.(map[string]any); ok {
				if _, has := m["cache_control"]; has {
					count++
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// 统计 messages 中的块
	if messages, ok := data["messages"].([]any); ok {
		for _, msg := range messages {
			if msgMap, ok := msg.(map[string]any); ok {
				if content, ok := msgMap["content"].([]any); ok {
					for _, item := range content {
						if m, ok := item.(map[string]any); ok {
							if _, has := m["cache_control"]; has {
								count++
						REDACTED
					REDACTED
				REDACTED
			REDACTED
		REDACTED
	REDACTED
REDACTED

	return count
REDACTED

// removeCacheControlFromMessages 从 messages 中移除一个 cache_control（从头开始）
// 返回 true 表示成功移除，false 表示没有可移除的
func removeCacheControlFromMessages(data map[string]any) bool {
	messages, ok := data["messages"].([]any)
	if !ok {
		return false
REDACTED

	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
	REDACTED
		content, ok := msgMap["content"].([]any)
		if !ok {
			continue
	REDACTED
		for _, item := range content {
			if m, ok := item.(map[string]any); ok {
				if _, has := m["cache_control"]; has {
					delete(m, "cache_control")
					return true
			REDACTED
		REDACTED
	REDACTED
REDACTED
	return false
REDACTED

// removeCacheControlFromSystem 从 system 中移除一个 cache_control（从尾部开始，保护注入的 prompt）
// 返回 true 表示成功移除，false 表示没有可移除的
func removeCacheControlFromSystem(data map[string]any) bool {
	system, ok := data["system"].([]any)
	if !ok {
		return false
REDACTED

	// 从尾部开始移除，保护开头注入的 Claude Code prompt
	for i := len(system) - 1; i >= 0; i-- {
		if m, ok := system[i].(map[string]any); ok {
			if _, has := m["cache_control"]; has {
				delete(m, "cache_control")
				return true
		REDACTED
	REDACTED
REDACTED
	return false
REDACTED

// Forward 转发请求到Claude API
func (s *GatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, parsed *ParsedRequest) (*ForwardResult, error) {
	startTime := time.Now()
	if parsed == nil {
		return nil, fmt.Errorf("parse request: empty request")
REDACTED

	body := parsed.Body
	reqModel := parsed.Model
	reqStream := parsed.Stream

	// 智能注入 Claude Code 系统提示词（仅 OAuth/SetupToken 账号需要）
	// 条件：1) OAuth/SetupToken 账号  2) 不是 Claude Code 客户端  3) 不是 Haiku 模型  4) system 中还没有 Claude Code 提示词
	if account.IsOAuth() &&
		!isClaudeCodeClient(c.GetHeader("User-Agent"), parsed.MetadataUserID) &&
		!strings.Contains(strings.ToLower(reqModel), "haiku") &&
		!systemIncludesClaudeCodePrompt(parsed.System) {
		body = injectClaudeCodePrompt(body, parsed.System)
REDACTED

	// 强制执行 cache_control 块数量限制（最多 4 个）
	body = enforceCacheControlLimit(body)

	// 应用模型映射（仅对apikey类型账号）
	originalModel := reqModel
	if account.Type == AccountTypeAPIKey {
		mappedModel := account.GetMappedModel(reqModel)
		if mappedModel != reqModel {
			// 替换请求体中的模型名
			body = s.replaceModelInBody(body, mappedModel)
			reqModel = mappedModel
			log.Printf("Model mapping applied: %s -> %s (account: %s)", originalModel, mappedModel, account.Name)
	REDACTED
REDACTED

	// 获取凭证
	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED

	// 获取代理URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	// 重试循环
	var resp *http.Response
	retryStart := time.Now()
	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		// 构建上游请求（每次重试需要重新构建，因为请求体需要重新读取）
		upstreamReq, err := s.buildUpstreamRequest(ctx, c, account, body, token, tokenType, reqModel)
		if err != nil {
			return nil, err
	REDACTED

		// 发送请求
		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
		REDACTED
			return nil, fmt.Errorf("upstream request failed: %w", err)
	REDACTED

		// 优先检测thinking block签名错误（400）并重试一次
		if resp.StatusCode == 400 {
			respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			if readErr == nil {
				_ = resp.Body.Close()

				if s.isThinkingBlockSignatureError(respBody) {
					looksLikeToolSignatureError := func(msg string) bool {
						m := strings.ToLower(msg)
						return strings.Contains(m, "tool_use") ||
							strings.Contains(m, "tool_result") ||
							strings.Contains(m, "functioncall") ||
							strings.Contains(m, "function_call") ||
							strings.Contains(m, "functionresponse") ||
							strings.Contains(m, "function_response")
				REDACTED

					// 避免在重试预算已耗尽时再发起额外请求
					if time.Since(retryStart) >= maxRetryElapsed {
						resp.Body = io.NopCloser(bytes.NewReader(respBody))
						break
				REDACTED
					log.Printf("Account %d: detected thinking block signature error, retrying with filtered thinking blocks", account.ID)

					// Conservative two-stage fallback:
					// 1) Disable thinking + thinking->text (preserve content)
					// 2) Only if upstream still errors AND error message points to tool/function signature issues:
					//    also downgrade tool_use/tool_result blocks to text.

					filteredBody := FilterThinkingBlocksForRetry(body)
					retryReq, buildErr := s.buildUpstreamRequest(ctx, c, account, filteredBody, token, tokenType, reqModel)
					if buildErr == nil {
						retryResp, retryErr := s.httpUpstream.Do(retryReq, proxyURL, account.ID, account.Concurrency)
						if retryErr == nil {
							if retryResp.StatusCode < 400 {
								log.Printf("Account %d: signature error retry succeeded (thinking downgraded)", account.ID)
								resp = retryResp
								break
						REDACTED

							retryRespBody, retryReadErr := io.ReadAll(io.LimitReader(retryResp.Body, 2<<20))
							_ = retryResp.Body.Close()
							if retryReadErr == nil && retryResp.StatusCode == 400 && s.isThinkingBlockSignatureError(retryRespBody) {
								msg2 := extractUpstreamErrorMessage(retryRespBody)
								if looksLikeToolSignatureError(msg2) && time.Since(retryStart) < maxRetryElapsed {
									log.Printf("Account %d: signature retry still failing and looks tool-related, retrying with tool blocks downgraded", account.ID)
									filteredBody2 := FilterSignatureSensitiveBlocksForRetry(body)
									retryReq2, buildErr2 := s.buildUpstreamRequest(ctx, c, account, filteredBody2, token, tokenType, reqModel)
									if buildErr2 == nil {
										retryResp2, retryErr2 := s.httpUpstream.Do(retryReq2, proxyURL, account.ID, account.Concurrency)
										if retryErr2 == nil {
											resp = retryResp2
											break
									REDACTED
										if retryResp2 != nil && retryResp2.Body != nil {
											_ = retryResp2.Body.Close()
									REDACTED
										log.Printf("Account %d: tool-downgrade signature retry failed: %v", account.ID, retryErr2)
								REDACTED else {
										log.Printf("Account %d: tool-downgrade signature retry build failed: %v", account.ID, buildErr2)
								REDACTED
							REDACTED
						REDACTED

							// Fall back to the original retry response context.
							resp = &http.Response{
								StatusCode: retryResp.StatusCode,
								Header:     retryResp.Header.Clone(),
								Body:       io.NopCloser(bytes.NewReader(retryRespBody)),
						REDACTED
							break
					REDACTED
						if retryResp != nil && retryResp.Body != nil {
							_ = retryResp.Body.Close()
					REDACTED
						log.Printf("Account %d: signature error retry failed: %v", account.ID, retryErr)
				REDACTED else {
						log.Printf("Account %d: signature error retry build request failed: %v", account.ID, buildErr)
				REDACTED

					// Retry failed: restore original response body and continue handling.
					resp.Body = io.NopCloser(bytes.NewReader(respBody))
					break
			REDACTED
				// 不是thinking签名错误，恢复响应体
				resp.Body = io.NopCloser(bytes.NewReader(respBody))
		REDACTED
	REDACTED

		// 检查是否需要通用重试（排除400，因为400已经在上面特殊处理过了）
		if resp.StatusCode >= 400 && resp.StatusCode != 400 && s.shouldRetryUpstreamError(account, resp.StatusCode) {
			if attempt < maxRetryAttempts {
				elapsed := time.Since(retryStart)
				if elapsed >= maxRetryElapsed {
					break
			REDACTED

				delay := retryBackoffDelay(attempt)
				remaining := maxRetryElapsed - elapsed
				if delay > remaining {
					delay = remaining
			REDACTED
				if delay <= 0 {
					break
			REDACTED

				log.Printf("Account %d: upstream error %d, retry %d/%d after %v (elapsed=%v/%v)",
					account.ID, resp.StatusCode, attempt, maxRetryAttempts, delay, elapsed, maxRetryElapsed)
				_ = resp.Body.Close()
				if err := sleepWithContext(ctx, delay); err != nil {
					return nil, err
			REDACTED
				continue
		REDACTED
			// 最后一次尝试也失败，跳出循环处理重试耗尽
			break
	REDACTED

		// 不需要重试（成功或不可重试的错误），跳出循环
		// DEBUG: 输出响应 headers（用于检测 rate limit 信息）
		if account.Platform == PlatformGemini && resp.StatusCode < 400 {
			log.Printf("[DEBUG] Gemini API Response Headers for account %d:", account.ID)
			for k, v := range resp.Header {
				log.Printf("[DEBUG]   %s: %v", k, v)
		REDACTED
	REDACTED
		break
REDACTED
	if resp == nil || resp.Body == nil {
		return nil, errors.New("upstream request failed: empty response")
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	// 处理重试耗尽的情况
	if resp.StatusCode >= 400 && s.shouldRetryUpstreamError(account, resp.StatusCode) {
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			s.handleRetryExhaustedSideEffects(ctx, resp, account)
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCodeREDACTED
	REDACTED
		return s.handleRetryExhaustedError(ctx, resp, c, account)
REDACTED

	// 处理可切换账号的错误
	if resp.StatusCode >= 400 && s.shouldFailoverUpstreamError(resp.StatusCode) {
		s.handleFailoverSideEffects(ctx, resp, account)
		return nil, &UpstreamFailoverError{StatusCode: resp.StatusCodeREDACTED
REDACTED

	// 处理错误响应（不可重试的错误）
	if resp.StatusCode >= 400 {
		// 可选：对部分 400 触发 failover（默认关闭以保持语义）
		if resp.StatusCode == 400 && s.cfg != nil && s.cfg.Gateway.FailoverOn400 {
			respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			if readErr != nil {
				// ReadAll failed, fall back to normal error handling without consuming the stream
				return s.handleErrorResponse(ctx, resp, c, account)
		REDACTED
			_ = resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(respBody))

			if s.shouldFailoverOn400(respBody) {
				if s.cfg.Gateway.LogUpstreamErrorBody {
					log.Printf(
						"Account %d: 400 error, attempting failover: %s",
						account.ID,
						truncateForLog(respBody, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes),
					)
			REDACTED else {
					log.Printf("Account %d: 400 error, attempting failover", account.ID)
			REDACTED
				s.handleFailoverSideEffects(ctx, resp, account)
				return nil, &UpstreamFailoverError{StatusCode: resp.StatusCodeREDACTED
		REDACTED
	REDACTED
		return s.handleErrorResponse(ctx, resp, c, account)
REDACTED

	// 处理正常响应
	var usage *ClaudeUsage
	var firstTokenMs *int
	var clientDisconnect bool
	if reqStream {
		streamResult, err := s.handleStreamingResponse(ctx, resp, c, account, startTime, originalModel, reqModel)
		if err != nil {
			if err.Error() == "have error in stream" {
				return nil, &UpstreamFailoverError{
					StatusCode: 403,
			REDACTED
		REDACTED
			return nil, err
	REDACTED
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
		clientDisconnect = streamResult.clientDisconnect
REDACTED else {
		usage, err = s.handleNonStreamingResponse(ctx, resp, c, account, originalModel, reqModel)
		if err != nil {
			return nil, err
	REDACTED
REDACTED

	return &ForwardResult{
		RequestID:        resp.Header.Get("x-request-id"),
		Usage:            *usage,
		Model:            originalModel, // 使用原始模型用于计费和日志
		Stream:           reqStream,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnect,
REDACTED, nil
REDACTED

func (s *GatewayService) buildUpstreamRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token, tokenType, modelID string) (*http.Request, error) {
	// 确定目标URL
	targetURL := claudeAPIURL
	if account.Type == AccountTypeAPIKey {
		baseURL := account.GetBaseURL()
		if baseURL != "" {
			validatedURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, err
		REDACTED
			targetURL = validatedURL + "/v1/messages"
	REDACTED
REDACTED

	// OAuth账号：应用统一指纹
	var fingerprint *Fingerprint
	if account.IsOAuth() && s.identityService != nil {
		// 1. 获取或创建指纹（包含随机生成的ClientID）
		fp, err := s.identityService.GetOrCreateFingerprint(ctx, account.ID, c.Request.Header)
		if err != nil {
			log.Printf("Warning: failed to get fingerprint for account %d: %v", account.ID, err)
			// 失败时降级为透传原始headers
	REDACTED else {
			fingerprint = fp

			// 2. 重写metadata.user_id（需要指纹中的ClientID和账号的account_uuid）
			accountUUID := account.GetExtraString("account_uuid")
			if accountUUID != "" && fp.ClientID != "" {
				if newBody, err := s.identityService.RewriteUserID(body, account.ID, accountUUID, fp.ClientID); err == nil && len(newBody) > 0 {
					body = newBody
			REDACTED
		REDACTED
	REDACTED
REDACTED

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
REDACTED

	// 设置认证头
	if tokenType == "oauth" {
		req.Header.Set("authorization", "Bearer "+token)
REDACTED else {
		req.Header.Set("x-api-key", token)
REDACTED

	// 白名单透传headers
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if allowedHeaders[lowerKey] {
			for _, v := range values {
				req.Header.Add(key, v)
		REDACTED
	REDACTED
REDACTED

	// OAuth账号：应用缓存的指纹到请求头（覆盖白名单透传的头）
	if fingerprint != nil {
		s.identityService.ApplyFingerprint(req, fingerprint)
REDACTED

	// 确保必要的headers存在
	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
REDACTED
	if req.Header.Get("anthropic-version") == "" {
		req.Header.Set("anthropic-version", "2023-06-01")
REDACTED

	// 处理anthropic-beta header（OAuth账号需要特殊处理）
	if tokenType == "oauth" {
		req.Header.Set("anthropic-beta", s.getBetaHeader(modelID, c.GetHeader("anthropic-beta")))
REDACTED else if s.cfg != nil && s.cfg.Gateway.InjectBetaForAPIKey && req.Header.Get("anthropic-beta") == "" {
		// API-key：仅在请求显式使用 beta 特性且客户端未提供时，按需补齐（默认关闭）
		if requestNeedsBetaFeatures(body) {
			if beta := defaultAPIKeyBetaHeader(body); beta != "" {
				req.Header.Set("anthropic-beta", beta)
		REDACTED
	REDACTED
REDACTED

	return req, nil
REDACTED

// getBetaHeader 处理anthropic-beta header
// 对于OAuth账号，需要确保包含oauth-2025-04-20
func (s *GatewayService) getBetaHeader(modelID string, clientBetaHeader string) string {
	// 如果客户端传了anthropic-beta
	if clientBetaHeader != "" {
		// 已包含oauth beta则直接返回
		if strings.Contains(clientBetaHeader, claude.BetaOAuth) {
			return clientBetaHeader
	REDACTED

		// 需要添加oauth beta
		parts := strings.Split(clientBetaHeader, ",")
		for i, p := range parts {
			parts[i] = strings.TrimSpace(p)
	REDACTED

		// 在claude-code-20250219后面插入oauth beta
		claudeCodeIdx := -1
		for i, p := range parts {
			if p == claude.BetaClaudeCode {
				claudeCodeIdx = i
				break
		REDACTED
	REDACTED

		if claudeCodeIdx >= 0 {
			// 在claude-code后面插入
			newParts := make([]string, 0, len(parts)+1)
			newParts = append(newParts, parts[:claudeCodeIdx+1]...)
			newParts = append(newParts, claude.BetaOAuth)
			newParts = append(newParts, parts[claudeCodeIdx+1:]...)
			return strings.Join(newParts, ",")
	REDACTED

		// 没有claude-code，放在第一位
		return claude.BetaOAuth + "," + clientBetaHeader
REDACTED

	// 客户端没传，根据模型生成
	// haiku 模型不需要 claude-code beta
	if strings.Contains(strings.ToLower(modelID), "haiku") {
		return claude.HaikuBetaHeader
REDACTED

	return claude.DefaultBetaHeader
REDACTED

func requestNeedsBetaFeatures(body []byte) bool {
	tools := gjson.GetBytes(body, "tools")
	if tools.Exists() && tools.IsArray() && len(tools.Array()) > 0 {
		return true
REDACTED
	if strings.EqualFold(gjson.GetBytes(body, "thinking.type").String(), "enabled") {
		return true
REDACTED
	return false
REDACTED

func defaultAPIKeyBetaHeader(body []byte) string {
	modelID := gjson.GetBytes(body, "model").String()
	if strings.Contains(strings.ToLower(modelID), "haiku") {
		return claude.APIKeyHaikuBetaHeader
REDACTED
	return claude.APIKeyBetaHeader
REDACTED

func truncateForLog(b []byte, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = 2048
REDACTED
	if len(b) > maxBytes {
		b = b[:maxBytes]
REDACTED
	s := string(b)
	// 保持一行，避免污染日志格式
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
REDACTED

// isThinkingBlockSignatureError 检测是否是thinking block相关错误
// 这类错误可以通过过滤thinking blocks并重试来解决
func (s *GatewayService) isThinkingBlockSignatureError(respBody []byte) bool {
	msg := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
	if msg == "" {
		return false
REDACTED

	// Log for debugging
	log.Printf("[SignatureCheck] Checking error message: %s", msg)

	// 检测signature相关的错误（更宽松的匹配）
	// 例如: "Invalid `signature` in `thinking` block", "***.signature" 等
	if strings.Contains(msg, "signature") {
		log.Printf("[SignatureCheck] Detected signature error")
		return true
REDACTED

	// 检测 thinking block 顺序/类型错误
	// 例如: "Expected `thinking` or `redacted_thinking`, but found `text`"
	if strings.Contains(msg, "expected") && (strings.Contains(msg, "thinking") || strings.Contains(msg, "redacted_thinking")) {
		log.Printf("[SignatureCheck] Detected thinking block type error")
		return true
REDACTED

	// 检测空消息内容错误（可能是过滤 thinking blocks 后导致的）
	// 例如: "all messages must have non-empty content"
	if strings.Contains(msg, "non-empty content") || strings.Contains(msg, "empty content") {
		log.Printf("[SignatureCheck] Detected empty content error")
		return true
REDACTED

	return false
REDACTED

func (s *GatewayService) shouldFailoverOn400(respBody []byte) bool {
	// 只对“可能是兼容性差异导致”的 400 允许切换，避免无意义重试。
	// 默认保守：无法识别则不切换。
	msg := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
	if msg == "" {
		return false
REDACTED

	// 缺少/错误的 beta header：换账号/链路可能成功（尤其是混合调度时）。
	// 更精确匹配 beta 相关的兼容性问题，避免误触发切换。
	if strings.Contains(msg, "anthropic-beta") ||
		strings.Contains(msg, "beta feature") ||
		strings.Contains(msg, "requires beta") {
		return true
REDACTED

	// thinking/tool streaming 等兼容性约束（常见于中间转换链路）
	if strings.Contains(msg, "thinking") || strings.Contains(msg, "thought_signature") || strings.Contains(msg, "signature") {
		return true
REDACTED
	if strings.Contains(msg, "tool_use") || strings.Contains(msg, "tool_result") || strings.Contains(msg, "tools") {
		return true
REDACTED

	return false
REDACTED

func extractUpstreamErrorMessage(body []byte) string {
	// Claude 风格：{"type":"error","error":{"type":"...","message":"..."REDACTEDREDACTED
	if m := gjson.GetBytes(body, "error.message").String(); strings.TrimSpace(m) != "" {
		inner := strings.TrimSpace(m)
		// 有些上游会把完整 JSON 作为字符串塞进 message
		if strings.HasPrefix(inner, "{") {
			if innerMsg := gjson.Get(inner, "error.message").String(); strings.TrimSpace(innerMsg) != "" {
				return innerMsg
		REDACTED
	REDACTED
		return m
REDACTED

	// 兜底：尝试顶层 message
	return gjson.GetBytes(body, "message").String()
REDACTED

func (s *GatewayService) handleErrorResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account) (*ForwardResult, error) {
	body, _ := io.ReadAll(resp.Body)

	// 处理上游错误，标记账号状态
	shouldDisable := false
	if s.rateLimitService != nil {
		shouldDisable = s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
REDACTED
	if shouldDisable {
		return nil, &UpstreamFailoverError{StatusCode: resp.StatusCodeREDACTED
REDACTED

	// 根据状态码返回适当的自定义错误响应（不透传上游详细信息）
	var errType, errMsg string
	var statusCode int

	switch resp.StatusCode {
	case 400:
		// 仅记录上游错误摘要（避免输出请求内容）；需要时可通过配置打开
		if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
			log.Printf(
				"Upstream 400 error (account=%d platform=%s type=%s): %s",
				account.ID,
				account.Platform,
				account.Type,
				truncateForLog(body, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes),
			)
	REDACTED
		c.Data(http.StatusBadRequest, "application/json", body)
		return nil, fmt.Errorf("upstream error: %d", resp.StatusCode)
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
REDACTED

	// 返回自定义错误响应
	c.JSON(statusCode, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": errMsg,
	REDACTED,
REDACTED)

	return nil, fmt.Errorf("upstream error: %d", resp.StatusCode)
REDACTED

func (s *GatewayService) handleRetryExhaustedSideEffects(ctx context.Context, resp *http.Response, account *Account) {
	body, _ := io.ReadAll(resp.Body)
	statusCode := resp.StatusCode

	// OAuth/Setup Token 账号的 403：标记账号异常
	if account.IsOAuth() && statusCode == 403 {
		s.rateLimitService.HandleUpstreamError(ctx, account, statusCode, resp.Header, body)
		log.Printf("Account %d: marked as error after %d retries for status %d", account.ID, maxRetryAttempts, statusCode)
REDACTED else {
		// API Key 未配置错误码：不标记账号状态
		log.Printf("Account %d: upstream error %d after %d retries (not marking account)", account.ID, statusCode, maxRetryAttempts)
REDACTED
REDACTED

func (s *GatewayService) handleFailoverSideEffects(ctx context.Context, resp *http.Response, account *Account) {
	body, _ := io.ReadAll(resp.Body)
	s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
REDACTED

// handleRetryExhaustedError 处理重试耗尽后的错误
// OAuth 403：标记账号异常
// API Key 未配置错误码：仅返回错误，不标记账号
func (s *GatewayService) handleRetryExhaustedError(ctx context.Context, resp *http.Response, c *gin.Context, account *Account) (*ForwardResult, error) {
	s.handleRetryExhaustedSideEffects(ctx, resp, account)

	// 返回统一的重试耗尽错误响应
	c.JSON(http.StatusBadGateway, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    "upstream_error",
			"message": "Upstream request failed after retries",
	REDACTED,
REDACTED)

	return nil, fmt.Errorf("upstream error: %d (retries exhausted)", resp.StatusCode)
REDACTED

// streamingResult 流式响应结果
type streamingResult struct {
	usage            *ClaudeUsage
	firstTokenMs     *int
	clientDisconnect bool // 客户端是否在流式传输过程中断开
REDACTED

func (s *GatewayService) handleStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, startTime time.Time, originalModel, mappedModel string) (*streamingResult, error) {
	// 更新5h窗口状态
	s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)

	if s.cfg != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.cfg.Security.ResponseHeaders)
REDACTED

	// 设置SSE响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// 透传其他响应头
	if v := resp.Header.Get("x-request-id"); v != "" {
		c.Header("x-request-id", v)
REDACTED

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
REDACTED

	usage := &ClaudeUsage{REDACTED
	var firstTokenMs *int
	scanner := bufio.NewScanner(resp.Body)
	// 设置更大的buffer以处理长行
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
REDACTED
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)

	type scanEvent struct {
		line string
		err  error
REDACTED
	// 独立 goroutine 读取上游，避免读取阻塞导致超时/keepalive无法处理
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
	// 仅监控上游数据间隔超时，避免下游写入阻塞导致误判
	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
REDACTED
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
REDACTED

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
	clientDisconnected := false // 客户端断开标志，断开后继续读取上游以获取完整usage

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				// 上游完成，返回结果
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: clientDisconnectedREDACTED, nil
		REDACTED
			if ev.err != nil {
				// 检测 context 取消（客户端断开会导致 context 取消，进而影响上游读取）
				if errors.Is(ev.err, context.Canceled) || errors.Is(ev.err, context.DeadlineExceeded) {
					log.Printf("Context canceled during streaming, returning collected usage")
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: trueREDACTED, nil
			REDACTED
				// 客户端已通过写入失败检测到断开，上游也出错了，返回已收集的 usage
				if clientDisconnected {
					log.Printf("Upstream read error after client disconnect: %v, returning collected usage", ev.err)
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: trueREDACTED, nil
			REDACTED
				// 客户端未断开，正常的错误处理
				if errors.Is(ev.err, bufio.ErrTooLong) {
					log.Printf("SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, ev.err)
					sendErrorEvent("response_too_large")
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, ev.err
			REDACTED
				sendErrorEvent("stream_read_error")
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, fmt.Errorf("stream read error: %w", ev.err)
		REDACTED
			line := ev.line
			if line == "event: error" {
				// 上游返回错误事件，如果客户端已断开仍返回已收集的 usage
				if clientDisconnected {
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: trueREDACTED, nil
			REDACTED
				return nil, errors.New("have error in stream")
		REDACTED

			// Extract data from SSE line (supports both "data: " and "data:" formats)
			var data string
			if sseDataRe.MatchString(line) {
				data = sseDataRe.ReplaceAllString(line, "")
				// 如果有模型映射，替换响应中的model字段
				if needModelReplace {
					line = s.replaceModelInSSELine(line, mappedModel, originalModel)
			REDACTED
		REDACTED

			// 写入客户端（统一处理 data 行和非 data 行）
			if !clientDisconnected {
				if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
					clientDisconnected = true
					log.Printf("Client disconnected during streaming, continuing to drain upstream for billing")
			REDACTED else {
					flusher.Flush()
			REDACTED
		REDACTED

			// 无论客户端是否断开，都解析 usage（仅对 data 行）
			if data != "" {
				if firstTokenMs == nil && data != "[DONE]" {
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
			REDACTED
				s.parseSSEUsage(data, usage)
		REDACTED

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
		REDACTED
			if clientDisconnected {
				// 客户端已断开，上游也超时了，返回已收集的 usage
				log.Printf("Upstream timeout after client disconnect, returning collected usage")
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: trueREDACTED, nil
		REDACTED
			log.Printf("Stream data interval timeout: account=%d model=%s interval=%s", account.ID, originalModel, streamInterval)
			sendErrorEvent("stream_timeout")
			return &streamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, fmt.Errorf("stream data interval timeout")
	REDACTED
REDACTED

REDACTED

// replaceModelInSSELine 替换SSE数据行中的model字段
func (s *GatewayService) replaceModelInSSELine(line, fromModel, toModel string) string {
	if !sseDataRe.MatchString(line) {
		return line
REDACTED
	data := sseDataRe.ReplaceAllString(line, "")
	if data == "" || data == "[DONE]" {
		return line
REDACTED

	var event map[string]any
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return line
REDACTED

	// 只替换 message_start 事件中的 message.model
	if event["type"] != "message_start" {
		return line
REDACTED

	msg, ok := event["message"].(map[string]any)
	if !ok {
		return line
REDACTED

	model, ok := msg["model"].(string)
	if !ok || model != fromModel {
		return line
REDACTED

	msg["model"] = toModel
	newData, err := json.Marshal(event)
	if err != nil {
		return line
REDACTED

	return "data: " + string(newData)
REDACTED

func (s *GatewayService) parseSSEUsage(data string, usage *ClaudeUsage) {
	// 解析message_start获取input tokens（标准Claude API格式）
	var msgStart struct {
		Type    string `json:"type"`
		Message struct {
			Usage ClaudeUsage `json:"usage"`
	REDACTED `json:"message"`
REDACTED
	if json.Unmarshal([]byte(data), &msgStart) == nil && msgStart.Type == "message_start" {
		usage.InputTokens = msgStart.Message.Usage.InputTokens
		usage.CacheCreationInputTokens = msgStart.Message.Usage.CacheCreationInputTokens
		usage.CacheReadInputTokens = msgStart.Message.Usage.CacheReadInputTokens
REDACTED

	// 解析message_delta获取tokens（兼容GLM等把所有usage放在delta中的API）
	var msgDelta struct {
		Type  string `json:"type"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	REDACTED `json:"usage"`
REDACTED
	if json.Unmarshal([]byte(data), &msgDelta) == nil && msgDelta.Type == "message_delta" {
		// output_tokens 总是从 message_delta 获取
		usage.OutputTokens = msgDelta.Usage.OutputTokens

		// 如果 message_start 中没有值，则从 message_delta 获取（兼容GLM等API）
		if usage.InputTokens == 0 {
			usage.InputTokens = msgDelta.Usage.InputTokens
	REDACTED
		if usage.CacheCreationInputTokens == 0 {
			usage.CacheCreationInputTokens = msgDelta.Usage.CacheCreationInputTokens
	REDACTED
		if usage.CacheReadInputTokens == 0 {
			usage.CacheReadInputTokens = msgDelta.Usage.CacheReadInputTokens
	REDACTED
REDACTED
REDACTED

func (s *GatewayService) handleNonStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, originalModel, mappedModel string) (*ClaudeUsage, error) {
	// 更新5h窗口状态
	s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
REDACTED

	// 解析usage
	var response struct {
		Usage ClaudeUsage `json:"usage"`
REDACTED
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
REDACTED

	// 如果有模型映射，替换响应中的model字段
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

	// 写入响应
	c.Data(resp.StatusCode, contentType, body)

	return &response.Usage, nil
REDACTED

// replaceModelInResponseBody 替换响应体中的model字段
func (s *GatewayService) replaceModelInResponseBody(body []byte, fromModel, toModel string) []byte {
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

// RecordUsageInput 记录使用量的输入参数
type RecordUsageInput struct {
	Result       *ForwardResult
	APIKey       *APIKey
	User         *User
	Account      *Account
	Subscription *UserSubscription // 可选：订阅信息
	UserAgent    string            // 请求的 User-Agent
	IPAddress    string            // 请求的客户端 IP 地址
REDACTED

// RecordUsage 记录使用量并扣费（或更新订阅用量）
func (s *GatewayService) RecordUsage(ctx context.Context, input *RecordUsageInput) error {
	result := input.Result
	apiKey := input.APIKey
	user := input.User
	account := input.Account
	subscription := input.Subscription

	// 获取费率倍数
	multiplier := s.cfg.Default.RateMultiplier
	if apiKey.GroupID != nil && apiKey.Group != nil {
		multiplier = apiKey.Group.RateMultiplier
REDACTED

	var cost *CostBreakdown

	// 根据请求类型选择计费方式
	if result.ImageCount > 0 {
		// 图片生成计费
		var groupConfig *ImagePriceConfig
		if apiKey.Group != nil {
			groupConfig = &ImagePriceConfig{
				Price1K: apiKey.Group.ImagePrice1K,
				Price2K: apiKey.Group.ImagePrice2K,
				Price4K: apiKey.Group.ImagePrice4K,
		REDACTED
	REDACTED
		cost = s.billingService.CalculateImageCost(result.Model, result.ImageSize, result.ImageCount, groupConfig, multiplier)
REDACTED else {
		// Token 计费
		tokens := UsageTokens{
			InputTokens:         result.Usage.InputTokens,
			OutputTokens:        result.Usage.OutputTokens,
			CacheCreationTokens: result.Usage.CacheCreationInputTokens,
			CacheReadTokens:     result.Usage.CacheReadInputTokens,
	REDACTED
		var err error
		cost, err = s.billingService.CalculateCost(result.Model, tokens, multiplier)
		if err != nil {
			log.Printf("Calculate cost failed: %v", err)
			cost = &CostBreakdown{ActualCost: 0REDACTED
	REDACTED
REDACTED

	// 判断计费方式：订阅模式 vs 余额模式
	isSubscriptionBilling := subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
	billingType := BillingTypeBalance
	if isSubscriptionBilling {
		billingType = BillingTypeSubscription
REDACTED

	// 创建使用日志
	durationMs := int(result.Duration.Milliseconds())
	var imageSize *string
	if result.ImageSize != "" {
		imageSize = &result.ImageSize
REDACTED
	usageLog := &UsageLog{
		UserID:              user.ID,
		APIKeyID:            apiKey.ID,
		AccountID:           account.ID,
		RequestID:           result.RequestID,
		Model:               result.Model,
		InputTokens:         result.Usage.InputTokens,
		OutputTokens:        result.Usage.OutputTokens,
		CacheCreationTokens: result.Usage.CacheCreationInputTokens,
		CacheReadTokens:     result.Usage.CacheReadInputTokens,
		InputCost:           cost.InputCost,
		OutputCost:          cost.OutputCost,
		CacheCreationCost:   cost.CacheCreationCost,
		CacheReadCost:       cost.CacheReadCost,
		TotalCost:           cost.TotalCost,
		ActualCost:          cost.ActualCost,
		RateMultiplier:      multiplier,
		BillingType:         billingType,
		Stream:              result.Stream,
		DurationMs:          &durationMs,
		FirstTokenMs:        result.FirstTokenMs,
		ImageCount:          result.ImageCount,
		ImageSize:           imageSize,
		CreatedAt:           time.Now(),
REDACTED

	// 添加 UserAgent
	if input.UserAgent != "" {
		usageLog.UserAgent = &input.UserAgent
REDACTED

	// 添加 IPAddress
	if input.IPAddress != "" {
		usageLog.IPAddress = &input.IPAddress
REDACTED

	// 添加分组和订阅关联
	if apiKey.GroupID != nil {
		usageLog.GroupID = apiKey.GroupID
REDACTED
	if subscription != nil {
		usageLog.SubscriptionID = &subscription.ID
REDACTED

	inserted, err := s.usageLogRepo.Create(ctx, usageLog)
	if err != nil {
		log.Printf("Create usage log failed: %v", err)
REDACTED

	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		log.Printf("[SIMPLE MODE] Usage recorded (not billed): user=%d, tokens=%d", usageLog.UserID, usageLog.TotalTokens())
		s.deferredService.ScheduleLastUsedUpdate(account.ID)
		return nil
REDACTED

	shouldBill := inserted || err != nil

	// 根据计费类型执行扣费
	if isSubscriptionBilling {
		// 订阅模式：更新订阅用量（使用 TotalCost 原始费用，不考虑倍率）
		if shouldBill && cost.TotalCost > 0 {
			if err := s.userSubRepo.IncrementUsage(ctx, subscription.ID, cost.TotalCost); err != nil {
				log.Printf("Increment subscription usage failed: %v", err)
		REDACTED
			// 异步更新订阅缓存
			s.billingCacheService.QueueUpdateSubscriptionUsage(user.ID, *apiKey.GroupID, cost.TotalCost)
	REDACTED
REDACTED else {
		// 余额模式：扣除用户余额（使用 ActualCost 考虑倍率后的费用）
		if shouldBill && cost.ActualCost > 0 {
			if err := s.userRepo.DeductBalance(ctx, user.ID, cost.ActualCost); err != nil {
				log.Printf("Deduct balance failed: %v", err)
		REDACTED
			// 异步更新余额缓存
			s.billingCacheService.QueueDeductBalance(user.ID, cost.ActualCost)
	REDACTED
REDACTED

	// Schedule batch update for account last_used_at
	s.deferredService.ScheduleLastUsedUpdate(account.ID)

	return nil
REDACTED

// ForwardCountTokens 转发 count_tokens 请求到上游 API
// 特点：不记录使用量、仅支持非流式响应
func (s *GatewayService) ForwardCountTokens(ctx context.Context, c *gin.Context, account *Account, parsed *ParsedRequest) error {
	if parsed == nil {
		s.countTokensError(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return fmt.Errorf("parse request: empty request")
REDACTED

	body := parsed.Body
	reqModel := parsed.Model

	// Antigravity 账户不支持 count_tokens 转发，直接返回空值
	if account.Platform == PlatformAntigravity {
		c.JSON(http.StatusOK, gin.H{"input_tokens": 0REDACTED)
		return nil
REDACTED

	// 应用模型映射（仅对 apikey 类型账号）
	if account.Type == AccountTypeAPIKey {
		if reqModel != "" {
			mappedModel := account.GetMappedModel(reqModel)
			if mappedModel != reqModel {
				body = s.replaceModelInBody(body, mappedModel)
				reqModel = mappedModel
				log.Printf("CountTokens model mapping applied: %s -> %s (account: %s)", parsed.Model, mappedModel, account.Name)
		REDACTED
	REDACTED
REDACTED

	// 获取凭证
	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to get access token")
		return err
REDACTED

	// 构建上游请求
	upstreamReq, err := s.buildCountTokensRequest(ctx, c, account, body, token, tokenType, reqModel)
	if err != nil {
		s.countTokensError(c, http.StatusInternalServerError, "api_error", "Failed to build request")
		return err
REDACTED

	// 获取代理URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	// 发送请求
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Request failed")
		return fmt.Errorf("upstream request failed: %w", err)
REDACTED

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to read response")
		return err
REDACTED

	// 检测 thinking block 签名错误（400）并重试一次（过滤 thinking blocks）
	if resp.StatusCode == 400 && s.isThinkingBlockSignatureError(respBody) {
		log.Printf("Account %d: detected thinking block signature error on count_tokens, retrying with filtered thinking blocks", account.ID)

		filteredBody := FilterThinkingBlocksForRetry(body)
		retryReq, buildErr := s.buildCountTokensRequest(ctx, c, account, filteredBody, token, tokenType, reqModel)
		if buildErr == nil {
			retryResp, retryErr := s.httpUpstream.Do(retryReq, proxyURL, account.ID, account.Concurrency)
			if retryErr == nil {
				resp = retryResp
				respBody, err = io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if err != nil {
					s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to read response")
					return err
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// 处理错误响应
	if resp.StatusCode >= 400 {
		// 标记账号状态（429/529等）
		s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)

		// 记录上游错误摘要便于排障（不回显请求内容）
		if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
			log.Printf(
				"count_tokens upstream error %d (account=%d platform=%s type=%s): %s",
				resp.StatusCode,
				account.ID,
				account.Platform,
				account.Type,
				truncateForLog(respBody, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes),
			)
	REDACTED

		// 返回简化的错误响应
		errMsg := "Upstream request failed"
		switch resp.StatusCode {
		case 429:
			errMsg = "Rate limit exceeded"
		case 529:
			errMsg = "Service overloaded"
	REDACTED
		s.countTokensError(c, resp.StatusCode, "upstream_error", errMsg)
		return fmt.Errorf("upstream error: %d", resp.StatusCode)
REDACTED

	// 透传成功响应
	c.Data(resp.StatusCode, "application/json", respBody)
	return nil
REDACTED

// buildCountTokensRequest 构建 count_tokens 上游请求
func (s *GatewayService) buildCountTokensRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token, tokenType, modelID string) (*http.Request, error) {
	// 确定目标 URL
	targetURL := claudeAPICountTokensURL
	if account.Type == AccountTypeAPIKey {
		baseURL := account.GetBaseURL()
		if baseURL != "" {
			validatedURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, err
		REDACTED
			targetURL = validatedURL + "/v1/messages/count_tokens"
	REDACTED
REDACTED

	// OAuth 账号：应用统一指纹和重写 userID
	if account.IsOAuth() && s.identityService != nil {
		fp, err := s.identityService.GetOrCreateFingerprint(ctx, account.ID, c.Request.Header)
		if err == nil {
			accountUUID := account.GetExtraString("account_uuid")
			if accountUUID != "" && fp.ClientID != "" {
				if newBody, err := s.identityService.RewriteUserID(body, account.ID, accountUUID, fp.ClientID); err == nil && len(newBody) > 0 {
					body = newBody
			REDACTED
		REDACTED
	REDACTED
REDACTED

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
REDACTED

	// 设置认证头
	if tokenType == "oauth" {
		req.Header.Set("authorization", "Bearer "+token)
REDACTED else {
		req.Header.Set("x-api-key", token)
REDACTED

	// 白名单透传 headers
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if allowedHeaders[lowerKey] {
			for _, v := range values {
				req.Header.Add(key, v)
		REDACTED
	REDACTED
REDACTED

	// OAuth 账号：应用指纹到请求头
	if account.IsOAuth() && s.identityService != nil {
		fp, _ := s.identityService.GetOrCreateFingerprint(ctx, account.ID, c.Request.Header)
		if fp != nil {
			s.identityService.ApplyFingerprint(req, fp)
	REDACTED
REDACTED

	// 确保必要的 headers 存在
	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
REDACTED
	if req.Header.Get("anthropic-version") == "" {
		req.Header.Set("anthropic-version", "2023-06-01")
REDACTED

	// OAuth 账号：处理 anthropic-beta header
	if tokenType == "oauth" {
		req.Header.Set("anthropic-beta", s.getBetaHeader(modelID, c.GetHeader("anthropic-beta")))
REDACTED else if s.cfg != nil && s.cfg.Gateway.InjectBetaForAPIKey && req.Header.Get("anthropic-beta") == "" {
		// API-key：与 messages 同步的按需 beta 注入（默认关闭）
		if requestNeedsBetaFeatures(body) {
			if beta := defaultAPIKeyBetaHeader(body); beta != "" {
				req.Header.Set("anthropic-beta", beta)
		REDACTED
	REDACTED
REDACTED

	return req, nil
REDACTED

// countTokensError 返回 count_tokens 错误响应
func (s *GatewayService) countTokensError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
	REDACTED,
REDACTED)
REDACTED

func (s *GatewayService) validateUpstreamBaseURL(raw string) (string, error) {
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

// GetAvailableModels returns the list of models available for a group
// It aggregates model_mapping keys from all schedulable accounts in the group
func (s *GatewayService) GetAvailableModels(ctx context.Context, groupID *int64, platform string) []string {
	var accounts []Account
	var err error

	if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupID(ctx, *groupID)
REDACTED else {
		accounts, err = s.accountRepo.ListSchedulable(ctx)
REDACTED

	if err != nil || len(accounts) == 0 {
		return nil
REDACTED

	// Filter by platform if specified
	if platform != "" {
		filtered := make([]Account, 0)
		for _, acc := range accounts {
			if acc.Platform == platform {
				filtered = append(filtered, acc)
		REDACTED
	REDACTED
		accounts = filtered
REDACTED

	// Collect unique models from all accounts
	modelSet := make(map[string]struct{REDACTED)
	hasAnyMapping := false

	for _, acc := range accounts {
		mapping := acc.GetModelMapping()
		if len(mapping) > 0 {
			hasAnyMapping = true
			for model := range mapping {
				modelSet[model] = struct{REDACTED{REDACTED
		REDACTED
	REDACTED
REDACTED

	// If no account has model_mapping, return nil (use default)
	if !hasAnyMapping {
		return nil
REDACTED

	// Convert to slice
	models := make([]string, 0, len(modelSet))
	for model := range modelSet {
		models = append(models, model)
REDACTED

	return models
REDACTED

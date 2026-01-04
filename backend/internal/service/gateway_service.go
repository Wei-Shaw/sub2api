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
	defaultMaxLineSize      = 10 * 1024 * 1024
)

// sseDataRe matches SSE data lines with optional whitespace after colon.
// Some upstream APIs return non-standard "data:" without space (should be "data: ").
var (
	sseDataRe      = regexp.MustCompile(`^data:\s*`)
	sessionIDRegex = regexp.MustCompile(`session_([a-f0-9-]{36REDACTED)`)
)

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
	GetSessionAccountID(ctx context.Context, sessionHash string) (int64, error)
	SetSessionAccountID(ctx context.Context, sessionHash string, accountID int64, ttl time.Duration) error
	RefreshSessionTTL(ctx context.Context, sessionHash string, ttl time.Duration) error
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
	RequestID    string
	Usage        ClaudeUsage
	Model        string
	Stream       bool
	Duration     time.Duration
	FirstTokenMs *int // 首字时间（流式请求）
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
func (s *GatewayService) BindStickySession(ctx context.Context, sessionHash string, accountID int64) error {
	if sessionHash == "" || accountID <= 0 || s.cache == nil {
		return nil
REDACTED
	return s.cache.SetSessionAccountID(ctx, sessionHash, accountID, stickySessionTTL)
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
		// 根据分组 platform 决定查询哪种账号
		group, err := s.groupRepo.GetByID(ctx, *groupID)
		if err != nil {
			return nil, fmt.Errorf("get group failed: %w", err)
	REDACTED
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

	// 强制平台模式：优先按分组查找，找不到再查全部该平台账户
	if hasForcePlatform && groupID != nil {
		account, err := s.selectAccountForModelWithPlatform(ctx, groupID, sessionHash, requestedModel, excludedIDs, platform)
		if err == nil {
			return account, nil
	REDACTED
		// 分组中找不到，回退查询全部该平台账户
		groupID = nil
REDACTED

	// antigravity 分组、强制平台模式或无分组使用单平台选择
	return s.selectAccountForModelWithPlatform(ctx, groupID, sessionHash, requestedModel, excludedIDs, platform)
REDACTED

// SelectAccountWithLoadAwareness selects account with load-awareness and wait plan.
func (s *GatewayService) SelectAccountWithLoadAwareness(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{REDACTED) (*AccountSelectionResult, error) {
	cfg := s.schedulingConfig()
	var stickyAccountID int64
	if sessionHash != "" && s.cache != nil {
		if accountID, err := s.cache.GetSessionAccountID(ctx, sessionHash); err == nil {
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

	platform, hasForcePlatform, err := s.resolvePlatform(ctx, groupID)
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
		accountID, err := s.cache.GetSessionAccountID(ctx, sessionHash)
		if err == nil && accountID > 0 && !isExcluded(accountID) {
			account, err := s.accountRepo.GetByID(ctx, accountID)
			if err == nil && s.isAccountAllowedForPlatform(account, platform, useMixed) &&
				account.IsSchedulable() &&
				(requestedModel == "" || s.isModelSupportedByAccount(account, requestedModel)) {
				result, err := s.tryAcquireAccountSlot(ctx, accountID, account.Concurrency)
				if err == nil && result.Acquired {
					_ = s.cache.RefreshSessionTTL(ctx, sessionHash, stickySessionTTL)
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
		if result, ok := s.tryAcquireByLegacyOrder(ctx, candidates, sessionHash, preferOAuth); ok {
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
					if sessionHash != "" {
						_ = s.cache.SetSessionAccountID(ctx, sessionHash, item.account.ID, stickySessionTTL)
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

func (s *GatewayService) tryAcquireByLegacyOrder(ctx context.Context, candidates []*Account, sessionHash string, preferOAuth bool) (*AccountSelectionResult, bool) {
	ordered := append([]*Account(nil), candidates...)
	sortAccountsByPriorityAndLastUsed(ordered, preferOAuth)

	for _, acc := range ordered {
		result, err := s.tryAcquireAccountSlot(ctx, acc.ID, acc.Concurrency)
		if err == nil && result.Acquired {
			if sessionHash != "" {
				_ = s.cache.SetSessionAccountID(ctx, sessionHash, acc.ID, stickySessionTTL)
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

func (s *GatewayService) resolvePlatform(ctx context.Context, groupID *int64) (string, bool, error) {
	forcePlatform, hasForcePlatform := ctx.Value(ctxkey.ForcePlatform).(string)
	if hasForcePlatform && forcePlatform != "" {
		return forcePlatform, true, nil
REDACTED
	if groupID != nil {
		group, err := s.groupRepo.GetByID(ctx, *groupID)
		if err != nil {
			return "", false, fmt.Errorf("get group failed: %w", err)
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
		if err == nil && len(accounts) == 0 && hasForcePlatform {
			accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, platform)
	REDACTED
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
	if sessionHash != "" {
		accountID, err := s.cache.GetSessionAccountID(ctx, sessionHash)
		if err == nil && accountID > 0 {
			if _, excluded := excludedIDs[accountID]; !excluded {
				account, err := s.accountRepo.GetByID(ctx, accountID)
				// 检查账号平台是否匹配（确保粘性会话不会跨平台）
				if err == nil && account.Platform == platform && account.IsSchedulable() && (requestedModel == "" || s.isModelSupportedByAccount(account, requestedModel)) {
					if err := s.cache.RefreshSessionTTL(ctx, sessionHash, stickySessionTTL); err != nil {
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
	if sessionHash != "" {
		if err := s.cache.SetSessionAccountID(ctx, sessionHash, selected.ID, stickySessionTTL); err != nil {
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
	if sessionHash != "" {
		accountID, err := s.cache.GetSessionAccountID(ctx, sessionHash)
		if err == nil && accountID > 0 {
			if _, excluded := excludedIDs[accountID]; !excluded {
				account, err := s.accountRepo.GetByID(ctx, accountID)
				// 检查账号是否有效：原生平台直接匹配，antigravity 需要启用混合调度
				if err == nil && account.IsSchedulable() && (requestedModel == "" || s.isModelSupportedByAccount(account, requestedModel)) {
					if account.Platform == nativePlatform || (account.Platform == PlatformAntigravity && account.IsMixedSchedulingEnabled()) {
						if err := s.cache.RefreshSessionTTL(ctx, sessionHash, stickySessionTTL); err != nil {
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
	if sessionHash != "" {
		if err := s.cache.SetSessionAccountID(ctx, sessionHash, selected.ID, stickySessionTTL); err != nil {
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
	case AccountTypeApiKey:
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
	maxRetries = 10              // 最大重试次数
	retryDelay = 3 * time.Second // 重试等待时间
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

// Forward 转发请求到Claude API
func (s *GatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, parsed *ParsedRequest) (*ForwardResult, error) {
	startTime := time.Now()
	if parsed == nil {
		return nil, fmt.Errorf("parse request: empty request")
REDACTED

	body := parsed.Body
	reqModel := parsed.Model
	reqStream := parsed.Stream

	if !parsed.HasSystem {
		body, _ = sjson.SetBytes(body, "system", []any{
			map[string]any{
				"type": "text",
				"text": "You are Claude Code, Anthropic's official CLI for Claude.",
				"cache_control": map[string]string{
					"type": "ephemeral",
			REDACTED,
		REDACTED,
	REDACTED)
REDACTED

	// 应用模型映射（仅对apikey类型账号）
	originalModel := reqModel
	if account.Type == AccountTypeApiKey {
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
	for attempt := 1; attempt <= maxRetries; attempt++ {
		// 构建上游请求（每次重试需要重新构建，因为请求体需要重新读取）
		upstreamReq, err := s.buildUpstreamRequest(ctx, c, account, body, token, tokenType, reqModel)
		if err != nil {
			return nil, err
	REDACTED

		// 发送请求
		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
		if err != nil {
			return nil, fmt.Errorf("upstream request failed: %w", err)
	REDACTED

		// 检查是否需要重试
		if resp.StatusCode >= 400 && s.shouldRetryUpstreamError(account, resp.StatusCode) {
			if attempt < maxRetries {
				log.Printf("Account %d: upstream error %d, retry %d/%d after %v",
					account.ID, resp.StatusCode, attempt, maxRetries, retryDelay)
				_ = resp.Body.Close()
				time.Sleep(retryDelay)
				continue
		REDACTED
			// 最后一次尝试也失败，跳出循环处理重试耗尽
			break
	REDACTED

		// 不需要重试（成功或不可重试的错误），跳出循环
		break
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
			respBody, readErr := io.ReadAll(resp.Body)
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
REDACTED else {
		usage, err = s.handleNonStreamingResponse(ctx, resp, c, account, originalModel, reqModel)
		if err != nil {
			return nil, err
	REDACTED
REDACTED

	return &ForwardResult{
		RequestID:    resp.Header.Get("x-request-id"),
		Usage:        *usage,
		Model:        originalModel, // 使用原始模型用于计费和日志
		Stream:       reqStream,
		Duration:     time.Since(startTime),
		FirstTokenMs: firstTokenMs,
REDACTED, nil
REDACTED

func (s *GatewayService) buildUpstreamRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token, tokenType, modelID string) (*http.Request, error) {
	// 确定目标URL
	targetURL := claudeAPIURL
	if account.Type == AccountTypeApiKey {
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
REDACTED else if s.cfg != nil && s.cfg.Gateway.InjectBetaForApiKey && req.Header.Get("anthropic-beta") == "" {
		// API-key：仅在请求显式使用 beta 特性且客户端未提供时，按需补齐（默认关闭）
		if requestNeedsBetaFeatures(body) {
			if beta := defaultApiKeyBetaHeader(body); beta != "" {
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

func defaultApiKeyBetaHeader(body []byte) string {
	modelID := gjson.GetBytes(body, "model").String()
	if strings.Contains(strings.ToLower(modelID), "haiku") {
		return claude.ApiKeyHaikuBetaHeader
REDACTED
	return claude.ApiKeyBetaHeader
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
	s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)

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
		log.Printf("Account %d: marked as error after %d retries for status %d", account.ID, maxRetries, statusCode)
REDACTED else {
		// API Key 未配置错误码：不标记账号状态
		log.Printf("Account %d: upstream error %d after %d retries (not marking account)", account.ID, statusCode, maxRetries)
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
	usage        *ClaudeUsage
	firstTokenMs *int
REDACTED

func (s *GatewayService) handleStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, startTime time.Time, originalModel, mappedModel string) (*streamingResult, error) {
	// 更新5h窗口状态
	s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)

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
	events := make(chan scanEvent, 1)
	done := make(chan struct{REDACTED)
	sendEvent := func(ev scanEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
	REDACTED
REDACTED
	go func() {
		defer close(events)
		for scanner.Scan() {
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
	// 仅监控上游数据间隔超时，避免上游挂起占用资源
	var intervalTimer *time.Timer
	if streamInterval > 0 {
		intervalTimer = time.NewTimer(streamInterval)
		defer intervalTimer.Stop()
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

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, nil
		REDACTED
			if ev.err != nil {
				if errors.Is(ev.err, bufio.ErrTooLong) {
					log.Printf("SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, ev.err)
					sendErrorEvent("response_too_large")
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, ev.err
			REDACTED
				sendErrorEvent("stream_read_error")
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, fmt.Errorf("stream read error: %w", ev.err)
		REDACTED
			line := ev.line
			if intervalTimer != nil {
				resetTimer(intervalTimer, streamInterval)
		REDACTED
			if line == "event: error" {
				return nil, errors.New("have error in stream")
		REDACTED

			// Extract data from SSE line (supports both "data: " and "data:" formats)
			if sseDataRe.MatchString(line) {
				data := sseDataRe.ReplaceAllString(line, "")

				// 如果有模型映射，替换响应中的model字段
				if needModelReplace {
					line = s.replaceModelInSSELine(line, mappedModel, originalModel)
			REDACTED

				// 转发行
				if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
					sendErrorEvent("write_failed")
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, err
			REDACTED
				flusher.Flush()

				// 记录首字时间：第一个有效的 content_block_delta 或 message_start
				if firstTokenMs == nil && data != "" && data != "[DONE]" {
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
			REDACTED
				s.parseSSEUsage(data, usage)
		REDACTED else {
				// 非 data 行直接转发
				if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
					sendErrorEvent("write_failed")
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, err
			REDACTED
				flusher.Flush()
		REDACTED

		case <-func() <-chan time.Time {
			if intervalTimer != nil {
				return intervalTimer.C
		REDACTED
			return nil
	REDACTED():
			log.Printf("Stream data interval timeout: account=%d model=%s interval=%s", account.ID, originalModel, streamInterval)
			sendErrorEvent("stream_timeout")
			return &streamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, fmt.Errorf("stream data interval timeout")
	REDACTED
REDACTED

	return &streamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, nil
REDACTED

func resetTimer(timer *time.Timer, interval time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
	REDACTED
REDACTED
	timer.Reset(interval)
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

	// 写入响应
	c.Data(resp.StatusCode, "application/json", body)

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
	ApiKey       *ApiKey
	User         *User
	Account      *Account
	Subscription *UserSubscription // 可选：订阅信息
REDACTED

// RecordUsage 记录使用量并扣费（或更新订阅用量）
func (s *GatewayService) RecordUsage(ctx context.Context, input *RecordUsageInput) error {
	result := input.Result
	apiKey := input.ApiKey
	user := input.User
	account := input.Account
	subscription := input.Subscription

	// 计算费用
	tokens := UsageTokens{
		InputTokens:         result.Usage.InputTokens,
		OutputTokens:        result.Usage.OutputTokens,
		CacheCreationTokens: result.Usage.CacheCreationInputTokens,
		CacheReadTokens:     result.Usage.CacheReadInputTokens,
REDACTED

	// 获取费率倍数
	multiplier := s.cfg.Default.RateMultiplier
	if apiKey.GroupID != nil && apiKey.Group != nil {
		multiplier = apiKey.Group.RateMultiplier
REDACTED

	cost, err := s.billingService.CalculateCost(result.Model, tokens, multiplier)
	if err != nil {
		log.Printf("Calculate cost failed: %v", err)
		// 使用默认费用继续
		cost = &CostBreakdown{ActualCost: 0REDACTED
REDACTED

	// 判断计费方式：订阅模式 vs 余额模式
	isSubscriptionBilling := subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
	billingType := BillingTypeBalance
	if isSubscriptionBilling {
		billingType = BillingTypeSubscription
REDACTED

	// 创建使用日志
	durationMs := int(result.Duration.Milliseconds())
	usageLog := &UsageLog{
		UserID:              user.ID,
		ApiKeyID:            apiKey.ID,
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
		CreatedAt:           time.Now(),
REDACTED

	// 添加分组和订阅关联
	if apiKey.GroupID != nil {
		usageLog.GroupID = apiKey.GroupID
REDACTED
	if subscription != nil {
		usageLog.SubscriptionID = &subscription.ID
REDACTED

	if err := s.usageLogRepo.Create(ctx, usageLog); err != nil {
		log.Printf("Create usage log failed: %v", err)
REDACTED

	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		log.Printf("[SIMPLE MODE] Usage recorded (not billed): user=%d, tokens=%d", usageLog.UserID, usageLog.TotalTokens())
		s.deferredService.ScheduleLastUsedUpdate(account.ID)
		return nil
REDACTED

	// 根据计费类型执行扣费
	if isSubscriptionBilling {
		// 订阅模式：更新订阅用量（使用 TotalCost 原始费用，不考虑倍率）
		if cost.TotalCost > 0 {
			if err := s.userSubRepo.IncrementUsage(ctx, subscription.ID, cost.TotalCost); err != nil {
				log.Printf("Increment subscription usage failed: %v", err)
		REDACTED
			// 异步更新订阅缓存
			s.billingCacheService.QueueUpdateSubscriptionUsage(user.ID, *apiKey.GroupID, cost.TotalCost)
	REDACTED
REDACTED else {
		// 余额模式：扣除用户余额（使用 ActualCost 考虑倍率后的费用）
		if cost.ActualCost > 0 {
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
	if account.Type == AccountTypeApiKey {
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
	defer func() {
		_ = resp.Body.Close()
REDACTED()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to read response")
		return err
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
	if account.Type == AccountTypeApiKey {
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
REDACTED else if s.cfg != nil && s.cfg.Gateway.InjectBetaForApiKey && req.Header.Get("anthropic-beta") == "" {
		// API-key：与 messages 同步的按需 beta 注入（默认关闭）
		if requestNeedsBetaFeatures(body) {
			if beta := defaultApiKeyBetaHeader(body); beta != "" {
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

package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// AdminService interface defines admin management operations
type AdminService interface {
	// User management
	ListUsers(ctx context.Context, page, pageSize int, filters UserListFilters) ([]User, int64, error)
	GetUser(ctx context.Context, id int64) (*User, error)
	CreateUser(ctx context.Context, input *CreateUserInput) (*User, error)
	UpdateUser(ctx context.Context, id int64, input *UpdateUserInput) (*User, error)
	DeleteUser(ctx context.Context, id int64) error
	UpdateUserBalance(ctx context.Context, userID int64, balance float64, operation string, notes string) (*User, error)
	GetUserAPIKeys(ctx context.Context, userID int64, page, pageSize int) ([]APIKey, int64, error)
	GetUserUsageStats(ctx context.Context, userID int64, period string) (any, error)

	// Group management
	ListGroups(ctx context.Context, page, pageSize int, platform, status, search string, isExclusive *bool) ([]Group, int64, error)
	GetAllGroups(ctx context.Context) ([]Group, error)
	GetAllGroupsByPlatform(ctx context.Context, platform string) ([]Group, error)
	GetGroup(ctx context.Context, id int64) (*Group, error)
	CreateGroup(ctx context.Context, input *CreateGroupInput) (*Group, error)
	UpdateGroup(ctx context.Context, id int64, input *UpdateGroupInput) (*Group, error)
	DeleteGroup(ctx context.Context, id int64) error
	GetGroupAPIKeys(ctx context.Context, groupID int64, page, pageSize int) ([]APIKey, int64, error)

	// Account management
	ListAccounts(ctx context.Context, page, pageSize int, platform, accountType, status, search string) ([]Account, int64, error)
	GetAccount(ctx context.Context, id int64) (*Account, error)
	GetAccountsByIDs(ctx context.Context, ids []int64) ([]*Account, error)
	CreateAccount(ctx context.Context, input *CreateAccountInput) (*Account, error)
	UpdateAccount(ctx context.Context, id int64, input *UpdateAccountInput) (*Account, error)
	DeleteAccount(ctx context.Context, id int64) error
	RefreshAccountCredentials(ctx context.Context, id int64) (*Account, error)
	ClearAccountError(ctx context.Context, id int64) (*Account, error)
	SetAccountSchedulable(ctx context.Context, id int64, schedulable bool) (*Account, error)
	BulkUpdateAccounts(ctx context.Context, input *BulkUpdateAccountsInput) (*BulkUpdateAccountsResult, error)

	// Proxy management
	ListProxies(ctx context.Context, page, pageSize int, protocol, status, search string) ([]Proxy, int64, error)
	ListProxiesWithAccountCount(ctx context.Context, page, pageSize int, protocol, status, search string) ([]ProxyWithAccountCount, int64, error)
	GetAllProxies(ctx context.Context) ([]Proxy, error)
	GetAllProxiesWithAccountCount(ctx context.Context) ([]ProxyWithAccountCount, error)
	GetProxy(ctx context.Context, id int64) (*Proxy, error)
	CreateProxy(ctx context.Context, input *CreateProxyInput) (*Proxy, error)
	UpdateProxy(ctx context.Context, id int64, input *UpdateProxyInput) (*Proxy, error)
	DeleteProxy(ctx context.Context, id int64) error
	GetProxyAccounts(ctx context.Context, proxyID int64, page, pageSize int) ([]Account, int64, error)
	CheckProxyExists(ctx context.Context, host string, port int, username, password string) (bool, error)
	TestProxy(ctx context.Context, id int64) (*ProxyTestResult, error)

	// Redeem code management
	ListRedeemCodes(ctx context.Context, page, pageSize int, codeType, status, search string) ([]RedeemCode, int64, error)
	GetRedeemCode(ctx context.Context, id int64) (*RedeemCode, error)
	GenerateRedeemCodes(ctx context.Context, input *GenerateRedeemCodesInput) ([]RedeemCode, error)
	DeleteRedeemCode(ctx context.Context, id int64) error
	BatchDeleteRedeemCodes(ctx context.Context, ids []int64) (int64, error)
	ExpireRedeemCode(ctx context.Context, id int64) (*RedeemCode, error)
REDACTED

// CreateUserInput represents input for creating a new user via admin operations.
type CreateUserInput struct {
	Email         string
	Password      string
	Username      string
	Notes         string
	Balance       float64
	Concurrency   int
	AllowedGroups []int64
REDACTED

type UpdateUserInput struct {
	Email         string
	Password      string
	Username      *string
	Notes         *string
	Balance       *float64 // 使用指针区分"未提供"和"设置为0"
	Concurrency   *int     // 使用指针区分"未提供"和"设置为0"
	Status        string
	AllowedGroups *[]int64 // 使用指针区分"未提供"和"设置为空数组"
REDACTED

type CreateGroupInput struct {
	Name             string
	Description      string
	Platform         string
	RateMultiplier   float64
	IsExclusive      bool
	SubscriptionType string   // standard/subscription
	DailyLimitUSD    *float64 // 日限额 (USD)
	WeeklyLimitUSD   *float64 // 周限额 (USD)
	MonthlyLimitUSD  *float64 // 月限额 (USD)
	// 图片生成计费配置（仅 antigravity 平台使用）
	ImagePrice1K    *float64
	ImagePrice2K    *float64
	ImagePrice4K    *float64
	ClaudeCodeOnly  bool   // 仅允许 Claude Code 客户端
	FallbackGroupID *int64 // 降级分组 ID
REDACTED

type UpdateGroupInput struct {
	Name             string
	Description      string
	Platform         string
	RateMultiplier   *float64 // 使用指针以支持设置为0
	IsExclusive      *bool
	Status           string
	SubscriptionType string   // standard/subscription
	DailyLimitUSD    *float64 // 日限额 (USD)
	WeeklyLimitUSD   *float64 // 周限额 (USD)
	MonthlyLimitUSD  *float64 // 月限额 (USD)
	// 图片生成计费配置（仅 antigravity 平台使用）
	ImagePrice1K    *float64
	ImagePrice2K    *float64
	ImagePrice4K    *float64
	ClaudeCodeOnly  *bool  // 仅允许 Claude Code 客户端
	FallbackGroupID *int64 // 降级分组 ID
REDACTED

type CreateAccountInput struct {
	Name               string
	Notes              *string
	Platform           string
	Type               string
	Credentials        map[string]any
	Extra              map[string]any
	ProxyID            *int64
	Concurrency        int
	Priority           int
	RateMultiplier     *float64 // 账号计费倍率（>=0，允许 0）
	GroupIDs           []int64
	ExpiresAt          *int64
	AutoPauseOnExpired *bool
	// SkipMixedChannelCheck skips the mixed channel risk check when binding groups.
	// This should only be set when the caller has explicitly confirmed the risk.
	SkipMixedChannelCheck bool
REDACTED

type UpdateAccountInput struct {
	Name                  string
	Notes                 *string
	Type                  string // Account type: oauth, setup-token, apikey
	Credentials           map[string]any
	Extra                 map[string]any
	ProxyID               *int64
	Concurrency           *int     // 使用指针区分"未提供"和"设置为0"
	Priority              *int     // 使用指针区分"未提供"和"设置为0"
	RateMultiplier        *float64 // 账号计费倍率（>=0，允许 0）
	Status                string
	GroupIDs              *[]int64
	ExpiresAt             *int64
	AutoPauseOnExpired    *bool
	SkipMixedChannelCheck bool // 跳过混合渠道检查（用户已确认风险）
REDACTED

// BulkUpdateAccountsInput describes the payload for bulk updating accounts.
type BulkUpdateAccountsInput struct {
	AccountIDs     []int64
	Name           string
	ProxyID        *int64
	Concurrency    *int
	Priority       *int
	RateMultiplier *float64 // 账号计费倍率（>=0，允许 0）
	Status         string
	Schedulable    *bool
	GroupIDs       *[]int64
	Credentials    map[string]any
	Extra          map[string]any
	// SkipMixedChannelCheck skips the mixed channel risk check when binding groups.
	// This should only be set when the caller has explicitly confirmed the risk.
	SkipMixedChannelCheck bool
REDACTED

// BulkUpdateAccountResult captures the result for a single account update.
type BulkUpdateAccountResult struct {
	AccountID int64  `json:"account_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
REDACTED

// BulkUpdateAccountsResult is the aggregated response for bulk updates.
type BulkUpdateAccountsResult struct {
	Success    int                       `json:"success"`
	Failed     int                       `json:"failed"`
	SuccessIDs []int64                   `json:"success_ids"`
	FailedIDs  []int64                   `json:"failed_ids"`
	Results    []BulkUpdateAccountResult `json:"results"`
REDACTED

type CreateProxyInput struct {
	Name     string
	Protocol string
	Host     string
	Port     int
	Username string
	Password string
REDACTED

type UpdateProxyInput struct {
	Name     string
	Protocol string
	Host     string
	Port     int
	Username string
	Password string
	Status   string
REDACTED

type GenerateRedeemCodesInput struct {
	Count        int
	Type         string
	Value        float64
	GroupID      *int64 // 订阅类型专用：关联的分组ID
	ValidityDays int    // 订阅类型专用：有效天数
REDACTED

// ProxyTestResult represents the result of testing a proxy
type ProxyTestResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	IPAddress string `json:"ip_address,omitempty"`
	City      string `json:"city,omitempty"`
	Region    string `json:"region,omitempty"`
	Country   string `json:"country,omitempty"`
REDACTED

// ProxyExitInfo represents proxy exit information from ipinfo.io
type ProxyExitInfo struct {
	IP      string
	City    string
	Region  string
	Country string
REDACTED

// ProxyExitInfoProber tests proxy connectivity and retrieves exit information
type ProxyExitInfoProber interface {
	ProbeProxy(ctx context.Context, proxyURL string) (*ProxyExitInfo, int64, error)
REDACTED

// adminServiceImpl implements AdminService
type adminServiceImpl struct {
	userRepo             UserRepository
	groupRepo            GroupRepository
	accountRepo          AccountRepository
	proxyRepo            ProxyRepository
	apiKeyRepo           APIKeyRepository
	redeemCodeRepo       RedeemCodeRepository
	billingCacheService  *BillingCacheService
	proxyProber          ProxyExitInfoProber
	authCacheInvalidator APIKeyAuthCacheInvalidator
REDACTED

// NewAdminService creates a new AdminService
func NewAdminService(
	userRepo UserRepository,
	groupRepo GroupRepository,
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	apiKeyRepo APIKeyRepository,
	redeemCodeRepo RedeemCodeRepository,
	billingCacheService *BillingCacheService,
	proxyProber ProxyExitInfoProber,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
) AdminService {
	return &adminServiceImpl{
		userRepo:             userRepo,
		groupRepo:            groupRepo,
		accountRepo:          accountRepo,
		proxyRepo:            proxyRepo,
		apiKeyRepo:           apiKeyRepo,
		redeemCodeRepo:       redeemCodeRepo,
		billingCacheService:  billingCacheService,
		proxyProber:          proxyProber,
		authCacheInvalidator: authCacheInvalidator,
REDACTED
REDACTED

// User management implementations
func (s *adminServiceImpl) ListUsers(ctx context.Context, page, pageSize int, filters UserListFilters) ([]User, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	users, result, err := s.userRepo.ListWithFilters(ctx, params, filters)
	if err != nil {
		return nil, 0, err
REDACTED
	return users, result.Total, nil
REDACTED

func (s *adminServiceImpl) GetUser(ctx context.Context, id int64) (*User, error) {
	return s.userRepo.GetByID(ctx, id)
REDACTED

func (s *adminServiceImpl) CreateUser(ctx context.Context, input *CreateUserInput) (*User, error) {
	user := &User{
		Email:         input.Email,
		Username:      input.Username,
		Notes:         input.Notes,
		Role:          RoleUser, // Always create as regular user, never admin
		Balance:       input.Balance,
		Concurrency:   input.Concurrency,
		Status:        StatusActive,
		AllowedGroups: input.AllowedGroups,
REDACTED
	if err := user.SetPassword(input.Password); err != nil {
		return nil, err
REDACTED
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
REDACTED
	return user, nil
REDACTED

func (s *adminServiceImpl) UpdateUser(ctx context.Context, id int64, input *UpdateUserInput) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED

	// Protect admin users: cannot disable admin accounts
	if user.Role == "admin" && input.Status == "disabled" {
		return nil, errors.New("cannot disable admin user")
REDACTED

	oldConcurrency := user.Concurrency
	oldStatus := user.Status
	oldRole := user.Role

	if input.Email != "" {
		user.Email = input.Email
REDACTED
	if input.Password != "" {
		if err := user.SetPassword(input.Password); err != nil {
			return nil, err
	REDACTED
REDACTED

	if input.Username != nil {
		user.Username = *input.Username
REDACTED
	if input.Notes != nil {
		user.Notes = *input.Notes
REDACTED

	if input.Status != "" {
		user.Status = input.Status
REDACTED

	if input.Concurrency != nil {
		user.Concurrency = *input.Concurrency
REDACTED

	if input.AllowedGroups != nil {
		user.AllowedGroups = *input.AllowedGroups
REDACTED

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
REDACTED
	if s.authCacheInvalidator != nil {
		if user.Concurrency != oldConcurrency || user.Status != oldStatus || user.Role != oldRole {
			s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, user.ID)
	REDACTED
REDACTED

	concurrencyDiff := user.Concurrency - oldConcurrency
	if concurrencyDiff != 0 {
		code, err := GenerateRedeemCode()
		if err != nil {
			log.Printf("failed to generate adjustment redeem code: %v", err)
			return user, nil
	REDACTED
		adjustmentRecord := &RedeemCode{
			Code:   code,
			Type:   AdjustmentTypeAdminConcurrency,
			Value:  float64(concurrencyDiff),
			Status: StatusUsed,
			UsedBy: &user.ID,
	REDACTED
		now := time.Now()
		adjustmentRecord.UsedAt = &now
		if err := s.redeemCodeRepo.Create(ctx, adjustmentRecord); err != nil {
			log.Printf("failed to create concurrency adjustment redeem code: %v", err)
	REDACTED
REDACTED

	return user, nil
REDACTED

func (s *adminServiceImpl) DeleteUser(ctx context.Context, id int64) error {
	// Protect admin users: cannot delete admin accounts
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return err
REDACTED
	if user.Role == "admin" {
		return errors.New("cannot delete admin user")
REDACTED
	if err := s.userRepo.Delete(ctx, id); err != nil {
		log.Printf("delete user failed: user_id=%d err=%v", id, err)
		return err
REDACTED
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, id)
REDACTED
	return nil
REDACTED

func (s *adminServiceImpl) UpdateUserBalance(ctx context.Context, userID int64, balance float64, operation string, notes string) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
REDACTED

	oldBalance := user.Balance

	switch operation {
	case "set":
		user.Balance = balance
	case "add":
		user.Balance += balance
	case "subtract":
		user.Balance -= balance
REDACTED

	if user.Balance < 0 {
		return nil, fmt.Errorf("balance cannot be negative, current balance: %.2f, requested operation would result in: %.2f", oldBalance, user.Balance)
REDACTED

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
REDACTED
	balanceDiff := user.Balance - oldBalance
	if s.authCacheInvalidator != nil && balanceDiff != 0 {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
REDACTED

	if s.billingCacheService != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.billingCacheService.InvalidateUserBalance(cacheCtx, userID); err != nil {
				log.Printf("invalidate user balance cache failed: user_id=%d err=%v", userID, err)
		REDACTED
	REDACTED()
REDACTED

	if balanceDiff != 0 {
		code, err := GenerateRedeemCode()
		if err != nil {
			log.Printf("failed to generate adjustment redeem code: %v", err)
			return user, nil
	REDACTED

		adjustmentRecord := &RedeemCode{
			Code:   code,
			Type:   AdjustmentTypeAdminBalance,
			Value:  balanceDiff,
			Status: StatusUsed,
			UsedBy: &user.ID,
			Notes:  notes,
	REDACTED
		now := time.Now()
		adjustmentRecord.UsedAt = &now

		if err := s.redeemCodeRepo.Create(ctx, adjustmentRecord); err != nil {
			log.Printf("failed to create balance adjustment redeem code: %v", err)
	REDACTED
REDACTED

	return user, nil
REDACTED

func (s *adminServiceImpl) GetUserAPIKeys(ctx context.Context, userID int64, page, pageSize int) ([]APIKey, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	keys, result, err := s.apiKeyRepo.ListByUserID(ctx, userID, params)
	if err != nil {
		return nil, 0, err
REDACTED
	return keys, result.Total, nil
REDACTED

func (s *adminServiceImpl) GetUserUsageStats(ctx context.Context, userID int64, period string) (any, error) {
	// Return mock data for now
	return map[string]any{
		"period":          period,
		"total_requests":  0,
		"total_cost":      0.0,
		"total_tokens":    0,
		"avg_duration_ms": 0,
REDACTED, nil
REDACTED

// Group management implementations
func (s *adminServiceImpl) ListGroups(ctx context.Context, page, pageSize int, platform, status, search string, isExclusive *bool) ([]Group, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	groups, result, err := s.groupRepo.ListWithFilters(ctx, params, platform, status, search, isExclusive)
	if err != nil {
		return nil, 0, err
REDACTED
	return groups, result.Total, nil
REDACTED

func (s *adminServiceImpl) GetAllGroups(ctx context.Context) ([]Group, error) {
	return s.groupRepo.ListActive(ctx)
REDACTED

func (s *adminServiceImpl) GetAllGroupsByPlatform(ctx context.Context, platform string) ([]Group, error) {
	return s.groupRepo.ListActiveByPlatform(ctx, platform)
REDACTED

func (s *adminServiceImpl) GetGroup(ctx context.Context, id int64) (*Group, error) {
	return s.groupRepo.GetByID(ctx, id)
REDACTED

func (s *adminServiceImpl) CreateGroup(ctx context.Context, input *CreateGroupInput) (*Group, error) {
	platform := input.Platform
	if platform == "" {
		platform = PlatformAnthropic
REDACTED

	subscriptionType := input.SubscriptionType
	if subscriptionType == "" {
		subscriptionType = SubscriptionTypeStandard
REDACTED

	// 限额字段：0 和 nil 都表示"无限制"
	dailyLimit := normalizeLimit(input.DailyLimitUSD)
	weeklyLimit := normalizeLimit(input.WeeklyLimitUSD)
	monthlyLimit := normalizeLimit(input.MonthlyLimitUSD)

	// 图片价格：负数表示清除（使用默认价格），0 保留（表示免费）
	imagePrice1K := normalizePrice(input.ImagePrice1K)
	imagePrice2K := normalizePrice(input.ImagePrice2K)
	imagePrice4K := normalizePrice(input.ImagePrice4K)

	// 校验降级分组
	if input.FallbackGroupID != nil {
		if err := s.validateFallbackGroup(ctx, 0, *input.FallbackGroupID); err != nil {
			return nil, err
	REDACTED
REDACTED

	group := &Group{
		Name:             input.Name,
		Description:      input.Description,
		Platform:         platform,
		RateMultiplier:   input.RateMultiplier,
		IsExclusive:      input.IsExclusive,
		Status:           StatusActive,
		SubscriptionType: subscriptionType,
		DailyLimitUSD:    dailyLimit,
		WeeklyLimitUSD:   weeklyLimit,
		MonthlyLimitUSD:  monthlyLimit,
		ImagePrice1K:     imagePrice1K,
		ImagePrice2K:     imagePrice2K,
		ImagePrice4K:     imagePrice4K,
		ClaudeCodeOnly:   input.ClaudeCodeOnly,
		FallbackGroupID:  input.FallbackGroupID,
REDACTED
	if err := s.groupRepo.Create(ctx, group); err != nil {
		return nil, err
REDACTED
	return group, nil
REDACTED

// normalizeLimit 将 0 或负数转换为 nil（表示无限制）
func normalizeLimit(limit *float64) *float64 {
	if limit == nil || *limit <= 0 {
		return nil
REDACTED
	return limit
REDACTED

// normalizePrice 将负数转换为 nil（表示使用默认价格），0 保留（表示免费）
func normalizePrice(price *float64) *float64 {
	if price == nil || *price < 0 {
		return nil
REDACTED
	return price
REDACTED

// validateFallbackGroup 校验降级分组的有效性
// currentGroupID: 当前分组 ID（新建时为 0）
// fallbackGroupID: 降级分组 ID
func (s *adminServiceImpl) validateFallbackGroup(ctx context.Context, currentGroupID, fallbackGroupID int64) error {
	// 不能将自己设置为降级分组
	if currentGroupID > 0 && currentGroupID == fallbackGroupID {
		return fmt.Errorf("cannot set self as fallback group")
REDACTED

	visited := map[int64]struct{REDACTED{REDACTED
	nextID := fallbackGroupID
	for {
		if _, seen := visited[nextID]; seen {
			return fmt.Errorf("fallback group cycle detected")
	REDACTED
		visited[nextID] = struct{REDACTED{REDACTED
		if currentGroupID > 0 && nextID == currentGroupID {
			return fmt.Errorf("fallback group cycle detected")
	REDACTED

		// 检查降级分组是否存在
		fallbackGroup, err := s.groupRepo.GetByIDLite(ctx, nextID)
		if err != nil {
			return fmt.Errorf("fallback group not found: %w", err)
	REDACTED

		// 降级分组不能启用 claude_code_only，否则会造成死循环
		if nextID == fallbackGroupID && fallbackGroup.ClaudeCodeOnly {
			return fmt.Errorf("fallback group cannot have claude_code_only enabled")
	REDACTED

		if fallbackGroup.FallbackGroupID == nil {
			return nil
	REDACTED
		nextID = *fallbackGroup.FallbackGroupID
REDACTED
REDACTED

func (s *adminServiceImpl) UpdateGroup(ctx context.Context, id int64, input *UpdateGroupInput) (*Group, error) {
	group, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED

	if input.Name != "" {
		group.Name = input.Name
REDACTED
	if input.Description != "" {
		group.Description = input.Description
REDACTED
	if input.Platform != "" {
		group.Platform = input.Platform
REDACTED
	if input.RateMultiplier != nil {
		group.RateMultiplier = *input.RateMultiplier
REDACTED
	if input.IsExclusive != nil {
		group.IsExclusive = *input.IsExclusive
REDACTED
	if input.Status != "" {
		group.Status = input.Status
REDACTED

	// 订阅相关字段
	if input.SubscriptionType != "" {
		group.SubscriptionType = input.SubscriptionType
REDACTED
	// 限额字段：0 和 nil 都表示"无限制"，正数表示具体限额
	if input.DailyLimitUSD != nil {
		group.DailyLimitUSD = normalizeLimit(input.DailyLimitUSD)
REDACTED
	if input.WeeklyLimitUSD != nil {
		group.WeeklyLimitUSD = normalizeLimit(input.WeeklyLimitUSD)
REDACTED
	if input.MonthlyLimitUSD != nil {
		group.MonthlyLimitUSD = normalizeLimit(input.MonthlyLimitUSD)
REDACTED
	// 图片生成计费配置：负数表示清除（使用默认价格）
	if input.ImagePrice1K != nil {
		group.ImagePrice1K = normalizePrice(input.ImagePrice1K)
REDACTED
	if input.ImagePrice2K != nil {
		group.ImagePrice2K = normalizePrice(input.ImagePrice2K)
REDACTED
	if input.ImagePrice4K != nil {
		group.ImagePrice4K = normalizePrice(input.ImagePrice4K)
REDACTED

	// Claude Code 客户端限制
	if input.ClaudeCodeOnly != nil {
		group.ClaudeCodeOnly = *input.ClaudeCodeOnly
REDACTED
	if input.FallbackGroupID != nil {
		// 校验降级分组
		if *input.FallbackGroupID > 0 {
			if err := s.validateFallbackGroup(ctx, id, *input.FallbackGroupID); err != nil {
				return nil, err
		REDACTED
			group.FallbackGroupID = input.FallbackGroupID
	REDACTED else {
			// 传入 0 或负数表示清除降级分组
			group.FallbackGroupID = nil
	REDACTED
REDACTED

	if err := s.groupRepo.Update(ctx, group); err != nil {
		return nil, err
REDACTED
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, id)
REDACTED
	return group, nil
REDACTED

func (s *adminServiceImpl) DeleteGroup(ctx context.Context, id int64) error {
	var groupKeys []string
	if s.authCacheInvalidator != nil {
		keys, err := s.apiKeyRepo.ListKeysByGroupID(ctx, id)
		if err == nil {
			groupKeys = keys
	REDACTED
REDACTED

	affectedUserIDs, err := s.groupRepo.DeleteCascade(ctx, id)
	if err != nil {
		return err
REDACTED

	// 事务成功后，异步失效受影响用户的订阅缓存
	if len(affectedUserIDs) > 0 && s.billingCacheService != nil {
		groupID := id
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			for _, userID := range affectedUserIDs {
				if err := s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID); err != nil {
					log.Printf("invalidate subscription cache failed: user_id=%d group_id=%d err=%v", userID, groupID, err)
			REDACTED
		REDACTED
	REDACTED()
REDACTED
	if s.authCacheInvalidator != nil {
		for _, key := range groupKeys {
			s.authCacheInvalidator.InvalidateAuthCacheByKey(ctx, key)
	REDACTED
REDACTED

	return nil
REDACTED

func (s *adminServiceImpl) GetGroupAPIKeys(ctx context.Context, groupID int64, page, pageSize int) ([]APIKey, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	keys, result, err := s.apiKeyRepo.ListByGroupID(ctx, groupID, params)
	if err != nil {
		return nil, 0, err
REDACTED
	return keys, result.Total, nil
REDACTED

// Account management implementations
func (s *adminServiceImpl) ListAccounts(ctx context.Context, page, pageSize int, platform, accountType, status, search string) ([]Account, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	accounts, result, err := s.accountRepo.ListWithFilters(ctx, params, platform, accountType, status, search)
	if err != nil {
		return nil, 0, err
REDACTED
	return accounts, result.Total, nil
REDACTED

func (s *adminServiceImpl) GetAccount(ctx context.Context, id int64) (*Account, error) {
	return s.accountRepo.GetByID(ctx, id)
REDACTED

func (s *adminServiceImpl) GetAccountsByIDs(ctx context.Context, ids []int64) ([]*Account, error) {
	if len(ids) == 0 {
		return []*Account{REDACTED, nil
REDACTED

	accounts, err := s.accountRepo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get accounts by IDs: %w", err)
REDACTED

	return accounts, nil
REDACTED

func (s *adminServiceImpl) CreateAccount(ctx context.Context, input *CreateAccountInput) (*Account, error) {
	// 绑定分组
	groupIDs := input.GroupIDs
	// 如果没有指定分组,自动绑定对应平台的默认分组
	if len(groupIDs) == 0 {
		defaultGroupName := input.Platform + "-default"
		groups, err := s.groupRepo.ListActiveByPlatform(ctx, input.Platform)
		if err == nil {
			for _, g := range groups {
				if g.Name == defaultGroupName {
					groupIDs = []int64{g.IDREDACTED
					break
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// 检查混合渠道风险（除非用户已确认）
	if len(groupIDs) > 0 && !input.SkipMixedChannelCheck {
		if err := s.checkMixedChannelRisk(ctx, 0, input.Platform, groupIDs); err != nil {
			return nil, err
	REDACTED
REDACTED

	account := &Account{
		Name:        input.Name,
		Notes:       normalizeAccountNotes(input.Notes),
		Platform:    input.Platform,
		Type:        input.Type,
		Credentials: input.Credentials,
		Extra:       input.Extra,
		ProxyID:     input.ProxyID,
		Concurrency: input.Concurrency,
		Priority:    input.Priority,
		Status:      StatusActive,
		Schedulable: true,
REDACTED
	if input.ExpiresAt != nil && *input.ExpiresAt > 0 {
		expiresAt := time.Unix(*input.ExpiresAt, 0)
		account.ExpiresAt = &expiresAt
REDACTED
	if input.AutoPauseOnExpired != nil {
		account.AutoPauseOnExpired = *input.AutoPauseOnExpired
REDACTED else {
		account.AutoPauseOnExpired = true
REDACTED
	if input.RateMultiplier != nil {
		if *input.RateMultiplier < 0 {
			return nil, errors.New("rate_multiplier must be >= 0")
	REDACTED
		account.RateMultiplier = input.RateMultiplier
REDACTED
	if err := s.accountRepo.Create(ctx, account); err != nil {
		return nil, err
REDACTED

	// 绑定分组
	if len(groupIDs) > 0 {
		if err := s.accountRepo.BindGroups(ctx, account.ID, groupIDs); err != nil {
			return nil, err
	REDACTED
REDACTED

	return account, nil
REDACTED

func (s *adminServiceImpl) UpdateAccount(ctx context.Context, id int64, input *UpdateAccountInput) (*Account, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED

	if input.Name != "" {
		account.Name = input.Name
REDACTED
	if input.Type != "" {
		account.Type = input.Type
REDACTED
	if input.Notes != nil {
		account.Notes = normalizeAccountNotes(input.Notes)
REDACTED
	if len(input.Credentials) > 0 {
		account.Credentials = input.Credentials
REDACTED
	if len(input.Extra) > 0 {
		account.Extra = input.Extra
REDACTED
	if input.ProxyID != nil {
		// 0 表示清除代理（前端发送 0 而不是 null 来表达清除意图）
		if *input.ProxyID == 0 {
			account.ProxyID = nil
	REDACTED else {
			account.ProxyID = input.ProxyID
	REDACTED
		account.Proxy = nil // 清除关联对象，防止 GORM Save 时根据 Proxy.ID 覆盖 ProxyID
REDACTED
	// 只在指针非 nil 时更新 Concurrency（支持设置为 0）
	if input.Concurrency != nil {
		account.Concurrency = *input.Concurrency
REDACTED
	// 只在指针非 nil 时更新 Priority（支持设置为 0）
	if input.Priority != nil {
		account.Priority = *input.Priority
REDACTED
	if input.RateMultiplier != nil {
		if *input.RateMultiplier < 0 {
			return nil, errors.New("rate_multiplier must be >= 0")
	REDACTED
		account.RateMultiplier = input.RateMultiplier
REDACTED
	if input.Status != "" {
		account.Status = input.Status
REDACTED
	if input.ExpiresAt != nil {
		if *input.ExpiresAt <= 0 {
			account.ExpiresAt = nil
	REDACTED else {
			expiresAt := time.Unix(*input.ExpiresAt, 0)
			account.ExpiresAt = &expiresAt
	REDACTED
REDACTED
	if input.AutoPauseOnExpired != nil {
		account.AutoPauseOnExpired = *input.AutoPauseOnExpired
REDACTED

	// 先验证分组是否存在（在任何写操作之前）
	if input.GroupIDs != nil {
		for _, groupID := range *input.GroupIDs {
			if _, err := s.groupRepo.GetByID(ctx, groupID); err != nil {
				return nil, fmt.Errorf("get group: %w", err)
		REDACTED
	REDACTED

		// 检查混合渠道风险（除非用户已确认）
		if !input.SkipMixedChannelCheck {
			if err := s.checkMixedChannelRisk(ctx, account.ID, account.Platform, *input.GroupIDs); err != nil {
				return nil, err
		REDACTED
	REDACTED
REDACTED

	if err := s.accountRepo.Update(ctx, account); err != nil {
		return nil, err
REDACTED

	// 绑定分组
	if input.GroupIDs != nil {
		if err := s.accountRepo.BindGroups(ctx, account.ID, *input.GroupIDs); err != nil {
			return nil, err
	REDACTED
REDACTED

	// 重新查询以确保返回完整数据（包括正确的 Proxy 关联对象）
	return s.accountRepo.GetByID(ctx, id)
REDACTED

// BulkUpdateAccounts updates multiple accounts in one request.
// It merges credentials/extra keys instead of overwriting the whole object.
func (s *adminServiceImpl) BulkUpdateAccounts(ctx context.Context, input *BulkUpdateAccountsInput) (*BulkUpdateAccountsResult, error) {
	result := &BulkUpdateAccountsResult{
		SuccessIDs: make([]int64, 0, len(input.AccountIDs)),
		FailedIDs:  make([]int64, 0, len(input.AccountIDs)),
		Results:    make([]BulkUpdateAccountResult, 0, len(input.AccountIDs)),
REDACTED

	if len(input.AccountIDs) == 0 {
		return result, nil
REDACTED

	// Preload account platforms for mixed channel risk checks if group bindings are requested.
	platformByID := map[int64]string{REDACTED
	if input.GroupIDs != nil && !input.SkipMixedChannelCheck {
		accounts, err := s.accountRepo.GetByIDs(ctx, input.AccountIDs)
		if err != nil {
			return nil, err
	REDACTED
		for _, account := range accounts {
			if account != nil {
				platformByID[account.ID] = account.Platform
		REDACTED
	REDACTED
REDACTED

	if input.RateMultiplier != nil {
		if *input.RateMultiplier < 0 {
			return nil, errors.New("rate_multiplier must be >= 0")
	REDACTED
REDACTED

	// Prepare bulk updates for columns and JSONB fields.
	repoUpdates := AccountBulkUpdate{
		Credentials: input.Credentials,
		Extra:       input.Extra,
REDACTED
	if input.Name != "" {
		repoUpdates.Name = &input.Name
REDACTED
	if input.ProxyID != nil {
		repoUpdates.ProxyID = input.ProxyID
REDACTED
	if input.Concurrency != nil {
		repoUpdates.Concurrency = input.Concurrency
REDACTED
	if input.Priority != nil {
		repoUpdates.Priority = input.Priority
REDACTED
	if input.RateMultiplier != nil {
		repoUpdates.RateMultiplier = input.RateMultiplier
REDACTED
	if input.Status != "" {
		repoUpdates.Status = &input.Status
REDACTED
	if input.Schedulable != nil {
		repoUpdates.Schedulable = input.Schedulable
REDACTED

	// Run bulk update for column/jsonb fields first.
	if _, err := s.accountRepo.BulkUpdate(ctx, input.AccountIDs, repoUpdates); err != nil {
		return nil, err
REDACTED

	// Handle group bindings per account (requires individual operations).
	for _, accountID := range input.AccountIDs {
		entry := BulkUpdateAccountResult{AccountID: accountIDREDACTED

		if input.GroupIDs != nil {
			// 检查混合渠道风险（除非用户已确认）
			if !input.SkipMixedChannelCheck {
				platform := platformByID[accountID]
				if platform == "" {
					account, err := s.accountRepo.GetByID(ctx, accountID)
					if err != nil {
						entry.Success = false
						entry.Error = err.Error()
						result.Failed++
						result.FailedIDs = append(result.FailedIDs, accountID)
						result.Results = append(result.Results, entry)
						continue
				REDACTED
					platform = account.Platform
			REDACTED
				if err := s.checkMixedChannelRisk(ctx, accountID, platform, *input.GroupIDs); err != nil {
					entry.Success = false
					entry.Error = err.Error()
					result.Failed++
					result.FailedIDs = append(result.FailedIDs, accountID)
					result.Results = append(result.Results, entry)
					continue
			REDACTED
		REDACTED

			if err := s.accountRepo.BindGroups(ctx, accountID, *input.GroupIDs); err != nil {
				entry.Success = false
				entry.Error = err.Error()
				result.Failed++
				result.FailedIDs = append(result.FailedIDs, accountID)
				result.Results = append(result.Results, entry)
				continue
		REDACTED
	REDACTED

		entry.Success = true
		result.Success++
		result.SuccessIDs = append(result.SuccessIDs, accountID)
		result.Results = append(result.Results, entry)
REDACTED

	return result, nil
REDACTED

func (s *adminServiceImpl) DeleteAccount(ctx context.Context, id int64) error {
	return s.accountRepo.Delete(ctx, id)
REDACTED

func (s *adminServiceImpl) RefreshAccountCredentials(ctx context.Context, id int64) (*Account, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED
	// TODO: Implement refresh logic
	return account, nil
REDACTED

func (s *adminServiceImpl) ClearAccountError(ctx context.Context, id int64) (*Account, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED
	account.Status = StatusActive
	account.ErrorMessage = ""
	if err := s.accountRepo.Update(ctx, account); err != nil {
		return nil, err
REDACTED
	return account, nil
REDACTED

func (s *adminServiceImpl) SetAccountSchedulable(ctx context.Context, id int64, schedulable bool) (*Account, error) {
	if err := s.accountRepo.SetSchedulable(ctx, id, schedulable); err != nil {
		return nil, err
REDACTED
	return s.accountRepo.GetByID(ctx, id)
REDACTED

// Proxy management implementations
func (s *adminServiceImpl) ListProxies(ctx context.Context, page, pageSize int, protocol, status, search string) ([]Proxy, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	proxies, result, err := s.proxyRepo.ListWithFilters(ctx, params, protocol, status, search)
	if err != nil {
		return nil, 0, err
REDACTED
	return proxies, result.Total, nil
REDACTED

func (s *adminServiceImpl) ListProxiesWithAccountCount(ctx context.Context, page, pageSize int, protocol, status, search string) ([]ProxyWithAccountCount, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	proxies, result, err := s.proxyRepo.ListWithFiltersAndAccountCount(ctx, params, protocol, status, search)
	if err != nil {
		return nil, 0, err
REDACTED
	return proxies, result.Total, nil
REDACTED

func (s *adminServiceImpl) GetAllProxies(ctx context.Context) ([]Proxy, error) {
	return s.proxyRepo.ListActive(ctx)
REDACTED

func (s *adminServiceImpl) GetAllProxiesWithAccountCount(ctx context.Context) ([]ProxyWithAccountCount, error) {
	return s.proxyRepo.ListActiveWithAccountCount(ctx)
REDACTED

func (s *adminServiceImpl) GetProxy(ctx context.Context, id int64) (*Proxy, error) {
	return s.proxyRepo.GetByID(ctx, id)
REDACTED

func (s *adminServiceImpl) CreateProxy(ctx context.Context, input *CreateProxyInput) (*Proxy, error) {
	proxy := &Proxy{
		Name:     input.Name,
		Protocol: input.Protocol,
		Host:     input.Host,
		Port:     input.Port,
		Username: input.Username,
		Password: input.Password,
		Status:   StatusActive,
REDACTED
	if err := s.proxyRepo.Create(ctx, proxy); err != nil {
		return nil, err
REDACTED
	return proxy, nil
REDACTED

func (s *adminServiceImpl) UpdateProxy(ctx context.Context, id int64, input *UpdateProxyInput) (*Proxy, error) {
	proxy, err := s.proxyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED

	if input.Name != "" {
		proxy.Name = input.Name
REDACTED
	if input.Protocol != "" {
		proxy.Protocol = input.Protocol
REDACTED
	if input.Host != "" {
		proxy.Host = input.Host
REDACTED
	if input.Port != 0 {
		proxy.Port = input.Port
REDACTED
	if input.Username != "" {
		proxy.Username = input.Username
REDACTED
	if input.Password != "" {
		proxy.Password = input.Password
REDACTED
	if input.Status != "" {
		proxy.Status = input.Status
REDACTED

	if err := s.proxyRepo.Update(ctx, proxy); err != nil {
		return nil, err
REDACTED
	return proxy, nil
REDACTED

func (s *adminServiceImpl) DeleteProxy(ctx context.Context, id int64) error {
	return s.proxyRepo.Delete(ctx, id)
REDACTED

func (s *adminServiceImpl) GetProxyAccounts(ctx context.Context, proxyID int64, page, pageSize int) ([]Account, int64, error) {
	// Return mock data for now - would need a dedicated repository method
	return []Account{REDACTED, 0, nil
REDACTED

func (s *adminServiceImpl) CheckProxyExists(ctx context.Context, host string, port int, username, password string) (bool, error) {
	return s.proxyRepo.ExistsByHostPortAuth(ctx, host, port, username, password)
REDACTED

// Redeem code management implementations
func (s *adminServiceImpl) ListRedeemCodes(ctx context.Context, page, pageSize int, codeType, status, search string) ([]RedeemCode, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	codes, result, err := s.redeemCodeRepo.ListWithFilters(ctx, params, codeType, status, search)
	if err != nil {
		return nil, 0, err
REDACTED
	return codes, result.Total, nil
REDACTED

func (s *adminServiceImpl) GetRedeemCode(ctx context.Context, id int64) (*RedeemCode, error) {
	return s.redeemCodeRepo.GetByID(ctx, id)
REDACTED

func (s *adminServiceImpl) GenerateRedeemCodes(ctx context.Context, input *GenerateRedeemCodesInput) ([]RedeemCode, error) {
	// 如果是订阅类型，验证必须有 GroupID
	if input.Type == RedeemTypeSubscription {
		if input.GroupID == nil {
			return nil, errors.New("group_id is required for subscription type")
	REDACTED
		// 验证分组存在且为订阅类型
		group, err := s.groupRepo.GetByID(ctx, *input.GroupID)
		if err != nil {
			return nil, fmt.Errorf("group not found: %w", err)
	REDACTED
		if !group.IsSubscriptionType() {
			return nil, errors.New("group must be subscription type")
	REDACTED
REDACTED

	codes := make([]RedeemCode, 0, input.Count)
	for i := 0; i < input.Count; i++ {
		codeValue, err := GenerateRedeemCode()
		if err != nil {
			return nil, err
	REDACTED
		code := RedeemCode{
			Code:   codeValue,
			Type:   input.Type,
			Value:  input.Value,
			Status: StatusUnused,
	REDACTED
		// 订阅类型专用字段
		if input.Type == RedeemTypeSubscription {
			code.GroupID = input.GroupID
			code.ValidityDays = input.ValidityDays
			if code.ValidityDays <= 0 {
				code.ValidityDays = 30 // 默认30天
		REDACTED
	REDACTED
		if err := s.redeemCodeRepo.Create(ctx, &code); err != nil {
			return nil, err
	REDACTED
		codes = append(codes, code)
REDACTED
	return codes, nil
REDACTED

func (s *adminServiceImpl) DeleteRedeemCode(ctx context.Context, id int64) error {
	return s.redeemCodeRepo.Delete(ctx, id)
REDACTED

func (s *adminServiceImpl) BatchDeleteRedeemCodes(ctx context.Context, ids []int64) (int64, error) {
	var deleted int64
	for _, id := range ids {
		if err := s.redeemCodeRepo.Delete(ctx, id); err == nil {
			deleted++
	REDACTED
REDACTED
	return deleted, nil
REDACTED

func (s *adminServiceImpl) ExpireRedeemCode(ctx context.Context, id int64) (*RedeemCode, error) {
	code, err := s.redeemCodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED
	code.Status = StatusExpired
	if err := s.redeemCodeRepo.Update(ctx, code); err != nil {
		return nil, err
REDACTED
	return code, nil
REDACTED

func (s *adminServiceImpl) TestProxy(ctx context.Context, id int64) (*ProxyTestResult, error) {
	proxy, err := s.proxyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED

	proxyURL := proxy.URL()
	exitInfo, latencyMs, err := s.proxyProber.ProbeProxy(ctx, proxyURL)
	if err != nil {
		return &ProxyTestResult{
			Success: false,
			Message: err.Error(),
	REDACTED, nil
REDACTED

	return &ProxyTestResult{
		Success:   true,
		Message:   "Proxy is accessible",
		LatencyMs: latencyMs,
		IPAddress: exitInfo.IP,
		City:      exitInfo.City,
		Region:    exitInfo.Region,
		Country:   exitInfo.Country,
REDACTED, nil
REDACTED

// checkMixedChannelRisk 检查分组中是否存在混合渠道（Antigravity + Anthropic）
// 如果存在混合，返回错误提示用户确认
func (s *adminServiceImpl) checkMixedChannelRisk(ctx context.Context, currentAccountID int64, currentAccountPlatform string, groupIDs []int64) error {
	// 判断当前账号的渠道类型（基于 platform 字段，而不是 type 字段）
	currentPlatform := getAccountPlatform(currentAccountPlatform)
	if currentPlatform == "" {
		// 不是 Antigravity 或 Anthropic，无需检查
		return nil
REDACTED

	// 检查每个分组中的其他账号
	for _, groupID := range groupIDs {
		accounts, err := s.accountRepo.ListByGroup(ctx, groupID)
		if err != nil {
			return fmt.Errorf("get accounts in group %d: %w", groupID, err)
	REDACTED

		// 检查是否存在不同渠道的账号
		for _, account := range accounts {
			if currentAccountID > 0 && account.ID == currentAccountID {
				continue // 跳过当前账号
		REDACTED

			otherPlatform := getAccountPlatform(account.Platform)
			if otherPlatform == "" {
				continue // 不是 Antigravity 或 Anthropic，跳过
		REDACTED

			// 检测混合渠道
			if currentPlatform != otherPlatform {
				group, _ := s.groupRepo.GetByID(ctx, groupID)
				groupName := fmt.Sprintf("Group %d", groupID)
				if group != nil {
					groupName = group.Name
			REDACTED

				return &MixedChannelError{
					GroupID:         groupID,
					GroupName:       groupName,
					CurrentPlatform: currentPlatform,
					OtherPlatform:   otherPlatform,
			REDACTED
		REDACTED
	REDACTED
REDACTED

	return nil
REDACTED

// getAccountPlatform 根据账号 platform 判断混合渠道检查用的平台标识
func getAccountPlatform(accountPlatform string) string {
	switch strings.ToLower(strings.TrimSpace(accountPlatform)) {
	case PlatformAntigravity:
		return "Antigravity"
	case PlatformAnthropic, "claude":
		return "Anthropic"
	default:
		return ""
REDACTED
REDACTED

// MixedChannelError 混合渠道错误
type MixedChannelError struct {
	GroupID         int64
	GroupName       string
	CurrentPlatform string
	OtherPlatform   string
REDACTED

func (e *MixedChannelError) Error() string {
	return fmt.Sprintf("mixed_channel_warning: Group '%s' contains both %s and %s accounts. Using mixed channels in the same context may cause thinking block signature validation issues, which will fallback to non-thinking mode for historical messages.",
		e.GroupName, e.CurrentPlatform, e.OtherPlatform)
REDACTED

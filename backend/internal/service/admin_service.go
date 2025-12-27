package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// AdminService interface defines admin management operations
type AdminService interface {
	// User management
	ListUsers(ctx context.Context, page, pageSize int, status, role, search string) ([]User, int64, error)
	GetUser(ctx context.Context, id int64) (*User, error)
	CreateUser(ctx context.Context, input *CreateUserInput) (*User, error)
	UpdateUser(ctx context.Context, id int64, input *UpdateUserInput) (*User, error)
	DeleteUser(ctx context.Context, id int64) error
	UpdateUserBalance(ctx context.Context, userID int64, balance float64, operation string, notes string) (*User, error)
	GetUserAPIKeys(ctx context.Context, userID int64, page, pageSize int) ([]ApiKey, int64, error)
	GetUserUsageStats(ctx context.Context, userID int64, period string) (any, error)

	// Group management
	ListGroups(ctx context.Context, page, pageSize int, platform, status string, isExclusive *bool) ([]Group, int64, error)
	GetAllGroups(ctx context.Context) ([]Group, error)
	GetAllGroupsByPlatform(ctx context.Context, platform string) ([]Group, error)
	GetGroup(ctx context.Context, id int64) (*Group, error)
	CreateGroup(ctx context.Context, input *CreateGroupInput) (*Group, error)
	UpdateGroup(ctx context.Context, id int64, input *UpdateGroupInput) (*Group, error)
	DeleteGroup(ctx context.Context, id int64) error
	GetGroupAPIKeys(ctx context.Context, groupID int64, page, pageSize int) ([]ApiKey, int64, error)

	// Account management
	ListAccounts(ctx context.Context, page, pageSize int, platform, accountType, status, search string) ([]Account, int64, error)
	GetAccount(ctx context.Context, id int64) (*Account, error)
	CreateAccount(ctx context.Context, input *CreateAccountInput) (*Account, error)
	UpdateAccount(ctx context.Context, id int64, input *UpdateAccountInput) (*Account, error)
	DeleteAccount(ctx context.Context, id int64) error
	RefreshAccountCredentials(ctx context.Context, id int64) (*Account, error)
	ClearAccountError(ctx context.Context, id int64) (*Account, error)
	SetAccountSchedulable(ctx context.Context, id int64, schedulable bool) (*Account, error)
	BulkUpdateAccounts(ctx context.Context, input *BulkUpdateAccountsInput) (*BulkUpdateAccountsResult, error)

	// Proxy management
	ListProxies(ctx context.Context, page, pageSize int, protocol, status, search string) ([]Proxy, int64, error)
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
}

// Input types for admin operations
type CreateUserInput struct {
	Email         string
	Password      string
	Username      string
	Wechat        string
	Notes         string
	Balance       float64
	Concurrency   int
	AllowedGroups []int64
}

type UpdateUserInput struct {
	Email         string
	Password      string
	Username      *string
	Wechat        *string
	Notes         *string
	Balance       *float64 // 使用指针区分"未提供"和"设置为0"
	Concurrency   *int     // 使用指针区分"未提供"和"设置为0"
	Status        string
	AllowedGroups *[]int64 // 使用指针区分"未提供"和"设置为空数组"
}

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
}

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
}

type CreateAccountInput struct {
	Name        string
	Platform    string
	Type        string
	Credentials map[string]any
	Extra       map[string]any
	ProxyID     *int64
	Concurrency int
	Priority    int
	GroupIDs    []int64
}

type UpdateAccountInput struct {
	Name        string
	Type        string // Account type: oauth, setup-token, apikey
	Credentials map[string]any
	Extra       map[string]any
	ProxyID     *int64
	Concurrency *int // 使用指针区分"未提供"和"设置为0"
	Priority    *int // 使用指针区分"未提供"和"设置为0"
	Status      string
	GroupIDs    *[]int64
}

// BulkUpdateAccountsInput describes the payload for bulk updating accounts.
type BulkUpdateAccountsInput struct {
	AccountIDs  []int64
	Name        string
	ProxyID     *int64
	Concurrency *int
	Priority    *int
	Status      string
	GroupIDs    *[]int64
	Credentials map[string]any
	Extra       map[string]any
}

// BulkUpdateAccountResult captures the result for a single account update.
type BulkUpdateAccountResult struct {
	AccountID int64  `json:"account_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

// BulkUpdateAccountsResult is the aggregated response for bulk updates.
type BulkUpdateAccountsResult struct {
	Success int                       `json:"success"`
	Failed  int                       `json:"failed"`
	Results []BulkUpdateAccountResult `json:"results"`
}

type CreateProxyInput struct {
	Name     string
	Protocol string
	Host     string
	Port     int
	Username string
	Password string
}

type UpdateProxyInput struct {
	Name     string
	Protocol string
	Host     string
	Port     int
	Username string
	Password string
	Status   string
}

type GenerateRedeemCodesInput struct {
	Count        int
	Type         string
	Value        float64
	GroupID      *int64 // 订阅类型专用：关联的分组ID
	ValidityDays int    // 订阅类型专用：有效天数
}

// ProxyTestResult represents the result of testing a proxy
type ProxyTestResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	IPAddress string `json:"ip_address,omitempty"`
	City      string `json:"city,omitempty"`
	Region    string `json:"region,omitempty"`
	Country   string `json:"country,omitempty"`
}

// ProxyExitInfo represents proxy exit information from ipinfo.io
type ProxyExitInfo struct {
	IP      string
	City    string
	Region  string
	Country string
}

// ProxyExitInfoProber tests proxy connectivity and retrieves exit information
type ProxyExitInfoProber interface {
	ProbeProxy(ctx context.Context, proxyURL string) (*ProxyExitInfo, int64, error)
}

// adminServiceImpl implements AdminService
type adminServiceImpl struct {
	userRepo            UserRepository
	groupRepo           GroupRepository
	accountRepo         AccountRepository
	proxyRepo           ProxyRepository
	apiKeyRepo          ApiKeyRepository
	redeemCodeRepo      RedeemCodeRepository
	billingCacheService *BillingCacheService
	proxyProber         ProxyExitInfoProber
}

// NewAdminService creates a new AdminService
func NewAdminService(
	userRepo UserRepository,
	groupRepo GroupRepository,
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	apiKeyRepo ApiKeyRepository,
	redeemCodeRepo RedeemCodeRepository,
	billingCacheService *BillingCacheService,
	proxyProber ProxyExitInfoProber,
) AdminService {
	return &adminServiceImpl{
		userRepo:            userRepo,
		groupRepo:           groupRepo,
		accountRepo:         accountRepo,
		proxyRepo:           proxyRepo,
		apiKeyRepo:          apiKeyRepo,
		redeemCodeRepo:      redeemCodeRepo,
		billingCacheService: billingCacheService,
		proxyProber:         proxyProber,
	}
}

// User management implementations
func (s *adminServiceImpl) ListUsers(ctx context.Context, page, pageSize int, status, role, search string) ([]User, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	users, result, err := s.userRepo.ListWithFilters(ctx, params, status, role, search)
	if err != nil {
		return nil, 0, err
	}
	return users, result.Total, nil
}

func (s *adminServiceImpl) GetUser(ctx context.Context, id int64) (*User, error) {
	return s.userRepo.GetByID(ctx, id)
}

func (s *adminServiceImpl) CreateUser(ctx context.Context, input *CreateUserInput) (*User, error) {
	user := &User{
		Email:         input.Email,
		Username:      input.Username,
		Wechat:        input.Wechat,
		Notes:         input.Notes,
		Role:          RoleUser, // Always create as regular user, never admin
		Balance:       input.Balance,
		Concurrency:   input.Concurrency,
		Status:        StatusActive,
		AllowedGroups: input.AllowedGroups,
	}
	if err := user.SetPassword(input.Password); err != nil {
		return nil, err
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *adminServiceImpl) UpdateUser(ctx context.Context, id int64, input *UpdateUserInput) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Protect admin users: cannot disable admin accounts
	if user.Role == "admin" && input.Status == "disabled" {
		return nil, errors.New("cannot disable admin user")
	}

	oldConcurrency := user.Concurrency

	if input.Email != "" {
		user.Email = input.Email
	}
	if input.Password != "" {
		if err := user.SetPassword(input.Password); err != nil {
			return nil, err
		}
	}

	if input.Username != nil {
		user.Username = *input.Username
	}
	if input.Wechat != nil {
		user.Wechat = *input.Wechat
	}
	if input.Notes != nil {
		user.Notes = *input.Notes
	}

	if input.Status != "" {
		user.Status = input.Status
	}

	if input.Concurrency != nil {
		user.Concurrency = *input.Concurrency
	}

	if input.AllowedGroups != nil {
		user.AllowedGroups = *input.AllowedGroups
	}

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}

	concurrencyDiff := user.Concurrency - oldConcurrency
	if concurrencyDiff != 0 {
		code, err := GenerateRedeemCode()
		if err != nil {
			log.Printf("failed to generate adjustment redeem code: %v", err)
			return user, nil
		}
		adjustmentRecord := &RedeemCode{
			Code:   code,
			Type:   AdjustmentTypeAdminConcurrency,
			Value:  float64(concurrencyDiff),
			Status: StatusUsed,
			UsedBy: &user.ID,
		}
		now := time.Now()
		adjustmentRecord.UsedAt = &now
		if err := s.redeemCodeRepo.Create(ctx, adjustmentRecord); err != nil {
			log.Printf("failed to create concurrency adjustment redeem code: %v", err)
		}
	}

	return user, nil
}

func (s *adminServiceImpl) DeleteUser(ctx context.Context, id int64) error {
	// Protect admin users: cannot delete admin accounts
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if user.Role == "admin" {
		return errors.New("cannot delete admin user")
	}
	return s.userRepo.Delete(ctx, id)
}

func (s *adminServiceImpl) UpdateUserBalance(ctx context.Context, userID int64, balance float64, operation string, notes string) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	oldBalance := user.Balance

	switch operation {
	case "set":
		user.Balance = balance
	case "add":
		user.Balance += balance
	case "subtract":
		user.Balance -= balance
	}

	if user.Balance < 0 {
		return nil, fmt.Errorf("balance cannot be negative, current balance: %.2f, requested operation would result in: %.2f", oldBalance, user.Balance)
	}

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}

	if s.billingCacheService != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.billingCacheService.InvalidateUserBalance(cacheCtx, userID); err != nil {
				log.Printf("invalidate user balance cache failed: user_id=%d err=%v", userID, err)
			}
		}()
	}

	balanceDiff := user.Balance - oldBalance
	if balanceDiff != 0 {
		code, err := GenerateRedeemCode()
		if err != nil {
			log.Printf("failed to generate adjustment redeem code: %v", err)
			return user, nil
		}

		adjustmentRecord := &RedeemCode{
			Code:   code,
			Type:   AdjustmentTypeAdminBalance,
			Value:  balanceDiff,
			Status: StatusUsed,
			UsedBy: &user.ID,
			Notes:  notes,
		}
		now := time.Now()
		adjustmentRecord.UsedAt = &now

		if err := s.redeemCodeRepo.Create(ctx, adjustmentRecord); err != nil {
			log.Printf("failed to create balance adjustment redeem code: %v", err)
		}
	}

	return user, nil
}

func (s *adminServiceImpl) GetUserAPIKeys(ctx context.Context, userID int64, page, pageSize int) ([]ApiKey, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	keys, result, err := s.apiKeyRepo.ListByUserID(ctx, userID, params)
	if err != nil {
		return nil, 0, err
	}
	return keys, result.Total, nil
}

func (s *adminServiceImpl) GetUserUsageStats(ctx context.Context, userID int64, period string) (any, error) {
	// Return mock data for now
	return map[string]any{
		"period":          period,
		"total_requests":  0,
		"total_cost":      0.0,
		"total_tokens":    0,
		"avg_duration_ms": 0,
	}, nil
}

// Group management implementations
func (s *adminServiceImpl) ListGroups(ctx context.Context, page, pageSize int, platform, status string, isExclusive *bool) ([]Group, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	groups, result, err := s.groupRepo.ListWithFilters(ctx, params, platform, status, isExclusive)
	if err != nil {
		return nil, 0, err
	}
	return groups, result.Total, nil
}

func (s *adminServiceImpl) GetAllGroups(ctx context.Context) ([]Group, error) {
	return s.groupRepo.ListActive(ctx)
}

func (s *adminServiceImpl) GetAllGroupsByPlatform(ctx context.Context, platform string) ([]Group, error) {
	return s.groupRepo.ListActiveByPlatform(ctx, platform)
}

func (s *adminServiceImpl) GetGroup(ctx context.Context, id int64) (*Group, error) {
	return s.groupRepo.GetByID(ctx, id)
}

func (s *adminServiceImpl) CreateGroup(ctx context.Context, input *CreateGroupInput) (*Group, error) {
	platform := input.Platform
	if platform == "" {
		platform = PlatformAnthropic
	}

	subscriptionType := input.SubscriptionType
	if subscriptionType == "" {
		subscriptionType = SubscriptionTypeStandard
	}

	group := &Group{
		Name:             input.Name,
		Description:      input.Description,
		Platform:         platform,
		RateMultiplier:   input.RateMultiplier,
		IsExclusive:      input.IsExclusive,
		Status:           StatusActive,
		SubscriptionType: subscriptionType,
		DailyLimitUSD:    input.DailyLimitUSD,
		WeeklyLimitUSD:   input.WeeklyLimitUSD,
		MonthlyLimitUSD:  input.MonthlyLimitUSD,
	}
	if err := s.groupRepo.Create(ctx, group); err != nil {
		return nil, err
	}
	return group, nil
}

func (s *adminServiceImpl) UpdateGroup(ctx context.Context, id int64, input *UpdateGroupInput) (*Group, error) {
	group, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Name != "" {
		group.Name = input.Name
	}
	if input.Description != "" {
		group.Description = input.Description
	}
	if input.Platform != "" {
		group.Platform = input.Platform
	}
	if input.RateMultiplier != nil {
		group.RateMultiplier = *input.RateMultiplier
	}
	if input.IsExclusive != nil {
		group.IsExclusive = *input.IsExclusive
	}
	if input.Status != "" {
		group.Status = input.Status
	}

	// 订阅相关字段
	if input.SubscriptionType != "" {
		group.SubscriptionType = input.SubscriptionType
	}
	// 限额字段支持设置为nil（清除限额）或具体值
	if input.DailyLimitUSD != nil {
		group.DailyLimitUSD = input.DailyLimitUSD
	}
	if input.WeeklyLimitUSD != nil {
		group.WeeklyLimitUSD = input.WeeklyLimitUSD
	}
	if input.MonthlyLimitUSD != nil {
		group.MonthlyLimitUSD = input.MonthlyLimitUSD
	}

	if err := s.groupRepo.Update(ctx, group); err != nil {
		return nil, err
	}
	return group, nil
}

func (s *adminServiceImpl) DeleteGroup(ctx context.Context, id int64) error {
	affectedUserIDs, err := s.groupRepo.DeleteCascade(ctx, id)
	if err != nil {
		return err
	}

	// 事务成功后，异步失效受影响用户的订阅缓存
	if len(affectedUserIDs) > 0 && s.billingCacheService != nil {
		groupID := id
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			for _, userID := range affectedUserIDs {
				if err := s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID); err != nil {
					log.Printf("invalidate subscription cache failed: user_id=%d group_id=%d err=%v", userID, groupID, err)
				}
			}
		}()
	}

	return nil
}

func (s *adminServiceImpl) GetGroupAPIKeys(ctx context.Context, groupID int64, page, pageSize int) ([]ApiKey, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	keys, result, err := s.apiKeyRepo.ListByGroupID(ctx, groupID, params)
	if err != nil {
		return nil, 0, err
	}
	return keys, result.Total, nil
}

// Account management implementations
func (s *adminServiceImpl) ListAccounts(ctx context.Context, page, pageSize int, platform, accountType, status, search string) ([]Account, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	accounts, result, err := s.accountRepo.ListWithFilters(ctx, params, platform, accountType, status, search)
	if err != nil {
		return nil, 0, err
	}
	return accounts, result.Total, nil
}

func (s *adminServiceImpl) GetAccount(ctx context.Context, id int64) (*Account, error) {
	return s.accountRepo.GetByID(ctx, id)
}

func (s *adminServiceImpl) CreateAccount(ctx context.Context, input *CreateAccountInput) (*Account, error) {
	account := &Account{
		Name:        input.Name,
		Platform:    input.Platform,
		Type:        input.Type,
		Credentials: input.Credentials,
		Extra:       input.Extra,
		ProxyID:     input.ProxyID,
		Concurrency: input.Concurrency,
		Priority:    input.Priority,
		Status:      StatusActive,
	}
	if err := s.accountRepo.Create(ctx, account); err != nil {
		return nil, err
	}
	// 绑定分组
	if len(input.GroupIDs) > 0 {
		if err := s.accountRepo.BindGroups(ctx, account.ID, input.GroupIDs); err != nil {
			return nil, err
		}
	}
	return account, nil
}

func (s *adminServiceImpl) UpdateAccount(ctx context.Context, id int64, input *UpdateAccountInput) (*Account, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Name != "" {
		account.Name = input.Name
	}
	if input.Type != "" {
		account.Type = input.Type
	}
	if len(input.Credentials) > 0 {
		account.Credentials = input.Credentials
	}
	if len(input.Extra) > 0 {
		account.Extra = input.Extra
	}
	if input.ProxyID != nil {
		account.ProxyID = input.ProxyID
		account.Proxy = nil // 清除关联对象，防止 GORM Save 时根据 Proxy.ID 覆盖 ProxyID
	}
	// 只在指针非 nil 时更新 Concurrency（支持设置为 0）
	if input.Concurrency != nil {
		account.Concurrency = *input.Concurrency
	}
	// 只在指针非 nil 时更新 Priority（支持设置为 0）
	if input.Priority != nil {
		account.Priority = *input.Priority
	}
	if input.Status != "" {
		account.Status = input.Status
	}

	// 先验证分组是否存在（在任何写操作之前）
	if input.GroupIDs != nil {
		for _, groupID := range *input.GroupIDs {
			if _, err := s.groupRepo.GetByID(ctx, groupID); err != nil {
				return nil, fmt.Errorf("get group: %w", err)
			}
		}
	}

	if err := s.accountRepo.Update(ctx, account); err != nil {
		return nil, err
	}

	// 绑定分组
	if input.GroupIDs != nil {
		if err := s.accountRepo.BindGroups(ctx, account.ID, *input.GroupIDs); err != nil {
			return nil, err
		}
	}

	// 重新查询以确保返回完整数据（包括正确的 Proxy 关联对象）
	return s.accountRepo.GetByID(ctx, id)
}

// BulkUpdateAccounts updates multiple accounts in one request.
// It merges credentials/extra keys instead of overwriting the whole object.
func (s *adminServiceImpl) BulkUpdateAccounts(ctx context.Context, input *BulkUpdateAccountsInput) (*BulkUpdateAccountsResult, error) {
	result := &BulkUpdateAccountsResult{
		Results: make([]BulkUpdateAccountResult, 0, len(input.AccountIDs)),
	}

	if len(input.AccountIDs) == 0 {
		return result, nil
	}

	// Prepare bulk updates for columns and JSONB fields.
	repoUpdates := AccountBulkUpdate{
		Credentials: input.Credentials,
		Extra:       input.Extra,
	}
	if input.Name != "" {
		repoUpdates.Name = &input.Name
	}
	if input.ProxyID != nil {
		repoUpdates.ProxyID = input.ProxyID
	}
	if input.Concurrency != nil {
		repoUpdates.Concurrency = input.Concurrency
	}
	if input.Priority != nil {
		repoUpdates.Priority = input.Priority
	}
	if input.Status != "" {
		repoUpdates.Status = &input.Status
	}

	// Run bulk update for column/jsonb fields first.
	if _, err := s.accountRepo.BulkUpdate(ctx, input.AccountIDs, repoUpdates); err != nil {
		return nil, err
	}

	// Handle group bindings per account (requires individual operations).
	for _, accountID := range input.AccountIDs {
		entry := BulkUpdateAccountResult{AccountID: accountID}

		if input.GroupIDs != nil {
			if err := s.accountRepo.BindGroups(ctx, accountID, *input.GroupIDs); err != nil {
				entry.Success = false
				entry.Error = err.Error()
				result.Failed++
				result.Results = append(result.Results, entry)
				continue
			}
		}

		entry.Success = true
		result.Success++
		result.Results = append(result.Results, entry)
	}

	return result, nil
}

func (s *adminServiceImpl) DeleteAccount(ctx context.Context, id int64) error {
	return s.accountRepo.Delete(ctx, id)
}

func (s *adminServiceImpl) RefreshAccountCredentials(ctx context.Context, id int64) (*Account, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// TODO: Implement refresh logic
	return account, nil
}

func (s *adminServiceImpl) ClearAccountError(ctx context.Context, id int64) (*Account, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	account.Status = StatusActive
	account.ErrorMessage = ""
	if err := s.accountRepo.Update(ctx, account); err != nil {
		return nil, err
	}
	return account, nil
}

func (s *adminServiceImpl) SetAccountSchedulable(ctx context.Context, id int64, schedulable bool) (*Account, error) {
	if err := s.accountRepo.SetSchedulable(ctx, id, schedulable); err != nil {
		return nil, err
	}
	return s.accountRepo.GetByID(ctx, id)
}

// Proxy management implementations
func (s *adminServiceImpl) ListProxies(ctx context.Context, page, pageSize int, protocol, status, search string) ([]Proxy, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	proxies, result, err := s.proxyRepo.ListWithFilters(ctx, params, protocol, status, search)
	if err != nil {
		return nil, 0, err
	}
	return proxies, result.Total, nil
}

func (s *adminServiceImpl) GetAllProxies(ctx context.Context) ([]Proxy, error) {
	return s.proxyRepo.ListActive(ctx)
}

func (s *adminServiceImpl) GetAllProxiesWithAccountCount(ctx context.Context) ([]ProxyWithAccountCount, error) {
	return s.proxyRepo.ListActiveWithAccountCount(ctx)
}

func (s *adminServiceImpl) GetProxy(ctx context.Context, id int64) (*Proxy, error) {
	return s.proxyRepo.GetByID(ctx, id)
}

func (s *adminServiceImpl) CreateProxy(ctx context.Context, input *CreateProxyInput) (*Proxy, error) {
	proxy := &Proxy{
		Name:     input.Name,
		Protocol: input.Protocol,
		Host:     input.Host,
		Port:     input.Port,
		Username: input.Username,
		Password: input.Password,
		Status:   StatusActive,
	}
	if err := s.proxyRepo.Create(ctx, proxy); err != nil {
		return nil, err
	}
	return proxy, nil
}

func (s *adminServiceImpl) UpdateProxy(ctx context.Context, id int64, input *UpdateProxyInput) (*Proxy, error) {
	proxy, err := s.proxyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Name != "" {
		proxy.Name = input.Name
	}
	if input.Protocol != "" {
		proxy.Protocol = input.Protocol
	}
	if input.Host != "" {
		proxy.Host = input.Host
	}
	if input.Port != 0 {
		proxy.Port = input.Port
	}
	if input.Username != "" {
		proxy.Username = input.Username
	}
	if input.Password != "" {
		proxy.Password = input.Password
	}
	if input.Status != "" {
		proxy.Status = input.Status
	}

	if err := s.proxyRepo.Update(ctx, proxy); err != nil {
		return nil, err
	}
	return proxy, nil
}

func (s *adminServiceImpl) DeleteProxy(ctx context.Context, id int64) error {
	return s.proxyRepo.Delete(ctx, id)
}

func (s *adminServiceImpl) GetProxyAccounts(ctx context.Context, proxyID int64, page, pageSize int) ([]Account, int64, error) {
	// Return mock data for now - would need a dedicated repository method
	return []Account{}, 0, nil
}

func (s *adminServiceImpl) CheckProxyExists(ctx context.Context, host string, port int, username, password string) (bool, error) {
	return s.proxyRepo.ExistsByHostPortAuth(ctx, host, port, username, password)
}

// Redeem code management implementations
func (s *adminServiceImpl) ListRedeemCodes(ctx context.Context, page, pageSize int, codeType, status, search string) ([]RedeemCode, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	codes, result, err := s.redeemCodeRepo.ListWithFilters(ctx, params, codeType, status, search)
	if err != nil {
		return nil, 0, err
	}
	return codes, result.Total, nil
}

func (s *adminServiceImpl) GetRedeemCode(ctx context.Context, id int64) (*RedeemCode, error) {
	return s.redeemCodeRepo.GetByID(ctx, id)
}

func (s *adminServiceImpl) GenerateRedeemCodes(ctx context.Context, input *GenerateRedeemCodesInput) ([]RedeemCode, error) {
	// 如果是订阅类型，验证必须有 GroupID
	if input.Type == RedeemTypeSubscription {
		if input.GroupID == nil {
			return nil, errors.New("group_id is required for subscription type")
		}
		// 验证分组存在且为订阅类型
		group, err := s.groupRepo.GetByID(ctx, *input.GroupID)
		if err != nil {
			return nil, fmt.Errorf("group not found: %w", err)
		}
		if !group.IsSubscriptionType() {
			return nil, errors.New("group must be subscription type")
		}
	}

	codes := make([]RedeemCode, 0, input.Count)
	for i := 0; i < input.Count; i++ {
		codeValue, err := GenerateRedeemCode()
		if err != nil {
			return nil, err
		}
		code := RedeemCode{
			Code:   codeValue,
			Type:   input.Type,
			Value:  input.Value,
			Status: StatusUnused,
		}
		// 订阅类型专用字段
		if input.Type == RedeemTypeSubscription {
			code.GroupID = input.GroupID
			code.ValidityDays = input.ValidityDays
			if code.ValidityDays <= 0 {
				code.ValidityDays = 30 // 默认30天
			}
		}
		if err := s.redeemCodeRepo.Create(ctx, &code); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, nil
}

func (s *adminServiceImpl) DeleteRedeemCode(ctx context.Context, id int64) error {
	return s.redeemCodeRepo.Delete(ctx, id)
}

func (s *adminServiceImpl) BatchDeleteRedeemCodes(ctx context.Context, ids []int64) (int64, error) {
	var deleted int64
	for _, id := range ids {
		if err := s.redeemCodeRepo.Delete(ctx, id); err == nil {
			deleted++
		}
	}
	return deleted, nil
}

func (s *adminServiceImpl) ExpireRedeemCode(ctx context.Context, id int64) (*RedeemCode, error) {
	code, err := s.redeemCodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	code.Status = StatusExpired
	if err := s.redeemCodeRepo.Update(ctx, code); err != nil {
		return nil, err
	}
	return code, nil
}

func (s *adminServiceImpl) TestProxy(ctx context.Context, id int64) (*ProxyTestResult, error) {
	proxy, err := s.proxyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	proxyURL := proxy.URL()
	exitInfo, latencyMs, err := s.proxyProber.ProbeProxy(ctx, proxyURL)
	if err != nil {
		return &ProxyTestResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	return &ProxyTestResult{
		Success:   true,
		Message:   "Proxy is accessible",
		LatencyMs: latencyMs,
		IPAddress: exitInfo.IP,
		City:      exitInfo.City,
		Region:    exitInfo.Region,
		Country:   exitInfo.Country,
	}, nil
}

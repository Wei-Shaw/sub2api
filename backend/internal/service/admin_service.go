package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"sub2api/internal/model"
	"sub2api/internal/pkg/pagination"
	"sub2api/internal/service/ports"

	"gorm.io/gorm"
)

// AdminService interface defines admin management operations
type AdminService interface {
	// User management
	ListUsers(ctx context.Context, page, pageSize int, status, role, search string) ([]model.User, int64, error)
	GetUser(ctx context.Context, id int64) (*model.User, error)
	CreateUser(ctx context.Context, input *CreateUserInput) (*model.User, error)
	UpdateUser(ctx context.Context, id int64, input *UpdateUserInput) (*model.User, error)
	DeleteUser(ctx context.Context, id int64) error
	UpdateUserBalance(ctx context.Context, userID int64, balance float64, operation string) (*model.User, error)
	GetUserAPIKeys(ctx context.Context, userID int64, page, pageSize int) ([]model.ApiKey, int64, error)
	GetUserUsageStats(ctx context.Context, userID int64, period string) (interface{}, error)

	// Group management
	ListGroups(ctx context.Context, page, pageSize int, platform, status string, isExclusive *bool) ([]model.Group, int64, error)
	GetAllGroups(ctx context.Context) ([]model.Group, error)
	GetAllGroupsByPlatform(ctx context.Context, platform string) ([]model.Group, error)
	GetGroup(ctx context.Context, id int64) (*model.Group, error)
	CreateGroup(ctx context.Context, input *CreateGroupInput) (*model.Group, error)
	UpdateGroup(ctx context.Context, id int64, input *UpdateGroupInput) (*model.Group, error)
	DeleteGroup(ctx context.Context, id int64) error
	GetGroupAPIKeys(ctx context.Context, groupID int64, page, pageSize int) ([]model.ApiKey, int64, error)

	// Account management
	ListAccounts(ctx context.Context, page, pageSize int, platform, accountType, status, search string) ([]model.Account, int64, error)
	GetAccount(ctx context.Context, id int64) (*model.Account, error)
	CreateAccount(ctx context.Context, input *CreateAccountInput) (*model.Account, error)
	UpdateAccount(ctx context.Context, id int64, input *UpdateAccountInput) (*model.Account, error)
	DeleteAccount(ctx context.Context, id int64) error
	RefreshAccountCredentials(ctx context.Context, id int64) (*model.Account, error)
	ClearAccountError(ctx context.Context, id int64) (*model.Account, error)
	SetAccountSchedulable(ctx context.Context, id int64, schedulable bool) (*model.Account, error)

	// Proxy management
	ListProxies(ctx context.Context, page, pageSize int, protocol, status, search string) ([]model.Proxy, int64, error)
	GetAllProxies(ctx context.Context) ([]model.Proxy, error)
	GetAllProxiesWithAccountCount(ctx context.Context) ([]model.ProxyWithAccountCount, error)
	GetProxy(ctx context.Context, id int64) (*model.Proxy, error)
	CreateProxy(ctx context.Context, input *CreateProxyInput) (*model.Proxy, error)
	UpdateProxy(ctx context.Context, id int64, input *UpdateProxyInput) (*model.Proxy, error)
	DeleteProxy(ctx context.Context, id int64) error
	GetProxyAccounts(ctx context.Context, proxyID int64, page, pageSize int) ([]model.Account, int64, error)
	CheckProxyExists(ctx context.Context, host string, port int, username, password string) (bool, error)
	TestProxy(ctx context.Context, id int64) (*ProxyTestResult, error)

	// Redeem code management
	ListRedeemCodes(ctx context.Context, page, pageSize int, codeType, status, search string) ([]model.RedeemCode, int64, error)
	GetRedeemCode(ctx context.Context, id int64) (*model.RedeemCode, error)
	GenerateRedeemCodes(ctx context.Context, input *GenerateRedeemCodesInput) ([]model.RedeemCode, error)
	DeleteRedeemCode(ctx context.Context, id int64) error
	BatchDeleteRedeemCodes(ctx context.Context, ids []int64) (int64, error)
	ExpireRedeemCode(ctx context.Context, id int64) (*model.RedeemCode, error)
}

// Input types for admin operations
type CreateUserInput struct {
	Email         string
	Password      string
	Balance       float64
	Concurrency   int
	AllowedGroups []int64
}

type UpdateUserInput struct {
	Email         string
	Password      string
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
	Credentials map[string]interface{}
	Extra       map[string]interface{}
	ProxyID     *int64
	Concurrency int
	Priority    int
	GroupIDs    []int64
}

type UpdateAccountInput struct {
	Name        string
	Type        string // Account type: oauth, setup-token, apikey
	Credentials map[string]interface{}
	Extra       map[string]interface{}
	ProxyID     *int64
	Concurrency *int // 使用指针区分"未提供"和"设置为0"
	Priority    *int // 使用指针区分"未提供"和"设置为0"
	Status      string
	GroupIDs    *[]int64
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
	userRepo            ports.UserRepository
	groupRepo           ports.GroupRepository
	accountRepo         ports.AccountRepository
	proxyRepo           ports.ProxyRepository
	apiKeyRepo          ports.ApiKeyRepository
	redeemCodeRepo      ports.RedeemCodeRepository
	usageLogRepo        ports.UsageLogRepository
	userSubRepo         ports.UserSubscriptionRepository
	billingCacheService *BillingCacheService
	proxyProber         ProxyExitInfoProber
}

// NewAdminService creates a new AdminService
func NewAdminService(
	userRepo ports.UserRepository,
	groupRepo ports.GroupRepository,
	accountRepo ports.AccountRepository,
	proxyRepo ports.ProxyRepository,
	apiKeyRepo ports.ApiKeyRepository,
	redeemCodeRepo ports.RedeemCodeRepository,
	usageLogRepo ports.UsageLogRepository,
	userSubRepo ports.UserSubscriptionRepository,
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
		usageLogRepo:        usageLogRepo,
		userSubRepo:         userSubRepo,
		billingCacheService: billingCacheService,
		proxyProber:         proxyProber,
	}
}

// User management implementations
func (s *adminServiceImpl) ListUsers(ctx context.Context, page, pageSize int, status, role, search string) ([]model.User, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	users, result, err := s.userRepo.ListWithFilters(ctx, params, status, role, search)
	if err != nil {
		return nil, 0, err
	}
	return users, result.Total, nil
}

func (s *adminServiceImpl) GetUser(ctx context.Context, id int64) (*model.User, error) {
	return s.userRepo.GetByID(ctx, id)
}

func (s *adminServiceImpl) CreateUser(ctx context.Context, input *CreateUserInput) (*model.User, error) {
	user := &model.User{
		Email:       input.Email,
		Role:        "user", // Always create as regular user, never admin
		Balance:     input.Balance,
		Concurrency: input.Concurrency,
		Status:      model.StatusActive,
	}
	if err := user.SetPassword(input.Password); err != nil {
		return nil, err
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *adminServiceImpl) UpdateUser(ctx context.Context, id int64, input *UpdateUserInput) (*model.User, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Protect admin users: cannot disable admin accounts
	if user.Role == "admin" && input.Status == "disabled" {
		return nil, errors.New("cannot disable admin user")
	}

	// Track balance and concurrency changes for logging
	oldBalance := user.Balance
	oldConcurrency := user.Concurrency

	if input.Email != "" {
		user.Email = input.Email
	}
	if input.Password != "" {
		if err := user.SetPassword(input.Password); err != nil {
			return nil, err
		}
	}
	// Role is not allowed to be changed via API to prevent privilege escalation
	if input.Status != "" {
		user.Status = input.Status
	}

	// 只在指针非 nil 时更新 Balance（支持设置为 0）
	if input.Balance != nil {
		user.Balance = *input.Balance
	}

	// 只在指针非 nil 时更新 Concurrency（支持设置为任意值）
	if input.Concurrency != nil {
		user.Concurrency = *input.Concurrency
	}

	// 只在指针非 nil 时更新 AllowedGroups
	if input.AllowedGroups != nil {
		user.AllowedGroups = *input.AllowedGroups
	}

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}

	// 余额变化时失效缓存
	if input.Balance != nil && *input.Balance != oldBalance {
		if s.billingCacheService != nil {
			go func() {
				cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.billingCacheService.InvalidateUserBalance(cacheCtx, id); err != nil {
					log.Printf("invalidate user balance cache failed: user_id=%d err=%v", id, err)
				}
			}()
		}
	}

	// Create adjustment records for balance/concurrency changes
	balanceDiff := user.Balance - oldBalance
	if balanceDiff != 0 {
		code, err := model.GenerateRedeemCode()
		if err != nil {
			log.Printf("failed to generate adjustment redeem code: %v", err)
			return user, nil
		}
		adjustmentRecord := &model.RedeemCode{
			Code:   code,
			Type:   model.AdjustmentTypeAdminBalance,
			Value:  balanceDiff,
			Status: model.StatusUsed,
			UsedBy: &user.ID,
		}
		now := time.Now()
		adjustmentRecord.UsedAt = &now
		if err := s.redeemCodeRepo.Create(ctx, adjustmentRecord); err != nil {
			log.Printf("failed to create balance adjustment redeem code: %v", err)
		}
	}

	concurrencyDiff := user.Concurrency - oldConcurrency
	if concurrencyDiff != 0 {
		code, err := model.GenerateRedeemCode()
		if err != nil {
			log.Printf("failed to generate adjustment redeem code: %v", err)
			return user, nil
		}
		adjustmentRecord := &model.RedeemCode{
			Code:   code,
			Type:   model.AdjustmentTypeAdminConcurrency,
			Value:  float64(concurrencyDiff),
			Status: model.StatusUsed,
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

func (s *adminServiceImpl) UpdateUserBalance(ctx context.Context, userID int64, balance float64, operation string) (*model.User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	switch operation {
	case "set":
		user.Balance = balance
	case "add":
		user.Balance += balance
	case "subtract":
		user.Balance -= balance
	}

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}

	// 失效余额缓存
	if s.billingCacheService != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.billingCacheService.InvalidateUserBalance(cacheCtx, userID); err != nil {
				log.Printf("invalidate user balance cache failed: user_id=%d err=%v", userID, err)
			}
		}()
	}

	return user, nil
}

func (s *adminServiceImpl) GetUserAPIKeys(ctx context.Context, userID int64, page, pageSize int) ([]model.ApiKey, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	keys, result, err := s.apiKeyRepo.ListByUserID(ctx, userID, params)
	if err != nil {
		return nil, 0, err
	}
	return keys, result.Total, nil
}

func (s *adminServiceImpl) GetUserUsageStats(ctx context.Context, userID int64, period string) (interface{}, error) {
	// Return mock data for now
	return map[string]interface{}{
		"period":          period,
		"total_requests":  0,
		"total_cost":      0.0,
		"total_tokens":    0,
		"avg_duration_ms": 0,
	}, nil
}

// Group management implementations
func (s *adminServiceImpl) ListGroups(ctx context.Context, page, pageSize int, platform, status string, isExclusive *bool) ([]model.Group, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	groups, result, err := s.groupRepo.ListWithFilters(ctx, params, platform, status, isExclusive)
	if err != nil {
		return nil, 0, err
	}
	return groups, result.Total, nil
}

func (s *adminServiceImpl) GetAllGroups(ctx context.Context) ([]model.Group, error) {
	return s.groupRepo.ListActive(ctx)
}

func (s *adminServiceImpl) GetAllGroupsByPlatform(ctx context.Context, platform string) ([]model.Group, error) {
	return s.groupRepo.ListActiveByPlatform(ctx, platform)
}

func (s *adminServiceImpl) GetGroup(ctx context.Context, id int64) (*model.Group, error) {
	return s.groupRepo.GetByID(ctx, id)
}

func (s *adminServiceImpl) CreateGroup(ctx context.Context, input *CreateGroupInput) (*model.Group, error) {
	platform := input.Platform
	if platform == "" {
		platform = model.PlatformAnthropic
	}

	subscriptionType := input.SubscriptionType
	if subscriptionType == "" {
		subscriptionType = model.SubscriptionTypeStandard
	}

	group := &model.Group{
		Name:             input.Name,
		Description:      input.Description,
		Platform:         platform,
		RateMultiplier:   input.RateMultiplier,
		IsExclusive:      input.IsExclusive,
		Status:           model.StatusActive,
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

func (s *adminServiceImpl) UpdateGroup(ctx context.Context, id int64, input *UpdateGroupInput) (*model.Group, error) {
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
	// 先获取分组信息，检查是否存在
	group, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("group not found: %w", err)
	}

	// 订阅类型分组：先获取受影响的用户ID列表（用于事务后失效缓存）
	var affectedUserIDs []int64
	if group.IsSubscriptionType() && s.billingCacheService != nil {
		var subscriptions []model.UserSubscription
		if err := s.groupRepo.DB().WithContext(ctx).
			Where("group_id = ?", id).
			Select("user_id").
			Find(&subscriptions).Error; err == nil {
			for _, sub := range subscriptions {
				affectedUserIDs = append(affectedUserIDs, sub.UserID)
			}
		}
	}

	// 使用事务处理所有级联删除
	db := s.groupRepo.DB()
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 如果是订阅类型分组，删除 user_subscriptions 中的相关记录
		if group.IsSubscriptionType() {
			if err := tx.Where("group_id = ?", id).Delete(&model.UserSubscription{}).Error; err != nil {
				return fmt.Errorf("delete user subscriptions: %w", err)
			}
		}

		// 2. 将 api_keys 中绑定该分组的 group_id 设为 nil（任何类型的分组都需要）
		if err := tx.Model(&model.ApiKey{}).Where("group_id = ?", id).Update("group_id", nil).Error; err != nil {
			return fmt.Errorf("clear api key group_id: %w", err)
		}

		// 3. 从 users.allowed_groups 数组中移除该分组 ID
		if err := tx.Model(&model.User{}).
			Where("? = ANY(allowed_groups)", id).
			Update("allowed_groups", gorm.Expr("array_remove(allowed_groups, ?)", id)).Error; err != nil {
			return fmt.Errorf("remove from allowed_groups: %w", err)
		}

		// 4. 删除 account_groups 中间表的数据
		if err := tx.Where("group_id = ?", id).Delete(&model.AccountGroup{}).Error; err != nil {
			return fmt.Errorf("delete account groups: %w", err)
		}

		// 5. 删除分组本身
		if err := tx.Delete(&model.Group{}, id).Error; err != nil {
			return fmt.Errorf("delete group: %w", err)
		}

		return nil
	})

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

func (s *adminServiceImpl) GetGroupAPIKeys(ctx context.Context, groupID int64, page, pageSize int) ([]model.ApiKey, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	keys, result, err := s.apiKeyRepo.ListByGroupID(ctx, groupID, params)
	if err != nil {
		return nil, 0, err
	}
	return keys, result.Total, nil
}

// Account management implementations
func (s *adminServiceImpl) ListAccounts(ctx context.Context, page, pageSize int, platform, accountType, status, search string) ([]model.Account, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	accounts, result, err := s.accountRepo.ListWithFilters(ctx, params, platform, accountType, status, search)
	if err != nil {
		return nil, 0, err
	}
	return accounts, result.Total, nil
}

func (s *adminServiceImpl) GetAccount(ctx context.Context, id int64) (*model.Account, error) {
	return s.accountRepo.GetByID(ctx, id)
}

func (s *adminServiceImpl) CreateAccount(ctx context.Context, input *CreateAccountInput) (*model.Account, error) {
	account := &model.Account{
		Name:        input.Name,
		Platform:    input.Platform,
		Type:        input.Type,
		Credentials: model.JSONB(input.Credentials),
		Extra:       model.JSONB(input.Extra),
		ProxyID:     input.ProxyID,
		Concurrency: input.Concurrency,
		Priority:    input.Priority,
		Status:      model.StatusActive,
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

func (s *adminServiceImpl) UpdateAccount(ctx context.Context, id int64, input *UpdateAccountInput) (*model.Account, error) {
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
		account.Credentials = model.JSONB(input.Credentials)
	}
	if len(input.Extra) > 0 {
		account.Extra = model.JSONB(input.Extra)
	}
	if input.ProxyID != nil {
		account.ProxyID = input.ProxyID
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

	if err := s.accountRepo.Update(ctx, account); err != nil {
		return nil, err
	}

	// 更新分组绑定
	if input.GroupIDs != nil {
		if err := s.accountRepo.BindGroups(ctx, account.ID, *input.GroupIDs); err != nil {
			return nil, err
		}
	}

	return account, nil
}

func (s *adminServiceImpl) DeleteAccount(ctx context.Context, id int64) error {
	return s.accountRepo.Delete(ctx, id)
}

func (s *adminServiceImpl) RefreshAccountCredentials(ctx context.Context, id int64) (*model.Account, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// TODO: Implement refresh logic
	return account, nil
}

func (s *adminServiceImpl) ClearAccountError(ctx context.Context, id int64) (*model.Account, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	account.Status = model.StatusActive
	account.ErrorMessage = ""
	if err := s.accountRepo.Update(ctx, account); err != nil {
		return nil, err
	}
	return account, nil
}

func (s *adminServiceImpl) SetAccountSchedulable(ctx context.Context, id int64, schedulable bool) (*model.Account, error) {
	if err := s.accountRepo.SetSchedulable(ctx, id, schedulable); err != nil {
		return nil, err
	}
	return s.accountRepo.GetByID(ctx, id)
}

// Proxy management implementations
func (s *adminServiceImpl) ListProxies(ctx context.Context, page, pageSize int, protocol, status, search string) ([]model.Proxy, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	proxies, result, err := s.proxyRepo.ListWithFilters(ctx, params, protocol, status, search)
	if err != nil {
		return nil, 0, err
	}
	return proxies, result.Total, nil
}

func (s *adminServiceImpl) GetAllProxies(ctx context.Context) ([]model.Proxy, error) {
	return s.proxyRepo.ListActive(ctx)
}

func (s *adminServiceImpl) GetAllProxiesWithAccountCount(ctx context.Context) ([]model.ProxyWithAccountCount, error) {
	return s.proxyRepo.ListActiveWithAccountCount(ctx)
}

func (s *adminServiceImpl) GetProxy(ctx context.Context, id int64) (*model.Proxy, error) {
	return s.proxyRepo.GetByID(ctx, id)
}

func (s *adminServiceImpl) CreateProxy(ctx context.Context, input *CreateProxyInput) (*model.Proxy, error) {
	proxy := &model.Proxy{
		Name:     input.Name,
		Protocol: input.Protocol,
		Host:     input.Host,
		Port:     input.Port,
		Username: input.Username,
		Password: input.Password,
		Status:   model.StatusActive,
	}
	if err := s.proxyRepo.Create(ctx, proxy); err != nil {
		return nil, err
	}
	return proxy, nil
}

func (s *adminServiceImpl) UpdateProxy(ctx context.Context, id int64, input *UpdateProxyInput) (*model.Proxy, error) {
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

func (s *adminServiceImpl) GetProxyAccounts(ctx context.Context, proxyID int64, page, pageSize int) ([]model.Account, int64, error) {
	// Return mock data for now - would need a dedicated repository method
	return []model.Account{}, 0, nil
}

func (s *adminServiceImpl) CheckProxyExists(ctx context.Context, host string, port int, username, password string) (bool, error) {
	return s.proxyRepo.ExistsByHostPortAuth(ctx, host, port, username, password)
}

// Redeem code management implementations
func (s *adminServiceImpl) ListRedeemCodes(ctx context.Context, page, pageSize int, codeType, status, search string) ([]model.RedeemCode, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	codes, result, err := s.redeemCodeRepo.ListWithFilters(ctx, params, codeType, status, search)
	if err != nil {
		return nil, 0, err
	}
	return codes, result.Total, nil
}

func (s *adminServiceImpl) GetRedeemCode(ctx context.Context, id int64) (*model.RedeemCode, error) {
	return s.redeemCodeRepo.GetByID(ctx, id)
}

func (s *adminServiceImpl) GenerateRedeemCodes(ctx context.Context, input *GenerateRedeemCodesInput) ([]model.RedeemCode, error) {
	// 如果是订阅类型，验证必须有 GroupID
	if input.Type == model.RedeemTypeSubscription {
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

	codes := make([]model.RedeemCode, 0, input.Count)
	for i := 0; i < input.Count; i++ {
		codeValue, err := model.GenerateRedeemCode()
		if err != nil {
			return nil, err
		}
		code := model.RedeemCode{
			Code:   codeValue,
			Type:   input.Type,
			Value:  input.Value,
			Status: model.StatusUnused,
		}
		// 订阅类型专用字段
		if input.Type == model.RedeemTypeSubscription {
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

func (s *adminServiceImpl) ExpireRedeemCode(ctx context.Context, id int64) (*model.RedeemCode, error) {
	code, err := s.redeemCodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	code.Status = model.StatusExpired
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

package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// AdminService interface defines admin management operations
type AdminService interface {
	// User management
	ListUsers(ctx context.Context, page, pageSize int, status, role, search string) ([]model.User, int64, error)
	GetUser(ctx context.Context, id int64) (*model.User, error)
	CreateUser(ctx context.Context, input *CreateUserInput) (*model.User, error)
	UpdateUser(ctx context.Context, id int64, input *UpdateUserInput) (*model.User, error)
	DeleteUser(ctx context.Context, id int64) error
	UpdateUserBalance(ctx context.Context, userID int64, balance float64, operation string, notes string) (*model.User, error)
	GetUserAPIKeys(ctx context.Context, userID int64, page, pageSize int) ([]model.ApiKey, int64, error)
	GetUserUsageStats(ctx context.Context, userID int64, period string) (any, error)

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
	BulkUpdateAccounts(ctx context.Context, input *BulkUpdateAccountsInput) (*BulkUpdateAccountsResult, error)

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
REDACTED

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
REDACTED

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
REDACTED

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
REDACTED

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
REDACTED

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
REDACTED

// BulkUpdateAccountResult captures the result for a single account update.
type BulkUpdateAccountResult struct {
	AccountID int64  `json:"account_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
REDACTED

// BulkUpdateAccountsResult is the aggregated response for bulk updates.
type BulkUpdateAccountsResult struct {
	Success int                       `json:"success"`
	Failed  int                       `json:"failed"`
	Results []BulkUpdateAccountResult `json:"results"`
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
	userRepo            UserRepository
	groupRepo           GroupRepository
	accountRepo         AccountRepository
	proxyRepo           ProxyRepository
	apiKeyRepo          ApiKeyRepository
	redeemCodeRepo      RedeemCodeRepository
	billingCacheService *BillingCacheService
	proxyProber         ProxyExitInfoProber
REDACTED

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
REDACTED
REDACTED

// User management implementations
func (s *adminServiceImpl) ListUsers(ctx context.Context, page, pageSize int, status, role, search string) ([]model.User, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	users, result, err := s.userRepo.ListWithFilters(ctx, params, status, role, search)
	if err != nil {
		return nil, 0, err
REDACTED
	return users, result.Total, nil
REDACTED

func (s *adminServiceImpl) GetUser(ctx context.Context, id int64) (*model.User, error) {
	return s.userRepo.GetByID(ctx, id)
REDACTED

func (s *adminServiceImpl) CreateUser(ctx context.Context, input *CreateUserInput) (*model.User, error) {
	user := &model.User{
		Email:       input.Email,
		Username:    input.Username,
		Wechat:      input.Wechat,
		Notes:       input.Notes,
		Role:        "user", // Always create as regular user, never admin
		Balance:     input.Balance,
		Concurrency: input.Concurrency,
		Status:      model.StatusActive,
REDACTED
	if err := user.SetPassword(input.Password); err != nil {
		return nil, err
REDACTED
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
REDACTED
	return user, nil
REDACTED

func (s *adminServiceImpl) UpdateUser(ctx context.Context, id int64, input *UpdateUserInput) (*model.User, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED

	// Protect admin users: cannot disable admin accounts
	if user.Role == "admin" && input.Status == "disabled" {
		return nil, errors.New("cannot disable admin user")
REDACTED

	oldConcurrency := user.Concurrency

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
	if input.Wechat != nil {
		user.Wechat = *input.Wechat
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

	concurrencyDiff := user.Concurrency - oldConcurrency
	if concurrencyDiff != 0 {
		code, err := model.GenerateRedeemCode()
		if err != nil {
			log.Printf("failed to generate adjustment redeem code: %v", err)
			return user, nil
	REDACTED
		adjustmentRecord := &model.RedeemCode{
			Code:   code,
			Type:   model.AdjustmentTypeAdminConcurrency,
			Value:  float64(concurrencyDiff),
			Status: model.StatusUsed,
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
	return s.userRepo.Delete(ctx, id)
REDACTED

func (s *adminServiceImpl) UpdateUserBalance(ctx context.Context, userID int64, balance float64, operation string, notes string) (*model.User, error) {
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

	if s.billingCacheService != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.billingCacheService.InvalidateUserBalance(cacheCtx, userID); err != nil {
				log.Printf("invalidate user balance cache failed: user_id=%d err=%v", userID, err)
		REDACTED
	REDACTED()
REDACTED

	balanceDiff := user.Balance - oldBalance
	if balanceDiff != 0 {
		code, err := model.GenerateRedeemCode()
		if err != nil {
			log.Printf("failed to generate adjustment redeem code: %v", err)
			return user, nil
	REDACTED

		adjustmentRecord := &model.RedeemCode{
			Code:   code,
			Type:   model.AdjustmentTypeAdminBalance,
			Value:  balanceDiff,
			Status: model.StatusUsed,
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

func (s *adminServiceImpl) GetUserAPIKeys(ctx context.Context, userID int64, page, pageSize int) ([]model.ApiKey, int64, error) {
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
func (s *adminServiceImpl) ListGroups(ctx context.Context, page, pageSize int, platform, status string, isExclusive *bool) ([]model.Group, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	groups, result, err := s.groupRepo.ListWithFilters(ctx, params, platform, status, isExclusive)
	if err != nil {
		return nil, 0, err
REDACTED
	return groups, result.Total, nil
REDACTED

func (s *adminServiceImpl) GetAllGroups(ctx context.Context) ([]model.Group, error) {
	return s.groupRepo.ListActive(ctx)
REDACTED

func (s *adminServiceImpl) GetAllGroupsByPlatform(ctx context.Context, platform string) ([]model.Group, error) {
	return s.groupRepo.ListActiveByPlatform(ctx, platform)
REDACTED

func (s *adminServiceImpl) GetGroup(ctx context.Context, id int64) (*model.Group, error) {
	return s.groupRepo.GetByID(ctx, id)
REDACTED

func (s *adminServiceImpl) CreateGroup(ctx context.Context, input *CreateGroupInput) (*model.Group, error) {
	platform := input.Platform
	if platform == "" {
		platform = model.PlatformAnthropic
REDACTED

	subscriptionType := input.SubscriptionType
	if subscriptionType == "" {
		subscriptionType = model.SubscriptionTypeStandard
REDACTED

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
REDACTED
	if err := s.groupRepo.Create(ctx, group); err != nil {
		return nil, err
REDACTED
	return group, nil
REDACTED

func (s *adminServiceImpl) UpdateGroup(ctx context.Context, id int64, input *UpdateGroupInput) (*model.Group, error) {
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
	// 限额字段支持设置为nil（清除限额）或具体值
	if input.DailyLimitUSD != nil {
		group.DailyLimitUSD = input.DailyLimitUSD
REDACTED
	if input.WeeklyLimitUSD != nil {
		group.WeeklyLimitUSD = input.WeeklyLimitUSD
REDACTED
	if input.MonthlyLimitUSD != nil {
		group.MonthlyLimitUSD = input.MonthlyLimitUSD
REDACTED

	if err := s.groupRepo.Update(ctx, group); err != nil {
		return nil, err
REDACTED
	return group, nil
REDACTED

func (s *adminServiceImpl) DeleteGroup(ctx context.Context, id int64) error {
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

	return nil
REDACTED

func (s *adminServiceImpl) GetGroupAPIKeys(ctx context.Context, groupID int64, page, pageSize int) ([]model.ApiKey, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	keys, result, err := s.apiKeyRepo.ListByGroupID(ctx, groupID, params)
	if err != nil {
		return nil, 0, err
REDACTED
	return keys, result.Total, nil
REDACTED

// Account management implementations
func (s *adminServiceImpl) ListAccounts(ctx context.Context, page, pageSize int, platform, accountType, status, search string) ([]model.Account, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	accounts, result, err := s.accountRepo.ListWithFilters(ctx, params, platform, accountType, status, search)
	if err != nil {
		return nil, 0, err
REDACTED
	return accounts, result.Total, nil
REDACTED

func (s *adminServiceImpl) GetAccount(ctx context.Context, id int64) (*model.Account, error) {
	return s.accountRepo.GetByID(ctx, id)
REDACTED

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
REDACTED
	if err := s.accountRepo.Create(ctx, account); err != nil {
		return nil, err
REDACTED
	// 绑定分组
	if len(input.GroupIDs) > 0 {
		if err := s.accountRepo.BindGroups(ctx, account.ID, input.GroupIDs); err != nil {
			return nil, err
	REDACTED
REDACTED
	return account, nil
REDACTED

func (s *adminServiceImpl) UpdateAccount(ctx context.Context, id int64, input *UpdateAccountInput) (*model.Account, error) {
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
	if len(input.Credentials) > 0 {
		account.Credentials = model.JSONB(input.Credentials)
REDACTED
	if len(input.Extra) > 0 {
		account.Extra = model.JSONB(input.Extra)
REDACTED
	if input.ProxyID != nil {
		account.ProxyID = input.ProxyID
REDACTED
	// 只在指针非 nil 时更新 Concurrency（支持设置为 0）
	if input.Concurrency != nil {
		account.Concurrency = *input.Concurrency
REDACTED
	// 只在指针非 nil 时更新 Priority（支持设置为 0）
	if input.Priority != nil {
		account.Priority = *input.Priority
REDACTED
	if input.Status != "" {
		account.Status = input.Status
REDACTED

	if err := s.accountRepo.Update(ctx, account); err != nil {
		return nil, err
REDACTED

	// 更新分组绑定
	if input.GroupIDs != nil {
		if err := s.accountRepo.BindGroups(ctx, account.ID, *input.GroupIDs); err != nil {
			return nil, err
	REDACTED
REDACTED

	return account, nil
REDACTED

// BulkUpdateAccounts updates multiple accounts in one request.
// It merges credentials/extra keys instead of overwriting the whole object.
func (s *adminServiceImpl) BulkUpdateAccounts(ctx context.Context, input *BulkUpdateAccountsInput) (*BulkUpdateAccountsResult, error) {
	result := &BulkUpdateAccountsResult{
		Results: make([]BulkUpdateAccountResult, 0, len(input.AccountIDs)),
REDACTED

	if len(input.AccountIDs) == 0 {
		return result, nil
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
	if input.Status != "" {
		repoUpdates.Status = &input.Status
REDACTED

	// Run bulk update for column/jsonb fields first.
	if _, err := s.accountRepo.BulkUpdate(ctx, input.AccountIDs, repoUpdates); err != nil {
		return nil, err
REDACTED

	// Handle group bindings per account (requires individual operations).
	for _, accountID := range input.AccountIDs {
		entry := BulkUpdateAccountResult{AccountID: accountIDREDACTED

		if input.GroupIDs != nil {
			if err := s.accountRepo.BindGroups(ctx, accountID, *input.GroupIDs); err != nil {
				entry.Success = false
				entry.Error = err.Error()
				result.Failed++
				result.Results = append(result.Results, entry)
				continue
		REDACTED
	REDACTED

		entry.Success = true
		result.Success++
		result.Results = append(result.Results, entry)
REDACTED

	return result, nil
REDACTED

func (s *adminServiceImpl) DeleteAccount(ctx context.Context, id int64) error {
	return s.accountRepo.Delete(ctx, id)
REDACTED

func (s *adminServiceImpl) RefreshAccountCredentials(ctx context.Context, id int64) (*model.Account, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED
	// TODO: Implement refresh logic
	return account, nil
REDACTED

func (s *adminServiceImpl) ClearAccountError(ctx context.Context, id int64) (*model.Account, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED
	account.Status = model.StatusActive
	account.ErrorMessage = ""
	if err := s.accountRepo.Update(ctx, account); err != nil {
		return nil, err
REDACTED
	return account, nil
REDACTED

func (s *adminServiceImpl) SetAccountSchedulable(ctx context.Context, id int64, schedulable bool) (*model.Account, error) {
	if err := s.accountRepo.SetSchedulable(ctx, id, schedulable); err != nil {
		return nil, err
REDACTED
	return s.accountRepo.GetByID(ctx, id)
REDACTED

// Proxy management implementations
func (s *adminServiceImpl) ListProxies(ctx context.Context, page, pageSize int, protocol, status, search string) ([]model.Proxy, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	proxies, result, err := s.proxyRepo.ListWithFilters(ctx, params, protocol, status, search)
	if err != nil {
		return nil, 0, err
REDACTED
	return proxies, result.Total, nil
REDACTED

func (s *adminServiceImpl) GetAllProxies(ctx context.Context) ([]model.Proxy, error) {
	return s.proxyRepo.ListActive(ctx)
REDACTED

func (s *adminServiceImpl) GetAllProxiesWithAccountCount(ctx context.Context) ([]model.ProxyWithAccountCount, error) {
	return s.proxyRepo.ListActiveWithAccountCount(ctx)
REDACTED

func (s *adminServiceImpl) GetProxy(ctx context.Context, id int64) (*model.Proxy, error) {
	return s.proxyRepo.GetByID(ctx, id)
REDACTED

func (s *adminServiceImpl) CreateProxy(ctx context.Context, input *CreateProxyInput) (*model.Proxy, error) {
	proxy := &model.Proxy{
		Name:     input.Name,
		Protocol: input.Protocol,
		Host:     input.Host,
		Port:     input.Port,
		Username: input.Username,
		Password: input.Password,
		Status:   model.StatusActive,
REDACTED
	if err := s.proxyRepo.Create(ctx, proxy); err != nil {
		return nil, err
REDACTED
	return proxy, nil
REDACTED

func (s *adminServiceImpl) UpdateProxy(ctx context.Context, id int64, input *UpdateProxyInput) (*model.Proxy, error) {
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

func (s *adminServiceImpl) GetProxyAccounts(ctx context.Context, proxyID int64, page, pageSize int) ([]model.Account, int64, error) {
	// Return mock data for now - would need a dedicated repository method
	return []model.Account{REDACTED, 0, nil
REDACTED

func (s *adminServiceImpl) CheckProxyExists(ctx context.Context, host string, port int, username, password string) (bool, error) {
	return s.proxyRepo.ExistsByHostPortAuth(ctx, host, port, username, password)
REDACTED

// Redeem code management implementations
func (s *adminServiceImpl) ListRedeemCodes(ctx context.Context, page, pageSize int, codeType, status, search string) ([]model.RedeemCode, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	codes, result, err := s.redeemCodeRepo.ListWithFilters(ctx, params, codeType, status, search)
	if err != nil {
		return nil, 0, err
REDACTED
	return codes, result.Total, nil
REDACTED

func (s *adminServiceImpl) GetRedeemCode(ctx context.Context, id int64) (*model.RedeemCode, error) {
	return s.redeemCodeRepo.GetByID(ctx, id)
REDACTED

func (s *adminServiceImpl) GenerateRedeemCodes(ctx context.Context, input *GenerateRedeemCodesInput) ([]model.RedeemCode, error) {
	// 如果是订阅类型，验证必须有 GroupID
	if input.Type == model.RedeemTypeSubscription {
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

	codes := make([]model.RedeemCode, 0, input.Count)
	for i := 0; i < input.Count; i++ {
		codeValue, err := model.GenerateRedeemCode()
		if err != nil {
			return nil, err
	REDACTED
		code := model.RedeemCode{
			Code:   codeValue,
			Type:   input.Type,
			Value:  input.Value,
			Status: model.StatusUnused,
	REDACTED
		// 订阅类型专用字段
		if input.Type == model.RedeemTypeSubscription {
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

func (s *adminServiceImpl) ExpireRedeemCode(ctx context.Context, id int64) (*model.RedeemCode, error) {
	code, err := s.redeemCodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED
	code.Status = model.StatusExpired
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

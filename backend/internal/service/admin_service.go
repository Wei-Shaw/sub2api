package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/util/soraerror"
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
	// GetUserBalanceHistory returns paginated balance/concurrency change records for a user.
	// codeType is optional - pass empty string to return all types.
	// Also returns totalRecharged (sum of all positive balance top-ups).
	GetUserBalanceHistory(ctx context.Context, userID int64, page, pageSize int, codeType string) ([]RedeemCode, int64, float64, error)

	// Group management
	ListGroups(ctx context.Context, page, pageSize int, platform, status, search string, isExclusive *bool) ([]Group, int64, error)
	GetAllGroups(ctx context.Context) ([]Group, error)
	GetAllGroupsByPlatform(ctx context.Context, platform string) ([]Group, error)
	GetGroup(ctx context.Context, id int64) (*Group, error)
	CreateGroup(ctx context.Context, input *CreateGroupInput) (*Group, error)
	UpdateGroup(ctx context.Context, id int64, input *UpdateGroupInput) (*Group, error)
	DeleteGroup(ctx context.Context, id int64) error
	GetGroupAPIKeys(ctx context.Context, groupID int64, page, pageSize int) ([]APIKey, int64, error)
	UpdateGroupSortOrders(ctx context.Context, updates []GroupSortOrderUpdate) error

	// API Key management (admin)
	AdminUpdateAPIKeyGroupID(ctx context.Context, keyID int64, groupID *int64) (*AdminUpdateAPIKeyGroupIDResult, error)

	// Account management
	ListAccounts(ctx context.Context, page, pageSize int, platform, accountType, status, search string, groupID int64) ([]Account, int64, error)
	GetAccount(ctx context.Context, id int64) (*Account, error)
	GetAccountsByIDs(ctx context.Context, ids []int64) ([]*Account, error)
	CreateAccount(ctx context.Context, input *CreateAccountInput) (*Account, error)
	UpdateAccount(ctx context.Context, id int64, input *UpdateAccountInput) (*Account, error)
	DeleteAccount(ctx context.Context, id int64) error
	RefreshAccountCredentials(ctx context.Context, id int64) (*Account, error)
	ClearAccountError(ctx context.Context, id int64) (*Account, error)
	SetAccountError(ctx context.Context, id int64, errorMsg string) error
	SetAccountSchedulable(ctx context.Context, id int64, schedulable bool) (*Account, error)
	BulkUpdateAccounts(ctx context.Context, input *BulkUpdateAccountsInput) (*BulkUpdateAccountsResult, error)
	CheckMixedChannelRisk(ctx context.Context, currentAccountID int64, currentAccountPlatform string, groupIDs []int64) error

	// Proxy management
	ListProxies(ctx context.Context, page, pageSize int, protocol, status, search string) ([]Proxy, int64, error)
	ListProxiesWithAccountCount(ctx context.Context, page, pageSize int, protocol, status, search string) ([]ProxyWithAccountCount, int64, error)
	GetAllProxies(ctx context.Context) ([]Proxy, error)
	GetAllProxiesWithAccountCount(ctx context.Context) ([]ProxyWithAccountCount, error)
	GetProxy(ctx context.Context, id int64) (*Proxy, error)
	GetProxiesByIDs(ctx context.Context, ids []int64) ([]Proxy, error)
	CreateProxy(ctx context.Context, input *CreateProxyInput) (*Proxy, error)
	UpdateProxy(ctx context.Context, id int64, input *UpdateProxyInput) (*Proxy, error)
	DeleteProxy(ctx context.Context, id int64) error
	BatchDeleteProxies(ctx context.Context, ids []int64) (*ProxyBatchDeleteResult, error)
	GetProxyAccounts(ctx context.Context, proxyID int64) ([]ProxyAccountSummary, error)
	CheckProxyExists(ctx context.Context, host string, port int, username, password string) (bool, error)
	TestProxy(ctx context.Context, id int64) (*ProxyTestResult, error)
	CheckProxyQuality(ctx context.Context, id int64) (*ProxyQualityCheckResult, error)

	// Redeem code management
	ListRedeemCodes(ctx context.Context, page, pageSize int, codeType, status, search string) ([]RedeemCode, int64, error)
	GetRedeemCode(ctx context.Context, id int64) (*RedeemCode, error)
	GenerateRedeemCodes(ctx context.Context, input *GenerateRedeemCodesInput) ([]RedeemCode, error)
	DeleteRedeemCode(ctx context.Context, id int64) error
	BatchDeleteRedeemCodes(ctx context.Context, ids []int64) (int64, error)
	ExpireRedeemCode(ctx context.Context, id int64) (*RedeemCode, error)
	ResetAccountQuota(ctx context.Context, id int64) error
REDACTED

// CreateUserInput represents input for creating a new user via admin operations.
type CreateUserInput struct {
	Email                 string
	Password              string
	Username              string
	Notes                 string
	Balance               float64
	Concurrency           int
	AllowedGroups         []int64
	SoraStorageQuotaBytes int64
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
	// GroupRates 用户专属分组倍率配置
	// map[groupID]*rate，nil 表示删除该分组的专属倍率
	GroupRates            map[int64]*float64
	SoraStorageQuotaBytes *int64
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
	ImagePrice1K *float64
	ImagePrice2K *float64
	ImagePrice4K *float64
	// Sora 按次计费配置
	SoraImagePrice360          *float64
	SoraImagePrice540          *float64
	SoraVideoPricePerRequest   *float64
	SoraVideoPricePerRequestHD *float64
	ClaudeCodeOnly             bool   // 仅允许 Claude Code 客户端
	FallbackGroupID            *int64 // 降级分组 ID
	// 无效请求兜底分组 ID（仅 anthropic 平台使用）
	FallbackGroupIDOnInvalidRequest *int64
	// 模型路由配置（仅 anthropic 平台使用）
	ModelRouting        map[string][]int64
	ModelRoutingEnabled bool // 是否启用模型路由
	MCPXMLInject        *bool
	// 支持的模型系列（仅 antigravity 平台使用）
	SupportedModelScopes []string
	// Sora 存储配额
	SoraStorageQuotaBytes int64
	// OpenAI Messages 调度配置（仅 openai 平台使用）
	AllowMessagesDispatch bool
	DefaultMappedModel    string
	// 从指定分组复制账号（创建分组后在同一事务内绑定）
	CopyAccountsFromGroupIDs []int64
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
	ImagePrice1K *float64
	ImagePrice2K *float64
	ImagePrice4K *float64
	// Sora 按次计费配置
	SoraImagePrice360          *float64
	SoraImagePrice540          *float64
	SoraVideoPricePerRequest   *float64
	SoraVideoPricePerRequestHD *float64
	ClaudeCodeOnly             *bool  // 仅允许 Claude Code 客户端
	FallbackGroupID            *int64 // 降级分组 ID
	// 无效请求兜底分组 ID（仅 anthropic 平台使用）
	FallbackGroupIDOnInvalidRequest *int64
	// 模型路由配置（仅 anthropic 平台使用）
	ModelRouting        map[string][]int64
	ModelRoutingEnabled *bool // 是否启用模型路由
	MCPXMLInject        *bool
	// 支持的模型系列（仅 antigravity 平台使用）
	SupportedModelScopes *[]string
	// Sora 存储配额
	SoraStorageQuotaBytes *int64
	// OpenAI Messages 调度配置（仅 openai 平台使用）
	AllowMessagesDispatch *bool
	DefaultMappedModel    *string
	// 从指定分组复制账号（同步操作：先清空当前分组的账号绑定，再绑定源分组的账号）
	CopyAccountsFromGroupIDs []int64
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
	LoadFactor         *int
	GroupIDs           []int64
	ExpiresAt          *int64
	AutoPauseOnExpired *bool
	// SkipDefaultGroupBind prevents auto-binding to platform default group when GroupIDs is empty.
	SkipDefaultGroupBind bool
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
	LoadFactor            *int
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
	LoadFactor     *int
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

// AdminUpdateAPIKeyGroupIDResult is the result of AdminUpdateAPIKeyGroupID.
type AdminUpdateAPIKeyGroupIDResult struct {
	APIKey                 *APIKey
	AutoGrantedGroupAccess bool   // true if a new exclusive group permission was auto-added
	GrantedGroupID         *int64 // the group ID that was auto-granted
	GrantedGroupName       string // the group name that was auto-granted
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

type ProxyBatchDeleteResult struct {
	DeletedIDs []int64                   `json:"deleted_ids"`
	Skipped    []ProxyBatchDeleteSkipped `json:"skipped"`
REDACTED

type ProxyBatchDeleteSkipped struct {
	ID     int64  `json:"id"`
	Reason string `json:"reason"`
REDACTED

// ProxyTestResult represents the result of testing a proxy
type ProxyTestResult struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	LatencyMs   int64  `json:"latency_ms,omitempty"`
	IPAddress   string `json:"ip_address,omitempty"`
	City        string `json:"city,omitempty"`
	Region      string `json:"region,omitempty"`
	Country     string `json:"country,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
REDACTED

type ProxyQualityCheckResult struct {
	ProxyID        int64                   `json:"proxy_id"`
	Score          int                     `json:"score"`
	Grade          string                  `json:"grade"`
	Summary        string                  `json:"summary"`
	ExitIP         string                  `json:"exit_ip,omitempty"`
	Country        string                  `json:"country,omitempty"`
	CountryCode    string                  `json:"country_code,omitempty"`
	BaseLatencyMs  int64                   `json:"base_latency_ms,omitempty"`
	PassedCount    int                     `json:"passed_count"`
	WarnCount      int                     `json:"warn_count"`
	FailedCount    int                     `json:"failed_count"`
	ChallengeCount int                     `json:"challenge_count"`
	CheckedAt      int64                   `json:"checked_at"`
	Items          []ProxyQualityCheckItem `json:"items"`
REDACTED

type ProxyQualityCheckItem struct {
	Target     string `json:"target"`
	Status     string `json:"status"` // pass/warn/fail/challenge
	HTTPStatus int    `json:"http_status,omitempty"`
	LatencyMs  int64  `json:"latency_ms,omitempty"`
	Message    string `json:"message,omitempty"`
	CFRay      string `json:"cf_ray,omitempty"`
REDACTED

// ProxyExitInfo represents proxy exit information from ip-api.com
type ProxyExitInfo struct {
	IP          string
	City        string
	Region      string
	Country     string
	CountryCode string
REDACTED

// ProxyExitInfoProber tests proxy connectivity and retrieves exit information
type ProxyExitInfoProber interface {
	ProbeProxy(ctx context.Context, proxyURL string) (*ProxyExitInfo, int64, error)
REDACTED

type proxyQualityTarget struct {
	Target          string
	URL             string
	Method          string
	AllowedStatuses map[int]struct{REDACTED
REDACTED

var proxyQualityTargets = []proxyQualityTarget{
	{
		Target: "openai",
		URL:    "https://api.openai.com/v1/models",
		Method: http.MethodGet,
		AllowedStatuses: map[int]struct{REDACTED{
			http.StatusUnauthorized: {REDACTED,
	REDACTED,
REDACTED,
	{
		Target: "anthropic",
		URL:    "https://api.anthropic.com/v1/messages",
		Method: http.MethodGet,
		AllowedStatuses: map[int]struct{REDACTED{
			http.StatusUnauthorized:     {REDACTED,
			http.StatusMethodNotAllowed: {REDACTED,
			http.StatusNotFound:         {REDACTED,
			http.StatusBadRequest:       {REDACTED,
	REDACTED,
REDACTED,
	{
		Target: "gemini",
		URL:    "https://generativelanguage.googleapis.com/$discovery/rest?version=v1beta",
		Method: http.MethodGet,
		AllowedStatuses: map[int]struct{REDACTED{
			http.StatusOK: {REDACTED,
	REDACTED,
REDACTED,
	{
		Target: "sora",
		URL:    "https://sora.chatgpt.com/backend/me",
		Method: http.MethodGet,
		AllowedStatuses: map[int]struct{REDACTED{
			http.StatusUnauthorized: {REDACTED,
	REDACTED,
REDACTED,
REDACTED

const (
	proxyQualityRequestTimeout        = 15 * time.Second
	proxyQualityResponseHeaderTimeout = 10 * time.Second
	proxyQualityMaxBodyBytes          = int64(8 * 1024)
	proxyQualityClientUserAgent       = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"
)

// adminServiceImpl implements AdminService
type adminServiceImpl struct {
	userRepo             UserRepository
	groupRepo            GroupRepository
	accountRepo          AccountRepository
	soraAccountRepo      SoraAccountRepository // Sora 账号扩展表仓储
	proxyRepo            ProxyRepository
	apiKeyRepo           APIKeyRepository
	redeemCodeRepo       RedeemCodeRepository
	userGroupRateRepo    UserGroupRateRepository
	billingCacheService  *BillingCacheService
	proxyProber          ProxyExitInfoProber
	proxyLatencyCache    ProxyLatencyCache
	authCacheInvalidator APIKeyAuthCacheInvalidator
	entClient            *dbent.Client // 用于开启数据库事务
	settingService       *SettingService
	defaultSubAssigner   DefaultSubscriptionAssigner
REDACTED

type userGroupRateBatchReader interface {
	GetByUserIDs(ctx context.Context, userIDs []int64) (map[int64]map[int64]float64, error)
REDACTED

type groupExistenceBatchReader interface {
	ExistsByIDs(ctx context.Context, ids []int64) (map[int64]bool, error)
REDACTED

// NewAdminService creates a new AdminService
func NewAdminService(
	userRepo UserRepository,
	groupRepo GroupRepository,
	accountRepo AccountRepository,
	soraAccountRepo SoraAccountRepository,
	proxyRepo ProxyRepository,
	apiKeyRepo APIKeyRepository,
	redeemCodeRepo RedeemCodeRepository,
	userGroupRateRepo UserGroupRateRepository,
	billingCacheService *BillingCacheService,
	proxyProber ProxyExitInfoProber,
	proxyLatencyCache ProxyLatencyCache,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
	entClient *dbent.Client,
	settingService *SettingService,
	defaultSubAssigner DefaultSubscriptionAssigner,
) AdminService {
	return &adminServiceImpl{
		userRepo:             userRepo,
		groupRepo:            groupRepo,
		accountRepo:          accountRepo,
		soraAccountRepo:      soraAccountRepo,
		proxyRepo:            proxyRepo,
		apiKeyRepo:           apiKeyRepo,
		redeemCodeRepo:       redeemCodeRepo,
		userGroupRateRepo:    userGroupRateRepo,
		billingCacheService:  billingCacheService,
		proxyProber:          proxyProber,
		proxyLatencyCache:    proxyLatencyCache,
		authCacheInvalidator: authCacheInvalidator,
		entClient:            entClient,
		settingService:       settingService,
		defaultSubAssigner:   defaultSubAssigner,
REDACTED
REDACTED

// User management implementations
func (s *adminServiceImpl) ListUsers(ctx context.Context, page, pageSize int, filters UserListFilters) ([]User, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	users, result, err := s.userRepo.ListWithFilters(ctx, params, filters)
	if err != nil {
		return nil, 0, err
REDACTED
	// 批量加载用户专属分组倍率
	if s.userGroupRateRepo != nil && len(users) > 0 {
		if batchRepo, ok := s.userGroupRateRepo.(userGroupRateBatchReader); ok {
			userIDs := make([]int64, 0, len(users))
			for i := range users {
				userIDs = append(userIDs, users[i].ID)
		REDACTED
			ratesByUser, err := batchRepo.GetByUserIDs(ctx, userIDs)
			if err != nil {
				logger.LegacyPrintf("service.admin", "failed to load user group rates in batch: err=%v", err)
				s.loadUserGroupRatesOneByOne(ctx, users)
		REDACTED else {
				for i := range users {
					if rates, ok := ratesByUser[users[i].ID]; ok {
						users[i].GroupRates = rates
				REDACTED
			REDACTED
		REDACTED
	REDACTED else {
			s.loadUserGroupRatesOneByOne(ctx, users)
	REDACTED
REDACTED
	return users, result.Total, nil
REDACTED

func (s *adminServiceImpl) loadUserGroupRatesOneByOne(ctx context.Context, users []User) {
	if s.userGroupRateRepo == nil {
		return
REDACTED
	for i := range users {
		rates, err := s.userGroupRateRepo.GetByUserID(ctx, users[i].ID)
		if err != nil {
			logger.LegacyPrintf("service.admin", "failed to load user group rates: user_id=%d err=%v", users[i].ID, err)
			continue
	REDACTED
		users[i].GroupRates = rates
REDACTED
REDACTED

func (s *adminServiceImpl) GetUser(ctx context.Context, id int64) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED
	// 加载用户专属分组倍率
	if s.userGroupRateRepo != nil {
		rates, err := s.userGroupRateRepo.GetByUserID(ctx, id)
		if err != nil {
			logger.LegacyPrintf("service.admin", "failed to load user group rates: user_id=%d err=%v", id, err)
	REDACTED else {
			user.GroupRates = rates
	REDACTED
REDACTED
	return user, nil
REDACTED

func (s *adminServiceImpl) CreateUser(ctx context.Context, input *CreateUserInput) (*User, error) {
	user := &User{
		Email:                 input.Email,
		Username:              input.Username,
		Notes:                 input.Notes,
		Role:                  RoleUser, // Always create as regular user, never admin
		Balance:               input.Balance,
		Concurrency:           input.Concurrency,
		Status:                StatusActive,
		AllowedGroups:         input.AllowedGroups,
		SoraStorageQuotaBytes: input.SoraStorageQuotaBytes,
REDACTED
	if err := user.SetPassword(input.Password); err != nil {
		return nil, err
REDACTED
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
REDACTED
	s.assignDefaultSubscriptions(ctx, user.ID)
	return user, nil
REDACTED

func (s *adminServiceImpl) assignDefaultSubscriptions(ctx context.Context, userID int64) {
	if s.settingService == nil || s.defaultSubAssigner == nil || userID <= 0 {
		return
REDACTED
	items := s.settingService.GetDefaultSubscriptions(ctx)
	for _, item := range items {
		if _, _, err := s.defaultSubAssigner.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{
			UserID:       userID,
			GroupID:      item.GroupID,
			ValidityDays: item.ValidityDays,
			Notes:        "auto assigned by default user subscriptions setting",
	REDACTED); err != nil {
			logger.LegacyPrintf("service.admin", "failed to assign default subscription: user_id=%d group_id=%d err=%v", userID, item.GroupID, err)
	REDACTED
REDACTED
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

	if input.SoraStorageQuotaBytes != nil {
		user.SoraStorageQuotaBytes = *input.SoraStorageQuotaBytes
REDACTED

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
REDACTED

	// 同步用户专属分组倍率
	if input.GroupRates != nil && s.userGroupRateRepo != nil {
		if err := s.userGroupRateRepo.SyncUserGroupRates(ctx, user.ID, input.GroupRates); err != nil {
			logger.LegacyPrintf("service.admin", "failed to sync user group rates: user_id=%d err=%v", user.ID, err)
	REDACTED
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
			logger.LegacyPrintf("service.admin", "failed to generate adjustment redeem code: %v", err)
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
			logger.LegacyPrintf("service.admin", "failed to create concurrency adjustment redeem code: %v", err)
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
		logger.LegacyPrintf("service.admin", "delete user failed: user_id=%d err=%v", id, err)
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
				logger.LegacyPrintf("service.admin", "invalidate user balance cache failed: user_id=%d err=%v", userID, err)
		REDACTED
	REDACTED()
REDACTED

	if balanceDiff != 0 {
		code, err := GenerateRedeemCode()
		if err != nil {
			logger.LegacyPrintf("service.admin", "failed to generate adjustment redeem code: %v", err)
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
			logger.LegacyPrintf("service.admin", "failed to create balance adjustment redeem code: %v", err)
	REDACTED
REDACTED

	return user, nil
REDACTED

func (s *adminServiceImpl) GetUserAPIKeys(ctx context.Context, userID int64, page, pageSize int) ([]APIKey, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	keys, result, err := s.apiKeyRepo.ListByUserID(ctx, userID, params, APIKeyListFilters{REDACTED)
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

// GetUserBalanceHistory returns paginated balance/concurrency change records for a user.
func (s *adminServiceImpl) GetUserBalanceHistory(ctx context.Context, userID int64, page, pageSize int, codeType string) ([]RedeemCode, int64, float64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	codes, result, err := s.redeemCodeRepo.ListByUserPaginated(ctx, userID, params, codeType)
	if err != nil {
		return nil, 0, 0, err
REDACTED
	// Aggregate total recharged amount (only once, regardless of type filter)
	totalRecharged, err := s.redeemCodeRepo.SumPositiveBalanceByUser(ctx, userID)
	if err != nil {
		return nil, 0, 0, err
REDACTED
	return codes, result.Total, totalRecharged, nil
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
	soraImagePrice360 := normalizePrice(input.SoraImagePrice360)
	soraImagePrice540 := normalizePrice(input.SoraImagePrice540)
	soraVideoPrice := normalizePrice(input.SoraVideoPricePerRequest)
	soraVideoPriceHD := normalizePrice(input.SoraVideoPricePerRequestHD)

	// 校验降级分组
	if input.FallbackGroupID != nil {
		if err := s.validateFallbackGroup(ctx, 0, *input.FallbackGroupID); err != nil {
			return nil, err
	REDACTED
REDACTED
	fallbackOnInvalidRequest := input.FallbackGroupIDOnInvalidRequest
	if fallbackOnInvalidRequest != nil && *fallbackOnInvalidRequest <= 0 {
		fallbackOnInvalidRequest = nil
REDACTED
	// 校验无效请求兜底分组
	if fallbackOnInvalidRequest != nil {
		if err := s.validateFallbackGroupOnInvalidRequest(ctx, 0, platform, subscriptionType, *fallbackOnInvalidRequest); err != nil {
			return nil, err
	REDACTED
REDACTED

	// MCPXMLInject：默认为 true，仅当显式传入 false 时关闭
	mcpXMLInject := true
	if input.MCPXMLInject != nil {
		mcpXMLInject = *input.MCPXMLInject
REDACTED

	// 如果指定了复制账号的源分组，先获取账号 ID 列表
	var accountIDsToCopy []int64
	if len(input.CopyAccountsFromGroupIDs) > 0 {
		// 去重源分组 IDs
		seen := make(map[int64]struct{REDACTED)
		uniqueSourceGroupIDs := make([]int64, 0, len(input.CopyAccountsFromGroupIDs))
		for _, srcGroupID := range input.CopyAccountsFromGroupIDs {
			if _, exists := seen[srcGroupID]; !exists {
				seen[srcGroupID] = struct{REDACTED{REDACTED
				uniqueSourceGroupIDs = append(uniqueSourceGroupIDs, srcGroupID)
		REDACTED
	REDACTED

		// 校验源分组的平台是否与新分组一致
		for _, srcGroupID := range uniqueSourceGroupIDs {
			srcGroup, err := s.groupRepo.GetByIDLite(ctx, srcGroupID)
			if err != nil {
				return nil, fmt.Errorf("source group %d not found: %w", srcGroupID, err)
		REDACTED
			if srcGroup.Platform != platform {
				return nil, fmt.Errorf("source group %d platform mismatch: expected %s, got %s", srcGroupID, platform, srcGroup.Platform)
		REDACTED
	REDACTED

		// 获取所有源分组的账号（去重）
		var err error
		accountIDsToCopy, err = s.groupRepo.GetAccountIDsByGroupIDs(ctx, uniqueSourceGroupIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to get accounts from source groups: %w", err)
	REDACTED
REDACTED

	group := &Group{
		Name:                            input.Name,
		Description:                     input.Description,
		Platform:                        platform,
		RateMultiplier:                  input.RateMultiplier,
		IsExclusive:                     input.IsExclusive,
		Status:                          StatusActive,
		SubscriptionType:                subscriptionType,
		DailyLimitUSD:                   dailyLimit,
		WeeklyLimitUSD:                  weeklyLimit,
		MonthlyLimitUSD:                 monthlyLimit,
		ImagePrice1K:                    imagePrice1K,
		ImagePrice2K:                    imagePrice2K,
		ImagePrice4K:                    imagePrice4K,
		SoraImagePrice360:               soraImagePrice360,
		SoraImagePrice540:               soraImagePrice540,
		SoraVideoPricePerRequest:        soraVideoPrice,
		SoraVideoPricePerRequestHD:      soraVideoPriceHD,
		ClaudeCodeOnly:                  input.ClaudeCodeOnly,
		FallbackGroupID:                 input.FallbackGroupID,
		FallbackGroupIDOnInvalidRequest: fallbackOnInvalidRequest,
		ModelRouting:                    input.ModelRouting,
		MCPXMLInject:                    mcpXMLInject,
		SupportedModelScopes:            input.SupportedModelScopes,
		SoraStorageQuotaBytes:           input.SoraStorageQuotaBytes,
		AllowMessagesDispatch:           input.AllowMessagesDispatch,
		DefaultMappedModel:              input.DefaultMappedModel,
REDACTED
	if err := s.groupRepo.Create(ctx, group); err != nil {
		return nil, err
REDACTED

	// 如果有需要复制的账号，绑定到新分组
	if len(accountIDsToCopy) > 0 {
		if err := s.groupRepo.BindAccountsToGroup(ctx, group.ID, accountIDsToCopy); err != nil {
			return nil, fmt.Errorf("failed to bind accounts to new group: %w", err)
	REDACTED
		group.AccountCount = int64(len(accountIDsToCopy))
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

// validateFallbackGroupOnInvalidRequest 校验无效请求兜底分组的有效性
// currentGroupID: 当前分组 ID（新建时为 0）
// platform/subscriptionType: 当前分组的有效平台/订阅类型
// fallbackGroupID: 兜底分组 ID
func (s *adminServiceImpl) validateFallbackGroupOnInvalidRequest(ctx context.Context, currentGroupID int64, platform, subscriptionType string, fallbackGroupID int64) error {
	if platform != PlatformAnthropic && platform != PlatformAntigravity {
		return fmt.Errorf("invalid request fallback only supported for anthropic or antigravity groups")
REDACTED
	if subscriptionType == SubscriptionTypeSubscription {
		return fmt.Errorf("subscription groups cannot set invalid request fallback")
REDACTED
	if currentGroupID > 0 && currentGroupID == fallbackGroupID {
		return fmt.Errorf("cannot set self as invalid request fallback group")
REDACTED

	fallbackGroup, err := s.groupRepo.GetByIDLite(ctx, fallbackGroupID)
	if err != nil {
		return fmt.Errorf("fallback group not found: %w", err)
REDACTED
	if fallbackGroup.Platform != PlatformAnthropic {
		return fmt.Errorf("fallback group must be anthropic platform")
REDACTED
	if fallbackGroup.SubscriptionType == SubscriptionTypeSubscription {
		return fmt.Errorf("fallback group cannot be subscription type")
REDACTED
	if fallbackGroup.FallbackGroupIDOnInvalidRequest != nil {
		return fmt.Errorf("fallback group cannot have invalid request fallback configured")
REDACTED
	return nil
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
	if input.SoraImagePrice360 != nil {
		group.SoraImagePrice360 = normalizePrice(input.SoraImagePrice360)
REDACTED
	if input.SoraImagePrice540 != nil {
		group.SoraImagePrice540 = normalizePrice(input.SoraImagePrice540)
REDACTED
	if input.SoraVideoPricePerRequest != nil {
		group.SoraVideoPricePerRequest = normalizePrice(input.SoraVideoPricePerRequest)
REDACTED
	if input.SoraVideoPricePerRequestHD != nil {
		group.SoraVideoPricePerRequestHD = normalizePrice(input.SoraVideoPricePerRequestHD)
REDACTED
	if input.SoraStorageQuotaBytes != nil {
		group.SoraStorageQuotaBytes = *input.SoraStorageQuotaBytes
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
	fallbackOnInvalidRequest := group.FallbackGroupIDOnInvalidRequest
	if input.FallbackGroupIDOnInvalidRequest != nil {
		if *input.FallbackGroupIDOnInvalidRequest > 0 {
			fallbackOnInvalidRequest = input.FallbackGroupIDOnInvalidRequest
	REDACTED else {
			fallbackOnInvalidRequest = nil
	REDACTED
REDACTED
	if fallbackOnInvalidRequest != nil {
		if err := s.validateFallbackGroupOnInvalidRequest(ctx, id, group.Platform, group.SubscriptionType, *fallbackOnInvalidRequest); err != nil {
			return nil, err
	REDACTED
REDACTED
	group.FallbackGroupIDOnInvalidRequest = fallbackOnInvalidRequest

	// 模型路由配置
	if input.ModelRouting != nil {
		group.ModelRouting = input.ModelRouting
REDACTED
	if input.ModelRoutingEnabled != nil {
		group.ModelRoutingEnabled = *input.ModelRoutingEnabled
REDACTED
	if input.MCPXMLInject != nil {
		group.MCPXMLInject = *input.MCPXMLInject
REDACTED

	// 支持的模型系列（仅 antigravity 平台使用）
	if input.SupportedModelScopes != nil {
		group.SupportedModelScopes = *input.SupportedModelScopes
REDACTED

	// OpenAI Messages 调度配置
	if input.AllowMessagesDispatch != nil {
		group.AllowMessagesDispatch = *input.AllowMessagesDispatch
REDACTED
	if input.DefaultMappedModel != nil {
		group.DefaultMappedModel = *input.DefaultMappedModel
REDACTED

	if err := s.groupRepo.Update(ctx, group); err != nil {
		return nil, err
REDACTED

	// 如果指定了复制账号的源分组，同步绑定（替换当前分组的账号）
	if len(input.CopyAccountsFromGroupIDs) > 0 {
		// 去重源分组 IDs
		seen := make(map[int64]struct{REDACTED)
		uniqueSourceGroupIDs := make([]int64, 0, len(input.CopyAccountsFromGroupIDs))
		for _, srcGroupID := range input.CopyAccountsFromGroupIDs {
			// 校验：源分组不能是自身
			if srcGroupID == id {
				return nil, fmt.Errorf("cannot copy accounts from self")
		REDACTED
			// 去重
			if _, exists := seen[srcGroupID]; !exists {
				seen[srcGroupID] = struct{REDACTED{REDACTED
				uniqueSourceGroupIDs = append(uniqueSourceGroupIDs, srcGroupID)
		REDACTED
	REDACTED

		// 校验源分组的平台是否与当前分组一致
		for _, srcGroupID := range uniqueSourceGroupIDs {
			srcGroup, err := s.groupRepo.GetByIDLite(ctx, srcGroupID)
			if err != nil {
				return nil, fmt.Errorf("source group %d not found: %w", srcGroupID, err)
		REDACTED
			if srcGroup.Platform != group.Platform {
				return nil, fmt.Errorf("source group %d platform mismatch: expected %s, got %s", srcGroupID, group.Platform, srcGroup.Platform)
		REDACTED
	REDACTED

		// 获取所有源分组的账号（去重）
		accountIDsToCopy, err := s.groupRepo.GetAccountIDsByGroupIDs(ctx, uniqueSourceGroupIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to get accounts from source groups: %w", err)
	REDACTED

		// 先清空当前分组的所有账号绑定
		if _, err := s.groupRepo.DeleteAccountGroupsByGroupID(ctx, id); err != nil {
			return nil, fmt.Errorf("failed to clear existing account bindings: %w", err)
	REDACTED

		// 再绑定源分组的账号
		if len(accountIDsToCopy) > 0 {
			if err := s.groupRepo.BindAccountsToGroup(ctx, id, accountIDsToCopy); err != nil {
				return nil, fmt.Errorf("failed to bind accounts to group: %w", err)
		REDACTED
	REDACTED
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
	// 注意：user_group_rate_multipliers 表通过外键 ON DELETE CASCADE 自动清理

	// 事务成功后，异步失效受影响用户的订阅缓存
	if len(affectedUserIDs) > 0 && s.billingCacheService != nil {
		groupID := id
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			for _, userID := range affectedUserIDs {
				if err := s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID); err != nil {
					logger.LegacyPrintf("service.admin", "invalidate subscription cache failed: user_id=%d group_id=%d err=%v", userID, groupID, err)
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

func (s *adminServiceImpl) UpdateGroupSortOrders(ctx context.Context, updates []GroupSortOrderUpdate) error {
	return s.groupRepo.UpdateSortOrders(ctx, updates)
REDACTED

// AdminUpdateAPIKeyGroupID 管理员修改 API Key 分组绑定
// groupID: nil=不修改, 指向0=解绑, 指向正整数=绑定到目标分组
func (s *adminServiceImpl) AdminUpdateAPIKeyGroupID(ctx context.Context, keyID int64, groupID *int64) (*AdminUpdateAPIKeyGroupIDResult, error) {
	apiKey, err := s.apiKeyRepo.GetByID(ctx, keyID)
	if err != nil {
		return nil, err
REDACTED

	if groupID == nil {
		// nil 表示不修改，直接返回
		return &AdminUpdateAPIKeyGroupIDResult{APIKey: apiKeyREDACTED, nil
REDACTED

	if *groupID < 0 {
		return nil, infraerrors.BadRequest("INVALID_GROUP_ID", "group_id must be non-negative")
REDACTED

	result := &AdminUpdateAPIKeyGroupIDResult{REDACTED

	if *groupID == 0 {
		// 0 表示解绑分组（不修改 user_allowed_groups，避免影响用户其他 Key）
		apiKey.GroupID = nil
		apiKey.Group = nil
REDACTED else {
		// 验证目标分组存在且状态为 active
		group, err := s.groupRepo.GetByID(ctx, *groupID)
		if err != nil {
			return nil, err
	REDACTED
		if group.Status != StatusActive {
			return nil, infraerrors.BadRequest("GROUP_NOT_ACTIVE", "target group is not active")
	REDACTED
		// 订阅类型分组：不允许通过此 API 直接绑定，需通过订阅管理流程
		if group.IsSubscriptionType() {
			return nil, infraerrors.BadRequest("SUBSCRIPTION_GROUP_NOT_ALLOWED", "subscription groups must be managed through the subscription workflow")
	REDACTED

		gid := *groupID
		apiKey.GroupID = &gid
		apiKey.Group = group

		// 专属标准分组：使用事务保证「添加分组权限」与「更新 API Key」的原子性
		if group.IsExclusive {
			opCtx := ctx
			var tx *dbent.Tx
			if s.entClient == nil {
				logger.LegacyPrintf("service.admin", "Warning: entClient is nil, skipping transaction protection for exclusive group binding")
		REDACTED else {
				var txErr error
				tx, txErr = s.entClient.Tx(ctx)
				if txErr != nil {
					return nil, fmt.Errorf("begin transaction: %w", txErr)
			REDACTED
				defer func() { _ = tx.Rollback() REDACTED()
				opCtx = dbent.NewTxContext(ctx, tx)
		REDACTED

			if addErr := s.userRepo.AddGroupToAllowedGroups(opCtx, apiKey.UserID, gid); addErr != nil {
				return nil, fmt.Errorf("add group to user allowed groups: %w", addErr)
		REDACTED
			if err := s.apiKeyRepo.Update(opCtx, apiKey); err != nil {
				return nil, fmt.Errorf("update api key: %w", err)
		REDACTED
			if tx != nil {
				if err := tx.Commit(); err != nil {
					return nil, fmt.Errorf("commit transaction: %w", err)
			REDACTED
		REDACTED

			result.AutoGrantedGroupAccess = true
			result.GrantedGroupID = &gid
			result.GrantedGroupName = group.Name

			// 失效认证缓存（在事务提交后执行）
			if s.authCacheInvalidator != nil {
				s.authCacheInvalidator.InvalidateAuthCacheByKey(ctx, apiKey.Key)
		REDACTED

			result.APIKey = apiKey
			return result, nil
	REDACTED
REDACTED

	// 非专属分组 / 解绑：无需事务，单步更新即可
	if err := s.apiKeyRepo.Update(ctx, apiKey); err != nil {
		return nil, fmt.Errorf("update api key: %w", err)
REDACTED

	// 失效认证缓存
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByKey(ctx, apiKey.Key)
REDACTED

	result.APIKey = apiKey
	return result, nil
REDACTED

// Account management implementations
func (s *adminServiceImpl) ListAccounts(ctx context.Context, page, pageSize int, platform, accountType, status, search string, groupID int64) ([]Account, int64, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSizeREDACTED
	accounts, result, err := s.accountRepo.ListWithFilters(ctx, params, platform, accountType, status, search, groupID)
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
	if len(groupIDs) == 0 && !input.SkipDefaultGroupBind {
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

	// Sora apikey 账号的 base_url 必填校验
	if input.Platform == PlatformSora && input.Type == AccountTypeAPIKey {
		baseURL, _ := input.Credentials["base_url"].(string)
		baseURL = strings.TrimSpace(baseURL)
		if baseURL == "" {
			return nil, errors.New("sora apikey 账号必须设置 base_url")
	REDACTED
		if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
			return nil, errors.New("base_url 必须以 http:// 或 https:// 开头")
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
	if input.LoadFactor != nil && *input.LoadFactor > 0 {
		if *input.LoadFactor > 10000 {
			return nil, errors.New("load_factor must be <= 10000")
	REDACTED
		account.LoadFactor = input.LoadFactor
REDACTED
	if err := s.accountRepo.Create(ctx, account); err != nil {
		return nil, err
REDACTED

	// 如果是 Sora 平台账号，自动创建 sora_accounts 扩展表记录
	if account.Platform == PlatformSora && s.soraAccountRepo != nil {
		soraUpdates := map[string]any{
			"access_token":  account.GetCredential("access_token"),
			"refresh_token": account.GetCredential("refresh_token"),
	REDACTED
		if err := s.soraAccountRepo.Upsert(ctx, account.ID, soraUpdates); err != nil {
			// 只记录警告日志，不阻塞账号创建
			logger.LegacyPrintf("service.admin", "[AdminService] 创建 sora_accounts 记录失败: account_id=%d err=%v", account.ID, err)
	REDACTED
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
		// 保留 quota_used，防止编辑账号时意外重置配额用量
		if oldQuotaUsed, ok := account.Extra["quota_used"]; ok {
			input.Extra["quota_used"] = oldQuotaUsed
	REDACTED
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
	if input.LoadFactor != nil {
		if *input.LoadFactor <= 0 {
			account.LoadFactor = nil // 0 或负数表示清除
	REDACTED else if *input.LoadFactor > 10000 {
			return nil, errors.New("load_factor must be <= 10000")
	REDACTED else {
			account.LoadFactor = input.LoadFactor
	REDACTED
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

	// Sora apikey 账号的 base_url 必填校验
	if account.Platform == PlatformSora && account.Type == AccountTypeAPIKey {
		baseURL, _ := account.Credentials["base_url"].(string)
		baseURL = strings.TrimSpace(baseURL)
		if baseURL == "" {
			return nil, errors.New("sora apikey 账号必须设置 base_url")
	REDACTED
		if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
			return nil, errors.New("base_url 必须以 http:// 或 https:// 开头")
	REDACTED
REDACTED

	// 先验证分组是否存在（在任何写操作之前）
	if input.GroupIDs != nil {
		if err := s.validateGroupIDsExist(ctx, *input.GroupIDs); err != nil {
			return nil, err
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
	updated, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED
	return updated, nil
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
	if input.GroupIDs != nil {
		if err := s.validateGroupIDsExist(ctx, *input.GroupIDs); err != nil {
			return nil, err
	REDACTED
REDACTED

	needMixedChannelCheck := input.GroupIDs != nil && !input.SkipMixedChannelCheck

	// 预加载账号平台信息（混合渠道检查需要）。
	platformByID := map[int64]string{REDACTED
	if needMixedChannelCheck {
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

	// 预检查混合渠道风险：在任何写操作之前，若发现风险立即返回错误。
	if needMixedChannelCheck {
		for _, accountID := range input.AccountIDs {
			platform := platformByID[accountID]
			if platform == "" {
				continue
		REDACTED
			if err := s.checkMixedChannelRisk(ctx, accountID, platform, *input.GroupIDs); err != nil {
				return nil, err
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
	if input.LoadFactor != nil {
		if *input.LoadFactor <= 0 {
			repoUpdates.LoadFactor = nil // 0 或负数表示清除
	REDACTED else if *input.LoadFactor > 10000 {
			return nil, errors.New("load_factor must be <= 10000")
	REDACTED else {
			repoUpdates.LoadFactor = input.LoadFactor
	REDACTED
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
	if err := s.accountRepo.Delete(ctx, id); err != nil {
		return err
REDACTED
	return nil
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

func (s *adminServiceImpl) SetAccountError(ctx context.Context, id int64, errorMsg string) error {
	return s.accountRepo.SetError(ctx, id, errorMsg)
REDACTED

func (s *adminServiceImpl) SetAccountSchedulable(ctx context.Context, id int64, schedulable bool) (*Account, error) {
	if err := s.accountRepo.SetSchedulable(ctx, id, schedulable); err != nil {
		return nil, err
REDACTED
	updated, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED
	return updated, nil
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
	s.attachProxyLatency(ctx, proxies)
	return proxies, result.Total, nil
REDACTED

func (s *adminServiceImpl) GetAllProxies(ctx context.Context) ([]Proxy, error) {
	return s.proxyRepo.ListActive(ctx)
REDACTED

func (s *adminServiceImpl) GetAllProxiesWithAccountCount(ctx context.Context) ([]ProxyWithAccountCount, error) {
	proxies, err := s.proxyRepo.ListActiveWithAccountCount(ctx)
	if err != nil {
		return nil, err
REDACTED
	s.attachProxyLatency(ctx, proxies)
	return proxies, nil
REDACTED

func (s *adminServiceImpl) GetProxy(ctx context.Context, id int64) (*Proxy, error) {
	return s.proxyRepo.GetByID(ctx, id)
REDACTED

func (s *adminServiceImpl) GetProxiesByIDs(ctx context.Context, ids []int64) ([]Proxy, error) {
	return s.proxyRepo.ListByIDs(ctx, ids)
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
	// Probe latency asynchronously so creation isn't blocked by network timeout.
	go s.probeProxyLatency(context.Background(), proxy)
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
	count, err := s.proxyRepo.CountAccountsByProxyID(ctx, id)
	if err != nil {
		return err
REDACTED
	if count > 0 {
		return ErrProxyInUse
REDACTED
	return s.proxyRepo.Delete(ctx, id)
REDACTED

func (s *adminServiceImpl) BatchDeleteProxies(ctx context.Context, ids []int64) (*ProxyBatchDeleteResult, error) {
	result := &ProxyBatchDeleteResult{REDACTED
	if len(ids) == 0 {
		return result, nil
REDACTED

	for _, id := range ids {
		count, err := s.proxyRepo.CountAccountsByProxyID(ctx, id)
		if err != nil {
			result.Skipped = append(result.Skipped, ProxyBatchDeleteSkipped{
				ID:     id,
				Reason: err.Error(),
		REDACTED)
			continue
	REDACTED
		if count > 0 {
			result.Skipped = append(result.Skipped, ProxyBatchDeleteSkipped{
				ID:     id,
				Reason: ErrProxyInUse.Error(),
		REDACTED)
			continue
	REDACTED
		if err := s.proxyRepo.Delete(ctx, id); err != nil {
			result.Skipped = append(result.Skipped, ProxyBatchDeleteSkipped{
				ID:     id,
				Reason: err.Error(),
		REDACTED)
			continue
	REDACTED
		result.DeletedIDs = append(result.DeletedIDs, id)
REDACTED

	return result, nil
REDACTED

func (s *adminServiceImpl) GetProxyAccounts(ctx context.Context, proxyID int64) ([]ProxyAccountSummary, error) {
	return s.proxyRepo.ListAccountSummariesByProxyID(ctx, proxyID)
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
		s.saveProxyLatency(ctx, id, &ProxyLatencyInfo{
			Success:   false,
			Message:   err.Error(),
			UpdatedAt: time.Now(),
	REDACTED)
		return &ProxyTestResult{
			Success: false,
			Message: err.Error(),
	REDACTED, nil
REDACTED

	latency := latencyMs
	s.saveProxyLatency(ctx, id, &ProxyLatencyInfo{
		Success:     true,
		LatencyMs:   &latency,
		Message:     "Proxy is accessible",
		IPAddress:   exitInfo.IP,
		Country:     exitInfo.Country,
		CountryCode: exitInfo.CountryCode,
		Region:      exitInfo.Region,
		City:        exitInfo.City,
		UpdatedAt:   time.Now(),
REDACTED)
	return &ProxyTestResult{
		Success:     true,
		Message:     "Proxy is accessible",
		LatencyMs:   latencyMs,
		IPAddress:   exitInfo.IP,
		City:        exitInfo.City,
		Region:      exitInfo.Region,
		Country:     exitInfo.Country,
		CountryCode: exitInfo.CountryCode,
REDACTED, nil
REDACTED

func (s *adminServiceImpl) CheckProxyQuality(ctx context.Context, id int64) (*ProxyQualityCheckResult, error) {
	proxy, err := s.proxyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED

	result := &ProxyQualityCheckResult{
		ProxyID:   id,
		Score:     100,
		Grade:     "A",
		CheckedAt: time.Now().Unix(),
		Items:     make([]ProxyQualityCheckItem, 0, len(proxyQualityTargets)+1),
REDACTED

	proxyURL := proxy.URL()
	if s.proxyProber == nil {
		result.Items = append(result.Items, ProxyQualityCheckItem{
			Target:  "base_connectivity",
			Status:  "fail",
			Message: "代理探测服务未配置",
	REDACTED)
		result.FailedCount++
		finalizeProxyQualityResult(result)
		s.saveProxyQualitySnapshot(ctx, id, result, nil)
		return result, nil
REDACTED

	exitInfo, latencyMs, err := s.proxyProber.ProbeProxy(ctx, proxyURL)
	if err != nil {
		result.Items = append(result.Items, ProxyQualityCheckItem{
			Target:    "base_connectivity",
			Status:    "fail",
			LatencyMs: latencyMs,
			Message:   err.Error(),
	REDACTED)
		result.FailedCount++
		finalizeProxyQualityResult(result)
		s.saveProxyQualitySnapshot(ctx, id, result, nil)
		return result, nil
REDACTED

	result.ExitIP = exitInfo.IP
	result.Country = exitInfo.Country
	result.CountryCode = exitInfo.CountryCode
	result.BaseLatencyMs = latencyMs
	result.Items = append(result.Items, ProxyQualityCheckItem{
		Target:    "base_connectivity",
		Status:    "pass",
		LatencyMs: latencyMs,
		Message:   "代理出口连通正常",
REDACTED)
	result.PassedCount++

	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL:              proxyURL,
		Timeout:               proxyQualityRequestTimeout,
		ResponseHeaderTimeout: proxyQualityResponseHeaderTimeout,
REDACTED)
	if err != nil {
		result.Items = append(result.Items, ProxyQualityCheckItem{
			Target:  "http_client",
			Status:  "fail",
			Message: fmt.Sprintf("创建检测客户端失败: %v", err),
	REDACTED)
		result.FailedCount++
		finalizeProxyQualityResult(result)
		s.saveProxyQualitySnapshot(ctx, id, result, exitInfo)
		return result, nil
REDACTED

	for _, target := range proxyQualityTargets {
		item := runProxyQualityTarget(ctx, client, target)
		result.Items = append(result.Items, item)
		switch item.Status {
		case "pass":
			result.PassedCount++
		case "warn":
			result.WarnCount++
		case "challenge":
			result.ChallengeCount++
		default:
			result.FailedCount++
	REDACTED
REDACTED

	finalizeProxyQualityResult(result)
	s.saveProxyQualitySnapshot(ctx, id, result, exitInfo)
	return result, nil
REDACTED

func runProxyQualityTarget(ctx context.Context, client *http.Client, target proxyQualityTarget) ProxyQualityCheckItem {
	item := ProxyQualityCheckItem{
		Target: target.Target,
REDACTED

	req, err := http.NewRequestWithContext(ctx, target.Method, target.URL, nil)
	if err != nil {
		item.Status = "fail"
		item.Message = fmt.Sprintf("构建请求失败: %v", err)
		return item
REDACTED
	req.Header.Set("Accept", "application/json,text/html,*/*")
	req.Header.Set("User-Agent", proxyQualityClientUserAgent)

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		item.Status = "fail"
		item.LatencyMs = time.Since(start).Milliseconds()
		item.Message = fmt.Sprintf("请求失败: %v", err)
		return item
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	item.LatencyMs = time.Since(start).Milliseconds()
	item.HTTPStatus = resp.StatusCode

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, proxyQualityMaxBodyBytes+1))
	if readErr != nil {
		item.Status = "fail"
		item.Message = fmt.Sprintf("读取响应失败: %v", readErr)
		return item
REDACTED
	if int64(len(body)) > proxyQualityMaxBodyBytes {
		body = body[:proxyQualityMaxBodyBytes]
REDACTED

	if target.Target == "sora" && soraerror.IsCloudflareChallengeResponse(resp.StatusCode, resp.Header, body) {
		item.Status = "challenge"
		item.CFRay = soraerror.ExtractCloudflareRayID(resp.Header, body)
		item.Message = "Sora 命中 Cloudflare challenge"
		return item
REDACTED

	if _, ok := target.AllowedStatuses[resp.StatusCode]; ok {
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			item.Status = "pass"
			item.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
	REDACTED else {
			item.Status = "warn"
			item.Message = fmt.Sprintf("HTTP %d（目标可达，但鉴权或方法受限）", resp.StatusCode)
	REDACTED
		return item
REDACTED

	if resp.StatusCode == http.StatusTooManyRequests {
		item.Status = "warn"
		item.Message = "目标返回 429，可能存在频控"
		return item
REDACTED

	item.Status = "fail"
	item.Message = fmt.Sprintf("非预期状态码: %d", resp.StatusCode)
	return item
REDACTED

func finalizeProxyQualityResult(result *ProxyQualityCheckResult) {
	if result == nil {
		return
REDACTED
	score := 100 - result.WarnCount*10 - result.FailedCount*22 - result.ChallengeCount*30
	if score < 0 {
		score = 0
REDACTED
	result.Score = score
	result.Grade = proxyQualityGrade(score)
	result.Summary = fmt.Sprintf(
		"通过 %d 项，告警 %d 项，失败 %d 项，挑战 %d 项",
		result.PassedCount,
		result.WarnCount,
		result.FailedCount,
		result.ChallengeCount,
	)
REDACTED

func proxyQualityGrade(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 75:
		return "B"
	case score >= 60:
		return "C"
	case score >= 40:
		return "D"
	default:
		return "F"
REDACTED
REDACTED

func proxyQualityOverallStatus(result *ProxyQualityCheckResult) string {
	if result == nil {
		return ""
REDACTED
	if result.ChallengeCount > 0 {
		return "challenge"
REDACTED
	if result.FailedCount > 0 {
		return "failed"
REDACTED
	if result.WarnCount > 0 {
		return "warn"
REDACTED
	if result.PassedCount > 0 {
		return "healthy"
REDACTED
	return "failed"
REDACTED

func proxyQualityFirstCFRay(result *ProxyQualityCheckResult) string {
	if result == nil {
		return ""
REDACTED
	for _, item := range result.Items {
		if item.CFRay != "" {
			return item.CFRay
	REDACTED
REDACTED
	return ""
REDACTED

func proxyQualityBaseConnectivityPass(result *ProxyQualityCheckResult) bool {
	if result == nil {
		return false
REDACTED
	for _, item := range result.Items {
		if item.Target == "base_connectivity" {
			return item.Status == "pass"
	REDACTED
REDACTED
	return false
REDACTED

func (s *adminServiceImpl) saveProxyQualitySnapshot(ctx context.Context, proxyID int64, result *ProxyQualityCheckResult, exitInfo *ProxyExitInfo) {
	if result == nil {
		return
REDACTED
	score := result.Score
	checkedAt := result.CheckedAt
	info := &ProxyLatencyInfo{
		Success:          proxyQualityBaseConnectivityPass(result),
		Message:          result.Summary,
		QualityStatus:    proxyQualityOverallStatus(result),
		QualityScore:     &score,
		QualityGrade:     result.Grade,
		QualitySummary:   result.Summary,
		QualityCheckedAt: &checkedAt,
		QualityCFRay:     proxyQualityFirstCFRay(result),
		UpdatedAt:        time.Now(),
REDACTED
	if result.BaseLatencyMs > 0 {
		latency := result.BaseLatencyMs
		info.LatencyMs = &latency
REDACTED
	if exitInfo != nil {
		info.IPAddress = exitInfo.IP
		info.Country = exitInfo.Country
		info.CountryCode = exitInfo.CountryCode
		info.Region = exitInfo.Region
		info.City = exitInfo.City
REDACTED
	s.saveProxyLatency(ctx, proxyID, info)
REDACTED

func (s *adminServiceImpl) probeProxyLatency(ctx context.Context, proxy *Proxy) {
	if s.proxyProber == nil || proxy == nil {
		return
REDACTED
	exitInfo, latencyMs, err := s.proxyProber.ProbeProxy(ctx, proxy.URL())
	if err != nil {
		s.saveProxyLatency(ctx, proxy.ID, &ProxyLatencyInfo{
			Success:   false,
			Message:   err.Error(),
			UpdatedAt: time.Now(),
	REDACTED)
		return
REDACTED

	latency := latencyMs
	s.saveProxyLatency(ctx, proxy.ID, &ProxyLatencyInfo{
		Success:     true,
		LatencyMs:   &latency,
		Message:     "Proxy is accessible",
		IPAddress:   exitInfo.IP,
		Country:     exitInfo.Country,
		CountryCode: exitInfo.CountryCode,
		Region:      exitInfo.Region,
		City:        exitInfo.City,
		UpdatedAt:   time.Now(),
REDACTED)
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

func (s *adminServiceImpl) validateGroupIDsExist(ctx context.Context, groupIDs []int64) error {
	if len(groupIDs) == 0 {
		return nil
REDACTED
	if s.groupRepo == nil {
		return errors.New("group repository not configured")
REDACTED

	if batchReader, ok := s.groupRepo.(groupExistenceBatchReader); ok {
		existsByID, err := batchReader.ExistsByIDs(ctx, groupIDs)
		if err != nil {
			return fmt.Errorf("check groups exists: %w", err)
	REDACTED
		for _, groupID := range groupIDs {
			if groupID <= 0 || !existsByID[groupID] {
				return fmt.Errorf("get group: %w", ErrGroupNotFound)
		REDACTED
	REDACTED
		return nil
REDACTED

	for _, groupID := range groupIDs {
		if _, err := s.groupRepo.GetByID(ctx, groupID); err != nil {
			return fmt.Errorf("get group: %w", err)
	REDACTED
REDACTED
	return nil
REDACTED

// CheckMixedChannelRisk checks whether target groups contain mixed channels for the current account platform.
func (s *adminServiceImpl) CheckMixedChannelRisk(ctx context.Context, currentAccountID int64, currentAccountPlatform string, groupIDs []int64) error {
	return s.checkMixedChannelRisk(ctx, currentAccountID, currentAccountPlatform, groupIDs)
REDACTED

func (s *adminServiceImpl) attachProxyLatency(ctx context.Context, proxies []ProxyWithAccountCount) {
	if s.proxyLatencyCache == nil || len(proxies) == 0 {
		return
REDACTED

	ids := make([]int64, 0, len(proxies))
	for i := range proxies {
		ids = append(ids, proxies[i].ID)
REDACTED

	latencies, err := s.proxyLatencyCache.GetProxyLatencies(ctx, ids)
	if err != nil {
		logger.LegacyPrintf("service.admin", "Warning: load proxy latency cache failed: %v", err)
		return
REDACTED

	for i := range proxies {
		info := latencies[proxies[i].ID]
		if info == nil {
			continue
	REDACTED
		if info.Success {
			proxies[i].LatencyStatus = "success"
			proxies[i].LatencyMs = info.LatencyMs
	REDACTED else {
			proxies[i].LatencyStatus = "failed"
	REDACTED
		proxies[i].LatencyMessage = info.Message
		proxies[i].IPAddress = info.IPAddress
		proxies[i].Country = info.Country
		proxies[i].CountryCode = info.CountryCode
		proxies[i].Region = info.Region
		proxies[i].City = info.City
		proxies[i].QualityStatus = info.QualityStatus
		proxies[i].QualityScore = info.QualityScore
		proxies[i].QualityGrade = info.QualityGrade
		proxies[i].QualitySummary = info.QualitySummary
		proxies[i].QualityChecked = info.QualityCheckedAt
REDACTED
REDACTED

func (s *adminServiceImpl) saveProxyLatency(ctx context.Context, proxyID int64, info *ProxyLatencyInfo) {
	if s.proxyLatencyCache == nil || info == nil {
		return
REDACTED

	merged := *info
	if latencies, err := s.proxyLatencyCache.GetProxyLatencies(ctx, []int64{proxyIDREDACTED); err == nil {
		if existing := latencies[proxyID]; existing != nil {
			if merged.QualityCheckedAt == nil &&
				merged.QualityScore == nil &&
				merged.QualityGrade == "" &&
				merged.QualityStatus == "" &&
				merged.QualitySummary == "" &&
				merged.QualityCFRay == "" {
				merged.QualityStatus = existing.QualityStatus
				merged.QualityScore = existing.QualityScore
				merged.QualityGrade = existing.QualityGrade
				merged.QualitySummary = existing.QualitySummary
				merged.QualityCheckedAt = existing.QualityCheckedAt
				merged.QualityCFRay = existing.QualityCFRay
		REDACTED
	REDACTED
REDACTED

	if err := s.proxyLatencyCache.SetProxyLatency(ctx, proxyID, &merged); err != nil {
		logger.LegacyPrintf("service.admin", "Warning: store proxy latency cache failed: %v", err)
REDACTED
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

func (s *adminServiceImpl) ResetAccountQuota(ctx context.Context, id int64) error {
	return s.accountRepo.ResetQuotaUsed(ctx, id)
REDACTED

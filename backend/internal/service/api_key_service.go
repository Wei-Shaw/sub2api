package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/infrastructure/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

var (
	ErrApiKeyNotFound     = infraerrors.NotFound("API_KEY_NOT_FOUND", "api key not found")
	ErrGroupNotAllowed    = infraerrors.Forbidden("GROUP_NOT_ALLOWED", "user is not allowed to bind this group")
	ErrApiKeyExists       = infraerrors.Conflict("API_KEY_EXISTS", "api key already exists")
	ErrApiKeyTooShort     = infraerrors.BadRequest("API_KEY_TOO_SHORT", "api key must be at least 16 characters")
	ErrApiKeyInvalidChars = infraerrors.BadRequest("API_KEY_INVALID_CHARS", "api key can only contain letters, numbers, underscores, and hyphens")
	ErrApiKeyRateLimited  = infraerrors.TooManyRequests("API_KEY_RATE_LIMITED", "too many failed attempts, please try again later")
)

const (
	apiKeyMaxErrorsPerHour = 20
)

type ApiKeyRepository interface {
	Create(ctx context.Context, key *ApiKey) error
	GetByID(ctx context.Context, id int64) (*ApiKey, error)
	// GetOwnerID 仅获取 API Key 的所有者 ID，用于删除前的轻量级权限验证
	GetOwnerID(ctx context.Context, id int64) (int64, error)
	GetByKey(ctx context.Context, key string) (*ApiKey, error)
	Update(ctx context.Context, key *ApiKey) error
	Delete(ctx context.Context, id int64) error

	ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams) ([]ApiKey, *pagination.PaginationResult, error)
	VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error)
	CountByUserID(ctx context.Context, userID int64) (int64, error)
	ExistsByKey(ctx context.Context, key string) (bool, error)
	ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]ApiKey, *pagination.PaginationResult, error)
	SearchApiKeys(ctx context.Context, userID int64, keyword string, limit int) ([]ApiKey, error)
	ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error)
	CountByGroupID(ctx context.Context, groupID int64) (int64, error)
REDACTED

// ApiKeyCache defines cache operations for API key service
type ApiKeyCache interface {
	GetCreateAttemptCount(ctx context.Context, userID int64) (int, error)
	IncrementCreateAttemptCount(ctx context.Context, userID int64) error
	DeleteCreateAttemptCount(ctx context.Context, userID int64) error

	IncrementDailyUsage(ctx context.Context, apiKey string) error
	SetDailyUsageExpiry(ctx context.Context, apiKey string, ttl time.Duration) error
REDACTED

// CreateApiKeyRequest 创建API Key请求
type CreateApiKeyRequest struct {
	Name      string  `json:"name"`
	GroupID   *int64  `json:"group_id"`
	CustomKey *string `json:"custom_key"` // 可选的自定义key
REDACTED

// UpdateApiKeyRequest 更新API Key请求
type UpdateApiKeyRequest struct {
	Name    *string `json:"name"`
	GroupID *int64  `json:"group_id"`
	Status  *string `json:"status"`
REDACTED

// ApiKeyService API Key服务
type ApiKeyService struct {
	apiKeyRepo  ApiKeyRepository
	userRepo    UserRepository
	groupRepo   GroupRepository
	userSubRepo UserSubscriptionRepository
	cache       ApiKeyCache
	cfg         *config.Config
REDACTED

// NewApiKeyService 创建API Key服务实例
func NewApiKeyService(
	apiKeyRepo ApiKeyRepository,
	userRepo UserRepository,
	groupRepo GroupRepository,
	userSubRepo UserSubscriptionRepository,
	cache ApiKeyCache,
	cfg *config.Config,
) *ApiKeyService {
	return &ApiKeyService{
		apiKeyRepo:  apiKeyRepo,
		userRepo:    userRepo,
		groupRepo:   groupRepo,
		userSubRepo: userSubRepo,
		cache:       cache,
		cfg:         cfg,
REDACTED
REDACTED

// GenerateKey 生成随机API Key
func (s *ApiKeyService) GenerateKey() (string, error) {
	// 生成32字节随机数据
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
REDACTED

	// 转换为十六进制字符串并添加前缀
	prefix := s.cfg.Default.ApiKeyPrefix
	if prefix == "" {
		prefix = "sk-"
REDACTED

	key := prefix + hex.EncodeToString(bytes)
	return key, nil
REDACTED

// ValidateCustomKey 验证自定义API Key格式
func (s *ApiKeyService) ValidateCustomKey(key string) error {
	// 检查长度
	if len(key) < 16 {
		return ErrApiKeyTooShort
REDACTED

	// 检查字符：只允许字母、数字、下划线、连字符
	for _, c := range key {
		if (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '_' || c == '-' {
			continue
	REDACTED
		return ErrApiKeyInvalidChars
REDACTED

	return nil
REDACTED

// checkApiKeyRateLimit 检查用户创建自定义Key的错误次数是否超限
func (s *ApiKeyService) checkApiKeyRateLimit(ctx context.Context, userID int64) error {
	if s.cache == nil {
		return nil
REDACTED

	count, err := s.cache.GetCreateAttemptCount(ctx, userID)
	if err != nil {
		// Redis 出错时不阻止用户操作
		return nil
REDACTED

	if count >= apiKeyMaxErrorsPerHour {
		return ErrApiKeyRateLimited
REDACTED

	return nil
REDACTED

// incrementApiKeyErrorCount 增加用户创建自定义Key的错误计数
func (s *ApiKeyService) incrementApiKeyErrorCount(ctx context.Context, userID int64) {
	if s.cache == nil {
		return
REDACTED

	_ = s.cache.IncrementCreateAttemptCount(ctx, userID)
REDACTED

// canUserBindGroup 检查用户是否可以绑定指定分组
// 对于订阅类型分组：检查用户是否有有效订阅
// 对于标准类型分组：使用原有的 AllowedGroups 和 IsExclusive 逻辑
func (s *ApiKeyService) canUserBindGroup(ctx context.Context, user *User, group *Group) bool {
	// 订阅类型分组：需要有效订阅
	if group.IsSubscriptionType() {
		_, err := s.userSubRepo.GetActiveByUserIDAndGroupID(ctx, user.ID, group.ID)
		return err == nil // 有有效订阅则允许
REDACTED
	// 标准类型分组：使用原有逻辑
	return user.CanBindGroup(group.ID, group.IsExclusive)
REDACTED

// Create 创建API Key
func (s *ApiKeyService) Create(ctx context.Context, userID int64, req CreateApiKeyRequest) (*ApiKey, error) {
	// 验证用户存在
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
REDACTED

	// 验证分组权限（如果指定了分组）
	if req.GroupID != nil {
		group, err := s.groupRepo.GetByID(ctx, *req.GroupID)
		if err != nil {
			return nil, fmt.Errorf("get group: %w", err)
	REDACTED

		// 检查用户是否可以绑定该分组
		if !s.canUserBindGroup(ctx, user, group) {
			return nil, ErrGroupNotAllowed
	REDACTED
REDACTED

	var key string

	// 判断是否使用自定义Key
	if req.CustomKey != nil && *req.CustomKey != "" {
		// 检查限流（仅对自定义key进行限流）
		if err := s.checkApiKeyRateLimit(ctx, userID); err != nil {
			return nil, err
	REDACTED

		// 验证自定义Key格式
		if err := s.ValidateCustomKey(*req.CustomKey); err != nil {
			return nil, err
	REDACTED

		// 检查Key是否已存在
		exists, err := s.apiKeyRepo.ExistsByKey(ctx, *req.CustomKey)
		if err != nil {
			return nil, fmt.Errorf("check key exists: %w", err)
	REDACTED
		if exists {
			// Key已存在，增加错误计数
			s.incrementApiKeyErrorCount(ctx, userID)
			return nil, ErrApiKeyExists
	REDACTED

		key = *req.CustomKey
REDACTED else {
		// 生成随机API Key
		var err error
		key, err = s.GenerateKey()
		if err != nil {
			return nil, fmt.Errorf("generate key: %w", err)
	REDACTED
REDACTED

	// 创建API Key记录
	apiKey := &ApiKey{
		UserID:  userID,
		Key:     key,
		Name:    req.Name,
		GroupID: req.GroupID,
		Status:  StatusActive,
REDACTED

	if err := s.apiKeyRepo.Create(ctx, apiKey); err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
REDACTED

	return apiKey, nil
REDACTED

// List 获取用户的API Key列表
func (s *ApiKeyService) List(ctx context.Context, userID int64, params pagination.PaginationParams) ([]ApiKey, *pagination.PaginationResult, error) {
	keys, pagination, err := s.apiKeyRepo.ListByUserID(ctx, userID, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list api keys: %w", err)
REDACTED
	return keys, pagination, nil
REDACTED

func (s *ApiKeyService) VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error) {
	if len(apiKeyIDs) == 0 {
		return []int64{REDACTED, nil
REDACTED

	validIDs, err := s.apiKeyRepo.VerifyOwnership(ctx, userID, apiKeyIDs)
	if err != nil {
		return nil, fmt.Errorf("verify api key ownership: %w", err)
REDACTED
	return validIDs, nil
REDACTED

// GetByID 根据ID获取API Key
func (s *ApiKeyService) GetByID(ctx context.Context, id int64) (*ApiKey, error) {
	apiKey, err := s.apiKeyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get api key: %w", err)
REDACTED
	return apiKey, nil
REDACTED

// GetByKey 根据Key字符串获取API Key（用于认证）
func (s *ApiKeyService) GetByKey(ctx context.Context, key string) (*ApiKey, error) {
	// 尝试从Redis缓存获取
	cacheKey := fmt.Sprintf("apikey:%s", key)

	// 这里可以添加Redis缓存逻辑，暂时直接查询数据库
	apiKey, err := s.apiKeyRepo.GetByKey(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get api key: %w", err)
REDACTED

	// 缓存到Redis（可选，TTL设置为5分钟）
	if s.cache != nil {
		// 这里可以序列化并缓存API Key
		_ = cacheKey // 使用变量避免未使用错误
REDACTED

	return apiKey, nil
REDACTED

// Update 更新API Key
func (s *ApiKeyService) Update(ctx context.Context, id int64, userID int64, req UpdateApiKeyRequest) (*ApiKey, error) {
	apiKey, err := s.apiKeyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get api key: %w", err)
REDACTED

	// 验证所有权
	if apiKey.UserID != userID {
		return nil, ErrInsufficientPerms
REDACTED

	// 更新字段
	if req.Name != nil {
		apiKey.Name = *req.Name
REDACTED

	if req.GroupID != nil {
		// 验证分组权限
		user, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("get user: %w", err)
	REDACTED

		group, err := s.groupRepo.GetByID(ctx, *req.GroupID)
		if err != nil {
			return nil, fmt.Errorf("get group: %w", err)
	REDACTED

		if !s.canUserBindGroup(ctx, user, group) {
			return nil, ErrGroupNotAllowed
	REDACTED

		apiKey.GroupID = req.GroupID
REDACTED

	if req.Status != nil {
		apiKey.Status = *req.Status
		// 如果状态改变，清除Redis缓存
		if s.cache != nil {
			_ = s.cache.DeleteCreateAttemptCount(ctx, apiKey.UserID)
	REDACTED
REDACTED

	if err := s.apiKeyRepo.Update(ctx, apiKey); err != nil {
		return nil, fmt.Errorf("update api key: %w", err)
REDACTED

	return apiKey, nil
REDACTED

// Delete 删除API Key
// 优化：使用 GetOwnerID 替代 GetByID 进行权限验证，
// 避免加载完整 ApiKey 对象及其关联数据（User、Group），提升删除操作的性能
func (s *ApiKeyService) Delete(ctx context.Context, id int64, userID int64) error {
	// 仅获取所有者 ID 用于权限验证，而非加载完整对象
	ownerID, err := s.apiKeyRepo.GetOwnerID(ctx, id)
	if err != nil {
		return fmt.Errorf("get api key: %w", err)
REDACTED

	// 验证当前用户是否为该 API Key 的所有者
	if ownerID != userID {
		return ErrInsufficientPerms
REDACTED

	// 清除Redis缓存（使用 ownerID 而非 apiKey.UserID）
	if s.cache != nil {
		_ = s.cache.DeleteCreateAttemptCount(ctx, ownerID)
REDACTED

	if err := s.apiKeyRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete api key: %w", err)
REDACTED

	return nil
REDACTED

// ValidateKey 验证API Key是否有效（用于认证中间件）
func (s *ApiKeyService) ValidateKey(ctx context.Context, key string) (*ApiKey, *User, error) {
	// 获取API Key
	apiKey, err := s.GetByKey(ctx, key)
	if err != nil {
		return nil, nil, err
REDACTED

	// 检查API Key状态
	if !apiKey.IsActive() {
		return nil, nil, infraerrors.Unauthorized("API_KEY_INACTIVE", "api key is not active")
REDACTED

	// 获取用户信息
	user, err := s.userRepo.GetByID(ctx, apiKey.UserID)
	if err != nil {
		return nil, nil, fmt.Errorf("get user: %w", err)
REDACTED

	// 检查用户状态
	if !user.IsActive() {
		return nil, nil, ErrUserNotActive
REDACTED

	return apiKey, user, nil
REDACTED

// IncrementUsage 增加API Key使用次数（可选：用于统计）
func (s *ApiKeyService) IncrementUsage(ctx context.Context, keyID int64) error {
	// 使用Redis计数器
	if s.cache != nil {
		cacheKey := fmt.Sprintf("apikey:usage:%d:%s", keyID, timezone.Now().Format("2006-01-02"))
		if err := s.cache.IncrementDailyUsage(ctx, cacheKey); err != nil {
			return fmt.Errorf("increment usage: %w", err)
	REDACTED
		// 设置24小时过期
		_ = s.cache.SetDailyUsageExpiry(ctx, cacheKey, 24*time.Hour)
REDACTED
	return nil
REDACTED

// GetAvailableGroups 获取用户有权限绑定的分组列表
// 返回用户可以选择的分组：
// - 标准类型分组：公开的（非专属）或用户被明确允许的
// - 订阅类型分组：用户有有效订阅的
func (s *ApiKeyService) GetAvailableGroups(ctx context.Context, userID int64) ([]Group, error) {
	// 获取用户信息
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
REDACTED

	// 获取所有活跃分组
	allGroups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
REDACTED

	// 获取用户的所有有效订阅
	activeSubscriptions, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list active subscriptions: %w", err)
REDACTED

	// 构建订阅分组 ID 集合
	subscribedGroupIDs := make(map[int64]bool)
	for _, sub := range activeSubscriptions {
		subscribedGroupIDs[sub.GroupID] = true
REDACTED

	// 过滤出用户有权限的分组
	availableGroups := make([]Group, 0)
	for _, group := range allGroups {
		if s.canUserBindGroupInternal(user, &group, subscribedGroupIDs) {
			availableGroups = append(availableGroups, group)
	REDACTED
REDACTED

	return availableGroups, nil
REDACTED

// canUserBindGroupInternal 内部方法，检查用户是否可以绑定分组（使用预加载的订阅数据）
func (s *ApiKeyService) canUserBindGroupInternal(user *User, group *Group, subscribedGroupIDs map[int64]bool) bool {
	// 订阅类型分组：需要有效订阅
	if group.IsSubscriptionType() {
		return subscribedGroupIDs[group.ID]
REDACTED
	// 标准类型分组：使用原有逻辑
	return user.CanBindGroup(group.ID, group.IsExclusive)
REDACTED

func (s *ApiKeyService) SearchApiKeys(ctx context.Context, userID int64, keyword string, limit int) ([]ApiKey, error) {
	keys, err := s.apiKeyRepo.SearchApiKeys(ctx, userID, keyword, limit)
	if err != nil {
		return nil, fmt.Errorf("search api keys: %w", err)
REDACTED
	return keys, nil
REDACTED

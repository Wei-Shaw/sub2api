package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sub2api/internal/config"
	"sub2api/internal/model"
	"sub2api/internal/pkg/pagination"
	"sub2api/internal/pkg/timezone"
	"sub2api/internal/service/ports"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
	ErrApiKeyNotFound     = errors.New("api key not found")
	ErrGroupNotAllowed    = errors.New("user is not allowed to bind this group")
	ErrApiKeyExists       = errors.New("api key already exists")
	ErrApiKeyTooShort     = errors.New("api key must be at least 16 characters")
	ErrApiKeyInvalidChars = errors.New("api key can only contain letters, numbers, underscores, and hyphens")
	ErrApiKeyRateLimited  = errors.New("too many failed attempts, please try again later")
)

const (
	apiKeyMaxErrorsPerHour = 20
)

// CreateApiKeyRequest 创建API Key请求
type CreateApiKeyRequest struct {
	Name      string  `json:"name"`
	GroupID   *int64  `json:"group_id"`
	CustomKey *string `json:"custom_key"` // 可选的自定义key
}

// UpdateApiKeyRequest 更新API Key请求
type UpdateApiKeyRequest struct {
	Name    *string `json:"name"`
	GroupID *int64  `json:"group_id"`
	Status  *string `json:"status"`
}

// ApiKeyService API Key服务
type ApiKeyService struct {
	apiKeyRepo  ports.ApiKeyRepository
	userRepo    ports.UserRepository
	groupRepo   ports.GroupRepository
	userSubRepo ports.UserSubscriptionRepository
	cache       ports.ApiKeyCache
	cfg         *config.Config
}

// NewApiKeyService 创建API Key服务实例
func NewApiKeyService(
	apiKeyRepo ports.ApiKeyRepository,
	userRepo ports.UserRepository,
	groupRepo ports.GroupRepository,
	userSubRepo ports.UserSubscriptionRepository,
	cache ports.ApiKeyCache,
	cfg *config.Config,
) *ApiKeyService {
	return &ApiKeyService{
		apiKeyRepo:  apiKeyRepo,
		userRepo:    userRepo,
		groupRepo:   groupRepo,
		userSubRepo: userSubRepo,
		cache:       cache,
		cfg:         cfg,
	}
}

func (s *ApiKeyService) apiKeyHMACSecret() string {
	// 独立密钥优先，否则回退到 JWT secret，避免空值导致无法认证。
	if s.cfg.Security.ApiKeyHMACSecret != "" {
		return s.cfg.Security.ApiKeyHMACSecret
	}
	return s.cfg.JWT.Secret
}

func (s *ApiKeyService) hashKey(key string) (string, error) {
	secret := s.apiKeyHMACSecret()
	if secret == "" {
		return "", errors.New("api key hmac secret is empty")
	}
	// 使用 HMAC-SHA256，避免直接保存明文 key。
	mac := hmac.New(sha256.New, []byte(secret))
	_, err := mac.Write([]byte(key))
	if err != nil {
		return "", fmt.Errorf("hash key: %w", err)
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (s *ApiKeyService) last4(key string) string {
	if len(key) <= 4 {
		return key
	}
	return key[len(key)-4:]
}

func (s *ApiKeyService) MaskKey(fullKey string, apiKey *model.ApiKey) string {
	last4 := ""
	if fullKey != "" {
		last4 = s.last4(fullKey)
	} else if apiKey != nil {
		if apiKey.KeyLast4 != "" {
			last4 = apiKey.KeyLast4
		} else if apiKey.Key != nil {
			last4 = s.last4(*apiKey.Key)
		}
	}

	prefix := s.cfg.Default.ApiKeyPrefix
	if prefix == "" {
		prefix = "sk-"
	}
	if last4 == "" {
		return prefix + "..."
	}
	return prefix + "..." + last4
}

// GenerateKey 生成随机API Key
func (s *ApiKeyService) GenerateKey() (string, error) {
	// 生成32字节随机数据
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}

	// 转换为十六进制字符串并添加前缀
	prefix := s.cfg.Default.ApiKeyPrefix
	if prefix == "" {
		prefix = "sk-"
	}

	key := prefix + hex.EncodeToString(bytes)
	return key, nil
}

// ValidateCustomKey 验证自定义API Key格式
func (s *ApiKeyService) ValidateCustomKey(key string) error {
	// 检查长度
	if len(key) < 16 {
		return ErrApiKeyTooShort
	}

	// 检查字符：只允许字母、数字、下划线、连字符
	for _, c := range key {
		if (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '_' || c == '-' {
			continue
		}
		return ErrApiKeyInvalidChars
	}

	return nil
}

// checkApiKeyRateLimit 检查用户创建自定义Key的错误次数是否超限
func (s *ApiKeyService) checkApiKeyRateLimit(ctx context.Context, userID int64) error {
	if s.cache == nil {
		return nil
	}

	count, err := s.cache.GetCreateAttemptCount(ctx, userID)
	if err != nil && !errors.Is(err, redis.Nil) {
		// Redis 出错时不阻止用户操作
		return nil
	}

	if count >= apiKeyMaxErrorsPerHour {
		return ErrApiKeyRateLimited
	}

	return nil
}

// incrementApiKeyErrorCount 增加用户创建自定义Key的错误计数
func (s *ApiKeyService) incrementApiKeyErrorCount(ctx context.Context, userID int64) {
	if s.cache == nil {
		return
	}

	_ = s.cache.IncrementCreateAttemptCount(ctx, userID)
}

// canUserBindGroup 检查用户是否可以绑定指定分组
// 对于订阅类型分组：检查用户是否有有效订阅
// 对于标准类型分组：使用原有的 AllowedGroups 和 IsExclusive 逻辑
func (s *ApiKeyService) canUserBindGroup(ctx context.Context, user *model.User, group *model.Group) bool {
	// 订阅类型分组：需要有效订阅
	if group.IsSubscriptionType() {
		_, err := s.userSubRepo.GetActiveByUserIDAndGroupID(ctx, user.ID, group.ID)
		return err == nil // 有有效订阅则允许
	}
	// 标准类型分组：使用原有逻辑
	return user.CanBindGroup(group.ID, group.IsExclusive)
}

// Create 创建API Key
func (s *ApiKeyService) Create(ctx context.Context, userID int64, req CreateApiKeyRequest) (*model.ApiKey, string, error) {
	// 验证用户存在
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrUserNotFound
		}
		return nil, "", fmt.Errorf("get user: %w", err)
	}

	// 验证分组权限（如果指定了分组）
	if req.GroupID != nil {
		group, err := s.groupRepo.GetByID(ctx, *req.GroupID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, "", errors.New("group not found")
			}
			return nil, "", fmt.Errorf("get group: %w", err)
		}

		// 检查用户是否可以绑定该分组
		if !s.canUserBindGroup(ctx, user, group) {
			return nil, "", ErrGroupNotAllowed
		}
	}

	var key string

	// 判断是否使用自定义Key
	if req.CustomKey != nil && *req.CustomKey != "" {
		// 检查限流（仅对自定义key进行限流）
		if err := s.checkApiKeyRateLimit(ctx, userID); err != nil {
			return nil, "", err
		}

		// 验证自定义Key格式
		if err := s.ValidateCustomKey(*req.CustomKey); err != nil {
			return nil, "", err
		}

		// 检查Key是否已存在（哈希存储 + 兼容旧明文列）
		keyHash, err := s.hashKey(*req.CustomKey)
		if err != nil {
			return nil, "", fmt.Errorf("hash api key: %w", err)
		}
		exists, err := s.apiKeyRepo.ExistsByHash(ctx, keyHash)
		if err != nil {
			return nil, "", fmt.Errorf("check key exists: %w", err)
		}
		legacyExists, err := s.apiKeyRepo.ExistsByKey(ctx, *req.CustomKey)
		if err != nil {
			return nil, "", fmt.Errorf("check legacy key exists: %w", err)
		}
		if exists || legacyExists {
			// Key已存在，增加错误计数
			s.incrementApiKeyErrorCount(ctx, userID)
			return nil, "", ErrApiKeyExists
		}

		key = *req.CustomKey
	} else {
		// 生成随机API Key
		var err error
		key, err = s.GenerateKey()
		if err != nil {
			return nil, "", fmt.Errorf("generate key: %w", err)
		}
	}

	keyHash, err := s.hashKey(key)
	if err != nil {
		return nil, "", fmt.Errorf("hash api key: %w", err)
	}
	last4 := s.last4(key)

	// 创建API Key记录
	apiKey := &model.ApiKey{
		UserID:   userID,
		KeyHash:  &keyHash,
		KeyLast4: last4,
		Name:     req.Name,
		GroupID:  req.GroupID,
		Status:   model.StatusActive,
	}

	if err := s.apiKeyRepo.Create(ctx, apiKey); err != nil {
		return nil, "", fmt.Errorf("create api key: %w", err)
	}

	return apiKey, key, nil
}

// List 获取用户的API Key列表
func (s *ApiKeyService) List(ctx context.Context, userID int64, params pagination.PaginationParams) ([]model.ApiKey, *pagination.PaginationResult, error) {
	keys, pagination, err := s.apiKeyRepo.ListByUserID(ctx, userID, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list api keys: %w", err)
	}
	return keys, pagination, nil
}

// GetByID 根据ID获取API Key
func (s *ApiKeyService) GetByID(ctx context.Context, id int64) (*model.ApiKey, error) {
	apiKey, err := s.apiKeyRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrApiKeyNotFound
		}
		return nil, fmt.Errorf("get api key: %w", err)
	}
	return apiKey, nil
}

// GetByKey 根据Key字符串获取API Key（用于认证）
func (s *ApiKeyService) GetByKey(ctx context.Context, key string) (*model.ApiKey, error) {
	// 尝试从Redis缓存获取
	cacheKey := fmt.Sprintf("apikey:%s", key)

	// 这里可以添加Redis缓存逻辑，暂时直接查询数据库
	keyHash, err := s.hashKey(key)
	if err != nil {
		return nil, fmt.Errorf("hash api key: %w", err)
	}
	apiKey, err := s.apiKeyRepo.GetByHash(ctx, keyHash)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get api key: %w", err)
		}
		// 旧版明文 key 兜底查找，并在成功后迁移为哈希存储。
		apiKey, err = s.apiKeyRepo.GetByKey(ctx, key)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrApiKeyNotFound
			}
			return nil, fmt.Errorf("get api key: %w", err)
		}
		last4 := s.last4(key)
		if err := s.apiKeyRepo.UpdateKeyMaterial(ctx, apiKey.ID, &keyHash, last4, nil); err != nil {
			log.Printf("failed to migrate legacy api key %d: %v", apiKey.ID, err)
		} else {
			apiKey.KeyHash = &keyHash
			apiKey.KeyLast4 = last4
			apiKey.Key = nil
		}
	}

	// 缓存到Redis（可选，TTL设置为5分钟）
	if s.cache != nil {
		// 这里可以序列化并缓存API Key
		_ = cacheKey // 使用变量避免未使用错误
	}

	return apiKey, nil
}

// Update 更新API Key
func (s *ApiKeyService) Update(ctx context.Context, id int64, userID int64, req UpdateApiKeyRequest) (*model.ApiKey, error) {
	apiKey, err := s.apiKeyRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrApiKeyNotFound
		}
		return nil, fmt.Errorf("get api key: %w", err)
	}

	// 验证所有权
	if apiKey.UserID != userID {
		return nil, ErrInsufficientPerms
	}

	// 更新字段
	if req.Name != nil {
		apiKey.Name = *req.Name
	}

	if req.GroupID != nil {
		// 验证分组权限
		user, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("get user: %w", err)
		}

		group, err := s.groupRepo.GetByID(ctx, *req.GroupID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("group not found")
			}
			return nil, fmt.Errorf("get group: %w", err)
		}

		if !s.canUserBindGroup(ctx, user, group) {
			return nil, ErrGroupNotAllowed
		}

		apiKey.GroupID = req.GroupID
	}

	if req.Status != nil {
		apiKey.Status = *req.Status
		// 如果状态改变，清除Redis缓存
		if s.cache != nil {
			_ = s.cache.DeleteCreateAttemptCount(ctx, apiKey.UserID)
		}
	}

	if err := s.apiKeyRepo.Update(ctx, apiKey); err != nil {
		return nil, fmt.Errorf("update api key: %w", err)
	}

	return apiKey, nil
}

// Delete 删除API Key
func (s *ApiKeyService) Delete(ctx context.Context, id int64, userID int64) error {
	apiKey, err := s.apiKeyRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrApiKeyNotFound
		}
		return fmt.Errorf("get api key: %w", err)
	}

	// 验证所有权
	if apiKey.UserID != userID {
		return ErrInsufficientPerms
	}

	// 清除Redis缓存
	if s.cache != nil {
		_ = s.cache.DeleteCreateAttemptCount(ctx, apiKey.UserID)
	}

	if err := s.apiKeyRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}

	return nil
}

// ValidateKey 验证API Key是否有效（用于认证中间件）
func (s *ApiKeyService) ValidateKey(ctx context.Context, key string) (*model.ApiKey, *model.User, error) {
	// 获取API Key
	apiKey, err := s.GetByKey(ctx, key)
	if err != nil {
		return nil, nil, err
	}

	// 检查API Key状态
	if !apiKey.IsActive() {
		return nil, nil, errors.New("api key is not active")
	}

	// 获取用户信息
	user, err := s.userRepo.GetByID(ctx, apiKey.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrUserNotFound
		}
		return nil, nil, fmt.Errorf("get user: %w", err)
	}

	// 检查用户状态
	if !user.IsActive() {
		return nil, nil, ErrUserNotActive
	}

	return apiKey, user, nil
}

// IncrementUsage 增加API Key使用次数（可选：用于统计）
func (s *ApiKeyService) IncrementUsage(ctx context.Context, keyID int64) error {
	// 使用Redis计数器
	if s.cache != nil {
		cacheKey := fmt.Sprintf("apikey:usage:%d:%s", keyID, timezone.Now().Format("2006-01-02"))
		if err := s.cache.IncrementDailyUsage(ctx, cacheKey); err != nil {
			return fmt.Errorf("increment usage: %w", err)
		}
		// 设置24小时过期
		_ = s.cache.SetDailyUsageExpiry(ctx, cacheKey, 24*time.Hour)
	}
	return nil
}

// GetAvailableGroups 获取用户有权限绑定的分组列表
// 返回用户可以选择的分组：
// - 标准类型分组：公开的（非专属）或用户被明确允许的
// - 订阅类型分组：用户有有效订阅的
func (s *ApiKeyService) GetAvailableGroups(ctx context.Context, userID int64) ([]model.Group, error) {
	// 获取用户信息
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	// 获取所有活跃分组
	allGroups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}

	// 获取用户的所有有效订阅
	activeSubscriptions, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("list active subscriptions: %w", err)
	}

	// 构建订阅分组 ID 集合
	subscribedGroupIDs := make(map[int64]bool)
	for _, sub := range activeSubscriptions {
		subscribedGroupIDs[sub.GroupID] = true
	}

	// 过滤出用户有权限的分组
	availableGroups := make([]model.Group, 0)
	for _, group := range allGroups {
		if s.canUserBindGroupInternal(user, &group, subscribedGroupIDs) {
			availableGroups = append(availableGroups, group)
		}
	}

	return availableGroups, nil
}

// canUserBindGroupInternal 内部方法，检查用户是否可以绑定分组（使用预加载的订阅数据）
func (s *ApiKeyService) canUserBindGroupInternal(user *model.User, group *model.Group, subscribedGroupIDs map[int64]bool) bool {
	// 订阅类型分组：需要有效订阅
	if group.IsSubscriptionType() {
		return subscribedGroupIDs[group.ID]
	}
	// 标准类型分组：使用原有逻辑
	return user.CanBindGroup(group.ID, group.IsExclusive)
}

func (s *ApiKeyService) SearchApiKeys(ctx context.Context, userID int64, keyword string, limit int) ([]model.ApiKey, error) {
	keys, err := s.apiKeyRepo.SearchApiKeys(ctx, userID, keyword, limit)
	if err != nil {
		return nil, fmt.Errorf("search api keys: %w", err)
	}
	return keys, nil
}

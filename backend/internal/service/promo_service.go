package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var (
	ErrPromoCodeNotFound    = infraerrors.NotFound("PROMO_CODE_NOT_FOUND", "promo code not found")
	ErrPromoCodeExpired     = infraerrors.BadRequest("PROMO_CODE_EXPIRED", "promo code has expired")
	ErrPromoCodeDisabled    = infraerrors.BadRequest("PROMO_CODE_DISABLED", "promo code is disabled")
	ErrPromoCodeMaxUsed     = infraerrors.BadRequest("PROMO_CODE_MAX_USED", "promo code has reached maximum uses")
	ErrPromoCodeAlreadyUsed = infraerrors.Conflict("PROMO_CODE_ALREADY_USED", "you have already used this promo code")
	ErrPromoCodeInvalid     = infraerrors.BadRequest("PROMO_CODE_INVALID", "invalid promo code")
)

// PromoService 优惠码服务
type PromoService struct {
	promoRepo            PromoCodeRepository
	userRepo             UserRepository
	billingCacheService  *BillingCacheService
	entClient            *dbent.Client
	authCacheInvalidator APIKeyAuthCacheInvalidator
}

// NewPromoService 创建优惠码服务实例
func NewPromoService(
	promoRepo PromoCodeRepository,
	userRepo UserRepository,
	billingCacheService *BillingCacheService,
	entClient *dbent.Client,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
) *PromoService {
	return &PromoService{
		promoRepo:            promoRepo,
		userRepo:             userRepo,
		billingCacheService:  billingCacheService,
		entClient:            entClient,
		authCacheInvalidator: authCacheInvalidator,
	}
}

// ValidatePromoCode 验证优惠码（注册前调用）
// 返回 nil, nil 表示空码（不报错）
func (s *PromoService) ValidatePromoCode(ctx context.Context, code string) (*PromoCode, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, nil // 空码不报错，直接返回
	}

	promoCode, err := s.promoRepo.GetByCode(ctx, code)
	if err != nil {
		// 保留原始错误类型，不要统一映射为 NotFound
		return nil, err
	}

	if err := s.validatePromoCodeStatus(promoCode); err != nil {
		return nil, err
	}

	return promoCode, nil
}

// validatePromoCodeStatus 验证优惠码状态
func (s *PromoService) validatePromoCodeStatus(promoCode *PromoCode) error {
	if !promoCode.CanUse() {
		if promoCode.IsExpired() {
			return ErrPromoCodeExpired
		}
		if promoCode.Status == PromoCodeStatusDisabled {
			return ErrPromoCodeDisabled
		}
		if promoCode.MaxUses > 0 && promoCode.UsedCount >= promoCode.MaxUses {
			return ErrPromoCodeMaxUsed
		}
		return ErrPromoCodeInvalid
	}
	return nil
}

// ApplyPromoCode 应用优惠码（注册成功后调用）
// 使用事务和行锁确保并发安全
func (s *PromoService) ApplyPromoCode(ctx context.Context, userID int64, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil
	}

	// 开启事务
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)

	// 在事务中获取并锁定优惠码记录（FOR UPDATE）
	promoCode, err := s.promoRepo.GetByCodeForUpdate(txCtx, code)
	if err != nil {
		return err
	}

	// 在事务中验证优惠码状态
	if err := s.validatePromoCodeStatus(promoCode); err != nil {
		return err
	}

	// 在事务中检查用户是否已使用过此优惠码
	existing, err := s.promoRepo.GetUsageByPromoCodeAndUser(txCtx, promoCode.ID, userID)
	if err != nil {
		return fmt.Errorf("check existing usage: %w", err)
	}
	if existing != nil {
		return ErrPromoCodeAlreadyUsed
	}

	// 增加用户余额
	if err := s.userRepo.UpdateBalance(txCtx, userID, promoCode.BonusAmount); err != nil {
		return fmt.Errorf("update user balance: %w", err)
	}

	// 创建使用记录
	usage := &PromoCodeUsage{
		PromoCodeID: promoCode.ID,
		UserID:      userID,
		BonusAmount: promoCode.BonusAmount,
		UsedAt:      time.Now(),
	}
	if err := s.promoRepo.CreateUsage(txCtx, usage); err != nil {
		return fmt.Errorf("create usage record: %w", err)
	}

	// 增加使用次数
	if err := s.promoRepo.IncrementUsedCount(txCtx, promoCode.ID); err != nil {
		return fmt.Errorf("increment used count: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	s.invalidatePromoCaches(ctx, userID, promoCode.BonusAmount)

	// 失效余额缓存
	if s.billingCacheService != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.billingCacheService.InvalidateUserBalance(cacheCtx, userID)
		}()
	}

	return nil
}

func (s *PromoService) invalidatePromoCaches(ctx context.Context, userID int64, bonusAmount float64) {
	if bonusAmount == 0 || s.authCacheInvalidator == nil {
		return
	}
	s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
}

// GenerateRandomCode 生成随机优惠码
func (s *PromoService) GenerateRandomCode() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return strings.ToUpper(hex.EncodeToString(bytes)), nil
}

// Create 创建优惠码
func (s *PromoService) Create(ctx context.Context, input *CreatePromoCodeInput) (*PromoCode, error) {
	code := strings.TrimSpace(input.Code)
	if code == "" {
		// 自动生成
		var err error
		code, err = s.GenerateRandomCode()
		if err != nil {
			return nil, err
		}
	}

	promoCode := &PromoCode{
		Code:        strings.ToUpper(code),
		BonusAmount: input.BonusAmount,
		MaxUses:     input.MaxUses,
		UsedCount:   0,
		Status:      PromoCodeStatusActive,
		ExpiresAt:   input.ExpiresAt,
		Notes:       input.Notes,
	}

	if err := s.promoRepo.Create(ctx, promoCode); err != nil {
		return nil, fmt.Errorf("create promo code: %w", err)
	}

	return promoCode, nil
}

// GetByID 根据ID获取优惠码
func (s *PromoService) GetByID(ctx context.Context, id int64) (*PromoCode, error) {
	code, err := s.promoRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return code, nil
}

// Update 更新优惠码
func (s *PromoService) Update(ctx context.Context, id int64, input *UpdatePromoCodeInput) (*PromoCode, error) {
	promoCode, err := s.promoRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Code != nil {
		promoCode.Code = strings.ToUpper(strings.TrimSpace(*input.Code))
	}
	if input.BonusAmount != nil {
		promoCode.BonusAmount = *input.BonusAmount
	}
	if input.MaxUses != nil {
		promoCode.MaxUses = *input.MaxUses
	}
	if input.Status != nil {
		promoCode.Status = *input.Status
	}
	if input.ExpiresAt != nil {
		promoCode.ExpiresAt = input.ExpiresAt
	}
	if input.Notes != nil {
		promoCode.Notes = *input.Notes
	}

	if err := s.promoRepo.Update(ctx, promoCode); err != nil {
		return nil, fmt.Errorf("update promo code: %w", err)
	}

	return promoCode, nil
}

// Delete 删除优惠码
func (s *PromoService) Delete(ctx context.Context, id int64) error {
	if err := s.promoRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete promo code: %w", err)
	}
	return nil
}

// List 获取优惠码列表
func (s *PromoService) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]PromoCode, *pagination.PaginationResult, error) {
	return s.promoRepo.ListWithFilters(ctx, params, status, search)
}

// ListUsages 获取使用记录
func (s *PromoService) ListUsages(ctx context.Context, promoCodeID int64, params pagination.PaginationParams) ([]PromoCodeUsage, *pagination.PaginationResult, error) {
	return s.promoRepo.ListUsagesByPromoCode(ctx, promoCodeID, params)
}

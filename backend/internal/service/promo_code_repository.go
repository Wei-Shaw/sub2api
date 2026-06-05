package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// PromoCodeRepository 优惠码仓储接口
type PromoCodeRepository interface {
	// 基础 CRUD
	Create(ctx context.Context, code *PromoCode) error
	GetByID(ctx context.Context, id int64) (*PromoCode, error)
	GetByCode(ctx context.Context, code string) (*PromoCode, error)
	GetByCodeForUpdate(ctx context.Context, code string) (*PromoCode, error) // 带行锁的查询，用于并发控制
	Update(ctx context.Context, code *PromoCode) error
	Delete(ctx context.Context, id int64) error

	// 列表查询
	List(ctx context.Context, params pagination.PaginationParams) ([]PromoCode, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, status, search string) ([]PromoCode, *pagination.PaginationResult, error)

	// 使用记录
	CreateUsage(ctx context.Context, usage *PromoCodeUsage) error
	GetUsageByPromoCodeAndUser(ctx context.Context, promoCodeID, userID int64) (*PromoCodeUsage, error)
	ListUsagesByPromoCode(ctx context.Context, promoCodeID int64, params pagination.PaginationParams) ([]PromoCodeUsage, *pagination.PaginationResult, error)

	// 计数操作
	IncrementUsedCount(ctx context.Context, id int64) error
}

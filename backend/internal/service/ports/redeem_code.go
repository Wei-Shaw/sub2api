package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"gorm.io/gorm"
)

type RedeemCodeRepository interface {
	// WithTx 在事务上下文内返回仓库实例。
	WithTx(tx *gorm.DB) RedeemCodeRepository
	// Transaction 统一封装事务执行入口。
	Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error
	Create(ctx context.Context, code *model.RedeemCode) error
	CreateBatch(ctx context.Context, codes []model.RedeemCode) error
	GetByID(ctx context.Context, id int64) (*model.RedeemCode, error)
	GetByCode(ctx context.Context, code string) (*model.RedeemCode, error)
	Update(ctx context.Context, code *model.RedeemCode) error
	Delete(ctx context.Context, id int64) error
	Use(ctx context.Context, id, userID int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]model.RedeemCode, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, codeType, status, search string) ([]model.RedeemCode, *pagination.PaginationResult, error)
	ListByUser(ctx context.Context, userID int64, limit int) ([]model.RedeemCode, error)
}

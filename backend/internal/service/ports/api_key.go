package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type ApiKeyRepository interface {
	Create(ctx context.Context, key *model.ApiKey) error
	GetByID(ctx context.Context, id int64) (*model.ApiKey, error)
	GetByKey(ctx context.Context, key string) (*model.ApiKey, error)
	// GetByHash 使用哈希 key 查询，避免明文检索。
	GetByHash(ctx context.Context, keyHash string) (*model.ApiKey, error)
	Update(ctx context.Context, key *model.ApiKey) error
	// UpdateKeyMaterial 用于旧明文 key 迁移为哈希存储。
	UpdateKeyMaterial(ctx context.Context, id int64, keyHash *string, keyLast4 string, legacyKey *string) error
	Delete(ctx context.Context, id int64) error

	ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams) ([]model.ApiKey, *pagination.PaginationResult, error)
	CountByUserID(ctx context.Context, userID int64) (int64, error)
	ExistsByKey(ctx context.Context, key string) (bool, error)
	// ExistsByHash 用于哈希 key 去重检测。
	ExistsByHash(ctx context.Context, keyHash string) (bool, error)
	ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]model.ApiKey, *pagination.PaginationResult, error)
	SearchApiKeys(ctx context.Context, userID int64, keyword string, limit int) ([]model.ApiKey, error)
	ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error)
	CountByGroupID(ctx context.Context, groupID int64) (int64, error)
}

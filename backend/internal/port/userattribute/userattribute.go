// Package userattribute contains the port interfaces (repository abstractions)
// for the user-attribute bounded context. DTO/value types live in
// internal/domain; this package only owns the persistence port contracts.
package userattribute

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// UserAttributeDefinitionRepository interface for attribute definition persistence.
type UserAttributeDefinitionRepository interface {
	Create(ctx context.Context, def *domain.UserAttributeDefinition) error
	GetByID(ctx context.Context, id int64) (*domain.UserAttributeDefinition, error)
	GetByKey(ctx context.Context, key string) (*domain.UserAttributeDefinition, error)
	Update(ctx context.Context, def *domain.UserAttributeDefinition) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, enabledOnly bool) ([]domain.UserAttributeDefinition, error)
	UpdateDisplayOrders(ctx context.Context, orders map[int64]int) error
	ExistsByKey(ctx context.Context, key string) (bool, error)
}

// UserAttributeValueRepository interface for user attribute value persistence.
type UserAttributeValueRepository interface {
	GetByUserID(ctx context.Context, userID int64) ([]domain.UserAttributeValue, error)
	GetByUserIDs(ctx context.Context, userIDs []int64) ([]domain.UserAttributeValue, error)
	UpsertBatch(ctx context.Context, userID int64, values []domain.UpdateUserAttributeInput) error
	DeleteByAttributeID(ctx context.Context, attributeID int64) error
	DeleteByUserID(ctx context.Context, userID int64) error
}

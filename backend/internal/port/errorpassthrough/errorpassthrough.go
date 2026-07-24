// Package errorpassthrough contains the port interfaces (repository/cache
// abstractions) for the error-passthrough bounded context.
// DTO/value types live in internal/domain; this package only owns the
// persistence/cache port contracts.
package errorpassthrough

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// RuleRepository persists error passthrough rules.
type RuleRepository interface {
	List(ctx context.Context) ([]*domain.ErrorPassthroughRule, error)
	GetByID(ctx context.Context, id int64) (*domain.ErrorPassthroughRule, error)
	Create(ctx context.Context, rule *domain.ErrorPassthroughRule) (*domain.ErrorPassthroughRule, error)
	Update(ctx context.Context, rule *domain.ErrorPassthroughRule) (*domain.ErrorPassthroughRule, error)
	Delete(ctx context.Context, id int64) error
}

// RuleCache caches error passthrough rules across instances.
type RuleCache interface {
	Get(ctx context.Context) ([]*domain.ErrorPassthroughRule, bool)
	Set(ctx context.Context, rules []*domain.ErrorPassthroughRule) error
	Invalidate(ctx context.Context) error
	NotifyUpdate(ctx context.Context) error
	SubscribeUpdates(ctx context.Context, handler func())
}

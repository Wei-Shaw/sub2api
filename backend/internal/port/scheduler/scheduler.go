// Package scheduler contains the port interfaces for the scheduler bounded
// context: the routing-snapshot cache, the event outbox, and the transient
// unschedulable state. Contracts reference only domain types so the repository
// layer can implement them without importing internal/service. The service
// package keeps a type alias to each interface so existing call sites and test
// stubs continue to satisfy the contracts.
package scheduler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// TempUnschedCache 临时不可调度缓存接口。
type TempUnschedCache interface {
	SetTempUnsched(ctx context.Context, accountID int64, state *domain.TempUnschedState) error
	GetTempUnsched(ctx context.Context, accountID int64) (*domain.TempUnschedState, error)
	DeleteTempUnsched(ctx context.Context, accountID int64) error
}

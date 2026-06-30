package service

import (
	"context"
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// 余额 RPC 入参校验错误。
var (
	// ErrLedgerInvalidAmount 金额必须为正。
	ErrLedgerInvalidAmount = infraerrors.BadRequest("LEDGER_INVALID_AMOUNT", "amount must be positive")
	// ErrLedgerDescriptionRequired 原因必填。
	ErrLedgerDescriptionRequired = infraerrors.BadRequest("LEDGER_DESCRIPTION_REQUIRED", "description is required")
)

// BalanceLedgerService 薄账本服务：扣费 / 退费 / 查询余额。
// 扣/退由仓储在单事务内原子完成，提交后同步失效该用户的 Redis 余额缓存。
type BalanceLedgerService struct {
	repo  BalanceLedgerRepository
	cache *BillingCacheService
}

// NewBalanceLedgerService 构造账本服务。
func NewBalanceLedgerService(repo BalanceLedgerRepository, cache *BillingCacheService) *BalanceLedgerService {
	return &BalanceLedgerService{repo: repo, cache: cache}
}

// Deduct 不透支扣费（幂等、原因必填）。
func (s *BalanceLedgerService) Deduct(ctx context.Context, cmd *LedgerDeductCommand) (*LedgerDeductResult, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("balance ledger: nil repo")
	}
	if cmd == nil {
		return nil, ErrLedgerInvalidAmount
	}
	cmd.Description = strings.TrimSpace(cmd.Description)
	if cmd.Amount <= 0 {
		return nil, ErrLedgerInvalidAmount
	}
	if cmd.Description == "" {
		return nil, ErrLedgerDescriptionRequired
	}

	res, err := s.repo.Deduct(ctx, cmd)
	if err != nil {
		return nil, err
	}
	// 提交后同步失效缓存：下次 GetBalance / 网关 preflight 立刻读到新余额。
	s.invalidateBalance(ctx, cmd.UserID)
	return res, nil
}

// Refund 部分退（凭原流水冲销、幂等、原因必填）。
func (s *BalanceLedgerService) Refund(ctx context.Context, cmd *LedgerRefundCommand) (*LedgerRefundResult, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("balance ledger: nil repo")
	}
	if cmd == nil {
		return nil, ErrLedgerInvalidAmount
	}
	cmd.Description = strings.TrimSpace(cmd.Description)
	if cmd.Amount <= 0 {
		return nil, ErrLedgerInvalidAmount
	}
	if cmd.Description == "" {
		return nil, ErrLedgerDescriptionRequired
	}

	res, err := s.repo.Refund(ctx, cmd)
	if err != nil {
		return nil, err
	}
	s.invalidateBalance(ctx, res.UserID)
	return res, nil
}

// GetBalance 返回用户当前余额（缓存优先，未命中回源 DB）。
func (s *BalanceLedgerService) GetBalance(ctx context.Context, userID int64) (float64, error) {
	if s == nil || s.cache == nil {
		return 0, fmt.Errorf("balance ledger: nil cache")
	}
	return s.cache.GetUserBalance(ctx, userID)
}

// AppStats 返回某接入方的累计扣/退统计。
func (s *BalanceLedgerService) AppStats(ctx context.Context, appID string) (*AppLedgerStats, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("balance ledger: nil repo")
	}
	return s.repo.AppStats(ctx, appID)
}

func (s *BalanceLedgerService) invalidateBalance(ctx context.Context, userID int64) {
	if s.cache == nil || userID == 0 {
		return
	}
	if err := s.cache.InvalidateUserBalance(ctx, userID); err != nil {
		logger.LegacyPrintf("service.balance_ledger", "Warning: invalidate balance cache failed for user %d: %v", userID, err)
	}
}

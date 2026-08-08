package web3deposit

import (
	"context"
	"errors"
	"time"
)

type AdminDepositFilter struct {
	Status        DepositStatus
	UserID        int64
	Address       string
	TxHash        string
	CreatedAtFrom *time.Time
	CreatedAtTo   *time.Time
	Page          int
	PageSize      int
}

type AdminDepositReader interface {
	ListAdminDeposits(ctx context.Context, filter AdminDepositFilter) ([]Deposit, int64, error)
	GetAdminDeposit(ctx context.Context, depositID int64) (Deposit, error)
	CountAdminDepositsByStatus(ctx context.Context) (map[DepositStatus]int64, error)
}

type AdminDepositOperator interface {
	ApproveReviewedDeposit(ctx context.Context, depositID int64) error
	IgnoreReviewedDeposit(ctx context.Context, depositID int64, reason string) error
}

var ErrAdminDepositStateConflict = errors.New("web3 deposit state does not allow admin operation")

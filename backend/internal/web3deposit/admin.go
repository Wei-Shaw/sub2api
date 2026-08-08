package web3deposit

import (
	"context"
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
